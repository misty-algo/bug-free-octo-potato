package router

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
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

// TestDriverIDParamFromClaims 验证司机 ID 的三处注入与防越权强制覆盖。
//
// 司机端契约路径 /api/drv/* 不接收司机 ID，但既有 handler 的取值位置不统一：
// 路径参数 id（出收车/心跳/区域偏好）、query driver_id（大厅/收益/流水）、
// body driver_id（抢单/提现），后两处带 binding:"required"。
// 只注入路径参数会让这五个接口一律 400——这是已修复的真实缺陷，本用例防回归。
func TestDriverIDParamFromClaims(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtCfg := middleware.JWTConfig{Secret: "spec-secret", Issuer: "spec-issuer", ExpireSeconds: 1800}
	token, err := middleware.GenerateToken(jwtCfg, middleware.JWTClaims{UserID: "drv-1", Role: "driver"})
	if err != nil {
		t.Fatalf("生成测试令牌失败: %v", err)
	}

	// 全部请求都显式携带他人 ID，用于验证会被 claims 值强制覆盖。
	const attacker = "attacker"

	newEngine := func(capture gin.HandlerFunc) *gin.Engine {
		e := gin.New()
		chain := []gin.HandlerFunc{middleware.JWTAuth(jwtCfg), middleware.RequireRole("driver"), driverIDParamFromClaims(capture)}
		e.POST("/t", chain...)
		e.GET("/t", chain...)
		return e
	}
	do := func(e *gin.Engine, method, body string) *httptest.ResponseRecorder {
		var rdr io.Reader
		if body != "" {
			rdr = strings.NewReader(body)
		}
		req := httptest.NewRequest(method, "/t?driver_id="+attacker, rdr)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		e.ServeHTTP(w, req)
		return w
	}

	// ── POST：路径参数 + query + body 三处都要拿到 claims 值 ──
	var gotParam, gotQuery, gotBody string
	e := newEngine(func(c *gin.Context) {
		gotParam = c.Param("id")
		gotQuery = c.Query("driver_id")
		raw, _ := io.ReadAll(c.Request.Body)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		if v, ok := m["driver_id"].(string); ok {
			gotBody = v
		}
		c.Status(http.StatusOK)
	})
	if w := do(e, http.MethodPost, `{"driver_id":"`+attacker+`","amount_fen":100}`); w.Code != http.StatusOK {
		t.Fatalf("POST 应通过鉴权，实际 %d", w.Code)
	}
	if gotParam != "drv-1" {
		t.Errorf("路径参数 id 注入失败: got %q, want %q", gotParam, "drv-1")
	}
	if gotQuery != "drv-1" {
		t.Errorf("query driver_id 应强制覆盖为 claims 值: got %q, want %q", gotQuery, "drv-1")
	}
	if gotBody != "drv-1" {
		t.Errorf("body driver_id 应强制覆盖为 claims 值: got %q, want %q", gotBody, "drv-1")
	}

	// ── GET：无 body 时 query 注入仍生效，且不应因缺 body 报错 ──
	gotQuery = ""
	e2 := newEngine(func(c *gin.Context) {
		gotQuery = c.Query("driver_id")
		c.Status(http.StatusOK)
	})
	if w := do(e2, http.MethodGet, ""); w.Code != http.StatusOK {
		t.Fatalf("GET 无 body 时应正常通过，实际 %d", w.Code)
	}
	if gotQuery != "drv-1" {
		t.Errorf("GET query driver_id 注入失败: got %q, want %q", gotQuery, "drv-1")
	}
}
