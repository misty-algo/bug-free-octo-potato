// Package model 定义乘客端落库模型。
//
// 硬约束：金额一律用「分」为单位的整数，字段名以 Fee/Amount/Price 结尾；
// 敏感字段走 Enc+Hash+Mask 三件套；时间统一 UTC 存储；
// 所有集合必带 created_at/updated_at/is_deleted；并发写集合必带 version。
package model

import "time"

// 集合名。与 06 数据字典保持一致。
const (
	CollectionAccounts         = "passenger_accounts"
	CollectionChannelBindings  = "passenger_channel_bindings"
	CollectionSessions         = "passenger_sessions"
	CollectionVerificationCodes = "passenger_verification_codes"
	CollectionEstimates        = "passenger_estimates"
	CollectionAddresses        = "passenger_addresses"
	CollectionOrders           = "passenger_orders"
	CollectionOrderStatusLogs  = "passenger_order_status_logs"
	CollectionQuickEntries     = "passenger_quick_entries"
	CollectionUsedQuickEntries = "passenger_used_quick_entries"
)

// AccountDocument 是乘客端主账号的 MongoDB 文档模型。
//
// 手机号走敏感三件套：Enc 为密文、Hash 用于等值查询建唯一索引、Mask 用于展示。
// 任何接口返回与日志输出 MUST 使用 Mask 版本。
type AccountDocument struct {
	ID       string `bson:"_id"`
	PhoneEnc string `bson:"phone_enc"`
	PhoneHash string `bson:"phone_hash"`
	PhoneMask string `bson:"phone_mask"`
	Status   string `bson:"status"`
	Nickname string `bson:"nickname"`
	Avatar   string `bson:"avatar_url"`
	// PasswordHash 为登录密码摘要，未设密码时为空。
	PasswordHash      string `bson:"password_hash"`
	RealNameVerified  bool   `bson:"real_name_verified"`
	CreatedAt         time.Time `bson:"created_at"`
	UpdatedAt         time.Time `bson:"updated_at"`
	IsDeleted         bool   `bson:"is_deleted"`
}

func (AccountDocument) MongoCollectionName() string { return CollectionAccounts }

// ChannelBindingDocument 是账号与第三方渠道绑定的 MongoDB 文档模型。
// channel + channel_uid 唯一，一个渠道标识只能绑到一个账号。
type ChannelBindingDocument struct {
	ID         string    `bson:"_id"`
	AccountID  string    `bson:"account_id"`
	Channel    string    `bson:"channel"`
	ChannelUID string    `bson:"channel_uid"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
	IsDeleted  bool      `bson:"is_deleted"`
}

func (ChannelBindingDocument) MongoCollectionName() string { return CollectionChannelBindings }

// SessionDocument 是登录会话的 MongoDB 文档模型。
// audience 标识令牌受众，乘客、司机、后台三套令牌互相隔离。
// 刷新令牌只存摘要，原文不落库。
type SessionDocument struct {
	ID               string    `bson:"_id"`
	AccountID        string    `bson:"account_id"`
	Audience         string    `bson:"audience"`
	Role             string    `bson:"role"`
	RefreshTokenHash string    `bson:"refresh_token_hash"`
	ExpiresAt        time.Time `bson:"expires_at"`
	RevokedAt        time.Time `bson:"revoked_at,omitempty"`
	CreatedAt        time.Time `bson:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at"`
	IsDeleted        bool      `bson:"is_deleted"`
}

func (SessionDocument) MongoCollectionName() string { return CollectionSessions }

// VerificationCodeDocument 是验证码的 MongoDB 文档模型。
// 该集合挂 TTL 索引按 expires_at 自动过期，验证码类数据用 TTL 而非物理删除。
type VerificationCodeDocument struct {
	ID        string    `bson:"_id"`
	PhoneHash string    `bson:"phone_hash"`
	Code      string    `bson:"code"`
	SentAt    time.Time `bson:"sent_at"`
	ExpiresAt time.Time `bson:"expires_at"`
	Used      bool      `bson:"used"`
	FailCount int       `bson:"fail_count"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (VerificationCodeDocument) MongoCollectionName() string { return CollectionVerificationCodes }

// EstimateDocument 是估价单的 MongoDB 文档模型。
// 金额一律为分。pricing_version 冻结命中的计价规则版本，
// 保证历史订单金额可复算。
type EstimateDocument struct {
	ID             string    `bson:"_id"`
	No             string    `bson:"no"`
	AccountID      string    `bson:"account_id"`
	CarType        int       `bson:"car_type"`
	BaseFee        int64     `bson:"base_fee"`
	ServiceFee     int64     `bson:"service_fee"`
	DiscountFee    int64     `bson:"discount_fee"`
	PayFee         int64     `bson:"pay_fee"`
	DistanceM      int64     `bson:"distance_m"`
	DurationS      int64     `bson:"duration_s"`
	PricingVersion string    `bson:"pricing_version"`
	CouponID       string    `bson:"coupon_id"`
	Used           bool      `bson:"used"`
	ExpiresAt      time.Time `bson:"expires_at"`
	CreatedAt      time.Time `bson:"created_at"`
	UpdatedAt      time.Time `bson:"updated_at"`
	IsDeleted      bool      `bson:"is_deleted"`
}

func (EstimateDocument) MongoCollectionName() string { return CollectionEstimates }

// AddressDocument 是常用地址与历史地址的 MongoDB 文档模型。
type AddressDocument struct {
	ID         string    `bson:"_id"`
	AccountID  string    `bson:"account_id"`
	Type       string    `bson:"type"`
	Tag        string    `bson:"tag"`
	Name       string    `bson:"name"`
	Addr       string    `bson:"addr"`
	POIID      string    `bson:"poi_id"`
	Lng        float64   `bson:"lng"`
	Lat        float64   `bson:"lat"`
	UseCount   int       `bson:"use_count"`
	LastUsedAt time.Time `bson:"last_used_at,omitempty"`
	CreatedAt  time.Time `bson:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at"`
	IsDeleted  bool      `bson:"is_deleted"`
}

func (AddressDocument) MongoCollectionName() string { return CollectionAddresses }

// OrderDocument 是订单的 MongoDB 文档模型。
// 金额一律为分。version 为乐观锁版本号，并发写集合必带。
type OrderDocument struct {
	ID              string    `bson:"_id"`
	OrderNo         string    `bson:"order_no"`
	AccountID       string    `bson:"account_id"`
	State           string    `bson:"state"`
	CarType         int       `bson:"car_type"`
	StartName       string    `bson:"start_name"`
	StartAddr       string    `bson:"start_addr"`
	StartLng        float64   `bson:"start_lng"`
	StartLat        float64   `bson:"start_lat"`
	EndName         string    `bson:"end_name"`
	EndAddr         string    `bson:"end_addr"`
	EndLng          float64   `bson:"end_lng"`
	EndLat          float64   `bson:"end_lat"`
	EstimateNo      string    `bson:"estimate_no"`
	TotalFee        int64     `bson:"total_fee"`
	CouponFee       int64     `bson:"coupon_fee"`
	CancelFee       int64     `bson:"cancel_fee"`
	Remark          string    `bson:"remark"`
	PricingSnapshot string    `bson:"pricing_snapshot"`
	IdempotencyKey  string    `bson:"idempotency_key"`
	Version         int64     `bson:"version"`
	CreatedAt       time.Time `bson:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
	IsDeleted       bool      `bson:"is_deleted"`
}

func (OrderDocument) MongoCollectionName() string { return CollectionOrders }

// OrderStatusLogDocument 是订单状态留痕的 MongoDB 文档模型。
// 每次状态变更写一条，保证任意一单可完整回放。该集合只追加不修改。
type OrderStatusLogDocument struct {
	ID        string    `bson:"_id"`
	OrderID   string    `bson:"order_id"`
	From      string    `bson:"from"`
	To        string    `bson:"to"`
	Operator  string    `bson:"operator"`
	Reason    string    `bson:"reason"`
	CreatedAt time.Time `bson:"created_at"`
}

func (OrderStatusLogDocument) MongoCollectionName() string { return CollectionOrderStatusLogs }

// QuickEntryDocument 是首页首次快速入口配置的 MongoDB 文档模型。
// 条目由后台配置，乘客端只读。
type QuickEntryDocument struct {
	EntryID   string    `bson:"_id"`
	Title     string    `bson:"title"`
	Icon      string    `bson:"icon"`
	Action    string    `bson:"action"`
	Target    string    `bson:"target"`
	Sort      int       `bson:"sort"`
	Enabled   bool      `bson:"enabled"`
	StartAt   time.Time `bson:"start_at,omitempty"`
	EndAt     time.Time `bson:"end_at,omitempty"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (QuickEntryDocument) MongoCollectionName() string { return CollectionQuickEntries }

// UsedQuickEntryDocument 是账号已用过入口的 MongoDB 文档模型。
// account_id + entry_id 唯一，重复点不会写两条。
type UsedQuickEntryDocument struct {
	ID        string    `bson:"_id"`
	AccountID string    `bson:"account_id"`
	EntryID   string    `bson:"entry_id"`
	UsedAt    time.Time `bson:"used_at"`
	CreatedAt time.Time `bson:"created_at"`
}

func (UsedQuickEntryDocument) MongoCollectionName() string { return CollectionUsedQuickEntries }
