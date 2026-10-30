package entity

import (
	"errors"
	"strings"
	"time"
)

// 司机登录验证码相关错误。
var (
	ErrInvalidPhone   = errors.New("invalid phone number")
	ErrInvalidCode    = errors.New("invalid verification code")
	ErrCodeExpired    = errors.New("verification code expired")
	ErrCodeUsed       = errors.New("verification code already used")
	ErrCodeLocked     = errors.New("too many failed attempts, please retry later")
	ErrCodeResendSoon = errors.New("verification code sent too frequently")
	ErrCodeNotFound   = errors.New("verification code not found, please request a new one")
	// ErrNameRequired 首次登录自动建档时必须提供姓名。
	ErrNameRequired = errors.New("driver name is required for first login")
)

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
)

// VerificationCode 司机登录验证码：与乘客侧同构，按号码存一条最新记录。
type VerificationCode struct {
	Phone     string
	Code      string
	SentAt    time.Time
	ExpiresAt time.Time
	// Used 标记验证码是否已被使用。保留记录才能继续对该号码执行重发限流。
	Used bool
	// FailCount 记录连续错误次数，达到阈值锁定。
	FailCount int
}

// NewVerificationCode 校验并创建验证码。
func NewVerificationCode(phone, code string, now time.Time) (*VerificationCode, error) {
	normalized, err := NormalizePhone(phone)
	if err != nil {
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
		Phone:     normalized,
		Code:      trimmed,
		SentAt:    now,
		ExpiresAt: now.Add(CodeTTL),
	}, nil
}

// Usable 报告验证码此刻是否还能使用。
func (c *VerificationCode) Usable(now time.Time) bool {
	return c != nil && !c.Used && now.UTC().Before(c.ExpiresAt)
}

// CanResend 报告此刻能否再次下发。
func (c *VerificationCode) CanResend(now time.Time) bool {
	return c == nil || now.UTC().Sub(c.SentAt) >= CodeResendInterval
}

// ResendWait 返回距离可再次下发的剩余时长。
func (c *VerificationCode) ResendWait(now time.Time) time.Duration {
	if c == nil {
		return 0
	}
	wait := CodeResendInterval - now.UTC().Sub(c.SentAt)
	if wait < 0 {
		return 0
	}
	return wait
}

// Locked 报告验证码是否因连续错误被锁定。
func (c *VerificationCode) Locked(now time.Time) bool {
	return c != nil && c.FailCount >= CodeMaxFailCount &&
		now.UTC().Sub(c.SentAt) < CodeLockDuration
}

// Verify 校验验证码。只校验不消耗：首次登录可能因为缺少姓名而中断，
// 若此时就把验证码作废，用户补完信息后反而登录不了。
// 流程真正走完（建档或登录成功）后由调用方显式调用 MarkUsed。
func (c *VerificationCode) Verify(code string, now time.Time) error {
	if c == nil {
		return ErrCodeNotFound
	}
	if c.Locked(now) {
		return ErrCodeLocked
	}
	if c.Used {
		return ErrCodeUsed
	}
	if !now.UTC().Before(c.ExpiresAt) {
		return ErrCodeExpired
	}
	if c.Code != strings.TrimSpace(code) {
		c.FailCount++
		return ErrInvalidCode
	}
	return nil
}

// MarkUsed 标记验证码已消耗。
func (c *VerificationCode) MarkUsed() {
	if c != nil {
		c.Used = true
	}
}

// NormalizePhone 归一化并校验中国大陆手机号。
func NormalizePhone(phone string) (string, error) {
	trimmed := strings.TrimSpace(phone)
	if len(trimmed) != 11 {
		return "", ErrInvalidPhone
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return "", ErrInvalidPhone
		}
	}
	if trimmed[0] != '1' {
		return "", ErrInvalidPhone
	}
	return trimmed, nil
}
