package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/shop/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoVirtualNumberRepository 使用共享 mongox 文档存储持久化虚拟号（P09）。
// virtual_numbers 集合 orderId 唯一：一单一号。
type MongoVirtualNumberRepository struct {
	documents *mongox.DocumentStore[dbmodel.VirtualNumbersDocument]
	log       *zap.Logger
}

// NewMongoVirtualNumberRepository 创建虚拟号仓储。
func NewMongoVirtualNumberRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoVirtualNumberRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoVirtualNumberRepository{
		documents: mongox.NewDocumentStore[dbmodel.VirtualNumbersDocument](db, log),
		log:       log,
	}
}

// Save 按 MongoDB _id 新增或更新虚拟号聚合。
func (r *MongoVirtualNumberRepository) Save(ctx context.Context, v *entity.VirtualNumber) error {
	start := time.Now()
	_, err := r.documents.UpsertByID(ctx, v.ID, toVirtualNumberDocument(v))
	r.logOperation("Save", v.ID, 0, start, err)
	return err
}

// FindByOrder 按订单号查询虚拟号。
func (r *MongoVirtualNumberRepository) FindByOrder(ctx context.Context, orderID string) (*entity.VirtualNumber, error) {
	start := time.Now()
	document, err := r.documents.FindOne(ctx, bson.M{"order_id": orderID, "is_deleted": false})
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrVirtualNumberNotFound
	}
	r.logOperation("FindByOrder", orderID, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toVirtualNumberDomain(document), nil
}

func (r *MongoVirtualNumberRepository) logOperation(operation string, id string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_virtual_number"),
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

func toVirtualNumberDocument(v *entity.VirtualNumber) *dbmodel.VirtualNumbersDocument {
	return &dbmodel.VirtualNumbersDocument{
		ID:          v.ID,
		OrderID:     v.OrderID,
		PassengerID: v.PassengerID,
		DriverID:    v.DriverID,
		VirtualNo:   v.VirtualNo,
		Status:      v.Status,
		BoundAt:     v.BoundAt,
		ReleasedAt:  v.ReleasedAt,
		ExpireAt:    v.ExpireAt,
		CreatedAt:   v.CreatedAt,
		UpdatedAt:   v.UpdatedAt,
		CreatedBy:   v.CreatedBy,
		UpdatedBy:   v.UpdatedBy,
		IsDeleted:   v.IsDeleted,
	}
}

func toVirtualNumberDomain(document *dbmodel.VirtualNumbersDocument) *entity.VirtualNumber {
	return &entity.VirtualNumber{
		ID:          document.ID,
		OrderID:     document.OrderID,
		PassengerID: document.PassengerID,
		DriverID:    document.DriverID,
		VirtualNo:   document.VirtualNo,
		Status:      document.Status,
		BoundAt:     document.BoundAt,
		ReleasedAt:  document.ReleasedAt,
		ExpireAt:    document.ExpireAt,
		CreatedAt:   document.CreatedAt,
		UpdatedAt:   document.UpdatedAt,
		CreatedBy:   document.CreatedBy,
		UpdatedBy:   document.UpdatedBy,
		IsDeleted:   document.IsDeleted,
	}
}

var _ entity.VirtualNumberRepository = (*MongoVirtualNumberRepository)(nil)
