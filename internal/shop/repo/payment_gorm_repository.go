package repo

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/shop/model"
)

// GormPaymentRepository 使用 Gorm 持久化支付单。
type GormPaymentRepository struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewGormPaymentRepository 创建支付单仓储。
func NewGormPaymentRepository(db *gorm.DB, loggers ...*zap.Logger) *GormPaymentRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &GormPaymentRepository{db: db, log: log}
}

// Create 落库支付单，order_no 唯一索引冲突时返回 ErrPaymentExists。
func (r *GormPaymentRepository) Create(ctx context.Context, p *entity.Payment) error {
	tx := r.db.WithContext(ctx).Create(toPaymentRecord(p))
	if tx.Error != nil {
		if errors.Is(tx.Error, gorm.ErrDuplicatedKey) {
			return entity.ErrPaymentExists
		}
		return tx.Error
	}
	return nil
}

// Update 更新支付单。
func (r *GormPaymentRepository) Update(ctx context.Context, p *entity.Payment) error {
	record := toPaymentRecord(p)
	tx := r.db.WithContext(ctx).Model(&dbmodel.PaymentModel{}).
		Where("order_no = ?", record.OrderNo).
		Updates(map[string]interface{}{
			"status":     record.Status,
			"channel":    record.Channel,
			"trade_no":   record.TradeNo,
			"paid_at":    record.PaidAt,
			"updated_at": record.UpdatedAt,
		})
	return tx.Error
}

// FindByOrderNo 按商户订单号查询支付单。
func (r *GormPaymentRepository) FindByOrderNo(ctx context.Context, orderNo string) (*entity.Payment, error) {
	var record dbmodel.PaymentModel
	tx := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&record)
	err := tx.Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return toPaymentDomain(&record), nil
}

// ListPendingBefore 返回创建时间早于 before 的 pending 支付单。
func (r *GormPaymentRepository) ListPendingBefore(ctx context.Context, before time.Time, limit int) ([]*entity.Payment, error) {
	var records []dbmodel.PaymentModel
	tx := r.db.WithContext(ctx).
		Where("status = ? AND created_at < ?", string(entity.PaymentStatusPending), before).
		Order("created_at asc").
		Limit(limit).
		Find(&records)
	if tx.Error != nil {
		return nil, tx.Error
	}
	items := make([]*entity.Payment, 0, len(records))
	for i := range records {
		items = append(items, toPaymentDomain(&records[i]))
	}
	return items, nil
}

func toPaymentRecord(p *entity.Payment) *dbmodel.PaymentModel {
	id := p.ID
	if id == "" {
		// shop 侧无独立支付单号概念，用 PAY-{order_no} 作为主键，
		// 与 payment 服务的 string 主键共用同一张 payments 表。
		id = "PAY-" + p.OrderNo
	}
	return &dbmodel.PaymentModel{
		ID:            id,
		OrderNo:       p.OrderNo,
		Amount:        p.Amount,
		Status:        string(p.Status),
		Channel:       string(p.Channel),
		TradeNo:       p.TradeNo,
		IdempotentKey: p.IdempotentKey,
		PaidAt:        p.PaidAt,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}

func toPaymentDomain(record *dbmodel.PaymentModel) *entity.Payment {
	return &entity.Payment{
		ID:            record.ID,
		OrderNo:       record.OrderNo,
		Amount:        record.Amount,
		Status:        entity.PaymentStatus(record.Status),
		Channel:       entity.PaymentChannel(record.Channel),
		TradeNo:       record.TradeNo,
		IdempotentKey: record.IdempotentKey,
		PaidAt:        record.PaidAt,
		CreatedAt:     record.CreatedAt,
		UpdatedAt:     record.UpdatedAt,
	}
}

var _ entity.PaymentRepository = (*GormPaymentRepository)(nil)
