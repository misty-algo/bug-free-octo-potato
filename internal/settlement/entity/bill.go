package entity

import (
	"errors"
	"time"
)

// 账单状态（对齐权威 schema BILL_STATE）。
const (
	BillStatusCalculating = "CALCULATING" // 计费中（未出账）
	BillStatusUnpaid      = "UNPAID"      // 已出账待支付
	BillStatusPaid        = "PAID"        // 已支付
	BillStatusVoid        = "VOID"        // 已作废（取消/关单）
)

// 分账流水分类。
const (
	TxCategoryPlatformCommission = "platform_commission" // 平台佣金
	TxCategoryDriverIncome       = "driver_income"       // 司机收入
	TxCategoryPlatformSubsidy    = "platform_subsidy"    // 平台补贴
	TxCategoryRefundReversal     = "refund_reversal"     // 退款冲正（退款后回冲司机入账）
)

// 流水 owner 类型。
const (
	OwnerTypePlatform = "platform"
	OwnerTypeDriver   = "driver"
)

// Bill 乘客账单（不可变，只追加分账流水）。
type Bill struct {
	ID              string // 账单号 bill_no
	OrderNo         string // 行程订单号
	UserID          string
	DriverID        string
	TotalAmount     int64 // 乘客实付（分），同 payFee
	PayFee          int64 // 乘客实付（分）
	DiscountFee     int64 // 优惠总额（分）
	RefundFee       int64 // 已退金额（分）
	DriverIncome    int64 // 司机应得（分）
	CommissionFee   int64 // 平台佣金（分）
	SubsidyFee      int64 // 平台补贴（分）
	Settled         bool  // 是否已结算给司机
	PlatformSubsidy int64 // 平台补贴（分，兼容旧字段）
	Status          string
	Remark          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// MarkPaid 支付成功：UNPAID → PAID，已是 PAID 幂等通过。
func (b *Bill) MarkPaid(now time.Time) error {
	switch b.Status {
	case BillStatusPaid:
		return nil
	case BillStatusUnpaid:
		b.Status = BillStatusPaid
		b.UpdatedAt = now
		return nil
	default:
		return ErrBillStateTransition
	}
}

// MarkVoid 作废：CALCULATING/UNPAID → VOID，已是 VOID 幂等通过。
func (b *Bill) MarkVoid(now time.Time) error {
	switch b.Status {
	case BillStatusVoid:
		return nil
	case BillStatusCalculating, BillStatusUnpaid:
		b.Status = BillStatusVoid
		b.UpdatedAt = now
		return nil
	default:
		return ErrBillStateTransition
	}
}

// BillTransaction 账单流水（只追加不可变）。
type BillTransaction struct {
	ID        string // 流水号 tx_no
	BillNo    string
	OrderNo   string
	Category  string // platform_commission / driver_income / platform_subsidy
	OwnerType string
	OwnerID   string
	Amount    int64 // 金额（分）
	CreatedAt time.Time
}

// Wallet 司机钱包。
type Wallet struct {
	ID            string // _id = driver_id
	DriverID      string
	Balance       int64 // 可用余额（分）
	FrozenBalance int64 // 冻结金额（分）
	TotalIncome   int64 // 累计入账（分）
	TotalWithdraw int64 // 累计出账/提现（分）
	Version       int64 // 乐观锁版本
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// 领域错误。
var (
	ErrBillNotFound            = errors.New("bill not found")
	ErrBillAlreadyExists       = errors.New("bill already exists")
	ErrBillAlreadySplit        = errors.New("bill already split")
	ErrBillStateTransition     = errors.New("invalid bill state transition")
	ErrInvalidSplitRequest     = errors.New("invalid split request")
	ErrWalletNotFound          = errors.New("wallet not found")
	ErrWalletVersionConflict   = errors.New("wallet version conflict")
	ErrTransactionNotFound     = errors.New("bill transaction not found")
	ErrTransactionAlreadyExists = errors.New("bill transaction already exists")
	ErrRefundSettlementExceeds = errors.New("refund amount exceeds driver settled income")
)
