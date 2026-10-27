package entity

import (
	"errors"
	"time"
)

// 优惠券与司机档案相关错误。
var (
	ErrCouponNotFound      = errors.New("coupon not found")
	ErrCouponUnavailable   = errors.New("coupon unavailable")
	ErrCouponLocked        = errors.New("coupon already locked")
	ErrDriverNotRegistered = errors.New("driver profile not registered")
	ErrDriverNotAssigned   = errors.New("no driver assigned yet")
	ErrCancelNotAllowed    = errors.New("cancel not allowed in current state")
)

// CouponState 是优惠券状态。
type CouponState string

// 优惠券状态常量。状态枚举全小写下划线。
const (
	CouponStateAvailable CouponState = "available"
	CouponStateLocked    CouponState = "locked"
	CouponStateUsed      CouponState = "used"
	CouponStateExpired   CouponState = "expired"
	CouponStateVoid      CouponState = "void"
)

// Coupon 是优惠券实例。
//
// 券在估价时匹配、在下单时锁定、在支付成功时核销。
// 锁定与核销 MUST 用原子条件更新，MUST NOT 先查后改。
type Coupon struct {
	ID        string
	AccountID string
	TemplateID string
	Name      string
	// FaceValue 为面值，单位分。
	FaceValue int64
	// Threshold 为使用门槛，单位分。
	Threshold int64
	// CarTypes 限定可用车型，空表示不限。
	CarTypes []int
	State    CouponState
	// LockOrderID 为锁定该券的订单号。
	LockOrderID string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Usable 报告券此刻能否用于估价。
func (c *Coupon) Usable(now time.Time, carType int, amount int64) bool {
	if c == nil || c.State != CouponStateAvailable {
		return false
	}
	if !now.UTC().Before(c.ExpiresAt) {
		return false
	}
	if amount < c.Threshold {
		return false
	}
	if len(c.CarTypes) > 0 {
		matched := false
		for _, item := range c.CarTypes {
			if item == carType {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// Discount 返回券的抵扣金额，不得超过订单金额。
func (c *Coupon) Discount(amount int64) int64 {
	if c == nil {
		return 0
	}
	if c.FaceValue > amount {
		return amount
	}
	return c.FaceValue
}

// DriverAuditState 是司机资料审核状态。
type DriverAuditState string

// 司机审核状态常量。
const (
	DriverAuditPending  DriverAuditState = "pending"
	DriverAuditApproved DriverAuditState = "approved"
	DriverAuditRejected DriverAuditState = "rejected"
)

// DriverProfile 是司机资料。乘客端只读不写，资料由司机域维护。
type DriverProfile struct {
	ID        string
	AccountID string
	Name      string
	PlateNo   string
	CarModel  string
	Rating    string
	// AuditState 为审核状态。
	AuditState DriverAuditState
	// PhoneMask 为掩码手机号，真实手机号不出现在乘客端响应。
	PhoneMask string
	// Online 表示司机当前在线。
	Online bool
	// LastLng / LastLat 为司机最近上报的位置。
	LastLng float64
	LastLat float64
	// LastReportAt 为最近上报时间，用于判断位置是否延迟。
	LastReportAt time.Time
	UpdatedAt     time.Time
}

// Approved 报告司机资料是否审核通过。
func (d *DriverProfile) Approved() bool {
	return d != nil && d.AuditState == DriverAuditApproved
}

// PositionStale 报告司机位置是否延迟。
//
// 超过三十秒没有新位置就标「位置更新延迟」，不能让乘客盯着一个不动的点。
const PositionStaleThreshold = 30 * time.Second

// PositionDelayed 报告位置是否已延迟。
func (d *DriverProfile) PositionDelayed(now time.Time) bool {
	if d == nil {
		return true
	}
	return now.UTC().Sub(d.LastReportAt) > PositionStaleThreshold
}

// DriverLocation 是司机实时位置的一帧。
type DriverLocation struct {
	DriverID string
	Lng      float64
	Lat      float64
	// ETAMin 为预计到达分钟数。
	ETAMin int
	// Delayed 标识位置是否已延迟。
	Delayed bool
	// ReportedAt 为该位置的上报时间。
	ReportedAt time.Time
}

// CancelQuote 是取消订单的一次报价。
//
// 取消分两段式：第一次调用只返回报价不执行取消，
// 乘客确认后带确认标记再次调用才真正取消。
type CancelQuote struct {
	OrderID string
	// CancelFee 为取消费，单位分。
	CancelFee int64
	// ExpireAt 为报价有效期，过期后需要重新取价。
	ExpireAt time.Time
	// State 为订单当前状态。
	State OrderState
}

// Valid 报告报价是否仍在有效期内。
func (q *CancelQuote) Valid(now time.Time) bool {
	return q != nil && now.UTC().Before(q.ExpireAt)
}

// CancelFeeFor 按订单状态计算取消费，单位分。
//
// 待接单取消免费；已接单与已到达收取取消费。
// 行程开始后只能走安全提前结束，不走普通取消。
func CancelFeeFor(state OrderState, acceptedFee, arrivedFee int64) int64 {
	switch state {
	case OrderStatePending, OrderStateNoDriver:
		return 0
	case OrderStateAccepted:
		return acceptedFee
	case OrderStateArrived, OrderStateInService:
		return arrivedFee
	default:
		return 0
	}
}
