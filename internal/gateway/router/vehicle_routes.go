package router

import (
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/gin-gonic/gin"
)

// registerVehicleRoutes 挂载车辆资料与审核域（D14/D15）。
// 司机写入口全部要求 driver 角色；审核命令要求 auditor/safety_handler。
func registerVehicleRoutes(api *gin.RouterGroup, v *handler.VehicleHandler, f *handler.FileHandler) {
	drv := api.Group("/drv", v.DriverGuard())
	drv.POST("/vehicle/draft", v.SaveDraft)
	drv.POST("/vehicle/application", v.SubmitApplication)
	drv.POST("/vehicle/application/withdraw", v.WithdrawApplication)
	drv.GET("/vehicle/application", v.ApplicationDetail)
	drv.POST("/vehicle/document", v.SaveDocument)
	drv.GET("/vehicle/binding", v.CurrentBinding)
	drv.GET("/vehicle/snapshot", v.BindingSnapshot)
	// 实时准入：司机出车前自查，返回限制原因而不是布尔值。
	drv.GET("/eligibility", v.Eligibility)

	files := api.Group("/drv", f.DriverGuard())
	files.POST("/file/upload-session", f.CreateUpload)
	files.POST("/file/upload", f.UploadContent)

	// 票据与代理下载对职责角色开放；具体能读哪个文件由服务层按用途判定。
	material := api.Group("/drv", f.MaterialGuard())
	material.POST("/file/download-grant", f.GrantDownload)
	material.GET("/file/download", f.Download)

	admin := api.Group("/admin", v.AuditorGuard())
	// 派单域按 driver_id 取快照，与司机自助入口共用同一实现。
	admin.GET("/driver-binding/snapshot", v.BindingSnapshot)
	admin.GET("/driver-audit/pending", v.PendingAudits)
	admin.POST("/driver-audit/claim", v.ClaimApplication)
	admin.POST("/driver-audit/transfer", v.TransferApplication)
	admin.POST("/driver-audit/approve", v.ApproveApplication)
	admin.POST("/driver-audit/reject", v.RejectApplication)
	admin.POST("/outbox/drain", v.DrainOutbox)
	admin.POST("/file/sweep", f.SweepStaging)
	// 到期提醒调度（可传模拟时间补跑）与任务分配的权威入口。
	admin.POST("/reminder/tick", v.TickReminders)
	admin.POST("/dispatch/assign", v.AssignTask)
}
