package repo

import (
	"context"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const colEventLogs = "event_logs"

// MongoEventWriter 复用订单域已有的可靠事件日志集合（eventId 唯一去重）。
// 审核域只追加事件，不读取事件；下游消费方按 kind/name 订阅。
type MongoEventWriter struct{ db *mongo.Database }

func NewMongoEventWriter(db *mongo.Database) *MongoEventWriter {
	return &MongoEventWriter{db: db}
}

func (w *MongoEventWriter) AppendEvent(ctx context.Context, eventID, name, kind, detail string, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := w.db.Collection(colEventLogs).InsertOne(ctx, bson.M{
		"eventId": eventID, "t": at, "name": name, "kind": kind, "detail": detail,
	})
	// 同一 outbox 事件重投时撞 eventId 唯一索引，属幂等成功。
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return err
	}
	return nil
}

var _ entity.EventWriter = (*MongoEventWriter)(nil)
