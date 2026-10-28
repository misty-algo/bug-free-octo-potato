package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	userv1 "github.com/BwCloudWeGo/bw-cli/api/gen/user/v1"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// PaxServiceHandler 处理乘客端客服页（P22，契约 3.7.3）。
// gateway 依次调 order 域 ListMyTickets（工单进度）与 user 域 GetServiceConfig（客服配置），合并返回契约结构。
type PaxServiceHandler struct {
	order orderv1.OrderServiceClient
	user  userv1.UserServiceClient
	log   *zap.Logger
}

// NewPaxServiceHandler 创建客服页 handler。
func NewPaxServiceHandler(order orderv1.OrderServiceClient, user userv1.UserServiceClient, log *zap.Logger) *PaxServiceHandler {
	return &PaxServiceHandler{order: order, user: user, log: log}
}

// servicePageData 是客服页契约 3.7.3 响应 data 载荷。
type servicePageData struct {
	FAQ         json.RawMessage             `json:"faq"` // JSON 数组；无记录缺省 []
	Hotline     string                      `json:"hotline"`
	ServiceTime string                      `json:"service_time"`
	MyTickets   []*orderv1.MyTicketProgress `json:"my_tickets"`
}

// GetService 将 GET /api/pax/service 合并返回 my_tickets 与 faq/hotline/service_time（契约 3.7.3）。
func (h *PaxServiceHandler) GetService(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	ticketsResp, err := h.order.ListMyTickets(outgoingContext(c), &orderv1.ListMyTicketsRequest{PassengerId: claims.UserID})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	cfgResp, err := h.user.GetServiceConfig(outgoingContext(c), &userv1.GetServiceConfigRequest{})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	myTickets := ticketsResp.GetList()
	if myTickets == nil {
		myTickets = []*orderv1.MyTicketProgress{}
	}
	httpx.PaxOK(c, servicePageData{
		FAQ:         faqRaw(cfgResp.GetFaqJson()),
		Hotline:     cfgResp.GetHotline(),
		ServiceTime: cfgResp.GetServiceTime(),
		MyTickets:   myTickets,
	})
}

// faqRaw 校验 FAQ JSON 数组文本；空/非法缺省 []（不 500）。
func faqRaw(value string) json.RawMessage {
	raw := json.RawMessage(value)
	if len(value) == 0 || !json.Valid(raw) || raw[0] != '[' {
		return json.RawMessage("[]")
	}
	return raw
}
