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

// D06 新增集合（mongodb.js 未定义轨迹与采集会话，需显式迁移）：
// trip_sessions 采集会话登记与主采集租约，trip_points 轨迹样本。
const (
	tripSessionsCollection = "trip_sessions"
	tripPointsCollection   = "trip_points"
	accountsCollection     = "accounts"
)

type MongoTripRepository struct{ db *mongo.Database }

func NewMongoTripRepository(db *mongo.Database) *MongoTripRepository {
	return &MongoTripRepository{db: db}
}

func (r *MongoTripRepository) TaskByNo(ctx context.Context, taskNo string) (*entity.DispatchTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var task entity.DispatchTask
	err := r.db.Collection(tasksCollection).FindOne(ctx, bson.M{"taskNo": taskNo}).Decode(&task)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("TASK_NOT_FOUND", "task does not exist")
	}
	return &task, mongoArrivalError(err)
}

func (r *MongoTripRepository) TaskByID(ctx context.Context, id bson.ObjectID) (*entity.DispatchTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var task entity.DispatchTask
	err := r.db.Collection(tasksCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&task)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("TASK_NOT_FOUND", "task does not exist")
	}
	return &task, mongoArrivalError(err)
}

func (r *MongoTripRepository) OrderByNo(ctx context.Context, orderNo string) (*entity.RideOrder, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var order entity.RideOrder
	err := r.db.Collection(ordersCollection).FindOne(ctx, bson.M{"orderNo": orderNo}).Decode(&order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("ORDER_NOT_FOUND", "order does not exist")
	}
	return &order, mongoArrivalError(err)
}

func (r *MongoTripRepository) OrdersByIDs(ctx context.Context, ids []bson.ObjectID) ([]entity.RideOrder, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if len(ids) == 0 {
		return nil, nil
	}
	cursor, err := r.db.Collection(ordersCollection).Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, mongoArrivalError(err)
	}
	var rows []entity.RideOrder
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, mongoArrivalError(err)
	}
	return rows, nil
}

func (r *MongoTripRepository) FindCommand(ctx context.Context, key entity.CommandKey) (*entity.CommandResult, error) {
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

// PassengerPhoneTail 只把手机号尾号四位交给行程域，不返回完整号码。
func (r *MongoTripRepository) PassengerPhoneTail(ctx context.Context, passengerID bson.ObjectID) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var account struct {
		Phone string `bson:"phone"`
	}
	err := r.db.Collection(accountsCollection).FindOne(ctx, bson.M{"_id": passengerID}).Decode(&account)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", nil
	}
	if err != nil {
		return "", mongoArrivalError(err)
	}
	if len(account.Phone) < entity.VerifyCodeLength {
		return "", nil
	}
	return account.Phone[len(account.Phone)-entity.VerifyCodeLength:], nil
}

// SaveVerify 只原子更新订单文档：计数与锁定在同一个文档内完成，
// 因此不需要事务，也不会留下需要回滚的中间授权票据。
func (r *MongoTripRepository) SaveVerify(ctx context.Context, m entity.VerifyMutation) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	update := bson.M{
		"$set": bson.M{
			"version":           m.Order.Version,
			"verified":          m.Verified,
			"verifyFailures":    m.Failures,
			"verifyLockedUntil": m.LockedUntil,
			"updatedAt":         time.Now().UTC(),
		},
	}
	if m.Timeline.Name != "" {
		update["$push"] = bson.M{"timeline": m.Timeline}
	}
	result, err := r.db.Collection(ordersCollection).UpdateOne(ctx,
		bson.M{"_id": m.Order.ID, "version": m.Order.Version - 1}, update)
	if err != nil {
		return mongoArrivalError(err)
	}
	if result.MatchedCount != 1 {
		return apperrors.Conflict("VERSION_CONFLICT", "order version changed")
	}
	return r.appendFacts(ctx, m.Events, m.Command)
}

// SaveStart 推进订单到 IN_TRIP 并记录发车时间。任务只在首次发车时迁移，
// 保证共同出发幂等。
func (r *MongoTripRepository) SaveStart(ctx context.Context, m entity.StartMutation) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, order := range m.Orders {
		result, err := r.db.Collection(ordersCollection).UpdateOne(ctx,
			bson.M{"_id": order.ID, "version": order.Version - 1, "state": entity.OrderArrived},
			bson.M{"$set": bson.M{
				"state":     entity.OrderInTrip,
				"version":   order.Version,
				"boardedAt": m.DepartedAt,
				"startedAt": m.DepartedAt,
				"updatedAt": m.DepartedAt,
			}})
		if err != nil {
			return mongoArrivalError(err)
		}
		if result.MatchedCount != 1 {
			return apperrors.Conflict("VERSION_CONFLICT", "order version changed")
		}
	}
	if _, err := r.db.Collection(tasksCollection).UpdateOne(ctx,
		bson.M{"taskNo": m.TaskNo, "departedAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"state": entity.TaskActive, "departedAt": m.DepartedAt}}); err != nil {
		return mongoArrivalError(err)
	}
	if _, err := r.db.Collection(tripSessionsCollection).InsertOne(ctx, m.Session); err != nil && !mongo.IsDuplicateKeyError(err) {
		return mongoArrivalError(err)
	}
	return r.appendFacts(ctx, m.Events, m.Command)
}

// SaveEnd 写 ENDED、结束事件与计费请求。运力只在最后一个活动子单终结时释放一次。
func (r *MongoTripRepository) SaveEnd(ctx context.Context, m entity.EndMutation) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := r.db.Collection(ordersCollection).UpdateOne(ctx,
		bson.M{"_id": m.Order.ID, "version": m.Order.Version - 1, "state": entity.OrderInTrip},
		bson.M{
			"$set": bson.M{
				"state":             entity.OrderEnded,
				"version":           m.Order.Version,
				"endedAt":           m.EndedAt,
				"actualDist":        m.ActualDist,
				"actualDur":         m.ActualDur,
				"billingRequestId":  m.BillingRequestID,
				"billingInputState": m.InputState,
				"updatedAt":         m.EndedAt,
			},
			"$push": bson.M{"timeline": m.Timeline},
		})
	if err != nil {
		return mongoArrivalError(err)
	}
	if result.MatchedCount != 1 {
		return apperrors.Conflict("VERSION_CONFLICT", "order version changed")
	}
	if m.ReleaseDriver {
		if _, err := r.db.Collection(tasksCollection).UpdateOne(ctx,
			bson.M{"_id": m.Order.DispatchID, "state": bson.M{"$ne": entity.TaskFinished}},
			bson.M{"$set": bson.M{"state": entity.TaskFinished}}); err != nil {
			return mongoArrivalError(err)
		}
		// 运力释放一次；支付、轨迹补核算不重新占用司机。
		// 不合格时收车（suspended），不自动恢复空闲，由司机重新出车。
		status := "idle"
		if m.DriverBlocked {
			status = "suspended"
		}
		if _, err := r.db.Collection(driversCollection).UpdateOne(ctx,
			bson.M{"_id": m.DriverID},
			bson.M{"$set": bson.M{"status": status, "updatedAt": m.EndedAt}}); err != nil {
			return mongoArrivalError(err)
		}
	}
	return r.appendFacts(ctx, m.Events, m.Command)
}

// SaveSample 以 taskNo+sessionId+seq 唯一去重：重复序号不重复累计里程，
// 只有比会话最新序号更新的点才允许刷新司机最新位置。
func (r *MongoTripRepository) SaveSample(ctx context.Context, m entity.SampleMutation) (entity.TripSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	sessions := r.db.Collection(tripSessionsCollection)
	filter := bson.M{"taskNo": m.TaskNo, "sessionId": m.SessionID}
	now := m.LatestAt
	if _, err := sessions.UpdateOne(ctx, filter, bson.M{
		"$setOnInsert": bson.M{"createdAt": now, "driverId": m.DriverID, "state": entity.SessionActive},
		"$set":         bson.M{"updatedAt": now},
	}, options.UpdateOne().SetUpsert(true)); err != nil {
		return entity.TripSession{}, mongoArrivalError(err)
	}
	_, err := r.db.Collection(tripPointsCollection).InsertOne(ctx, m.Point)
	duplicated := mongo.IsDuplicateKeyError(err)
	if err != nil && !duplicated {
		return entity.TripSession{}, mongoArrivalError(err)
	}
	update := bson.M{"$set": bson.M{"updatedAt": now}}
	if m.UpdateLatest {
		update["$max"] = bson.M{"lastSeq": m.Point.Seq}
	}
	if _, err := sessions.UpdateOne(ctx, filter, update); err != nil {
		return entity.TripSession{}, mongoArrivalError(err)
	}
	// 水位只随连续序号推进，跳点保留为待补传缺口。
	var session entity.TripSession
	if err := sessions.FindOne(ctx, filter).Decode(&session); err != nil {
		return entity.TripSession{}, mongoArrivalError(err)
	}
	if m.UpdateLatest && m.Point.Seq == session.Watermark+1 {
		if _, err := sessions.UpdateOne(ctx, filter, bson.M{"$set": bson.M{"watermark": m.Point.Seq}}); err != nil {
			return entity.TripSession{}, mongoArrivalError(err)
		}
		session.Watermark = m.Point.Seq
	}
	return session, nil
}

func (r *MongoTripRepository) Session(ctx context.Context, taskNo, sessionID string) (*entity.TripSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var session entity.TripSession
	err := r.db.Collection(tripSessionsCollection).FindOne(ctx, bson.M{"taskNo": taskNo, "sessionId": sessionID}).Decode(&session)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &session, mongoArrivalError(err)
}

// appendFacts 写事件与命令记录：事件按 eventId 去重，命令键冲突即为重复提交。
func (r *MongoTripRepository) appendFacts(ctx context.Context, events []entity.EventLog, command entity.CommandResult) error {
	for _, event := range events {
		if _, err := r.db.Collection(eventLogsCollection).InsertOne(ctx, event); err != nil && !mongo.IsDuplicateKeyError(err) {
			return mongoArrivalError(err)
		}
	}
	if _, err := r.db.Collection(commandCollection).InsertOne(ctx, command); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperrors.Conflict("IDEMPOTENCY_CONFLICT", "the command key was already applied")
		}
		return mongoArrivalError(err)
	}
	return nil
}

var _ entity.TripRepository = (*MongoTripRepository)(nil)
