package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerDriverRoutes 在独立业务文件中注册 /api/v1/drivers 接口。
func registerDriverRoutes(v1 *gin.RouterGroup, driverHandler *handler.DriverHandler) {
	routes := v1.Group("/drivers")
	routes.POST("", driverHandler.Create)
	routes.GET("", driverHandler.List)
	routes.GET("/:id", driverHandler.Get)
	routes.PUT("/:id", driverHandler.Update)
	routes.DELETE("/:id", driverHandler.Delete)
}
