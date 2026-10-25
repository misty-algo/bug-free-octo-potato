package repo

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// contactIntentsCollection 是 AuthorizeContactCommand 持久化的唯一意图集合，
// uk(orderNo, requestNo) 保证同一业务请求号只产生一条意图。
const contactIntentsCollection = "contact_intents"

type MongoContactRepository struct{ db *mongo.Database }

func NewMongoContactRepository(db *mongo.Database) *MongoContactRepository {
	return &MongoContactRepository{db: db}
}

func (r *MongoContactRepository) OrderByNo(ctx context.Context, orderNo string) (*entity.RideOrder, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var order entity.RideOrder
	err := r.db.Collection(ordersCollection).FindOne(ctx, bson.M{"orderNo": orderNo}).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("ORDER_NOT_FOUND", "order does not exist")
	}
	return &order, mongoArrivalError(err)
}

func (r *MongoContactRepository) TaskByID(ctx context.Context, id bson.ObjectID) (*entity.DispatchTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var task entity.DispatchTask
	err := r.db.Collection(tasksCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&task)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("TASK_NOT_FOUND", "task does not exist")
	}
	return &task, mongoArrivalError(err)
}

func (r *MongoContactRepository) FindCommand(ctx context.Context, key entity.CommandKey) (*entity.CommandResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var v entity.CommandResult
	err := r.db.Collection(commandCollection).FindOne(ctx, bson.M{
		"actor_id": key.ActorID, "command_type": key.CommandType,
		"resource_id": key.ResourceID, "idempotency_key": key.IdempotencyKey,
	}).Decode(&v)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &v, mongoArrivalError(err)
}

// SaveContact 先落唯一意图，再对订单文档做 version 守卫的原子更新，
// 最后落幂等命令记录。顺序保证授权判定与意图持久化在同一边界内。
func (r *MongoContactRepository) SaveContact(ctx context.Context, m entity.ContactMutation) (entity.ContactIntent, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := r.db.Collection(contactIntentsCollection).InsertOne(ctx, m.Intent)
	if err != nil {
		if !mongo.IsDuplicateKeyError(err) {
			return entity.ContactIntent{}, false, mongoArrivalError(err)
		}
		var existing entity.ContactIntent
		derr := r.db.Collection(contactIntentsCollection).FindOne(ctx, bson.M{"orderNo": m.Intent.OrderNo, "requestNo": m.Intent.RequestNo}).Decode(&existing)
		if derr != nil {
			return entity.ContactIntent{}, true, apperrors.Conflict("IDEMPOTENCY_CONFLICT", "the contact intent was already applied")
		}
		return existing, true, nil
	}
	set := bson.M{"version": m.Order.Version, "updatedAt": time.Now().UTC()}
	update := bson.M{"$set": set}
	filter := bson.M{"_id": m.Order.ID, "version": m.Order.Version - 1}
	if m.Message != nil {
		update["$push"] = bson.M{"messages": *m.Message}
		// 同一序号只允许落一次，避免并发或重试产生重复消息。
		filter["messages.seq"] = bson.M{"$ne": m.Message.Seq}
	}
	if m.VirtualCall != nil {
		set["virtualCall"] = *m.VirtualCall
	}
	result, err := r.db.Collection(ordersCollection).UpdateOne(ctx, filter, update)
	if err != nil {
		return entity.ContactIntent{}, false, mongoArrivalError(err)
	}
	if result.MatchedCount != 1 {
		return entity.ContactIntent{}, false, apperrors.Conflict("VERSION_CONFLICT", "order version changed")
	}
	for _, event := range m.Events {
		if _, err := r.db.Collection(eventLogsCollection).InsertOne(ctx, event); err != nil && !mongo.IsDuplicateKeyError(err) {
			return entity.ContactIntent{}, false, mongoArrivalError(err)
		}
	}
	if _, err := r.db.Collection(commandCollection).InsertOne(ctx, m.Command); err != nil && !mongo.IsDuplicateKeyError(err) {
		return entity.ContactIntent{}, false, mongoArrivalError(err)
	}
	return m.Intent, false, nil
}

// SetMessageState 供通讯域回写 STORED。
func (r *MongoContactRepository) SetMessageState(ctx context.Context, orderNo string, seq int64, state string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := r.db.Collection(ordersCollection).UpdateOne(ctx, bson.M{"orderNo": orderNo},
		bson.M{"$set": bson.M{"messages.$[m].state": state}},
		options.UpdateOne().SetArrayFilters([]any{bson.M{"m.seq": seq}}))
	return mongoArrivalError(err)
}

// MarkDelivered 标记对端已拉取；这不是已读回执。
func (r *MongoContactRepository) MarkDelivered(ctx context.Context, orderNo, from string, upToSeq int64) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, err := r.db.Collection(ordersCollection).UpdateOne(ctx, bson.M{"orderNo": orderNo},
		bson.M{"$set": bson.M{"messages.$[m].state": entity.MessageDelivered}},
		options.UpdateOne().SetArrayFilters([]any{bson.M{
			"m.from":  from,
			"m.seq":   bson.M{"$lte": upToSeq},
			"m.state": bson.M{"$in": []string{entity.MessageAccepted, entity.MessageStored}},
		}}))
	return mongoArrivalError(err)
}

var _ entity.ContactRepository = (*MongoContactRepository)(nil)
