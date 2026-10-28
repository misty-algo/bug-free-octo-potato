package middleware

import (
	"net/http"

	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// RequireRole 校验 JWTAuth 写入的 claims 角色与要求一致：
// claims 缺失（未挂 JWTAuth）→ 401；角色不匹配 → 403。两种情况均以契约信封
// 输出并中断链路，后续 handler 不执行。必须挂在 JWTAuth 之后。
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, ok := c.Get(claimsContextKey); !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, httpx.PaxBody{
				Code:    httpx.PaxCodeUnauthorized,
				Message: "missing authentication",
				Data:    nil,
			})
			return
		}
		claims := ClaimsFromContext(c)
		if claims.Role != role {
			c.AbortWithStatusJSON(http.StatusForbidden, httpx.PaxBody{
				Code:    httpx.PaxCodeUnauthorized,
				Message: "role not allowed",
				Data:    nil,
			})
			return
		}
		c.Next()
	}
}
