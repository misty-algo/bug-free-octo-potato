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

	userH := handler.NewUserHandler(clients.User, middlewareCfg.JWT, log)
	registerUserRoutes(v1, userH, middlewareCfg.JWT)
	svcH := handler.NewPaxServiceHandler(clients.Order, clients.User, log)
	registerPaxRoutes(api, handler.NewPaxOrderHandler(clients.Order, log), userH, svcH, middlewareCfg.JWT)
	registerOrderRoutes(v1, handler.NewOrderHandler(clients.Order, log))
	registerPassengerRoutes(v1, handler.NewPassengerHandler(clients.Passenger, log))
	registerDriverRoutes(v1, handler.NewDriverHandler(clients.Driver, log))
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
	registerShopRoutes(v1, handler.NewShopHandler(clients.Shop, log))
	registerPaymentRoutes(v1, handler.NewPaymentHandler(clients.Payment, log))
	registerSettlementRoutes(v1, handler.NewSettlementHandler(clients.Settlement, log))
	// 行程六态链路走 shop-service，与 dev 主干的 /api/v1/passenger 相互独立。
	registerTripRoutes(api, handler.NewTripHandler(clients.Shop, log))
	// 《后端接口文档 v2.0》契约路径（登录 L01-L07 / 乘客 P01-P19 / 司机 D01-D15 /
	// 支付回调 / 只读分享 / 事件总线）。管理端 A01-A18 暂不注册。
	registerContractRoutes(api,
		handler.NewPassengerHandler(clients.Passenger, log),
		handler.NewDriverHandler(clients.Driver, log),
		handler.NewSpecContractHandler(middlewareCfg.JWT, log),
		middlewareCfg.JWT,
	)

	// 支付宝异步通知公网回调：cpolar 穿透路径 /aLiPay 原样透传，固定在根路径注册。
	r.POST("/aLiPay", handler.NewShopHandler(clients.Shop, log).AlipayNotify)
}
