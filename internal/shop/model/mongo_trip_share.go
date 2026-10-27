package model

import "time"

const tripSharesMongoCollectionName = "trip_shares"

// TripSharesDocument 是 trip_shares 集合（行程分享，P11）的 MongoDB 文档模型。
// 对应表结构 huaxiaozhu-final-schema-20261021.js name: 'trip_shares'。
type TripSharesDocument struct {
	ID          string    `bson:"_id"`
	OrderID     string    `bson:"order_id"`
	ShareToken  string    `bson:"share_token"`
	CreatorID   string    `bson:"creator_id"`
	ExpireAt    time.Time `bson:"expire_at"`
	Revoked     bool      `bson:"revoked"`
	AccessCount int64     `bson:"access_count"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (TripSharesDocument) MongoCollectionName() string {
	return tripSharesMongoCollectionName
}
