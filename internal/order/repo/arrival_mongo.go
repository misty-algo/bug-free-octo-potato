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

// Collection names from 《花小猪打车平台-数据库-mongodb.js》.
const (
	tasksCollection     = "dispatch_tasks"
	ordersCollection    = "orders"
	driversCollection   = "drivers"
	poisCollection      = "pois"
	eventLogsCollection = "event_logs"
	commandCollection   = "command_results"
)

// MongoArrivalRepository persists arrival commands without multi-document
// transactions: the deployed MongoDB is standalone, so every write is a
// single-document atomic update guarded by the document version and the
// command is made convergent by the idempotent command record.
type MongoArrivalRepository struct{ db *mongo.Database }

func NewMongoArrivalRepository(db *mongo.Database) *MongoArrivalRepository {
	return &MongoArrivalRepository{db: db}
}

func mongoArrivalError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := apperrors.As(err); ok {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return apperrors.Wrap(apperrors.KindDeadline, "DEADLINE_EXCEEDED", "database deadline exceeded; retry the same command key", err)
	}
	if mongo.IsDuplicateKeyError(err) {
		return apperrors.Wrap(apperrors.KindConflict, "IDEMPOTENCY_CONFLICT", "unique business fact already exists", err)
	}
	return apperrors.Wrap(apperrors.KindUnavailable, "TEMPORARILY_UNAVAILABLE", "database operation failed; retry the same command key", err)
}

func (r *MongoArrivalRepository) TaskByNo(ctx context.Context, taskNo string) (*entity.DispatchTask, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var task entity.DispatchTask
	err := r.db.Collection(tasksCollection).FindOne(ctx, bson.M{"taskNo": taskNo}).Decode(&task)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, apperrors.NotFound("TASK_NOT_FOUND", "task does not exist")
	}
	return &task, mongoArrivalError(err)
}

func (r *MongoArrivalRepository) OrdersByIDs(ctx context.Context, ids []bson.ObjectID) ([]entity.RideOrder, error) {
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

func (r *MongoArrivalRepository) DriverByID(ctx context.Context, id bson.ObjectID) (*entity.Driver, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var driver entity.Driver
	err := r.db.Collection(driversCollection).FindOne(ctx, bson.M{"_id": id}).Decode(&driver)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	return &driver, mongoArrivalError(err)
}

// Pickup resolves the boarding coordinate from the embedded GeoJSON point,
// falling back to the referenced POI (orders.startId → pois._id).
func (r *MongoArrivalRepository) Pickup(ctx context.Context, order *entity.RideOrder) (entity.Coordinate, error) {
	if coordinate, ok := order.StartLocation.Coordinate(); ok {
		return coordinate, nil
	}
	if order.StartID == bson.NilObjectID {
		return entity.Coordinate{}, apperrors.InvalidArgument("INVALID_LOCATION", "order has no boarding coordinate")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var poi struct {
		Loc *entity.GeoPoint `bson:"loc"`
	}
	err := r.db.Collection(poisCollection).FindOne(ctx, bson.M{"_id": order.StartID}).Decode(&poi)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return entity.Coordinate{}, apperrors.NotFound("POI_NOT_FOUND", "boarding POI does not exist")
	}
	if err != nil {
		return entity.Coordinate{}, mongoArrivalError(err)
	}
	coordinate, ok := poi.Loc.Coordinate()
	if !ok {
		return entity.Coordinate{}, apperrors.InvalidArgument("INVALID_LOCATION", "boarding POI coordinate is unusable")
	}
	return coordinate, nil
}

func (r *MongoArrivalRepository) FindCommand(ctx context.Context, key entity.CommandKey) (*entity.CommandResult, error) {
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

// RecordPosition stores the latest driver position on the driver document.
func (r *MongoArrivalRepository) RecordPosition(ctx context.Context, driverID bson.ObjectID, c entity.Coordinate, at time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	result, err := r.db.Collection(driversCollection).UpdateOne(ctx, bson.M{"_id": driverID}, bson.M{"$set": bson.M{
		"lastLocation":   entity.GeoPoint{Type: "Point", Coordinates: []float64{c.Longitude, c.Latitude}},
		"lastLocationAt": at,
		"updatedAt":      at,
	}})
	if err != nil {
		return mongoArrivalError(err)
	}
	if result.MatchedCount != 1 {
		return apperrors.NotFound("DRIVER_NOT_FOUND", "driver does not exist")
	}
	return nil
}

// SaveArrival applies the arrival as a sequence of single-document atomic
// updates. Order updates are version guarded, the task transition is protected
// by the absence of arrivedAt, and the command record makes a replayed key
// answer with the original fact instead of applying the command twice.
func (r *MongoArrivalRepository) SaveArrival(ctx context.Context, m entity.ArrivalMutation) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for _, order := range m.Orders {
		result, err := r.db.Collection(ordersCollection).UpdateOne(ctx,
			bson.M{"_id": order.ID, "version": order.Version - 1, "state": entity.OrderAccepted},
			bson.M{
				"$set":  bson.M{"state": entity.OrderArrived, "version": order.Version, "arrivedAt": m.ArrivedAt, "updatedAt": m.ArrivedAt},
				"$push": bson.M{"timeline": m.Timeline},
			})
		if err != nil {
			return mongoArrivalError(err)
		}
		if result.MatchedCount != 1 {
			return apperrors.Conflict("VERSION_CONFLICT", "order version changed")
		}
	}
	// 任务只在首次到达时迁移；已到达时保持幂等，不重置等待计时。
	if _, err := r.db.Collection(tasksCollection).UpdateOne(ctx,
		bson.M{"taskNo": m.TaskNo, "arrivedAt": bson.M{"$exists": false}},
		bson.M{"$set": bson.M{"state": entity.TaskActive, "arrivedAt": m.ArrivedAt}}); err != nil {
		return mongoArrivalError(err)
	}
	for _, event := range m.Events {
		if _, err := r.db.Collection(eventLogsCollection).InsertOne(ctx, event); err != nil && !mongo.IsDuplicateKeyError(err) {
			return mongoArrivalError(err)
		}
	}
	if _, err := r.db.Collection(commandCollection).InsertOne(ctx, m.Command); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return apperrors.Conflict("IDEMPOTENCY_CONFLICT", "the command key was already applied")
		}
		return mongoArrivalError(err)
	}
	return nil
}

var _ entity.ArrivalRepository = (*MongoArrivalRepository)(nil)
