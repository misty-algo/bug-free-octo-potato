package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/gopay"
)

// fakePayments 是 entity.PaymentRepository 的内存实现，用于校验行程支付落库。
type fakePayments struct {
	items map[string]*entity.Payment
}

func newFakePayments() *fakePayments {
	return &fakePayments{items: make(map[string]*entity.Payment)}
}

func (f *fakePayments) Create(ctx context.Context, p *entity.Payment) error {
	if _, ok := f.items[p.OrderNo]; ok {
		return entity.ErrPaymentExists
	}
	stored := *p
	f.items[p.OrderNo] = &stored
	return nil
}

func (f *fakePayments) Update(ctx context.Context, p *entity.Payment) error {
	f.items[p.OrderNo] = p
	return nil
}

func (f *fakePayments) FindByOrderNo(ctx context.Context, orderNo string) (*entity.Payment, error) {
	item, ok := f.items[orderNo]
	if !ok {
		return nil, entity.ErrPaymentNotFound
	}
	copied := *item
	return &copied, nil
}

func (f *fakePayments) ListPendingBefore(ctx context.Context, before time.Time, limit int) ([]*entity.Payment, error) {
	return nil, nil
}

// newPayTripService 组装带行程、计价、结算、支付单与支付宝客户端的服务。
// alipay 用零值客户端：requireAlipay 只校验非 nil，业务守卫因此可被完整测到，
// 真正发起支付时会因内部客户端为空而失败——那一步不属单元测试职责。
func newPayTripService() (*Service, *fakeTrips, *fakePayments) {
	svc, trips, _ := newRideService(testRule())
	payments := newFakePayments()
	svc.SetPaymentRepository(payments)
	svc.alipay = &gopay.Client{}
	return svc, trips, payments
}

// readyToPay 结束一条行程（计价 + 建账），返回行程号供支付用例使用。
func readyToPay(t *testing.T, svc *Service, tripID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: tripID, PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: tripID, DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if _, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: tripID}); err != nil {
		t.Fatalf("DriverArrived failed: %v", err)
	}
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: tripID}); err != nil {
		t.Fatalf("BoardTrip failed: %v", err)
	}
	if _, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: tripID, DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	}); err != nil {
		t.Fatalf("EndTrip failed: %v", err)
	}
}

// TestPayTripRejectsUnfinishedTrip 未结束的行程不允许收款（防预扣）。
func TestPayTripRejectsUnfinishedTrip(t *testing.T) {
	svc, _, _ := newPayTripService()
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-P1", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: "TRIP-P1", DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	// ACCEPTED 状态：行程还没跑完，不能收钱。
	_, err := svc.PayTrip(ctx, dto.PayTripCommand{TripID: "TRIP-P1", Scene: "wap"})
	if err == nil || !errors.Is(err, entity.ErrInvalidTripStatus) {
		t.Fatalf("PayTrip on ACCEPTED trip: err = %v, want ErrInvalidTripStatus", err)
	}
}

// TestPayTripAmountComesFromFare 支付金额必须等于计价结果，调用方无法指定金额。
func TestPayTripAmountComesFromFare(t *testing.T) {
	svc, trips, payments := newPayTripService()
	ctx := context.Background()
	readyToPay(t, svc, "TRIP-P2")

	trip, err := trips.FindByID(ctx, "TRIP-P2")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if trip.PassengerPaid <= 0 {
		t.Fatalf("precondition failed: PassengerPaid = %d", trip.PassengerPaid)
	}

	// 真实网关调用会失败（测试客户端内部为空），但守卫与落库已完成：
	// 支付单号与金额在发起支付前就已写入。
	_, _ = svc.PayTrip(ctx, dto.PayTripCommand{TripID: "TRIP-P2", Scene: "wap"})

	stored, err := payments.FindByOrderNo(ctx, "TRIP-P2")
	if err != nil {
		t.Fatalf("payment not created: %v", err)
	}
	// 支付单号 = 行程号 = 账单 OrderNo：回调据此一路找到账单。
	if stored.OrderNo != "TRIP-P2" {
		t.Fatalf("payment order_no = %q, want TRIP-P2", stored.OrderNo)
	}
	// 金额取自计价结果，不是调用方传参——这是防篡改的核心断言。
	if stored.Amount != trip.PassengerPaid {
		t.Fatalf("payment amount = %d, want fare %d", stored.Amount, trip.PassengerPaid)
	}
	if stored.Channel != entity.PaymentChannelAlipay {
		t.Fatalf("channel = %s, want alipay", stored.Channel)
	}
}

// TestPayTripRejectsDuplicatePayment 同一行程已付款后再次发起支付应被拒绝。
func TestPayTripRejectsDuplicatePayment(t *testing.T) {
	svc, trips, payments := newPayTripService()
	ctx := context.Background()
	readyToPay(t, svc, "TRIP-P3")

	trip, err := trips.FindByID(ctx, "TRIP-P3")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	// 种子支付单金额必须与计价结果一致，否则会先撞上金额不符的守卫。
	if err := payments.Create(ctx, &entity.Payment{
		OrderNo: "TRIP-P3", Amount: trip.PassengerPaid, Channel: entity.PaymentChannelAlipay,
		Status: entity.PaymentStatusSuccess,
	}); err != nil {
		t.Fatalf("seed payment failed: %v", err)
	}
	_, err = svc.PayTrip(ctx, dto.PayTripCommand{TripID: "TRIP-P3", Scene: "wap"})
	if err == nil || !errors.Is(err, entity.ErrPaymentExists) {
		t.Fatalf("PayTrip on paid trip: err = %v, want ErrPaymentExists", err)
	}
}

// TestPayTripRejectsAmountMismatch 已存在的支付单金额与计价结果不符时必须拒绝。
// 这是防金额篡改的最后一道闸：账单、支付单、计价结果三者必须一致。
func TestPayTripRejectsAmountMismatch(t *testing.T) {
	svc, _, payments := newPayTripService()
	ctx := context.Background()
	readyToPay(t, svc, "TRIP-P5")

	// 刻意塞入一个与计价结果不符的历史支付单（模拟数据被改或重复下单带了错金额）。
	if err := payments.Create(ctx, &entity.Payment{
		OrderNo: "TRIP-P5", Amount: 1, Channel: entity.PaymentChannelAlipay,
		Status: entity.PaymentStatusPending,
	}); err != nil {
		t.Fatalf("seed payment failed: %v", err)
	}
	_, err := svc.PayTrip(ctx, dto.PayTripCommand{TripID: "TRIP-P5", Scene: "wap"})
	if err == nil || !errors.Is(err, entity.ErrPaymentAmountMismatch) {
		t.Fatalf("PayTrip with mismatched amount: err = %v, want ErrPaymentAmountMismatch", err)
	}
}

// TestPayTripRejectsInvalidScene 非法的支付场景在建单前就被拒绝。
func TestPayTripRejectsInvalidScene(t *testing.T) {
	svc, _, _ := newPayTripService()
	ctx := context.Background()
	readyToPay(t, svc, "TRIP-P4")

	_, err := svc.PayTrip(ctx, dto.PayTripCommand{TripID: "TRIP-P4", Scene: "hack"})
	if err == nil || !errors.Is(err, entity.ErrInvalidPaymentRequest) {
		t.Fatalf("PayTrip with bad scene: err = %v, want ErrInvalidPaymentRequest", err)
	}
}

// TestEndToEndTripToSplit 端到端：下单 → 送达计价建账 → 支付 → 回调 → 分账入司机钱包。
// 这是「乘客下单 → 支付宝支付 → 支付回调 → 结算分账」全链路的贯通验证。
func TestEndToEndTripToSplit(t *testing.T) {
	svc, trips, settlement := newRideService(testRule())
	ctx := context.Background()

	const tripID = "TRIP-E2E"
	readyToPayWithSvc(t, svc, tripID)

	// 1. 行程结束已计价并按锁定费率建账。
	trip, err := trips.FindByID(ctx, tripID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if trip.Status != entity.TripStatusEnded {
		t.Fatalf("status = %s, want ENDED", trip.Status)
	}
	if trip.CommissionRateBP != 2000 {
		t.Fatalf("locked rate = %d, want 2000", trip.CommissionRateBP)
	}
	if !trip.BillCreated {
		t.Fatalf("bill not created after EndTrip")
	}
	if settlement.calls != 1 {
		t.Fatalf("CreateBill calls = %d, want 1", settlement.calls)
	}
	if settlement.lastReq.GetOrderNo() != tripID {
		t.Fatalf("bill order_no = %q, want %q", settlement.lastReq.GetOrderNo(), tripID)
	}

	// 2. 模拟支付宝回调到账：走与 NotifyPayment 相同的结算入口。
	//    支付单号即行程号，因此无需任何映射就能定位到账单。
	if err := svc.settleAfterPayment(ctx, tripID, trip.PassengerPaid); err != nil {
		t.Fatalf("settleAfterPayment failed: %v", err)
	}

	// 3. 司机钱包按锁定费率入账：先置 PAID，再按锁定费率分账。
	if !settlement.markPaidCalled {
		t.Fatalf("MarkBillPaid not called on payment callback")
	}
	if settlement.lastMarkPaid.GetPayFee() != trip.PassengerPaid {
		t.Fatalf("mark paid fee = %d, want %d", settlement.lastMarkPaid.GetPayFee(), trip.PassengerPaid)
	}
	if !settlement.splitCalled {
		t.Fatalf("SplitBill not called on payment callback")
	}
	// 分账必须按建账锁定的费率，而不是任何调用方传入的值。
	if !settlement.lastSplit.GetUseBillRate() {
		t.Fatalf("SplitBill must use locked bill rate")
	}
	if settlement.lastSplit.GetOrderNo() != tripID {
		t.Fatalf("split order_no = %q, want %q", settlement.lastSplit.GetOrderNo(), tripID)
	}
}

// readyToPayWithSvc 结束一条行程，供端到端用例复用。
func readyToPayWithSvc(t *testing.T, svc *Service, tripID string) {
	t.Helper()
	ctx := context.Background()
	steps := []func() error{
		func() error {
			_, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
				TripID: tripID, PassengerID: "PAX-1",
				Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
			})
			return err
		},
		func() error {
			_, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: tripID, DriverID: "DRV-1"})
			return err
		},
		func() error {
			_, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: tripID})
			return err
		},
		func() error {
			_, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: tripID})
			return err
		},
		func() error {
			_, err := svc.EndTrip(ctx, dto.EndTripCommand{
				TripID: tripID, DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
			})
			return err
		},
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("e2e setup step %d failed: %v", i+1, err)
		}
	}
}
