package entity

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
)

// 可靠事件状态。
const (
	EventStatusPending   = "pending"
	EventStatusPublished = "published"
	EventStatusFailed    = "failed"
)

// OutboxEvent 出站可靠事件：与业务写入同事务落地，由后台投递器发布到消息队列。
//
// 存在的原因：乘客下单后必须通知司机端生成派单任务。若直接发 MQ，broker 抖动或
// 进程重启都会让事件丢失——行程停在 WAITING 永远等不到司机，且无任何告警。
// 先落库再异步投递，broker 不可用只影响延迟，不影响送达。
//
// 消费者按 event_id 去重，配合这里 event_id 唯一的落库幂等，
// 跨模块事件做到「至少一次投递 + 不重复处理」。
type OutboxEvent struct {
	EventID     string
	EventType   string
	Payload     string // JSON
	Status      string
	ErrorMsg    string
	RetryTimes  int
	CreatedAt   time.Time
	PublishedAt time.Time
}

// NewOutboxEvent 创建待投递事件。eventID 由调用方给出，需保证业务内唯一。
func NewOutboxEvent(eventID, eventType, payload string) *OutboxEvent {
	return &OutboxEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
		Status:    EventStatusPending,
		CreatedAt: biztime.UTC(),
	}
}
