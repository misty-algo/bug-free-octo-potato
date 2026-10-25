package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/payment/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/payment/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/redisx"
	"github.com/BwCloudWeGo/bw-cli/pkg/rocketmqx"
)

// 支付/退款事件 RocketMQ topic。
const (
	PaySuccessTopic    = "pay_success_events"
	RefundSuccessTopic = "refund_success_events"
)

// EventPublisher 是支付成功消息发布端口，*rocketmqx.SimpleProducer 天然满足。
type EventPublisher interface {
	Publish(ctx context.Context, message rocketmqx.Message) (rocketmqx.SendResult, error)
}

// RefundGateway 是退款渠道端口（P16）。实际部署可接入支付宝/微信退款，测试用 stub 直通成功。
type RefundGateway interface {
	// Refund 向渠道发起退款，返回渠道退款交易号；失败返回 error。
	Refund(ctx context.Context, req RefundGatewayRequest) (RefundGatewayResult, error)
}

// RefundGatewayRequest 退款渠道请求。
type RefundGatewayRequest struct {
	PaymentNo    string // 原支付单号
	OrderNo      string
	TradeNo      string // 原渠道交易号
	RefundNo     string // 退款单号
	OutRequestNo string // 渠道退款请求号（幂等）
	Amount       int64  // 退款金额（分）
	Reason       string
}

// RefundGatewayResult 退款渠道响应。
type RefundGatewayResult struct {
	ChannelTradeNo string // 渠道退款交易号
}

// Service 编排 payment 用例。
type Service struct {
	repo          entity.Repository
	log           *zap.Logger
	publisher     EventPublisher // 为 nil 时回退到 payment_events 待处理表
	locker        *redisx.Locker // 为 nil 时跳过分布式锁（幂等仍由唯一键/状态保证）
	refundGateway RefundGateway  // 为 nil 时退款直通成功（本地 stub）
}

// NewService 创建 payment 用例服务。
func NewService(repo entity.Repository, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, log: log}
}

// SetEventPublisher 注入支付成功消息发布器（RocketMQ）。
func (s *Service) SetEventPublisher(publisher EventPublisher) {
	s.publisher = publisher
}

// SetRefundGateway 注入退款渠道端口（支付宝/微信）。
func (s *Service) SetRefundGateway(gateway RefundGateway) {
	s.refundGateway = gateway
}

// SetLocker 注入 Redis 分布式锁（用于 NotifyPayment 并发防重）。
func (s *Service) SetLocker(locker *redisx.Locker) {
	s.locker = locker
}

// CreatePayment 创建支付单记录，order_no 为幂等唯一键。
func (s *Service) CreatePayment(ctx context.Context, cmd dto.CreatePaymentCommand) (*dto.PaymentDTO, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	if cmd.Amount <= 0 {
		return nil, fmt.Errorf("%w: positive amount is required", entity.ErrInvalidPaymentRequest)
	}
	idempotentKey := firstNonEmpty(cmd.IdempotentKey, cmd.OrderNo)

	if existing, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo); err == nil {
		if existing.IdempotentKey == idempotentKey {
			return toDTO(existing), nil
		}
		return nil, entity.ErrPaymentAlreadyExists
	} else if !errors.Is(err, entity.ErrPaymentNotFound) {
		return nil, err
	}

	now := time.Now()
	payment := &entity.Payment{
		ID:            generatePaymentNo(cmd.OrderNo),
		OrderNo:       cmd.OrderNo,
		UserID:        cmd.UserID,
		Subject:       cmd.Subject,
		Amount:        cmd.Amount,
		Status:        entity.PaymentStatusCreated,
		Channel:       cmd.Channel,
		IdempotentKey: idempotentKey,
		PrepayID:      cmd.PrepayID,
		ExpireAt:      cmd.ExpireAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, err
	}
	s.log.Info("payment created", zap.String("payment_no", payment.ID), zap.String("order_no", payment.OrderNo), zap.Int64("amount", payment.Amount))
	return toDTO(payment), nil
}

// NotifyPayment 处理支付成功异步通知，幂等可重试：
// 1. 已成功 → 直接返回 duplicate=true，不重复入账；
// 2. CREATED/PROCESSING/FAILED → SUCCESS（FAILED 后渠道最终确认成功也允许恢复）；
// 3. CLOSED 为终态，拒绝支付成功。
// 首次成功 → 更新支付单、发送 RocketMQ 消息；MQ 不可用/发送失败时落 payment_events(pending)。
func (s *Service) NotifyPayment(ctx context.Context, cmd dto.NotifyPaymentCommand) (*dto.NotifyPaymentResult, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	payment, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if payment.Status == entity.PaymentStatusSuccess {
		return &dto.NotifyPaymentResult{Success: true, PaymentNo: payment.ID, Status: payment.Status, Duplicate: true}, nil
	}
	if payment.Status == entity.PaymentStatusClosed {
		return nil, fmt.Errorf("%w: closed payment cannot be paid, order_no=%s", entity.ErrPaymentClosed, cmd.OrderNo)
	}

	lockKey := "payment:notify:" + cmd.OrderNo
	lock, acquired, err := s.tryLock(ctx, lockKey, 5*time.Second)
	if err != nil {
		return nil, err
	}
	// 未抢到锁说明同一订单的通知正在并发处理，直接拒绝本次，由渠道重试通知。
	if !acquired {
		return nil, entity.ErrNotifyConcurrent
	}
	defer func() { _ = lock.Release(ctx) }()
	// 二次检查，避免并发窗口内重复入账。
	if payment.Status == entity.PaymentStatusSuccess {
		return &dto.NotifyPaymentResult{Success: true, PaymentNo: payment.ID, Status: payment.Status, Duplicate: true}, nil
	}

	now := time.Now()
	paidAt := time.Unix(cmd.PaidAt, 0)
	if cmd.PaidAt <= 0 {
		paidAt = now
	}
	payment.Status = entity.PaymentStatusSuccess
	payment.PaidAmount = cmd.PaidAmount
	payment.TradeNo = cmd.TradeNo
	if cmd.Channel != "" {
		payment.Channel = cmd.Channel
	}
	payment.PaidAt = &paidAt
	payment.FailReason = ""
	payment.NotifyCount++
	payment.UpdatedAt = now
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, err
	}

	// 发送支付成功事件（幂等：同一订单同一类型只落一条 pending 事件）。
	payload, _ := json.Marshal(map[string]any{
		"order_no":   payment.OrderNo,
		"payment_no": payment.ID,
		"trade_no":   payment.TradeNo,
		"amount":     payment.PaidAmount,
		"paid_at":    paidAt.Unix(),
	})
	if err := s.publishPaySuccess(ctx, payment.OrderNo, string(payload)); err != nil {
		// MQ 发送失败不阻塞通知成功，事件落 pending 由补偿任务或 gateway 触发分账。
		s.log.Warn("publish pay success event failed, fallback to pending event", zap.String("order_no", payment.OrderNo), zap.Error(err))
	}
	s.log.Info("payment notify processed", zap.String("payment_no", payment.ID), zap.String("order_no", payment.OrderNo))
	return &dto.NotifyPaymentResult{Success: true, PaymentNo: payment.ID, Status: payment.Status, Duplicate: false}, nil
}

// BeginPayment 渠道预支付受理回调：CREATED → PROCESSING（幂等）。
func (s *Service) BeginPayment(ctx context.Context, cmd dto.BeginPaymentCommand) (*dto.PaymentDTO, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	payment, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if payment.Status == entity.PaymentStatusProcessing || payment.Status == entity.PaymentStatusSuccess {
		return toDTO(payment), nil // 幂等：已受理/已成功直接返回
	}
	if payment.Status != entity.PaymentStatusCreated {
		return nil, fmt.Errorf("%w: only CREATED payment can begin, current=%s", entity.ErrInvalidPaymentRequest, payment.Status)
	}
	payment.Status = entity.PaymentStatusProcessing
	if cmd.PrepayID != "" {
		payment.PrepayID = cmd.PrepayID
	}
	payment.UpdatedAt = time.Now()
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, err
	}
	s.log.Info("payment processing", zap.String("payment_no", payment.ID), zap.String("order_no", payment.OrderNo))
	return toDTO(payment), nil
}

// FailPayment 渠道支付失败通知：CREATED/PROCESSING → FAILED（记录失败原因）。
func (s *Service) FailPayment(ctx context.Context, cmd dto.FailPaymentCommand) (*dto.PaymentDTO, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	payment, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if payment.Status == entity.PaymentStatusFailed {
		return toDTO(payment), nil // 幂等
	}
	if payment.Status == entity.PaymentStatusSuccess || payment.Status == entity.PaymentStatusClosed {
		return nil, fmt.Errorf("%w: only CREATED/PROCESSING payment can fail, current=%s", entity.ErrInvalidPaymentRequest, payment.Status)
	}
	payment.Status = entity.PaymentStatusFailed
	payment.FailReason = cmd.FailReason
	payment.UpdatedAt = time.Now()
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, err
	}
	s.log.Info("payment failed", zap.String("payment_no", payment.ID), zap.String("order_no", payment.OrderNo), zap.String("reason", cmd.FailReason))
	return toDTO(payment), nil
}

// ClosePayment 关单：CREATED/PROCESSING → CLOSED（15 分钟未支付自动关单 / 主动关闭，幂等）。
func (s *Service) ClosePayment(ctx context.Context, cmd dto.ClosePaymentCommand) (*dto.PaymentDTO, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	payment, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if payment.Status == entity.PaymentStatusClosed {
		return toDTO(payment), nil // 幂等
	}
	if payment.Status == entity.PaymentStatusSuccess {
		return nil, fmt.Errorf("%w: paid payment cannot be closed, current=%s", entity.ErrInvalidPaymentRequest, payment.Status)
	}
	payment.Status = entity.PaymentStatusClosed
	if cmd.Reason != "" {
		payment.FailReason = cmd.Reason
	}
	payment.UpdatedAt = time.Now()
	if err := s.repo.SavePayment(ctx, payment); err != nil {
		return nil, err
	}
	s.log.Info("payment closed", zap.String("payment_no", payment.ID), zap.String("order_no", payment.OrderNo), zap.String("reason", cmd.Reason))
	return toDTO(payment), nil
}

// QueryPayment 按订单号查询支付单。
func (s *Service) QueryPayment(ctx context.Context, orderNo string) (*dto.PaymentDTO, error) {
	if orderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	payment, err := s.repo.FindPaymentByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	return toDTO(payment), nil
}

// RefundPayment 对已支付订单发起退款（P16 退款状态机）。
//
// 状态机：支付单保持 SUCCESS（退款状态由 refunds.status 承载）：
//
//	RefundPayment → 创建退款单 REVIEW（待审核，不立即提交渠道）
//	ReviewRefund(approved=true)  → REVIEW → PROCESSING → 渠道 → SUCCESS / FAILED
//	ReviewRefund(approved=false) → REVIEW → REJECTED（终态）
//
// 幂等：out_request_no 是退款幂等锚点，重复调用命中已有退款单直接返回，不重复创建。
// 防超退：已创建未终止（非 REJECTED/FAILED）的退款金额合计 + 本次 ≤ 实付金额。
// 校验：仅 SUCCESS 支付单可退款。
func (s *Service) RefundPayment(ctx context.Context, cmd dto.RefundPaymentCommand) (*dto.RefundResult, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidPaymentRequest)
	}
	if cmd.RefundAmount <= 0 {
		return nil, fmt.Errorf("%w: positive refund_amount is required", entity.ErrInvalidPaymentRequest)
	}
	outRequestNo := firstNonEmpty(cmd.OutRequestNo, "REFUND-"+cmd.OrderNo+"-"+fmt.Sprint(time.Now().UnixNano()))

	// 幂等：同一渠道退款请求号命中已有退款单，直接返回历史结果。
	if existing, err := s.repo.FindRefundByOutRequestNo(ctx, outRequestNo); err == nil {
		return &dto.RefundResult{
			Success:      existing.Status == entity.RefundStatusSuccess,
			RefundNo:     existing.ID,
			PaymentNo:    existing.PaymentNo,
			OrderNo:      existing.OrderNo,
			RefundAmount: existing.Amount,
			Status:       existing.Status,
			Duplicate:    true,
		}, nil
	} else if !errors.Is(err, entity.ErrRefundNotFound) {
		return nil, err
	}

	payment, err := s.repo.FindPaymentByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if payment.Status != entity.PaymentStatusSuccess {
		return nil, fmt.Errorf("%w: only success payment can be refunded, current=%s", entity.ErrInvalidPaymentRequest, payment.Status)
	}

	// 防超退：在途（REVIEW/PROCESSING/SUCCESS）退款合计 + 本次 ≤ 实付。
	inFlight, err := s.pendingRefundTotal(ctx, payment.OrderNo)
	if err != nil {
		return nil, err
	}
	if inFlight+cmd.RefundAmount > payment.PaidAmount {
		return nil, entity.ErrRefundExceedsPaid
	}

	now := time.Now()
	refund := &entity.Refund{
		ID:           "REF-" + outRequestNo, // 单号绑定幂等锚点，天然唯一（out_request_no 有唯一键）
		PaymentNo:    payment.ID,
		OrderNo:      payment.OrderNo,
		UserID:       payment.UserID,
		Amount:       cmd.RefundAmount,
		Status:       entity.RefundStatusReview,
		OutRequestNo: outRequestNo,
		Reason:       cmd.Reason,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.SaveRefund(ctx, refund); err != nil {
		return nil, err
	}
	s.log.Info("refund requested, awaiting review", zap.String("refund_no", refund.ID), zap.String("order_no", payment.OrderNo), zap.Int64("amount", cmd.RefundAmount))
	return &dto.RefundResult{
		Success:      false, // 尚未退款成功，等待审核
		RefundNo:     refund.ID,
		PaymentNo:    payment.ID,
		OrderNo:      payment.OrderNo,
		RefundAmount: cmd.RefundAmount,
		Status:       refund.Status, // REVIEW
		Duplicate:    false,
	}, nil
}

// ReviewRefund 审核退款单：REVIEW → REJECTED（拒绝）或 REVIEW → PROCESSING → 渠道 → SUCCESS/FAILED。
// FAILED 状态可再次审核重试渠道提交。
func (s *Service) ReviewRefund(ctx context.Context, cmd dto.ReviewRefundCommand) (*dto.RefundResult, error) {
	if cmd.RefundNo == "" {
		return nil, fmt.Errorf("%w: refund_no is required", entity.ErrInvalidPaymentRequest)
	}
	refund, err := s.repo.FindRefundByRefundNo(ctx, cmd.RefundNo)
	if err != nil {
		return nil, err
	}

	// 拒绝：仅 REVIEW 可拒绝；已成功/处理中/已拒绝均拒绝状态流转。
	if !cmd.Approved {
		if refund.Status != entity.RefundStatusReview {
			return nil, fmt.Errorf("%w: only REVIEW refund can be rejected, current=%s", entity.ErrRefundStateTransition, refund.Status)
		}
		refund.Status = entity.RefundStatusRejected
		refund.RejectReason = cmd.RejectReason
		refund.UpdatedAt = time.Now()
		if err := s.repo.SaveRefund(ctx, refund); err != nil {
			return nil, err
		}
		s.log.Info("refund rejected", zap.String("refund_no", refund.ID), zap.String("reason", cmd.RejectReason))
		return &dto.RefundResult{
			Success:      false,
			RefundNo:     refund.ID,
			PaymentNo:    refund.PaymentNo,
			OrderNo:      refund.OrderNo,
			RefundAmount: refund.Amount,
			Status:       refund.Status,
			Duplicate:    false,
		}, nil
	}

	// 通过：REVIEW/FAILED 可提交渠道重试；SUCCESS/REJECTED/PROCESSING 拒绝。
	if refund.Status != entity.RefundStatusReview && refund.Status != entity.RefundStatusFailed {
		return nil, fmt.Errorf("%w: only REVIEW/FAILED refund can be submitted, current=%s", entity.ErrRefundStateTransition, refund.Status)
	}
	payment, err := s.repo.FindPaymentByPaymentNo(ctx, refund.PaymentNo)
	if err != nil {
		return nil, err
	}

	refund.Status = entity.RefundStatusProcessing
	refund.UpdatedAt = time.Now()
	if err := s.repo.SaveRefund(ctx, refund); err != nil {
		return nil, err
	}

	// 调渠道退款（gateway 为 nil 时直通成功，便于本地联调/测试）。
	channelTradeNo := ""
	if s.refundGateway != nil {
		result, err := s.refundGateway.Refund(ctx, RefundGatewayRequest{
			PaymentNo:    payment.ID,
			OrderNo:      payment.OrderNo,
			TradeNo:      payment.TradeNo,
			RefundNo:     refund.ID,
			OutRequestNo: refund.OutRequestNo,
			Amount:       refund.Amount,
			Reason:       refund.Reason,
		})
		if err != nil {
			// 渠道失败：退款单 FAILED，支付单保持 SUCCESS，可重试。
			refund.Status = entity.RefundStatusFailed
			refund.UpdatedAt = time.Now()
			_ = s.repo.SaveRefund(ctx, refund)
			s.log.Warn("refund channel failed", zap.String("refund_no", refund.ID), zap.String("order_no", payment.OrderNo), zap.Error(err))
			return &dto.RefundResult{
				Success:      false,
				RefundNo:     refund.ID,
				PaymentNo:    payment.ID,
				OrderNo:      payment.OrderNo,
				RefundAmount: refund.Amount,
				Status:       refund.Status,
				Duplicate:    false,
			}, nil
		}
		channelTradeNo = result.ChannelTradeNo
	}

	// 渠道退款成功：退款单 SUCCESS，支付单累计已退金额（原子累加，防并发互相覆盖）。
	refundedAt := time.Now()
	refund.Status = entity.RefundStatusSuccess
	refund.ChannelTradeNo = channelTradeNo
	refund.RefundedAt = &refundedAt
	refund.UpdatedAt = refundedAt
	if err := s.repo.SaveRefund(ctx, refund); err != nil {
		return nil, err
	}

	if err := s.repo.AddRefundedAmount(ctx, payment.ID, refund.Amount); err != nil {
		// 原子累加失败（理论不应发生，发起退款时已防超退；并发极端下可能命中）：
		// 渠道退款已成功不可逆，记录错误交由人工对账介入。
		s.log.Error("add refunded amount failed after channel success",
			zap.String("payment_no", payment.ID), zap.String("refund_no", refund.ID), zap.Error(err))
		return nil, err
	}

	// 发送退款成功事件（幂等，兜底落 payment_events）。
	payload, _ := json.Marshal(map[string]any{
		"order_no":         payment.OrderNo,
		"payment_no":       payment.ID,
		"refund_no":        refund.ID,
		"refund_amount":    refund.Amount,
		"channel_trade_no": refund.ChannelTradeNo,
		"refunded_at":      refundedAt.Unix(),
	})
	if err := s.publishRefundSuccess(ctx, payment.OrderNo, string(payload)); err != nil {
		s.log.Warn("publish refund success event failed, fallback to pending event", zap.String("order_no", payment.OrderNo), zap.Error(err))
	}
	s.log.Info("refund succeeded", zap.String("payment_no", payment.ID), zap.String("refund_no", refund.ID),
		zap.String("order_no", payment.OrderNo), zap.Int64("refund_amount", refund.Amount))
	return &dto.RefundResult{
		Success:      true,
		RefundNo:     refund.ID,
		PaymentNo:    payment.ID,
		OrderNo:      payment.OrderNo,
		RefundAmount: refund.Amount,
		Status:       refund.Status, // SUCCESS
		Duplicate:    false,
	}, nil
}

// pendingRefundTotal 统计订单在途/已退的退款金额（REVIEW/PROCESSING/SUCCESS），用于防超退。
func (s *Service) pendingRefundTotal(ctx context.Context, orderNo string) (int64, error) {
	refunds, err := s.repo.ListRefundsByOrderNo(ctx, orderNo)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, r := range refunds {
		if r.Status == entity.RefundStatusReview || r.Status == entity.RefundStatusProcessing || r.Status == entity.RefundStatusSuccess {
			total += r.Amount
		}
	}
	return total, nil
}

// publishPaySuccess 优先走 RocketMQ，MQ 不可用时落 payment_events 待处理表（幂等）。
func (s *Service) publishPaySuccess(ctx context.Context, orderNo string, payload string) error {
	// 事件幂等：已存在同订单同类型事件则跳过。
	if _, err := s.repo.FindEventByOrderNoAndType(ctx, orderNo, entity.EventTypePaySuccess); err == nil {
		return nil
	} else if !errors.Is(err, entity.ErrEventNotFound) {
		return err
	}

	if s.publisher != nil {
		_, err := s.publisher.Publish(ctx, rocketmqx.Message{
			Topic: PaySuccessTopic,
			Tag:   "pay_success",
			Key:   orderNo,
			Body:  []byte(payload),
		})
		if err == nil {
			return nil
		}
		// MQ 发送失败不丢弃事件：落 pending 由补偿任务重发。
		s.log.Warn("rocketmq publish pay_success failed, fallback to pending event",
			zap.String("order_no", orderNo), zap.Error(err))
	}

	now := time.Now()
	event := &entity.PaymentEvent{
		ID:         "EVT-" + orderNo + "-" + entity.EventTypePaySuccess,
		OrderNo:    orderNo,
		EventType:  entity.EventTypePaySuccess,
		Payload:    payload,
		Status:     entity.EventStatusPending,
		RetryCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return s.repo.SaveEvent(ctx, event)
}

// publishRefundSuccess 发布退款成功事件，优先 RocketMQ，兜底落 payment_events（幂等）。
func (s *Service) publishRefundSuccess(ctx context.Context, orderNo string, payload string) error {
	if _, err := s.repo.FindEventByOrderNoAndType(ctx, orderNo, entity.EventTypeRefundSuccess); err == nil {
		return nil
	} else if !errors.Is(err, entity.ErrEventNotFound) {
		return err
	}

	if s.publisher != nil {
		_, err := s.publisher.Publish(ctx, rocketmqx.Message{
			Topic: RefundSuccessTopic,
			Tag:   "refund_success",
			Key:   orderNo,
			Body:  []byte(payload),
		})
		if err == nil {
			return nil
		}
		// MQ 发送失败不丢弃事件：落 pending 由补偿任务重发。
		s.log.Warn("rocketmq publish refund_success failed, fallback to pending event",
			zap.String("order_no", orderNo), zap.Error(err))
	}

	now := time.Now()
	event := &entity.PaymentEvent{
		ID:         "EVT-" + orderNo + "-" + entity.EventTypeRefundSuccess,
		OrderNo:    orderNo,
		EventType:  entity.EventTypeRefundSuccess,
		Payload:    payload,
		Status:     entity.EventStatusPending,
		RetryCount: 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	return s.repo.SaveEvent(ctx, event)
}

// ProcessPendingEvents 补偿扫描：重发 payment_events 中 pending 的事件（P1）。
// 每轮最多处理 limit 条：publisher 可用且发送成功 → success；发送失败或 publisher 不可用 →
// RetryCount+1，超过 MaxEventRetry 标记 failed（保留现场供告警/人工介入）。
func (s *Service) ProcessPendingEvents(ctx context.Context, limit int) (int, error) {
	events, err := s.repo.ListPendingEvents(ctx, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, e := range events {
		if e == nil {
			continue
		}
		topic, tag := topicForEvent(e.EventType)
		var publishErr error
		if s.publisher != nil {
			_, publishErr = s.publisher.Publish(ctx, rocketmqx.Message{
				Topic: topic,
				Tag:   tag,
				Key:   e.OrderNo,
				Body:  []byte(e.Payload),
			})
		} else {
			publishErr = errors.New("event publisher not configured")
		}
		if publishErr == nil {
			if err := s.repo.UpdateEventStatus(ctx, e.ID, entity.EventStatusSuccess, e.RetryCount); err != nil {
				return processed, err
			}
			processed++
			continue
		}
		// 重发失败：重试计数递增，超限标 failed。
		retry := e.RetryCount + 1
		status := entity.EventStatusPending
		if retry >= entity.MaxEventRetry {
			status = entity.EventStatusFailed
		}
		if err := s.repo.UpdateEventStatus(ctx, e.ID, status, retry); err != nil {
			return processed, err
		}
		processed++
		s.log.Warn("pending event resend failed", zap.String("event_no", e.ID), zap.String("order_no", e.OrderNo),
			zap.Int("retry", retry), zap.Error(publishErr))
	}
	return processed, nil
}

// topicForEvent 事件类型 → RocketMQ topic/tag。
func topicForEvent(eventType string) (string, string) {
	switch eventType {
	case entity.EventTypeRefundSuccess:
		return RefundSuccessTopic, "refund_success"
	default:
		return PaySuccessTopic, "pay_success"
	}
}

func (s *Service) tryLock(ctx context.Context, key string, ttl time.Duration) (*redisx.Lock, bool, error) {
	if s.locker == nil {
		// 未配置分布式锁视为直行（幂等仍由状态机/唯一键保证），避免误拒所有通知。
		return nil, true, nil
	}
	return s.locker.TryAcquire(ctx, key, ttl)
}

func generatePaymentNo(orderNo string) string {
	return "PAY-" + orderNo
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func toDTO(p *entity.Payment) *dto.PaymentDTO {
	return &dto.PaymentDTO{
		ID:             p.ID,
		OrderNo:        p.OrderNo,
		UserID:         p.UserID,
		Subject:        p.Subject,
		Amount:         p.Amount,
		PaidAmount:     p.PaidAmount,
		Status:         p.Status,
		Channel:        p.Channel,
		TradeNo:        p.TradeNo,
		RefundedAmount: p.RefundedAmount,
		PrepayID:       p.PrepayID,
		ExpireAt:       p.ExpireAt,
		FailReason:     p.FailReason,
		NotifyCount:    p.NotifyCount,
		PaidAt:         p.PaidAt,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}
