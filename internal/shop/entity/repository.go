package entity

import "context"

// Repository 定义 shop service 层需要的持久化行为。
type Repository interface {
	Save(ctx context.Context, item *Shop) error
	FindByID(ctx context.Context, id string) (*Shop, error)
	List(ctx context.Context, offset int, limit int) ([]*Shop, int64, error)
	Delete(ctx context.Context, id string) error
}
