package model

import "time"

// BillDocument 是 bills 集合的 MongoDB 文档模型。
type BillDocument struct {
	ID              string `bson:"_id"` // 账单号
	OrderNo         string `bson:"order_no"`
	UserID          string `bson:"user_id"`
	DriverID        string `bson:"driver_id"`
	TotalAmount     int64  `bson:"total_amount"`
	PayFee          int64  `bson:"pay_fee"`
	DiscountFee     int64  `bson:"discount_fee"`
	RefundFee       int64  `bson:"refund_fee"`
	DriverIncome    int64  `bson:"driver_income"`
	CommissionFee   int64  `bson:"commission_fee"`
	SubsidyFee      int64  `bson:"subsidy_fee"`
	Settled         bool   `bson:"settled"`
	PlatformSubsidy int64  `bson:"platform_subsidy"`
	// CommissionRateBP 建账时锁定的佣金率，万分比（2000 = 20%）。
	CommissionRateBP int64     `bson:"commission_rate_bp"`
	Status           string    `bson:"status"`
	Remark           string    `bson:"remark"`
	CreatedAt        time.Time `bson:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

func (BillDocument) MongoCollectionName() string {
	return "bills"
}

// BillTransactionDocument 是 bill_transactions 集合的 MongoDB 文档模型（只追加不可变）。
type BillTransactionDocument struct {
	ID        string    `bson:"_id"` // 流水号 tx_no
	BillNo    string    `bson:"bill_no"`
	OrderNo   string    `bson:"order_no"`
	Category  string    `bson:"category"`
	OwnerType string    `bson:"owner_type"`
	OwnerID   string    `bson:"owner_id"`
	Amount    int64     `bson:"amount"`
	CreatedAt time.Time `bson:"created_at"`
}

func (BillTransactionDocument) MongoCollectionName() string {
	return "bill_transactions"
}

// WalletDocument 是 wallets 集合的 MongoDB 文档模型，_id 为司机 ID。
type WalletDocument struct {
	ID            string    `bson:"_id"` // driver_id
	DriverID      string    `bson:"driver_id"`
	Balance       int64     `bson:"balance"`
	FrozenBalance int64     `bson:"frozen_balance"`
	TotalIncome   int64     `bson:"total_income"`
	TotalWithdraw int64     `bson:"total_withdraw"`
	Version       int64     `bson:"version"`
	CreatedAt     time.Time `bson:"created_at"`
	UpdatedAt     time.Time `bson:"updated_at"`
}

func (WalletDocument) MongoCollectionName() string {
	return "wallets"
}
