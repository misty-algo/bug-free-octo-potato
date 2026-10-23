package router

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"os"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/client"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
)

// registerAPIRoutes 创建 /api/v1 路由命名空间，再按业务模块分发。
func registerAPIRoutes(r *gin.Engine, clients *client.Clients, log *zap.Logger, middlewareCfg config.MiddlewareConfig) {
	api := r.Group("/api")
	v1 := api.Group("/v1")

	registerUserRoutes(v1, handler.NewUserHandler(clients.User, middlewareCfg.JWT, log), middlewareCfg.JWT)
	registerNoteRoutes(v1, handler.NewNoteHandler(clients.Note, log))
	registerOrderRoutes(v1, handler.NewOrderHandler(clients.Order, log))
	writes := strings.EqualFold(os.Getenv("ARRIVAL_ENABLED"), "true")
	if writes || strings.EqualFold(os.Getenv("ARRIVAL_READ_ENABLED"), "true") {
		registerArrivalRoutes(api, handler.NewArrivalHandler(clients.Arrival, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}, writes))
		registerContactRoutes(api, handler.NewContactHandler(clients.Contact, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}, writes))
	}
	// 服务规范是纯只读入口，与写开关无关，始终可用。
	registerSpecRoutes(api, handler.NewSpecHandler(clients.Spec, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}))
	// 车辆资料与材料上传：司机端读写 + 审核域底层读写能力，与行程写开关无关。
	registerVehicleRoutes(api,
		handler.NewVehicleHandler(clients.Vehicle, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}),
		handler.NewFileHandler(clients.File, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}),
	)
	// 安全事件独立于行程与投诉：即使行程写开关关闭也必须可用。
	registerSafetyRoutes(api,
		handler.NewSafetyHandler(clients.Safety, authn.Config{Secret: middlewareCfg.JWT.Secret, Issuer: middlewareCfg.JWT.Issuer}),
	)
}
