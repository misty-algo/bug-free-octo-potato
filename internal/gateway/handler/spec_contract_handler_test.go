package handler

// spec_contract_handler_test.go 覆盖接口文档契约层编排实现（非管理端）。
//
// 按验收要求，每个用例执行三轮：验证幂等、状态机与资金口径在重复执行下不漂移。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// specFakeClock 是可推进的模拟时钟，用于跨日/过期类测试（文档 1.1）。
type specFakeClock struct{ at time.Time }

func (c *specFakeClock) Now() time.Time { return c.at }

func (c *specFakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func newSpecContractHandlerForTest(t *testing.T) (*SpecContractHandler, *specFakeClock) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := middleware.JWTConfig{Secret: "spec-contract-secret", Issuer: "spec-contract-issuer", ExpireSeconds: 1800}
	now := &specFakeClock{at: time.Unix(1700000000, 0).UTC()}
	resetSpecContractStore()
	return NewSpecContractHandler(cfg, nil).WithClock(now), now
}

// resetSpecContractStore 清空全局存储的运行时状态，保证用例之间互不干扰。
// 保留预置的计价配置、POI、角色与字典（属于种子数据，不是运行时状态）。
func resetSpecContractStore() {
	s := specGlobalStore
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq = 0
	s.logs = nil
	s.events = nil
	s.flows = nil
	s.eventsIdx = map[string]struct{}{}

	s.orders = map[string]*specOrder{}
	s.bills = map[string]*specBill{}
	s.attempts = map[string]*specAttempt{}
	s.estimates = map[string]*specEstimate{}
	s.tasks = map[string]*specTask{}
	s.tickets = map[string]*specTicket{}
	s.draws = map[string]*specWithdrawal{}
	s.funds = map[string]*specFund{}
	s.refreshes = map[string]*specSession{}
	s.addresses = map[string][]*specAddress{}
	s.profiles = map[string]*specPaxProfile{}
	s.applications = map[string]*specVehicleApplication{}
	s.campaigns = map[string]*specCampaign{}
	s.exps = map[string]*specExportTask{}
	s.bindings = map[string]*specChannelBindings{}
	s.sms = map[string]*specSMSRecord{}
	s.idem = map[string]*specIdemRecord{}
	s.vehicleStatus = map[string]*specVehicleStatusRecord{}
	s.driverStatus = map[string]specDriverRuntime{}
	s.verifyAttempts = map[string]specVerifyState{}
	s.shareVisits = map[string]map[int64]int{}
}

// specContractEngine 只注册契约层自有实现的路径，便于用 httptest 直接驱动。
func specContractEngine(h *SpecContractHandler) *gin.Engine {
	engine := gin.New()
	api := engine.Group("/api")

	auth := api.Group("/auth")
	auth.POST("/sms/send", h.SendSMS)
	auth.GET("/restore-target", h.RestoreTarget)
	auth.POST("/binding", h.Binding)
	auth.POST("/account-cancel", h.AccountCancel)

	pax := api.Group("/pax")
	pax.GET("/vehicle-types", h.VehicleTypes)
	pax.GET("/order/wait-status", h.WaitStatus)
	pax.GET("/order/message", h.ListPaxMessages)
	pax.POST("/order/message", h.SendPaxMessage)
	pax.POST("/order/virtual-call", h.PaxVirtualCall)
	pax.POST("/order/change-endpoint", h.ChangeEndpoint)
	pax.POST("/order/share", h.CreateShare)
	pax.GET("/pay/checkout", h.Checkout)
	pax.POST("/pay", h.Pay)
	pax.GET("/pay/query", h.PayQuery)
	pax.POST("/pay/close", h.PayClose)  // 文档 3.5.4 关单
	api.POST("/callback/pay", h.PayCallback) // 文档 3.5.3 渠道回调

	drv := api.Group("/drv")
	drv.POST("/invite/respond", h.InviteRespond)
	drv.GET("/vehicle/status", h.VehicleStatus)

	api.GET("/share/:token", h.VisitShare)
	api.GET("/events", h.ListEvents)
	return engine
}

func specCall(t *testing.T, h *SpecContractHandler, method, target string, body, accountID any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	raw := []byte(nil)
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		raw = encoded
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if id, ok := accountID.(string); ok && id != "" {
		req.Header.Set(middleware.HeaderAccountID, id)
	}
	rec := httptest.NewRecorder()
	specContractEngine(h).ServeHTTP(rec, req)

	parsed := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func specCode(t *testing.T, rec *httptest.ResponseRecorder, body map[string]any, status, code int, name string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("%s: HTTP 状态期望 %d，实际 %d，响应=%s", name, status, rec.Code, rec.Body.String())
	}
	got, _ := body["code"].(float64)
	if int(got) != code {
		t.Fatalf("%s: 业务码期望 %d，实际 %v，响应=%s", name, code, body["code"], rec.Body.String())
	}
}

func seedSpecOrder(h *SpecContractHandler, id, pax, state string) *specOrder {
	now := h.now()
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	order := &specOrder{
		ID: id, PassengerID: pax, Type: "kuaiche", State: state,
		StartName: "世纪大道地铁站", EndName: "张江高科站",
		DistM: 6500, DurS: 960, Version: 1, CreatedAt: now,
		PriceSnap: map[string]int64{"P": 1870},
		Timeline:  []specTimeline{{Name: "CREATED", T: now}},
	}
	h.store.orders[id] = order
	return order
}

// ── L01 短信验证码：60 秒重发限制 ──

func TestSpecContract_SendSMS_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			phone := "1380000000" + strconv.Itoa(round)

			rec, body := specCall(t, h, http.MethodPost, "/api/auth/sms/send",
				specSMSSendRequest{Phone: phone, Scene: "login", DeviceID: "dev-1"}, "")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "首次下发")
			data, _ := body["data"].(map[string]any)
			if retry, _ := data["retry_after"].(float64); int(retry) != 60 {
				t.Fatalf("retry_after 期望 60，实际 %v", data["retry_after"])
			}

			// 60 秒内重发 → 42901。
			rec, body = specCall(t, h, http.MethodPost, "/api/auth/sms/send",
				specSMSSendRequest{Phone: phone, Scene: "login", DeviceID: "dev-1"}, "")
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeRealNameRateLimit, "60秒内重发")
		})
	}
}

// ── L03 登录恢复目标优先级 ──

func TestSpecContract_RestoreTarget_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-RT-" + strconv.Itoa(round)

			rec, body := specCall(t, h, http.MethodGet, "/api/auth/restore-target", nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "无业务→default")
			if specTargetType(body) != "default" {
				t.Fatalf("期望 default，实际 %v", body["data"])
			}

			seedSpecOrder(h, "ORD-RT-"+strconv.Itoa(round), accountID, OrderStateAccepted)
			rec, body = specCall(t, h, http.MethodGet, "/api/auth/restore-target", nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "活动行程优先")
			if specTargetType(body) != "active_trip" {
				t.Fatalf("期望 active_trip，实际 %v", body["data"])
			}

			h.store.mu.Lock()
			h.store.orders["ORD-RT-"+strconv.Itoa(round)].State = OrderStateEnded
			h.store.bills["BILL-RT-"+strconv.Itoa(round)] = &specBill{
				ID: "BILL-RT-" + strconv.Itoa(round), OrderID: "ORD-RT-" + strconv.Itoa(round),
				Kind: "trip", P: 1870, D: 1360, N: 510, State: BillStateUnpaid,
			}
			h.store.mu.Unlock()
			rec, body = specCall(t, h, http.MethodGet, "/api/auth/restore-target", nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "未付账单次之")
			if specTargetType(body) != "unpaid_bill" {
				t.Fatalf("期望 unpaid_bill，实际 %v", body["data"])
			}
		})
	}
}

func specTargetType(body map[string]any) string {
	data, _ := body["data"].(map[string]any)
	target, _ := data["restore_target"].(map[string]any)
	value, _ := target["type"].(string)
	return value
}

// ── L06 渠道换绑 ──

func TestSpecContract_Binding_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-BD-" + strconv.Itoa(round)

			rec, body := specCall(t, h, http.MethodPost, "/api/auth/binding",
					specBindingRequest{Action: "bind", Channel: "wechat", DeviceID: "dev-1"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "绑定微信")

			rec, body = specCall(t, h, http.MethodPost, "/api/auth/binding",
					specBindingRequest{Action: "unbind", Channel: "wechat", DeviceID: "dev-1"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "解绑微信仍保留短信")

			rec, body = specCall(t, h, http.MethodPost, "/api/auth/binding",
					specBindingRequest{Action: "unbind", Channel: "sms", DeviceID: "dev-1"}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "解绑最后一种登录方式")

			rec, body = specCall(t, h, http.MethodPost, "/api/auth/binding",
					specBindingRequest{Action: "bind", Channel: "alipay"}, accountID)
			specCode(t, rec, body, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "非法渠道")
		})
	}
}

// ── L07 注销账号前置检查与冷静期 ──

func TestSpecContract_AccountCancel_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-AC-" + strconv.Itoa(round)
			orderID := "ORD-AC-" + strconv.Itoa(round)

			seedSpecOrder(h, orderID, accountID, OrderStateWaiting)
			rec, body := specCall(t, h, http.MethodPost, "/api/auth/account-cancel",
					specAccountCancelRequest{Action: "apply", Role: "pax"}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "在途行程拒绝注销")

			h.store.mu.Lock()
			delete(h.store.orders, orderID)
			h.store.mu.Unlock()

			rec, body = specCall(t, h, http.MethodPost, "/api/auth/account-cancel",
					specAccountCancelRequest{Action: "apply", Role: "pax"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "申请注销")
			data, _ := body["data"].(map[string]any)
			if data["state"] != "cooling_off" {
				t.Fatalf("期望 cooling_off，实际 %v", data["state"])
			}
			if until, _ := data["cooling_off_until"].(float64); int64(until) != 1700000000+7*24*3600 {
				t.Fatalf("冷静期应为 now+7d，实际 %v", until)
			}

			// 冷静期内撤回。
			rec, body = specCall(t, h, http.MethodPost, "/api/auth/account-cancel",
					specAccountCancelRequest{Action: "withdraw", Role: "pax"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "撤回注销")
			data, _ = body["data"].(map[string]any)
			if data["state"] != "applied" {
				t.Fatalf("撤回后期望 applied，实际 %v", data["state"])
			}
		})
	}
}

// ── P05 车型列表 ──

func TestSpecContract_VehicleTypes_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)

			rec, body := specCall(t, h, http.MethodGet, "/api/pax/vehicle-types", nil, "PAX-VT")
			specCode(t, rec, body, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "缺少城市")

			rec, body = specCall(t, h, http.MethodGet, "/api/pax/vehicle-types?city=上海", nil, "PAX-VT")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "已开通城市")
			types := body["data"].(map[string]any)["types"].([]any)
			if len(types) != 3 {
				t.Fatalf("期望 3 种车型，实际 %d", len(types))
			}
			for _, item := range types {
				if enabled, _ := item.(map[string]any)["enabled"].(bool); !enabled {
					t.Fatalf("已开通城市车型应可用，实际 %v", item)
				}
			}

			rec, body = specCall(t, h, http.MethodGet, "/api/pax/vehicle-types?city=未开通", nil, "PAX-VT")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "未开通城市")
			for _, item := range body["data"].(map[string]any)["types"].([]any) {
				if enabled, _ := item.(map[string]any)["enabled"].(bool); enabled {
					t.Fatalf("未开通城市车型应置灰，实际 %v", item)
				}
			}
		})
	}
}

// ── P07 等待接单状态查询 ──

func TestSpecContract_WaitStatus_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-WS-" + strconv.Itoa(round)

			rec, body := specCall(t, h, http.MethodGet, "/api/pax/order/wait-status", nil, accountID)
			specCode(t, rec, body, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "缺少订单号")

			rec, body = specCall(t, h, http.MethodGet, "/api/pax/order/wait-status?order_id=NOT-EXIST", nil, accountID)
			specCode(t, rec, body, http.StatusNotFound, httpx.PaxCodeNotFound, "订单不存在")

			orderID := "ORD-WS-" + strconv.Itoa(round)
			seedSpecOrder(h, orderID, accountID, OrderStateWaiting)
			rec, body = specCall(t, h, http.MethodGet, "/api/pax/order/wait-status?order_id="+orderID, nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "等待接单")
			if body["data"].(map[string]any)["state"] != OrderStateWaiting {
				t.Fatalf("期望 WAITING，实际 %v", body["data"])
			}
		})
	}
}

// ── P09 订单消息与虚拟号（联系入口状态门禁）──

func TestSpecContract_Contact_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-CT-" + strconv.Itoa(round)
			orderID := "ORD-CT-" + strconv.Itoa(round)
			seedSpecOrder(h, orderID, accountID, OrderStateWaiting)

			// 未接单 → 联系入口关闭。
			rec, body := specCall(t, h, http.MethodPost, "/api/pax/order/message",
					specMessageRequest{OrderID: orderID, Text: "我在门口", From: "pax"}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "未接单发消息")

			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/virtual-call",
					specMessageRequest{OrderID: orderID}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "未接单虚拟号")

			h.store.mu.Lock()
			h.store.orders[orderID].State = OrderStateAccepted
			h.store.mu.Unlock()

			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/message",
					specMessageRequest{OrderID: orderID, Text: "我在门口", From: "pax"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "已接单发消息")

			rec, body = specCall(t, h, http.MethodGet, "/api/pax/order/message?order_id="+orderID, nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "消息列表")
			msgs, _ := body["data"].(map[string]any)["messages"].([]any)
			if len(msgs) != 1 {
				t.Fatalf("期望 1 条消息，实际 %d", len(msgs))
			}

			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/virtual-call",
					specMessageRequest{OrderID: orderID}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "已接单虚拟号")
			if number, _ := body["data"].(map[string]any)["virtual_number"].(string); number == "" {
				t.Fatalf("虚拟号不应为空：%v", body["data"])
			}

			// 行程结束 → 入口立即关闭。
			h.store.mu.Lock()
			h.store.orders[orderID].State = OrderStateEnded
			h.store.mu.Unlock()
			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/message",
					specMessageRequest{OrderID: orderID, Text: "谢谢"}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "结束后发消息")
		})
	}
}

// ── P10 变更终点：拼车禁改 / 版本乐观锁 ──

func TestSpecContract_ChangeEndpoint_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-CE-" + strconv.Itoa(round)
			orderID := "ORD-CE-" + strconv.Itoa(round)
			seedSpecOrder(h, orderID, accountID, OrderStateInTrip)

			// 拼车禁止单方改点。
			h.store.mu.Lock()
			h.store.orders[orderID].Type = "pinche"
			h.store.mu.Unlock()
			rec, body := specCall(t, h, http.MethodPost, "/api/pax/order/change-endpoint",
					specChangeEndpointRequest{OrderID: orderID, NewEndID: 3, Version: 1}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "拼车禁改点")

			h.store.mu.Lock()
			h.store.orders[orderID].Type = "kuaiche"
			h.store.mu.Unlock()

			// 版本冲突。
			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/change-endpoint",
					specChangeEndpointRequest{OrderID: orderID, NewEndID: 3, Version: 99}, accountID)
			specCode(t, rec, body, http.StatusConflict, httpx.PaxCodeConflict, "改点版本冲突")

			// 版本一致 → 成功。
			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/change-endpoint",
					specChangeEndpointRequest{OrderID: orderID, NewEndID: 3, Version: 1}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "改点成功")
			if name, _ := body["data"].(map[string]any)["new_end_name"].(string); name != "虹桥火车站" {
				t.Fatalf("新终点名称不匹配：%v", body["data"])
			}
		})
	}
}

// ── P11 行程分享：创建 / 访问 / 撤销 / 过期 ──

func TestSpecContract_Share_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-SH-" + strconv.Itoa(round)
			orderID := "ORD-SH-" + strconv.Itoa(round)
			seedSpecOrder(h, orderID, accountID, OrderStateInTrip)

			rec, body := specCall(t, h, http.MethodPost, "/api/pax/order/share",
					specShareRequest{OrderID: orderID, Action: "create"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "创建分享")
			token, _ := body["data"].(map[string]any)["share_token"].(string)
			if token == "" {
				t.Fatalf("分享令牌不应为空：%v", body["data"])
			}

			rec, body = specCall(t, h, http.MethodGet, "/api/share/"+token, nil, "")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "访问分享")
			route := body["data"].(map[string]any)["route"].(map[string]any)
			if route["state"] != OrderStateInTrip {
				t.Fatalf("分享页状态不匹配：%v", route)
			}

			rec, body = specCall(t, h, http.MethodPost, "/api/pax/order/share",
					specShareRequest{OrderID: orderID, Action: "revoke"}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "撤销分享")

			rec, _ = specCall(t, h, http.MethodGet, "/api/share/"+token, nil, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("撤销后应拒绝访问，实际 %d", rec.Code)
			}
		})
	}
}

// ── P13–P16 支付链路：收银台 / 幂等 / 回调去重 / 查单 / 关单 ──

func TestSpecContract_Payment_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			h, _ := newSpecContractHandlerForTest(t)
			accountID := "PAX-PY-" + strconv.Itoa(round)
			orderID := "ORD-PY-" + strconv.Itoa(round)
			billID := "BILL-PY-" + strconv.Itoa(round)

			order := seedSpecOrder(h, orderID, accountID, OrderStateEnded)
			h.store.mu.Lock()
			order.DriverID = "DRV-PY-" + strconv.Itoa(round)
			h.store.bills[billID] = &specBill{
				ID: billID, OrderID: orderID, Kind: "trip",
				T: 1870, Q: 1700, H: 170, F: 1870, K: 340,
				P: 1870, D: 1360, N: 510, G: 0,
				State: BillStateUnpaid,
			}
			h.store.ensureFundLocked(order.DriverID)
			h.store.mu.Unlock()

			// 流水基线：全局流水在三轮之间会累积，只校验本轮增量。
			h.store.mu.RLock()
			flowsBase := len(h.store.flows)
			h.store.mu.RUnlock()

			// 收银台金额来自账单。
			rec, body := specCall(t, h, http.MethodGet, "/api/pax/pay/checkout?order_id="+orderID+"&bill_type=trip", nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "收银台")
			bill := body["data"].(map[string]any)["bill"].(map[string]any)
			if payable, _ := bill["P"].(float64); int64(payable) != 1870 {
				t.Fatalf("收银台金额应取账单，实际 %v", bill["P"])
			}

			// 发起支付。
			key := "pay-key-round-" + strconv.Itoa(round)
			rec, body = specCall(t, h, http.MethodPost, "/api/pax/pay",
					specPayRequest{OrderID: orderID, BillType: "trip", Channel: "wechat", IdemKey: key}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "发起支付")
			if state, _ := body["data"].(map[string]any)["pay_state"].(string); state != PayStateProcessing {
				t.Fatalf("期望 PROCESSING，实际 %v", body["data"])
			}

			// 幂等重放返回首次结果。
			rec, body = specCall(t, h, http.MethodPost, "/api/pax/pay",
					specPayRequest{OrderID: orderID, BillType: "trip", Channel: "wechat", IdemKey: key}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "支付幂等")
			if rec.Header().Get("X-Idempotent-Replay") != "true" {
				t.Fatalf("重放应带 X-Idempotent-Replay 标记")
			}

			// 回调成功（验签 + 金额一致）。
			trade := "trade-round-" + strconv.Itoa(round)
			rec, body = specCall(t, h, http.MethodPost, "/api/callback/pay",
					specPayCallbackRequest{ChannelTradeNo: trade, Channel: "wechat",
						BillID: billID, AmountFen: 1870, Sign: "sig"}, "")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "支付回调")
			if result, _ := body["data"].(map[string]any)["result"].(string); result != "SUCCESS" {
				t.Fatalf("期望 SUCCESS，实际 %v", body["data"])
			}

			h.store.mu.RLock()
			pendingAfter := h.store.funds[order.DriverID].U
			h.store.mu.RUnlock()
			if pendingAfter != 1360 {
				t.Fatalf("司机待结算应为 D=1360，实际 %d", pendingAfter)
			}

			// 重复成功回调不重复入账。
			rec, body = specCall(t, h, http.MethodPost, "/api/callback/pay",
					specPayCallbackRequest{ChannelTradeNo: trade, Channel: "wechat",
						BillID: billID, AmountFen: 1870, Sign: "sig"}, "")
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "重复回调")
			if repeated, _ := body["data"].(map[string]any)["repeated"].(bool); !repeated {
				t.Fatalf("重复回调应标记 repeated=true，实际 %v", body["data"])
			}
			h.store.mu.RLock()
			stillPending := h.store.funds[order.DriverID].U
			flowsAfter := len(h.store.flows)
			h.store.mu.RUnlock()
			if stillPending != 1360 {
				t.Fatalf("重复回调不应重复入账，实际 %d", stillPending)
			}
			// 只校验本轮增量：全局流水在三轮之间会累积。
			if flowsAfter != flowsBase+1 {
				t.Fatalf("司机收入流水本轮应只新增 1 条，实际新增 %d", flowsAfter-flowsBase)
			}

			// 查单与关单：已 SUCCESS 的账单不被回退。
			rec, body = specCall(t, h, http.MethodGet, "/api/pax/pay/query?bill_id="+billID, nil, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "支付查单")
			if state, _ := body["data"].(map[string]any)["pay_state"].(string); state != PayStateSuccess {
				t.Fatalf("期望 SUCCESS，实际 %v", body["data"])
			}

			rec, body = specCall(t, h, http.MethodPost, "/api/pax/pay/close",
					specPayQueryRequest{BillID: billID}, accountID)
			specCode(t, rec, body, http.StatusOK, httpx.PaxCodeOK, "支付关单")
			if state, _ := body["data"].(map[string]any)["pay_state"].(string); state != PayStateSuccess {
				t.Fatalf("关单不应回退已成功状态，实际 %v", body["data"])
			}
		})
	}
}

// ── 计价公式与资金守恒（文档 4.1）──

func TestSpecComputeFare_ThreeRounds(t *testing.T) {
	cfg := specPricingConfig{
		Start: 1300, Incl: 3000, PerKM: 260, PerMin: 50,
		ServiceBP: 1000, CommBP: 2000, SurgeBP: 10000, DiscountBP: 7000,
	}
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			// 6.5km / 16min，券 500 分（平台承担），溢出系数 10000。
			// T = 1300 + roundDiv(3500*260, 1000) + roundDiv(960*50, 60)
			//   = 1300 + 910 + 800 = 3010
			fare := computeFare(cfg, 6500, 960, 500, 0, false)
			if fare.T != 3010 {
				t.Fatalf("T 期望 3010，实际 %d", fare.T)
			}
			if fare.Q != fare.T {
				t.Fatalf("无溢价时 Q 应等于 T，实际 Q=%d T=%d", fare.Q, fare.T)
			}
			if fare.H != 301 || fare.K != 602 {
				t.Fatalf("分账明细不符：H=%d K=%d", fare.H, fare.K)
			}
			if fare.P != fare.F-500 || fare.D != fare.Q-fare.K || fare.N != fare.H+fare.K-500 {
				t.Fatalf("分账口径不符：%+v", fare)
			}
			// 资金守恒：P = D + N + G
			if fare.P != fare.D+fare.N+fare.G {
				t.Fatalf("资金不守恒：P=%d D=%d N=%d G=%d", fare.P, fare.D, fare.N, fare.G)
			}

			// 拼车走固定折扣。
			carpool := computeFare(cfg, 6500, 960, 0, 0, true)
			if carpool.Q >= fare.Q {
				t.Fatalf("拼车应有折扣：Q=%d 原价=%d", carpool.Q, fare.Q)
			}
			if carpool.P != carpool.D+carpool.N+carpool.G {
				t.Fatalf("拼车资金不守恒：%+v", carpool)
			}

			// 短途不超过包含里程时只收起步价。
			short := computeFare(cfg, 1000, 300, 0, 0, false)
			if short.T != cfg.Start+specRoundDiv(300*cfg.PerMin, 60) {
				t.Fatalf("短途计价不符：%+v", short)
			}
		})
	}
}

// ── 退款尾差（文档 T14）──

func TestSpecDriverBear_ThreeRounds(t *testing.T) {
	for round := 1; round <= 3; round++ {
		t.Run("round"+strconv.Itoa(round), func(t *testing.T) {
			// D0=1000, P0=2000：分两次各退 1000 分。
			first := specDriverBear(1000, 2000, 0, 1000, "PROPORTIONAL")
			second := specDriverBear(1000, 2000, 1000, 1000, "PROPORTIONAL")
			if first != 500 {
				t.Fatalf("首次退款司机承担期望 500，实际 %d", first)
			}
			if second != 500 {
				t.Fatalf("二次退款司机承担期望 500，实际 %d", second)
			}
			// 累计口径验证：一次退全额应等于两次之和，无尾差累积。
			all := specDriverBear(1000, 2000, 0, 2000, "PROPORTIONAL")
			if all != first+second {
				t.Fatalf("尾差累积：一次退全额=%d，分两次=%d", all, first+second)
			}
			// 平台承担模式司机不担。
			if got := specDriverBear(1000, 2000, 0, 1000, "PLATFORM"); got != 0 {
				t.Fatalf("PLATFORM 模式司机承担应为 0，实际 %d", got)
			}
		})
	}
}
