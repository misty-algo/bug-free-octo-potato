package entity

import (
	"context"
	"time"
)

// AccountRepository 定义账号持久化行为。
type AccountRepository interface {
	FindByID(ctx context.Context, id string) (*Account, error)
	FindByPhone(ctx context.Context, phone string) (*Account, error)
	Save(ctx context.Context, account *Account) error
	// UpdatePhone 在乐观锁校验通过后更新手机号。
	UpdatePhone(ctx context.Context, account *Account) error
}

// ChannelRepository 定义渠道绑定持久化行为。
type ChannelRepository interface {
	FindByChannelUID(ctx context.Context, channel Channel, channelUID string) (*ChannelBinding, error)
	Save(ctx context.Context, binding *ChannelBinding) error
	Delete(ctx context.Context, id string) error
	ListByAccount(ctx context.Context, accountID string) ([]*ChannelBinding, error)
}

// SessionRepository 定义会话持久化行为。
type SessionRepository interface {
	Save(ctx context.Context, session *Session) error
	FindByID(ctx context.Context, id string) (*Session, error)
	// RevokeByAccount 撤销账号的全部刷新令牌，退出登录与注销账号时调用。
	RevokeByAccount(ctx context.Context, accountID string, now time.Time) error
}

// CodeRepository 定义验证码持久化行为。
type CodeRepository interface {
	// Save 覆盖保存该号码的验证码。
	Save(ctx context.Context, code *VerificationCode) error
	// Load 返回该号码的验证码，不存在时返回 ErrAccountNotFound。
	Load(ctx context.Context, phone string) (*VerificationCode, error)
	// CountRecentHourly 统计该号码近一小时的下发次数，用于小时级限流。
	CountRecentHourly(ctx context.Context, phone string, since time.Time) (int, error)
}

// EstimateRepository 定义估价单持久化行为。
type EstimateRepository interface {
	Save(ctx context.Context, estimate *Estimate) error
	FindByNo(ctx context.Context, no string) (*Estimate, error)
	// Consume 在乐观锁校验通过后把估价单置为已消费。
	Consume(ctx context.Context, estimate *Estimate) error
}

// AddressRepository 定义地址簿持久化行为。
type AddressRepository interface {
	Save(ctx context.Context, address *Address) error
	FindByID(ctx context.Context, id string) (*Address, error)
	Delete(ctx context.Context, id string) error
	// ListByType 返回该账号指定类型的地址。
	ListByType(ctx context.Context, accountID string, addrType AddressType, limit int) ([]*Address, error)
	// CountByType 统计该账号指定类型的地址条数。
	CountByType(ctx context.Context, accountID string, addrType AddressType) (int, error)
	// ClearHistory 清空该账号的历史地址。
	ClearHistory(ctx context.Context, accountID string) error
}

// POIRepository 定义地点检索行为。真实实现接地图服务，
// 未配置密钥时走 Mock Adapter。
type POIRepository interface {
	Search(ctx context.Context, keyword, city string, lng, lat float64, limit int) ([]*POI, error)
	// Recommend 返回当前位置附近的上车点。
	Recommend(ctx context.Context, lng, lat float64, limit int) ([]*POI, error)
}

// QuickEntryRepository 定义快速入口配置与已用记录持久化行为。
type QuickEntryRepository interface {
	// ListEnabled 返回启用中的入口配置。
	ListEnabled(ctx context.Context, now time.Time) ([]*QuickEntry, error)
	// ListUsedEntryIDs 返回该账号已用过的入口。
	ListUsedEntryIDs(ctx context.Context, accountID string) ([]string, error)
	// MarkUsed 记录该账号用过某入口。
	MarkUsed(ctx context.Context, accountID, entryID string, now time.Time) error
}

// CouponRepository 定义优惠券持久化行为。
type CouponRepository interface {
	// FindByID 按标识查券。
	FindByID(ctx context.Context, id string) (*Coupon, error)
	// ListUsable 列出该账号可用的券。
	ListUsable(ctx context.Context, accountID string, now time.Time) ([]*Coupon, error)
	// ListIDsByAccount 列出该账号名下所有券的标识（不限状态），
	// 供领券中心判断「是否已领取过」。
	ListIDsByAccount(ctx context.Context, accountID string) ([]string, error)
	// Save 新增或覆盖券。领券写入走这里；锁券与核销必须走 Lock/Unlock 条件更新。
	Save(ctx context.Context, coupon *Coupon) error
	// Lock 在原子条件更新下锁定券。已被其他订单锁定时返回 ErrCouponLocked。
	Lock(ctx context.Context, id string, orderID string, now time.Time) error
	// Unlock 释放锁定的券，用于订单取消或支付超时。
	Unlock(ctx context.Context, id string, now time.Time) error
}

// DriverRepository 定义司机档案只读行为。乘客端不写司机域的数据。
type DriverRepository interface {
	// FindByAccountID 返回账号的司机档案。
	FindByAccountID(ctx context.Context, accountID string) (*DriverProfile, error)
	// FindByOrderID 返回订单当前分配到的司机档案。
	FindByOrderID(ctx context.Context, orderID string) (*DriverProfile, error)
}

// OrderRepository 定义订单持久化行为。
type OrderRepository interface {
	Save(ctx context.Context, order *Order) error
	FindByID(ctx context.Context, id string) (*Order, error)
	FindByEstimateNo(ctx context.Context, estimateNo string) (*Order, error)
	// FindByOrderNo 按订单号查订单（支付回调按订单号定位订单）。
	FindByOrderNo(ctx context.Context, orderNo string) (*Order, error)
	FindByIdempotencyKey(ctx context.Context, accountID, key string) (*Order, error)
	// FindActive 返回该账号进行中的订单。
	FindActive(ctx context.Context, accountID string) (*Order, error)
	// ListByAccount 分页列出该账号的订单，可按状态过滤。
	ListByAccount(ctx context.Context, accountID string, state OrderState, offset int, limit int) ([]*Order, error)
	// CountByAccount 统计该账号符合条件的订单数。
	CountByAccount(ctx context.Context, accountID string, state OrderState) (int, error)
	// UpdateState 在乐观锁校验通过后更新状态。
	UpdateState(ctx context.Context, order *Order) error
	// AppendStatusLog 写一条状态留痕。
	AppendStatusLog(ctx context.Context, log *OrderStatusLog) error
}
