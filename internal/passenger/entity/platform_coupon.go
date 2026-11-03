package entity

import (
	"context"
	"errors"
	"time"
)

// 平台券（管理端营销中心发放）相关错误。
var (
	// ErrPlatformCouponNotFound 表示券码在平台侧不存在或已删除。
	ErrPlatformCouponNotFound = errors.New("platform coupon not found")
	// ErrPlatformCouponUnavailable 表示券已被领完、作废或已过期。
	ErrPlatformCouponUnavailable = errors.New("platform coupon unavailable")
	// ErrInvalidRealName 表示姓名或证件号格式不合法。
	ErrInvalidRealName = errors.New("invalid real name or id no")
	// ErrInvalidCouponStock 表示发布数量不在允许区间内。
	ErrInvalidCouponStock = errors.New("invalid coupon stock")
)

// MaxPublishCouponStock 是单次发布允许的最大券数量。
//
// 管理端按「张」逐条写券，数量过大时一次请求就会写入海量文档，
// 因此设上限；超限由 usecase 直接拒绝，而不是静默截断，避免运营以为已足额发放。
const MaxPublishCouponStock = 500

// 平台侧（管理端营销中心）优惠券状态，与 coupons 集合 state 枚举一致。
const (
	PlatformCouponStateAvailable = "AVAILABLE"
	// PlatformCouponStateClaimed 表示券已被某位乘客抢领成功，
	// 不再出现在领券中心（先到先得：一张平台券只发一张本域券）。
	PlatformCouponStateClaimed = "CLAIMED"
)

// PlatformCoupon 是平台侧发放、等待乘客领取的优惠券。
//
// 数据由管理端营销中心写入共享库 coupons 集合，乘客端只读；
// 乘客领取后在本域 passenger_coupons 生成一张自己的券，之后的
// 估价匹配、下单锁定、核销全部复用本域既有链路。
type PlatformCoupon struct {
	// CouponNo 为平台券码，也是领取幂等的去重键。
	CouponNo string
	// TemplateNo 为管理端券模板编号，用于追溯批次。
	TemplateNo string
	// Name 为券的展示名称，历史券可能为空。
	Name string
	// State 为平台侧状态（AVAILABLE/CLAIMED/LOCKED/USED/EXPIRED/VOID）。
	State string
	// FaceValueFen 为面值，单位分。
	FaceValueFen int64
	// ThresholdFen 为使用门槛，单位分。
	ThresholdFen int64
	// ExpireAt 为券的过期时间。
	ExpireAt time.Time
}

// PlatformCouponRepository 定义平台券的读取与领取行为。
//
// 由 adapter 桥接到管理端营销中心的 coupons 集合；
// 跨域不直接复用管理端 store，由本域定义自己需要的最小语义。
type PlatformCouponRepository interface {
	// ListAvailable 列出当前可领取的平台券（state=AVAILABLE、券码非空且未过期）。
	ListAvailable(ctx context.Context, limit int, now time.Time) ([]*PlatformCoupon, error)
	// FindByCouponNo 按券码查询；不存在返回 ErrPlatformCouponNotFound。
	FindByCouponNo(ctx context.Context, couponNo string) (*PlatformCoupon, error)
	// ClaimOne 以原子条件更新抢占一张券：只有 state=AVAILABLE 且未过期
	// 才允许置为 CLAIMED，返回是否抢占成功（先到先得，防并发超领）。
	ClaimOne(ctx context.Context, couponNo, holderAccountID string, now time.Time) (bool, error)
	// Publish 管理端营销中心发布平台券：批量写入 AVAILABLE 券，供领券中心展示与领取。
	// stock 为发放数量（1..MaxPublishCouponStock），expireAt 为券过期时间，
	// now 由调用方注入，避免仓储自带时钟导致与 usecase 时间口径不一致。
	// 返回实际写入的券数量。
	Publish(ctx context.Context, templateNo, name string, faceValueFen, thresholdFen int64, expireAt, now time.Time, stock int) (int, error)
}
