package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerPaymentRoutes 在独立业务文件中注册 /api/v1/payments 接口。
func registerPaymentRoutes(v1 *gin.RouterGroup, paymentHandler *handler.PaymentHandler) {
	routes := v1.Group("/payments")
	routes.POST("", paymentHandler.Create)
	routes.POST("/notify", paymentHandler.Notify)
	routes.GET("/:order_no", paymentHandler.Query)
	routes.POST("/refund", paymentHandler.Refund)
}
