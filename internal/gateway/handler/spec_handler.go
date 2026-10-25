package handler

import (
	"net/http"
	"strings"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// SpecHandler 是司机端规范只读入口。只有 GET：
// 本服务不提供任何创建/编辑/发布路径，司机只读由服务端权限兜底。
type SpecHandler struct {
	client orderv1.ServiceSpecServiceClient
	auth   authn.Config
}

func NewSpecHandler(c orderv1.ServiceSpecServiceClient, cfg authn.Config) *SpecHandler {
	return &SpecHandler{client: c, auth: cfg}
}

// Guard 只放行已验证的司机令牌。
func (h *SpecHandler) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required"))
			c.Abort()
			return
		}
		principal, err := authn.VerifyToken(parts[1], h.auth)
		if err != nil {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "token is invalid or expired"))
			c.Abort()
			return
		}
		if !strings.EqualFold(principal.Role, "driver") {
			httpx.Error(c, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver role is required"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// cacheHeaders 施加 60 秒在线检查周期。60 秒是刷新周期而非内容存活时间：
// 断网后仍可读离线内容，可读性由前端 IndexedDB 维护并持续标注缓存属性。
func cacheHeaders(c *gin.Context, etag string) {
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=60, must-revalidate")
	c.Header("Vary", "Authorization")
}

// notModified 支持 If-None-Match 条件请求，命中则不回正文。
func notModified(c *gin.Context, etag string) bool {
	if etag == "" {
		return false
	}
	header := strings.TrimSpace(c.GetHeader("If-None-Match"))
	if header == "" {
		return false
	}
	strip := func(v string) string {
		v = strings.TrimSpace(v)
		v = strings.TrimPrefix(v, "W/")
		return strings.Trim(v, `"`)
	}
	if header == "*" {
		return true
	}
	// 聚合校验器内部含逗号承载多个分类摘要，因此整体比较而非按逗号拆分。
	return strip(header) == strip(etag)
}

// GetSpec: GET /api/drv/service-spec?category=&mode=brief|full
func (h *SpecHandler) GetSpec(c *gin.Context) {
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.GetContent(ctx, &orderv1.GetContentRequest{
		Category: strings.TrimSpace(c.Query("category")),
		Mode:     strings.TrimSpace(c.Query("mode")),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	cacheHeaders(c, out.GetEtag())
	if notModified(c, out.GetEtag()) {
		c.Status(http.StatusNotModified)
		return
	}
	httpx.OK(c, out)
}

// Manifest: GET /api/drv/service-spec/manifest
func (h *SpecHandler) Manifest(c *gin.Context) {
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.GetManifest(ctx, &orderv1.GetManifestRequest{})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	cacheHeaders(c, "")
	httpx.OK(c, out)
}
