package service

import (
	"context"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

func newUnpaidOrder(t *testing.T, svc *Service, orders *fakeOrderRepository, bridge *fakeHallBridge) *entity.Order {
	t.Helper()
	order := &entity.Order{
		ID: "order-pay-1", AccountID: "account-1", OrderNo: "HXZ-PAY-1",
		State: entity.OrderStateCompletedUnpaid, CarType: 1, TotalFee: 2607,
	}
	if err := orders.Save(context.Background(), order); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	bridge.tasks[order.OrderNo] = &entity.HallTaskView{
		TaskID: "task-1", Status: "ENDED", DriverID: "driver-9", DriverName: "张师傅",
	}
	svc.WithHallOrderBridge(bridge)
	return order
}

func TestMarkOrderPaidCompletesOrder(t *testing.T) {
	now := time.Now()
	svc, _, _, orders, _ := newTestService(t, now)
	bridge := &fakeHallBridge{tasks: map[string]*entity.HallTaskView{}}
	order := newUnpaidOrder(t, svc, orders, bridge)
	ctx := context.Background()

	out, err := svc.MarkOrderPaid(ctx, dto.MarkOrderPaidCommand{OrderNo: order.OrderNo, TradeNo: "trade-1"})
	if err != nil {
		t.Fatalf("MarkOrderPaid() error = %v", err)
	}
	if out.State != string(entity.OrderStateCompleted) {
		t.Fatalf("state = %q, want COMPLETED", out.State)
	}
	if out.DriverID != "driver-9" {
		t.Fatalf("driverId = %q, want driver-9", out.DriverID)
	}
	if out.DriverIncomeFen != 2086 {
		t.Fatalf("driverIncomeFen = %d, want 2086", out.DriverIncomeFen)
	}
	stored, err := orders.FindByID(ctx, order.ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.State != entity.OrderStateCompleted {
		t.Fatalf("stored state = %q, want COMPLETED", stored.State)
	}
}

func TestMarkOrderPaidIsIdempotent(t *testing.T) {
	now := time.Now()
	svc, _, _, orders, _ := newTestService(t, now)
	bridge := &fakeHallBridge{tasks: map[string]*entity.HallTaskView{}}
	order := newUnpaidOrder(t, svc, orders, bridge)
	ctx := context.Background()
	cmd := dto.MarkOrderPaidCommand{OrderNo: order.OrderNo, TradeNo: "trade-1"}

	if _, err := svc.MarkOrderPaid(ctx, cmd); err != nil {
		t.Fatalf("first MarkOrderPaid() error = %v", err)
	}
	// 重复回调：直接返回已完成订单，不得报错或重复推进。
	again, err := svc.MarkOrderPaid(ctx, cmd)
	if err != nil {
		t.Fatalf("duplicate MarkOrderPaid() error = %v", err)
	}
	if again.State != string(entity.OrderStateCompleted) {
		t.Fatalf("duplicate state = %q, want COMPLETED", again.State)
	}
}

func TestMarkOrderPaidRejectsNonUnpaidOrder(t *testing.T) {
	now := time.Now()
	svc, _, _, orders, _ := newTestService(t, now)
	bridge := &fakeHallBridge{tasks: map[string]*entity.HallTaskView{}}
	order := newUnpaidOrder(t, svc, orders, bridge)
	// 待接单订单不允许走支付成功回调。
	if err := orders.UpdateState(context.Background(), &entity.Order{
		ID: order.ID, OrderNo: order.OrderNo, AccountID: order.AccountID,
		State: entity.OrderStatePending, Version: 2,
	}); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}
	if _, err := svc.MarkOrderPaid(context.Background(), dto.MarkOrderPaidCommand{OrderNo: order.OrderNo}); err == nil {
		t.Fatal("MarkOrderPaid() on pending order should fail, got nil")
	}
}

func TestPlatformCommissionFen(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)
	svc.WithCommissionBP(2000)
	// 20% 抽成：2607 分 → 521 分（四舍五入）；10000 分 → 2000 分。
	if got := svc.platformCommissionFen(2607); got != 521 {
		t.Fatalf("platformCommissionFen(2607) = %d, want 521", got)
	}
	if got := svc.platformCommissionFen(10000); got != 2000 {
		t.Fatalf("platformCommissionFen(10000) = %d, want 2000", got)
	}
}
