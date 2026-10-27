package entity

import "context"

// Repository 是 settlement 聚合的持久化端口（MongoDB）。
type Repository interface {
	// SaveBill 新增或更新账单。
	SaveBill(ctx context.Context, bill *Bill) error
	// FindBillByNo 按账单号查询。
	FindBillByNo(ctx context.Context, billNo string) (*Bill, error)
	// FindBillByOrderNo 按行程订单号查询。
	FindBillByOrderNo(ctx context.Context, orderNo string) (*Bill, error)
	// ListUnpaidBillsByUser 查询用户全部待支付账单（未付禁叫车校验）。
	ListUnpaidBillsByUser(ctx context.Context, userID string) ([]*Bill, error)

	// AppendTransaction 追加一条不可变分账流水。
	AppendTransaction(ctx context.Context, tx *BillTransaction) error
	// FindTransactionByTxNo 按流水号查询（分账幂等判重锚点）。
	FindTransactionByTxNo(ctx context.Context, txNo string) (*BillTransaction, error)
	// FindTransactionsByBillNo 查询某账单的全部分账流水。
	FindTransactionsByBillNo(ctx context.Context, billNo string) ([]*BillTransaction, error)

	// GetWallet 按司机 ID 查询钱包。
	GetWallet(ctx context.Context, driverID string) (*Wallet, error)
	// UpsertWallet 新增或整体更新钱包。
	UpsertWallet(ctx context.Context, wallet *Wallet) error
	// UpdateWalletBalance 乐观锁版本化更新司机钱包余额：条件 Version 匹配且新余额非负。
	// 返回 ErrWalletNotFound / ErrWalletVersionConflict。
	UpdateWalletBalance(ctx context.Context, driverID string, balanceDelta int64, totalIncomeDelta int64, expectVersion int64) error
}

// TxFunc 事务函数：仓储在事务内执行的更新逻辑。
type TxFunc func(ctx context.Context, repo Repository) error

// WithTransaction 在同一事务内执行 fn，事务内只能使用传入的 repo（同一连接）。
type WithTransaction func(ctx context.Context, fn TxFunc) error
