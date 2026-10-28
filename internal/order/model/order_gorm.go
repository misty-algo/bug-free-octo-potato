package model

import "time"

// OrderModel 是 orders 表的 Gorm 持久化模型。
// 注：order 服务已切换 Mongo 仓储；Gorm 路径保留编译供其他场景使用。
type OrderModel struct {
	ID          string `gorm:"primaryKey;size:64"`
	Name        string `gorm:"size:128;not null"`
	Description string `gorm:"type:text"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (OrderModel) TableName() string {
	return "orders"
}
