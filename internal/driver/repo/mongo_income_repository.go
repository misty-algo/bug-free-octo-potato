package repo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
)

// ─────────────────────────── 流水 / 提现 ───────────────────────────

type incomeRepo struct {
	db  *mongo.Database
	log *zap.Logger
}

func (r *incomeRepo) flows() *mongo.Collection {
	return r.db.Collection(dbmodel.IncomeFlowCollectionName)
}

func (r *incomeRepo) withdrawals() *mongo.Collection {
	return r.db.Collection(dbmodel.WithdrawCollectionName)
}

func (r *incomeRepo) ledgers() *mongo.Collection {
	return r.db.Collection(dbmodel.LedgerCollectionName)
}

func (r *incomeRepo) CreateFlow(ctx context.Context, f *entity.IncomeFlow) error {
	_, err := r.flows().InsertOne(ctx, toFlowDocument(f))
	if mongo.IsDuplicateKeyError(err) {
		// event_id 唯一索引兜底：同一事件重复投递时不会重复记账。
		return entity.ErrDuplicateFlowEvent
	}
	logRepoOp(r.log, "income.CreateFlow", f.ID, err)
	return err
}

func (r *incomeRepo) GetFlowByEventID(ctx context.Context, eventID string) (*entity.IncomeFlow, error) {
	var doc dbmodel.IncomeFlowDocument
	err := r.flows().FindOne(ctx, bson.M{"event_id": eventID}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return flowFromDocument(&doc), nil
}

func (r *incomeRepo) GetFlowByID(ctx context.Context, id string) (*entity.IncomeFlow, error) {
	var doc dbmodel.IncomeFlowDocument
	err := r.flows().FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return flowFromDocument(&doc), nil
}

func (r *incomeRepo) GetFlowByKey(ctx context.Context, driverID, idempotencyKey string) (*entity.IncomeFlow, error) {
	var doc dbmodel.IncomeFlowDocument
	err := r.flows().FindOne(ctx, bson.M{"driver_id": driverID, "idempotency_key": idempotencyKey}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return flowFromDocument(&doc), nil
}

func (r *incomeRepo) ListFlows(ctx context.Context, driverID string, offset, limit int) ([]*entity.IncomeFlow, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	filter := bson.M{"driver_id": driverID}
	total, err := r.flows().CountDocuments(ctx, filter)
	if err != nil {
		logRepoOp(r.log, "income.ListFlows.count", driverID, err)
		return nil, 0, err
	}
	cur, err := r.flows().Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(offset)).SetLimit(int64(limit)))
	if err != nil {
		logRepoOp(r.log, "income.ListFlows", driverID, err)
		return nil, 0, err
	}
	defer cur.Close(ctx)
	items := make([]*entity.IncomeFlow, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.IncomeFlowDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		items = append(items, flowFromDocument(&doc))
	}
	return items, total, nil
}

// SumByAccountingDate 按记账日汇总正向收入与反向金额（退款/处罚为负）。
func (r *incomeRepo) SumByAccountingDate(ctx context.Context, driverID, dateKey string) (int64, int64, error) {
	cur, err := r.flows().Find(ctx, bson.M{"driver_id": driverID, "accounting_date": dateKey})
	if err != nil {
		logRepoOp(r.log, "income.SumByAccountingDate", driverID, err)
		return 0, 0, err
	}
	defer cur.Close(ctx)
	var incomeFen, refundFen int64
	for cur.Next(ctx) {
		var doc dbmodel.IncomeFlowDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		if doc.DriverIncomeFen >= 0 {
			incomeFen += doc.DriverIncomeFen
		} else {
			refundFen += -doc.DriverIncomeFen
		}
	}
	return incomeFen, refundFen, nil
}

// ListPendingSettlement 待结算流水（U 桶），供财务域按 T+1 确认结算。
func (r *incomeRepo) ListPendingSettlement(ctx context.Context, limit int) ([]*entity.IncomeFlow, error) {
	if limit <= 0 {
		limit = 100
	}
	cur, err := r.flows().Find(ctx, bson.M{"bucket_to": entity.BucketPending},
		options.Find().SetLimit(int64(limit)))
	if err != nil {
		logRepoOp(r.log, "income.ListPendingSettlement", "", err)
		return nil, err
	}
	defer cur.Close(ctx)
	items := make([]*entity.IncomeFlow, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.IncomeFlowDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		items = append(items, flowFromDocument(&doc))
	}
	return items, nil
}

// MarkSettled 将待结算流水标记为已结算（U → A 在仓储层同步改写记账桶标记）。
func (r *incomeRepo) MarkSettled(ctx context.Context, flowIDs []string) error {
	if len(flowIDs) == 0 {
		return nil
	}
	_, err := r.flows().UpdateMany(ctx, bson.M{"_id": bson.M{"$in": flowIDs}},
		bson.M{"$set": bson.M{"bucket_to": entity.BucketAvailable}})
	logRepoOp(r.log, "income.MarkSettled", "", err)
	return err
}

func (r *incomeRepo) CreateLedger(ctx context.Context, e *entity.LedgerEntry) error {
	_, err := r.ledgers().InsertOne(ctx, dbmodel.LedgerDocument{
		ID: e.ID, DriverID: e.DriverID, FromBucket: e.FromBucket, ToBucket: e.ToBucket,
		AmountFen: e.AmountFen, RefType: e.RefType, RefID: e.RefID,
		IdempotencyKey: e.IdempotencyKey, CreatedAt: e.CreatedAt,
	})
	if mongo.IsDuplicateKeyError(err) {
		return nil // 幂等键重复：分录已存在
	}
	logRepoOp(r.log, "income.CreateLedger", e.ID, err)
	return err
}

// CreateWithdraw 幂等创建提现申请：相同幂等键返回原单据。
func (r *incomeRepo) CreateWithdraw(ctx context.Context, w *entity.Withdrawal) (*entity.WithdrawCreateResult, error) {
	if w.IdempotencyKey != "" {
		var exist dbmodel.WithdrawDocument
		err := r.withdrawals().FindOne(ctx, bson.M{"driver_id": w.DriverID, "idempotency_key": w.IdempotencyKey}).Decode(&exist)
		if err == nil {
			return &entity.WithdrawCreateResult{Withdrawal: withdrawalFromDocument(&exist), Duplicate: true}, nil
		}
		if !isNoDocuments(err) {
			logRepoOp(r.log, "income.CreateWithdraw.get", w.DriverID, err)
			return nil, err
		}
	}
	_, err := r.withdrawals().InsertOne(ctx, toWithdrawDocument(w))
	if mongo.IsDuplicateKeyError(err) {
		var exist dbmodel.WithdrawDocument
		if e := r.withdrawals().FindOne(ctx, bson.M{"driver_id": w.DriverID, "idempotency_key": w.IdempotencyKey}).Decode(&exist); e == nil {
			return &entity.WithdrawCreateResult{Withdrawal: withdrawalFromDocument(&exist), Duplicate: true}, nil
		}
	}
	logRepoOp(r.log, "income.CreateWithdraw", w.ID, err)
	if err != nil {
		return nil, err
	}
	return &entity.WithdrawCreateResult{Withdrawal: w, Duplicate: false}, nil
}

// GetWithdrawByKey 按幂等键查询提现单（提现前预检，避免重复预占额度）。
func (r *incomeRepo) GetWithdrawByKey(ctx context.Context, driverID, idempotencyKey string) (*entity.Withdrawal, error) {
	if idempotencyKey == "" {
		return nil, entity.ErrWithdrawNotFound
	}
	var doc dbmodel.WithdrawDocument
	err := r.withdrawals().FindOne(ctx, bson.M{"driver_id": driverID, "idempotency_key": idempotencyKey}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrWithdrawNotFound
	}
	if err != nil {
		logRepoOp(r.log, "income.GetWithdrawByKey", driverID, err)
		return nil, err
	}
	return withdrawalFromDocument(&doc), nil
}

func (r *incomeRepo) GetWithdraw(ctx context.Context, id string) (*entity.Withdrawal, error) {
	var doc dbmodel.WithdrawDocument
	err := r.withdrawals().FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if isNoDocuments(err) {
		return nil, entity.ErrWithdrawNotFound
	}
	if err != nil {
		logRepoOp(r.log, "income.GetWithdraw", id, err)
		return nil, err
	}
	return withdrawalFromDocument(&doc), nil
}

func (r *incomeRepo) UpdateWithdraw(ctx context.Context, w *entity.Withdrawal) error {
	_, err := r.withdrawals().ReplaceOne(ctx, bson.M{"_id": w.ID}, toWithdrawDocument(w))
	logRepoOp(r.log, "income.UpdateWithdraw", w.ID, err)
	return err
}

func (r *incomeRepo) ListWithdraws(ctx context.Context, driverID string, offset, limit int) ([]*entity.Withdrawal, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	filter := bson.M{"driver_id": driverID}
	total, err := r.withdrawals().CountDocuments(ctx, filter)
	if err != nil {
		logRepoOp(r.log, "income.ListWithdraws.count", driverID, err)
		return nil, 0, err
	}
	cur, err := r.withdrawals().Find(ctx, filter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64(offset)).SetLimit(int64(limit)))
	if err != nil {
		logRepoOp(r.log, "income.ListWithdraws", driverID, err)
		return nil, 0, err
	}
	defer cur.Close(ctx)
	items := make([]*entity.Withdrawal, 0, limit)
	for cur.Next(ctx) {
		var doc dbmodel.WithdrawDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		items = append(items, withdrawalFromDocument(&doc))
	}
	return items, total, nil
}

// WithdrawTodayStats 当日已受理提现金额与次数（驳回不计次数）。
func (r *incomeRepo) WithdrawTodayStats(ctx context.Context, driverID string, dayStart time.Time) (int64, int64, error) {
	cur, err := r.withdrawals().Find(ctx, bson.M{
		"driver_id":  driverID,
		"created_at": bson.M{"$gte": dayStart},
		"status":     bson.M{"$ne": entity.WithdrawStatusRejected},
	})
	if err != nil {
		logRepoOp(r.log, "income.WithdrawTodayStats", driverID, err)
		return 0, 0, err
	}
	defer cur.Close(ctx)
	var amountFen, count int64
	for cur.Next(ctx) {
		var doc dbmodel.WithdrawDocument
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		amountFen += doc.AmountFen
		count++
	}
	return amountFen, count, nil
}

// ─────────────────────────── 文档转换 ───────────────────────────

func toFlowDocument(f *entity.IncomeFlow) dbmodel.IncomeFlowDocument {
	return dbmodel.IncomeFlowDocument{
		ID: f.ID, DriverID: f.DriverID, OrderID: f.OrderID, TaskID: f.TaskID,
		FlowType: f.FlowType, ReverseOf: f.ReverseOf,
		OrigFareFen: f.OrigFareFen, PassengerPaidFen: f.PassengerPaidFen,
		PlatformCouponFen: f.PlatformCouponFen, DriverCouponFen: f.DriverCouponFen,
		CommissionFen: f.CommissionFen, SubsidyFen: f.SubsidyFen, DriverIncomeFen: f.DriverIncomeFen,
		AccountingDate: f.AccountingDate, BucketFrom: f.BucketFrom, BucketTo: f.BucketTo,
		EventID: f.EventID, IdempotencyKey: f.IdempotencyKey, Reason: f.Reason, CreatedAt: f.CreatedAt,
	}
}

func flowFromDocument(doc *dbmodel.IncomeFlowDocument) *entity.IncomeFlow {
	return &entity.IncomeFlow{
		ID: doc.ID, DriverID: doc.DriverID, OrderID: doc.OrderID, TaskID: doc.TaskID,
		FlowType: doc.FlowType, ReverseOf: doc.ReverseOf,
		OrigFareFen: doc.OrigFareFen, PassengerPaidFen: doc.PassengerPaidFen,
		PlatformCouponFen: doc.PlatformCouponFen, DriverCouponFen: doc.DriverCouponFen,
		CommissionFen: doc.CommissionFen, SubsidyFen: doc.SubsidyFen, DriverIncomeFen: doc.DriverIncomeFen,
		AccountingDate: doc.AccountingDate, BucketFrom: doc.BucketFrom, BucketTo: doc.BucketTo,
		EventID: doc.EventID, IdempotencyKey: doc.IdempotencyKey, Reason: doc.Reason, CreatedAt: doc.CreatedAt,
	}
}

func toWithdrawDocument(w *entity.Withdrawal) dbmodel.WithdrawDocument {
	apps := make([]dbmodel.ApprovalDoc, 0, len(w.Approvals))
	for _, a := range w.Approvals {
		apps = append(apps, dbmodel.ApprovalDoc{ApproverID: a.ApproverID, Comment: a.Comment, ApprovedAt: a.ApprovedAt})
	}
	return dbmodel.WithdrawDocument{
		ID: w.ID, DriverID: w.DriverID, AmountFen: w.AmountFen, Status: w.Status,
		DualApproval: w.DualApproval, Approvals: apps,
		IdempotencyKey: w.IdempotencyKey, Reason: w.Reason,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

func withdrawalFromDocument(doc *dbmodel.WithdrawDocument) *entity.Withdrawal {
	apps := make([]entity.ApprovalRecord, 0, len(doc.Approvals))
	for _, a := range doc.Approvals {
		apps = append(apps, entity.ApprovalRecord{ApproverID: a.ApproverID, Comment: a.Comment, ApprovedAt: a.ApprovedAt})
	}
	return &entity.Withdrawal{
		ID: doc.ID, DriverID: doc.DriverID, AmountFen: doc.AmountFen, Status: doc.Status,
		DualApproval: doc.DualApproval, Approvals: apps,
		IdempotencyKey: doc.IdempotencyKey, Reason: doc.Reason,
		CreatedAt: doc.CreatedAt, UpdatedAt: doc.UpdatedAt,
	}
}
