package service

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// orderCreatedEvent 兼容乘客域（樊凯军 P07）发布的 order.created 事件。
// 初版事件负载不含坐标与金额，缺失字段使用课程演示默认值：
// 起点春熙路、终点天府广场、车费 28.80 元（2880 分）。
type orderCreatedEvent struct {
	EventID string `json:"event_id"`
	ID      string `json:"id"`
	OrderID string `json:"order_id"`
	Name    string `json:"name"`
	Description string `json:"description"`
	PassengerName string `json:"passenger_name"`
	PassengerPaidFen int64 `json:"passenger_paid_fen"`
	PlatformCouponFen int64 `json:"platform_coupon_fen"`
}

// HandleOrderCreated 消费乘客下单事件：去重后生成派单任务并进入单通道分发。
func (s *Service) HandleOrderCreated(ctx context.Context, body []byte) error {
	var evt orderCreatedEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return err
	}
	eventID := evt.EventID
	if eventID == "" {
		eventID = fmt.Sprintf("order-created-%s", firstNonEmpty(evt.OrderID, evt.ID))
	}
	claimed, err := s.support.ClaimInbound(ctx, eventID, contracts.EventOrderCreated, localConsumer)
	if err != nil || !claimed {
		return nil // 同一事件重复投递：直接忽略
	}
	paid := evt.PassengerPaidFen
	if paid <= 0 {
		paid = 2880
	}
	name := firstNonEmpty(evt.PassengerName, evt.Name, "乘客")
	cmd := dto.CreateTaskCommand{
		OrderID:           firstNonEmpty(evt.OrderID, evt.ID, eventID),
		PassengerName:     name,
		PassengerCount:    1,
		CityCode:          "510100",
		OriginName:        "春熙路", OriginLat: 30.6570, OriginLng: 104.0810,
		DestName: "天府广场", DestLat: 30.6570, DestLng: 104.0650,
		PassengerPaidFen:  paid,
		PlatformCouponFen: evt.PlatformCouponFen,
		EventID:           eventID,
	}
	if _, err := s.CreateTask(ctx, cmd); err != nil {
		return err
	}
	s.log.Info("order created from mq event", zap.String("event_id", eventID),
		zap.String("order_id", cmd.OrderID), zap.String("occurred_at", biztime.RFC3339(biztime.UTC())))
	return nil
}

// PublishPendingEvents 可靠事件投递：把待投递事件发布到消息队列后标记为已发布。
// broker 不可用时保留 pending 状态等待下次重试，保证跨模块事件不丢。
func (s *Service) PublishPendingEvents(ctx context.Context, limit int) (int, error) {
	pending, err := s.support.FetchPendingEvents(ctx, limit)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, e := range pending {
		if s.publisher == nil {
			return published, nil
		}
		if err := s.publisher(ctx, e.EventType, []byte(e.Payload)); err != nil {
			_ = s.support.MarkFailed(ctx, e.EventID, err.Error())
			continue
		}
		if err := s.support.MarkPublished(ctx, e.EventID); err != nil {
			continue
		}
		published++
	}
	return published, nil
}

// SetEventPublisher 注入事件投递器（在 cmd 接线时传入 RabbitMQ 发布函数）。
func (s *Service) SetEventPublisher(publisher func(ctx context.Context, routingKey string, body []byte) error) {
	s.publisher = publisher
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
