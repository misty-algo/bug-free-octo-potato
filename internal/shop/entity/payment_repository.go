package entity

import (
	"context"
	"time"
)

// PaymentRepository 定义 shop service 层需要的支付单持久化行为。
// Create 以 order_no 唯一索引作为幂等锚点：重复订单返回 ErrPaymentExists。
type PaymentRepository interface {
	// Create 落库一条 pending 支付单；order_no 已存在时返回 ErrPaymentExists。
	Create(ctx context.Context, p *Payment) error
	// Update 更新支付单状态等信息。
	Update(ctx context.Context, p *Payment) error
	// FindByOrderNo 按商户订单号查询支付单；不存在返回 ErrPaymentNotFound。
	FindByOrderNo(ctx context.Context, orderNo string) (*Payment, error)
	// ListPendingBefore 返回创建时间早于 before 的 pending 支付单，用于掉单补偿扫描。
	ListPendingBefore(ctx context.Context, before time.Time, limit int) ([]*Payment, error)
}
