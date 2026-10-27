// Package token 实现乘客端的令牌签发与校验。
//
// 乘客、司机、后台三套令牌按受众隔离：一套令牌只能访问自己那一端的路由，
// 拿乘客令牌调司机接口直接拒绝。角色写在令牌里，决定本次会话进入哪一端。
package token

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// 令牌相关错误。
var (
	ErrInvalidToken   = errors.New("invalid token")
	ErrTokenExpired   = errors.New("token expired")
	ErrAudienceDenied = errors.New("token audience mismatch")
)

// 令牌有效期。
const (
	// AccessTTL 为访问令牌有效期，三十分钟。
	AccessTTL = 30 * time.Minute
	// RefreshTTL 为刷新令牌有效期，七天。
	RefreshTTL = 7 * 24 * time.Hour
)

// Claims 是乘客端令牌载荷。
type Claims struct {
	jwt.RegisteredClaims
	// Audience 标识令牌受众。
	Audience entity.Audience `json:"aud"`
	// Role 标识本次会话进入的业务端。
	Role entity.Role `json:"role"`
}

// Issuer 签发与校验令牌。
type Issuer struct {
	secret []byte
	// now 可替换，便于测试。
	now func() time.Time
}

// NewIssuer 创建令牌签发器。
func NewIssuer(secret string) *Issuer {
	return &Issuer{secret: []byte(secret), now: time.Now}
}

// WithClock 替换签发器时钟，仅用于测试。
func (i *Issuer) WithClock(now func() time.Time) *Issuer {
	if now != nil {
		i.now = now
	}
	return i
}

// Issue 签发访问令牌与刷新令牌。
//
// 两个令牌的受众一致但类型不同，刷新令牌只能换新令牌，不能直接访问接口。
func (i *Issuer) Issue(accountID string, audience entity.Audience, role entity.Role, now time.Time) (string, string, time.Duration, error) {
	if !audience.Valid() {
		return "", "", 0, ErrAudienceDenied
	}
	if !role.Valid() {
		return "", "", 0, ErrInvalidToken
	}
	now = now.UTC()

	access, err := i.sign(accountID, audience, role, "access", now, now.Add(AccessTTL))
	if err != nil {
		return "", "", 0, err
	}
	refresh, err := i.sign(accountID, audience, role, "refresh", now, now.Add(RefreshTTL))
	if err != nil {
		return "", "", 0, err
	}
	return access, refresh, AccessTTL, nil
}

// ParseAccess 解析访问令牌并校验受众与角色。
func (i *Issuer) ParseAccess(token string, expectAudience entity.Audience, now time.Time) (string, entity.Role, error) {
	return i.parse(token, expectAudience, "access", now)
}

// ParseRefresh 解析刷新令牌并校验受众。
func (i *Issuer) ParseRefresh(token string, expectAudience entity.Audience, now time.Time) (string, entity.Role, error) {
	return i.parse(token, expectAudience, "refresh", now)
}

// sign 签发一个指定类型的令牌。
func (i *Issuer) sign(accountID string, audience entity.Audience, role entity.Role, tokenType string, issuedAt, expiresAt time.Time) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID,
			Issuer:    "huaxiaozhu-weapp",
			Audience:  jwt.ClaimStrings{string(audience)},
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        fmt.Sprintf("%s-%d", tokenType, issuedAt.UnixNano()),
		},
		Audience: audience,
		Role:     role,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
}

// parse 解析并校验令牌。
func (i *Issuer) parse(token string, expectAudience entity.Audience, tokenType string, now time.Time) (string, entity.Role, error) {
	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return i.secret, nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", "", ErrTokenExpired
		}
		return "", "", ErrInvalidToken
	}
	if !parsed.Valid {
		return "", "", ErrInvalidToken
	}
	// 令牌类型必须匹配，刷新令牌不能当访问令牌用。
	if !tokenTypeMatches(claims.ID, tokenType) {
		return "", "", ErrInvalidToken
	}
	// 受众隔离：跨端令牌直接拒绝，不做静默兼容。
	if claims.Audience != expectAudience {
		return "", "", ErrAudienceDenied
	}
	if !claims.Role.Valid() {
		return "", "", ErrInvalidToken
	}
	return claims.Subject, claims.Role, nil
}

// tokenTypeMatches 校验令牌类型。令牌 ID 形如 "access-时间戳"。
func tokenTypeMatches(id, expect string) bool {
	if len(id) <= len(expect) {
		return false
	}
	if id[:len(expect)] != expect {
		return false
	}
	return id[len(expect)] == '-'
}

// HashToken 计算令牌摘要。刷新令牌只存摘要，原文不落库。
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
