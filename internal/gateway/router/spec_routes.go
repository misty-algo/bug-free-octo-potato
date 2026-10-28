package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// registerContractRoutes 注册《后端接口文档 v2.0》约定的契约路径（非管理端）。
//
// 合并前主干只暴露了 /api/v1/* 与少量 /api/pax/*、/api/drv/*，
// 与文档约定的 /api/auth/*、/api/pax/*、/api/drv/*、/api/events 不一致。
//
// 这里采用「能力复用 + 少量补齐」的方式对齐：
//   - 已有后端域承载的能力 → 直接把契约路径指向既有 handler，不复制业务规则；
//   - 尚无后端域承载的能力（恢复目标、渠道换绑、注销、车型、等待接单、
//     乘客消息/虚拟号/改点/分享、支付收银台与回调、听单邀请、车辆审核状态、
//     事件总线）→ 由 gateway 契约编排层 SpecContractHandler 实现。
func registerContractRoutes(
	api *gin.RouterGroup,
	paxH *handler.PassengerHandler,
	drvH *handler.DriverHandler,
	contractH *handler.SpecContractHandler,
	jwtCfg middleware.JWTConfig,
) {
	registerAuthContractRoutes(api, paxH, contractH, jwtCfg)
	registerPaxContractRoutes(api, paxH, contractH, jwtCfg)
	registerDriverContractRoutes(api, drvH, contractH, jwtCfg)
	registerSystemContractRoutes(api, contractH)
}

// registerAuthContractRoutes 注册公共登录 L01-L07。
func registerAuthContractRoutes(
	api *gin.RouterGroup,
	paxH *handler.PassengerHandler,
	contractH *handler.SpecContractHandler,
	jwtCfg middleware.JWTConfig,
) {
	// 登录前可用：短信下发、三渠道登录、刷新、退出。
	public := api.Group("/auth")
	public.POST("/sms/send", contractH.SendSMS)
	public.POST("/login", paxH.Login)
	public.POST("/refresh", paxH.Refresh)
	public.POST("/logout", paxH.Logout)

	// 登录后可用：恢复目标、渠道绑定/解绑、注销申请/撤回。
	// 角色切换必须重新登录，因此这里按当前令牌角色校验。
	session := api.Group("/auth", middleware.JWTAuthPax(jwtCfg))
	session.GET("/restore-target", contractH.RestoreTarget)
	session.POST("/binding", contractH.Binding)
	session.POST("/account-cancel", contractH.AccountCancel)
}

// registerPaxContractRoutes 注册乘客端 P01-P19 中尚未挂载的契约路径。
func registerPaxContractRoutes(
	api *gin.RouterGroup,
	paxH *handler.PassengerHandler,
	contractH *handler.SpecContractHandler,
	jwtCfg middleware.JWTConfig,
) {
	// accountIDFromClaims 必须排在鉴权之后：既有 passenger 域从请求头取账号标识，
	// 契约路径下由令牌 claims 回填，避免客户端重复传参且防止越权指定他人 ID。
	pax := api.Group("/pax", middleware.JWTAuthPax(jwtCfg), middleware.RequireRole("pax"), accountIDFromClaims())

	// ── P01 首页合并数据 / P02 POI 搜索 / P03 地址管理 ──
	pax.GET("/home", paxH.Home)
	pax.GET("/poi/search", paxH.SearchPOI)
	pax.GET("/address", paxH.ListAddresses)
	pax.POST("/address", contractH.PaxAddressCommand)

	// ── P05 车型列表 / P06 预估计价 / P07 创建订单与等待接单 ──
	pax.GET("/vehicle-types", contractH.VehicleTypes)
	pax.POST("/estimate", paxH.Estimate)
	pax.POST("/order", paxH.CreateOrder)
	pax.GET("/order/wait-status", contractH.WaitStatus)

	// ── P08 司机实时位置 / P09 订单消息与虚拟号 ──
	pax.GET("/order/driver-position", paxH.DriverLocation)
	pax.GET("/order/message", contractH.ListPaxMessages)
	pax.POST("/order/message", contractH.SendPaxMessage)
	pax.POST("/order/virtual-call", contractH.PaxVirtualCall)

	// ── P10 改点 / P11 分享 / P12 两段式取消 ──
	pax.POST("/order/change-endpoint", contractH.ChangeEndpoint)
	pax.POST("/order/share", contractH.CreateShare)
	pax.POST("/order/cancel-quote", paxH.CancelQuote)
	pax.POST("/order/cancel", paxH.CancelOrder)

	// ── P13 收银台 / P14 发起支付 / P15 查单关单 ──
	pax.GET("/pay/checkout", contractH.Checkout)
	pax.POST("/pay", contractH.Pay)
	pax.GET("/pay/query", contractH.PayQuery)
	pax.POST("/pay/close", contractH.PayClose)
}

// registerDriverContractRoutes 注册司机端 D01-D15 中尚未挂载的契约路径。
func registerDriverContractRoutes(
	api *gin.RouterGroup,
	drvH *handler.DriverHandler,
	contractH *handler.SpecContractHandler,
	jwtCfg middleware.JWTConfig,
) {
	drv := api.Group("/drv", middleware.JWTAuth(jwtCfg), middleware.RequireRole("driver"))

	// ── D01 出收车与模式切换 / D02 心跳 / D03 区域偏好 ──
	// 既有 driver 域从路径参数取司机 ID，契约路径无 ID，故从令牌 claims 注入。
	drv.POST("/status", driverIDParamFromClaims(drvH.UpdateStatus))
	drv.POST("/heartbeat", driverIDParamFromClaims(drvH.Heartbeat))
	drv.PUT("/region-preference", driverIDParamFromClaims(drvH.UpdateAreaConfig))
	drv.POST("/region-preference", driverIDParamFromClaims(drvH.UpdateAreaConfig))

	// ── D04 接单大厅合并数据 / D02 听单邀请响应 / D04 抢单 ──
	drv.GET("/hall", drvH.ListHallOrders)
	drv.POST("/invite/respond", contractH.InviteRespond)
	drv.POST("/grab", drvH.GrabOrder)

	// ── D14 车辆审核状态 ──
	drv.GET("/vehicle/status", contractH.VehicleStatus)

	// ── D09 收益中心 / D12 收入流水 / D13 申请提现 ──
	drv.GET("/income", drvH.GetSummary)
	drv.GET("/income/entries", drvH.GetFlows)
	drv.POST("/withdraw", drvH.Withdraw)
}

// registerSystemContractRoutes 注册支付渠道回调、只读分享与事件总线。
func registerSystemContractRoutes(api *gin.RouterGroup, contractH *handler.SpecContractHandler) {
	// P16 支付回调：由渠道服务端调用，不挂访问令牌，靠 sign 验签兜底。
	api.POST("/callback/pay", contractH.PayCallback)
	// P11 只读分享页：链接可跨端打开，令牌本身即凭证。
	api.GET("/share/:token", contractH.VisitShare)
	// 3.18 事件总线查询。
	api.GET("/events", contractH.ListEvents)
}

// accountIDFromClaims 把当前令牌的账号标识回填到请求头，
// 让既有 passenger 域 handler 在契约路径下也能识别身份，且客户端无法指定他人 ID。
// 必须挂在 JWTAuth 之后。
func accountIDFromClaims() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(c.GetHeader(middleware.HeaderAccountID)) == "" {
			if id := middleware.ClaimsFromContext(c).UserID; id != "" {
				c.Request.Header.Set(middleware.HeaderAccountID, id)
			}
		}
		c.Next()
	}
}

// driverIDParamFromClaims 把当前令牌的账号标识注入为路径参数 id，
// 让既有 driver 域 handler 能在契约路径下原样复用。
func driverIDParamFromClaims(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(middleware.HeaderAccountID)
		if claims := middleware.ClaimsFromContext(c); claims.UserID != "" {
			id = claims.UserID
		}
		if id == "" {
			httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
			c.Abort()
			return
		}
		c.Params = append(c.Params, gin.Param{Key: "id", Value: id})
		next(c)
	}
}
