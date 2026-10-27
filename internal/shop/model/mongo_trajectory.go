package model

import "time"

const trajectoriesMongoCollectionName = "trajectories"

// TrajectoriesDocument 是 trajectories 集合（实时位置轨迹，P08）的 MongoDB 文档模型。
// 对应表结构 huaxiaozhu-final-schema-20261021.js name: 'trajectories'。
type TrajectoriesDocument struct {
	ID          string    `bson:"_id"`
	OrderID     string    `bson:"order_id"`
	DriverID    string    `bson:"driver_id"`
	Location    GeoPoint  `bson:"location"`
	Speed       float64   `bson:"speed"`
	Bearing     float64   `bson:"bearing"`
	Accuracy    float64   `bson:"accuracy"`
	Mileage     float64   `bson:"mileage"`
	PointType   string    `bson:"point_type"` // pickup / service
	ReportedAt  time.Time `bson:"reported_at"`
	CollectedAt time.Time `bson:"collected_at"`
	Source      string    `bson:"source"` // app / device

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (TrajectoriesDocument) MongoCollectionName() string {
	return trajectoriesMongoCollectionName
}
