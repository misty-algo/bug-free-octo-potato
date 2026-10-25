// Package authn verifies identity independently of HTTP and RPC transports.
package authn

import (
	"fmt"
	"strings"

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
