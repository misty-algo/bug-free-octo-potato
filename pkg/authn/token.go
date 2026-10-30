// Package authn verifies identity independently of HTTP and RPC transports.
package authn

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Config supplies trusted token verification settings.
type Config struct {
	Secret string
	Issuer string
}

// Principal contains the verified identity. Business boundaries authorize roles.
type Principal struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

type claims struct {
	Principal
	jwt.RegisteredClaims
}

// VerifyToken requires HS256, issuer, expiration, and a nonempty identity.
// It accepts legacy tokens with an empty role; each use case must authorize its role.
// IssueToken 用与 VerifyToken 对称的载荷签发 HS256 令牌。
// ttl 为有效期；now 传入业务时钟，便于测试固定时间。
func IssueToken(cfg Config, userID, role string, ttl time.Duration, now time.Time) (string, error) {
	if strings.TrimSpace(cfg.Secret) == "" || strings.TrimSpace(cfg.Issuer) == "" {
		return "", fmt.Errorf("token issuing requires secret and issuer")
	}
	if strings.TrimSpace(userID) == "" {
		return "", fmt.Errorf("token issuing requires user id")
	}
	if ttl <= 0 {
		return "", fmt.Errorf("token issuing requires positive ttl")
	}
	payload := claims{
		Principal: Principal{UserID: userID, Role: role},
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    cfg.Issuer,
			IssuedAt:  jwt.NewNumericDate(now.UTC()),
			NotBefore: jwt.NewNumericDate(now.UTC()),
			ExpiresAt: jwt.NewNumericDate(now.UTC().Add(ttl)),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, payload).SignedString([]byte(cfg.Secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

func VerifyToken(token string, cfg Config) (Principal, error) {
	if strings.TrimSpace(cfg.Secret) == "" || strings.TrimSpace(cfg.Issuer) == "" {
		return Principal{}, fmt.Errorf("token verification requires secret and issuer")
	}
	var payload claims
	parsed, err := jwt.ParseWithClaims(token, &payload, func(_ *jwt.Token) (interface{}, error) {
		return []byte(cfg.Secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(cfg.Issuer), jwt.WithExpirationRequired())
	if err != nil {
		return Principal{}, fmt.Errorf("verify token: %w", err)
	}
	if !parsed.Valid || strings.TrimSpace(payload.UserID) == "" {
		return Principal{}, fmt.Errorf("token identity is missing or invalid")
	}
	return payload.Principal, nil
}
