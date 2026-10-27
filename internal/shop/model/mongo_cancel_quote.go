package model

import "time"

const cancelQuotesMongoCollectionName = "cancel_quotes"

// CancelQuotesDocument 是 cancel_quotes 集合（取消报价，P12）的 MongoDB 文档模型。
// 对应表结构 huaxiaozhu-final-schema-20261021.js name: 'cancel_quotes'。
type CancelQuotesDocument struct {
	ID           string    `bson:"_id"`
	OrderID      string    `bson:"order_id"`
	QuoteNo      string    `bson:"quote_no"`
	CancelFee    int64     `bson:"cancel_fee"`
	BoundVersion int64     `bson:"bound_version"`
	ReasonCode   string    `bson:"reason_code"`
	ExpiresAt    time.Time `bson:"expires_at"`
	Used         bool      `bson:"used"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (CancelQuotesDocument) MongoCollectionName() string {
	return cancelQuotesMongoCollectionName
}
