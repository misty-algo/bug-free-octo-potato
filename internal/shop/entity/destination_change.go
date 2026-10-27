package entity

import (
	"errors"
	"time"
)

// 中途改点状态。
const (
	DestinationChangePending   = "pending"
	DestinationChangeAccepted  = "accepted"
	DestinationChangeRejected  = "rejected"
	DestinationChangeTimeout   = "timeout"
	DestinationChangeCancelled = "cancelled"
)

// 中途改点规则（P10）。
const (
	// MaxDestinationChangePerOrder 单订单最多成功改点次数。
	MaxDestinationChangePerOrder = 2
	// DestinationChangeConfirmWindow 司机确认窗口（30 秒）。
	DestinationChangeConfirmWindow = 30 * time.Second
)

// DestinationChange 是中途改点聚合（P10：最多 2 次、司机 30 秒确认、订单版本乐观锁、拼车禁改）。
type DestinationChange struct {
	ID             string
	OrderID        string
	NewEndLocation GeoPoint
	NewEndAddress  string
	NewEndPoiID    string
	NewEstimateFee int64 // 变更后预估金额（分）
	Status         string
	RequestedAt    time.Time
	ConfirmedAt    time.Time
	ExpiresAt      time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
	IsDeleted bool
	Version   int64
}

// NewDestinationChange 创建改点申请（30 秒确认窗口）。
func NewDestinationChange(orderID string, location GeoPoint, address string, poiID string, newEstimateFee int64, now time.Time) (*DestinationChange, error) {
	if orderID == "" {
		return nil, ErrInvalidDestinationChange
	}
	if location.Type != "Point" || len(location.Coordinates) != 2 {
		return nil, ErrInvalidDestinationChange
	}
	if address == "" || newEstimateFee < 0 {
		return nil, ErrInvalidDestinationChange
	}
	return &DestinationChange{
		OrderID:        orderID,
		NewEndLocation: location,
		NewEndAddress:  address,
		NewEndPoiID:    poiID,
		NewEstimateFee: newEstimateFee,
		Status:         DestinationChangePending,
		RequestedAt:    now,
		ExpiresAt:      now.Add(DestinationChangeConfirmWindow),
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Confirm 司机确认改点：仅 pending 且未超时方可确认。
func (c *DestinationChange) Confirm(now time.Time) error {
	if c.Status != DestinationChangePending {
		return ErrDestinationChangeNotPending
	}
	if now.After(c.ExpiresAt) {
		c.Status = DestinationChangeTimeout
		c.UpdatedAt = now
		return ErrDestinationChangeExpired
	}
	c.Status = DestinationChangeAccepted
	c.ConfirmedAt = now
	c.UpdatedAt = now
	return nil
}

// Reject 司机拒绝改点。
func (c *DestinationChange) Reject(now time.Time) error {
	if c.Status != DestinationChangePending {
		return ErrDestinationChangeNotPending
	}
	c.Status = DestinationChangeRejected
	c.UpdatedAt = now
	return nil
}

// Cancel 乘客撤销改点申请。
func (c *DestinationChange) Cancel(now time.Time) error {
	if c.Status != DestinationChangePending {
		return ErrDestinationChangeNotPending
	}
	c.Status = DestinationChangeCancelled
	c.UpdatedAt = now
	return nil
}

// 领域错误。
var (
	ErrInvalidDestinationChange   = errors.New("invalid destination change")
	ErrDestinationChangeNotPending = errors.New("destination change is not pending")
	ErrDestinationChangeExpired   = errors.New("destination change confirm window expired")
	ErrDestinationChangeLimitHit  = errors.New("destination change limit reached")
)
