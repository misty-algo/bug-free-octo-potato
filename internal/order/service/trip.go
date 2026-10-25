package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// TripService 承载 D06 的核验、开始、送达与轨迹采集。
// 与 D05 一样不依赖多文档事务：每个命令只原子更新订单文档，
// 跨文档一致性由 version 守卫与幂等命令记录收敛。
type TripService struct {
	repo        entity.TripRepository
	clock       clock.Clock
	hmacSecret  string
	thresholds  entity.SampleThresholds
	eligibility EligibilityGate
	log         *zap.Logger
}

func NewTripService(r entity.TripRepository, c clock.Clock, hmacSecret string) *TripService {
	return &TripService{
		repo: r, clock: c, hmacSecret: hmacSecret,
		thresholds: entity.DefaultSampleThresholds(), log: zap.NewNop(),
	}
}

// EligibilityGate 由司机域实现：末单结束时按最新准入决定收车状态。
// 只有"能接新任务"才置空闲，否则收车；途中到期从不打断行程。
type EligibilityGate interface {
	CanAcceptNewTask(ctx context.Context, driverID string) (bool, error)
}

// SetEligibilityGate 注入准入门禁。未注入时保持原有的无条件 idle 行为。
func (s *TripService) SetEligibilityGate(g EligibilityGate, log *zap.Logger) {
	s.eligibility = g
	if log != nil {
		s.log = log
	}
}

func (s *TripService) SetSampleThresholds(t entity.SampleThresholds) { s.thresholds = t }

// codeDigest 用服务端密钥把「订单号 + 尾号四位」摘要成比对值。
// 不落库、不回显，也避免存一个可被离线枚举的普通哈希。
func (s *TripService) codeDigest(orderNo, code string) string {
	mac := hmac.New(sha256.New, []byte(s.hmacSecret))
	mac.Write([]byte(orderNo))
	mac.Write([]byte(":"))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}

func isDigits(v string) bool {
	if v == "" {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Verify 是乘客上车核验（接口文档 3.9.2）。
// 身份来自令牌；比对只在服务端完成，不返回正确答案，也不记录输入值。
func (s *TripService) Verify(ctx context.Context, cmd dto.VerifyCommand) (*dto.VerifyResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.OrderID = strings.TrimSpace(cmd.OrderID)
	if cmd.OrderID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "order_id is required")
	}
	code := strings.TrimSpace(cmd.Code)
	if len(code) != entity.VerifyCodeLength || !isDigits(code) {
		return nil, apperrors.InvalidArgument("INVALID_VERIFY_CODE", "code must be the last four digits of the passenger phone number")
	}
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}
	order, err := loadOrderByRef(ctx, s.repo, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this order")
	}
	now := utcMillis(s.clock.Now())
	// 幂等必须先于状态判定：重放同一个命令键要返回原结果，而不是被当前状态挡住。
	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "VERIFY", ResourceID: order.OrderNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.VerifyResult](saved, hash); err != nil || result != nil {
		return result, err
	}
	// 锁定中拒绝且不延长锁定。
	if order.VerifyLockedUntil != nil && now.Before(*order.VerifyLockedUntil) {
		return nil, apperrors.Conflict("VERIFY_LOCKED", "verification is locked; ask the passenger or contact support")
	}
	// 已开始、已结束、已取消的请求不消耗该乘客的有效核验次数。
	switch strings.ToUpper(order.State) {
	case entity.OrderArrived:
	case entity.OrderInTrip, entity.OrderEnded:
		return nil, apperrors.Conflict("INVALID_ORDER_STATE", "order already started")
	case entity.OrderCancelled:
		return nil, apperrors.Conflict("INVALID_ORDER_STATE", "order is cancelled")
	default:
		return nil, apperrors.Conflict("INVALID_ORDER_STATE", "order is not waiting for boarding")
	}

	tail, err := s.repo.PassengerPhoneTail(ctx, order.PassengerID)
	if err != nil {
		// 身份服务超时是可恢复错误，不计入输错次数。
		return nil, err
	}
	if tail == "" {
		return nil, apperrors.NotFound("PASSENGER_NOT_FOUND", "passenger phone tail is unavailable")
	}
	matched := hmac.Equal([]byte(s.codeDigest(order.OrderNo, tail)), []byte(s.codeDigest(order.OrderNo, code)))

	failures := order.VerifyFailures
	var lockedUntil *time.Time
	if matched {
		failures = 0
	} else {
		failures++
		if failures >= entity.VerifyMaxAttempts {
			until := now.Add(entity.VerifyLockDuration)
			lockedUntil = &until
		}
	}
	remaining := entity.VerifyMaxAttempts - failures
	if remaining < 0 {
		remaining = 0
	}
	result := &dto.VerifyResult{OrderID: order.OrderNo, Verified: matched, RemainingAttempts: remaining, LockUntil: lockedUntil}

	mutation := entity.VerifyMutation{Order: *order, Verified: matched, Failures: failures, LockedUntil: lockedUntil}
	mutation.Order.Version++
	mutation.Order.Verified = matched
	mutation.Order.VerifyFailures = failures
	mutation.Order.VerifyLockedUntil = lockedUntil
	if matched {
		mutation.Timeline = entity.TimelineItem{Name: "乘客已核验", Desc: "核验通过，可以开始行程", T: now}
		mutation.Events = append(mutation.Events, entity.EventLog{EventID: "VERIFY_OK:" + uuid.NewString(), T: now, Name: "PassengerVerified", Detail: order.OrderNo + " 核验通过", Kind: "trip"})
	} else {
		// 审计只记录第几次失败，绝不记录输入值。
		mutation.Events = append(mutation.Events, entity.EventLog{EventID: "VERIFY_FAIL:" + uuid.NewString(), T: now, Name: "PassengerVerifyFailed", Detail: order.OrderNo + " 核验失败（不记录输入值）", Kind: "audit"})
	}
	mutation.Command, err = commandRecord(key, hash, order.OrderNo, result, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveVerify(ctx, mutation); err != nil {
		return nil, err
	}
	return result, nil
}

// Start 是开始行程（接口文档 3.9.3）。拼车要求所有未取消子单均已核验；
// 等待满 5 分钟仍未上车者必须先走有证据的取消/爽约，不能隐藏乘客直接发车。
func (s *TripService) Start(ctx context.Context, cmd dto.StartCommand) (*dto.StartResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.TaskID = strings.TrimSpace(cmd.TaskID)
	if cmd.TaskID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "task_id is required")
	}
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}
	task, err := loadTaskByRef(ctx, s.repo, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	if task.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this task")
	}
	switch normalizeTaskState(task.State) {
	case entity.TaskAssigned, entity.TaskActive:
	case entity.TaskFinished, "CANCELLED":
		return nil, apperrors.Conflict("INVALID_TASK_STATE", "task is already finished")
	default:
		return nil, apperrors.Conflict("INVALID_TASK_STATE", "task is not assigned to a driver")
	}
	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "START", ResourceID: task.TaskNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.StartResult](saved, hash); err != nil || result != nil {
		return result, err
	}

	orders, err := s.repo.OrdersByIDs(ctx, task.OrderIDs)
	if err != nil {
		return nil, err
	}
	now := utcMillis(s.clock.Now())
	pending := make([]entity.RideOrder, 0, len(orders))
	for _, order := range orders {
		switch strings.ToUpper(order.State) {
		case entity.OrderCancelled, entity.OrderEnded, entity.OrderInTrip:
			continue
		}
		if !order.Verified {
			if order.ArrivedAt != nil && now.Sub(*order.ArrivedAt) >= entity.BoardWaitLimit {
				return nil, apperrors.Conflict("BOARD_TIMEOUT", "passenger has not boarded within the waiting limit; cancel or no-show first")
			}
			return nil, apperrors.Conflict("VERIFY_REQUIRED", "every non-cancelled order must be verified before departure")
		}
		// 缺失必要价格快照则保持 ARRIVED 并提示服务异常，不伪造计价版本。
		if order.PriceSnap == nil || order.CfgVersion == nil {
			return nil, apperrors.Conflict("PRICE_SNAPSHOT_MISSING", "fare snapshot is missing; order stays ARRIVED")
		}
		order.Version++
		order.State = entity.OrderInTrip
		order.BoardedAt = &now
		order.StartedAt = &now
		pending = append(pending, order)
	}
	if len(pending) == 0 {
		return nil, apperrors.Conflict("NO_STARTABLE_ORDERS", "task has no order ready to start")
	}

	result := &dto.StartResult{TaskID: task.TaskNo, State: entity.TaskActive, DepartedAt: now}
	mutation := entity.StartMutation{TaskNo: task.TaskNo, DepartedAt: now, Orders: pending}
	mutation.Session = entity.TripSession{TaskNo: task.TaskNo, SessionID: uuid.NewString(), DriverID: task.DriverID, State: entity.SessionActive, CreatedAt: now, UpdatedAt: now}
	mutation.Events = append(mutation.Events, entity.EventLog{EventID: "DEPART:" + uuid.NewString(), T: now, Name: "TripStarted", Detail: task.TaskNo + " 发车", Kind: "trip"})
	mutation.Command, err = commandRecord(key, hash, task.TaskNo, result, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveStart(ctx, mutation); err != nil {
		return nil, err
	}
	return result, nil
}

// End 是确认送达（接口文档 3.9.4）。结束点由服务端可信时间确定，
// 不接受客户端时间追溯减少车费；计费输入先落 COLLECTING，轨迹齐备后才置 READY。
func (s *TripService) End(ctx context.Context, cmd dto.EndCommand) (*dto.EndResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.OrderID = strings.TrimSpace(cmd.OrderID)
	if cmd.OrderID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "order_id is required")
	}
	if cmd.ActualDist < 0 || cmd.ActualDur < 0 {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "actual_dist and actual_dur must not be negative")
	}
	cmd.EndReason = strings.TrimSpace(cmd.EndReason)
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}
	order, err := loadOrderByRef(ctx, s.repo, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this order")
	}
	now := utcMillis(s.clock.Now())
	// 幂等必须先于状态判定：送达后重放同一命令键要返回原账单投影。
	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "END", ResourceID: order.OrderNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.EndResult](saved, hash); err != nil || result != nil {
		return result, err
	}
	if strings.ToUpper(order.State) != entity.OrderInTrip {
		return nil, apperrors.Conflict("INVALID_ORDER_STATE", "order is not in trip")
	}

	billingRequestID := "BR:" + uuid.NewString()
	total := int64(0)
	if order.PriceSnap != nil {
		total = order.PriceSnap.T
	}
	result := &dto.EndResult{OrderID: order.OrderNo, State: entity.OrderEnded, Bill: dto.BillProjection{T: total, P: total, State: "CALCULATING"}}

	mutation := entity.EndMutation{
		Order:            *order,
		EndedAt:          now,
		ActualDist:       cmd.ActualDist,
		ActualDur:        cmd.ActualDur,
		EndReason:        cmd.EndReason,
		BillingRequestID: billingRequestID,
		InputState:       entity.BillingCollecting,
		DriverID:         order.DriverID,
	}
	mutation.Order.Version++
	mutation.Order.State = entity.OrderEnded
	mutation.Order.EndedAt = &now
	mutation.Order.ActualDist = &cmd.ActualDist
	mutation.Order.ActualDur = &cmd.ActualDur
	mutation.Order.BillingRequestID = billingRequestID
	mutation.Order.BillingInputState = entity.BillingCollecting
	description := "行程结束，账单核算中"
	if cmd.EndReason != "" {
		description = "提前结束：" + cmd.EndReason
	}
	mutation.Timeline = entity.TimelineItem{Name: "行程结束", Desc: description, T: now}
	// TripEnded 只告知结束，不授权计价按尚未齐全的数据出正式金额。
	mutation.Events = append(mutation.Events, entity.EventLog{EventID: "END:" + uuid.NewString(), T: now, Name: "TripEnded", Detail: order.OrderNo + " " + description, Kind: "trip"})

	// 运力释放：任务上最后一个活动子单终结时才释放一次。
	if order.DispatchID != bson.NilObjectID {
		if task, err := s.repo.TaskByID(ctx, order.DispatchID); err == nil && task != nil {
			taskOrders, err := s.repo.OrdersByIDs(ctx, task.OrderIDs)
			if err == nil {
				active := 0
				for _, o := range taskOrders {
					if o.ID == order.ID {
						continue
					}
					switch strings.ToUpper(o.State) {
					case entity.OrderCancelled, entity.OrderEnded:
					default:
						active++
					}
				}
				mutation.ReleaseDriver = active == 0
			}
		}
	}
	// 收车判定：最后一个活动子单结束时按最新准入决定，途中到期不打断。
	if mutation.ReleaseDriver && s.eligibility != nil {
		driverID := order.DriverID.Hex()
		ok, err := s.eligibility.CanAcceptNewTask(ctx, driverID)
		if err != nil {
			// 门禁不可用不能阻止送达；降级为置空闲并由司机主动出车时再判。
			s.log.Error("check eligibility on release failed",
				zap.String("driver_id", driverID), zap.Error(err))
		} else if !ok {
			mutation.DriverBlocked = true
		}
	}

	mutation.Command, err = commandRecord(key, hash, billingRequestID, result, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveEnd(ctx, mutation); err != nil {
		return nil, err
	}
	return result, nil
}

// RecordSample 是轨迹样本上报（接口文档 3.9.5 扩展）。
// 浏览器通常返回 WGS84，服务端统一转换为 GCJ02；转换不可靠的样本拒绝用于
// 到达及计费，但保留证据。taskNo+sessionId+seq 去重，重放不重复累计里程。
func (s *TripService) RecordSample(ctx context.Context, cmd dto.SampleCommand) (*dto.SampleResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if math.IsNaN(cmd.Lng) || math.IsNaN(cmd.Lat) || cmd.Lng < -180 || cmd.Lng > 180 || cmd.Lat < -90 || cmd.Lat > 90 {
		return nil, apperrors.InvalidArgument("INVALID_LOCATION", "finite coordinates in geographic range are required")
	}
	cmd.TaskID = strings.TrimSpace(cmd.TaskID)
	cmd.SessionID = strings.TrimSpace(cmd.SessionID)
	if cmd.TaskID == "" || cmd.SessionID == "" || cmd.Seq < 1 {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "task_id, session_id and a positive seq are required")
	}
	task, err := loadTaskByRef(ctx, s.repo, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	if task.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this task")
	}
	driverID, err := objectID(cmd.Principal.UserID)
	if err != nil {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "token driver id is not a valid ObjectId")
	}

	now := utcMillis(s.clock.Now())
	collectedAt := now
	if cmd.CollectedAt != nil {
		collectedAt = utcMillis(*cmd.CollectedAt)
	}
	lng, lat := cmd.Lng, cmd.Lat
	converted := false
	note := ""
	switch strings.ToUpper(strings.TrimSpace(cmd.CRS)) {
	case "GCJ02", "":
	case "WGS84":
		lng, lat, converted = entity.WGS84ToGCJ02(lng, lat)
		if !converted {
			note = "坐标系转换不可靠，样本不用于到达及计费"
		}
	default:
		return nil, apperrors.InvalidArgument("INVALID_LOCATION", "crs must be WGS84 or GCJ02")
	}

	point := entity.TripPoint{
		TaskNo:      task.TaskNo,
		SessionID:   cmd.SessionID,
		Seq:         cmd.Seq,
		CollectedAt: collectedAt,
		ReceivedAt:  now,
		CRS:         "GCJ02",
		Accuracy:    cmd.Accuracy,
		Location:    entity.GeoPoint{Type: "Point", Coordinates: []float64{lng, lat}},
		Note:        note,
	}
	point.Quality = entity.EvaluateSample(nil, point, s.thresholds)
	if !converted && strings.ToUpper(strings.TrimSpace(cmd.CRS)) == "WGS84" {
		point.Quality = entity.SampleRejected
	}

	previous, err := s.repo.Session(ctx, task.TaskNo, cmd.SessionID)
	if err != nil {
		return nil, err
	}
	updateLatest := previous == nil || cmd.Seq > previous.LastSeq
	mutation := entity.SampleMutation{Point: point, SessionID: cmd.SessionID, TaskNo: task.TaskNo, UpdateLatest: updateLatest, DriverID: driverID, LatestAt: now}
	session, err := s.repo.SaveSample(ctx, mutation)
	if err != nil {
		return nil, err
	}
	result := &dto.SampleResult{Accepted: true, TaskID: task.TaskNo, SessionID: cmd.SessionID, Seq: cmd.Seq, Watermark: session.Watermark, Quality: point.Quality, Converted: converted}
	// 已持久化过的序号只确认，不重复累计里程。
	if previous != nil && cmd.Seq <= previous.LastSeq {
		result.Duplicated = true
	}
	for seq := session.Watermark + 1; seq < session.LastSeq; seq++ {
		result.Missing = append(result.Missing, seq)
	}
	return result, nil
}
