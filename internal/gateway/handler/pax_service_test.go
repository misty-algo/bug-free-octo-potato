package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// servicePageOrderClient 嵌接口只覆写 ListMyTickets，并记录收到的归属 ID。
type servicePageOrderClient struct {
	orderv1.OrderServiceClient
	passengerID string
}

func (f *servicePageOrderClient) ListMyTickets(_ context.Context, in *orderv1.ListMyTicketsRequest, _ ...grpc.CallOption) (*orderv1.ListMyTicketsResponse, error) {
	f.passengerID = in.GetPassengerId()
	return &orderv1.ListMyTicketsResponse{
		Total: 2,
		List: []*orderv1.MyTicketProgress{
			{
				TicketId: "TK2610270001", TicketNo: "TK2610270001", OrderNo: "C1",
				Type: "绕路", State: "PROCESSING", StateName: "处理中",
				Round: 2, CreatedAt: "2026-10-27T10:00:00Z",
				Timeline: []*orderv1.TicketTimelineItem{
					{Seq: 1, Actor: "系统", Action: "创建", At: "2026-10-27T10:00:00Z"},
					{Seq: 2, Actor: "系统", Action: "重新分配", At: "2026-10-27T12:00:00Z"},
				},
			},
			{
				TicketId: "TK2610270002", TicketNo: "TK2610270002", OrderNo: "C2",
				Type: "其他", State: "RESOLVED", StateName: "已完结",
				Round: 1, CreatedAt: "2026-10-27T11:00:00Z",
			},
		},
	}, nil
}

// servicePageUserClient 嵌接口只覆写 GetServiceConfig。
type servicePageUserClient struct {
	userv1.UserServiceClient
	faqJSON string
}

func (f *servicePageUserClient) GetServiceConfig(context.Context, *userv1.GetServiceConfigRequest, ...grpc.CallOption) (*userv1.ServiceConfigResponse, error) {
	return &userv1.ServiceConfigResponse{
		Hotline:     "400-123",
		ServiceTime: "07:00-23:00",
		ServiceSpec: "",
		FaqJson:     f.faqJSON,
	}, nil
}

// servicePageBody 是客服页契约 3.7.3 响应信封（data 子集）。
type servicePageBody struct {
	Code int `json:"code"`
	Data struct {
		FAQ         json.RawMessage `json:"faq"`
		Hotline     string          `json:"hotline"`
		ServiceTime string          `json:"service_time"`
		MyTickets   []struct {
			TicketID  string           `json:"ticket_id"`
			TicketNo  string           `json:"ticket_no"`
			State     string           `json:"state"`
			StateName string           `json:"state_name"`
			Round     int              `json:"round"`
			Timeline  []map[string]any `json:"timeline"`
		} `json:"my_tickets"`
	} `json:"data"`
}

// TestGetService_MergesTicketsAndConfig：faq/hotline/service_time/my_tickets 合并、工单带进度。
func TestGetService_MergesTicketsAndConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtCfg := middleware.JWTConfig{Secret: "test-secret", Issuer: "test"}
	token, err := middleware.GenerateToken(jwtCfg, middleware.JWTClaims{UserID: "pax-1", Role: "pax"})
	require.NoError(t, err)

	orderClient := &servicePageOrderClient{}
	userClient := &servicePageUserClient{faqJSON: `[{"q":"如何开发票","a":"行程结束后在订单详情申请"}]`}
	h := NewPaxServiceHandler(orderClient, userClient, zap.NewNop())

	router := gin.New()
	router.GET("/api/pax/service", middleware.JWTAuth(jwtCfg), middleware.RequireRole("pax"), h.GetService)

	req := httptest.NewRequest(http.MethodGet, "/api/pax/service", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "pax-1", orderClient.passengerID)

	var body servicePageBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, 0, body.Code)
	require.JSONEq(t, `[{"q":"如何开发票","a":"行程结束后在订单详情申请"}]`, string(body.Data.FAQ))
	require.Equal(t, "400-123", body.Data.Hotline)
	require.Equal(t, "07:00-23:00", body.Data.ServiceTime)

	// my_tickets 带进度：state/round/timeline 齐全
	require.Len(t, body.Data.MyTickets, 2)
	first := body.Data.MyTickets[0]
	require.Equal(t, "TK2610270001", first.TicketID)
	require.Equal(t, "TK2610270001", first.TicketNo)
	require.Equal(t, "PROCESSING", first.State)
	require.Equal(t, "处理中", first.StateName)
	require.Equal(t, 2, first.Round)
	require.Len(t, first.Timeline, 2)
	require.Equal(t, "创建", first.Timeline[0]["action"])
	require.Equal(t, "已完结", body.Data.MyTickets[1].StateName)

	// FAQ 无记录：缺省空数组（不 500）
	userClient.faqJSON = ""
	req2 := httptest.NewRequest(http.MethodGet, "/api/pax/service", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	require.Equal(t, http.StatusOK, w2.Code)
	var body2 servicePageBody
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body2))
	require.JSONEq(t, "[]", string(body2.Data.FAQ))
	require.Len(t, body2.Data.MyTickets, 2)
}
