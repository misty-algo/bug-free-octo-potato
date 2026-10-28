package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// PaxOrderHandler 将乘客端订单 HTTP 接口适配到 order gRPC client（P17-P18）。
// 路由挂在 /api/pax 下，经 JWTAuth + RequireRole("pax") 鉴权；
// 归属身份一律取自 claims.UserID，不接受客户端传参（D4）。
type PaxOrderHandler struct {
	client orderv1.OrderServiceClient
	log    *zap.Logger
}

// NewPaxOrderHandler 注入 order gRPC client。
func NewPaxOrderHandler(client orderv1.OrderServiceClient, log *zap.Logger) *PaxOrderHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &PaxOrderHandler{client: client, log: log}
}

// ListOrders 将 GET /api/pax/orders 代理到 ListMyOrders（契约 3.6.1）。
func (h *PaxOrderHandler) ListOrders(c *gin.Context) {
	var req request.ListPaxOrderRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, err.Error())
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.ListMyOrders(outgoingContext(c), &orderv1.ListMyOrdersRequest{
		PassengerId: claims.UserID,
		Filter:      req.Filter,
		Page:        req.Page,
		Size:        req.Size,
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, gin.H{"list": resp.GetList(), "total": resp.GetTotal()})
}

// GetOrderDetail 将 GET /api/pax/orders/:order_id 代理到 GetMyOrderDetail（契约 3.6.2）。
func (h *PaxOrderHandler) GetOrderDetail(c *gin.Context) {
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.GetMyOrderDetail(outgoingContext(c), &orderv1.GetMyOrderDetailRequest{
		PassengerId: claims.UserID,
		OrderId:     c.Param("order_id"),
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// RateOrder 将 POST /api/pax/order/rate 代理到 SubmitRating（契约 3.6.3）。
func (h *PaxOrderHandler) RateOrder(c *gin.Context) {
	var req request.RatePaxOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, err.Error())
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.SubmitRating(outgoingContext(c), &orderv1.SubmitRatingRequest{
		PassengerId: claims.UserID,
		OrderId:     req.OrderID,
		Rating:      req.Rating,
		Anonymous:   req.Anonymous,
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}

// SubmitComplaint 将 POST /api/pax/order/complaint 代理到 SubmitComplaint（契约 3.6.4）。
func (h *PaxOrderHandler) SubmitComplaint(c *gin.Context) {
	var req request.ComplaintPaxOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, err.Error())
		return
	}
	claims := middleware.ClaimsFromContext(c)
	if claims.UserID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	resp, err := h.client.SubmitComplaint(outgoingContext(c), &orderv1.SubmitComplaintRequest{
		PassengerId:    claims.UserID,
		OrderId:        req.OrderID,
		Type:           req.Type,
		Desc:           req.Desc,
		Images:         req.Images,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.PaxAppError(c, apperrors.FromGRPC(err))
		return
	}
	httpx.PaxOK(c, resp)
}
