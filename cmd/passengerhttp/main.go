// Command passengerhttp 走 HTTP 网关跑一遍乘客端真实链路，用于验证网关层。
//
// 与 cmd/passengertest 的区别：那个直连 gRPC，这个走完整的
// HTTP → 网关 → gRPC → 业务域 → 数据库链路。
//
// 运行方式：
//   go run ./cmd/passengerhttp -base http://127.0.0.1:8088
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	base := flag.String("base", "http://127.0.0.1:8088", "网关地址")
	flag.Parse()
	baseURL := strings.TrimRight(*base, "/")

	client := &http.Client{Timeout: 15 * time.Second}
	step := 0

	// 1. 健康检查。
	resp := do(client, http.MethodGet, baseURL+"/healthz", "", nil)
	if resp.status != http.StatusOK {
		fail("healthz status = %d, want 200", resp.status)
	}
	fmt.Printf("[1] GET /healthz ... OK %s\n", strings.TrimSpace(resp.body))
	step++

	// 2. 游客看价：拿不到估价单。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/estimate/quote", "", map[string]any{
		"carType": 1, "startLng": 121.601, "startLat": 31.2054,
		"endLng": 121.3208, "endLat": 31.1979,
	})
	if resp.status != http.StatusOK {
		fail("quote status = %d, want 200: %s", resp.status, resp.body)
	}
	quote := resp.data
	estimateNo, _ := quote["estimate_no"].(string)
	if estimateNo != "" {
		fail("quote returned an estimate number %q for a visitor", estimateNo)
	}
	fmt.Printf("[2] POST /passenger/estimate/quote ... OK pay_fee=%v estimate_no=<empty as expected>\n", quote["pay_fee"])
	step++

	// 3. 发验证码。
	suffix := time.Now().UnixNano() % 100000000
	phone := fmt.Sprintf("138%08d", suffix)
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/sms-code", "", map[string]any{"phone": phone})
	if resp.status != http.StatusOK {
		fail("sms-code status = %d, want 200: %s", resp.status, resp.body)
	}
	code, _ := resp.data["debug_code"].(string)
	if code == "" {
		fail("sms-code returned no debug code")
	}
	fmt.Printf("[3] POST /passenger/auth/sms-code ... OK code=%s\n", code)
	step++

	// 4. 验证码登录。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/login", "", map[string]any{
		"channel": "sms", "phone": phone, "code": code, "role": "passenger",
	})
	if resp.status != http.StatusOK {
		fail("login status = %d, want 200: %s", resp.status, resp.body)
	}
	login := resp.data
	accountID, _ := login["account_id"].(string)
	accessToken, _ := login["access_token"].(string)
	if accountID == "" || accessToken == "" {
		fail("login returned no account id or token")
	}
	fmt.Printf("[4] POST /passenger/auth/login ... OK accountId=%s tokenLen=%d restoreTarget=%v\n",
		accountID, len(accessToken), login["restore_target"])
	step++

	// 5. 首页。
	resp = do(client, http.MethodGet, baseURL+"/api/v1/passenger/home?accountId="+accountID, accountID, nil)
	if resp.status != http.StatusOK {
		fail("home status = %d, want 200: %s", resp.status, resp.body)
	}
	home := resp.data
	carTypes, _ := home["car_types"].([]any)
	quickEntries, _ := home["quick_entries"].([]any)
	fmt.Printf("[5] GET /passenger/home ... OK carTypes=%d quickEntries=%d\n", len(carTypes), len(quickEntries))
	step++

	// 6. POI 搜索。
	query := url.Values{}
	query.Set("accountId", accountID)
	query.Set("keyword", "张江")
	query.Set("hasCoord", "true")
	query.Set("lng", "121.601")
	query.Set("lat", "31.2054")
	query.Set("limit", "10")
	resp = do(client, http.MethodGet,
		baseURL+"/api/v1/passenger/poi/search?"+query.Encode(),
		accountID, nil)
	if resp.status != http.StatusOK {
		fail("search status = %d, want 200: %s", resp.status, resp.body)
	}
	pois, _ := resp.data["list"].([]any)
	fmt.Printf("[6] GET /passenger/poi/search ... OK results=%d\n", len(pois))
	step++

	// 7. 保存常用地址。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/addresses", accountID, map[string]any{
		"type": "home", "tag": "家", "name": "家", "addr": "上海市浦东新区科苑路 88 号",
		"lng": 121.601, "lat": 31.2054,
	})
	if resp.status != http.StatusOK {
		fail("save address status = %d, want 200: %s", resp.status, resp.body)
	}
	address, _ := resp.data["address"].(map[string]any)
	addressID, _ := address["id"].(string)
	fmt.Printf("[7] POST /passenger/addresses ... OK addressId=%s\n", addressID)
	step++

	// 8. 地址列表。
	resp = do(client, http.MethodGet, baseURL+"/api/v1/passenger/addresses?accountId="+accountID+"&type=home", accountID, nil)
	if resp.status != http.StatusOK {
		fail("list addresses status = %d, want 200: %s", resp.status, resp.body)
	}
	addresses, _ := resp.data["list"].([]any)
	fmt.Printf("[8] GET /passenger/addresses ... OK count=%d\n", len(addresses))
	step++

	// 9. 上报历史地址。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/addresses/history", accountID, map[string]any{
		"poiId": "poi_1", "name": "张江镇科苑路", "addr": "科苑路 88 号",
		"lng": 121.601, "lat": 31.2054,
	})
	if resp.status != http.StatusOK {
		fail("record history status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[9] POST /passenger/addresses/history ... OK\n")
	step++

	// 10. 快速入口标记已用。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/home/quick-entries/use", accountID, map[string]any{
		"entryId": "scan_ride",
	})
	if resp.status != http.StatusOK {
		fail("use quick entry status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[10] POST /passenger/home/quick-entries/use ... OK\n")
	step++

	// 11. 绑定手机号，返回掩码。
	boundPhone := fmt.Sprintf("139%08d", suffix+1)
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/bind-phone", accountID, map[string]any{
		"phone": boundPhone,
	})
	if resp.status != http.StatusOK {
		fail("bind phone status = %d, want 200: %s", resp.status, resp.body)
	}
	masked, _ := resp.data["phone"].(string)
	if masked != boundPhone[:3]+"****"+boundPhone[7:] {
		fail("bind phone returned %q, want the masked form", masked)
	}
	fmt.Printf("[11] POST /passenger/auth/bind-phone ... OK phone=%s (masked)\n", masked)
	step++

	// 12. 设置密码。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/sms-code", "", map[string]any{"phone": boundPhone})
	if resp.status != http.StatusOK {
		fail("sms-code for password status = %d, want 200: %s", resp.status, resp.body)
	}
	pwdCode, _ := resp.data["debug_code"].(string)
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/set-password", accountID, map[string]any{
		"phone": boundPhone, "code": pwdCode, "password": "abc123",
	})
	if resp.status != http.StatusOK {
		fail("set password status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[12] POST /passenger/auth/set-password ... OK\n")
	step++

	// 13. 密码登录。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/login", "", map[string]any{
		"channel": "password", "phone": boundPhone, "password": "abc123", "role": "passenger",
	})
	if resp.status != http.StatusOK {
		fail("password login status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[13] POST /passenger/auth/login(password) ... OK hasPassword=%v\n", resp.data["has_password"])
	step++

	// 14. 微信登录。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/login", "", map[string]any{
		"channel": "wechat", "code": "wx-code-1", "role": "passenger",
	})
	if resp.status != http.StatusOK {
		fail("wechat login status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[14] POST /passenger/auth/login(wechat) ... OK accountId=%v\n", resp.data["account_id"])
	step++

	// 15. 估价（需要登录）。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/estimate", accountID, map[string]any{
		"accountId": accountID,
		"carType":   1, "startLng": 121.601, "startLat": 31.2054,
		"endLng": 121.3208, "endLat": 31.1979,
	})
	if resp.status != http.StatusOK {
		fail("estimate status = %d, want 200: %s", resp.status, resp.body)
	}
	estimateNo, _ = resp.data["estimate_no"].(string)
	if estimateNo == "" {
		fail("estimate returned no estimate number for a signed-in account")
	}
	fmt.Printf("[15] POST /passenger/estimate ... OK estimateNo=%s pay_fee=%v\n", estimateNo, resp.data["pay_fee"])
	step++

	// 16. 游客下单被拒。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/orders", "", map[string]any{
		"estimateNo": estimateNo, "carType": 1,
		"startLng": 121.601, "startLat": 31.2054, "endLng": 121.3208, "endLat": 31.1979,
		"idempotencyKey": "key-guest",
	})
	if resp.status == http.StatusOK {
		fail("a visitor was allowed to create an order")
	}
	fmt.Printf("[16] POST /passenger/orders(visitor) ... OK rejected with HTTP %d\n", resp.status)
	step++

	// 17. 刷新令牌。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/refresh", "", map[string]any{
		"refreshToken": login["refresh_token"], "audience": "pax",
	})
	if resp.status != http.StatusOK {
		fail("refresh status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[17] POST /passenger/auth/refresh ... OK\n")
	step++

	// 18. 退出登录。
	resp = do(client, http.MethodPost, baseURL+"/api/v1/passenger/auth/logout", accountID, map[string]any{})
	if resp.status != http.StatusOK {
		fail("logout status = %d, want 200: %s", resp.status, resp.body)
	}
	fmt.Printf("[18] POST /passenger/auth/logout ... OK\n")
	step++

	fmt.Printf("\nHTTP 网关链路验证通过，共 %d 步\n", step)
}

// apiResponse 是网关统一响应外壳。
type apiResponse struct {
	status int
	body   string
	data   map[string]any
}

// do 发一个 HTTP 请求并解析统一响应外壳。
func do(client *http.Client, method, url, accountID string, payload any) *apiResponse {
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			fail("marshal payload: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		fail("build request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if accountID != "" {
		request.Header.Set("X-Account-Id", accountID)
	}

	response, err := client.Do(request)
	if err != nil {
		fail("%s %s: %v", method, url, err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	raw, err := io.ReadAll(response.Body)
	if err != nil {
		fail("read body: %v", err)
	}
	out := &apiResponse{status: response.StatusCode, body: string(raw)}
	var envelope struct {
		Data  map[string]any `json:"data"`
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil {
		out.data = envelope.Data
	}
	return out
}

// fail 打印错误并退出。
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}
