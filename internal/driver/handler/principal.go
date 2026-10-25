package handler

import (
	"context"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// principalFrom 从 gRPC metadata 取已验证身份。
// 身份永远来自令牌，不来自请求体里的 driver_id。
func principalFrom(ctx context.Context, cfg authn.Config) (authn.Principal, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required")
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required")
	}
	principal, err := authn.VerifyToken(parts[1], cfg)
	if err != nil {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "token is invalid or expired")
	}
	return principal, nil
}
