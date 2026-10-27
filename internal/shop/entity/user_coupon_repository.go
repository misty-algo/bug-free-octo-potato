package entity

import (
	"context"
	"time"
)

// UserCouponRepository 定义用户券（P14）的持久化行为。
// user_coupons 是并发写集合：Save 携带 version 乐观锁；couponNo 唯一。
type UserCouponRepository interface {
	// Save 新增或更新用户券（携带乐观锁，成功时 Version 自增）。
	Save(ctx context.Context, c *UserCoupon) error
	// FindByCouponNo 按券码查询；不存在返回 ErrUserCouponNotFound。
	FindByCouponNo(ctx context.Context, couponNo string) (*UserCoupon, error)
	// FindByID 按 _id 查询；不存在返回 ErrUserCouponNotFound。
	FindByID(ctx context.Context, id string) (*UserCoupon, error)
	// FindByLockOrder 按锁单订单号查询被锁定的券；不存在返回 ErrUserCouponNotFound。
	FindByLockOrder(ctx context.Context, orderID string) (*UserCoupon, error)
	// ListByUser 按用户查询券列表，可按状态过滤（status 为空表示全部），按有效结束时间升序。
	ListByUser(ctx context.Context, userID string, status string, limit int) ([]*UserCoupon, error)
	// ListExpiredDue 返回已过有效期且仍为 AVAILABLE/LOCKED 的券（过期扫描批次）。
	ListExpiredDue(ctx context.Context, now time.Time, limit int) ([]*UserCoupon, error)
	// CountUsableByUser 返回某用户当前可用（AVAILABLE 且在有效期内）的券数。
	CountUsableByUser(ctx context.Context, userID string) (int64, error)
}
