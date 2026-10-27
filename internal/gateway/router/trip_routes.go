package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerTripRoutes 注册乘客侧行程接口 /api/pax/trips。
// 链路：下单 → 司机接单 → 到达 → 上车 → 送达（计价 + 建账）→ 支付。
//
// 与 registerPassengerRoutes（dev 主干的 /api/v1/passenger 登录/首页/下单）是两个
// 独立的入口：那条走 passenger-service，这条走 shop-service 的行程六态链路。
func registerTripRoutes(api *gin.RouterGroup, tripHandler *handler.TripHandler) {
	routes := api.Group("/pax/trips")
	routes.POST("", tripHandler.CreateTrip)
	routes.GET("", tripHandler.ListTrips)
	routes.GET("/:trip_id", tripHandler.GetTrip)
	routes.POST("/assign", tripHandler.AssignDriver)
	routes.POST("/arrive", tripHandler.DriverArrived)
	routes.POST("/board", tripHandler.BoardTrip)
	routes.POST("/cancel", tripHandler.CancelTrip)
	routes.POST("/end", tripHandler.EndTrip)
	routes.POST("/pay", tripHandler.PayTrip)
}
