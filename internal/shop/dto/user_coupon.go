package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// IssueUserCouponCommand 发券入参（P14）。
type IssueUserCouponCommand struct {
	CouponNo    string
	UserID      string
	TemplateID  string
	BatchID     string
	ValidStart  time.Time
	ValidEnd    time.Time
	Source      string
	FaceValue   int64 // 模板快照：面值（分）
	MinCharge   int64 // 模板快照：使用门槛（分）
	MaxDiscount int64 // 模板快照：单笔封顶（分）
}

// LockUserCouponCommand 锁券入参（下单占券）。
type LockUserCouponCommand struct {
	CouponNo string
	OrderID  string
}

// UseUserCouponCommand 核销入参（支付成功确认）。
type UseUserCouponCommand struct {
	CouponNo       string
	OrderID        string
	DiscountAmount int64
}

// ReleaseUserCouponCommand 解锁入参（支付失败/取消订单退回）。
type ReleaseUserCouponCommand struct {
	CouponNo string
	OrderID  string
}

// VoidUserCouponCommand 作废入参（风控/运营介入）。
type VoidUserCouponCommand struct {
	CouponNo string
}

// ListUserCouponsCommand 查券列表入参。
type ListUserCouponsCommand struct {
	UserID string
	Status string // 空表示全部
	Limit  int
}

// UserCouponDTO 是券的 service 响应 DTO。
type UserCouponDTO struct {
	ID             string
	CouponNo       string
	UserID         string
	TemplateID     string
	BatchID        string
	Status         string
	ObtainAt       time.Time
	ValidStart     time.Time
	ValidEnd       time.Time
	UsedAt         *time.Time
	OrderID        string
	LockOrderID    string
	DiscountAmount int64
	Source         string
	FaceValue      int64
	MinCharge      int64
	MaxDiscount    int64
	Version        int64
}

// ListUserCouponsDTO 是券列表的 service 响应 DTO。
type ListUserCouponsDTO struct {
	Items []*UserCouponDTO
	Total int64
}

// FromUserCoupon 将券聚合转换为 DTO。
func FromUserCoupon(c *entity.UserCoupon) *UserCouponDTO {
	if c == nil {
		return nil
	}
	return &UserCouponDTO{
		ID:             c.ID,
		CouponNo:       c.CouponNo,
		UserID:         c.UserID,
		TemplateID:     c.TemplateID,
		BatchID:        c.BatchID,
		Status:         c.Status,
		ObtainAt:       c.ObtainAt,
		ValidStart:     c.ValidStart,
		ValidEnd:       c.ValidEnd,
		UsedAt:         c.UsedAt,
		OrderID:        c.OrderID,
		LockOrderID:    c.LockOrderID,
		DiscountAmount: c.DiscountAmount,
		Source:         c.Source,
		FaceValue:      c.FaceValue,
		MinCharge:      c.MinCharge,
		MaxDiscount:    c.MaxDiscount,
		Version:        c.Version,
	}
}
