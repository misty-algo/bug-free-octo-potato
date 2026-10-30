package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// registerDriverRoutes 注册司机端路由（上下文统一 /api/v1/driver）。
//
// 鉴权划分：司机端业务接口要求司机令牌（role=driver）；跨模块回调由支付域 /
// 财务域 / 管理端以服务身份调用，不携带司机令牌，因此单独成组豁免。
func registerDriverRoutes(v1 *gin.RouterGroup, h *handler.DriverHandler, jwtCfg middleware.JWTConfig) {
	// ── 司机账号：登录前可用 ──
	auth := v1.Group("/driver/auth")
	auth.POST("/sms-code", h.SendDriverCode)
	auth.POST("/login", h.DriverLogin)

	// ── 司机端业务接口：需要司机令牌 ──
	drv := v1.Group("/driver", middleware.JWTAuth(jwtCfg), middleware.RequireRole("driver"))

	// ── 司机档案 / 状态 / 模式 / 区域 / 心跳 ──
	drv.POST("/profile", h.Register)
	drv.GET("/profile/:id", h.GetProfile)
	drv.POST("/profile/:id/status", h.UpdateStatus)
	drv.POST("/profile/:id/mode", h.UpdateMode)
	drv.POST("/profile/:id/heartbeat", h.Heartbeat)
	drv.POST("/profile/:id/offline-after-trip", h.UpdateOfflineAfterTrip)
	drv.PUT("/profile/:id/area", h.UpdateAreaConfig)

	// ── 接单大厅 ──
	drv.POST("/hall/orders", h.CreateHallOrder)
	drv.GET("/hall/orders", h.ListHallOrders)
	drv.POST("/hall/orders/grab", h.GrabOrder)
	drv.POST("/hall/orders/accept", h.AcceptOrder)
	drv.POST("/hall/orders/release", h.ReleaseOrder)
	drv.POST("/hall/orders/start", h.StartTrip)
	drv.POST("/hall/orders/complete", h.CompleteOrder)
	drv.POST("/hall/orders/cancel", h.CancelOrder)

	// ── 首页收益对比 ──
	drv.GET("/income/compare", h.GetIncomeCompare)

	// ── 收入结算 ──
	drv.GET("/income/flows", h.GetFlows)
	drv.GET("/income/summary", h.GetSummary)
	drv.POST("/income/withdraw", h.Withdraw)
	drv.GET("/income/withdrawals", h.ListWithdrawals)

	// ── 跨模块回调（支付域 / 财务域 / 后台集成使用，服务身份调用不鉴权）──
	callbacks := v1.Group("/driver")
	callbacks.POST("/payments", h.ReportPayment)
	callbacks.POST("/refunds", h.ReportRefund)
	callbacks.POST("/settlements", h.SettleIncome)
	callbacks.POST("/withdrawals/approve", h.ApproveWithdraw)
	callbacks.POST("/withdrawals/reject", h.RejectWithdraw)
	callbacks.POST("/withdrawals/complete", h.CompleteWithdraw)
}
