package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/shop/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoTripRepository 使用共享 mongox 文档存储持久化乘客行程聚合。
// trips 集合以行程号为 _id；状态流转存在并发（司机接单 vs 乘客取消），
// Save 走 version 乐观锁，版本冲突返回 ErrTripVersionConflict。
type MongoTripRepository struct {
	documents *mongox.DocumentStore[dbmodel.RideDocument]
	log       *zap.Logger
}

// NewMongoTripRepository 创建行程仓储。
func NewMongoTripRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoTripRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoTripRepository{
		documents: mongox.NewDocumentStore[dbmodel.RideDocument](db, log),
		log:       log,
	}
}

// Save 条件替换写入行程：_id 与 version 同时匹配才写入，并把新版本回写聚合。
func (r *MongoTripRepository) Save(ctx context.Context, t *entity.Trip) error {
	start := time.Now()
	document := toRideDocument(t)
	document.Version = t.Version + 1
	filter := bson.M{"_id": t.ID, "version": t.Version}
	result, err := r.documents.Collection().ReplaceOne(ctx, filter, document,
		options.Replace().SetUpsert(true))
	switch {
	case err != nil:
		// 保持原样返回底层错误
	case result != nil && result.MatchedCount == 0 && result.UpsertedCount == 0:
		err = entity.ErrTripVersionConflict
	default:
		t.Version = document.Version
	}
	r.logOperation("Save", t.ID, 0, start, err)
	return err
}

// FindByID 按行程号查询。
func (r *MongoTripRepository) FindByID(ctx context.Context, id string) (*entity.Trip, error) {
	start := time.Now()
	document, err := r.documents.FindByID(ctx, id)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrTripNotFound
	}
	r.logOperation("FindByID", id, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toRideDomain(document), nil
}

// ListByPassenger 按创建时间倒序返回乘客的行程，最多 limit 条（limit<=0 返回全部）。
func (r *MongoTripRepository) ListByPassenger(ctx context.Context, passengerID string, limit int) ([]*entity.Trip, error) {
	start := time.Now()
	opt := options.Find().SetSort(bson.M{"created_at": -1})
	if limit > 0 {
		opt = opt.SetLimit(int64(limit))
	}
	documents, err := r.documents.FindMany(ctx, bson.M{"passenger_id": passengerID}, opt)
	r.logOperation("ListByPassenger", passengerID, int64(len(documents)), start, err)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.Trip, 0, len(documents))
	for i := range documents {
		items = append(items, toRideDomain(&documents[i]))
	}
	return items, nil
}

// ListPendingBillTrips 查询已结束但尚未建账的行程（建账补偿扫描用）。
// 按创建时间升序，保证老单先补。
func (r *MongoTripRepository) ListPendingBillTrips(ctx context.Context, limit int) ([]*entity.Trip, error) {
	start := time.Now()
	filter := bson.M{"status": entity.TripStatusEnded, "bill_created": false}
	opt := options.Find().SetSort(bson.M{"created_at": 1})
	if limit > 0 {
		opt = opt.SetLimit(int64(limit))
	}
	documents, err := r.documents.FindMany(ctx, filter, opt)
	r.logOperation("ListPendingBillTrips", "ended-unbilled", int64(len(documents)), start, err)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.Trip, 0, len(documents))
	for i := range documents {
		items = append(items, toRideDomain(&documents[i]))
	}
	return items, nil
}

func (r *MongoTripRepository) logOperation(operation string, id string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_trip"),
		zap.String("operation", operation),
		zap.String("aggregate_id", id),
		zap.Int64("total", total),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("mongodb repository operation completed with error", fields...)
		return
	}
	r.log.Info("mongodb repository operation completed", fields...)
}

func toRideDocument(t *entity.Trip) *dbmodel.RideDocument {
	return &dbmodel.RideDocument{
		ID:               t.ID,
		PassengerID:      t.PassengerID,
		DriverID:         t.DriverID,
		Status:           t.Status,
		Origin:           dbmodel.GeoPoint{Type: t.Origin.Type, Coordinates: t.Origin.Coordinates},
		Destination:      dbmodel.GeoPoint{Type: t.Destination.Type, Coordinates: t.Destination.Coordinates},
		DistanceM:        t.DistanceM,
		DurationSec:      t.DurationSec,
		WaitSec:          t.WaitSec,
		StartedAt:        t.StartedAt,
		EndedAt:          t.EndedAt,
		BillingRequestID: t.BillingRequestID,
		PassengerPaid:    t.PassengerPaid,
		CommissionRateBP: t.CommissionRateBP,
		BillCreated:      t.BillCreated,
		Version:          t.Version,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}

func toRideDomain(document *dbmodel.RideDocument) *entity.Trip {
	return &entity.Trip{
		ID:               document.ID,
		PassengerID:      document.PassengerID,
		DriverID:         document.DriverID,
		Status:           document.Status,
		Origin:           entity.GeoPoint{Type: document.Origin.Type, Coordinates: document.Origin.Coordinates},
		Destination:      entity.GeoPoint{Type: document.Destination.Type, Coordinates: document.Destination.Coordinates},
		DistanceM:        document.DistanceM,
		DurationSec:      document.DurationSec,
		WaitSec:          document.WaitSec,
		StartedAt:        document.StartedAt,
		EndedAt:          document.EndedAt,
		BillingRequestID: document.BillingRequestID,
		PassengerPaid:    document.PassengerPaid,
		CommissionRateBP: document.CommissionRateBP,
		BillCreated:      document.BillCreated,
		Version:          document.Version,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}
}

var _ entity.TripRepository = (*MongoTripRepository)(nil)
