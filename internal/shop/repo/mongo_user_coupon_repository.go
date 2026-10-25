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

// MongoUserCouponRepository 使用共享 mongox 文档存储持久化用户券（P14）。
// user_coupons 是并发写集合：Save 携带 version 乐观锁。
type MongoUserCouponRepository struct {
	documents *mongox.DocumentStore[dbmodel.UserCouponsDocument]
	log       *zap.Logger
}

// NewMongoUserCouponRepository 创建用户券仓储。
func NewMongoUserCouponRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoUserCouponRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoUserCouponRepository{
		documents: mongox.NewDocumentStore[dbmodel.UserCouponsDocument](db, log),
		log:       log,
	}
}

// Save 新增或更新用户券；乐观锁冲突返回 ErrUserCouponVersionConflict。
func (r *MongoUserCouponRepository) Save(ctx context.Context, c *entity.UserCoupon) error {
	start := time.Now()
	document := toUserCouponDocument(c)
	document.Version = c.Version + 1
	filter := bson.M{"_id": c.ID, "version": c.Version}
	result, err := r.documents.Collection().ReplaceOne(ctx, filter, &document,
		options.Replace().SetUpsert(true))
	if err == nil && result != nil && result.MatchedCount == 0 && result.UpsertedCount == 0 {
		err = entity.ErrUserCouponVersionConflict
	} else if err == nil {
		c.Version = document.Version
		c.UpdatedAt = document.UpdatedAt
	}
	r.logOperation("Save", c.CouponNo, 0, start, err)
	return err
}

// FindByCouponNo 按券码查询。
func (r *MongoUserCouponRepository) FindByCouponNo(ctx context.Context, couponNo string) (*entity.UserCoupon, error) {
	start := time.Now()
	doc, err := r.documents.FindOne(ctx, bson.M{"coupon_no": couponNo, "is_deleted": false})
	if err != nil {
		r.logOperation("FindByCouponNo", couponNo, 0, start, err)
		return nil, mapCouponErr(err)
	}
	r.logOperation("FindByCouponNo", couponNo, 1, start, nil)
	return toUserCouponDomain(doc), nil
}

// FindByID 按 _id 查询。
func (r *MongoUserCouponRepository) FindByID(ctx context.Context, id string) (*entity.UserCoupon, error) {
	start := time.Now()
	doc, err := r.documents.FindByID(ctx, id)
	if err != nil {
		r.logOperation("FindByID", id, 0, start, err)
		return nil, mapCouponErr(err)
	}
	r.logOperation("FindByID", id, 1, start, nil)
	return toUserCouponDomain(doc), nil
}

// FindByLockOrder 按锁单订单号查询。
func (r *MongoUserCouponRepository) FindByLockOrder(ctx context.Context, orderID string) (*entity.UserCoupon, error) {
	start := time.Now()
	doc, err := r.documents.FindOne(ctx, bson.M{"lock_order_id": orderID, "is_deleted": false})
	if err != nil {
		r.logOperation("FindByLockOrder", orderID, 0, start, err)
		return nil, mapCouponErr(err)
	}
	r.logOperation("FindByLockOrder", orderID, 1, start, nil)
	return toUserCouponDomain(doc), nil
}

// ListByUser 按用户查询券列表。
func (r *MongoUserCouponRepository) ListByUser(ctx context.Context, userID string, status string, limit int) ([]*entity.UserCoupon, error) {
	start := time.Now()
	filter := bson.M{"user_id": userID, "is_deleted": false}
	if status != "" {
		filter["status"] = status
	}
	opts := options.Find().SetSort(bson.D{{Key: "valid_end", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	documents, err := r.documents.FindMany(ctx, filter, opts)
	if err != nil {
		r.logOperation("ListByUser", userID, 0, start, err)
		return nil, err
	}
	coupons := make([]*entity.UserCoupon, 0, len(documents))
	for i := range documents {
		coupons = append(coupons, toUserCouponDomain(&documents[i]))
	}
	r.logOperation("ListByUser", userID, int64(len(coupons)), start, nil)
	return coupons, nil
}

// ListExpiredDue 返回已过有效期且仍为 AVAILABLE/LOCKED 的券。
func (r *MongoUserCouponRepository) ListExpiredDue(ctx context.Context, now time.Time, limit int) ([]*entity.UserCoupon, error) {
	start := time.Now()
	filter := bson.M{
		"is_deleted": false,
		"valid_end":  bson.M{"$lt": now},
		"status":     bson.M{"$in": []string{entity.CouponStatusAvailable, entity.CouponStatusLocked}},
	}
	opts := options.Find().SetSort(bson.D{{Key: "valid_end", Value: 1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	documents, err := r.documents.FindMany(ctx, filter, opts)
	if err != nil {
		r.logOperation("ListExpiredDue", "expired-scan", 0, start, err)
		return nil, err
	}
	coupons := make([]*entity.UserCoupon, 0, len(documents))
	for i := range documents {
		coupons = append(coupons, toUserCouponDomain(&documents[i]))
	}
	r.logOperation("ListExpiredDue", "expired-scan", int64(len(coupons)), start, nil)
	return coupons, nil
}

// CountUsableByUser 返回某用户可用券数。
func (r *MongoUserCouponRepository) CountUsableByUser(ctx context.Context, userID string) (int64, error) {
	start := time.Now()
	filter := bson.M{
		"user_id":     userID,
		"is_deleted":  false,
		"status":      entity.CouponStatusAvailable,
		"valid_start": bson.M{"$lte": time.Now()},
		"valid_end":   bson.M{"$gt": time.Now()},
	}
	count, err := r.documents.Count(ctx, filter)
	if err != nil {
		r.logOperation("CountUsableByUser", userID, 0, start, err)
		return 0, err
	}
	r.logOperation("CountUsableByUser", userID, count, start, nil)
	return count, nil
}

func (r *MongoUserCouponRepository) logOperation(op string, key string, affected int64, start time.Time, err error) {
	if err != nil {
		r.log.Warn("user_coupon repo op failed", zap.String("op", op), zap.String("key", key), zap.Error(err))
		return
	}
	r.log.Debug("user_coupon repo op", zap.String("op", op), zap.String("key", key), zap.Int64("affected", affected), zap.Duration("elapsed", time.Since(start)))
}

func mapCouponErr(err error) error {
	if errors.Is(err, mongox.ErrNotFound) {
		return entity.ErrUserCouponNotFound
	}
	return err
}

func toUserCouponDocument(c *entity.UserCoupon) dbmodel.UserCouponsDocument {
	return dbmodel.UserCouponsDocument{
		ID:             c.ID,
		CouponNo:       c.CouponNo,
		UserID:         c.UserID,
		TemplateID:     c.TemplateID,
		BatchID:        c.BatchID,
		Status:         c.Status,
		ObtainAt:       c.ObtainAt,
		ValidStart:     c.ValidStart,
		ValidEnd:       c.ValidEnd,
		UsedAt:         c.UsedAt,
		OrderID:        c.OrderID,
		LockOrderID:    c.LockOrderID,
		DiscountAmount: c.DiscountAmount,
		Source:         c.Source,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
		IsDeleted:      false,
		Version:        c.Version,
	}
}

func toUserCouponDomain(doc *dbmodel.UserCouponsDocument) *entity.UserCoupon {
	if doc == nil {
		return nil
	}
	return &entity.UserCoupon{
		ID:             doc.ID,
		CouponNo:       doc.CouponNo,
		UserID:         doc.UserID,
		TemplateID:     doc.TemplateID,
		BatchID:        doc.BatchID,
		Status:         doc.Status,
		ObtainAt:       doc.ObtainAt,
		ValidStart:     doc.ValidStart,
		ValidEnd:       doc.ValidEnd,
		UsedAt:         doc.UsedAt,
		OrderID:        doc.OrderID,
		LockOrderID:    doc.LockOrderID,
		DiscountAmount: doc.DiscountAmount,
		Source:         doc.Source,
		CreatedAt:      doc.CreatedAt,
		UpdatedAt:      doc.UpdatedAt,
		Version:        doc.Version,
	}
}
