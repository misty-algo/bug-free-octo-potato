package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// CouponDocument 是优惠券实例的 MongoDB 文档模型。
type CouponDocument struct {
	ID         string `bson:"_id"`
	AccountID  string `bson:"account_id"`
	TemplateID string `bson:"template_id"`
	Name       string `bson:"name"`
	// FaceValue 与 Threshold 单位为分。
	FaceValue   int64     `bson:"face_value"`
	Threshold   int64     `bson:"threshold"`
	CarTypes    []int     `bson:"car_types"`
	State       string    `bson:"state"`
	LockOrderID string    `bson:"lock_order_id"`
	ExpiresAt   time.Time `bson:"expires_at"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
	IsDeleted   bool      `bson:"is_deleted"`
}

func (CouponDocument) MongoCollectionName() string { return "passenger_coupons" }

// DriverDocument 是司机资料的 MongoDB 文档模型。
//
// 乘客端只读这个集合，不写入。手机号以掩码形式落库，
// 真实手机号不出现在乘客端响应里。
type DriverDocument struct {
	ID           string    `bson:"_id"`
	AccountID    string    `bson:"account_id"`
	OrderID      string    `bson:"order_id"`
	Name         string    `bson:"name"`
	PlateNo      string    `bson:"plate_no"`
	CarModel     string    `bson:"car_model"`
	Rating       string    `bson:"rating"`
	AuditState   string    `bson:"audit_state"`
	PhoneMask    string    `bson:"phone_mask"`
	Online       bool      `bson:"online"`
	LastLng      float64   `bson:"last_lng"`
	LastLat      float64   `bson:"last_lat"`
	LastReportAt time.Time `bson:"last_report_at"`
	CreatedAt    time.Time `bson:"created_at"`
	UpdatedAt    time.Time `bson:"updated_at"`
	IsDeleted    bool      `bson:"is_deleted"`
}

func (DriverDocument) MongoCollectionName() string { return "passenger_drivers" }

// CouponRepository 是优惠券的 MongoDB 仓储实现。
type CouponRepository struct {
	collection *mongox.Collection[CouponDocument]
}

// NewCouponRepository 创建优惠券仓储。
func NewCouponRepository(db *mongo.Database) *CouponRepository {
	return &CouponRepository{collection: mongox.NewCollection[CouponDocument](db, CouponDocument{}.MongoCollectionName())}
}

// FindByID 按标识查券。
func (r *CouponRepository) FindByID(ctx context.Context, id string) (*entity.Coupon, error) {
	document, err := r.collection.FindOne(ctx, bson.M{"_id": id, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrCouponNotFound
		}
		return nil, err
	}
	return toCouponEntity(document), nil
}

// ListUsable 列出该账号未过期且未使用的券。
func (r *CouponRepository) ListUsable(ctx context.Context, accountID string, now time.Time) ([]*entity.Coupon, error) {
	documents, err := r.collection.FindMany(ctx, bson.M{
		"account_id": accountID,
		"state":      string(entity.CouponStateAvailable),
		"expires_at": bson.M{"$gt": now},
		"is_deleted": false,
	})
	if err != nil {
		return nil, err
	}
	list := make([]*entity.Coupon, 0, len(documents))
	for i := range documents {
		list = append(list, toCouponEntity(&documents[i]))
	}
	return list, nil
}

// Lock 在原子条件更新下锁定券。
//
// 用条件更新而不是先查后改：并发下同一张券只能被一个订单锁住，
// 第二个订单会因匹配数为零而失败。
func (r *CouponRepository) Lock(ctx context.Context, id string, orderID string, now time.Time) error {
	result, err := r.collection.UpdateOne(ctx, bson.M{
		"_id":        id,
		"state":      string(entity.CouponStateAvailable),
		"is_deleted": false,
	}, bson.M{
		"$set": bson.M{
			"state":         string(entity.CouponStateLocked),
			"lock_order_id": orderID,
			"updated_at":    now,
		},
	})
	if err != nil {
		return err
	}
	if result == nil || result.MatchedCount == 0 {
		return entity.ErrCouponLocked
	}
	return nil
}

// Unlock 释放锁定的券。
func (r *CouponRepository) Unlock(ctx context.Context, id string, now time.Time) error {
	_, err := r.collection.UpdateOne(ctx, bson.M{
		"_id":        id,
		"state":      string(entity.CouponStateLocked),
		"is_deleted": false,
	}, bson.M{
		"$set": bson.M{
			"state":         string(entity.CouponStateAvailable),
			"lock_order_id": "",
			"updated_at":    now,
		},
	})
	return err
}

// DriverRepository 是司机档案的 MongoDB 仓储实现，乘客端只读。
type DriverRepository struct {
	collection *mongox.Collection[DriverDocument]
}

// NewDriverRepository 创建司机档案仓储。
func NewDriverRepository(db *mongo.Database) *DriverRepository {
	return &DriverRepository{collection: mongox.NewCollection[DriverDocument](db, DriverDocument{}.MongoCollectionName())}
}

// FindByAccountID 按账号查司机档案。
func (r *DriverRepository) FindByAccountID(ctx context.Context, accountID string) (*entity.DriverProfile, error) {
	document, err := r.collection.FindOne(ctx, bson.M{"account_id": accountID, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrDriverNotRegistered
		}
		return nil, err
	}
	return toDriverEntity(document), nil
}

// FindByOrderID 按订单查当前分配到的司机档案。
func (r *DriverRepository) FindByOrderID(ctx context.Context, orderID string) (*entity.DriverProfile, error) {
	document, err := r.collection.FindOne(ctx, bson.M{"order_id": orderID, "is_deleted": false})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrDriverNotAssigned
		}
		return nil, err
	}
	return toDriverEntity(document), nil
}

// ListByAccount 分页列出订单。
func (s *Store) listOrders(ctx context.Context, accountID string, state entity.OrderState, offset, limit int) ([]*entity.Order, error) {
	filter := bson.M{"account_id": accountID, "is_deleted": false}
	if state != "" {
		filter["state"] = string(state)
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetSkip(int64(offset)).SetLimit(int64(limit))
	documents, err := s.orders.FindMany(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	list := make([]*entity.Order, 0, len(documents))
	for i := range documents {
		list = append(list, toOrderEntity(&documents[i]))
	}
	return list, nil
}

// countOrders 统计符合条件的订单数。
func (s *Store) countOrders(ctx context.Context, accountID string, state entity.OrderState) (int, error) {
	filter := bson.M{"account_id": accountID, "is_deleted": false}
	if state != "" {
		filter["state"] = string(state)
	}
	count, err := s.orders.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// toCouponEntity 把落库文档转成优惠券实体。
func toCouponEntity(document *CouponDocument) *entity.Coupon {
	return &entity.Coupon{
		ID:          document.ID,
		AccountID:   document.AccountID,
		TemplateID:  document.TemplateID,
		Name:        document.Name,
		FaceValue:   document.FaceValue,
		Threshold:   document.Threshold,
		CarTypes:    document.CarTypes,
		State:       entity.CouponState(document.State),
		LockOrderID: document.LockOrderID,
		ExpiresAt:   document.ExpiresAt,
		CreatedAt:   document.CreatedAt,
		UpdatedAt:   document.UpdatedAt,
	}
}

// toDriverEntity 把落库文档转成司机档案实体。
func toDriverEntity(document *DriverDocument) *entity.DriverProfile {
	return &entity.DriverProfile{
		ID:           document.ID,
		AccountID:    document.AccountID,
		Name:         document.Name,
		PlateNo:      document.PlateNo,
		CarModel:     document.CarModel,
		Rating:       document.Rating,
		AuditState:   entity.DriverAuditState(document.AuditState),
		PhoneMask:    document.PhoneMask,
		Online:       document.Online,
		LastLng:      document.LastLng,
		LastLat:      document.LastLat,
		LastReportAt: document.LastReportAt,
		UpdatedAt:    document.UpdatedAt,
	}
}
