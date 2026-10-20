package entity

import "context"

// Repository 定义 driver service 层需要的持久化行为。
type Repository interface {
	Save(ctx context.Context, item *Driver) error
	FindByID(ctx context.Context, id string) (*Driver, error)
	List(ctx context.Context, offset int, limit int) ([]*Driver, int64, error)
	Delete(ctx context.Context, id string) error
}
