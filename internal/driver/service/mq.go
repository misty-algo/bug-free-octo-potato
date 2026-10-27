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
// 携带坐标时以事件为准；缺失字段才使用课程演示默认值：
// 起点春熙路、终点天府广场、车费 28.80 元（2880 分）。
//
// 坐标必须优先取事件值：派单按距离与片区筛选司机，若一律回落默认值，
// 司机会被引导到与乘客实际位置无关的上车点，派单结果直接失效。
type orderCreatedEvent struct {
	EventID           string       `json:"event_id"`
	ID                string       `json:"id"`
	OrderID           string       `json:"order_id"`
	Name              string       `json:"name"`
	Description       string       `json:"description"`
	PassengerName     string       `json:"passenger_name"`
	PassengerPaidFen  int64        `json:"passenger_paid_fen"`
	PlatformCouponFen int64        `json:"platform_coupon_fen"`
	Pickup            *eventLatLng `json:"pickup"`
	Dropoff           *eventLatLng `json:"dropoff"`
}

// eventLatLng 事件内的 GCJ02 坐标点。
type eventLatLng struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// usable 判断坐标是否可用于派单：经纬度都在合法范围内且非零。
// 零值坐标视为「未上报」而非「在几内亚湾」，避免把司机派到 (0,0)。
func (p *eventLatLng) usable() bool {
	return p != nil && p.Lat != 0 && p.Lng != 0 &&
		p.Lat >= -90 && p.Lat <= 90 && p.Lng >= -180 && p.Lng <= 180
}

// resolvePoint 决定派单使用的地点：事件携带了可用坐标时以事件为准，
// 否则回落到课程演示默认值（fallbackName/Lat/Lng），地点名缺省用 defaultName。
//
// 必须优先取事件值：派单按距离与片区筛选司机，若一律回落默认值，
// 司机会被引导到与乘客实际位置无关的上车点，派单结果直接失效。
func resolvePoint(p *eventLatLng, fallbackName string, fallbackLat, fallbackLng float64,
	defaultName string) (name string, lat, lng float64) {
	if !p.usable() {
		return fallbackName, fallbackLat, fallbackLng
	}
	return firstNonEmpty(p.Name, defaultName), p.Lat, p.Lng
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
	originName, originLat, originLng := resolvePoint(evt.Pickup,
		"春熙路", 30.6570, 104.0810, "上车点")
	destName, destLat, destLng := resolvePoint(evt.Dropoff,
		"天府广场", 30.6570, 104.0650, "目的地")
	cmd := dto.CreateTaskCommand{
		OrderID:        firstNonEmpty(evt.OrderID, evt.ID, eventID),
		PassengerName:  name,
		PassengerCount: 1,
		CityCode:       "510100",
		OriginName:     originName, OriginLat: originLat, OriginLng: originLng,
		DestName: destName, DestLat: destLat, DestLng: destLng,
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
