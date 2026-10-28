package router_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/client"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/router"
	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// paxRouteCase 是一条乘客端路由（9 条，契约 3.6/3.7）。
type paxRouteCase struct {
	name   string
	method string
	path   string
	body   string
}

func paxRouteCases() []paxRouteCase {
	return []paxRouteCase{
		{"订单列表", http.MethodGet, "/api/pax/orders?filter=全部", ""},
		{"订单详情", http.MethodGet, "/api/pax/orders/C1", ""},
		{"提交评价", http.MethodPost, "/api/pax/order/rate", `{"order_id":"C1","rating":5}`},
		{"提交投诉", http.MethodPost, "/api/pax/order/complaint", `{"order_id":"C1","type":"绕路","desc":"绕路","images":[],"idempotency_key":"k1","passenger_id":"evil-9"}`},
		{"实名查询", http.MethodGet, "/api/pax/realname", ""},
		{"实名提交", http.MethodPost, "/api/pax/realname", `{"name":"张三丰","id_no":"110101199001011234","idempotency_key":"k2"}`},
		{"账号安全", http.MethodGet, "/api/pax/account-security", ""},
		{"协议记录", http.MethodPost, "/api/pax/account-security", `{"protocol_ver":"v2.0"}`},
		{"客服页", http.MethodGet, "/api/pax/service", ""},
	}
}

// paxJWT 是测试令牌签发配置（与既有 router 测试一致）。
func paxJWT() middleware.JWTConfig {
	return middleware.JWTConfig{Secret: "test-secret", Issuer: "xiaolanshu", ExpireSeconds: 7200}
}

// signPaxToken 用 middleware.GenerateToken 本地签发令牌（验收口径）。
func signPaxToken(t *testing.T, userID, role string) string {
	t.Helper()
	token, err := middleware.GenerateToken(paxJWT(), middleware.JWTClaims{UserID: userID, Role: role})
	require.NoError(t, err)
	return token
}

// buildPaxEngine 构造注入 fake 后端的 gateway engine。
func buildPaxEngine(order *e2eOrderClient, user *e2eUserClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return router.New(&client.Clients{
		User:  user,
		Note:  fakeNoteClient{},
		Order: order,
	}, zap.NewNop(), config.MiddlewareConfig{JWT: paxJWT()})
}

// paxBody 是 pax 契约信封。
type paxBody struct {
	Code int             `json:"code"`
	Data json.RawMessage `json:"data"`
}

// TestPaxRoutes_RequirePaxToken：9 条路由鉴权收口（附录 C#1）——无令牌 401、非 pax 角色 403。
func TestPaxRoutes_RequirePaxToken(t *testing.T) {
	engine := buildPaxEngine(&e2eOrderClient{}, &e2eUserClient{})
	driverToken := signPaxToken(t, "drv-1", "driver")

	for _, tc := range paxRouteCases() {
		// 无令牌 → 401（code 40101）
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusUnauthorized, rec.Code, "%s 无令牌应 401", tc.name)
		var body paxBody
		require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &body), "%s", tc.name)
		require.Equalf(t, 40101, body.Code, "%s 无令牌应 code 40101", tc.name)

		// role≠pax → 403（code 40101）
		req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+driverToken)
		rec = httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusForbidden, rec.Code, "%s 非 pax 角色应 403", tc.name)
		require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &body), "%s", tc.name)
		require.Equalf(t, 40101, body.Code, "%s 非 pax 角色应 code 40101", tc.name)
	}
}

// TestPaxRoutes_PaxTokenWalkthrough：pax 令牌走通 9 个接口（附录 C 端到端验收），
// 并复查归属过滤：后端收到的 passenger_id/account_id 恒等于令牌身份（请求体伪造不了归属）。
func TestPaxRoutes_PaxTokenWalkthrough(t *testing.T) {
	order := &e2eOrderClient{}
	user := &e2eUserClient{}
	engine := buildPaxEngine(order, user)
	token := signPaxToken(t, "pax-1", "pax")

	for _, tc := range paxRouteCases() {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		require.Equalf(t, http.StatusOK, rec.Code, "%s 应 200", tc.name)
		var body paxBody
		require.NoErrorf(t, json.Unmarshal(rec.Body.Bytes(), &body), "%s", tc.name)
		require.Equalf(t, 0, body.Code, "%s 应 code 0", tc.name)
	}

	// 归属过滤复查：全部调用的归属 ID 均取自令牌身份（客服页配置无归属，记 "-"）
	require.Equal(t, []string{"pax-1", "pax-1", "pax-1", "pax-1", "pax-1"}, order.passengerIDs)
	require.Equal(t, []string{"pax-1", "pax-1", "pax-1", "pax-1", "-"}, user.accountIDs)

	// 业务字段抽查（附录 C）
	assertDataField(t, engine, token, http.MethodGet, "/api/pax/orders", "", "list")
	assertDataField(t, engine, token, http.MethodPost, "/api/pax/order/rate", `{"order_id":"C1","rating":5}`, "rating")
	assertDataField(t, engine, token, http.MethodPost, "/api/pax/order/complaint", `{"order_id":"C1","type":"绕路","desc":"d","images":[],"idempotency_key":"k1"}`, "ticket_id")
	assertDataField(t, engine, token, http.MethodGet, "/api/pax/account-security", "", "bindings")
	assertDataField(t, engine, token, http.MethodGet, "/api/pax/service", "", "my_tickets")
	assertDataField(t, engine, token, http.MethodGet, "/api/pax/service", "", "hotline")
}

// assertDataField 断言信封 data 含指定字段。
func assertDataField(t *testing.T, engine *gin.Engine, token, method, path, body, field string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var envelope paxBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	var data map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(envelope.Data, &data))
	require.Containsf(t, data, field, "%s %s 应含字段 %s", method, path, field)
}

// e2eOrderClient 嵌接口覆写 P17–P22 的 RPC，记录归属 ID。
type e2eOrderClient struct {
	orderv1.OrderServiceClient
	passengerIDs []string
}

func (f *e2eOrderClient) ListMyOrders(_ context.Context, in *orderv1.ListMyOrdersRequest, _ ...grpc.CallOption) (*orderv1.ListMyOrdersResponse, error) {
	f.passengerIDs = append(f.passengerIDs, in.GetPassengerId())
	return &orderv1.ListMyOrdersResponse{List: []*orderv1.MyOrderListItem{}, Total: 1}, nil
}

func (f *e2eOrderClient) GetMyOrderDetail(_ context.Context, in *orderv1.GetMyOrderDetailRequest, _ ...grpc.CallOption) (*orderv1.MyOrderDetailResponse, error) {
	f.passengerIDs = append(f.passengerIDs, in.GetPassengerId())
	return &orderv1.MyOrderDetailResponse{}, nil
}

func (f *e2eOrderClient) SubmitRating(_ context.Context, in *orderv1.SubmitRatingRequest, _ ...grpc.CallOption) (*orderv1.SubmitRatingResponse, error) {
	f.passengerIDs = append(f.passengerIDs, in.GetPassengerId())
	return &orderv1.SubmitRatingResponse{OrderId: in.GetOrderId(), Rating: in.GetRating()}, nil
}

func (f *e2eOrderClient) SubmitComplaint(_ context.Context, in *orderv1.SubmitComplaintRequest, _ ...grpc.CallOption) (*orderv1.SubmitComplaintResponse, error) {
	f.passengerIDs = append(f.passengerIDs, in.GetPassengerId())
	return &orderv1.SubmitComplaintResponse{TicketId: "TK2610270001", State: "PROCESSING", StateName: "处理中"}, nil
}

func (f *e2eOrderClient) ListMyTickets(_ context.Context, in *orderv1.ListMyTicketsRequest, _ ...grpc.CallOption) (*orderv1.ListMyTicketsResponse, error) {
	f.passengerIDs = append(f.passengerIDs, in.GetPassengerId())
	return &orderv1.ListMyTicketsResponse{List: []*orderv1.MyTicketProgress{}, Total: 1}, nil
}

// e2eUserClient 嵌接口覆写 P20–P22 的 RPC，记录归属 ID。
type e2eUserClient struct {
	userv1.UserServiceClient
	accountIDs []string
}

func (f *e2eUserClient) GetRealNameStatus(_ context.Context, in *userv1.GetRealNameStatusRequest, _ ...grpc.CallOption) (*userv1.RealNameStatusResponse, error) {
	f.accountIDs = append(f.accountIDs, in.GetAccountId())
	return &userv1.RealNameStatusResponse{State: "已认证", Name: "张*丰", IdNo: "110***********1234"}, nil
}

func (f *e2eUserClient) SubmitRealName(_ context.Context, in *userv1.SubmitRealNameRequest, _ ...grpc.CallOption) (*userv1.RealNameStatusResponse, error) {
	f.accountIDs = append(f.accountIDs, in.GetAccountId())
	return &userv1.RealNameStatusResponse{State: "已认证", Name: "张*丰", IdNo: "110***********1234"}, nil
}

func (f *e2eUserClient) GetAccountSecurity(_ context.Context, in *userv1.GetAccountSecurityRequest, _ ...grpc.CallOption) (*userv1.AccountSecurityResponse, error) {
	f.accountIDs = append(f.accountIDs, in.GetAccountId())
	return &userv1.AccountSecurityResponse{
		Bindings:      []*userv1.ChannelBinding{{Channel: "wechat", Bound: true, BoundAt: 1700000000}},
		ProtocolVer:   "v2.0",
		RealNameState: "已认证",
	}, nil
}

func (f *e2eUserClient) RecordAgreement(_ context.Context, in *userv1.RecordAgreementRequest, _ ...grpc.CallOption) (*userv1.RecordAgreementResponse, error) {
	f.accountIDs = append(f.accountIDs, in.GetAccountId())
	return &userv1.RecordAgreementResponse{ProtocolVer: in.GetProtocolVer()}, nil
}

func (f *e2eUserClient) GetServiceConfig(_ context.Context, in *userv1.GetServiceConfigRequest, _ ...grpc.CallOption) (*userv1.ServiceConfigResponse, error) {
	f.accountIDs = append(f.accountIDs, "-")
	return &userv1.ServiceConfigResponse{
		Hotline:     "400-123",
		ServiceTime: "07:00-23:00",
		ServiceSpec: "",
		FaqJson:     "[]",
	}, nil
}
