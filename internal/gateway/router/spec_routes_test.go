package router

import (
	"sort"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/internal/gateway/handler"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// TestContractRoutesRegistered 断言《后端接口文档 v2.0》约定的非管理端契约路径已挂载。
func TestContractRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api")

	jwtCfg := middleware.JWTConfig{Secret: "spec-secret", Issuer: "spec-issuer", ExpireSeconds: 1800}
	registerContractRoutes(api,
		handler.NewPassengerHandler(nil, nil),
		handler.NewDriverHandler(nil, nil),
		handler.NewSpecContractHandler(jwtCfg, nil),
		jwtCfg,
	)

	want := []string{
		"POST /api/auth/account-cancel",
		"POST /api/auth/binding",
		"POST /api/auth/login",
		"POST /api/auth/logout",
		"POST /api/auth/refresh",
		"GET /api/auth/restore-target",
		"POST /api/auth/sms/send",
		"POST /api/callback/pay",
		"POST /api/drv/grab",
		"GET /api/drv/hall",
		"POST /api/drv/heartbeat",
		"POST /api/drv/invite/respond",
		"GET /api/drv/income",
		"GET /api/drv/income/entries",
		"POST /api/drv/region-preference",
		"POST /api/drv/status",
		"GET /api/drv/vehicle/status",
		"GET /api/events",
		"GET /api/pax/address",
		"POST /api/pax/address",
		"POST /api/pax/estimate",
		"GET /api/pax/home",
		"POST /api/pax/order",
		"POST /api/pax/order/cancel",
		"POST /api/pax/order/cancel-quote",
		"POST /api/pax/order/change-endpoint",
		"GET /api/pax/order/driver-position",
		"GET /api/pax/order/message",
		"POST /api/pax/order/message",
		"POST /api/pax/order/share",
		"POST /api/pax/order/virtual-call",
		"GET /api/pax/order/wait-status",
		"POST /api/pax/pay",
		"GET /api/pax/pay/checkout",
		"POST /api/pax/pay/close",
		"GET /api/pax/pay/query",
		"GET /api/pax/vehicle-types",
		"GET /api/share/:token",
	}

	have := map[string]bool{}
	for _, r := range engine.Routes() {
		have[r.Method+" "+r.Path] = true
	}

	missing := make([]string, 0, len(want))
	for _, w := range want {
		if !have[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("契约路由缺失: %v", missing)
	}
}
