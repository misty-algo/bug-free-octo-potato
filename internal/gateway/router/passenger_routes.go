package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerPassengerRoutes 在独立业务文件中注册 /api/v1/passenger 接口。
//
// 登录、发码、刷新与游客看价是公开路由；其余需要会话，
// 由调用方在请求头带上账号标识或令牌。
func registerPassengerRoutes(v1 *gin.RouterGroup, passengerHandler *handler.PassengerHandler, serviceHandler *handler.PaxServiceHandler) {
	passenger := v1.Group("/passenger")

	// 公开路由：登录前就能调用。
	passenger.POST("/auth/sms-code", passengerHandler.SendCode)
	passenger.POST("/auth/login", passengerHandler.Login)
	passenger.POST("/auth/refresh", passengerHandler.Refresh)
	passenger.POST("/estimate/quote", passengerHandler.QuotePrice)

	// 账号相关。
	passenger.POST("/auth/set-password", passengerHandler.SetPassword)
	passenger.POST("/auth/bind-phone", passengerHandler.BindPhone)
	passenger.POST("/auth/logout", passengerHandler.Logout)
	passenger.POST("/auth/unbind-channel", passengerHandler.UnbindChannel)
	passenger.POST("/auth/cancel-account", passengerHandler.CancelAccount)
	passenger.POST("/auth/withdraw-cancel", passengerHandler.WithdrawCancelAccount)

	// 首页。
	passenger.GET("/home", passengerHandler.Home)
	passenger.GET("/poi/search", passengerHandler.SearchPOI)
	passenger.GET("/poi/recommend", passengerHandler.RecommendPickup)
	passenger.GET("/addresses", passengerHandler.ListAddresses)
	passenger.POST("/addresses", passengerHandler.SaveAddress)
	passenger.DELETE("/addresses", passengerHandler.DeleteAddress)
	passenger.POST("/addresses/history", passengerHandler.RecordHistory)
	passenger.POST("/addresses/history/clear", passengerHandler.ClearHistory)
	passenger.POST("/home/quick-entries/use", passengerHandler.UseQuickEntry)
	passenger.GET("/coupons", passengerHandler.ListCoupons)
	passenger.GET("/coupons/claimable", passengerHandler.ListClaimableCoupons)
	passenger.POST("/coupons/claim", passengerHandler.ClaimCoupon)

	// 实名认证（P20）：v1 组无 JWT，身份走 X-Account-Id 头。
	passenger.GET("/realname", passengerHandler.GetRealNameStatus)
	passenger.POST("/realname", passengerHandler.SubmitRealName)

	// 客服页（契约 3.7.3）：order 域工单 + user 域客服配置合并，身份走 X-Account-Id 头。
	passenger.GET("/service", serviceHandler.GetServiceHeader)

	// 打车下单。
	passenger.POST("/estimate", passengerHandler.Estimate)
	passenger.POST("/orders", passengerHandler.CreateOrder)
	passenger.GET("/orders", passengerHandler.ListOrders)
	passenger.GET("/orders/current", passengerHandler.CurrentOrder)
	passenger.GET("/orders/detail", passengerHandler.GetOrder)
	passenger.GET("/orders/cancel-quote", passengerHandler.CancelQuote)
	passenger.POST("/orders/cancel", passengerHandler.CancelOrder)
	passenger.GET("/orders/driver-location", passengerHandler.DriverLocation)
}
