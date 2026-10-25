package entity

import (
	"context"
	"errors"
)

// TrajectoryRepository 定义实时轨迹点（P08）的持久化行为。
// trajectories 集合 append-only，不允许覆盖历史点。
type TrajectoryRepository interface {
	// Append 追加一个轨迹上报点。
	Append(ctx context.Context, point *TrajectoryPoint) error
	// ListByOrder 按上报时间升序返回某订单最近 limit 个轨迹点（limit<=0 返回全部）。
	ListByOrder(ctx context.Context, orderID string, limit int) ([]*TrajectoryPoint, error)
	// LatestByOrder 返回某订单最新的轨迹点，用于相邻点质量校验；不存在返回 ErrTrajectoryNotFound。
	LatestByOrder(ctx context.Context, orderID string) (*TrajectoryPoint, error)
}

// VirtualNumberRepository 定义虚拟号（P09）的持久化行为。
// virtual_numbers 集合 orderId 唯一：一单一号。
type VirtualNumberRepository interface {
	// Save 按 _id 新增或更新虚拟号。
	Save(ctx context.Context, v *VirtualNumber) error
	// FindByOrder 按订单号查询虚拟号；不存在返回 ErrVirtualNumberNotFound。
	FindByOrder(ctx context.Context, orderID string) (*VirtualNumber, error)
}

// DestinationChangeRepository 定义中途改点（P10）的持久化行为。
// destination_changes 是并发写集合：Save 携带 version 乐观锁，版本冲突返回 ErrDestinationChangeVersionConflict。
type DestinationChangeRepository interface {
	// Save 新增或更新改点申请（携带乐观锁，成功时 Version 自增）。
	Save(ctx context.Context, c *DestinationChange) error
	// FindByID 按 _id 查询改点申请；不存在返回 ErrDestinationChangeNotFound。
	FindByID(ctx context.Context, id string) (*DestinationChange, error)
	// ListByOrder 按申请时间升序返回某订单全部改点申请。
	ListByOrder(ctx context.Context, orderID string) ([]*DestinationChange, error)
	// CountAcceptedByOrder 返回某订单已确认（accepted）的改点次数，用于 2 次上限校验。
	CountAcceptedByOrder(ctx context.Context, orderID string) (int64, error)
}

// TripShareRepository 定义行程分享（P11）的持久化行为。
// trip_shares 集合 shareToken 唯一。
type TripShareRepository interface {
	// Save 按 _id 新增或更新行程分享。
	Save(ctx context.Context, s *TripShare) error
	// FindByToken 按分享令牌查询；不存在返回 ErrTripShareNotFound。
	FindByToken(ctx context.Context, token string) (*TripShare, error)
}

// CancelQuoteRepository 定义取消报价（P12）的持久化行为。
// cancel_quotes 集合 quoteNo 唯一，orderId+expiresAt 索引。
type CancelQuoteRepository interface {
	// Save 按 _id 新增或更新取消报价。
	Save(ctx context.Context, q *CancelQuote) error
	// FindByQuoteNo 按报价单号查询；不存在返回 ErrCancelQuoteNotFound。
	FindByQuoteNo(ctx context.Context, quoteNo string) (*CancelQuote, error)
	// FindByOrderLatest 返回某订单最新（expiresAt 最大）的报价；不存在返回 ErrCancelQuoteNotFound。
	FindByOrderLatest(ctx context.Context, orderID string) (*CancelQuote, error)
}

// 仓储层领域错误。
var (
	ErrTrajectoryNotFound            = errors.New("trajectory not found")
	ErrDestinationChangeNotFound     = errors.New("destination change not found")
	ErrDestinationChangeVersionConflict = errors.New("destination change version conflict")
	ErrTripShareNotFound             = errors.New("trip share not found")
	ErrCancelQuoteNotFound           = errors.New("cancel quote not found")
)
