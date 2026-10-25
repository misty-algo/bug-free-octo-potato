package entity

import "context"

// Repository 是 payment 聚合的持久化端口。
type Repository interface {
	// SavePayment 新增或更新支付单。
	SavePayment(ctx context.Context, payment *Payment) error
	// FindPaymentByOrderNo 按业务订单号查询支付单。
	FindPaymentByOrderNo(ctx context.Context, orderNo string) (*Payment, error)
	// FindPaymentByPaymentNo 按支付单号查询。
	FindPaymentByPaymentNo(ctx context.Context, paymentNo string) (*Payment, error)
	// FindPaymentByIdempotentKey 按创建幂等键查询。
	FindPaymentByIdempotentKey(ctx context.Context, key string) (*Payment, error)

	// SaveEvent 保存待处理支付事件（RocketMQ 不可用时的兜底通道）。
	SaveEvent(ctx context.Context, event *PaymentEvent) error
	// FindEventByOrderNoAndType 查询某订单某类型的事件（幂等判重）。
	FindEventByOrderNoAndType(ctx context.Context, orderNo string, eventType string) (*PaymentEvent, error)
	// UpdateEventStatus 更新事件状态与重试次数。
	UpdateEventStatus(ctx context.Context, eventNo string, status string, retryCount int) error
	// ListPendingEvents 列出待处理事件（供补偿任务扫描）。
	ListPendingEvents(ctx context.Context, limit int) ([]*PaymentEvent, error)

	// SaveRefund 新增或更新退款单。
	SaveRefund(ctx context.Context, refund *Refund) error
	// FindRefundByRefundNo 按退款单号查询。
	FindRefundByRefundNo(ctx context.Context, refundNo string) (*Refund, error)
	// FindRefundByOutRequestNo 按渠道退款请求号查询（退款幂等锚点）。
	FindRefundByOutRequestNo(ctx context.Context, outRequestNo string) (*Refund, error)
	// ListRefundsByOrderNo 按业务订单号列出退款单。
	ListRefundsByOrderNo(ctx context.Context, orderNo string) ([]*Refund, error)

	// AddRefundedAmount 原子累加支付单累计退款金额（带防超退条件）：
	// 仅当 refunded_amount + amount <= paid_amount 时成功，避免并发退款互相覆盖。
	AddRefundedAmount(ctx context.Context, paymentNo string, amount int64) error
}
