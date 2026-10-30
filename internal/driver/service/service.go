package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	"github.com/BwCloudWeGo/bw-cli/pkg/geo"
)

const localConsumer = "driver-service"

// DispatchWeights 派单评分权重（来源：后台 A06 配置，四项权重总和必须等于 10000）。
type DispatchWeights struct {
	Distance int
	Rating   int
	Online   int
	Success  int
}

// BusinessConfig 司机端业务参数（由 configs/config.yaml 的 driver 段注入）。
// 金额单位一律为「分」。
type BusinessConfig struct {
	CommissionRate        float64 // 平台抽成比例（0–1）
	MinWithdrawFen        int64   // 最低提现门槛
	MaxWithdrawFen        int64   // 单笔提现上限
	DailyWithdrawLimitFen int64   // 单日提现累计上限
	DailyWithdrawCount    int     // 单日提现受理次数
	DualApprovalFen       int64   // 达到该金额需双人审批
	DailyReleaseLimit     int     // 单日退单次数
	AcceptTimeoutSec      int     // 邀请确认超时
	ListenWindowSec       int     // 听单通道窗口（之后转抢单池）
	DispatchWindowSec     int     // 总派单时限（超时平台责任取消）
	HeartbeatTimeoutSec   int     // 空闲无心跳自动收车
	HallListLimit         int     // 大厅列表上限
	Weights               DispatchWeights
}

// Service 编排司机端用例：接单大厅（D01–D04）、首页收益对比（D09–D11）、收入结算（D12–D13）。
type Service struct {
	drivers   entity.DriverRepository
	orders    entity.HallOrderRepository
	incomes   entity.IncomeRepository
	support   entity.SupportRepository
	codes     entity.DriverCodeRepository
	cfg       BusinessConfig
	auth      AuthConfig
	log       *zap.Logger
	publisher func(ctx context.Context, routingKey string, body []byte) error
}

// NewService 创建司机端用例服务。
func NewService(drivers entity.DriverRepository, orders entity.HallOrderRepository,
	incomes entity.IncomeRepository, support entity.SupportRepository, cfg BusinessConfig, log *zap.Logger) *Service {
	return &Service{drivers: drivers, orders: orders, incomes: incomes, support: support, cfg: cfg, log: log}
}

// WithAuth 注入司机登录所需的验证码仓储与令牌密钥，返回同一实例便于链式配置。
func (s *Service) WithAuth(codes entity.DriverCodeRepository, auth AuthConfig) *Service {
	s.codes = codes
	s.auth = auth
	return s
}

// SyncDriverNoSequence 用库内已有最大编号校准内存序列，避免重启后编号重复。
func (s *Service) SyncDriverNoSequence(ctx context.Context) {
	max, err := s.drivers.MaxDriverNo(ctx)
	if err != nil {
		s.log.Warn("read max driver_no failed", zap.Error(err))
		return
	}
	entity.PrimeDriverNoSequence(max)
	if max != "" {
		s.log.Info("driver no sequence synced", zap.String("max_driver_no", max))
	}
}

// ───────────────────────── 司机档案 ─────────────────────────

// RegisterDriver 注册司机，默认空闲 + 抢单模式。
func (s *Service) RegisterDriver(ctx context.Context, cmd dto.RegisterDriverCommand) (*dto.DriverProfileDTO, error) {
	driver, err := entity.NewDriver(cmd.Name, cmd.Phone)
	if err != nil {
		return nil, err
	}
	if err := s.drivers.Create(ctx, driver); err != nil {
		return nil, err
	}
	s.emit(ctx, &contracts.DriverStatusChangedPayload{
		EventID:  "dsc-" + driver.ID + "-" + fmt.Sprint(driver.CreatedAt.UnixNano()),
		DriverID: driver.ID, FromStatus: "", ToStatus: driver.Status, Reason: "register",
		OccurredAt: biztime.RFC3339(driver.CreatedAt),
	}, contracts.EventDriverStatusChanged)
	return dto.FromDriver(driver), nil
}

// GetDriverProfile 查询司机档案。
func (s *Service) GetDriverProfile(ctx context.Context, driverID string) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, driverID)
	if err != nil {
		return nil, err
	}
	return dto.FromDriver(driver), nil
}

// UpdateStatus 收车 / 出车。忙碌与接单预占状态不允许直接流转。
func (s *Service) UpdateStatus(ctx context.Context, cmd dto.UpdateStatusCommand) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if err := driver.EnsureVersion(cmd.Version); err != nil {
		return nil, err
	}
	from := driver.Status
	if err := driver.SetStatus(cmd.Status); err != nil {
		return nil, err
	}
	if cmd.Status == entity.DriverStatusIdle {
		driver.TouchHeartbeat(biztime.UTC(), 0, 0)
	}
	if err := s.drivers.UpdateStatus(ctx, driver.ID, driver.Status, cmd.Version, driver.UpdatedAt); err != nil {
		return nil, err
	}
	s.emit(ctx, &contracts.DriverStatusChangedPayload{
		EventID:  "dsc-" + driver.ID + "-" + fmt.Sprint(driver.UpdatedAt.UnixNano()),
		DriverID: driver.ID, FromStatus: from, ToStatus: driver.Status,
		Reason: "driver_operated", OccurredAt: biztime.RFC3339(driver.UpdatedAt),
	}, contracts.EventDriverStatusChanged)
	return dto.FromDriver(driver), nil
}

// UpdateMode 听单 / 抢单互斥切换：空闲才可切换，且递增模式版本使旧邀请立即失效。
func (s *Service) UpdateMode(ctx context.Context, cmd dto.UpdateModeCommand) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if err := driver.EnsureVersion(cmd.Version); err != nil {
		return nil, err
	}
	prevModeVersion := driver.ModeVersion
	if err := driver.SetMode(cmd.Mode); err != nil {
		return nil, err
	}
	if prevModeVersion != driver.ModeVersion {
		// 切换模式：清空本司机待确认的推送（旧邀请随版本递增失效）。
		if released, err := s.orders.ReleasePendingForDriver(ctx, driver.ID); err == nil && released > 0 {
			driver.ActiveTaskID = ""
			driver.Status = entity.DriverStatusIdle
		}
	}
	if err := s.drivers.UpdateMode(ctx, driver.ID, driver.Mode, cmd.Version, driver.ModeVersion, driver.UpdatedAt); err != nil {
		return nil, err
	}
	return dto.FromDriver(driver), nil
}

// UpdateAreaConfig 修改接单区域偏好（仅空闲允许）。
func (s *Service) UpdateAreaConfig(ctx context.Context, cmd dto.UpdateAreaCommand) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if err := driver.EnsureVersion(cmd.Version); err != nil {
		return nil, err
	}
	if err := driver.UpdateAreaConfig(cmd.SwitchOn, cmd.Areas); err != nil {
		return nil, err
	}
	if err := s.drivers.UpdateAreaConfig(ctx, driver.ID, driver.AreaLimitSwitch, driver.Areas, cmd.Version, driver.UpdatedAt); err != nil {
		return nil, err
	}
	return dto.FromDriver(driver), nil
}

// Heartbeat 司机心跳上报：空闲连续超时未上报则自动收车（忙碌掉线保留任务）。
func (s *Service) Heartbeat(ctx context.Context, cmd dto.HeartbeatCommand) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	now := biztime.UTC()
	driver.TouchHeartbeat(now, cmd.Lat, cmd.Lng)
	if err := s.drivers.UpdateHeartbeat(ctx, driver.ID, now, driver.LastLat, driver.LastLng, 0, driver.UpdatedAt); err != nil {
		return nil, err
	}
	return dto.FromDriver(driver), nil
}

// SetOfflineAfterTrip 设置"本单后收车"。
func (s *Service) SetOfflineAfterTrip(ctx context.Context, cmd dto.OfflineAfterTripCommand) (*dto.DriverProfileDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if err := driver.EnsureVersion(cmd.Version); err != nil {
		return nil, err
	}
	driver.SetOfflineAfterTrip(cmd.On)
	if err := s.drivers.UpdateOfflineAfterTrip(ctx, driver.ID, cmd.On, cmd.Version, driver.UpdatedAt); err != nil {
		return nil, err
	}
	return dto.FromDriver(driver), nil
}

// ───────────────────────── 接单大厅 ─────────────────────────

// CreateTask 创建派单任务：乘客域下单事件 / 模拟下单共用。
// 单通道分发先走听单窗口，无听单运力时直接转抢单池曝光。
func (s *Service) CreateTask(ctx context.Context, cmd dto.CreateTaskCommand) (*dto.HallOrderDTO, error) {
	commission := int64(math.Round(float64(cmd.PassengerPaidFen) * s.cfg.CommissionRate)) //计算佣金
	order, err := entity.NewHallOrder(entity.CreateHallOrderParam{
		OrderID: cmd.OrderID, PassengerName: cmd.PassengerName, PassengerCount: cmd.PassengerCount,
		CityCode: cmd.CityCode, OrderType: cmd.OrderType,
		RequireSeats: cmd.RequireSeats, RequireCarType: cmd.RequireCarType,
		Origin:      entity.Location{Name: cmd.OriginName, Lat: cmd.OriginLat, Lng: cmd.OriginLng},
		Destination: entity.Location{Name: cmd.DestName, Lat: cmd.DestLat, Lng: cmd.DestLng},
		OrigFareFen: cmd.OrigFareFen, PassengerPaidFen: cmd.PassengerPaidFen,
		PlatformCouponFen: cmd.PlatformCouponFen, DriverCouponFen: cmd.DriverCouponFen,
		CommissionFen: commission, SubsidyFen: cmd.SubsidyFen,
		ListenWindowSec: s.cfg.ListenWindowSec, TotalWindowSec: s.cfg.DispatchWindowSec,
		AcceptTimeoutSec: s.cfg.AcceptTimeoutSec,
	})
	if err != nil {
		return nil, err
	}
	if err := s.orders.Create(ctx, order); err != nil {
		return nil, err
	}
	// 先尝试听单派发（并决定是否立即曝光到抢单池）。
	s.dispatchByListen(ctx, order)
	latest, err := s.orders.Get(ctx, order.ID)
	if err != nil {
		return dto.FromHallOrder(order), nil
	}
	return dto.FromHallOrder(latest), nil
}

// ListHallOrders 抢单大厅列表：按接单区域过滤 + 显示接驾距离。
func (s *Service) ListHallOrders(ctx context.Context, driverID string, lat, lng float64, limit int) ([]*dto.HallOrderDTO, int64, error) {
	driver, err := s.drivers.Get(ctx, driverID)
	if err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > s.cfg.HallListLimit {
		limit = s.cfg.HallListLimit
	}
	list, err := s.orders.ListPool(ctx, driver, lat, lng, limit)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.HallOrderDTO, 0, len(list))
	for _, o := range list {
		if !driver.InArea(o.Origin.Lat, o.Origin.Lng) {
			continue // 区域偏好：起点不在片区内不可见
		}
		if o.DriverID != "" && !o.InviteExpired(biztime.UTC()) {
			continue // 已被他人占用（邀请未超时）
		}
		o.PickupDistanceMeters = o.DistanceTo(lat, lng)
		out = append(out, dto.FromHallOrder(o))
	}
	return out, int64(len(out)), nil
}

// GrabOrder 抢单：原子预占任务与司机 15 秒（速度为王，本质是数据库条件更新）。
func (s *Service) GrabOrder(ctx context.Context, cmd dto.GrabOrderCommand) (*dto.HallOrderDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if err := driver.CanReceiveOrder(); err != nil {
		return nil, err
	}
	order, err := s.orders.Get(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	now := biztime.UTC()
	if !order.VisibleToGrabPool(now) {
		// 听单窗口内且没有生效邀请（无人听单）时，允许立即转入抢单池，避免运力空转；
		// 已有司机持有未超时邀请时，其他司机不可见、不可抢。
		noActiveInvite := order.DriverID == "" || order.InviteExpired(now)
		if order.Status == entity.OrderStatusWaiting && !order.VisibleInPool && noActiveInvite {
			if switched, sErr := s.orders.OpenToGrabPool(ctx, order.ID); sErr == nil {
				order = switched
			}
		}
	}
	if !order.VisibleToGrabPool(now) {
		return nil, entity.ErrOrderNotVisible
	}
	if err := order.EnsureVersion(0); err != nil {
		return nil, err
	}
	assigned, err := s.orders.Assign(ctx, order.ID, driver.ID, driver.Name, 0, now.Add(s.acceptTimeout()), order.DistanceTo(order.Origin.Lat, order.Origin.Lng))
	if err != nil {
		return nil, err
	}
	if err := s.bindPreOccupied(ctx, driver, assigned.TaskID); err != nil {
		// 绑定失败回滚占用，保证"一任务一司机 / 一司机一活动任务"都成立。
		_, _ = s.orders.ReleaseToWaiting(ctx, assigned.ID, driver.ID, contracts.PartyPlatform, "driver_bind_failed")
		return nil, err
	}
	s.emit(ctx, &contracts.TaskAssignedPayload{
		EventID: "tas-" + assigned.ID + "-" + fmt.Sprint(assigned.InviteRound),
		TaskID:  assigned.TaskID, OrderID: assigned.OrderID, DriverID: driver.ID,
		Channel: entity.DispatchChannelGrabPool, InviteRound: assigned.InviteRound,
		AcceptDeadline: biztime.RFC3339(assigned.AcceptDeadline), OccurredAt: biztime.RFC3339(now),
	}, contracts.EventTaskAssigned)
	return dto.FromHallOrder(assigned), nil
}

// AcceptOrder 确认接单：只有被邀请/抢占到的司机在 15 秒内确认有效。
func (s *Service) AcceptOrder(ctx context.Context, cmd dto.AcceptOrderCommand) (*dto.HallOrderDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	accepted, err := s.orders.ConfirmAccept(ctx, cmd.OrderID, driver.ID, cmd.Version)
	if err != nil {
		return nil, err
	}
	if err := s.bindBusy(ctx, driver, accepted.TaskID); err != nil {
		// 回滚：司机绑定失败时把订单退回等待状态并释放该司机运力，避免卡在预占态。
		_, _ = s.orders.ReleaseToWaiting(ctx, accepted.ID, driver.ID, contracts.PartyPlatform, "driver_bind_failed")
		_ = s.drivers.UpdateTaskBinding(ctx, driver.ID, entity.DriverStatusIdle, "", 0, biztime.UTC())
		return nil, err
	}
	s.emit(ctx, &contracts.TaskAcceptedPayload{
		EventID: "tac-" + accepted.ID, TaskID: accepted.TaskID, OrderID: accepted.OrderID,
		DriverID: driver.ID, DriverName: driver.Name, AcceptedAt: biztime.RFC3339(accepted.AcceptedAt),
	}, contracts.EventTaskAccepted)
	return dto.FromHallOrder(accepted), nil
}

// ReleaseOrder 退单：仅未开始行程可退；司机责任计入每日 3 次上限，退单后按原通道重派（继承剩余预算）。
func (s *Service) ReleaseOrder(ctx context.Context, cmd dto.ReleaseOrderCommand) (*dto.HallOrderDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	order, err := s.orders.Get(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if err := order.CanRelease(driver.ID); err != nil {
		return nil, err
	}
	party := stringsDefault(cmd.ResponsibleParty, contracts.PartyDriver)
	if party == contracts.PartyDriver {
		//司机退单次数
		count, err := s.drivers.CountDriverReleasesToday(ctx, driver.ID, biztime.DayStart(biztime.Now()))
		if err != nil {
			return nil, err
		}
		if count >= int64(s.cfg.DailyReleaseLimit) {
			return nil, entity.ErrReleaseLimitExceeded
		}
	}
	released, err := s.orders.ReleaseToWaiting(ctx, order.ID, driver.ID, party, stringsDefault(cmd.Reason, "driver_released"))
	if err != nil {
		return nil, err
	}
	_ = s.drivers.RecordRelease(ctx, driver.ID, party, biztime.UTC())
	_ = s.releaseDriver(ctx, driver, false)
	s.emit(ctx, &contracts.TaskReleasedPayload{
		EventID: "tre-" + released.ID + "-" + fmt.Sprint(released.InviteRound+1),
		TaskID:  released.TaskID, OrderID: released.OrderID, DriverID: driver.ID,
		Reason: released.CancelReason, ResponsibleParty: party, ReleasedAt: biztime.RFC3339(biztime.UTC()),
	}, contracts.EventTaskReleased)
	// 继承剩余派单预算重新派发。
	if released.ListenDeadline.After(biztime.UTC()) {
		s.dispatchByListen(ctx, released)
	}
	return dto.FromHallOrder(released), nil
}

// StartTrip 开始行程（行程域/司机端调用）：ACCEPTED → IN_TRIP。
func (s *Service) StartTrip(ctx context.Context, cmd dto.StartTripCommand) (*dto.HallOrderDTO, error) {
	started, err := s.orders.StartTrip(ctx, cmd.OrderID, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	return dto.FromHallOrder(started), nil
}

// EndTrip 送达结束：行程进入 ENDED 并释放运力。
// 行程结束 ≠ 收入到账：司机记账由支付成功事件驱动（ReportPaymentSucceeded），
// 乘客未支付时订单停在待支付，司机完单量与收入不提前计入。
func (s *Service) EndTrip(ctx context.Context, cmd dto.EndTripCommand) (*dto.HallOrderDTO, error) {
	ended, err := s.orders.Complete(ctx, cmd.OrderID, cmd.DriverID, 0, cmd.DistanceMeters, cmd.DurationSec)
	if err != nil {
		return nil, err
	}
	driver, dErr := s.drivers.Get(ctx, cmd.DriverID)
	if dErr == nil {
		_ = s.releaseDriver(ctx, driver, driver.OfflineAfterTrip)
	}
	s.emit(ctx, &contracts.TripEndedPayload{
		EventID: stringsDefault(cmd.EventID, "tte-"+ended.ID), TaskID: ended.TaskID, OrderID: ended.OrderID,
		DriverID: ended.DriverID, EndedAt: biztime.RFC3339(ended.EndedAt),
		DurationSec: cmd.DurationSec, DistanceMeters: cmd.DistanceMeters, Amount: ended.Amount(),
	}, contracts.EventTripEnded)
	return dto.FromHallOrder(ended), nil
}

// CancelByExternal 乘客取消 / 平台责任取消（由乘客域事件触发）：不计司机退单次数。
func (s *Service) CancelByExternal(ctx context.Context, orderID, party, reason string) (*dto.HallOrderDTO, error) {
	cancelled, err := s.orders.Cancel(ctx, orderID, stringsDefault(party, contracts.PartyPassenger), reason)
	if err != nil {
		return nil, err
	}
	if cancelled.DriverID != "" {
		if driver, err := s.drivers.Get(ctx, cancelled.DriverID); err == nil {
			_ = s.releaseDriver(ctx, driver, false)
		}
	}
	return dto.FromHallOrder(cancelled), nil
}

// ───────────────────────── 内部辅助 ─────────────────────────

func (s *Service) acceptTimeout() time.Duration {
	sec := s.cfg.AcceptTimeoutSec
	if sec <= 0 {
		sec = 15
	}
	return time.Duration(sec) * time.Second
}

// bindPreOccupied 把司机置为接单预占（活动任务绑定）。
func (s *Service) bindPreOccupied(ctx context.Context, driver *entity.Driver, taskID string) error {
	if err := driver.ToPreOccupied(taskID); err != nil {
		return err
	}
	return s.drivers.UpdateTaskBinding(ctx, driver.ID, driver.Status, driver.ActiveTaskID, 0, driver.UpdatedAt)
}

// bindBusy 把司机置为忙碌。
func (s *Service) bindBusy(ctx context.Context, driver *entity.Driver, taskID string) error {
	if err := driver.ToBusy(taskID); err != nil {
		return err
	}
	return s.drivers.UpdateTaskBinding(ctx, driver.ID, driver.Status, driver.ActiveTaskID, 0, driver.UpdatedAt)
}

// releaseDriver 释放运力：行程送达即释放，且"本单后收车"命中时自动收车。
func (s *Service) releaseDriver(ctx context.Context, driver *entity.Driver, force bool) error {
	offline := driver.ReleaseTask()
	if force {
		driver.Status = entity.DriverStatusOffline
	}
	if err := s.drivers.UpdateTaskBinding(ctx, driver.ID, driver.Status, driver.ActiveTaskID, 0, driver.UpdatedAt); err != nil {
		return err
	}
	if offline {
		s.emit(ctx, &contracts.DriverStatusChangedPayload{
			EventID:  "dsc-" + driver.ID + "-" + fmt.Sprint(driver.UpdatedAt.UnixNano()),
			DriverID: driver.ID, FromStatus: entity.DriverStatusBusy, ToStatus: entity.DriverStatusOffline,
			Reason: "offline_after_trip", OccurredAt: biztime.RFC3339(driver.UpdatedAt),
		}, contracts.EventDriverStatusChanged)
	}
	return nil
}

// emit 把领域事件写入可靠事件表（与业务同库），由投递器发布到消息队列。
// 事件 ID 由业务侧显式给出（通常是「业务键 + 轮次」），保证重复投递可被消费端去重。
func (s *Service) emit(ctx context.Context, payload any, eventType string) {
	raw, err := json.Marshal(payload)
	if err != nil {
		s.log.Error("marshal event payload failed", zap.String("type", eventType), zap.Error(err))
		return
	}
	eventID := eventIDFromPayload(payload)
	if eventID == "" {
		eventID = fmt.Sprintf("%s-%d", eventType, biztime.UTC().UnixNano())
	}
	if err := s.support.EnqueueEvent(ctx, entity.NewOutboxEvent(eventID, eventType, string(raw))); err != nil {
		s.log.Warn("enqueue event failed", zap.String("type", eventType), zap.Error(err))
	}
}

// eventIDFromPayload 从载荷的 EventID 字段提取事件 ID（反射，避免逐个载荷写分支）。
func eventIDFromPayload(payload any) string {
	v := reflect.ValueOf(payload)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("EventID")
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String()
	}
	return ""
}

func stringsDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// ───────────────────────── 派单评分 ─────────────────────────

// scoreCandidate 按 A06 权重归一化到 0–10000 的整数：
// 距离越近、好评率越高、在线越久、成功率越高得分越高；并列时用司机编号打破，保证可复现。
func (s *Service) scoreCandidate(d *entity.Driver, order *entity.HallOrder) int64 {
	w := s.cfg.Weights
	var score float64
	if w.Distance > 0 {
		// 以司机最近心跳位置到起点的距离计分，5 公里内线性衰减，超出计 0。
		var dist float64
		if d.LastLat != 0 || d.LastLng != 0 {
			dist = float64(geo.DistanceMeters(geo.Point{Lat: d.LastLat, Lng: d.LastLng}, order.Origin.Point()))
		} else {
			dist = 5000
		}
		ratio := 1 - dist/5000
		if ratio < 0 {
			ratio = 0
		}
		score += ratio * float64(w.Distance)
	}
	if w.Rating > 0 {
		score += (d.Rating / 5.0) * float64(w.Rating)
	}
	if w.Online > 0 {
		onlineSec := biztime.UTC().Sub(d.OnlineSinceAt).Seconds()
		ratio := onlineSec / 3600
		if ratio > 1 {
			ratio = 1
		}
		score += ratio * float64(w.Online)
	}
	if w.Success > 0 {
		score += d.SuccessRate * float64(w.Success)
	}
	return int64(score)
}

// rankCandidates 排序：本月累计已结算收入升序（挣得少的优先） → 综合评分降序 → 司机编号升序。
func (s *Service) rankCandidates(list []*entity.Driver, order *entity.HallOrder) []*entity.Driver {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.MonthlyIncomeFen != b.MonthlyIncomeFen {
			return a.MonthlyIncomeFen < b.MonthlyIncomeFen
		}
		sa, sb := s.scoreCandidate(a, order), s.scoreCandidate(b, order)
		if sa != sb {
			return sa > sb
		}
		return a.DriverNo < b.DriverNo
	})
	return list
}

// filterCandidate 车型/座位/城市/区域的精确校验。
func (s *Service) filterCandidate(d *entity.Driver, order *entity.HallOrder) bool {
	if !d.MatchRequirement(order.RequireSeats, order.RequireCarType, order.CityCode) {
		return false
	}
	if !d.InArea(order.Origin.Lat, order.Origin.Lng) {
		return false
	}
	return true
}
