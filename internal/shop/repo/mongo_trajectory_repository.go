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

// MongoTrajectoryRepository 使用共享 mongox 文档存储持久化实时轨迹点（P08）。
// trajectories 为 append-only 集合，不做覆盖更新。
type MongoTrajectoryRepository struct {
	documents *mongox.DocumentStore[dbmodel.TrajectoriesDocument]
	log       *zap.Logger
}

// NewMongoTrajectoryRepository 创建实时轨迹仓储。
func NewMongoTrajectoryRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoTrajectoryRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoTrajectoryRepository{
		documents: mongox.NewDocumentStore[dbmodel.TrajectoriesDocument](db, log),
		log:       log,
	}
}

// Append 追加一个轨迹上报点。
func (r *MongoTrajectoryRepository) Append(ctx context.Context, point *entity.TrajectoryPoint) error {
	start := time.Now()
	_, err := r.documents.Insert(ctx, toTrajectoryDocument(point))
	r.logOperation("Append", point.OrderID, 0, start, err)
	return err
}

// ListByOrder 按上报时间升序返回某订单最近 limit 个轨迹点。
func (r *MongoTrajectoryRepository) ListByOrder(ctx context.Context, orderID string, limit int) ([]*entity.TrajectoryPoint, error) {
	start := time.Now()
	filter := bson.M{"order_id": orderID, "is_deleted": false}
	opts := options.Find().SetSort(bson.D{{Key: "reported_at", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	documents, err := r.documents.FindMany(ctx, filter, opts)
	if err != nil {
		r.logOperation("ListByOrder", orderID, 0, start, err)
		return nil, err
	}
	points := make([]*entity.TrajectoryPoint, 0, len(documents))
	for i := range documents {
		points = append(points, toTrajectoryDomain(&documents[i]))
	}
	r.logOperation("ListByOrder", orderID, int64(len(points)), start, nil)
	return points, nil
}

// LatestByOrder 返回某订单最新的轨迹点，用于相邻点质量校验。
func (r *MongoTrajectoryRepository) LatestByOrder(ctx context.Context, orderID string) (*entity.TrajectoryPoint, error) {
	start := time.Now()
	document, err := r.documents.FindOne(ctx, bson.M{"order_id": orderID, "is_deleted": false},
		options.FindOne().SetSort(bson.D{{Key: "reported_at", Value: -1}}))
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrTrajectoryNotFound
	}
	r.logOperation("LatestByOrder", orderID, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toTrajectoryDomain(document), nil
}

func (r *MongoTrajectoryRepository) logOperation(operation string, orderID string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_trajectory"),
		zap.String("operation", operation),
		zap.String("order_id", orderID),
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

func toTrajectoryDocument(point *entity.TrajectoryPoint) *dbmodel.TrajectoriesDocument {
	return &dbmodel.TrajectoriesDocument{
		ID:          point.ID,
		OrderID:     point.OrderID,
		DriverID:    point.DriverID,
		Location:    dbmodel.GeoPoint{Type: point.Location.Type, Coordinates: point.Location.Coordinates},
		Speed:       point.Speed,
		Bearing:     point.Bearing,
		Accuracy:    point.Accuracy,
		Mileage:     point.Mileage,
		PointType:   point.PointType,
		ReportedAt:  point.ReportedAt,
		CollectedAt: point.CollectedAt,
		Source:      point.Source,
		CreatedAt:   point.CollectedAt,
		UpdatedAt:   point.CollectedAt,
	}
}

func toTrajectoryDomain(document *dbmodel.TrajectoriesDocument) *entity.TrajectoryPoint {
	return &entity.TrajectoryPoint{
		ID:          document.ID,
		OrderID:     document.OrderID,
		DriverID:    document.DriverID,
		Location:    entity.GeoPoint{Type: document.Location.Type, Coordinates: document.Location.Coordinates},
		Speed:       document.Speed,
		Bearing:     document.Bearing,
		Accuracy:    document.Accuracy,
		Mileage:     document.Mileage,
		PointType:   document.PointType,
		ReportedAt:  document.ReportedAt,
		CollectedAt: document.CollectedAt,
		Source:      document.Source,
	}
}

var _ entity.TrajectoryRepository = (*MongoTrajectoryRepository)(nil)
