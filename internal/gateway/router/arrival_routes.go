package router

import (
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/gin-gonic/gin"
)

// registerArrivalRoutes 挂载司机端行程服务（接口文档 3.9）：
// POST /api/drv/trip/arrive 到达上车点确认，POST /api/drv/position 司机位置上报。
func registerArrivalRoutes(api *gin.RouterGroup, h *handler.ArrivalHandler) {
	drv := api.Group("/drv", h.Guard())
	drv.POST("/trip/arrive", h.Arrive)
	drv.POST("/trip/verify", h.Verify)
	drv.POST("/trip/start", h.Start)
	drv.POST("/trip/end", h.End)
	drv.POST("/trip/sample", h.RecordSample)
	drv.POST("/position", h.RecordLocation)
}

// registerSpecRoutes 挂载司机端服务规范只读入口（D08）：GET /api/drv/service-spec。
// 只有 GET：司机角色不得创建、编辑或发布规范内容。
func registerSpecRoutes(api *gin.RouterGroup, h *handler.SpecHandler) {
	drv := api.Group("/drv", h.Guard())
	drv.GET("/service-spec", h.GetSpec)
	drv.GET("/service-spec/manifest", h.Manifest)
}

// registerContactRoutes 挂载司机端乘客联系（D07）：/api/drv/order/message、/api/drv/order/virtual-call。
func registerContactRoutes(api *gin.RouterGroup, h *handler.ContactHandler) {
	drv := api.Group("/drv", h.Guard())
	drv.GET("/order/message", h.ListMessages)
	drv.POST("/order/message", h.SendMessage)
	drv.POST("/order/virtual-call", h.RequestVirtualCall)
}
