package model

import "time"

const paymentsMongoCollectionName = "payments"

// PaymentsDocument 是 payments 集合（支付单）的 MongoDB 文档模型。
type PaymentsDocument struct {
	ID              string                 `bson:"_id"`
	PayNo           string                 `bson:"pay_no"`
	OrderID         string                 `bson:"order_id"`
	BillID          string                 `bson:"bill_id"`
	UserID          string                 `bson:"user_id"`
	Amount          int64                  `bson:"amount"`
	Channel         string                 `bson:"channel"`
	PayScene        string                 `bson:"pay_scene"`
	Status          string                 `bson:"status"`
	IdempotentKey   string                 `bson:"idempotent_key"`
	ChannelTradeNo  string                 `bson:"channel_trade_no"`
	ChannelResponse map[string]interface{} `bson:"channel_response"`
	PrepayID        string                 `bson:"prepay_id"`
	PaidAt          time.Time              `bson:"paid_at"`
	ExpireAt        time.Time              `bson:"expire_at"`
	RefundedAmount  int64                  `bson:"refunded_amount"`
	FailReason      string                 `bson:"fail_reason"`
	NotifyCount     int64                  `bson:"notify_count"`
	Version         int64                  `bson:"version"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (PaymentsDocument) MongoCollectionName() string {
	return paymentsMongoCollectionName
}

const refundsMongoCollectionName = "refunds"

// RefundsDocument 是 refunds 集合（退款单）的 MongoDB 文档模型。
type RefundsDocument struct {
	ID              string    `bson:"_id"`
	RefundNo        string    `bson:"refund_no"`
	PaymentID       string    `bson:"payment_id"`
	OrderID         string    `bson:"order_id"`
	BillID          string    `bson:"bill_id"`
	Amount          int64     `bson:"amount"`
	RefundType      string    `bson:"refund_type"`
	ReasonCode      string    `bson:"reason_code"`
	ReasonText      string    `bson:"reason_text"`
	RefundTo        string    `bson:"refund_to"`
	Status          string    `bson:"status"`
	ChannelRefundNo string    `bson:"channel_refund_no"`
	ApplicantType   string    `bson:"applicant_type"`
	ApplicantID     string    `bson:"applicant_id"`
	ApproverID      string    `bson:"approver_id"`
	CouponReturned  bool      `bson:"coupon_returned"`
	RefundedAt      time.Time `bson:"refunded_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (RefundsDocument) MongoCollectionName() string {
	return refundsMongoCollectionName
}

const driverWalletsMongoCollectionName = "driver_wallets"

// DriverWalletsDocument 是 driver_wallets 集合（司机钱包）的 MongoDB 文档模型。
type DriverWalletsDocument struct {
	ID            string    `bson:"_id"`
	DriverID      string    `bson:"driver_id"`
	Balance       int64     `bson:"balance"`
	FrozenBalance int64     `bson:"frozen_balance"`
	TotalIncome   int64     `bson:"total_income"`
	TotalWithdraw int64     `bson:"total_withdraw"`
	TodayIncome   int64     `bson:"today_income"`
	SettleStatus  string    `bson:"settle_status"`
	Version       int64     `bson:"version"`
	LastSettleAt  time.Time `bson:"last_settle_at"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (DriverWalletsDocument) MongoCollectionName() string {
	return driverWalletsMongoCollectionName
}

const walletTransactionsMongoCollectionName = "wallet_transactions"

// WalletTransactionsDocument 是 wallet_transactions 集合（钱包流水）的 MongoDB 文档模型。
type WalletTransactionsDocument struct {
	ID           string `bson:"_id"`
	TxNo         string `bson:"tx_no"`
	OwnerType    string `bson:"owner_type"`
	OwnerID      string `bson:"owner_id"`
	Direction    string `bson:"direction"`
	Amount       int64  `bson:"amount"`
	BalanceAfter int64  `bson:"balance_after"`
	BizType      string `bson:"biz_type"`
	BizID        string `bson:"biz_id"`
	OrderID      string `bson:"order_id"`
	Description  string `bson:"description"`
	OperatorID   string `bson:"operator_id"`
	Status       string `bson:"status"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (WalletTransactionsDocument) MongoCollectionName() string {
	return walletTransactionsMongoCollectionName
}

const withdrawalsMongoCollectionName = "withdrawals"

// WithdrawalsDocument 是 withdrawals 集合（提现单）的 MongoDB 文档模型。
type WithdrawalsDocument struct {
	ID             string    `bson:"_id"`
	WithdrawNo     string    `bson:"withdraw_no"`
	DriverID       string    `bson:"driver_id"`
	Amount         int64     `bson:"amount"`
	Fee            int64     `bson:"fee"`
	ActualAmount   int64     `bson:"actual_amount"`
	BankCardEnc    string    `bson:"bank_card_enc"`
	BankName       string    `bson:"bank_name"`
	AccountName    string    `bson:"account_name"`
	Status         string    `bson:"status"`
	FreeQuotaUsed  bool      `bson:"free_quota_used"`
	RiskChecked    bool      `bson:"risk_checked"`
	RiskRemark     string    `bson:"risk_remark"`
	AuditorID      string    `bson:"auditor_id"`
	AuditedAt      time.Time `bson:"audited_at"`
	RejectReason   string    `bson:"reject_reason"`
	ChannelTradeNo string    `bson:"channel_trade_no"`
	PaidAt         time.Time `bson:"paid_at"`
	FailReason     string    `bson:"fail_reason"`

	// 16.6 增补字段
	RiskLevel    string `bson:"risk_level"`
	AppealStatus string `bson:"appeal_status"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (WithdrawalsDocument) MongoCollectionName() string {
	return withdrawalsMongoCollectionName
}

const invoicesMongoCollectionName = "invoices"

// InvoicesDocument 是 invoices 集合（发票）的 MongoDB 文档模型。
type InvoicesDocument struct {
	ID             string    `bson:"_id"`
	InvoiceNo      string    `bson:"invoice_no"`
	UserID         string    `bson:"user_id"`
	OrderIds       []string  `bson:"order_ids"`
	Amount         int64     `bson:"amount"`
	InvoiceType    string    `bson:"invoice_type"`
	TitleType      string    `bson:"title_type"`
	Title          string    `bson:"title"`
	TaxNo          string    `bson:"tax_no"`
	CompanyAddress string    `bson:"company_address"`
	CompanyPhone   string    `bson:"company_phone"`
	BankName       string    `bson:"bank_name"`
	BankAccount    string    `bson:"bank_account"`
	Email          string    `bson:"email"`
	Status         string    `bson:"status"`
	InvoiceFileURL string    `bson:"invoice_file_url"`
	IssuedAt       time.Time `bson:"issued_at"`
	FailReason     string    `bson:"fail_reason"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (InvoicesDocument) MongoCollectionName() string {
	return invoicesMongoCollectionName
}
