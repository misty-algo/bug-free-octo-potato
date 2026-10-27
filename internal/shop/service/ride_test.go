package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	pricingentity "github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
	pricingservice "github.com/BwCloudWeGo/bw-cli/internal/pricing/service"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// fakeTrips 是 entity.TripRepository 的内存实现，带 version 乐观锁。
type fakeTrips struct {
	items map[string]*entity.Trip
}

func newFakeTrips() *fakeTrips {
	return &fakeTrips{items: make(map[string]*entity.Trip)}
}

func (f *fakeTrips) Save(ctx context.Context, t *entity.Trip) error {
	if existing, ok := f.items[t.ID]; ok && existing.Version != t.Version {
		return entity.ErrTripVersionConflict
	}
	stored := *t
	stored.Version = t.Version + 1
	f.items[t.ID] = &stored
	t.Version = stored.Version
	return nil
}

func (f *fakeTrips) FindByID(ctx context.Context, id string) (*entity.Trip, error) {
	item, ok := f.items[id]
	if !ok {
		return nil, entity.ErrTripNotFound
	}
	copied := *item
	return &copied, nil
}

func (f *fakeTrips) ListByPassenger(ctx context.Context, passengerID string, limit int) ([]*entity.Trip, error) {
	var out []*entity.Trip
	for _, item := range f.items {
		if item.PassengerID == passengerID {
			copied := *item
			out = append(out, &copied)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListPendingBillTrips 按创建时间升序返回已结束未建账的行程，最多 limit 条。
// 与 Mongo 实现保持一致的老单先补语义。
func (f *fakeTrips) ListPendingBillTrips(ctx context.Context, limit int) ([]*entity.Trip, error) {
	var out []*entity.Trip
	for _, item := range f.items {
		if item.Status == entity.TripStatusEnded && !item.BillCreated {
			copied := *item
			out = append(out, &copied)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// fakeSnapshots 固定返回一条计价规则快照，佣金率可控，便于断言分账口径。
type fakeSnapshots struct {
	rule pricingentity.PricingRule
}

func (f *fakeSnapshots) SnapshotByOrder(ctx context.Context, orderNo string) (pricingentity.PricingRule, error) {
	return f.rule, nil
}

// fakeSettlement 嵌入接口以继承全部方法，只覆盖本用例用到的方法。
type fakeSettlement struct {
	settlementv1.SettlementServiceClient

	calls    int
	lastReq  *settlementv1.CreateBillRequest
	failBill bool

	// 支付回调链路：置 PAID → 分账。
	markPaidCalled bool
	lastMarkPaid   *settlementv1.MarkBillPaidRequest
	splitCalled    bool
	lastSplit      *settlementv1.SplitBillRequest
}

func (f *fakeSettlement) CreateBill(ctx context.Context, in *settlementv1.CreateBillRequest,
	opts ...grpc.CallOption) (*settlementv1.BillResponse, error) {
	f.calls++
	f.lastReq = in
	if f.failBill {
		return nil, errors.New("settlement unavailable")
	}
	return &settlementv1.BillResponse{BillNo: "BILL-" + in.GetOrderNo(), OrderNo: in.GetOrderNo()}, nil
}

func (f *fakeSettlement) MarkBillPaid(ctx context.Context, in *settlementv1.MarkBillPaidRequest,
	opts ...grpc.CallOption) (*settlementv1.BillResponse, error) {
	f.markPaidCalled = true
	f.lastMarkPaid = in
	return &settlementv1.BillResponse{BillNo: "BILL-" + in.GetOrderNo(), OrderNo: in.GetOrderNo()}, nil
}

func (f *fakeSettlement) SplitBill(ctx context.Context, in *settlementv1.SplitBillRequest,
	opts ...grpc.CallOption) (*settlementv1.SplitBillResponse, error) {
	f.splitCalled = true
	f.lastSplit = in
	return &settlementv1.SplitBillResponse{
		BillNo:             "BILL-" + in.GetOrderNo(),
		DriverIncome:       8000, // 10000 * (1 - 20%)，与 2000BP 锁定费率一致
		PlatformCommission: 2000,
		Status:             "split",
	}, nil
}

// newRideService 组装带行程、计价与结算依赖的服务。
func newRideService(rule pricingentity.PricingRule) (*Service, *fakeTrips, *fakeSettlement) {
	svc := NewService(nil, zap.NewNop())
	trips := newFakeTrips()
	settlement := &fakeSettlement{}
	svc.SetTripRepository(trips)
	svc.SetPricingService(pricingservice.New(&fakeSnapshots{rule: rule}))
	svc.SetSettlementClient(settlement)
	return svc, trips, settlement
}

func testRule() pricingentity.PricingRule {
	return pricingentity.PricingRule{
		BaseFee:          1000, // 起步价 10 元
		BaseDistanceM:    3000,
		BaseDurationSec:  600,
		MinFee:           500,
		MaxFee:           100000,
		CommissionRateBP: 2000, // 平台抽 20%
	}
}

// TestReconcilePendingBills 建账补偿：行程已计价但建账失败时，补偿把账单补出来。
func TestReconcilePendingBills(t *testing.T) {
	svc, trips, settlement := newRideService(testRule())
	ctx := context.Background()

	// 建账失败，制造「行程已结束、已计价、但账单不存在」的中间态。
	settlement.failBill = true
	trips.items["TRIP-B1"] = endedTripWithoutBill(t, svc, "TRIP-B1")
	// 未结束的行程不该被动。
	_ = createWaitingTrip(t, svc, "TRIP-B2")

	// 结算在建账上报错：单笔失败跳过，不阻断整批，本批算 0 单。
	got, err := svc.ReconcilePendingBills(ctx, 10)
	if err != nil {
		t.Fatalf("ReconcilePendingBills should not fail the whole batch: %v", err)
	}
	if got != 0 {
		t.Fatalf("compensated with failing settlement = %d, want 0", got)
	}

	// 结算恢复后，补偿把账单补出来。
	settlement.failBill = false
	compensated, err := svc.ReconcilePendingBills(ctx, 10)
	if err != nil {
		t.Fatalf("ReconcilePendingBills failed: %v", err)
	}
	if compensated != 1 {
		t.Fatalf("compensated = %d, want 1", compensated)
	}
	trip, err := trips.FindByID(ctx, "TRIP-B1")
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if !trip.BillCreated {
		t.Fatalf("bill not created after reconcile")
	}
	// 锁定费率必须来自计价规则快照，而不是零值。
	if settlement.lastReq.GetCommissionRateBp() != 2000 {
		t.Fatalf("commission rate = %d, want 2000", settlement.lastReq.GetCommissionRateBp())
	}
	if settlement.lastReq.GetOrderNo() != "TRIP-B1" {
		t.Fatalf("bill order_no = %q, want TRIP-B1", settlement.lastReq.GetOrderNo())
	}

	// 再跑一次：幂等，不会再建。
	again, err := svc.ReconcilePendingBills(ctx, 10)
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}
	if again != 0 {
		t.Fatalf("second compensated = %d, want 0", again)
	}
}

// TestReconcilePendingBillsRequiresSettlement 结算客户端未注入时补偿必须显式报错，
// 不能降级成「扫到 → 什么都没做 → 记为成功」，否则会逐轮空转且毫无告警。
func TestReconcilePendingBillsRequiresSettlement(t *testing.T) {
	svc := NewService(nil, zap.NewNop())
	svc.SetTripRepository(newFakeTrips())
	if _, err := svc.ReconcilePendingBills(context.Background(), 10); err == nil {
		t.Fatalf("ReconcilePendingBills without settlement client: want error")
	}
	// 批次上限也必须为正，否则 limit<=0 会让扫描退化。
	if _, err := svc.ReconcilePendingBills(context.Background(), 0); err == nil {
		t.Fatalf("ReconcilePendingBills with limit 0: want error")
	}
}

// TestReconcilePendingBillsRespectsBatchLimit 补偿按批次上限处理，避免一次扫全表。
func TestReconcilePendingBillsRespectsBatchLimit(t *testing.T) {
	svc, _, settlement := newRideService(testRule())
	ctx := context.Background()
	settlement.failBill = true
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("TRIP-C%d", i)
		if err := svc.trips.Save(ctx, endedTripWithoutBill(t, svc, id)); err != nil {
			t.Fatalf("seed trip failed: %v", err)
		}
	}
	settlement.failBill = false

	if got, _ := svc.ReconcilePendingBills(ctx, 2); got != 2 {
		t.Fatalf("ReconcilePendingBills(2) = %d, want 2", got)
	}
	if got, _ := svc.ReconcilePendingBills(ctx, 2); got != 2 {
		t.Fatalf("ReconcilePendingBills(2) = %d, want 2", got)
	}
	if got, _ := svc.ReconcilePendingBills(ctx, 2); got != 1 {
		t.Fatalf("ReconcilePendingBills(2) = %d, want 1", got)
	}
}

// TestBillReconcileWorkerLifecycle 建账补偿工人启动即扫一批，且停止时能干净退出。
func TestBillReconcileWorkerLifecycle(t *testing.T) {
	svc, trips, settlement := newRideService(testRule())
	ctx := context.Background()
	settlement.failBill = true
	if err := svc.trips.Save(ctx, endedTripWithoutBill(t, svc, "TRIP-W1")); err != nil {
		t.Fatalf("seed trip failed: %v", err)
	}
	settlement.failBill = false

	worker := NewBillReconcileWorker(svc, zap.NewNop(), 10*time.Millisecond, 100)
	worker.Start()

	// 启动即扫一批：轮询等待账单落地（补偿是异步的）。
	deadline := time.Now().Add(2 * time.Second)
	for {
		trip, err := trips.FindByID(ctx, "TRIP-W1")
		if err == nil && trip.BillCreated {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bill reconcile worker did not compensate within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	worker.Stop()
	// 停止后可再次停止（幂等），不应 panic 或挂死。
	worker.Stop()
}

// endedTripWithoutBill 造一条「已结束、已计价、但账单未建出」的行程——
// 这是建账补偿要救的中间态，真实场景源于 EndTrip 里计价成功而建账失败。
func endedTripWithoutBill(t *testing.T, svc *Service, tripID string) *entity.Trip {
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
	// 建账在 EndTrip 内失败：行程与计价已落库，账单没建出来。
	if _, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: tripID, DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	}); err == nil {
		t.Fatalf("EndTrip should fail when bill creation fails")
	}
	trip, err := svc.trips.FindByID(ctx, tripID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if trip.Status != entity.TripStatusEnded || trip.BillCreated {
		t.Fatalf("precondition failed: status=%s bill_created=%v", trip.Status, trip.BillCreated)
	}
	if trip.PassengerPaid <= 0 {
		t.Fatalf("precondition failed: PassengerPaid = %d", trip.PassengerPaid)
	}
	return trip
}

// createWaitingTrip 造一条待接单的行程，用于验证补偿不会误伤未结束的行程。
func createWaitingTrip(t *testing.T, svc *Service, tripID string) *entity.Trip {
	t.Helper()
	if _, err := svc.CreateTrip(context.Background(), dto.CreateTripCommand{
		TripID: tripID, PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	return nil
}

func point(lng, lat float64) entity.GeoPoint {
	return entity.GeoPoint{Type: "Point", Coordinates: []float64{lng, lat}}
}

// TestCreateTrip 乘客下单落一条 WAITING 行程。
func TestCreateTrip(t *testing.T) {
	svc, _, _ := newRideService(testRule())
	ctx := context.Background()

	got, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID:      "TRIP-1",
		PassengerID: "PAX-1",
		Origin:      point(104.06, 30.57),
		Destination: point(104.08, 30.65),
	})
	if err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if got.Status != entity.TripStatusWaiting {
		t.Fatalf("status = %s, want WAITING", got.Status)
	}

	// 非法坐标必须被拒。
	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-2", PassengerID: "PAX-1",
		Origin: point(999, 0), Destination: point(104.08, 30.65),
	}); err == nil {
		t.Fatalf("CreateTrip with out-of-range origin should fail")
	}
}

// TestTripStatusTransitions 六态流转：正常路径走通，非法流转被拒。
func TestTripStatusTransitions(t *testing.T) {
	svc, _, _ := newRideService(testRule())
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-3", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}

	// WATING 不能直接上车（必须先接单、到达）。
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: "TRIP-3"}); err == nil {
		t.Fatalf("board from WAITING should fail")
	}

	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: "TRIP-3", DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if _, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: "TRIP-3"}); err != nil {
		t.Fatalf("DriverArrived failed: %v", err)
	}
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: "TRIP-3"}); err != nil {
		t.Fatalf("BoardTrip failed: %v", err)
	}
	got, err := svc.GetTrip(ctx, "TRIP-3")
	if err != nil {
		t.Fatalf("GetTrip failed: %v", err)
	}
	if got.Status != entity.TripStatusInTrip {
		t.Fatalf("status = %s, want IN_TRIP", got.Status)
	}
}

// TestEndTripFareAndBill 行程结束：计价回填金额与锁定费率，并按该费率建账。
func TestEndTripFareAndBill(t *testing.T) {
	svc, trips, settlement := newRideService(testRule())
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-4", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: "TRIP-4", DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if _, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: "TRIP-4"}); err != nil {
		t.Fatalf("DriverArrived failed: %v", err)
	}
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: "TRIP-4"}); err != nil {
		t.Fatalf("BoardTrip failed: %v", err)
	}

	got, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: "TRIP-4", DriverID: "DRV-1",
		DistanceM: 8000, DurationSec: 1200, WaitSec: 120,
	})
	if err != nil {
		t.Fatalf("EndTrip failed: %v", err)
	}
	if got.Status != entity.TripStatusEnded {
		t.Fatalf("status = %s, want ENDED", got.Status)
	}
	if got.PassengerPaid <= 0 {
		t.Fatalf("passenger_paid = %d, want positive", got.PassengerPaid)
	}
	// 佣金率必须等于计价规则快照里的值，而不是 0 或调用方传的。
	if got.CommissionRateBP != 2000 {
		t.Fatalf("commission_rate_bp = %d, want 2000", got.CommissionRateBP)
	}

	// 建账请求必须带上锁定费率与实付金额。
	if settlement.calls != 1 {
		t.Fatalf("CreateBill calls = %d, want 1", settlement.calls)
	}
	if settlement.lastReq.GetCommissionRateBp() != 2000 {
		t.Fatalf("bill commission_rate_bp = %d, want 2000", settlement.lastReq.GetCommissionRateBp())
	}
	if settlement.lastReq.GetTotalAmount() != got.PassengerPaid {
		t.Fatalf("bill total_amount = %d, want %d", settlement.lastReq.GetTotalAmount(), got.PassengerPaid)
	}
	if settlement.lastReq.GetDriverId() != "DRV-1" {
		t.Fatalf("bill driver_id = %q, want DRV-1", settlement.lastReq.GetDriverId())
	}
	if !trips.items["TRIP-4"].BillCreated {
		t.Fatalf("BillCreated flag not persisted")
	}
}

// TestEndTripIdempotent 重复结束：不重复计价、不重复建账。
func TestEndTripIdempotent(t *testing.T) {
	svc, _, settlement := newRideService(testRule())
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-5", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: "TRIP-5", DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if _, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: "TRIP-5"}); err != nil {
		t.Fatalf("DriverArrived failed: %v", err)
	}
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: "TRIP-5"}); err != nil {
		t.Fatalf("BoardTrip failed: %v", err)
	}

	first, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: "TRIP-5", DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	})
	if err != nil {
		t.Fatalf("EndTrip failed: %v", err)
	}
	second, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: "TRIP-5", DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	})
	if err != nil {
		t.Fatalf("repeated EndTrip failed: %v", err)
	}
	if first.PassengerPaid != second.PassengerPaid {
		t.Fatalf("passenger_paid changed on repeat: %d -> %d", first.PassengerPaid, second.PassengerPaid)
	}
	if settlement.calls != 1 {
		t.Fatalf("CreateBill calls = %d, want 1", settlement.calls)
	}
}

// TestEndTripRetriesBillAfterFailure 建账失败后可重入补建，且不会重复计价。
func TestEndTripRetriesBillAfterFailure(t *testing.T) {
	svc, _, settlement := newRideService(testRule())
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-6", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	if _, err := svc.AssignDriver(ctx, dto.AssignDriverCommand{TripID: "TRIP-6", DriverID: "DRV-1"}); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if _, err := svc.DriverArrived(ctx, dto.TripActionCommand{TripID: "TRIP-6"}); err != nil {
		t.Fatalf("DriverArrived failed: %v", err)
	}
	if _, err := svc.BoardTrip(ctx, dto.TripActionCommand{TripID: "TRIP-6"}); err != nil {
		t.Fatalf("BoardTrip failed: %v", err)
	}

	settlement.failBill = true
	if _, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: "TRIP-6", DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	}); err == nil {
		t.Fatalf("EndTrip should fail when bill creation fails")
	}

	// 故障恢复后重试：只补建账，不重新计价。
	settlement.failBill = false
	got, err := svc.EndTrip(ctx, dto.EndTripCommand{
		TripID: "TRIP-6", DriverID: "DRV-1", DistanceM: 8000, DurationSec: 1200,
	})
	if err != nil {
		t.Fatalf("retry EndTrip failed: %v", err)
	}
	if !got.BillCreated {
		t.Fatalf("bill not created after retry")
	}
	if got.PassengerPaid <= 0 {
		t.Fatalf("passenger_paid = %d, want positive", got.PassengerPaid)
	}
}

// TestTripVersionConflict 并发写同一行程：后写者拿到版本冲突，不会静默覆盖。
func TestTripVersionConflict(t *testing.T) {
	svc, trips, _ := newRideService(testRule())
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-7", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}

	// 模拟两个并发读：司机接单与乘客取消各持一份旧版本。
	a, _ := trips.FindByID(ctx, "TRIP-7")
	b, _ := trips.FindByID(ctx, "TRIP-7")

	if err := a.AssignDriver("DRV-1", time.Now()); err != nil {
		t.Fatalf("AssignDriver failed: %v", err)
	}
	if err := trips.Save(ctx, a); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	if err := b.Cancel(time.Now()); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if err := trips.Save(ctx, b); !errors.Is(err, entity.ErrTripVersionConflict) {
		t.Fatalf("stale save error = %v, want ErrTripVersionConflict", err)
	}
}
