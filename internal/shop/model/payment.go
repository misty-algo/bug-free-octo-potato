package model

import "time"

// PaymentModel 是 payments 表的 Gorm 持久化模型（强一致交易表）。
// 与 payment 服务共用同一张表，结构必须与 internal/payment/model 完全一致：
// 主键统一为 string 支付单号，避免双服务 AutoMigrate 时结构互斥。
type PaymentModel struct {
	ID             string     `gorm:"primaryKey;size:64"`           // 支付单号
	OrderNo        string     `gorm:"size:64;not null;uniqueIndex"` // 业务订单号（幂等唯一键）
	UserID         string     `gorm:"size:64;index"`
	Subject        string     `gorm:"size:128"`
	Amount         int64      `gorm:"not null"`
	PaidAmount     int64      `gorm:"not null;default:0"`
	Status         string     `gorm:"size:32;not null;index"`
	Channel        string     `gorm:"size:32"`
	TradeNo        string     `gorm:"size:128"`
	IdempotentKey  string     `gorm:"size:64;uniqueIndex"`
	RefundedAmount int64      `gorm:"not null;default:0"`
	PrepayID       string     `gorm:"size:128"`
	ExpireAt       *time.Time `gorm:"index"`
	FailReason     string     `gorm:"size:255"`
	NotifyCount    int        `gorm:"not null;default:0"`
	PaidAt         *time.Time `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (PaymentModel) TableName() string {
	return "payments"
}
