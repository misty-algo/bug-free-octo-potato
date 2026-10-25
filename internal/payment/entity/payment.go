package entity

import (
	"errors"
	"time"
)

// 支付单状态机（P13，对齐权威 schema PAY_STATE 五态）：
//
//	CREATED → PROCESSING → SUCCESS（渠道支付成功）
//	                       └─ FAILED（支付失败）
//	CREATED / PROCESSING → CLOSED（15 分钟未支付自动关单 / 主动关单）
//
// 支付单自身不承载退款状态：退款状态由 refunds.status（REFUND_STATE）表达，
// 全额退款完成后支付单保持 SUCCESS，refunded_amount == paid_amount。
const (
	PaymentStatusCreated    = "CREATED"    // 已创建待支付（初始态）
	PaymentStatusProcessing = "PROCESSING" // 支付处理中（渠道已受理预支付）
	PaymentStatusSuccess    = "SUCCESS"    // 支付成功（可发起退款）
	PaymentStatusFailed     = "FAILED"     // 支付失败
	PaymentStatusClosed     = "CLOSED"     // 已关单（超时未支付 / 主动关闭，终态）
)

// 退款单状态机（P16，对齐权威 schema REFUND_STATE 五态）：
//
//	REVIEW → PROCESSING（审核通过，提交渠道）→ SUCCESS（渠道退款成功）
//	                                              └─ FAILED（渠道退款失败，可重试）
//	REVIEW → REJECTED（审核拒绝，终态）
const (
	RefundStatusReview     = "REVIEW"     // 待审核（初始态）
	RefundStatusProcessing = "PROCESSING" // 已提交渠道，等待确认
	RefundStatusSuccess    = "SUCCESS"    // 渠道退款成功
	RefundStatusFailed     = "FAILED"     // 渠道退款失败（可重试）
	RefundStatusRejected   = "REJECTED"   // 审核拒绝（终态）
)

// 支付/退款事件状态（RocketMQ 不可用时的待处理表）。
const (
	EventStatusPending = "pending"
	EventStatusSuccess = "success"
	EventStatusFailed  = "failed"
)

// 事件类型。
const (
	EventTypePaySuccess    = "pay_success"    // 支付成功
	EventTypeRefundSuccess = "refund_success" // 退款成功
)

// MaxEventRetry 待处理事件的最大重试次数，超过后标记 failed 由人工/告警介入。
const MaxEventRetry = 5

// 支付单聚合。
type Payment struct {
	ID             string // 支付单号 payment_no
	OrderNo        string // 业务订单号（幂等唯一键）
	UserID         string
	Subject        string
	Amount         int64 // 应付金额（分）
	PaidAmount     int64 // 实付金额（分）
	Status         string
	Channel        string
	TradeNo        string // 渠道交易号
	IdempotentKey  string // 创建幂等键
	RefundedAmount int64  // 累计已退金额（分）
	PrepayID       string // 渠道预支付 ID（如 alipay trade 预创建）
	ExpireAt       *time.Time
	FailReason     string // 支付失败/关单原因
	NotifyCount    int    // 渠道异步通知次数（幂等防重参考）
	PaidAt         *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// 支付成功事件（待处理消息）。
type PaymentEvent struct {
	ID         string // 事件号
	OrderNo    string
	EventType  string
	Payload    string
	Status     string
	RetryCount int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Refund 退款单聚合（P16）。
// 与支付单一一关联：同一支付单可多次部分退款，但累计退款金额不得超过实付金额。
type Refund struct {
	ID             string // 退款单号 refund_no
	PaymentNo      string // 关联支付单号
	OrderNo        string // 业务订单号
	UserID         string
	Amount         int64 // 退款金额（分）
	Status         string
	OutRequestNo   string // 渠道退款请求号（幂等锚点，唯一键）
	ChannelTradeNo string // 渠道退款交易号
	Reason         string // 退款原因
	RejectReason   string // 审核拒绝原因
	RefundedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// 领域错误。
var (
	ErrPaymentNotFound       = errors.New("payment not found")
	ErrPaymentAlreadyExists  = errors.New("payment already exists")
	ErrPaymentAlreadyPaid    = errors.New("payment already paid")
	ErrPaymentClosed         = errors.New("payment closed")
	ErrInvalidPaymentRequest = errors.New("invalid payment request")
	ErrEventNotFound         = errors.New("payment event not found")
	ErrRefundNotFound        = errors.New("refund not found")
	ErrRefundAlreadyExists   = errors.New("refund already exists")
	ErrRefundChannelFailed   = errors.New("refund channel failed")
	ErrRefundExceedsPaid     = errors.New("refund amount exceeds paid amount")
	ErrRefundStateTransition = errors.New("invalid refund state transition")
	ErrNotifyConcurrent      = errors.New("payment notify in progress, retry later")
)
