package model

import "time"

// OutboxCollectionName 乘客域出站事件表。
// 与司机域的 driver_outbox_events 相互独立：各模块只通过事件交互，不读对方库表。
const OutboxCollectionName = "shop_outbox_events"

// OutboxEventDocument 出站事件文档，_id 即 event_id，天然幂等。
type OutboxEventDocument struct {
	EventID     string    `bson:"_id"`
	EventType   string    `bson:"event_type"`
	Payload     string    `bson:"payload"`
	Status      string    `bson:"status"`
	ErrorMsg    string    `bson:"error_msg"`
	RetryTimes  int       `bson:"retry_times"`
	CreatedAt   time.Time `bson:"created_at"`
	PublishedAt time.Time `bson:"published_at"`
}

// MongoCollectionName 声明文档对应集合。
func (OutboxEventDocument) MongoCollectionName() string { return OutboxCollectionName }
