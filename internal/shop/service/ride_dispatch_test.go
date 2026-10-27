package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// fakeOutbox 是 entity.OutboxRepository 的内存实现，用于验证派单事件的落地与投递。
type fakeOutbox struct {
	events     map[string]*entity.OutboxEvent
	failAlways bool // 模拟 broker 持续不可用
	published  []sentEvent
}

type sentEvent struct {
	routingKey string
	body       string
}

func newFakeOutbox() *fakeOutbox {
	return &fakeOutbox{events: make(map[string]*entity.OutboxEvent)}
}

func (f *fakeOutbox) EnqueueEvent(ctx context.Context, e *entity.OutboxEvent) error {
	if _, ok := f.events[e.EventID]; ok {
		return nil // event_id 唯一：重复落地直接忽略
	}
	stored := *e
	f.events[e.EventID] = &stored
	return nil
}

func (f *fakeOutbox) FetchPendingEvents(ctx context.Context, limit int) ([]*entity.OutboxEvent, error) {
	var out []*entity.OutboxEvent
	for _, e := range f.events {
		if e.Status == entity.EventStatusPending {
			copied := *e
			out = append(out, &copied)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeOutbox) MarkPublished(ctx context.Context, eventID string) error {
	e, ok := f.events[eventID]
	if !ok {
		return errors.New("event not found")
	}
	e.Status = entity.EventStatusPublished
	e.ErrorMsg = ""
	return nil
}

// MarkFailed 与 Mongo 实现同语义：次数 +1，达到上限才置 failed，否则保持 pending 可重试。
func (f *fakeOutbox) MarkFailed(ctx context.Context, eventID, errMsg string, maxRetries int) error {
	e, ok := f.events[eventID]
	if !ok {
		return errors.New("event not found")
	}
	e.RetryTimes++
	e.ErrorMsg = errMsg
	if e.RetryTimes >= maxRetries {
		e.Status = entity.EventStatusFailed
	}
	return nil
}

// newDispatchService 组装带 outbox 的服务，publisher 记录投递内容供断言。
func newDispatchService(outbox *fakeOutbox, ob *fakeOutbox) (*Service, *fakeOutbox) {
	svc, _, _ := newRideService(testRule())
	svc.SetOutboxRepository(outbox)
	svc.SetEventPublisher(func(ctx context.Context, routingKey string, body []byte) error {
		if ob.failAlways {
			return errors.New("broker unavailable")
		}
		ob.published = append(ob.published, sentEvent{routingKey: routingKey, body: string(body)})
		return nil
	})
	return svc, outbox
}

// TestCreateTripEnqueuesOrderCreated 下单必须落一条 order.created 事件，供司机端派单。
func TestCreateTripEnqueuesOrderCreated(t *testing.T) {
	outbox := newFakeOutbox()
	svc, _ := newDispatchService(outbox, outbox)
	ctx := context.Background()

	// 成都天府三街 → 天府广场；经纬度刻意非对称，便于抓出经纬写反的 bug。
	origin := point(104.0665, 30.5470)
	dest := point(104.0810, 30.6570)
	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-D1", PassengerID: "PAX-1", Origin: origin, Destination: dest,
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}

	if len(outbox.events) != 1 {
		t.Fatalf("enqueued events = %d, want 1", len(outbox.events))
	}
	var stored *entity.OutboxEvent
	for _, e := range outbox.events {
		stored = e
	}
	if stored.EventType != contracts.EventOrderCreated {
		t.Fatalf("event_type = %q, want %q", stored.EventType, contracts.EventOrderCreated)
	}
	if stored.Status != entity.EventStatusPending {
		t.Fatalf("status = %q, want pending", stored.Status)
	}

	// 载荷必须是契约定义的完整结构，且坐标按 [lng, lat] 正确还原。
	var payload contracts.OrderCreatedPayload
	if err := json.Unmarshal([]byte(stored.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.OrderID != "TRIP-D1" {
		t.Fatalf("payload order_id = %q, want TRIP-D1", payload.OrderID)
	}
	// 经纬写反会让司机被派到完全错误的位置，这里是最容易出错的一环。
	if payload.Pickup.Lng != origin.Lng() || payload.Pickup.Lat != origin.Lat() {
		t.Fatalf("pickup = (%v,%v), want (%v,%v) as (lng,lat)",
			payload.Pickup.Lng, payload.Pickup.Lat, origin.Lng(), origin.Lat())
	}
	if payload.Dropoff.Lng != dest.Lng() || payload.Dropoff.Lat != dest.Lat() {
		t.Fatalf("dropoff = (%v,%v), want (%v,%v) as (lng,lat)",
			payload.Dropoff.Lng, payload.Dropoff.Lat, dest.Lng(), dest.Lat())
	}
}

// TestCreateTripIdempotentEnqueue 重复下单同一行程号不会重复通知司机端。
func TestCreateTripIdempotentEnqueue(t *testing.T) {
	outbox := newFakeOutbox()
	svc, _ := newDispatchService(outbox, outbox)
	ctx := context.Background()

	// 行程号相同即同一单：第二次 Save 会因版本冲突失败，事件也不该重复落。
	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-D2", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	// 直接重复入队：event_id 唯一决定只留一条。
	evt := entity.NewOutboxEvent("order-created-TRIP-D2", contracts.EventOrderCreated, "{}")
	if err := outbox.EnqueueEvent(ctx, evt); err != nil {
		t.Fatalf("EnqueueEvent failed: %v", err)
	}
	if err := outbox.EnqueueEvent(ctx, evt); err != nil {
		t.Fatalf("duplicate EnqueueEvent failed: %v", err)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("events = %d, want 1 (duplicate ignored)", len(outbox.events))
	}
}

// TestPublishPendingEvents 待投事件发布成功后标记为 published。
func TestPublishPendingEvents(t *testing.T) {
	outbox := newFakeOutbox()
	svc, _ := newDispatchService(outbox, outbox)
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-D3", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}

	published, err := svc.PublishPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("PublishPendingEvents failed: %v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1", published)
	}
	if len(outbox.published) != 1 {
		t.Fatalf("broker received = %d, want 1", len(outbox.published))
	}
	if outbox.published[0].routingKey != contracts.EventOrderCreated {
		t.Fatalf("routing key = %q, want %q", outbox.published[0].routingKey, contracts.EventOrderCreated)
	}
	// 已发布的不该被重复捞起。
	again, err := svc.PublishPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if again != 0 {
		t.Fatalf("second published = %d, want 0", again)
	}
}

// TestPublishRetriesOnBrokerFailure broker 不可用时事件保持待投，达到上限才移出队列。
func TestPublishRetriesOnBrokerFailure(t *testing.T) {
	outbox := newFakeOutbox()
	outbox.failAlways = true
	svc, _ := newDispatchService(outbox, outbox)
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-D4", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}

	// broker 持续不可用：本批发布数为 0，事件保持 pending 等待重试，而不是被丢弃。
	for round := 1; round < maxEventRetries; round++ {
		if _, err := svc.PublishPendingEvents(ctx, 10); err != nil {
			t.Fatalf("publish round %d failed: %v", round, err)
		}
		pending, _ := outbox.FetchPendingEvents(ctx, 10)
		if len(pending) != 1 {
			t.Fatalf("round %d: pending = %d, want 1 (event must stay for retry)", round, len(pending))
		}
	}
	// 达到重试上限后置 failed，避免坏事件反复失败饿死后续事件。
	if _, err := svc.PublishPendingEvents(ctx, 10); err != nil {
		t.Fatalf("final publish round failed: %v", err)
	}
	pending, _ := outbox.FetchPendingEvents(ctx, 10)
	if len(pending) != 0 {
		t.Fatalf("pending after max retries = %d, want 0", len(pending))
	}
}

// TestPublishPendingEventsWithoutPublisher 未接 MQ 时事件留在表里，不丢失也不报错。
func TestPublishPendingEventsWithoutPublisher(t *testing.T) {
	outbox := newFakeOutbox()
	svc, _, _ := newRideService(testRule())
	svc.SetOutboxRepository(outbox)
	ctx := context.Background()

	if _, err := svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID: "TRIP-D5", PassengerID: "PAX-1",
		Origin: point(104.06, 30.57), Destination: point(104.08, 30.65),
	}); err != nil {
		t.Fatalf("CreateTrip failed: %v", err)
	}
	published, err := svc.PublishPendingEvents(ctx, 10)
	if err != nil {
		t.Fatalf("PublishPendingEvents without publisher: %v", err)
	}
	if published != 0 {
		t.Fatalf("published = %d, want 0", published)
	}
	pending, _ := outbox.FetchPendingEvents(ctx, 10)
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1 (event must survive until broker is wired)", len(pending))
	}
}

// TestPublishPendingEventsRequiresOutbox 未注入 outbox 时投递必须显式报错。
func TestPublishPendingEventsRequiresOutbox(t *testing.T) {
	svc := NewService(nil, zap.NewNop())
	if _, err := svc.PublishPendingEvents(context.Background(), 10); err == nil {
		t.Fatalf("PublishPendingEvents without outbox: want error")
	}
}
