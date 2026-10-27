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

// MongoCancelQuoteRepository 使用共享 mongox 文档存储持久化取消报价（P12）。
// cancel_quotes 集合 quoteNo 唯一，orderId+expiresAt 索引。
type MongoCancelQuoteRepository struct {
	documents *mongox.DocumentStore[dbmodel.CancelQuotesDocument]
	log       *zap.Logger
}

// NewMongoCancelQuoteRepository 创建取消报价仓储。
func NewMongoCancelQuoteRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoCancelQuoteRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoCancelQuoteRepository{
		documents: mongox.NewDocumentStore[dbmodel.CancelQuotesDocument](db, log),
		log:       log,
	}
}

// Save 按 MongoDB _id 新增或更新取消报价聚合。
func (r *MongoCancelQuoteRepository) Save(ctx context.Context, q *entity.CancelQuote) error {
	start := time.Now()
	_, err := r.documents.UpsertByID(ctx, q.ID, toCancelQuoteDocument(q))
	r.logOperation("Save", q.ID, 0, start, err)
	return err
}

// FindByQuoteNo 按报价单号查询取消报价。
func (r *MongoCancelQuoteRepository) FindByQuoteNo(ctx context.Context, quoteNo string) (*entity.CancelQuote, error) {
	start := time.Now()
	document, err := r.documents.FindOne(ctx, bson.M{"quote_no": quoteNo, "is_deleted": false})
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrCancelQuoteNotFound
	}
	r.logOperation("FindByQuoteNo", quoteNo, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toCancelQuoteDomain(document), nil
}

// FindByOrderLatest 返回某订单最新（expiresAt 最大）的报价。
func (r *MongoCancelQuoteRepository) FindByOrderLatest(ctx context.Context, orderID string) (*entity.CancelQuote, error) {
	start := time.Now()
	document, err := r.documents.FindOne(ctx, bson.M{"order_id": orderID, "is_deleted": false},
		options.FindOne().SetSort(bson.D{{Key: "expires_at", Value: -1}}))
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrCancelQuoteNotFound
	}
	r.logOperation("FindByOrderLatest", orderID, 0, start, err)
	if err != nil {
		return nil, err
	}
	return toCancelQuoteDomain(document), nil
}

func (r *MongoCancelQuoteRepository) logOperation(operation string, id string, total int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "shop_mongo_cancel_quote"),
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

func toCancelQuoteDocument(q *entity.CancelQuote) *dbmodel.CancelQuotesDocument {
	return &dbmodel.CancelQuotesDocument{
		ID:           q.ID,
		OrderID:      q.OrderID,
		QuoteNo:      q.QuoteNo,
		CancelFee:    q.CancelFee,
		BoundVersion: q.BoundVersion,
		ReasonCode:   q.ReasonCode,
		ExpiresAt:    q.ExpiresAt,
		Used:         q.Used,
		CreatedAt:    q.CreatedAt,
		UpdatedAt:    q.UpdatedAt,
		CreatedBy:    q.CreatedBy,
		UpdatedBy:    q.UpdatedBy,
		IsDeleted:    q.IsDeleted,
	}
}

func toCancelQuoteDomain(document *dbmodel.CancelQuotesDocument) *entity.CancelQuote {
	return &entity.CancelQuote{
		ID:           document.ID,
		OrderID:      document.OrderID,
		QuoteNo:      document.QuoteNo,
		CancelFee:    document.CancelFee,
		BoundVersion: document.BoundVersion,
		ReasonCode:   document.ReasonCode,
		ExpiresAt:    document.ExpiresAt,
		Used:         document.Used,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
		CreatedBy:    document.CreatedBy,
		UpdatedBy:    document.UpdatedBy,
		IsDeleted:    document.IsDeleted,
	}
}

var _ entity.CancelQuoteRepository = (*MongoCancelQuoteRepository)(nil)
