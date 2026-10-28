package router

import (
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"

	"github.com/gin-gonic/gin"
)

// registerPaxRoutes 注册乘客端接口 /api/pax/*（契约 3.6/3.7）。
// 全部挂 JWTAuth + RequireRole("pax")：只有乘客角色可访问，身份取 claims.UserID。
func registerPaxRoutes(api *gin.RouterGroup, h *handler.PaxOrderHandler, userH *handler.UserHandler, svcH *handler.PaxServiceHandler, jwtCfg middleware.JWTConfig) {
	pax := api.Group("/pax", middleware.JWTAuthPax(jwtCfg), middleware.RequireRole("pax"))

	// ── P17 订单列表 / P18 订单详情 / P19 提交评价·提交投诉 ──
	pax.GET("/orders", h.ListOrders)
	pax.GET("/orders/:order_id", h.GetOrderDetail)
	pax.POST("/order/rate", h.RateOrder)
	pax.POST("/order/complaint", h.SubmitComplaint)

	// ── P20 实名认证（契约 3.7.1）──
	pax.GET("/realname", userH.GetRealName)
	pax.POST("/realname", userH.SubmitRealName)

	// ── P21 账号与安全（契约 3.7.2）──
	pax.GET("/account-security", userH.GetAccountSecurity)
	pax.POST("/account-security", userH.RecordAgreement)

	// ── P22 客服页（契约 3.7.3）──
	pax.GET("/service", svcH.GetService)
}
