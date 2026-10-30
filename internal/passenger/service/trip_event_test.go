package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// seedAcceptedOrder 造一张已接单的订单并放进内存仓储。
func seedAcceptedOrder(t *testing.T, orders *fakeOrderRepository, now time.Time) *entity.Order {
	t.Helper()
	order, err := entity.NewOrder("order-1", "HXZ-TEST-1", "account-1", now)
	if err != nil {
		t.Fatalf("NewOrder() error = %v", err)
	}
	order.State = entity.OrderStateAccepted
	if err := orders.Save(context.Background(), order); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return order
}

func tripEndedBody(t *testing.T, orderNo string) []byte {
	t.Helper()
	body, err := json.Marshal(contracts.TripEndedPayload{
		EventID: "tte-1", OrderID: orderNo, DriverID: "driver-1",
		EndedAt: "2026-10-30T02:00:00Z", DistanceMeters: 5200, DurationSec: 900,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return body
}

// TestHandleTripEndedMovesOrderToUnpaid 覆盖真实故障：司机从"已接单"直接送达时
// 订单必须推进到待支付，否则乘客端永远停在"已接单"、也拿不到支付入口。
func TestHandleTripEndedMovesOrderToUnpaid(t *testing.T) {
	now := time.Now()
	svc, _, orders, _, _, _ := newExtendedService(t, now)
	order := seedAcceptedOrder(t, orders, now)

	if err := svc.HandleTripEnded(context.Background(), tripEndedBody(t, order.OrderNo)); err != nil {
		t.Fatalf("HandleTripEnded() error = %v", err)
	}
	stored, err := orders.FindByOrderNo(context.Background(), order.OrderNo)
	if err != nil {
		t.Fatalf("FindByOrderNo() error = %v", err)
	}
	if stored.State != entity.OrderStateCompletedUnpaid {
		t.Fatalf("state = %s, want COMPLETED_UNPAID", stored.State)
	}
}

// TestHandleTripEndedIsIdempotent 重复投递（MQ 重投/重试）不应报错也不应改变终态。
func TestHandleTripEndedIsIdempotent(t *testing.T) {
	now := time.Now()
	svc, _, orders, _, _, _ := newExtendedService(t, now)
	order := seedAcceptedOrder(t, orders, now)
	body := tripEndedBody(t, order.OrderNo)

	for i := 0; i < 3; i++ {
		if err := svc.HandleTripEnded(context.Background(), body); err != nil {
			t.Fatalf("HandleTripEnded() round %d error = %v", i+1, err)
		}
	}
	stored, err := orders.FindByOrderNo(context.Background(), order.OrderNo)
	if err != nil {
		t.Fatalf("FindByOrderNo() error = %v", err)
	}
	if stored.State != entity.OrderStateCompletedUnpaid {
		t.Fatalf("state = %s, want COMPLETED_UNPAID", stored.State)
	}
}

// TestHandleTripEndedUnknownOrderDropped 不属于本域的订单（历史/其它环境）
// 直接丢弃，避免消息被反复重投堵塞队列。
func TestHandleTripEndedUnknownOrderDropped(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _, _ := newExtendedService(t, now)
	if err := svc.HandleTripEnded(context.Background(), tripEndedBody(t, "HXZ-NOT-EXIST")); err != nil {
		t.Fatalf("HandleTripEnded() error = %v, want nil for unknown order", err)
	}
	if err := svc.HandleTripEnded(context.Background(), []byte("not-json")); err == nil {
		t.Fatal("HandleTripEnded() accepted broken payload, want error")
	}
}

// TestAdvanceToSkipsReachedIntermediateStates 直接锁定 advanceTo 的推进语义：
// 链上出现与当前状态相同的中间态时必须继续往后推进，而不是终止。
func TestAdvanceToSkipsReachedIntermediateStates(t *testing.T) {
	now := time.Now()
	svc, _, orders, _, _, _ := newExtendedService(t, now)
	order := seedAcceptedOrder(t, orders, now)

	chain := []entity.OrderState{
		entity.OrderStateAccepted,
		entity.OrderStateArrived,
		entity.OrderStateInService,
		entity.OrderStateCompletedUnpaid,
	}
	if err := svc.advanceTo(context.Background(), order, chain, "test", now); err != nil {
		t.Fatalf("advanceTo() error = %v", err)
	}
	if order.State != entity.OrderStateCompletedUnpaid {
		t.Fatalf("state = %s, want COMPLETED_UNPAID", order.State)
	}
}
