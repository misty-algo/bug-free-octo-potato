// Package entity 定义乘客端业务域的领域实体与不变量。
//
// 该包 MUST NOT 引入 Gin / gRPC / GORM / MongoDB SDK / 日志框架，
// 只放业务规则与仓储接口。
package entity

import (
	"errors"
	"strings"
	"time"
)

// 乘客端领域错误。错误值统一定义在 entity，service 与 handler 只做映射。
var (
	ErrAccountNotFound    = errors.New("account not found")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrInvalidPhone       = errors.New("invalid phone")
	ErrPhoneAlreadyUsed   = errors.New("phone number is already used")
	ErrInvalidCode        = errors.New("invalid verification code")
	ErrCodeExpired        = errors.New("verification code expired")
	ErrCodeUsed           = errors.New("verification code already used")
	ErrCodeRateLimited    = errors.New("too many verification code requests")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrPasswordNotSet     = errors.New("password is not set")
	ErrInvalidRole        = errors.New("invalid role")
	ErrInvalidChannel     = errors.New("invalid channel")
	ErrChannelNotBound    = errors.New("channel not bound")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrEstimateNotFound   = errors.New("estimate not found")
	ErrEstimateExpired    = errors.New("estimate expired")
	ErrEstimateUsed       = errors.New("estimate already used")
	ErrInvalidCoordinate  = errors.New("invalid coordinate")
	ErrTooClose           = errors.New("origin and destination too close")
	ErrAddressNotFound    = errors.New("address not found")
	ErrCarTypeUnavailable = errors.New("car type unavailable")
	ErrOrderNotFound      = errors.New("order not found")
	ErrOrderStateConflict = errors.New("order state conflict")
	ErrIdempotencyUsed    = errors.New("idempotency key used with different request")
	ErrRestoreNotFound    = errors.New("no restorable target")
)

// Role 表示账号进入哪一端。同一个账号可以同时具备两个角色。
type Role string

// 角色常量。与后端管理端、司机端按受众隔离的令牌配套使用。
const (
	RolePassenger Role = "passenger"
	RoleDriver    Role = "driver"
)

// Valid 报告角色是否合法。
func (r Role) Valid() bool {
	return r == RolePassenger || r == RoleDriver
}

// Audience 表示令牌受众。乘客、司机、后台三套令牌互相隔离，
// 一套令牌只能访问自己那一端的路由。
type Audience string

// 受众常量。
const (
	AudiencePassenger Audience = "pax"
	AudienceDriver    Audience = "drv"
	AudienceAdmin     Audience = "admin"
)

// Valid 报告受众是否合法。
func (a Audience) Valid() bool {
	return a == AudiencePassenger || a == AudienceDriver || a == AudienceAdmin
}

// Channel 表示第三方登录渠道。
type Channel string

// 渠道常量。
const (
	ChannelWeChat Channel = "wechat"
	ChannelQQ     Channel = "qq"
	ChannelSMS    Channel = "sms"
)

// Valid 报告渠道是否合法。
func (c Channel) Valid() bool {
	return c == ChannelWeChat || c == ChannelQQ || c == ChannelSMS
}

// AccountStatus 表示账号状态。
type AccountStatus string

// 账号状态常量。
const (
	AccountStatusEnabled  AccountStatus = "enabled"
	AccountStatusDisabled AccountStatus = "disabled"
	AccountStatusCanceled AccountStatus = "canceled"
)

// Account 是主账号聚合。乘客与司机共用同一个账号，角色不拆分账号。
type Account struct {
	ID     string
	Phone  string
	Status AccountStatus
	// Nickname 与 Avatar 展示用，可空。
	Nickname string
	Avatar   string
	// PasswordHash 为登录密码摘要，空表示未设密码，只能用验证码登录。
	PasswordHash string
	// RealNameVerified 表示实名是否通过，下单前置条件之一。
	RealNameVerified bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// NewAccount 校验并创建主账号。
//
// 手机号可空：第三方渠道首次登录时用户可能还没授权手机号，
// 手机号绑定是后续动作。手机号存在时必须合法。
func NewAccount(id, phone string, now time.Time) (*Account, error) {
	phone = strings.TrimSpace(phone)
	if phone != "" {
		if err := ValidatePhone(phone); err != nil {
			return nil, err
		}
	}
	return &Account{
		ID:        id,
		Phone:     phone,
		Status:    AccountStatusEnabled,
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	}, nil
}

// Enabled 报告账号是否可用。
func (a *Account) Enabled() bool {
	return a != nil && a.Status == AccountStatusEnabled
}

// HasPassword 报告账号是否已设置密码。
func (a *Account) HasPassword() bool {
	return a != nil && strings.TrimSpace(a.PasswordHash) != ""
}

// WithPasswordHash 返回带密码摘要的账号副本。摘要由仓储层之外的哈希器产生。
func (a *Account) WithPasswordHash(hash string) (*Account, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, ErrPasswordNotSet
	}
	copied := *a
	copied.PasswordHash = hash
	return &copied, nil
}

// ChannelBinding 是账号与第三方渠道的绑定关系。
// 同一渠道下 channel_uid 唯一，一个渠道标识只能绑到一个账号。
type ChannelBinding struct {
	ID        string
	AccountID string
	Channel   Channel
	ChannelUID string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewChannelBinding 校验并创建渠道绑定。
func NewChannelBinding(id, accountID string, channel Channel, channelUID string, now time.Time) (*ChannelBinding, error) {
	if !channel.Valid() {
		return nil, ErrInvalidChannel
	}
	if strings.TrimSpace(channelUID) == "" {
		return nil, ErrInvalidChannel
	}
	return &ChannelBinding{
		ID:         id,
		AccountID:  accountID,
		Channel:    channel,
		ChannelUID: strings.TrimSpace(channelUID),
		CreatedAt:  now.UTC(),
		UpdatedAt:  now.UTC(),
	}, nil
}

// Session 是一次登录会话。刷新令牌单独成表，便于按账号撤销。
type Session struct {
	ID        string
	AccountID string
	Audience Audience
	Role      Role
	// RefreshTokenHash 为刷新令牌的摘要，原文不落库。
	RefreshTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Valid 报告会话是否已过期。
func (s *Session) Valid(now time.Time) bool {
	return s != nil && now.UTC().Before(s.ExpiresAt)
}

// VerificationCode 是一次验证码下发记录。
type VerificationCode struct {
	Phone     string
	Code      string
	SentAt    time.Time
	ExpiresAt time.Time
	// Used 标记验证码是否已被使用。使用后不删除，保留 SentAt 才能继续
	// 对该号码执行重发间隔限流，删掉会导致限流失效。
	Used bool
	// FailCount 记录连续错误次数，达到阈值锁定。
	FailCount int
}

// Usable 报告验证码此刻是否还能使用。
func (c *VerificationCode) Usable(now time.Time) bool {
	return c != nil && !c.Used && now.UTC().Before(c.ExpiresAt)
}

// Expired 报告验证码是否已过期。
func (c *VerificationCode) Expired(now time.Time) bool {
	return c != nil && !now.UTC().Before(c.ExpiresAt)
}

// 验证码规则。
const (
	// CodeLength 为验证码位数。
	CodeLength = 6
	// CodeTTL 为验证码有效期。
	CodeTTL = 5 * time.Minute
	// CodeResendInterval 为同号码重发间隔。
	CodeResendInterval = 60 * time.Second
	// CodeMaxFailCount 为连续错误上限，达到后锁定。
	CodeMaxFailCount = 5
	// CodeLockDuration 为锁定时长。
	CodeLockDuration = 30 * time.Minute
	// CodeHourlyLimit 为同号码同设备每小时下发上限。
	CodeHourlyLimit = 5
)

// NewVerificationCode 校验并创建验证码。
func NewVerificationCode(phone, code string, now time.Time) (*VerificationCode, error) {
	if err := ValidatePhone(phone); err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(code)
	if len(trimmed) != CodeLength {
		return nil, ErrInvalidCode
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return nil, ErrInvalidCode
		}
	}
	now = now.UTC()
	return &VerificationCode{
		Phone:     phone,
		Code:      trimmed,
		SentAt:    now,
		ExpiresAt: now.Add(CodeTTL),
	}, nil
}

// CanResend 报告此刻能否再次下发。
func (c *VerificationCode) CanResend(now time.Time) bool {
	return now.UTC().Sub(c.SentAt) >= CodeResendInterval
}

// Locked 报告验证码是否因连续错误被锁定。
func (c *VerificationCode) Locked(now time.Time) bool {
	return c != nil && c.FailCount >= CodeMaxFailCount &&
		now.UTC().Sub(c.SentAt) < CodeLockDuration
}

// 密码规则。
const (
	// PasswordMinLength 为密码最短长度。
	PasswordMinLength = 6
	// PasswordMaxLength 为密码最长长度。
	PasswordMaxLength = 32
)

// ValidatePassword 校验密码强度：6-32 位，且同时包含字母与数字。
func ValidatePassword(password string) error {
	length := len([]rune(password))
	if length < PasswordMinLength || length > PasswordMaxLength {
		return ErrInvalidPassword
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	if !hasLetter || !hasDigit {
		return ErrInvalidPassword
	}
	return nil
}

// ValidatePhone 校验中国大陆手机号。
func ValidatePhone(phone string) error {
	trimmed := strings.TrimSpace(phone)
	if len(trimmed) != 11 {
		return ErrInvalidPhone
	}
	if trimmed[0] != '1' {
		return ErrInvalidPhone
	}
	for _, r := range trimmed[1:] {
		if r < '0' || r > '9' {
			return ErrInvalidPhone
		}
	}
	return nil
}

// MaskPhone 对手机号脱敏，格式 138****8888。
// 任何接口返回与日志输出都必须使用脱敏后的值。
func MaskPhone(phone string) string {
	trimmed := strings.TrimSpace(phone)
	if len(trimmed) != 11 {
		return trimmed
	}
	return trimmed[:3] + "****" + trimmed[7:]
}
