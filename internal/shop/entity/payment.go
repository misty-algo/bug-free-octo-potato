package entity

import (
	"errors"
	"time"
)

// 支付相关领域错误，供 service 返回并由 handler 映射为 gRPC 错误。
var (
	// ErrInvalidPaymentRequest 表示支付/退款入参不合法。
	ErrInvalidPaymentRequest = errors.New("invalid payment request")
	// ErrAlipayNotConfigured 表示支付宝客户端未配置，支付用例不可用。
	ErrAlipayNotConfigured = errors.New("alipay not configured")
	// ErrWechatNotConfigured 表示微信支付客户端未配置，微信支付用例不可用。
	ErrWechatNotConfigured = errors.New("wechat pay not configured")
	// ErrPaymentNotFound 表示按 order_no 未找到本地支付单。
	ErrPaymentNotFound = errors.New("payment not found")
	// ErrPaymentExists 表示 order_no 已存在重复支付单（幂等锚点冲突）。
	ErrPaymentExists = errors.New("payment already exists")
	// ErrPaymentAmountMismatch 表示同一 order_no 重复下单但金额不一致，防止渠道与本地金额分裂。
	ErrPaymentAmountMismatch = errors.New("payment amount mismatch")
	// ErrPaymentChannelUnsupported 表示不支持的支付渠道。
	ErrPaymentChannelUnsupported = errors.New("unsupported payment channel")
	// ErrPaymentInvalidState 表示支付单当前状态不允许目标状态迁移（终态保护）。
	ErrPaymentInvalidState = errors.New("payment state transition not allowed")
	// ErrRefundNotFound 表示按 out_request_no 未找到本地退款记录。
	ErrRefundNotFound = errors.New("refund not found")
	// ErrRefundExists 表示 out_request_no 幂等键冲突（重复退款请求）。
	ErrRefundExists = errors.New("refund already exists")
	// ErrRefundExceedsPaid 表示累计退款将超过实付金额。
	ErrRefundExceedsPaid = errors.New("refund exceeds paid amount")
)

// PaymentStatus 支付单状态。
type PaymentStatus string

const (
	// PaymentStatusPending 已下单等待支付。
	PaymentStatusPending PaymentStatus = "pending"
	// PaymentStatusSuccess 支付成功。
	PaymentStatusSuccess PaymentStatus = "success"
	// PaymentStatusRefunded 已退款。
	PaymentStatusRefunded PaymentStatus = "refunded"
	// PaymentStatusFailed 支付失败/已关闭。
	PaymentStatusFailed PaymentStatus = "failed"
)

// PaymentChannel 支付渠道。
type PaymentChannel string

const (
	// PaymentChannelAlipay 支付宝。
	PaymentChannelAlipay PaymentChannel = "alipay"
	// PaymentChannelWechat 微信支付。
	PaymentChannelWechat PaymentChannel = "wechat"
)

// Payment 是本地支付单聚合，与渠道交易单一一对应。
type Payment struct {
	ID            string // 支付单号（payments 表主键，与 payment 服务共用表结构）
	OrderNo       string
	Amount        int64 // 金额，单位分
	Status        PaymentStatus
	Channel       PaymentChannel
	TradeNo       string
	IdempotentKey string // 幂等键（与 payment 服务口径一致：OrderNo），避免空串撞 PG 唯一索引
	PaidAt        *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IsPending 判断支付单是否处于待支付状态。
func (p *Payment) IsPending() bool {
	return p.Status == PaymentStatusPending
}
