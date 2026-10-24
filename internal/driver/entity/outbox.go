package entity

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
)

// 可靠事件表的事件投递状态。
const (
	EventStatusPending   = "pending"
	EventStatusPublished = "published"
	EventStatusFailed    = "failed"
)

// OutboxEvent 可靠事件：与业务写入同事务落地，由后台投递器发布到消息队列，
// 消费者按 event_id 去重，保证跨模块事件至少一次投递且不重复记账。
type OutboxEvent struct {
	EventID   string
	EventType string
	Payload   string // JSON
	Version   int64
	Status    string
	ErrorMsg  string
	RetryTimes int
	CreatedAt time.Time
	PublishedAt time.Time
}

// NewOutboxEvent 创建待投递事件。
func NewOutboxEvent(eventID, eventType, payload string) *OutboxEvent {
	now := biztime.UTC()
	return &OutboxEvent{
		EventID:    eventID,
		EventType:  eventType,
		Payload:    payload,
		Version:    1,
		Status:     EventStatusPending,
		CreatedAt:  now,
	}
}

// InboundEvent 已消费的外部事件记录（按 event_id + consumer 去重）。
type InboundEvent struct {
	EventID    string
	EventType  string
	Consumer   string
	ProcessedAt time.Time
}

// CommandRecord 幂等命令记录：相同幂等键返回原结果，不重复执行。
type CommandRecord struct {
	Key       string
	Scope     string
	TargetID  string
	ResultRef string
	CreatedAt time.Time
}
