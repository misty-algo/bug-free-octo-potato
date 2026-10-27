package request

// CreateBillRequest 是 POST /api/v1/settlements/bills 使用的 JSON 载荷，金额单位分。
type CreateBillRequest struct {
	OrderNo         string `json:"order_no" binding:"required"`
	UserID          string `json:"user_id"`
	DriverID        string `json:"driver_id" binding:"required"`
	TotalAmount     int64  `json:"total_amount" binding:"required,gt=0"`
	PlatformSubsidy int64  `json:"platform_subsidy"`
	Remark          string `json:"remark"`
	// CommissionRateBP 建账时锁定的平台佣金率（万分比），如 2000 = 20%。
	CommissionRateBP int64 `json:"commission_rate_bp" binding:"min=0,max=10000"`
}

// SplitBillRequest 是 POST /api/v1/settlements/split 使用的 JSON 载荷。
// CommissionRateBps 为平台佣金比例（万分比），如 2000 = 20%。
type SplitBillRequest struct {
	OrderNo           string `json:"order_no" binding:"required"`
	CommissionRateBps int64  `json:"commission_rate_bps" binding:"min=0,max=10000"`
	PlatformSubsidy   int64  `json:"platform_subsidy"`
	// UseBillRate 为 true 时忽略 commission_rate_bps，按建账锁定的费率分账。
	UseBillRate bool `json:"use_bill_rate"`
}

// GetWalletBalanceRequest 是 GET /api/v1/settlements/wallets/:driver_id 使用的查询参数载荷。
type GetWalletBalanceRequest struct {
	DriverID string `json:"driver_id"`
}

// MarkBillPaidRequest 是 POST /api/v1/settlements/mark-bill-paid 使用的 JSON 载荷。
// BillNo 与 OrderNo 二选一；金额单位分，不传则沿用账单实付。
type MarkBillPaidRequest struct {
	BillNo      string `json:"bill_no"`
	OrderNo     string `json:"order_no" binding:"required_without=bill_no"`
	PayFee      int64  `json:"pay_fee"`
	DiscountFee int64  `json:"discount_fee"`
	RefundFee   int64  `json:"refund_fee"`
}

// ApplyRefundRequest 是 POST /api/v1/settlements/apply-refund 使用的 JSON 载荷。
// RefundNo 为退款单号（幂等记录），RefundAmount 为本次冲正金额（分）。
type ApplyRefundRequest struct {
	OrderNo      string `json:"order_no" binding:"required"`
	RefundNo     string `json:"refund_no" binding:"required"`
	RefundAmount int64  `json:"refund_amount" binding:"required,gt=0"`
}

// CheckUnpaidBanRequest 是 GET /api/v1/settlements/check-unpaid-ban/:user_id 使用的查询参数载荷。
type CheckUnpaidBanRequest struct {
	UserID string `json:"user_id"`
}

// VoidBillRequest 是 POST /api/v1/settlements/void-bill 使用的 JSON 载荷。
// BillNo 与 OrderNo 二选一。
type VoidBillRequest struct {
	BillNo  string `json:"bill_no"`
	OrderNo string `json:"order_no" binding:"required_without=bill_no"`
	Reason  string `json:"reason"`
}
