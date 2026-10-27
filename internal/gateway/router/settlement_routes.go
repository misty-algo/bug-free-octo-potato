package router

import (
	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
)

// registerSettlementRoutes 在独立业务文件中注册 /api/v1/settlements 接口。
func registerSettlementRoutes(v1 *gin.RouterGroup, settlementHandler *handler.SettlementHandler) {
	routes := v1.Group("/settlements")
	routes.POST("/bills", settlementHandler.CreateBill)
	routes.POST("/split", settlementHandler.Split)
	routes.POST("/mark-bill-paid", settlementHandler.MarkBillPaid)
	routes.POST("/apply-refund", settlementHandler.ApplyRefund)
	routes.GET("/check-unpaid-ban/:user_id", settlementHandler.CheckUnpaidBan)
	routes.POST("/void-bill", settlementHandler.VoidBill)
	routes.GET("/wallets/:driver_id", settlementHandler.GetWalletBalance)
}
