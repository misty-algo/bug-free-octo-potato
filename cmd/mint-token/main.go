package main

import (
	"fmt"
	"os"
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/golang-jwt/jwt/v5"
)

// mint-token 是联调用的开发工具：直接签发访问令牌，跳过登录链路
// （L01-L07 尚未实现）。密钥与签发方取自 configs/config.yaml，
// 与网关、order 服务共用同一份配置，因此签出的令牌可被直接验过。
//
// 用法：go run ./cmd/mint-token <user_id> [role]   # role 默认 driver
func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: mint-token <user_id> [role]")
		os.Exit(2)
	}
	userID := os.Args[1]
	role := "driver"
	if len(os.Args) > 2 && os.Args[2] != "" {
		role = os.Args[2]
	}
	if err := config.InitGlobal("configs/config.yaml"); err != nil {
		fmt.Fprintln(os.Stderr, "load config failed:", err)
		os.Exit(1)
	}
	cfg := config.MustGlobal()
	secret := cfg.Middleware.JWT.Secret
	issuer := cfg.Middleware.JWT.Issuer
	if secret == "" || issuer == "" {
		fmt.Fprintln(os.Stderr, "config: middleware.jwt.secret and issuer must be set")
		os.Exit(1)
	}

	now := time.Now()
	ttl := time.Duration(cfg.Middleware.JWT.ExpireSeconds) * time.Second
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	claims := jwt.MapClaims{
		"user_id": userID,
		"role":    role,
		"iss":     issuer,
		"nbf":     now.Add(-time.Minute).Unix(),
		"iat":     now.Unix(),
		"exp":     now.Add(ttl).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign token failed:", err)
		os.Exit(1)
	}
	fmt.Println(signed)
}
