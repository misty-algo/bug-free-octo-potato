package model

import "time"

// PaymentModel 是 payments 表的 Gorm 持久化模型（强一致交易表，MySQL）。
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

// PaymentEventModel 是 payment_events 表的 Gorm 模型（RocketMQ 不可用时的待处理消息表）。
type PaymentEventModel struct {
	ID         string `gorm:"primaryKey;size:64"` // 事件号
	OrderNo    string `gorm:"size:64;not null;uniqueIndex:idx_event_order_type"`
	EventType  string `gorm:"size:32;not null;uniqueIndex:idx_event_order_type"`
	Payload    string `gorm:"type:text"`
	Status     string `gorm:"size:32;not null;index"`
	RetryCount int    `gorm:"not null;default:0"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (PaymentEventModel) TableName() string {
	return "payment_events"
}

// RefundModel 是 refunds 表的 Gorm 持久化模型（强一致交易表，MySQL）。
type RefundModel struct {
	ID             string     `gorm:"primaryKey;size:64"`     // 退款单号
	PaymentNo      string     `gorm:"size:64;not null;index"` // 关联支付单号
	OrderNo        string     `gorm:"size:64;not null;index"`
	UserID         string     `gorm:"size:64;index"`
	Amount         int64      `gorm:"not null"`
	Status         string     `gorm:"size:32;not null;index"`
	OutRequestNo   string     `gorm:"size:64;not null;uniqueIndex"` // 渠道退款请求号（幂等唯一键）
	ChannelTradeNo string     `gorm:"size:128"`
	Reason         string     `gorm:"size:255"`
	RejectReason   string     `gorm:"size:255"`
	RefundedAt     *time.Time `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (RefundModel) TableName() string {
	return "refunds"
}
