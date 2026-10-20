package repo

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	dbmodel "github.com/BwCloudWeGo/bw-cli/internal/driver/model"
)

// GormRepository 使用 Gorm 持久化 driver 聚合。
type GormRepository struct {
	db  *gorm.DB
	log *zap.Logger
}

// NewGormRepository 创建 driver 仓储，并支持可选结构化日志。
func NewGormRepository(db *gorm.DB, loggers ...*zap.Logger) *GormRepository {
	log := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		log = loggers[0]
	}
	return &GormRepository{db: db, log: log}
}

// AutoMigrate 创建或更新 drivers 表结构。
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(&dbmodel.DriverModel{})
}

// Save 新增或更新 driver 聚合。
func (r *GormRepository) Save(ctx context.Context, item *entity.Driver) error {
	start := time.Now()
	tx := r.db.WithContext(ctx).Save(toRecord(item))
	r.logOperation("Save", tx.RowsAffected, start, tx.Error)
	return tx.Error
}

// FindByID 根据 ID 加载 driver 聚合。
func (r *GormRepository) FindByID(ctx context.Context, id string) (*entity.Driver, error) {
	start := time.Now()
	var record dbmodel.DriverModel
	tx := r.db.WithContext(ctx).Where("id = ?", id).First(&record)
	err := tx.Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = entity.ErrDriverNotFound
	}
	if err != nil {
		r.logOperation("FindByID", tx.RowsAffected, start, err)
		return nil, err
	}
	r.logOperation("FindByID", tx.RowsAffected, start, nil)
	return toDomain(&record), nil
}

// List 加载分页 driver 聚合。
func (r *GormRepository) List(ctx context.Context, offset int, limit int) ([]*entity.Driver, int64, error) {
	start := time.Now()
	var total int64
	countTx := r.db.WithContext(ctx).Model(&dbmodel.DriverModel{}).Count(&total)
	if countTx.Error != nil {
		r.logOperation("Count", countTx.RowsAffected, start, countTx.Error)
		return nil, 0, countTx.Error
	}
	var records []dbmodel.DriverModel
	tx := r.db.WithContext(ctx).
		Order("created_at desc").
		Offset(offset).
		Limit(limit).
		Find(&records)
	if tx.Error != nil {
		r.logOperation("List", tx.RowsAffected, start, tx.Error)
		return nil, 0, tx.Error
	}
	items := make([]*entity.Driver, 0, len(records))
	for i := range records {
		items = append(items, toDomain(&records[i]))
	}
	r.logOperation("List", tx.RowsAffected, start, nil)
	return items, total, nil
}

// Delete 根据 ID 删除 driver 聚合。
func (r *GormRepository) Delete(ctx context.Context, id string) error {
	start := time.Now()
	tx := r.db.WithContext(ctx).Where("id = ?", id).Delete(&dbmodel.DriverModel{})
	err := tx.Error
	if err == nil && tx.RowsAffected == 0 {
		err = entity.ErrDriverNotFound
	}
	r.logOperation("Delete", tx.RowsAffected, start, err)
	return err
}

func (r *GormRepository) logOperation(operation string, rows int64, start time.Time, err error) {
	fields := []zap.Field{
		zap.String("repository", "driver"),
		zap.String("operation", operation),
		zap.Int64("rows_affected", rows),
		zap.Float64("latency_ms", float64(time.Since(start).Microseconds())/1000),
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		r.log.Warn("repository operation completed with error", fields...)
		return
	}
	r.log.Info("repository operation completed", fields...)
}

func toRecord(item *entity.Driver) *dbmodel.DriverModel {
	return &dbmodel.DriverModel{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func toDomain(record *dbmodel.DriverModel) *entity.Driver {
	return &entity.Driver{
		ID:          record.ID,
		Name:        record.Name,
		Description: record.Description,
		CreatedAt:   record.CreatedAt,
		UpdatedAt:   record.UpdatedAt,
	}
}

var _ entity.Repository = (*GormRepository)(nil)
