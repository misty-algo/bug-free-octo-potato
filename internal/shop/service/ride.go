package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	pricingservice "github.com/BwCloudWeGo/bw-cli/internal/pricing/service"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	"github.com/BwCloudWeGo/bw-cli/pkg/gopay"
)

// ErrTripRepositoryNotReady 行程仓储未注入。
var ErrTripRepositoryNotReady = errors.New("trip repository not configured")

// requireTrips 校验行程仓储已注入。
func (s *Service) requireTrips() error {
	if s.trips == nil {
		return ErrTripRepositoryNotReady
	}
	return nil
}

// CreateTrip 乘客下单：落一条 WAITING 行程，等司机接单。
func (s *Service) CreateTrip(ctx context.Context, cmd dto.CreateTripCommand) (*dto.TripDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	now := time.Now()
	trip, err := entity.NewTrip(cmd.TripID, cmd.PassengerID, cmd.Origin, cmd.Destination, now)
	if err != nil {
		return nil, err
	}
	if err := s.trips.Save(ctx, trip); err != nil {
		return nil, err
	}
	// 通知司机端生成派单任务。事件落库失败不阻断下单——行程已建成，
	// 派单通知由后台投递器重试补上；反过来若先发事件后落库，则会派出一单不存在的行程。
	if err := s.enqueueOrderCreated(ctx, trip); err != nil {
		s.log.Warn("enqueue order created event failed", zap.String("trip_id", trip.ID), zap.Error(err))
	}
	s.log.Info("trip created", zap.String("trip_id", trip.ID), zap.String("passenger_id", trip.PassengerID))
	return dto.FromTrip(trip), nil
}

// maxEventRetries 单条事件的最大投递尝试次数，超过后移出待投队列，避免坏事件饿死后续事件。
const maxEventRetries = 10

// enqueueOrderCreated 把下单事件写入出站表，由后台投递器通知司机端派单。
//
// 走 outbox 而非直接发 MQ：行程已落库才派单，且 broker 抖动时事件不会丢——
// 否则乘客下单成功却永远等不到司机，且没有任何告警信号。
//
// 金额刻意留空：下单时尚未计价（计价发生在行程结束，见 EndTrip），
// 真实金额由 trip.ended 事件携带，司机端对缺省金额自有演示默认值。
func (s *Service) enqueueOrderCreated(ctx context.Context, trip *entity.Trip) error {
	if s.outbox == nil {
		return errors.New("outbox repository not configured")
	}
	payload := contracts.OrderCreatedPayload{
		EventID:        "order-created-" + trip.ID,
		OrderID:        trip.ID,
		TaskID:         trip.ID,
		OrderType:      "real_time",
		PassengerName:  "乘客",
		PassengerCount: 1,
		RequireSeats:   1,
		Pickup: contracts.LatLng{
			Name: "上车点",
			Lng:  trip.Origin.Lng(),
			Lat:  trip.Origin.Lat(),
		},
		Dropoff: contracts.LatLng{
			Name: "目的地",
			Lng:  trip.Destination.Lng(),
			Lat:  trip.Destination.Lat(),
		},
		CreatedAt: trip.CreatedAt.Format(time.RFC3339),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal order created payload: %w", err)
	}
	return s.outbox.EnqueueEvent(ctx, entity.NewOutboxEvent(
		payload.EventID, contracts.EventOrderCreated, string(body)))
}

// PublishPendingEvents 可靠事件投递：把待投递事件发布到消息队列后标记为已发布。
// broker 不可用时事件保留在待投队列等待重试，保证派单通知不丢。
func (s *Service) PublishPendingEvents(ctx context.Context, limit int) (int, error) {
	if s.outbox == nil {
		return 0, errors.New("outbox repository not configured")
	}
	if s.publisher == nil {
		return 0, nil // 未接 MQ：事件留在表里，接线后自动补投
	}
	pending, err := s.outbox.FetchPendingEvents(ctx, limit)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, e := range pending {
		if err := s.publisher(ctx, e.EventType, []byte(e.Payload)); err != nil {
			_ = s.outbox.MarkFailed(ctx, e.EventID, err.Error(), maxEventRetries)
			continue
		}
		if err := s.outbox.MarkPublished(ctx, e.EventID); err != nil {
			continue
		}
		published++
	}
	return published, nil
}

// GetTrip 按行程号查询。
func (s *Service) GetTrip(ctx context.Context, tripID string) (*dto.TripDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	trip, err := s.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	return dto.FromTrip(trip), nil
}

// ListTrips 查询乘客的行程列表。
func (s *Service) ListTrips(ctx context.Context, cmd dto.ListTripsCommand) (*dto.ListTripsDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	items, err := s.trips.ListByPassenger(ctx, cmd.PassengerID, cmd.Limit)
	if err != nil {
		return nil, err
	}
	out := &dto.ListTripsDTO{Items: make([]*dto.TripDTO, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, dto.FromTrip(item))
	}
	return out, nil
}

// AssignDriver 司机接单：WAITING → ACCEPTED。
func (s *Service) AssignDriver(ctx context.Context, cmd dto.AssignDriverCommand) (*dto.TripDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	trip, err := s.trips.FindByID(ctx, cmd.TripID)
	if err != nil {
		return nil, err
	}
	if err := trip.AssignDriver(cmd.DriverID, time.Now()); err != nil {
		return nil, err
	}
	if err := s.trips.Save(ctx, trip); err != nil {
		return nil, err
	}
	s.log.Info("trip assigned", zap.String("trip_id", trip.ID), zap.String("driver_id", trip.DriverID))
	return dto.FromTrip(trip), nil
}

// DriverArrived 司机到达上车点：ACCEPTED → ARRIVED。
func (s *Service) DriverArrived(ctx context.Context, cmd dto.TripActionCommand) (*dto.TripDTO, error) {
	return s.applyTripAction(ctx, cmd.TripID, "arrived", func(t *entity.Trip, at time.Time) error {
		return t.Arrive(at)
	})
}

// BoardTrip 乘客上车：ARRIVED → IN_TRIP。
func (s *Service) BoardTrip(ctx context.Context, cmd dto.TripActionCommand) (*dto.TripDTO, error) {
	return s.applyTripAction(ctx, cmd.TripID, "boarded", func(t *entity.Trip, at time.Time) error {
		return t.Board(at)
	})
}

// CancelTrip 取消行程：未上车前的任一状态 → CANCELLED。
func (s *Service) CancelTrip(ctx context.Context, cmd dto.TripActionCommand) (*dto.TripDTO, error) {
	return s.applyTripAction(ctx, cmd.TripID, "cancelled", func(t *entity.Trip, at time.Time) error {
		return t.Cancel(at)
	})
}

// applyTripAction 加载行程 → 执行状态流转 → 落库，统一错误与日志处理。
func (s *Service) applyTripAction(ctx context.Context, tripID, action string,
	apply func(*entity.Trip, time.Time) error) (*dto.TripDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	trip, err := s.trips.FindByID(ctx, tripID)
	if err != nil {
		return nil, err
	}
	if err := apply(trip, time.Now()); err != nil {
		return nil, err
	}
	if err := s.trips.Save(ctx, trip); err != nil {
		return nil, err
	}
	s.log.Info("trip status changed", zap.String("trip_id", trip.ID), zap.String("action", action),
		zap.String("status", trip.Status))
	return dto.FromTrip(trip), nil
}

// EndTrip 司机送达：IN_TRIP → ENDED，随后计价并按锁定费率建账。
//
// 这条方法是「乘客付钱 → 司机挣钱」链路的起点：行程结束才产生应付金额，
// 计价给出实付与佣金率，建账把佣金率锁进账单，后续支付成功才触发分账。
//
// 幂等：重复调用不会重复计价、重复建账。已结束时只补做尚未完成的建账，
// 用于修复「行程已结束但建账失败」的中间态。
func (s *Service) EndTrip(ctx context.Context, cmd dto.EndTripCommand) (*dto.TripDTO, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	if s.pricing == nil {
		return nil, errors.New("pricing service not configured")
	}
	trip, err := s.trips.FindByID(ctx, cmd.TripID)
	if err != nil {
		return nil, err
	}
	now := time.Now()

	// 已结束：跳过状态流转与计价，只补做建账。
	if trip.Status != entity.TripStatusEnded {
		if err := s.settleFare(ctx, trip, cmd, now); err != nil {
			return nil, err
		}
	}
	if err := s.ensureBillCreated(ctx, trip); err != nil {
		return nil, err
	}
	s.log.Info("trip ended", zap.String("trip_id", trip.ID), zap.Int64("passenger_paid", trip.PassengerPaid),
		zap.Int64("commission_rate_bp", trip.CommissionRateBP), zap.Bool("bill_created", trip.BillCreated))
	return dto.FromTrip(trip), nil
}

// settleFare 结束行程、调用计价域算钱，并把结果与锁定的佣金率回填行程。
// 分三步落库：结束时先落状态（事实不可逆），计价后落金额，最后落建账标记——
// 任一步失败都可重入，不会算出第二份价。
func (s *Service) settleFare(ctx context.Context, trip *entity.Trip, cmd dto.EndTripCommand, now time.Time) error {
	if err := trip.End(now, cmd.DistanceM, cmd.DurationSec, cmd.WaitSec); err != nil {
		return err
	}
	if err := s.trips.Save(ctx, trip); err != nil {
		return err
	}

	// 计价请求号用行程号派生：同一行程重复请求命中同一计价结果。
	billingRequestID := "BR-" + trip.ID
	fare, err := s.pricing.CalculateBill(ctx, pricingservice.CalculateBillCommand{
		OrderNo:          trip.ID,
		BillingRequestID: billingRequestID,
		DistanceM:        trip.DistanceM,
		DurationSec:      trip.DurationSec,
		WaitSec:          trip.WaitSec,
		StartAt:          derefTime(trip.StartedAt, trip.CreatedAt),
	})
	if err != nil {
		return fmt.Errorf("calculate bill failed: %w", err)
	}
	// 佣金率来自计价规则快照，不是调用方传参——规则变更不会让分账漂移。
	if err := trip.ApplyFare(billingRequestID, fare.PassengerPaid, fare.CommissionRateBP, time.Now()); err != nil {
		return err
	}
	if err := s.trips.Save(ctx, trip); err != nil {
		return err
	}
	s.log.Info("trip fare applied", zap.String("trip_id", trip.ID),
		zap.Int64("passenger_paid", fare.PassengerPaid), zap.Int64("commission_rate_bp", fare.CommissionRateBP))
	return nil
}

// PayTrip 乘客为已结束的行程付款：金额取计价结果，支付单号复用行程号。
//
// 这是「行程 → 支付宝 → 回调 → 分账」的接入口。支付单号刻意与行程号同值，
// 因为行程号同时是账单的 OrderNo——支付回调只需拿 out_trade_no 就能
// 一路走到 MarkBillPaid 与 SplitBill，不需要任何映射表。
//
// 金额不可由调用方指定：必须是计价算出的实付，否则乘客可以自己定价。
func (s *Service) PayTrip(ctx context.Context, cmd dto.PayTripCommand) (*dto.PayTripResult, error) {
	if err := s.requireTrips(); err != nil {
		return nil, err
	}
	if err := s.requireAlipay(); err != nil {
		return nil, err
	}
	if err := s.requirePayments(); err != nil {
		return nil, err
	}
	if cmd.Scene != "wap" && cmd.Scene != "page" {
		return nil, fmt.Errorf("%w: scene must be 'wap' or 'page'", entity.ErrInvalidPaymentRequest)
	}
	trip, err := s.trips.FindByID(ctx, cmd.TripID)
	if err != nil {
		return nil, err
	}
	// 只给已结束的行程收款：未结束就收等于预扣，取消/改价都要退。
	if trip.Status != entity.TripStatusEnded {
		return nil, fmt.Errorf("%w: trip %s is %s, only ENDED trip can be paid",
			entity.ErrInvalidTripStatus, trip.ID, trip.Status)
	}
	if trip.PassengerPaid <= 0 {
		return nil, fmt.Errorf("%w: trip %s has no payable amount", entity.ErrInvalidTrip, trip.ID)
	}
	// 账单必须先存在：否则支付成功后的分账找不到锚点，钱会滞留。
	if !trip.BillCreated {
		return nil, fmt.Errorf("%w: trip %s bill not created yet", entity.ErrInvalidTripStatus, trip.ID)
	}

	// 支付单号 = 行程号 = 账单 OrderNo，三者同值让回调链路无需映射。
	item, duplicate, err := s.createOrGetPayment(ctx, trip.ID, trip.PassengerPaid, entity.PaymentChannelAlipay)
	if err != nil {
		return nil, err
	}
	if duplicate {
		s.log.Info("duplicate trip pay request", zap.String("trip_id", trip.ID))
		if item.Status != entity.PaymentStatusPending {
			return nil, fmt.Errorf("%w: trip %s already paid with status %s",
				entity.ErrPaymentExists, trip.ID, item.Status)
		}
	}
	resp, err := s.alipay.Pay(gopay.PayRequest{
		OutTradeNo:  trip.ID,
		Subject:     "网约车行程费用",
		TotalAmount: formatCents(trip.PassengerPaid),
		Body:        "行程号 " + trip.ID,
	}, cmd.Scene)
	if err != nil {
		s.markFailed(ctx, trip.ID, "alipay trip pay")
		return nil, fmt.Errorf("alipay pay: %w", err)
	}
	s.log.Info("trip pay requested", zap.String("trip_id", trip.ID),
		zap.Int64("amount_cents", trip.PassengerPaid), zap.String("scene", cmd.Scene))
	return &dto.PayTripResult{
		TripID:  trip.ID,
		OrderNo: trip.ID,
		Amount:  trip.PassengerPaid,
		PayURL:  resp.PayURL,
	}, nil
}

// ReconcilePendingBills 建账补偿：扫描已结束但尚未建账的行程并补建账，返回本批成功单数。
//
// 补的是 EndTrip 里「行程已计价但建账失败」的中间态——行程结束是已发生的事实、
// 金额已按规则快照锁定，只差账单没建出来。没有它，乘客付不了款（PayTrip 要求账单存在），
// 整条资金链就卡在行程这一环。
//
// 每单复用 ensureBillCreated，与在线链路同一套锁定费率，不存在两套口径。
// 单笔失败不中断整批：跳过并记日志，下一轮继续扫；建账幂等，重复补偿不会重复建账。
func (s *Service) ReconcilePendingBills(ctx context.Context, limit int) (int, error) {
	if err := s.requireTrips(); err != nil {
		return 0, err
	}
	// 结算客户端缺失必须显式报错。ensureBillCreated 在客户端未注入时会降级返回 nil，
	// 若这里不拦，补偿会「扫到 → 什么都没做 → 记为成功」并逐轮空转。
	if s.settlement == nil {
		return 0, errors.New("settlement client not configured")
	}
	if limit <= 0 {
		return 0, errors.New("positive limit is required")
	}
	trips, err := s.trips.ListPendingBillTrips(ctx, limit)
	if err != nil {
		return 0, err
	}
	compensated := 0
	for _, trip := range trips {
		if err := s.ensureBillCreated(ctx, trip); err != nil {
			s.log.Warn("reconcile create bill failed", zap.String("trip_id", trip.ID), zap.Error(err))
			continue
		}
		compensated++
		s.log.Info("reconcile bill created", zap.String("trip_id", trip.ID))
	}
	return compensated, nil
}

// ensureBillCreated 保证行程对应账单已创建。已建过直接返回，未建则按锁定费率建账。
// 建账与行程结束不在同一事务，靠 BillCreated 标记 + 结算域自身的幂等双重保证。
func (s *Service) ensureBillCreated(ctx context.Context, trip *entity.Trip) error {
	if trip.BillCreated {
		return nil
	}
	if s.settlement == nil {
		// 结算客户端未注入：行程与计价已落库，账单留给补偿或人工处理，
		// 但不能让行程结束失败——行程结束是已发生的事实。
		s.log.Warn("settlement client not injected, skip bill creation", zap.String("trip_id", trip.ID))
		return nil
	}
	_, err := s.settlement.CreateBill(ctx, &settlementv1.CreateBillRequest{
		OrderNo:          trip.ID,
		UserId:           trip.PassengerID,
		DriverId:         trip.DriverID,
		TotalAmount:      trip.PassengerPaid,
		CommissionRateBp: trip.CommissionRateBP,
	})
	if err != nil {
		return fmt.Errorf("create bill failed: %w", err)
	}
	if trip.MarkBillCreated(time.Now()) {
		if err := s.trips.Save(ctx, trip); err != nil {
			return err
		}
	}
	return nil
}

// derefTime 取指针时间值，nil 时返回兜底值。
func derefTime(t *time.Time, fallback time.Time) time.Time {
	if t == nil {
		return fallback
	}
	return *t
}
