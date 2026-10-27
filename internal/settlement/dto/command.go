package dto

import "time"

// CreateBillCommand 行程结束生成乘客账单入参，金额 int64 分。
type CreateBillCommand struct {
	OrderNo         string
	UserID          string
	DriverID        string
	TotalAmount     int64 // 乘客实付金额
	DiscountFee     int64 // 优惠总额（可选，默认 0）
	PlatformSubsidy int64 // 平台补贴（可选，默认 0）
	Remark          string
}

// MarkBillPaidCommand 支付成功标记账单已付入参（bill_no 与 order_no 二选一）。
type MarkBillPaidCommand struct {
	BillNo      string
	OrderNo     string
	PayFee      int64 // 实际支付金额（可选，默认沿用账单实付）
	DiscountFee int64 // 优惠总额（可选，默认沿用账单）
	RefundFee   int64 // 已退金额（可选，默认沿用账单）
}

// VoidBillCommand 作废账单入参（bill_no 与 order_no 二选一）。
type VoidBillCommand struct {
	BillNo  string
	OrderNo string
	Reason  string
}

// ListUnpaidBillsQuery 查询用户待支付账单。
type ListUnpaidBillsQuery struct {
	UserID string
}

// UnpaidBanDTO 未付禁叫车校验结果。
type UnpaidBanDTO struct {
	Banned          bool  // 是否存在未付账单
	UnpaidCount     int   // 未付账单数
	UnpaidAmountSum int64 // 未付总金额（分）
}

// SplitBillCommand 分账入参。
type SplitBillCommand struct {
	OrderNo         string // 行程订单号（幂等锚点，同一订单只分账一次）
	CommissionRate  int64  // 平台佣金比例，万分比，如 2000 = 20%
	DriverSubsidy   int64  // 司机补贴（可选）
	PlatformSubsidy int64  // 平台补贴（可选，大于 0 时覆盖账单补贴）
	IdempotentKey   string // 幂等键（可选，默认沿用账单号派生锚点）
}

// ApplyRefundCommand 退款冲正入参。
type ApplyRefundCommand struct {
	OrderNo      string // 行程订单号（幂等锚点，同一订单只冲正一次）
	RefundNo     string // 退款单号（仅记录）
	RefundAmount int64  // 本次退款金额（分）
}

// RefundSettlementResult 退款冲正结果。
type RefundSettlementResult struct {
	BillNo          string
	OrderNo         string
	ReversedAmount  int64    // 本次冲正金额
	Repeated        bool     // 是否重复冲正（幂等命中）
	ReversedHistory []string // 已冲正的流水号（幂等命中时携带历史）
}

// GetWalletBalanceQuery 查询司机钱包。
type GetWalletBalanceQuery struct {
	DriverID string
}

// BillDTO 账单视图。
type BillDTO struct {
	ID              string
	OrderNo         string
	UserID          string
	DriverID        string
	TotalAmount     int64
	PayFee          int64
	DiscountFee     int64
	RefundFee       int64
	DriverIncome    int64
	CommissionFee   int64
	SubsidyFee      int64
	Settled         bool
	PlatformSubsidy int64
	Status          string
	Remark          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// SplitResult 分账结果。
type SplitResult struct {
	BillNo             string
	OrderNo            string
	PassengerPaid      int64 // 乘客实付
	PlatformCommission int64 // 平台佣金
	DriverIncome       int64 // 司机收入
	PlatformSubsidy    int64 // 平台补贴
	Status             string // 分账后账单状态（真实状态，P9：不再硬编码）
	Repeated           bool  // 是否重复分账（幂等命中）
}

// WalletDTO 钱包视图。
type WalletDTO struct {
	DriverID      string
	Balance       int64
	FrozenBalance int64
	TotalIncome   int64
	TotalWithdraw int64
}
