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
	Name         string    `bson:"name,omitempty"`
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

// ListAvailable 列出当前可领取的平台券（AVAILABLE、券码非空且未过期），
// 按发放时间倒序。券码为空的历史券无法被领取（领取请求按券码定位），
// 不进领券中心，避免出现点了就报参数校验错误的死条目。
func (r *PlatformCouponRepository) ListAvailable(ctx context.Context, limit int, now time.Time) ([]*entity.PlatformCoupon, error) {
	documents, err := r.collection.FindMany(ctx, bson.M{
		"state":    entity.PlatformCouponStateAvailable,
		"couponNo": bson.M{"$exists": true, "$nin": bson.A{nil, ""}},
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

// ClaimOne 原子抢占一张券：state=AVAILABLE 且未过期才会被置为 CLAIMED。
// 条件更新由 Mongo 保证并发下只有一个人能领到（MatchedCount=1），
// 没抢到（已被他人领走/已过期）返回 false，由 service 转成业务错误。
func (r *PlatformCouponRepository) ClaimOne(ctx context.Context, couponNo, holderAccountID string, now time.Time) (bool, error) {
	res, err := r.collection.UpdateOne(ctx,
		bson.M{
			"couponNo": couponNo,
			"state":    entity.PlatformCouponStateAvailable,
			"expireAt": bson.M{"$gt": now},
		},
		bson.M{"$set": bson.M{
			"state":         entity.PlatformCouponStateClaimed,
			"claimedAt":     now,
			"claimedBy":     holderAccountID,
			"claimedDomain": "passenger",
			"updatedAt":     now,
		}},
	)
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// toPlatformCoupon 把平台券文档转成领域对象。
func toPlatformCoupon(document *platformCouponDocument) *entity.PlatformCoupon {
	return &entity.PlatformCoupon{
		CouponNo:     document.CouponNo,
		TemplateNo:   document.TemplateNo,
		Name:         document.Name,
		State:        document.State,
		FaceValueFen: document.FaceValueFen,
		ThresholdFen: document.ThresholdFen,
		ExpireAt:     document.ExpireAt,
	}
}
