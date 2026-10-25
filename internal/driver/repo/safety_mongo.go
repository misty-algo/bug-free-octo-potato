package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

const (
	colSafetyEvents = "safety_events"
	// drivers 由订单域负责建表，本域只读其中的最后已知位置。
	colDrivers = "drivers"
)

// MongoSafetyRepository 全部写入都是条件 upsert / 条件更新：
// 受理与幂等是同一次原子操作，不存在"命令记录成功但业务没落"的中间态。
type MongoSafetyRepository struct{ db *mongo.Database }

func NewMongoSafetyRepository(db *mongo.Database) *MongoSafetyRepository {
	return &MongoSafetyRepository{db: db}
}

func (r *MongoSafetyRepository) CreateReport(ctx context.Context, event entity.SafetyEvent) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{
		"driverId":            event.DriverID,
		"raw.clientRequestId": event.Raw.ClientRequestID,
	}
	res, err := r.db.Collection(colSafetyEvents).UpdateOne(ctx, filter,
		bson.M{"$setOnInsert": event}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		// 并发同一请求号撞唯一索引：不算失败，交给调用方比较内容哈希。
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}
	return res.UpsertedCount == 1, nil
}

func (r *MongoSafetyRepository) ByEventID(ctx context.Context, eventID string) (*entity.SafetyEvent, error) {
	return r.findOne(ctx, bson.M{"eventId": eventID})
}

func (r *MongoSafetyRepository) ByRequest(ctx context.Context, driverID, clientRequestID string) (*entity.SafetyEvent, error) {
	return r.findOne(ctx, bson.M{"driverId": driverID, "raw.clientRequestId": clientRequestID})
}

func (r *MongoSafetyRepository) findOne(ctx context.Context, filter bson.M) (*entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var event entity.SafetyEvent
	err := r.db.Collection(colSafetyEvents).FindOne(ctx, filter).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *MongoSafetyRepository) AddSupplement(ctx context.Context, eventID string, sup entity.SafetySupplement) (*entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var event entity.SafetyEvent
	err := r.db.Collection(colSafetyEvents).FindOneAndUpdate(ctx,
		bson.M{"eventId": eventID},
		bson.M{
			"$push": bson.M{"supplements": sup},
			"$set":  bson.M{"updatedAt": sup.At},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// AddAttachment 用 $addToSet 追加，重复绑定同一个文件不产生第二条。
func (r *MongoSafetyRepository) AddAttachment(ctx context.Context, eventID, fileID string) (*entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var event entity.SafetyEvent
	err := r.db.Collection(colSafetyEvents).FindOneAndUpdate(ctx,
		bson.M{"eventId": eventID},
		bson.M{"$addToSet": bson.M{"attachments": fileID}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *MongoSafetyRepository) Pending(ctx context.Context, driverID, state string, limit int) ([]entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	filter := bson.M{}
	if driverID != "" {
		filter["driverId"] = driverID
	}
	if state != "" {
		filter["state"] = state
	}
	cursor, err := r.db.Collection(colSafetyEvents).Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "raw.receivedAt", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]entity.SafetyEvent, 0, limit)
	for cursor.Next(ctx) {
		var event entity.SafetyEvent
		if err := cursor.Decode(&event); err != nil {
			return out, err
		}
		out = append(out, event)
	}
	return out, cursor.Err()
}

// PendingOutbox 只取每个事件里最老的一条待投递意图，避免多条互相阻塞。
func (r *MongoSafetyRepository) PendingOutbox(ctx context.Context, limit int) ([]entity.SafetyOutboxRef, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cursor, err := r.db.Collection(colSafetyEvents).Find(ctx,
		bson.M{"outbox.state": entity.OutboxPending},
		options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]entity.SafetyOutboxRef, 0, limit)
	for cursor.Next(ctx) {
		var event entity.SafetyEvent
		if err := cursor.Decode(&event); err != nil {
			return out, err
		}
		for _, entry := range event.Outbox {
			if entry.State != entity.OutboxPending {
				continue
			}
			out = append(out, entity.SafetyOutboxRef{EventID: event.EventID, Entry: entry})
			break
		}
	}
	return out, cursor.Err()
}

func (r *MongoSafetyRepository) MarkOutboxSent(ctx context.Context, eventID, entryID string, at time.Time) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colSafetyEvents).UpdateOne(ctx,
		bson.M{"eventId": eventID, "outbox.id": entryID},
		bson.M{"$set": bson.M{"outbox.$.state": entity.OutboxSent, "outbox.$.sentAt": at}})
	return err
}

func (r *MongoSafetyRepository) BumpOutboxAttempt(ctx context.Context, eventID, entryID string) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	_, err := r.db.Collection(colSafetyEvents).UpdateOne(ctx,
		bson.M{"eventId": eventID, "outbox.id": entryID},
		bson.M{"$inc": bson.M{"outbox.$.attempts": 1}})
	return err
}

// updateEvent 是处置命令的统一条件更新：filter 带 eventId + version + 状态条件，
// 匹配不到返回 (nil, nil)。所有处置命令与升级都走这一条原子边界。
func (r *MongoSafetyRepository) updateEvent(ctx context.Context, filter bson.M, update bson.M) (*entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	var event entity.SafetyEvent
	err := r.db.Collection(colSafetyEvents).FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&event)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (r *MongoSafetyRepository) Claim(ctx context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	return r.updateEvent(ctx,
		bson.M{"eventId": eventID, "state": entity.SafetyStateReceived, "version": expectedVersion},
		bson.M{
			"$set": bson.M{
				"state": entity.SafetyStateClaimed, "handler": handler,
				"claimedAt": at, "version": expectedVersion + 1, "updatedAt": at,
			},
			"$push": bson.M{"actions": action},
		})
}

func (r *MongoSafetyRepository) Start(ctx context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	return r.updateEvent(ctx,
		bson.M{"eventId": eventID, "state": entity.SafetyStateClaimed, "handler": handler, "version": expectedVersion},
		bson.M{
			"$set":  bson.M{"state": entity.SafetyStateProcessing, "version": expectedVersion + 1, "updatedAt": at},
			"$push": bson.M{"actions": action},
		})
}

// Transfer 转派只换处理人，不改 state、不改 claimedAt（受理/领取事实保留）。
func (r *MongoSafetyRepository) Transfer(ctx context.Context, eventID string, expectedVersion int64, fromHandler, toHandler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	return r.updateEvent(ctx,
		bson.M{"eventId": eventID, "state": bson.M{"$in": []string{entity.SafetyStateClaimed, entity.SafetyStateProcessing}}, "handler": fromHandler, "version": expectedVersion},
		bson.M{
			"$set":  bson.M{"handler": toHandler, "version": expectedVersion + 1, "updatedAt": at},
			"$push": bson.M{"actions": action},
		})
}

func (r *MongoSafetyRepository) AddAction(ctx context.Context, eventID string, expectedVersion int64, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	return r.updateEvent(ctx,
		bson.M{"eventId": eventID, "state": bson.M{"$ne": entity.SafetyStateResolved}, "version": expectedVersion},
		bson.M{
			"$set":  bson.M{"version": expectedVersion + 1, "updatedAt": at},
			"$push": bson.M{"actions": action},
		})
}

func (r *MongoSafetyRepository) Resolve(ctx context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	return r.updateEvent(ctx,
		bson.M{"eventId": eventID, "state": bson.M{"$in": []string{entity.SafetyStateClaimed, entity.SafetyStateProcessing}}, "handler": handler, "version": expectedVersion},
		bson.M{
			"$set":  bson.M{"state": entity.SafetyStateResolved, "version": expectedVersion + 1, "updatedAt": at},
			"$push": bson.M{"actions": action},
		})
}

func (r *MongoSafetyRepository) EscalateCandidates(ctx context.Context, now time.Time, limit int) ([]entity.SafetyEvent, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	cutoff := now.Add(-time.Duration(entity.EscalationAfter) * time.Second)
	cursor, err := r.db.Collection(colSafetyEvents).Find(ctx,
		bson.M{
			"state":             entity.SafetyStateReceived,
			"raw.receivedAt":    bson.M{"$lte": cutoff},
			"escalations.level": bson.M{"$ne": entity.EscalationLevelUnclaimed},
		},
		options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make([]entity.SafetyEvent, 0, limit)
	for cursor.Next(ctx) {
		var event entity.SafetyEvent
		if err := cursor.Decode(&event); err != nil {
			return out, err
		}
		out = append(out, event)
	}
	return out, cursor.Err()
}

// TryEscalate 是升级的权威写入：带 state=RECEIVED 与档位不存在的条件。
// 领取先提交 → state 已变 → 匹配 0 行，升级被跳过；升级先提交则事实保留。
func (r *MongoSafetyRepository) TryEscalate(ctx context.Context, eventID string, escalation entity.SafetyEscalation, outbox entity.SafetyOutbox) (bool, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	res, err := r.db.Collection(colSafetyEvents).UpdateOne(ctx,
		bson.M{
			"eventId":           eventID,
			"state":             entity.SafetyStateReceived,
			"escalations.level": bson.M{"$ne": escalation.Level},
		},
		bson.M{
			"$push": bson.M{"escalations": escalation, "outbox": outbox},
			"$set":  bson.M{"updatedAt": escalation.At},
		})
	if err != nil {
		return false, err
	}
	return res.MatchedCount == 1, nil
}

// LastKnownLocation 读 D05 上报写入的最后位置。无位置时 at 为 nil。
func (r *MongoSafetyRepository) LastKnownLocation(ctx context.Context, driverID string) (float64, float64, *time.Time, error) {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	oid, err := objectID(driverID)
	if err != nil {
		return 0, 0, nil, err
	}
	var doc struct {
		LastLocation struct {
			Coordinates []float64 `bson:"coordinates"`
		} `bson:"lastLocation"`
		LastLocationAt *time.Time `bson:"lastLocationAt"`
	}
	err = r.db.Collection(colDrivers).FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, 0, nil, nil
	}
	if err != nil {
		return 0, 0, nil, err
	}
	if len(doc.LastLocation.Coordinates) != 2 || doc.LastLocationAt == nil {
		return 0, 0, nil, nil
	}
	return doc.LastLocation.Coordinates[0], doc.LastLocation.Coordinates[1], doc.LastLocationAt, nil
}

var (
	_ entity.SafetyRepository     = (*MongoSafetyRepository)(nil)
	_ entity.DriverLocationReader = (*MongoSafetyRepository)(nil)
)
