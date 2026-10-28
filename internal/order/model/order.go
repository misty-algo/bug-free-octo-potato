package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// MongoCollectionName 供 mongox.NewDocumentStore[OrderDocument] 定位 orders 集合。
func (OrderDocument) MongoCollectionName() string { return "orders" }

// OrderDocument 对齐线上 huazhu.orders 的 36 个字段（2026-10-27 实测）。
// 账单/退款/时间线是内嵌子文档（bills/refunds/timeline），不是独立集合。
type OrderDocument struct {
	ID bson.ObjectID `bson:"_id"`

	OrderNo     string            `bson:"orderNo"`
	PassengerID bson.ObjectID     `bson:"passengerId"`
	Type        int32             `bson:"type"`
	TypeName    string            `bson:"typeName"`
	StartID     bson.ObjectID     `bson:"startId"`
	EndID       bson.ObjectID     `bson:"endId"`
	StartLoc    *GeoPointDoc      `bson:"startLocation,omitempty"`
	DistM       int64             `bson:"distM"`
	DurS        int64             `bson:"durS"`
	Seats       int32             `bson:"seats"`
	State       string            `bson:"state"`
	Version     int64             `bson:"version"`
	CpFen       int64             `bson:"cpFen"`
	CdFen       int64             `bson:"cdFen"`
	Fixed       bool              `bson:"fixed"`
	PriceSnap   *PriceSnapshotDoc `bson:"priceSnap,omitempty"`
	DriverID    bson.ObjectID     `bson:"driverId"`
	DriverSnap  *DriverSnapDoc    `bson:"driverSnap,omitempty"`
	Rated       bool              `bson:"rated"`
	Changes     int32             `bson:"changes"`
	CreatedAt   time.Time         `bson:"createdAt"`
	AcceptedAt  *time.Time        `bson:"acceptedAt,omitempty"`
	ArrivedAt   *time.Time        `bson:"arrivedAt,omitempty"`
	EndedAt     *time.Time        `bson:"endedAt,omitempty"`
	UpdatedAt   time.Time         `bson:"updatedAt"`
	Timeline    []TimelineDoc     `bson:"timeline"`
	Refunds     []RefundDoc       `bson:"refunds"`
	Bills       []BillDoc         `bson:"bills"`
	DispatchID  bson.ObjectID     `bson:"dispatchId"`
	BizType     int32             `bson:"bizType"`
	CityCode    string            `bson:"cityCode"`
	PricingRule bson.ObjectID     `bson:"pricingRuleId"`
	PricingSnap bson.ObjectID     `bson:"pricingSnapshotId"`
	PricingVer  string            `bson:"pricingVersion"`
	RiskLevel   string            `bson:"riskLevel"`
	SurgeRatio  float64           `bson:"surgeRatio"`
	// Matching 子文档线上暂无样本，按原样透传，字段名待匹配域落地后校准。
	Matching bson.M `bson:"matching,omitempty"`

	// Rating 是 P19 评价内嵌子文档（本模块新增字段；线上尚未有已评价订单）。
	Rating *RatingDoc `bson:"rating,omitempty"`
}

// RatingDoc 订单内嵌评价结果。
type RatingDoc struct {
	Score     int32     `bson:"score"`
	Content   string    `bson:"content,omitempty"`
	Anonymous bool      `bson:"anonymous"`
	RatedAt   time.Time `bson:"ratedAt"`
}

// BillDoc 订单内嵌账单元素。
// TODO(孙源/账单流程)：字段名待账单落地后与线上实际写入校准。
type BillDoc struct {
	Kind                  string     `bson:"kind,omitempty"`
	TotalFee              int64      `bson:"totalFee"`
	DriverGrossFee        int64      `bson:"driverGrossFee"`
	PlatformServiceFee    int64      `bson:"platformServiceFee"`
	SurgeFee              int64      `bson:"surgeFee"`
	CouponFee             int64      `bson:"couponFee"`
	PlatformCouponBearFee int64      `bson:"platformCouponBearFee"`
	DriverCouponBearFee   int64      `bson:"driverCouponBearFee"`
	PayableFee            int64      `bson:"payableFee"`
	DriverIncomeFee       int64      `bson:"driverIncomeFee"`
	PlatformNetFee        int64      `bson:"platformNetFee"`
	State                 string     `bson:"state,omitempty"`
	PaidAt                *time.Time `bson:"paidAt,omitempty"`
	Channel               string     `bson:"channel,omitempty"`
}

// RefundDoc 订单内嵌退款元素。
// TODO(孙源/账单流程)：字段名待退款落地后校准。
type RefundDoc struct {
	RefundFee       int64     `bson:"refundFee"`
	DriverRefundFee int64     `bson:"driverRefundFee"`
	Mode            string    `bson:"mode,omitempty"`
	State           string    `bson:"state,omitempty"`
	T               time.Time `bson:"t"`
}

// TimelineDoc 订单内嵌时间线元素（线上字段：name、t）。
type TimelineDoc struct {
	Name string    `bson:"name"`
	T    time.Time `bson:"t"`
}

// DriverSnapDoc 司机快照（防止司机改名后历史单漂移）。
type DriverSnapDoc struct {
	Name string `bson:"name"`
}

// PriceSnapshotDoc 对齐线上 orders.priceSnap 子文档（mongodb.js: { T,Q,H,F,K,fixed }，单位分）。
type PriceSnapshotDoc struct {
	T     int64 `bson:"T,omitempty"`
	Q     int64 `bson:"Q,omitempty"`
	H     int64 `bson:"H,omitempty"`
	F     int64 `bson:"F,omitempty"`
	K     int64 `bson:"K,omitempty"`
	Fixed bool  `bson:"fixed,omitempty"`
}

// GeoPointDoc 对应线上 GeoJSON Point 子文档。
type GeoPointDoc struct {
	Type        string    `bson:"type"`
	Coordinates []float64 `bson:"coordinates"`
}
