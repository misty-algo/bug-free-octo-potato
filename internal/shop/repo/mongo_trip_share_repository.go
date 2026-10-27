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

// MongoTripShareRepository 使用共享 mongox 文档存储持久化行程分享（P11）。
// trip_shares 集合 shareToken 唯一。
type MongoTripShareRepository struct {
	documents *mongox.DocumentStore[dbmodel.TripSharesDocument]
	log       *zap.Logger
}

// NewMongoTripShareRepository 创建行程分享仓储。
func NewMongoTripShareRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoTripShareRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoTripShareRepository{
		documents: mongox.NewDocumentStore[dbmodel.TripSharesDocument](db, log),
		log:       log,
	}
}

// Save 按 MongoDB _id 新增或更新行程分享聚合。
func (r *MongoTripShareRepository) Save(ctx context.Context, s *entity.TripShare) error {
	start := time.Now()
	_, err := r.documents.UpsertByID(ctx, s.ID, toTripShareDocument(s))
	r.logOperation("Save", s.ID, 0, start, err)
	return err
}

// FindByToken 按分享令牌查询行程分享。
func (r *MongoTripShareRepository) FindByToken(ctx context.Context, token string) (*entity.TripShare, error) {
	start := time.Now()
	document, err := r.documents.FindOne(ctx, bson.M{"share_token": token, "is_deleted": false})
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrTripShareNotFound
	}
	r.logOperation("FindByToken", token, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toTripShareDomain(document), nil
}

func (r *MongoTripShareRepository) logOperation(operation string, id string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_trip_share"),
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

func toTripShareDocument(s *entity.TripShare) *dbmodel.TripSharesDocument {
	return &dbmodel.TripSharesDocument{
		ID:          s.ID,
		OrderID:     s.OrderID,
		ShareToken:  s.ShareToken,
		CreatorID:   s.CreatorID,
		ExpireAt:    s.ExpireAt,
		Revoked:     s.Revoked,
		AccessCount: s.AccessCount,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
		CreatedBy:   s.CreatedBy,
		UpdatedBy:   s.UpdatedBy,
		IsDeleted:   s.IsDeleted,
	}
}

func toTripShareDomain(document *dbmodel.TripSharesDocument) *entity.TripShare {
	return &entity.TripShare{
		ID:          document.ID,
		OrderID:     document.OrderID,
		ShareToken:  document.ShareToken,
		CreatorID:   document.CreatorID,
		ExpireAt:    document.ExpireAt,
		Revoked:     document.Revoked,
		AccessCount: document.AccessCount,
		CreatedAt:   document.CreatedAt,
		UpdatedAt:   document.UpdatedAt,
		CreatedBy:   document.CreatedBy,
		UpdatedBy:   document.UpdatedBy,
		IsDeleted:   document.IsDeleted,
	}
}

var _ entity.TripShareRepository = (*MongoTripShareRepository)(nil)
