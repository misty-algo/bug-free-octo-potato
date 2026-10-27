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

// MongoDestinationChangeRepository 使用共享 mongox 文档存储持久化中途改点（P10）。
// destination_changes 是并发写集合：Save 以 version 乐观锁防并发覆盖，冲突返回领域错误。
type MongoDestinationChangeRepository struct {
	documents *mongox.DocumentStore[dbmodel.DestinationChangesDocument]
	log       *zap.Logger
}

// NewMongoDestinationChangeRepository 创建中途改点仓储。
func NewMongoDestinationChangeRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoDestinationChangeRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoDestinationChangeRepository{
		documents: mongox.NewDocumentStore[dbmodel.DestinationChangesDocument](db, log),
		log:       log,
	}
}

// Save 新增或更新改点申请。携带乐观锁：以 {_id, version} 作为过滤条件整体替换，
// 命中失败且未 upsert 时视为版本冲突（并发修改）。
func (r *MongoDestinationChangeRepository) Save(ctx context.Context, c *entity.DestinationChange) error {
	start := time.Now()
	document := toDestinationChangeDocument(c)
	document.Version = c.Version + 1
	filter := bson.M{"_id": c.ID, "version": c.Version}
	result, err := r.documents.Collection().ReplaceOne(ctx, filter, document,
		options.Replace().SetUpsert(true))
	if err == nil && result != nil && result.MatchedCount == 0 && result.UpsertedCount == 0 {
		err = entity.ErrDestinationChangeVersionConflict
	} else if err == nil {
		c.Version = document.Version
		c.UpdatedAt = document.UpdatedAt
	}
	r.logOperation("Save", c.ID, 0, start, err)
	return err
}

// FindByID 按 MongoDB _id 加载改点申请。
func (r *MongoDestinationChangeRepository) FindByID(ctx context.Context, id string) (*entity.DestinationChange, error) {
	start := time.Now()
	document, err := r.documents.FindByID(ctx, id)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrDestinationChangeNotFound
	}
	r.logOperation("FindByID", id, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toDestinationChangeDomain(document), nil
}

// ListByOrder 按申请时间升序返回某订单全部改点申请。
func (r *MongoDestinationChangeRepository) ListByOrder(ctx context.Context, orderID string) ([]*entity.DestinationChange, error) {
	start := time.Now()
	filter := bson.M{"order_id": orderID, "is_deleted": false}
	documents, err := r.documents.FindMany(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "requested_at", Value: 1}}))
	if err != nil {
		r.logOperation("ListByOrder", orderID, 0, start, err)
		return nil, err
	}
	items := make([]*entity.DestinationChange, 0, len(documents))
	for i := range documents {
		items = append(items, toDestinationChangeDomain(&documents[i]))
	}
	r.logOperation("ListByOrder", orderID, int64(len(items)), start, nil)
	return items, nil
}

// CountAcceptedByOrder 返回某订单已确认（accepted）的改点次数，用于 2 次上限校验。
func (r *MongoDestinationChangeRepository) CountAcceptedByOrder(ctx context.Context, orderID string) (int64, error) {
	start := time.Now()
	filter := bson.M{"order_id": orderID, "status": entity.DestinationChangeAccepted, "is_deleted": false}
	count, err := r.documents.Count(ctx, filter)
	r.logOperation("CountAcceptedByOrder", orderID, count, start, err)
	return count, err
}

func (r *MongoDestinationChangeRepository) logOperation(operation string, id string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_destination_change"),
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

func toDestinationChangeDocument(c *entity.DestinationChange) *dbmodel.DestinationChangesDocument {
	return &dbmodel.DestinationChangesDocument{
		ID:             c.ID,
		OrderID:        c.OrderID,
		NewEndLocation: dbmodel.GeoPoint{Type: c.NewEndLocation.Type, Coordinates: c.NewEndLocation.Coordinates},
		NewEndAddress:  c.NewEndAddress,
		NewEndPoiID:    c.NewEndPoiID,
		NewEstimateFee: c.NewEstimateFee,
		Status:         c.Status,
		RequestedAt:    c.RequestedAt,
		ConfirmedAt:    c.ConfirmedAt,
		ExpiresAt:      c.ExpiresAt,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
		CreatedBy:      c.CreatedBy,
		UpdatedBy:      c.UpdatedBy,
		IsDeleted:      c.IsDeleted,
		Version:        c.Version,
	}
}

func toDestinationChangeDomain(document *dbmodel.DestinationChangesDocument) *entity.DestinationChange {
	return &entity.DestinationChange{
		ID:             document.ID,
		OrderID:        document.OrderID,
		NewEndLocation: entity.GeoPoint{Type: document.NewEndLocation.Type, Coordinates: document.NewEndLocation.Coordinates},
		NewEndAddress:  document.NewEndAddress,
		NewEndPoiID:    document.NewEndPoiID,
		NewEstimateFee: document.NewEstimateFee,
		Status:         document.Status,
		RequestedAt:    document.RequestedAt,
		ConfirmedAt:    document.ConfirmedAt,
		ExpiresAt:      document.ExpiresAt,
		CreatedAt:      document.CreatedAt,
		UpdatedAt:      document.UpdatedAt,
		CreatedBy:      document.CreatedBy,
		UpdatedBy:      document.UpdatedBy,
		IsDeleted:      document.IsDeleted,
		Version:        document.Version,
	}
}

var _ entity.DestinationChangeRepository = (*MongoDestinationChangeRepository)(nil)
