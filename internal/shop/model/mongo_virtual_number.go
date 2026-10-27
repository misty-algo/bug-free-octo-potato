package model

import "time"

const virtualNumbersMongoCollectionName = "virtual_numbers"

// VirtualNumbersDocument 是 virtual_numbers 集合（虚拟号，P09/D07）的 MongoDB 文档模型。
// 对应表结构 huaxiaozhu-final-schema-20261021.js name: 'virtual_numbers'。
type VirtualNumbersDocument struct {
	ID          string    `bson:"_id"`
	OrderID     string    `bson:"order_id"`
	PassengerID string    `bson:"passenger_id"`
	DriverID    string    `bson:"driver_id"`
	VirtualNo   string    `bson:"virtual_no"`
	Status      string    `bson:"status"` // bound / released / expired
	BoundAt     time.Time `bson:"bound_at"`
	ReleasedAt  time.Time `bson:"released_at"`
	ExpireAt    time.Time `bson:"expire_at"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (VirtualNumbersDocument) MongoCollectionName() string {
	return virtualNumbersMongoCollectionName
}
