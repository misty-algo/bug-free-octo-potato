package repo

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/BwCloudWeGo/bw-cli/internal/payment/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/payment/model"
)

// GormRepository 使用 MySQL Gorm 持久化 payment 聚合，实现 entity.Repository。
type GormRepository struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewGormRepository 创建基于 Gorm 的 payment 仓储。
func NewGormRepository(db *gorm.DB, loggers ...*zap.Logger) *GormRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &GormRepository{db: db, log: log}
}

// AutoMigrate 迁移 payments、payment_events 与 refunds 表（包级入口，供 cmd 调用）。
func AutoMigrate(db *gorm.DB) error {
	return NewGormRepository(db).AutoMigrate()
}

// AutoMigrate 迁移 payments、payment_events 与 refunds 表。
func (r *GormRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&dbmodel.PaymentModel{}, &dbmodel.PaymentEventModel{}, &dbmodel.RefundModel{})
}

// SavePayment 新增或更新支付单。
//
// P11：不再使用 GORM Save 全量 upsert（会绕过 order_no 唯一约束保护）。
// 不存在则显式 Insert（并发撞 order_no 唯一键直接报错，交由调用方幂等处理）；
// 存在则仅更新状态相关字段，保留创建时不可变字段（order_no/user_id/amount/idempotent_key）。
func (r *GormRepository) SavePayment(ctx context.Context, payment *entity.Payment) error {
	start := time.Now()
	record := toRecord(payment)

	var count int64
	if err := r.db.WithContext(ctx).Model(&dbmodel.PaymentModel{}).Where("id = ?", record.ID).Count(&count).Error; err != nil {
		r.logOperation("SavePayment", payment.ID, start, err)
		return err
	}
	var err error
	if count == 0 {
		err = r.db.WithContext(ctx).Create(record).Error
	} else {
		err = r.db.WithContext(ctx).Model(&dbmodel.PaymentModel{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status":          record.Status,
			"paid_amount":     record.PaidAmount,
			"trade_no":        record.TradeNo,
			"channel":         record.Channel,
			"prepay_id":       record.PrepayID,
			"expire_at":       record.ExpireAt,
			"fail_reason":     record.FailReason,
			"notify_count":    record.NotifyCount,
			"paid_at":         record.PaidAt,
			"refunded_amount": record.RefundedAmount,
			"updated_at":      record.UpdatedAt,
		}).Error
	}
	r.logOperation("SavePayment", payment.ID, start, err)
	return err
}

// FindPaymentByOrderNo 按业务订单号查询。
func (r *GormRepository) FindPaymentByOrderNo(ctx context.Context, orderNo string) (*entity.Payment, error) {
	var record dbmodel.PaymentModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrPaymentNotFound
	}
	r.logOperation("FindPaymentByOrderNo", orderNo, start, err)
	if err != nil {
		return nil, err
	}
	return toDomain(&record), nil
}

// FindPaymentByPaymentNo 按支付单号查询。
func (r *GormRepository) FindPaymentByPaymentNo(ctx context.Context, paymentNo string) (*entity.Payment, error) {
	var record dbmodel.PaymentModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("id = ?", paymentNo).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrPaymentNotFound
	}
	r.logOperation("FindPaymentByPaymentNo", paymentNo, start, err)
	if err != nil {
		return nil, err
	}
	return toDomain(&record), nil
}

// FindPaymentByIdempotentKey 按创建幂等键查询。
func (r *GormRepository) FindPaymentByIdempotentKey(ctx context.Context, key string) (*entity.Payment, error) {
	var record dbmodel.PaymentModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("idempotent_key = ?", key).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrPaymentNotFound
	}
	r.logOperation("FindPaymentByIdempotentKey", key, start, err)
	if err != nil {
		return nil, err
	}
	return toDomain(&record), nil
}

// SaveEvent 保存待处理支付事件。
func (r *GormRepository) SaveEvent(ctx context.Context, event *entity.PaymentEvent) error {
	start := time.Now()
	record := toEventRecord(event)
	err := r.db.WithContext(ctx).Create(record).Error
	r.logOperation("SaveEvent", event.ID, start, err)
	return err
}

// FindEventByOrderNoAndType 查询订单指定类型事件（幂等判重）。
func (r *GormRepository) FindEventByOrderNoAndType(ctx context.Context, orderNo string, eventType string) (*entity.PaymentEvent, error) {
	var record dbmodel.PaymentEventModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("order_no = ? AND event_type = ?", orderNo, eventType).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrEventNotFound
	}
	r.logOperation("FindEventByOrderNoAndType", orderNo, start, err)
	if err != nil {
		return nil, err
	}
	return toEventDomain(&record), nil
}

// UpdateEventStatus 更新事件状态与重试次数。
func (r *GormRepository) UpdateEventStatus(ctx context.Context, eventNo string, status string, retryCount int) error {
	start := time.Now()
	err := r.db.WithContext(ctx).Model(&dbmodel.PaymentEventModel{}).
		Where("id = ?", eventNo).
		Updates(map[string]any{"status": status, "retry_count": retryCount, "updated_at": time.Now()}).Error
	r.logOperation("UpdateEventStatus", eventNo, start, err)
	return err
}

// ListPendingEvents 列出待处理事件。
func (r *GormRepository) ListPendingEvents(ctx context.Context, limit int) ([]*entity.PaymentEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var records []dbmodel.PaymentEventModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("status = ?", entity.EventStatusPending).
		Order("created_at asc").Limit(limit).Find(&records).Error
	r.logOperation("ListPendingEvents", "", start, err)
	if err != nil {
		return nil, err
	}
	events := make([]*entity.PaymentEvent, 0, len(records))
	for i := range records {
		events = append(events, toEventDomain(&records[i]))
	}
	return events, nil
}

// SaveRefund 新增或更新退款单。
//
// P11：不再使用 GORM Save 全量 upsert（会绕过 out_request_no 唯一约束保护）。
// 不存在则显式 Insert；存在则仅更新状态相关字段。
func (r *GormRepository) SaveRefund(ctx context.Context, refund *entity.Refund) error {
	start := time.Now()
	record := toRefundRecord(refund)

	var count int64
	if err := r.db.WithContext(ctx).Model(&dbmodel.RefundModel{}).Where("id = ?", record.ID).Count(&count).Error; err != nil {
		r.logOperation("SaveRefund", refund.ID, start, err)
		return err
	}
	var err error
	if count == 0 {
		err = r.db.WithContext(ctx).Create(record).Error
	} else {
		err = r.db.WithContext(ctx).Model(&dbmodel.RefundModel{}).Where("id = ?", record.ID).Updates(map[string]any{
			"status":           record.Status,
			"channel_trade_no": record.ChannelTradeNo,
			"reject_reason":    record.RejectReason,
			"refunded_at":      record.RefundedAt,
			"updated_at":       record.UpdatedAt,
		}).Error
	}
	r.logOperation("SaveRefund", refund.ID, start, err)
	return err
}

// FindRefundByRefundNo 按退款单号查询。
func (r *GormRepository) FindRefundByRefundNo(ctx context.Context, refundNo string) (*entity.Refund, error) {
	var record dbmodel.RefundModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("id = ?", refundNo).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrRefundNotFound
	}
	r.logOperation("FindRefundByRefundNo", refundNo, start, err)
	if err != nil {
		return nil, err
	}
	return toRefundDomain(&record), nil
}

// FindRefundByOutRequestNo 按渠道退款请求号查询（幂等锚点）。
func (r *GormRepository) FindRefundByOutRequestNo(ctx context.Context, outRequestNo string) (*entity.Refund, error) {
	var record dbmodel.RefundModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("out_request_no = ?", outRequestNo).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, entity.ErrRefundNotFound
	}
	r.logOperation("FindRefundByOutRequestNo", outRequestNo, start, err)
	if err != nil {
		return nil, err
	}
	return toRefundDomain(&record), nil
}

// ListRefundsByOrderNo 按业务订单号列出退款单。
func (r *GormRepository) ListRefundsByOrderNo(ctx context.Context, orderNo string) ([]*entity.Refund, error) {
	var records []dbmodel.RefundModel
	start := time.Now()
	err := r.db.WithContext(ctx).Where("order_no = ?", orderNo).Order("created_at asc").Find(&records).Error
	r.logOperation("ListRefundsByOrderNo", orderNo, start, err)
	if err != nil {
		return nil, err
	}
	refunds := make([]*entity.Refund, 0, len(records))
	for i := range records {
		refunds = append(refunds, toRefundDomain(&records[i]))
	}
	return refunds, nil
}

// AddRefundedAmount 原子累加累计退款金额：仅在未超实付金额时执行更新。
// 并发退款场景下避免"读-改-写"互相覆盖（P1：退款幂等+防超退）。
func (r *GormRepository) AddRefundedAmount(ctx context.Context, paymentNo string, amount int64) error {
	start := time.Now()
	res := r.db.WithContext(ctx).Model(&dbmodel.PaymentModel{}).
		Where("id = ? AND refunded_amount + ? <= paid_amount", paymentNo, amount).
		Updates(map[string]any{
			"refunded_amount": gorm.Expr("refunded_amount + ?", amount),
			"updated_at":      time.Now(),
		})
	if res.Error != nil {
		r.logOperation("AddRefundedAmount", paymentNo, start, res.Error)
		return res.Error
	}
	r.logOperation("AddRefundedAmount", paymentNo, start, nil)
	if res.RowsAffected > 0 {
		return nil
	}
	// 无行更新：支付单不存在或累加后会超退，回查区分。
	var count int64
	if err := r.db.WithContext(ctx).Model(&dbmodel.PaymentModel{}).
		Where("id = ?", paymentNo).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return entity.ErrPaymentNotFound
	}
	return entity.ErrRefundExceedsPaid
}

func (r *GormRepository) logOperation(operation string, id string, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "payment_gorm"),
		zap.String("operation", operation),
		zap.String("id", id),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("gorm repository operation completed with error", fields...)
		return
	}
	r.log.Info("gorm repository operation completed", fields...)
}

func toRecord(p *entity.Payment) *dbmodel.PaymentModel {
	return &dbmodel.PaymentModel{
		ID:             p.ID,
		OrderNo:        p.OrderNo,
		UserID:         p.UserID,
		Subject:        p.Subject,
		Amount:         p.Amount,
		PaidAmount:     p.PaidAmount,
		Status:         p.Status,
		Channel:        p.Channel,
		TradeNo:        p.TradeNo,
		IdempotentKey:  p.IdempotentKey,
		RefundedAmount: p.RefundedAmount,
		PrepayID:       p.PrepayID,
		ExpireAt:       p.ExpireAt,
		FailReason:     p.FailReason,
		NotifyCount:    p.NotifyCount,
		PaidAt:         p.PaidAt,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func toDomain(record *dbmodel.PaymentModel) *entity.Payment {
	return &entity.Payment{
		ID:             record.ID,
		OrderNo:        record.OrderNo,
		UserID:         record.UserID,
		Subject:        record.Subject,
		Amount:         record.Amount,
		PaidAmount:     record.PaidAmount,
		Status:         record.Status,
		Channel:        record.Channel,
		TradeNo:        record.TradeNo,
		IdempotentKey:  record.IdempotentKey,
		RefundedAmount: record.RefundedAmount,
		PrepayID:       record.PrepayID,
		ExpireAt:       record.ExpireAt,
		FailReason:     record.FailReason,
		NotifyCount:    record.NotifyCount,
		PaidAt:         record.PaidAt,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func toEventRecord(e *entity.PaymentEvent) *dbmodel.PaymentEventModel {
	return &dbmodel.PaymentEventModel{
		ID:         e.ID,
		OrderNo:    e.OrderNo,
		EventType:  e.EventType,
		Payload:    e.Payload,
		Status:     e.Status,
		RetryCount: e.RetryCount,
		CreatedAt:  e.CreatedAt,
		UpdatedAt:  e.UpdatedAt,
	}
}

func toEventDomain(record *dbmodel.PaymentEventModel) *entity.PaymentEvent {
	return &entity.PaymentEvent{
		ID:         record.ID,
		OrderNo:    record.OrderNo,
		EventType:  record.EventType,
		Payload:    record.Payload,
		Status:     record.Status,
		RetryCount: record.RetryCount,
		CreatedAt:  record.CreatedAt,
		UpdatedAt:  record.UpdatedAt,
	}
}

func toRefundRecord(r *entity.Refund) *dbmodel.RefundModel {
	return &dbmodel.RefundModel{
		ID:             r.ID,
		PaymentNo:      r.PaymentNo,
		OrderNo:        r.OrderNo,
		UserID:         r.UserID,
		Amount:         r.Amount,
		Status:         r.Status,
		OutRequestNo:   r.OutRequestNo,
		ChannelTradeNo: r.ChannelTradeNo,
		Reason:         r.Reason,
		RejectReason:   r.RejectReason,
		RefundedAt:     r.RefundedAt,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
	}
}

func toRefundDomain(record *dbmodel.RefundModel) *entity.Refund {
	return &entity.Refund{
		ID:             record.ID,
		PaymentNo:      record.PaymentNo,
		OrderNo:        record.OrderNo,
		UserID:         record.UserID,
		Amount:         record.Amount,
		Status:         record.Status,
		OutRequestNo:   record.OutRequestNo,
		ChannelTradeNo: record.ChannelTradeNo,
		Reason:         record.Reason,
		RejectReason:   record.RejectReason,
		RefundedAt:     record.RefundedAt,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

var _ entity.Repository = (*GormRepository)(nil)
