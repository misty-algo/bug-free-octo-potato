package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/payment/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/payment/entity"
)

// fakeRepo 是 entity.Repository 的内存实现，用于 service 单测。
type fakeRepo struct {
	byOrd   map[string]*entity.Payment
	byPay   map[string]*entity.Payment
	byKey   map[string]*entity.Payment
	events  map[string]*entity.PaymentEvent
	refunds map[string]*entity.Refund
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byOrd:   make(map[string]*entity.Payment),
		byPay:   make(map[string]*entity.Payment),
		byKey:   make(map[string]*entity.Payment),
		events:  make(map[string]*entity.PaymentEvent),
		refunds: make(map[string]*entity.Refund),
	}
}

func (f *fakeRepo) SavePayment(ctx context.Context, payment *entity.Payment) error {
	f.byOrd[payment.OrderNo] = payment
	f.byPay[payment.ID] = payment
	f.byKey[payment.IdempotentKey] = payment
	return nil
}

func (f *fakeRepo) FindPaymentByOrderNo(ctx context.Context, orderNo string) (*entity.Payment, error) {
	p, ok := f.byOrd[orderNo]
	if !ok {
		return nil, entity.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakeRepo) FindPaymentByPaymentNo(ctx context.Context, paymentNo string) (*entity.Payment, error) {
	p, ok := f.byPay[paymentNo]
	if !ok {
		return nil, entity.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakeRepo) FindPaymentByIdempotentKey(ctx context.Context, key string) (*entity.Payment, error) {
	p, ok := f.byKey[key]
	if !ok {
		return nil, entity.ErrPaymentNotFound
	}
	return p, nil
}

func (f *fakeRepo) SaveEvent(ctx context.Context, event *entity.PaymentEvent) error {
	f.events[event.ID] = event
	return nil
}

func (f *fakeRepo) FindEventByOrderNoAndType(ctx context.Context, orderNo string, eventType string) (*entity.PaymentEvent, error) {
	id := "EVT-" + orderNo + "-" + eventType
	e, ok := f.events[id]
	if !ok {
		return nil, entity.ErrEventNotFound
	}
	return e, nil
}

func (f *fakeRepo) UpdateEventStatus(ctx context.Context, eventNo string, status string, retryCount int) error {
	e, ok := f.events[eventNo]
	if !ok {
		return entity.ErrEventNotFound
	}
	e.Status = status
	e.RetryCount = retryCount
	return nil
}

func (f *fakeRepo) ListPendingEvents(ctx context.Context, limit int) ([]*entity.PaymentEvent, error) {
	var out []*entity.PaymentEvent
	for _, e := range f.events {
		if e.Status == entity.EventStatusPending {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepo) SaveRefund(ctx context.Context, refund *entity.Refund) error {
	f.refunds[refund.ID] = refund
	return nil
}

func (f *fakeRepo) FindRefundByRefundNo(ctx context.Context, refundNo string) (*entity.Refund, error) {
	r, ok := f.refunds[refundNo]
	if !ok {
		return nil, entity.ErrRefundNotFound
	}
	return r, nil
}

func (f *fakeRepo) FindRefundByOutRequestNo(ctx context.Context, outRequestNo string) (*entity.Refund, error) {
	for _, r := range f.refunds {
		if r.OutRequestNo == outRequestNo {
			return r, nil
		}
	}
	return nil, entity.ErrRefundNotFound
}

func (f *fakeRepo) ListRefundsByOrderNo(ctx context.Context, orderNo string) ([]*entity.Refund, error) {
	var out []*entity.Refund
	for _, r := range f.refunds {
		if r.OrderNo == orderNo {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeRepo) AddRefundedAmount(ctx context.Context, paymentNo string, amount int64) error {
	p, ok := f.byPay[paymentNo]
	if !ok {
		return entity.ErrPaymentNotFound
	}
	if p.RefundedAmount+amount > p.PaidAmount {
		return entity.ErrRefundExceedsPaid
	}
	p.RefundedAmount += amount
	return nil
}

func newTestService() *Service {
	return NewService(newFakeRepo(), zap.NewNop())
}

// TestCreatePaymentIdempotent 同幂等键重复创建返回已有支付单；不同幂等键拒绝。
func TestCreatePaymentIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	first, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo:       "ORDER-2001",
		Subject:       "打车订单",
		Amount:        8800,
		Channel:       "alipay",
		IdempotentKey: "idem-2001",
	})
	if err != nil {
		t.Fatalf("first CreatePayment failed: %v", err)
	}

	// 同 order_no + 同幂等键 → 返回已有记录，不报错
	second, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo:       "ORDER-2001",
		Subject:       "打车订单",
		Amount:        8800,
		Channel:       "alipay",
		IdempotentKey: "idem-2001",
	})
	if err != nil {
		t.Fatalf("idempotent CreatePayment failed: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("idempotent create returned different payment: %s vs %s", first.ID, second.ID)
	}

	// 同 order_no + 不同幂等键 → ErrPaymentAlreadyExists
	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo:       "ORDER-2001",
		Subject:       "打车订单",
		Amount:        8800,
		Channel:       "alipay",
		IdempotentKey: "idem-2001-other",
	}); err == nil {
		t.Fatalf("CreatePayment with different idempotent key should fail")
	} else if !errors.Is(err, entity.ErrPaymentAlreadyExists) {
		t.Fatalf("error = %v, want ErrPaymentAlreadyExists", err)
	}
}

// TestNotifyPaymentIdempotent 重复通知不重复入账：第二次标记 duplicate，事件仅一条。
func TestNotifyPaymentIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2002",
		Subject: "打车订单",
		Amount:  6600,
		Channel: "alipay",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}

	first, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2002",
		TradeNo:    "ALI-TRADE-001",
		Channel:    "alipay",
		PaidAmount: 6600,
		PaidAt:     1700000000,
	})
	if err != nil {
		t.Fatalf("first NotifyPayment failed: %v", err)
	}
	if first.Duplicate {
		t.Fatalf("first notify should not be duplicate")
	}

	// publisher 为 nil 时事件应落 pending 表，且仅一条
	events, err := svc.repo.ListPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("ListPendingEvents failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("pending events count = %d, want 1", len(events))
	}

	second, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2002",
		TradeNo:    "ALI-TRADE-001",
		Channel:    "alipay",
		PaidAmount: 6600,
		PaidAt:     1700000000,
	})
	if err != nil {
		t.Fatalf("second NotifyPayment failed: %v", err)
	}
	if !second.Duplicate {
		t.Fatalf("second notify should be duplicate")
	}

	events, _ = svc.repo.ListPendingEvents(ctx, 10)
	if len(events) != 1 {
		t.Fatalf("pending events count after duplicate = %d, want 1", len(events))
	}

	payment, err := svc.QueryPayment(ctx, "ORDER-2002")
	if err != nil {
		t.Fatalf("QueryPayment failed: %v", err)
	}
	if payment.Status != entity.PaymentStatusSuccess || payment.PaidAmount != 6600 {
		t.Fatalf("payment after notify: status=%s paid=%d, want success/6600", payment.Status, payment.PaidAmount)
	}
}

// TestRefundPaymentRejectsTwice 累计退款不得超过实付金额（含在途 REVIEW）。
func TestRefundPaymentRejectsTwice(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2003",
		Subject: "打车订单",
		Amount:  5000,
		Channel: "alipay",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if _, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2003",
		TradeNo:    "ALI-TRADE-003",
		PaidAmount: 5000,
	}); err != nil {
		t.Fatalf("NotifyPayment failed: %v", err)
	}

	first, err := svc.RefundPayment(ctx, dto.RefundPaymentCommand{OrderNo: "ORDER-2003", RefundAmount: 3000, OutRequestNo: "R-1"})
	if err != nil {
		t.Fatalf("first RefundPayment failed: %v", err)
	}
	if first.Status != entity.RefundStatusReview {
		t.Fatalf("first refund status = %s, want REVIEW（待审核）", first.Status)
	}
	if first.Success {
		t.Fatalf("first refund should not succeed before review")
	}

	if _, err := svc.RefundPayment(ctx, dto.RefundPaymentCommand{OrderNo: "ORDER-2003", RefundAmount: 3000, OutRequestNo: "R-2"}); err == nil {
		t.Fatalf("second RefundPayment should fail (in-flight refunds exceed paid amount)")
	} else if !errors.Is(err, entity.ErrRefundExceedsPaid) {
		t.Fatalf("error = %v, want ErrRefundExceedsPaid", err)
	}
}

// TestReviewRefundFlow 退款审核全流程：拒绝 → 通过 → 渠道成功 → 累计退款；超退拒绝。
func TestReviewRefundFlow(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2004",
		Subject: "打车订单",
		Amount:  10000,
		Channel: "wechat",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if _, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2004",
		TradeNo:    "WX-TRADE-004",
		PaidAmount: 10000,
	}); err != nil {
		t.Fatalf("NotifyPayment failed: %v", err)
	}

	// 发起两笔退款（REVIEW），在途 4000+3000=7000 ≤ 10000
	r1, err := svc.RefundPayment(ctx, dto.RefundPaymentCommand{OrderNo: "ORDER-2004", RefundAmount: 4000, OutRequestNo: "R-A", Reason: "多收费"})
	if err != nil {
		t.Fatalf("RefundPayment R-A failed: %v", err)
	}
	r2, err := svc.RefundPayment(ctx, dto.RefundPaymentCommand{OrderNo: "ORDER-2004", RefundAmount: 3000, OutRequestNo: "R-B", Reason: "部分退"})
	if err != nil {
		t.Fatalf("RefundPayment R-B failed: %v", err)
	}

	// 再发起 4000 会导致 7000+4000 > 10000，超退拒绝
	if _, err := svc.RefundPayment(ctx, dto.RefundPaymentCommand{OrderNo: "ORDER-2004", RefundAmount: 4000, OutRequestNo: "R-C"}); err == nil {
		t.Fatalf("RefundPayment R-C should fail (exceeds)")
	} else if !errors.Is(err, entity.ErrRefundExceedsPaid) {
		t.Fatalf("error = %v, want ErrRefundExceedsPaid", err)
	}

	// 拒绝 r2（R-B）：REVIEW → REJECTED
	rejected, err := svc.ReviewRefund(ctx, dto.ReviewRefundCommand{RefundNo: r2.RefundNo, Approved: false, RejectReason: "超出应退金额"})
	if err != nil {
		t.Fatalf("ReviewRefund reject failed: %v", err)
	}
	if rejected.Status != entity.RefundStatusRejected {
		t.Fatalf("rejected status = %s, want REJECTED", rejected.Status)
	}

	// 通过 r1（R-A）：REVIEW → PROCESSING → 渠道成功 → SUCCESS，支付单累计退款 4000
	approved, err := svc.ReviewRefund(ctx, dto.ReviewRefundCommand{RefundNo: r1.RefundNo, Approved: true})
	if err != nil {
		t.Fatalf("ReviewRefund approve failed: %v", err)
	}
	if approved.Status != entity.RefundStatusSuccess {
		t.Fatalf("approved status = %s, want SUCCESS", approved.Status)
	}
	if !approved.Success {
		t.Fatalf("approved refund should succeed")
	}

	payment, err := svc.QueryPayment(ctx, "ORDER-2004")
	if err != nil {
		t.Fatalf("QueryPayment failed: %v", err)
	}
	if payment.Status != entity.PaymentStatusSuccess {
		t.Fatalf("payment status = %s, want SUCCESS（支付单不受退款影响）", payment.Status)
	}
	if payment.RefundedAmount != 4000 {
		t.Fatalf("payment refunded_amount = %d, want 4000", payment.RefundedAmount)
	}

	// 已拒绝的 r2 不可再提交渠道
	if _, err := svc.ReviewRefund(ctx, dto.ReviewRefundCommand{RefundNo: r2.RefundNo, Approved: true}); err == nil {
		t.Fatalf("submitting rejected refund should fail")
	} else if !errors.Is(err, entity.ErrRefundStateTransition) {
		t.Fatalf("error = %v, want ErrRefundStateTransition", err)
	}
}

// TestPaymentStateMachine 支付五态流转：CREATED → PROCESSING → SUCCESS；CLOSED 拒绝支付成功。
func TestPaymentStateMachine(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2005",
		Subject: "打车订单",
		Amount:  8800,
		Channel: "alipay",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}

	// CREATED → PROCESSING（渠道受理）
	begin, err := svc.BeginPayment(ctx, dto.BeginPaymentCommand{OrderNo: "ORDER-2005", PrepayID: "ALI-PREPAY-001"})
	if err != nil {
		t.Fatalf("BeginPayment failed: %v", err)
	}
	if begin.Status != entity.PaymentStatusProcessing {
		t.Fatalf("status after begin = %s, want PROCESSING", begin.Status)
	}
	if begin.PrepayID != "ALI-PREPAY-001" {
		t.Fatalf("prepay_id = %s, want ALI-PREPAY-001", begin.PrepayID)
	}

	// PROCESSING → SUCCESS
	if _, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2005",
		TradeNo:    "ALI-TRADE-005",
		PaidAmount: 8800,
	}); err != nil {
		t.Fatalf("NotifyPayment failed: %v", err)
	}

	// SUCCESS 不可再 Begin（幂等返回）/ Close（拒绝）
	if _, err := svc.BeginPayment(ctx, dto.BeginPaymentCommand{OrderNo: "ORDER-2005"}); err != nil {
		t.Fatalf("BeginPayment on SUCCESS should be idempotent ok: %v", err)
	}
	if _, err := svc.ClosePayment(ctx, dto.ClosePaymentCommand{OrderNo: "ORDER-2005", Reason: "超时"}); err == nil {
		t.Fatalf("ClosePayment on SUCCESS should fail")
	}
}

// TestClosePaymentFlow 关单：CREATED → CLOSED，CLOSED 后渠道成功通知被拒绝。
func TestClosePaymentFlow(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2006",
		Subject: "打车订单",
		Amount:  5000,
		Channel: "wechat",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}

	closed, err := svc.ClosePayment(ctx, dto.ClosePaymentCommand{OrderNo: "ORDER-2006", Reason: "15分钟未支付自动关单"})
	if err != nil {
		t.Fatalf("ClosePayment failed: %v", err)
	}
	if closed.Status != entity.PaymentStatusClosed {
		t.Fatalf("status after close = %s, want CLOSED", closed.Status)
	}

	// 幂等：重复关单返回已有 CLOSED
	if again, err := svc.ClosePayment(ctx, dto.ClosePaymentCommand{OrderNo: "ORDER-2006"}); err != nil {
		t.Fatalf("idempotent ClosePayment failed: %v", err)
	} else if again.Status != entity.PaymentStatusClosed {
		t.Fatalf("idempotent close status = %s", again.Status)
	}

	// 关单后渠道才回成功 → 拒绝入账
	if _, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2006",
		TradeNo:    "WX-TRADE-006",
		PaidAmount: 5000,
	}); err == nil {
		t.Fatalf("NotifyPayment on CLOSED should fail")
	} else if !errors.Is(err, entity.ErrPaymentClosed) {
		t.Fatalf("error = %v, want ErrPaymentClosed", err)
	}
}

// TestFailPaymentFlow 支付失败：PROCESSING → FAILED → 渠道最终确认成功允许恢复 SUCCESS。
func TestFailPaymentFlow(t *testing.T) {
	ctx := context.Background()
	svc := newTestService()

	if _, err := svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo: "ORDER-2007",
		Subject: "打车订单",
		Amount:  3000,
		Channel: "alipay",
	}); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}
	if _, err := svc.BeginPayment(ctx, dto.BeginPaymentCommand{OrderNo: "ORDER-2007"}); err != nil {
		t.Fatalf("BeginPayment failed: %v", err)
	}

	failed, err := svc.FailPayment(ctx, dto.FailPaymentCommand{OrderNo: "ORDER-2007", FailReason: "余额不足"})
	if err != nil {
		t.Fatalf("FailPayment failed: %v", err)
	}
	if failed.Status != entity.PaymentStatusFailed {
		t.Fatalf("status after fail = %s, want FAILED", failed.Status)
	}
	if failed.FailReason != "余额不足" {
		t.Fatalf("fail_reason = %s", failed.FailReason)
	}

	// FAILED → SUCCESS（渠道最终确认成功）
	notify, err := svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    "ORDER-2007",
		TradeNo:    "ALI-TRADE-007",
		PaidAmount: 3000,
	})
	if err != nil {
		t.Fatalf("NotifyPayment after fail failed: %v", err)
	}
	if notify.Status != entity.PaymentStatusSuccess {
		t.Fatalf("status after notify = %s, want SUCCESS", notify.Status)
	}
}
