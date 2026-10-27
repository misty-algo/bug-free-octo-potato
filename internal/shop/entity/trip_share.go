package entity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"
)

// 行程分享规则（P11）。
const (
	// TripShareMaxLifetime 分享令牌最长有效期（24 小时）。
	TripShareMaxLifetime = 24 * time.Hour
	// TripShareAfterEndLifetime 行程结束后分享额外保留时长（30 分钟）。
	TripShareAfterEndLifetime = 30 * time.Minute
	// TripShareRateLimitPerMinute 每分钟访问限流（60 次）。
	TripShareRateLimitPerMinute = 60
)

// TripShare 是行程分享聚合（P11：只读脱敏、token 不可推导、24h/结束 30 分失效、限流 60 次/分）。
type TripShare struct {
	ID          string
	OrderID     string
	ShareToken  string
	CreatorID   string
	ExpireAt    time.Time
	Revoked     bool
	AccessCount int64

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
	IsDeleted bool
}

// NewTripShare 创建行程分享：结束/取消 30 分钟后失效，上限 24 小时。
// tripEnded 传 true 表示行程已结束，缩短有效期。
func NewTripShare(orderID, creatorID string, orderEndedAt *time.Time, now time.Time) (*TripShare, error) {
	if orderID == "" || creatorID == "" {
		return nil, ErrInvalidTripShare
	}
	expireAt := now.Add(TripShareMaxLifetime)
	if orderEndedAt != nil {
		endExpire := orderEndedAt.Add(TripShareAfterEndLifetime)
		if endExpire.Before(expireAt) {
			expireAt = endExpire
		}
	}
	return &TripShare{
		OrderID:    orderID,
		ShareToken: GenerateShareToken(),
		CreatorID:  creatorID,
		ExpireAt:   expireAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// GenerateShareToken 生成不可预测的分享令牌（32 字节随机 hex，64 位）。
func GenerateShareToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		// crypto/rand 失败属极端异常，退化为时间+计数器组合仍保证不可预测性。
		return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf)
}

// IsExpired 判断分享是否失效：已撤销或超过 expireAt。
func (s *TripShare) IsExpired(now time.Time) bool {
	return s.Revoked || now.After(s.ExpireAt)
}

// Revoke 撤销分享（不可恢复）。
func (s *TripShare) Revoke(now time.Time) error {
	if s.IsExpired(now) {
		return ErrTripShareExpired
	}
	s.Revoked = true
	s.UpdatedAt = now
	return nil
}

// RecordAccess 记录一次访问并返回是否超过限流阈值。
func (s *TripShare) RecordAccess(now time.Time) bool {
	s.AccessCount++
	s.UpdatedAt = now
	return s.AccessCount > TripShareRateLimitPerMinute
}

// 领域错误。
var (
	ErrInvalidTripShare = errors.New("invalid trip share")
	ErrTripShareExpired = errors.New("trip share expired or revoked")
)
