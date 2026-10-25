package dto

import "time"

// CreatePaymentCommand 创建支付单入参，金额 int64 分。
type CreatePaymentCommand struct {
	OrderNo       string
	Subject       string
	Amount        int64
	Channel       string
	UserID        string
	IdempotentKey string
	PrepayID      string // 渠道预支付 ID（可选）
	ExpireAt      *time.Time
}

// NotifyPaymentCommand 异步通知支付成功入参，金额 int64 分。
type NotifyPaymentCommand struct {
	OrderNo    string
	TradeNo    string
	Channel    string
	PaidAmount int64
	PaidAt     int64 // Unix 秒
}

// QueryPaymentCommand 查单入参。
type QueryPaymentCommand struct {
	OrderNo string
}

// BeginPaymentCommand 渠道预支付受理回调入参（CREATED → PROCESSING）。
type BeginPaymentCommand struct {
	OrderNo  string
	PrepayID string
}

// FailPaymentCommand 渠道支付失败通知入参（CREATED/PROCESSING → FAILED）。
type FailPaymentCommand struct {
	OrderNo    string
	FailReason string
}

// ClosePaymentCommand 关单入参（CREATED/PROCESSING → CLOSED，超时/主动关闭）。
type ClosePaymentCommand struct {
	OrderNo string
	Reason  string
}

// RefundPaymentCommand 退款入参，金额 int64 分。
type RefundPaymentCommand struct {
	OrderNo      string
	RefundAmount int64
	Reason       string
	OutRequestNo string
}

// ReviewRefundCommand 退款审核入参（REVIEW → PROCESSING / REJECTED）。
type ReviewRefundCommand struct {
	RefundNo     string
	Approved     bool
	RejectReason string
}

// PaymentDTO 支付单视图。
type PaymentDTO struct {
	ID             string
	OrderNo        string
	UserID         string
	Subject        string
	Amount         int64
	PaidAmount     int64
	Status         string
	Channel        string
	TradeNo        string
	RefundedAmount int64
	PrepayID       string
	ExpireAt       *time.Time
	FailReason     string
	NotifyCount    int
	PaidAt         *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
