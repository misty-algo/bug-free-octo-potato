package entity

import (
	"errors"
	"time"
)

// 用户券状态机（表结构 user_coupons status 枚举，评审口径）。
const (
	CouponStatusAvailable = "AVAILABLE" // 可用
	CouponStatusLocked    = "LOCKED"    // 锁单占券（下单锁定，待支付）
	CouponStatusUsed      = "USED"      // 已核销
	CouponStatusExpired   = "EXPIRED"   // 已过期
	CouponStatusVoid      = "VOID"      // 作废
)

// 券来源（表结构 user_coupons source 枚举）。
const (
	CouponSourceNewUser     = "new_user"
	CouponSourceActivity    = "activity"
	CouponSourceInvite      = "invite"
	CouponSourceManual      = "manual"
	CouponSourceExchange    = "exchange"
	CouponSourceCompensate  = "compensate"
)

// UserCoupon 是用户优惠券聚合（P14 券状态机）。
// 状态机：
//
//	AVAILABLE ──Lock(orderID)──▶ LOCKED ──Use(orderID)──▶ USED
//	    │                            │
//	    ├─ MarkExpired ──▶ EXPIRED   ├─ MarkExpired ──▶ EXPIRED
//	    └─ Void ────────▶ VOID      └─ Void ────────▶ VOID
//	LOCKED ──Release──▶ AVAILABLE（支付失败/取消订单解锁）
//
// 并发写集合：Save 携带 version 乐观锁，版本冲突返回 ErrUserCouponVersionConflict。
type UserCoupon struct {
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
	OrderID        string // 核销订单（USED 后写入）
	LockOrderID    string // 锁定订单（LOCKED 时写入）
	DiscountAmount int64  // 实际抵扣（分）
	Source         string

	// 模板快照（不入库，仅用于抵扣上限校验）。
	FaceValue   int64 // 券面值（分）
	MinCharge   int64 // 使用门槛（分）
	MaxDiscount int64 // 单笔封顶（分）

	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

// NewUserCoupon 构造一张可用券。
func NewUserCoupon(couponNo string, userID string, templateID string, batchID string,
	validStart time.Time, validEnd time.Time, source string, now time.Time) (*UserCoupon, error) {
	if couponNo == "" || userID == "" || templateID == "" {
		return nil, errors.New("coupon_no, user_id and template_id are required")
	}
	if validStart.IsZero() || validEnd.IsZero() {
		return nil, errors.New("valid_start and valid_end are required")
	}
	if !validEnd.After(validStart) {
		return nil, errors.New("valid_end must be after valid_start")
	}
	if !isValidCouponSource(source) {
		return nil, errors.New("invalid coupon source")
	}
	return &UserCoupon{
		CouponNo:   couponNo,
		UserID:     userID,
		TemplateID: templateID,
		BatchID:    batchID,
		Status:     CouponStatusAvailable,
		ObtainAt:   now,
		ValidStart: validStart,
		ValidEnd:   validEnd,
		Source:     source,
		CreatedAt:  now,
		UpdatedAt:  now,
		Version:    1,
	}, nil
}

// IsUsable 判断券当前是否可用：状态 AVAILABLE 且在有效期内。
func (c *UserCoupon) IsUsable(now time.Time) bool {
	return c.Status == CouponStatusAvailable && !now.Before(c.ValidStart) && now.Before(c.ValidEnd)
}

// Lock 下单锁券：AVAILABLE → LOCKED。同一订单重复锁定返回 nil（幂等），其他状态拒绝。
func (c *UserCoupon) Lock(orderID string, now time.Time) error {
	if c.Status == CouponStatusLocked && c.LockOrderID == orderID {
		return nil
	}
	if c.Status != CouponStatusAvailable {
		return ErrUserCouponInvalidState
	}
	if now.After(c.ValidEnd) || now.Before(c.ValidStart) {
		return ErrUserCouponExpired
	}
	c.Status = CouponStatusLocked
	c.LockOrderID = orderID
	c.UpdatedAt = now
	return nil
}

// Release 解锁：LOCKED → AVAILABLE（支付失败/取消订单时退回）。
func (c *UserCoupon) Release(now time.Time) error {
	if c.Status != CouponStatusLocked {
		return ErrUserCouponInvalidState
	}
	c.Status = CouponStatusAvailable
	c.LockOrderID = ""
	c.UpdatedAt = now
	return nil
}

// Use 核销：LOCKED → USED。校验锁单与订单一致、抵扣不超过面值/封顶。
func (c *UserCoupon) Use(orderID string, discountAmount int64, now time.Time) error {
	if c.Status != CouponStatusLocked {
		return ErrUserCouponInvalidState
	}
	if c.LockOrderID != "" && c.LockOrderID != orderID {
		return ErrUserCouponInvalidState
	}
	if now.After(c.ValidEnd) {
		return ErrUserCouponExpired
	}
	if c.FaceValue > 0 && discountAmount > c.FaceValue {
		return ErrUserCouponExceedsFaceValue
	}
	if c.MaxDiscount > 0 && discountAmount > c.MaxDiscount {
		return ErrUserCouponExceedsFaceValue
	}
	c.Status = CouponStatusUsed
	c.OrderID = orderID
	c.DiscountAmount = discountAmount
	usedAt := now
	c.UsedAt = &usedAt
	c.LockOrderID = ""
	c.UpdatedAt = now
	return nil
}

// MarkExpired 标记过期：AVAILABLE / LOCKED → EXPIRED（仅有效期已过时允许）。
func (c *UserCoupon) MarkExpired(now time.Time) error {
	if c.Status != CouponStatusAvailable && c.Status != CouponStatusLocked {
		return ErrUserCouponInvalidState
	}
	if !now.After(c.ValidEnd) {
		return ErrUserCouponInvalidState
	}
	c.Status = CouponStatusExpired
	c.LockOrderID = ""
	c.UpdatedAt = now
	return nil
}

// Void 作废：AVAILABLE / LOCKED → VOID（风控/运营介入）。
func (c *UserCoupon) Void(now time.Time) error {
	if c.Status != CouponStatusAvailable && c.Status != CouponStatusLocked {
		return ErrUserCouponInvalidState
	}
	c.Status = CouponStatusVoid
	c.LockOrderID = ""
	c.UpdatedAt = now
	return nil
}

func isValidCouponSource(source string) bool {
	switch source {
	case CouponSourceNewUser, CouponSourceActivity, CouponSourceInvite,
		CouponSourceManual, CouponSourceExchange, CouponSourceCompensate:
		return true
	default:
		return false
	}
}

// 券领域错误。
var (
	ErrUserCouponNotFound            = errors.New("user coupon not found")
	ErrUserCouponAlreadyExists       = errors.New("user coupon already exists")
	ErrUserCouponInvalidState        = errors.New("user coupon invalid state transition")
	ErrUserCouponExpired             = errors.New("user coupon expired")
	ErrUserCouponExceedsFaceValue    = errors.New("user coupon discount exceeds face value")
	ErrUserCouponVersionConflict     = errors.New("user coupon version conflict")
)
