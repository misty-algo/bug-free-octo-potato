package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerPassengerRoutes 注册乘客侧行程接口 /api/pax/trips。
// 链路：下单 → 司机接单 → 到达 → 上车 → 送达（计价 + 建账）。
func registerPassengerRoutes(api *gin.RouterGroup, paxHandler *handler.PassengerHandler) {
	routes := api.Group("/pax/trips")
	routes.POST("", paxHandler.CreateTrip)
	routes.GET("", paxHandler.ListTrips)
	routes.GET("/:trip_id", paxHandler.GetTrip)
	routes.POST("/assign", paxHandler.AssignDriver)
	routes.POST("/arrive", paxHandler.DriverArrived)
	routes.POST("/board", paxHandler.BoardTrip)
	routes.POST("/cancel", paxHandler.CancelTrip)
	routes.POST("/end", paxHandler.EndTrip)
	routes.POST("/pay", paxHandler.PayTrip)
}
