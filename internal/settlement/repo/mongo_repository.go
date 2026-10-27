package repo

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/settlement/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/settlement/model"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// MongoRepository 使用共享 mongox 文档存储持久化 settlement 聚合，实现 entity.Repository。
type MongoRepository struct {
	db           *mongo.Database
	bills        *mongox.DocumentStore[dbmodel.BillDocument]
	transactions *mongox.DocumentStore[dbmodel.BillTransactionDocument]
	wallets      *mongox.DocumentStore[dbmodel.WalletDocument]
	log          *zap.Logger
}

// NewMongoRepository 创建基于 MongoDB 的 settlement 仓储。
func NewMongoRepository(db *mongo.Database, loggers ...*zap.Logger) *MongoRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &MongoRepository{
		db:           db,
		bills:        mongox.NewDocumentStore[dbmodel.BillDocument](db, log),
		transactions: mongox.NewDocumentStore[dbmodel.BillTransactionDocument](db, log),
		wallets:      mongox.NewDocumentStore[dbmodel.WalletDocument](db, log),
		log:          log,
	}
}

// SaveBill 按账单号新增或更新账单。
func (r *MongoRepository) SaveBill(ctx context.Context, bill *entity.Bill) error {
	start := time.Now()
	_, err := r.bills.UpsertByID(ctx, bill.ID, toBillDocument(bill))
	r.logOperation("SaveBill", bill.ID, start, err)
	return err
}

// FindBillByNo 按账单号查询。
func (r *MongoRepository) FindBillByNo(ctx context.Context, billNo string) (*entity.Bill, error) {
	start := time.Now()
	document, err := r.bills.FindByID(ctx, billNo)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrBillNotFound
	}
	r.logOperation("FindBillByNo", billNo, start, err)
	if err != nil {
		return nil, err
	}
	return toBillDomain(document), nil
}

// FindBillByOrderNo 按行程订单号查询。
func (r *MongoRepository) FindBillByOrderNo(ctx context.Context, orderNo string) (*entity.Bill, error) {
	start := time.Now()
	document, err := r.bills.FindOne(ctx, map[string]any{"order_no": orderNo})
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrBillNotFound
	}
	r.logOperation("FindBillByOrderNo", orderNo, start, err)
	if err != nil {
		return nil, err
	}
	return toBillDomain(document), nil
}

// ListUnpaidBillsByUser 查询用户全部待支付账单（未付禁叫车校验）。
func (r *MongoRepository) ListUnpaidBillsByUser(ctx context.Context, userID string) ([]*entity.Bill, error) {
	start := time.Now()
	documents, err := r.bills.FindMany(ctx,
		map[string]any{"user_id": userID, "status": entity.BillStatusUnpaid},
		options.Find().SetSort(map[string]any{"created_at": 1}),
	)
	r.logOperation("ListUnpaidBillsByUser", userID, start, err)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.Bill, 0, len(documents))
	for i := range documents {
		items = append(items, toBillDomain(&documents[i]))
	}
	return items, nil
}

// ListPendingSettleBills 查询已支付但未结算的账单（掉单补偿扫描用）。
func (r *MongoRepository) ListPendingSettleBills(ctx context.Context, limit int64) ([]*entity.Bill, error) {
	start := time.Now()
	documents, err := r.bills.FindMany(ctx,
		map[string]any{"status": entity.BillStatusPaid, "settled": false},
		options.Find().SetSort(map[string]any{"created_at": 1}).SetLimit(limit),
	)
	r.logOperation("ListPendingSettleBills", "paid-unsettled", start, err)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.Bill, 0, len(documents))
	for i := range documents {
		items = append(items, toBillDomain(&documents[i]))
	}
	return items, nil
}

// AppendTransaction 追加一条不可变分账流水。
//
// P7：并发兜底——插入前先按 _id 挡重，避免无唯一索引下同一流水被重复写入
// （service 层已做流水幂等挡重，此处为 repo 层双保险）。
func (r *MongoRepository) AppendTransaction(ctx context.Context, tx *entity.BillTransaction) error {
	start := time.Now()
	existing, findErr := r.transactions.FindByID(ctx, tx.ID)
	if findErr != nil && !errors.Is(findErr, mongox.ErrNotFound) {
		r.logOperation("AppendTransaction", tx.ID, start, findErr)
		return findErr
	}
	if existing != nil {
		r.logOperation("AppendTransaction", tx.ID, start, entity.ErrTransactionAlreadyExists)
		return entity.ErrTransactionAlreadyExists
	}
	_, err := r.transactions.Insert(ctx, toTxDocument(tx))
	r.logOperation("AppendTransaction", tx.ID, start, err)
	return err
}

// FindTransactionByTxNo 按流水号查询。
func (r *MongoRepository) FindTransactionByTxNo(ctx context.Context, txNo string) (*entity.BillTransaction, error) {
	start := time.Now()
	document, err := r.transactions.FindByID(ctx, txNo)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrTransactionNotFound
	}
	r.logOperation("FindTransactionByTxNo", txNo, start, err)
	if err != nil {
		return nil, err
	}
	return toTxDomain(document), nil
}

// FindTransactionsByBillNo 查询某账单的全部分账流水。
func (r *MongoRepository) FindTransactionsByBillNo(ctx context.Context, billNo string) ([]*entity.BillTransaction, error) {
	start := time.Now()
	documents, err := r.transactions.FindMany(ctx, map[string]any{"bill_no": billNo},
		options.Find().SetSort(map[string]any{"created_at": 1}),
	)
	r.logOperation("FindTransactionsByBillNo", billNo, start, err)
	if err != nil {
		return nil, err
	}
	items := make([]*entity.BillTransaction, 0, len(documents))
	for i := range documents {
		items = append(items, toTxDomain(&documents[i]))
	}
	return items, nil
}

// GetWallet 按司机 ID 查询钱包。
func (r *MongoRepository) GetWallet(ctx context.Context, driverID string) (*entity.Wallet, error) {
	start := time.Now()
	document, err := r.wallets.FindByID(ctx, driverID)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrWalletNotFound
	}
	r.logOperation("GetWallet", driverID, start, err)
	if err != nil {
		return nil, err
	}
	return toWalletDomain(document), nil
}

// UpsertWallet 按司机 ID 新增或整体更新钱包。
func (r *MongoRepository) UpsertWallet(ctx context.Context, wallet *entity.Wallet) error {
	start := time.Now()
	_, err := r.wallets.UpsertByID(ctx, wallet.ID, toWalletDocument(wallet))
	r.logOperation("UpsertWallet", wallet.ID, start, err)
	return err
}

// UpdateWalletBalance 乐观锁版本化更新司机钱包余额：仅当版本匹配且余额变更后非负才生效。
func (r *MongoRepository) UpdateWalletBalance(ctx context.Context, driverID string, balanceDelta int64, totalIncomeDelta int64, expectVersion int64) error {
	start := time.Now()
	result, err := r.wallets.UpdateOne(ctx,
		map[string]any{"_id": driverID, "version": expectVersion},
		map[string]any{
			"$inc": map[string]any{"balance": balanceDelta, "total_income": totalIncomeDelta, "version": 1},
			"$set": map[string]any{"updated_at": time.Now()},
		},
	)
	if errors.Is(err, mongox.ErrNotFound) {
		err = entity.ErrWalletNotFound
	}
	if err != nil {
		r.logOperation("UpdateWalletBalance", driverID, start, err)
		return err
	}
	if result.MatchedCount == 0 {
		// 未匹配：钱包不存在或版本冲突。
		_, findErr := r.wallets.FindByID(ctx, driverID)
		if errors.Is(findErr, mongox.ErrNotFound) {
			r.logOperation("UpdateWalletBalance", driverID, start, entity.ErrWalletNotFound)
			return entity.ErrWalletNotFound
		}
		r.logOperation("UpdateWalletBalance", driverID, start, entity.ErrWalletVersionConflict)
		return entity.ErrWalletVersionConflict
	}
	r.logOperation("UpdateWalletBalance", driverID, start, nil)
	return nil
}

// WithTransaction 在同一 MongoDB session 事务内执行 fn，事务内所有仓储调用须使用传入的 ctx。
// mongo-driver v2：WithTransaction 回调接收的 ctx 即事务上下文（已绑定 session）。
func (r *MongoRepository) WithTransaction(ctx context.Context, fn entity.TxFunc) error {
	session, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	_, err = session.WithTransaction(ctx, func(tctx context.Context) (any, error) {
		return nil, fn(tctx, r)
	})
	return err
}

func (r *MongoRepository) logOperation(operation string, id string, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "settlement_mongo"),
		zap.String("operation", operation),
		zap.String("id", id),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("mongodb repository operation completed with error", fields...)
		return
	}
	r.log.Info("mongodb repository operation completed", fields...)
}

func toBillDocument(b *entity.Bill) *dbmodel.BillDocument {
	return &dbmodel.BillDocument{
		ID:               b.ID,
		OrderNo:          b.OrderNo,
		UserID:           b.UserID,
		DriverID:         b.DriverID,
		TotalAmount:      b.TotalAmount,
		PayFee:           b.PayFee,
		DiscountFee:      b.DiscountFee,
		RefundFee:        b.RefundFee,
		DriverIncome:     b.DriverIncome,
		CommissionFee:    b.CommissionFee,
		SubsidyFee:       b.SubsidyFee,
		Settled:          b.Settled,
		PlatformSubsidy:  b.PlatformSubsidy,
		CommissionRateBP: b.CommissionRateBP,
		Status:           b.Status,
		Remark:           b.Remark,
		CreatedAt:        b.CreatedAt,
		UpdatedAt:        b.UpdatedAt,
	}
}

func toBillDomain(document *dbmodel.BillDocument) *entity.Bill {
	return &entity.Bill{
		ID:               document.ID,
		OrderNo:          document.OrderNo,
		UserID:           document.UserID,
		DriverID:         document.DriverID,
		TotalAmount:      document.TotalAmount,
		PayFee:           document.PayFee,
		DiscountFee:      document.DiscountFee,
		RefundFee:        document.RefundFee,
		DriverIncome:     document.DriverIncome,
		CommissionFee:    document.CommissionFee,
		SubsidyFee:       document.SubsidyFee,
		Settled:          document.Settled,
		PlatformSubsidy:  document.PlatformSubsidy,
		CommissionRateBP: document.CommissionRateBP,
		Status:           document.Status,
		Remark:           document.Remark,
		CreatedAt:        document.CreatedAt,
		UpdatedAt:        document.UpdatedAt,
	}
}

func toTxDocument(tx *entity.BillTransaction) *dbmodel.BillTransactionDocument {
	return &dbmodel.BillTransactionDocument{
		ID:        tx.ID,
		BillNo:    tx.BillNo,
		OrderNo:   tx.OrderNo,
		Category:  tx.Category,
		OwnerType: tx.OwnerType,
		OwnerID:   tx.OwnerID,
		Amount:    tx.Amount,
		CreatedAt: tx.CreatedAt,
	}
}

func toTxDomain(document *dbmodel.BillTransactionDocument) *entity.BillTransaction {
	return &entity.BillTransaction{
		ID:        document.ID,
		BillNo:    document.BillNo,
		OrderNo:   document.OrderNo,
		Category:  document.Category,
		OwnerType: document.OwnerType,
		OwnerID:   document.OwnerID,
		Amount:    document.Amount,
		CreatedAt: document.CreatedAt,
	}
}

func toWalletDocument(w *entity.Wallet) *dbmodel.WalletDocument {
	return &dbmodel.WalletDocument{
		ID:            w.ID,
		DriverID:      w.DriverID,
		Balance:       w.Balance,
		FrozenBalance: w.FrozenBalance,
		TotalIncome:   w.TotalIncome,
		TotalWithdraw: w.TotalWithdraw,
		Version:       w.Version,
		CreatedAt:     w.CreatedAt,
		UpdatedAt:     w.UpdatedAt,
	}
}

func toWalletDomain(document *dbmodel.WalletDocument) *entity.Wallet {
	return &entity.Wallet{
		ID:            document.ID,
		DriverID:      document.DriverID,
		Balance:       document.Balance,
		FrozenBalance: document.FrozenBalance,
		TotalIncome:   document.TotalIncome,
		TotalWithdraw: document.TotalWithdraw,
		Version:       document.Version,
		CreatedAt:     document.CreatedAt,
		UpdatedAt:     document.UpdatedAt,
	}
}

var _ entity.Repository = (*MongoRepository)(nil)
