package handler

import (
	"context"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/metadata"
)

// materialGuard 只放行指定角色。司机只读与写入范围由服务端兜底，
// 前端隐藏入口不能作为权限依据。
func materialGuard(cfg authn.Config, roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required"))
			c.Abort()
			return
		}
		principal, err := authn.VerifyToken(parts[1], cfg)
		if err != nil {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "token is invalid or expired"))
			c.Abort()
			return
		}
		for _, role := range roles {
			if strings.EqualFold(principal.Role, role) {
				c.Next()
				return
			}
		}
		httpx.Error(c, apperrors.Forbidden("FORBIDDEN", strings.Join(roles, "/")+" role is required"))
		c.Abort()
	}
}

// authorizedContext 把令牌与请求号转发给 gRPC，身份不在请求体里重复声明。
func authorizedContext(c *gin.Context) context.Context {
	ctx := metadata.AppendToOutgoingContext(c.Request.Context(),
		"authorization", c.GetHeader("Authorization"))
	if id := c.GetHeader("X-Request-Id"); id != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", id)
	}
	return ctx
}

// requestContext 在转发令牌的同时施加请求超时。
func requestContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(authorizedContext(c), 15*time.Second)
}

// attachmentHeaders 标注私有材料：不缓存、不内联渲染，
// 避免浏览器长期缓存证件内容或被 CDN 二次分发。
func attachmentHeaders(c *gin.Context, contentType, filename string) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	c.Status(http.StatusOK)
}
