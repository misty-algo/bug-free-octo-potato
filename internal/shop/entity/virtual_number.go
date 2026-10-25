package entity

import (
	"errors"
	"time"
)

// 虚拟号状态。
const (
	VirtualNumberBound    = "bound"
	VirtualNumberReleased = "released"
	VirtualNumberExpired  = "expired"
)

// VirtualNumber 是虚拟号聚合（P09：订单存续 +7 天有效，真实号不对端暴露）。
type VirtualNumber struct {
	ID          string
	OrderID     string
	PassengerID string
	DriverID    string
	VirtualNo   string
	Status      string
	BoundAt     time.Time
	ReleasedAt  time.Time
	ExpireAt    time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
	IsDeleted bool
}

// NewVirtualNumber 绑定虚拟号：订单行程中任一可联系状态（ACCEPTED/ARRIVED/IN_TRIP）均可绑定。
func NewVirtualNumber(orderID, passengerID, driverID, virtualNo string, boundAt time.Time, expireAt time.Time) (*VirtualNumber, error) {
	if orderID == "" || passengerID == "" || driverID == "" || virtualNo == "" {
		return nil, ErrInvalidVirtualNumber
	}
	if !expireAt.After(boundAt) {
		return nil, ErrInvalidVirtualNumber
	}
	return &VirtualNumber{
		OrderID:     orderID,
		PassengerID: passengerID,
		DriverID:    driverID,
		VirtualNo:   virtualNo,
		Status:      VirtualNumberBound,
		BoundAt:     boundAt,
		ExpireAt:    expireAt,
		CreatedAt:   boundAt,
		UpdatedAt:   boundAt,
	}, nil
}

// CanContact 判断订单状态是否允许通过虚拟号联系（P09 硬性规则）。
func CanContact(orderStatus string) bool {
	switch orderStatus {
	case TripStatusAccepted, TripStatusArrived, TripStatusInTrip:
		return true
	default:
		return false
	}
}

// Release 结束/取消时立即关闭虚拟号：状态置 released。
func (v *VirtualNumber) Release(now time.Time) error {
	if v.Status != VirtualNumberBound {
		return ErrVirtualNumberAlreadyReleased
	}
	v.Status = VirtualNumberReleased
	v.ReleasedAt = now
	v.UpdatedAt = now
	return nil
}

// IsActive 判断虚拟号是否可用：bound 且未过期。
func (v *VirtualNumber) IsActive(now time.Time) bool {
	return v.Status == VirtualNumberBound && now.Before(v.ExpireAt)
}

// 领域错误。
var (
	ErrInvalidVirtualNumber        = errors.New("invalid virtual number")
	ErrVirtualNumberAlreadyReleased = errors.New("virtual number already released")
	ErrVirtualNumberNotFound       = errors.New("virtual number not found")
)
