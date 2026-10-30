package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// platformCouponDocument 对齐管理端营销中心写入的 coupons 集合文档（只读）。
//
// 集合由管理端运营发放券时写入，字段命名与 MongoBSON 风格均以管理端为准：
// couponNo/templateNo/faceValueFen/thresholdFen/expireAt/state。
// passengerId 为管理端账号的 ObjectID，与乘客端账号体系不同，
// 因此乘客端领取按「公开发放、先到先得」语义处理，不做 passengerId 匹配。
type platformCouponDocument struct {
	CouponNo     string    `bson:"couponNo"`
	TemplateNo   string    `bson:"templateNo,omitempty"`
	State        string    `bson:"state"`
	FaceValueFen int64     `bson:"faceValueFen"`
	ThresholdFen int64     `bson:"thresholdFen"`
	ExpireAt     time.Time `bson:"expireAt"`
	CreatedAt    time.Time `bson:"createdAt"`
}

func (platformCouponDocument) MongoCollectionName() string { return "coupons" }

// PlatformCouponRepository 是平台券的 MongoDB 只读仓储。
type PlatformCouponRepository struct {
	collection *mongox.Collection[platformCouponDocument]
}

// NewPlatformCouponRepository 创建平台券只读仓储。
func NewPlatformCouponRepository(db *mongo.Database) *PlatformCouponRepository {
	return &PlatformCouponRepository{
		collection: mongox.NewCollection[platformCouponDocument](db, platformCouponDocument{}.MongoCollectionName()),
	}
}

// ListAvailable 列出当前可领取的平台券（AVAILABLE 且未过期），按发放时间倒序。
func (r *PlatformCouponRepository) ListAvailable(ctx context.Context, limit int, now time.Time) ([]*entity.PlatformCoupon, error) {
	documents, err := r.collection.FindMany(ctx, bson.M{
		"state":    entity.PlatformCouponStateAvailable,
		"expireAt": bson.M{"$gt": now},
	}, options.Find().SetSort(bson.D{{Key: "createdAt", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	list := make([]*entity.PlatformCoupon, 0, len(documents))
	for i := range documents {
		list = append(list, toPlatformCoupon(&documents[i]))
	}
	return list, nil
}

// FindByCouponNo 按券码查询；不存在返回 ErrPlatformCouponNotFound。
func (r *PlatformCouponRepository) FindByCouponNo(ctx context.Context, couponNo string) (*entity.PlatformCoupon, error) {
	document, err := r.collection.FindOne(ctx, bson.M{"couponNo": couponNo})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrPlatformCouponNotFound
		}
		return nil, err
	}
	return toPlatformCoupon(document), nil
}

// toPlatformCoupon 把平台券文档转成领域对象。
func toPlatformCoupon(document *platformCouponDocument) *entity.PlatformCoupon {
	return &entity.PlatformCoupon{
		CouponNo:     document.CouponNo,
		TemplateNo:   document.TemplateNo,
		State:        document.State,
		FaceValueFen: document.FaceValueFen,
		ThresholdFen: document.ThresholdFen,
		ExpireAt:     document.ExpireAt,
	}
}
