package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/shop/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoOutboxRepository 使用共享 mongox 文档存储持久化出站可靠事件。
type MongoOutboxRepository struct {
	documents *mongox.DocumentStore[dbmodel.OutboxEventDocument]
	// db 用于创建索引：mongox.Collection 只封装了 CRUD，未暴露底层集合。
	db  *mongo.Database
	log *zap.Logger
}

// NewMongoOutboxRepository 创建出站事件仓储。
func NewMongoOutboxRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoOutboxRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoOutboxRepository{
		documents: mongox.NewDocumentStore[dbmodel.OutboxEventDocument](db, log),
		db:        db,
		log:       log,
	}
}

// EnqueueEvent 写入待投递事件。event_id 为主键，重复落地直接忽略——
// 下单重试会产生同一条事件，靠这里保证不重复通知司机端。
func (r *MongoOutboxRepository) EnqueueEvent(ctx context.Context, e *entity.OutboxEvent) error {
	start := time.Now()
	_, err := r.documents.Insert(ctx, &dbmodel.OutboxEventDocument{
		EventID:    e.EventID,
		EventType:  e.EventType,
		Payload:    e.Payload,
		Status:     e.Status,
		ErrorMsg:   e.ErrorMsg,
		RetryTimes: e.RetryTimes,
		CreatedAt:  e.CreatedAt,
	})
	if mongo.IsDuplicateKeyError(err) {
		r.logOperation("EnqueueEvent(duplicate)", e.EventID, start, nil)
		return nil
	}
	r.logOperation("EnqueueEvent", e.EventID, start, err)
	return err
}

// FetchPendingEvents 按创建时间升序返回待投递事件，最多 limit 条。
func (r *MongoOutboxRepository) FetchPendingEvents(ctx context.Context, limit int) ([]*entity.OutboxEvent, error) {
	start := time.Now()
	if limit <= 0 {
		limit = 50
	}
	opt := options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "created_at", Value: 1}})
	documents, err := r.documents.FindMany(ctx, bson.M{"status": entity.EventStatusPending}, opt)
	r.logOperation("FetchPendingEvents", "pending", start, err)
	if err != nil {
		return nil, err
	}
	out := make([]*entity.OutboxEvent, 0, len(documents))
	for i := range documents {
		d := &documents[i]
		out = append(out, &entity.OutboxEvent{
			EventID:     d.EventID,
			EventType:   d.EventType,
			Payload:     d.Payload,
			Status:      d.Status,
			ErrorMsg:    d.ErrorMsg,
			RetryTimes:  d.RetryTimes,
			CreatedAt:   d.CreatedAt,
			PublishedAt: d.PublishedAt,
		})
	}
	return out, nil
}

// MarkPublished 标记事件已成功投递。
func (r *MongoOutboxRepository) MarkPublished(ctx context.Context, eventID string) error {
	start := time.Now()
	_, err := r.documents.UpdateOne(ctx, bson.M{"_id": eventID}, bson.M{"$set": bson.M{
		"status": entity.EventStatusPublished, "published_at": biztime.UTC(), "error_msg": "",
	}})
	r.logOperation("MarkPublished", eventID, start, err)
	return err
}

// MarkFailed 记录一次投递失败：重试次数 +1，达到上限才把事件置为 failed。
//
// 未达上限时保持 pending，让下一轮继续捞起——MQ 抖动这类瞬时故障应当自动恢复，
// 不该因为一次失败就永久丢弃一条下单事件（那等于这个订单永远派不出车）。
// 达到上限后置 failed 移出待投队列，避免一条坏事件反复失败、把后面正常的事件饿死。
//
// 计数与状态判定放在同一条聚合更新里，保证自增与判定的原子性（需 MongoDB 4.2+）。
func (r *MongoOutboxRepository) MarkFailed(ctx context.Context, eventID, errMsg string, maxRetries int) error {
	start := time.Now()
	nextRetries := bson.M{"$add": bson.A{"$retry_times", 1}}
	_, err := r.documents.UpdateOne(ctx, bson.M{"_id": eventID}, mongo.Pipeline{
		bson.D{{Key: "$set", Value: bson.M{
			"retry_times": nextRetries,
			"error_msg":   errMsg,
			"status": bson.M{"$cond": bson.A{
				bson.M{"$gte": bson.A{nextRetries, maxRetries}},
				entity.EventStatusFailed,
				"$status",
			}},
		}}},
	})
	r.logOperation("MarkFailed", eventID, start, err)
	return err
}

// EnsureIndexes 建立投递扫描所需的索引。
//
// 待投事件按 status 过滤、created_at 排序，发布循环默认每 2 秒跑一次，
// 缺索引会退化成全表扫描，随着事件累积逐渐拖垮整个 shop 服务。
// 索引创建失败不阻断启动：功能仍可用，只是扫描变慢。
func (r *MongoOutboxRepository) EnsureIndexes(ctx context.Context) error {
	_, err := r.db.Collection(dbmodel.OutboxCollectionName).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: 1}},
		Options: options.Index().SetName("idx_outbox_pending_scan"),
	})
	return err
}

func (r *MongoOutboxRepository) logOperation(operation, id string, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_outbox"),
		zap.String("operation", operation),
		zap.String("event_id", id),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("mongodb repository operation completed with error", fields...)
		return
	}
	r.log.Info("mongodb repository operation completed", fields...)
}
