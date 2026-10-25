package model

import "time"

const destinationChangesMongoCollectionName = "destination_changes"

// DestinationChangesDocument 是 destination_changes 集合（中途改点，P10）的 MongoDB 文档模型。
// 对应表结构 huaxiaozhu-final-schema-20261021.js name: 'destination_changes'。
type DestinationChangesDocument struct {
	ID            string    `bson:"_id"`
	OrderID       string    `bson:"order_id"`
	NewEndLocation GeoPoint  `bson:"new_end_location"`
	NewEndAddress string    `bson:"new_end_address"`
	NewEndPoiID   string    `bson:"new_end_poi_id"`
	NewEstimateFee int64     `bson:"new_estimate_fee"`
	Status        string    `bson:"status"` // pending / accepted / rejected / timeout / cancelled
	RequestedAt   time.Time `bson:"requested_at"`
	ConfirmedAt   time.Time `bson:"confirmed_at"`
	ExpiresAt     time.Time `bson:"expires_at"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DestinationChangesDocument) MongoCollectionName() string {
	return destinationChangesMongoCollectionName
}
