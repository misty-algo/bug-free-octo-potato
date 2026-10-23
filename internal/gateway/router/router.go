// router 包负责 Gin 引擎构建和路由注册。
package router

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/client"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// New 使用配置好的中间件和版本化 API 路由构建网关 Gin 引擎。
func New(clients *client.Clients, log *zap.Logger, middlewareCfg config.MiddlewareConfig) *gin.Engine {
	if strings.EqualFold(os.Getenv("ARRIVAL_ENABLED"), "true") || strings.EqualFold(os.Getenv("ARRIVAL_READ_ENABLED"), "true") {
		headers := middlewareCfg.CORS.AllowHeaders
		if len(headers) == 0 {
			headers = middleware.DefaultCORSConfig().AllowHeaders
		}
		headers = append([]string(nil), headers...)
		found := false
		for _, header := range headers {
			if strings.EqualFold(header, "Idempotency-Key") {
				found = true
				break
			}
		}
		if !found {
			headers = append(headers, "Idempotency-Key")
		}
		middlewareCfg.CORS.AllowHeaders = headers
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(
		middleware.CORS(middlewareCfg.CORS),
		middleware.RequestID(),
		middleware.RequestLogger(log),
		gin.Recovery(),
	)
	r.OPTIONS("/*path", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	registerHealthRoutes(r)
	registerAPIRoutes(r, clients, log, middlewareCfg)
	return r
}
