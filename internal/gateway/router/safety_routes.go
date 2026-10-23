package router

import (
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/gin-gonic/gin"
)

// registerSafetyRoutes 挂载安全事件域（D17）。
// 求助入口对司机开放，不以订单/定位/附件/审核为前提；
// 后台队列与通知投递对审核与安全处置角色开放。
func registerSafetyRoutes(api *gin.RouterGroup, s *handler.SafetyHandler) {
	drv := api.Group("/drv", s.DriverGuard())
	drv.POST("/safety/event", s.ReportEvent)
	drv.GET("/safety/event/by-request", s.EventByRequest)
	drv.POST("/safety/event/supplement", s.AddSupplement)
	drv.POST("/safety/event/attachment", s.AddAttachment)
	// 紧急电话指引与说明内容都是只读的受控内容。
	drv.GET("/safety/hotline", s.Hotline)
	drv.GET("/safety/notice", s.GetNotice)

	admin := api.Group("/admin", s.StaffGuard())
	admin.GET("/safety/events", s.PendingEvents)
	admin.POST("/safety/outbox/drain", s.DrainOutbox)
	// 处置闭环：领取 → 开始处理 → 转派/追加处置/附证据 → 办结。
	admin.POST("/safety/event/claim", s.ClaimEvent)
	admin.POST("/safety/event/start", s.StartProcessing)
	admin.POST("/safety/event/transfer", s.TransferEvent)
	admin.POST("/safety/event/action", s.AddAction)
	admin.POST("/safety/event/resolve", s.ResolveEvent)
	// 必要证据（读取也审计）与升级扫描。
	admin.GET("/safety/evidence", s.GetEvidence)
	admin.POST("/safety/escalate/tick", s.EscalateTick)
}
