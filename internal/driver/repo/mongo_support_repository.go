package repo

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
)

// supportRepo 实现可靠事件表、消费去重表与幂等命令表。
//
// 依据统一技术底座：事件与业务同事务写入本地表，再由投递器发布；
// 消费者按 event_id 去重，命令按幂等键去重，避免跨模块重复记账。
type supportRepo struct {
	db  *mongo.Database
	log *zap.Logger
}

func (r *supportRepo) EnqueueEvent(ctx context.Context, e *entity.OutboxEvent) error {
	_, err := r.db.Collection(dbmodel.OutboxCollectionName).InsertOne(ctx, dbmodel.OutboxEventDocument{
		EventID: e.EventID, EventType: e.EventType, Payload: e.Payload,
		Version: e.Version, Status: e.Status, ErrorMsg: e.ErrorMsg,
		RetryTimes: e.RetryTimes, CreatedAt: e.CreatedAt, PublishedAt: e.PublishedAt,
	})
	if mongo.IsDuplicateKeyError(err) {
		return nil // 同一事件重复落地：天然幂等
	}
	logRepoOp(r.log, "support.EnqueueEvent", e.EventID, err)
	return err
}

func (r *supportRepo) FetchPendingEvents(ctx context.Context, limit int) ([]*entity.OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	cur, err := r.db.Collection(dbmodel.OutboxCollectionName).Find(ctx,
		bson.M{"status": entity.EventStatusPending},
		options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "created_at", Value: 1}}))
	if err != nil {
		logRepoOp(r.log, "support.FetchPendingEvents", "", err)
		return nil, err
	}
	defer cur.Close(ctx)
	out := make([]*entity.OutboxEvent, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.OutboxEventDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		out = append(out, &entity.OutboxEvent{
			EventID: doc.EventID, EventType: doc.EventType, Payload: doc.Payload,
			Version: doc.Version, Status: doc.Status, ErrorMsg: doc.ErrorMsg,
			RetryTimes: doc.RetryTimes, CreatedAt: doc.CreatedAt, PublishedAt: doc.PublishedAt,
		})
	}
	return out, nil
}

func (r *supportRepo) MarkPublished(ctx context.Context, eventID string) error {
	_, err := r.db.Collection(dbmodel.OutboxCollectionName).UpdateOne(ctx, bson.M{"_id": eventID},
		bson.M{"$set": bson.M{"status": entity.EventStatusPublished, "published_at": biztime.UTC(), "error_msg": ""}})
	logRepoOp(r.log, "support.MarkPublished", eventID, err)
	return err
}

func (r *supportRepo) MarkFailed(ctx context.Context, eventID, errMsg string) error {
	_, err := r.db.Collection(dbmodel.OutboxCollectionName).UpdateOne(ctx, bson.M{"_id": eventID},
		bson.M{"$set": bson.M{"status": entity.EventStatusFailed, "error_msg": errMsg}, "$inc": bson.M{"retry_times": 1}})
	logRepoOp(r.log, "support.MarkFailed", eventID, err)
	return err
}

// ClaimInbound 外部事件去重：已处理返回 false。
func (r *supportRepo) ClaimInbound(ctx context.Context, eventID, eventType, consumer string) (bool, error) {
	coll := r.db.Collection(dbmodel.InboundCollectionName)
	_, err := coll.InsertOne(ctx, dbmodel.InboundEventDocument{
		EventID: eventID, EventType: eventType, Consumer: consumer, ProcessedAt: biztime.UTC(),
	})
	if err == nil {
		return true, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	logRepoOp(r.log, "support.ClaimInbound", eventID, err)
	return false, err
}

// ClaimCommand 幂等命令登记：首次返回 true，重复请求返回 false 与原结果。
func (r *supportRepo) ClaimCommand(ctx context.Context, rec *entity.CommandRecord) (bool, *entity.CommandRecord, error) {
	coll := r.db.Collection(dbmodel.IdempotencyCollectionName)
	_, err := coll.InsertOne(ctx, dbmodel.CommandDocument{
		Key: rec.Key, Scope: rec.Scope, TargetID: rec.TargetID, ResultRef: rec.ResultRef, CreatedAt: biztime.UTC(),
	})
	if err == nil {
		return true, rec, nil
	}
	if mongo.IsDuplicateKeyError(err) {
		var exist dbmodel.CommandDocument
		if e := coll.FindOne(ctx, bson.M{"_id": rec.Key}).Decode(&exist); e == nil {
			return false, &entity.CommandRecord{
				Key: exist.Key, Scope: exist.Scope, TargetID: exist.TargetID,
				ResultRef: exist.ResultRef, CreatedAt: exist.CreatedAt,
			}, nil
		}
		return false, nil, nil
	}
	logRepoOp(r.log, "support.ClaimCommand", rec.Key, err)
	return false, nil, err
}

// EnsureIndexes 建立跨模块一致性所需的唯一约束：
//   - hall_orders.order_id：一个订单只能有一个任务；
//   - income_flows.event_id：收益事件去重，防止重复记账；
//   - withdraw_requests(driver_id, idempotency_key)：提现幂等；
//   - driver_profiles.active_task_id：一司机一个活动任务（关键唯一性）；
//   - driver_ledgers.idempotency_key：分录幂等。
func (r *supportRepo) EnsureIndexes(ctx context.Context) error {
	type idx struct {
		coll   string
		model  mongo.IndexModel
	}
	indexes := []idx{
		{
			coll: dbmodel.HallOrderCollectionName,
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "order_id", Value: 1}},
				Options: options.Index().SetUnique(true).SetSparse(true),
			},
		},
		{
			coll: dbmodel.IncomeFlowCollectionName,
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "event_id", Value: 1}},
				Options: options.Index().SetUnique(true).SetSparse(true),
			},
		},
		{
			coll: dbmodel.WithdrawCollectionName,
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "driver_id", Value: 1}, {Key: "idempotency_key", Value: 1}},
				Options: options.Index().SetUnique(true).SetSparse(true),
			},
		},
		{
			coll: dbmodel.LedgerCollectionName,
			model: mongo.IndexModel{
				Keys:    bson.D{{Key: "idempotency_key", Value: 1}},
				Options: options.Index().SetUnique(true).SetSparse(true),
			},
		},
		{
			// 一司机一个活动任务：只对绑定了任务的司机建约束。
			coll: dbmodel.DriverCollectionName,
			model: mongo.IndexModel{
				Keys: bson.D{{Key: "active_task_id", Value: 1}},
				Options: options.Index().SetUnique(true).
					SetPartialFilterExpression(bson.M{"active_task_id": bson.M{"$gt": ""}}),
			},
		},
	}
	for _, i := range indexes {
		if _, err := r.db.Collection(i.coll).Indexes().CreateOne(ctx, i.model); err != nil {
			// 索引已存在或参数不同：记录告警但不阻断启动。
			logRepoOp(r.log, "support.EnsureIndexes."+i.coll, "", err)
		}
	}
	return nil
}
