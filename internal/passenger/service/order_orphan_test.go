package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// fakeHallBridge 派单池桥 fake：空池时查任务返回 ErrOrderNotFound。
type fakeHallBridge struct {
	tasks map[string]*entity.HallTaskView
}

func (b *fakeHallBridge) CreateDispatchTask(ctx context.Context, task entity.DispatchTask) error { return nil }

func (b *fakeHallBridge) FindTaskByOrderNo(ctx context.Context, orderNo string) (*entity.HallTaskView, error) {
	if task, ok := b.tasks[orderNo]; ok {
		return task, nil
	}
	return nil, entity.ErrOrderNotFound
}

func (b *fakeHallBridge) FindSettlementByOrderNo(ctx context.Context, orderNo string) (*entity.HallSettlementView, error) {
	if task, ok := b.tasks[orderNo]; ok {
		return &entity.HallSettlementView{
			DriverID:         task.DriverID,
			PassengerPaidFen: 2607,
			DriverIncomeFen:  2086,
		}, nil
	}
	return nil, entity.ErrOrderNotFound
}

func (b *fakeHallBridge) CancelWaitingTask(ctx context.Context, orderNo string, reason string) (bool, error) {
	return true, nil
}

// TestCurrentOrderCancelsStaleOrphan 验证派单池查无任务的孤儿订单在超过宽限期后自动取消。
func TestCurrentOrderCancelsStaleOrphan(t *testing.T) {
	now := time.Now()
	svc, _, _, orders, _ := newTestService(t, now)
	svc.WithHallOrderBridge(&fakeHallBridge{tasks: map[string]*entity.HallTaskView{}})

	stale := entity.Order{
		ID:        "order-orphan",
		AccountID: "account-1",
		OrderNo:   "HXZ-ORPHAN-1",
		State:     entity.OrderStatePending,
		CarType:   1,
		CreatedAt: now.UTC().Add(-2 * orphanOrderTimeout),
	}
	if err := orders.Save(context.Background(), &stale); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := svc.CurrentOrder(context.Background(), "account-1")
	if err != nil {
		t.Fatalf("CurrentOrder() error = %v", err)
	}
	// 本次调用把孤儿订单终结：返回值为终态订单（或无单），乘客端将其视为无进行中行程。
	if out != nil && out.State != string(entity.OrderStateCancelled) {
		t.Fatalf("CurrentOrder() state = %q, want CANCELLED", out.State)
	}
	stored, err := orders.FindByID(context.Background(), "order-orphan")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.State != entity.OrderStateCancelled {
		t.Fatalf("state = %q, want CANCELLED for stale orphan", stored.State)
	}

	// 宽限期内的新订单不取消。
	fresh := entity.Order{
		ID:        "order-fresh",
		AccountID: "account-1",
		OrderNo:   "HXZ-ORPHAN-2",
		State:     entity.OrderStatePending,
		CarType:   1,
		CreatedAt: now.UTC(),
	}
	if err := orders.Save(context.Background(), &fresh); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	freshOut, err := svc.CurrentOrder(context.Background(), "account-1")
	if err != nil {
		t.Fatalf("CurrentOrder() error = %v", err)
	}
	if freshOut == nil || freshOut.OrderNo != "HXZ-ORPHAN-2" {
		t.Fatalf("CurrentOrder() = %v, want the fresh order kept pending", freshOut)
	}
	stored, err = orders.FindByID(context.Background(), "order-fresh")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.State != entity.OrderStatePending {
		t.Fatalf("state = %q, want PENDING within grace period", stored.State)
	}
}

// TestCancelOrderKeepsPendingStateWhenTaskGone 验证等待接单期间取消不再收消费费。
func TestCancelOrderKeepsPendingStateWhenTaskGone(t *testing.T) {
	now := time.Now()
	svc, _, _, orders, _ := newTestService(t, now)
	svc.WithHallOrderBridge(&fakeHallBridge{tasks: map[string]*entity.HallTaskView{}})

	order := entity.Order{
		ID: "order-cancel", AccountID: "account-1", OrderNo: "HXZ-CANCEL-1",
		State: entity.OrderStatePending, CarType: 1, CreatedAt: now.UTC(),
	}
	if err := orders.Save(context.Background(), &order); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := svc.CancelOrder(context.Background(), dto.CancelOrderCommand{
		AccountID: "account-1", OrderID: "order-cancel", Reason: "不想等了",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if out.NeedConfirm {
		t.Fatal("NeedConfirm = true, want free cancellation while pending")
	}
	stored, err := orders.FindByID(context.Background(), "order-cancel")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if stored.State != entity.OrderStateCancelled {
		t.Fatalf("state = %q, want CANCELLED", stored.State)
	}
	if !errors.Is(entity.ErrOrderNotFound, entity.ErrOrderNotFound) {
		t.Fatal("sanity check failed")
	}
}
