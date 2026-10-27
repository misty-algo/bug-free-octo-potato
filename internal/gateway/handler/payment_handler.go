package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	paymentv1 "github.com/BwCloudWeGo/bw-cli/api/gen/payment/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// PaymentHandler 将 payment HTTP 接口适配到生成的 gRPC client。
type PaymentHandler struct {
	client paymentv1.PaymentServiceClient
	log    *zap.Logger
}

// NewPaymentHandler 将 payment gRPC client 注入 HTTP handler 方法。
func NewPaymentHandler(client paymentv1.PaymentServiceClient, log *zap.Logger) *PaymentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &PaymentHandler{client: client, log: log}
}

// Create 将 POST /api/v1/payments 代理到 CreatePayment。
func (h *PaymentHandler) Create(c *gin.Context) {
	var req request.CreatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreatePayment(outgoingContext(c), &paymentv1.CreatePaymentRequest{
		OrderNo:       req.OrderNo,
		Subject:       req.Subject,
		Amount:        req.Amount,
		Channel:       req.Channel,
		UserId:        req.UserID,
		IdempotentKey: req.IdempotentKey,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway payment create proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.Created(c, resp)
}

// Notify 将 POST /api/v1/payments/notify 代理到 NotifyPayment。
func (h *PaymentHandler) Notify(c *gin.Context) {
	var req request.NotifyPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.NotifyPayment(outgoingContext(c), &paymentv1.NotifyPaymentRequest{
		OrderNo:    req.OrderNo,
		TradeNo:    req.TradeNo,
		Channel:    req.Channel,
		PaidAmount: req.PaidAmount,
		PaidAtUnix: req.PaidAtUnix,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway payment notify proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetPaymentNo()))
	httpx.OK(c, resp)
}

// Query 将 GET /api/v1/payments/:order_no 代理到 QueryPayment。
func (h *PaymentHandler) Query(c *gin.Context) {
	resp, err := h.client.QueryPayment(outgoingContext(c), &paymentv1.QueryPaymentRequest{OrderNo: c.Param("order_no")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Refund 将 POST /api/v1/payments/refund 代理到 RefundPayment。
func (h *PaymentHandler) Refund(c *gin.Context) {
	var req request.RefundPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.RefundPayment(outgoingContext(c), &paymentv1.RefundPaymentRequest{
		OrderNo:      req.OrderNo,
		RefundAmount: req.RefundAmount,
		Reason:       req.Reason,
		OutRequestNo: req.OutRequestNo,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway payment refund proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}
