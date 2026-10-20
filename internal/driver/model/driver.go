package model

import "time"

// DriverModel 是 drivers 表的 Gorm 持久化模型。
type DriverModel struct {
	ID          string `gorm:"primaryKey;size:64"`
	Name        string `gorm:"size:128;not null"`
	Description string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (DriverModel) TableName() string {
	return "drivers"
}

const driverMongoCollectionName = "drivers"

// DriverDocument 是 drivers 集合的 MongoDB 文档模型。
type DriverDocument struct {
	ID          string    `bson:"_id"`
	Name        string    `bson:"name"`
	Description string    `bson:"description"`
	CreatedAt   time.Time `bson:"created_at"`
	UpdatedAt   time.Time `bson:"updated_at"`
}

func (DriverDocument) MongoCollectionName() string {
	return driverMongoCollectionName
}
