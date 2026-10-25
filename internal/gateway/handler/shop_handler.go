package handler

import (
	"io"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// ShopHandler 将 shop HTTP 接口适配到生成的 gRPC client。
type ShopHandler struct {
	client shopv1.ShopServiceClient
	log    *zap.Logger
}

// NewShopHandler 将 shop gRPC client 注入 HTTP handler 方法。
func NewShopHandler(client shopv1.ShopServiceClient, log *zap.Logger) *ShopHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &ShopHandler{
		client: client,
		log:    log,
	}
}

// Create 将 POST /api/v1/shops 代理到 CreateShop。
func (h *ShopHandler) Create(c *gin.Context) {
	var req request.CreateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreateShop(outgoingContext(c), &shopv1.CreateShopRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop create proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", resp.GetId()))
	httpx.Created(c, resp)
}

// Get 将 GET /api/v1/shops/:id 代理到 GetShop。
func (h *ShopHandler) Get(c *gin.Context) {
	resp, err := h.client.GetShop(outgoingContext(c), &shopv1.GetShopRequest{Id: c.Param("id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// List 将 GET /api/v1/shops 代理到 ListShops。
func (h *ShopHandler) List(c *gin.Context) {
	var req request.ListShopRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ListShops(outgoingContext(c), &shopv1.ListShopsRequest{
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Update 将 PUT /api/v1/shops/:id 代理到 UpdateShop。
func (h *ShopHandler) Update(c *gin.Context) {
	var req request.UpdateShopRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.UpdateShop(outgoingContext(c), &shopv1.UpdateShopRequest{
		Id:          c.Param("id"),
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop update proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", resp.GetId()))
	httpx.OK(c, resp)
}

// Delete 将 DELETE /api/v1/shops/:id 代理到 DeleteShop。
func (h *ShopHandler) Delete(c *gin.Context) {
	resp, err := h.client.DeleteShop(outgoingContext(c), &shopv1.DeleteShopRequest{Id: c.Param("id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop delete proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", c.Param("id")))
	httpx.OK(c, resp)
}

// PayNotify 将支付渠道异步通知代理到 NotifyPayment。
// 支付宝回调为 application/x-www-form-urlencoded，不能 ShouldBindJSON，
// 直接读取原始表单字符串透传给 gRPC 服务端验签。
func (h *ShopHandler) PayNotify(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", "read notify body failed"))
		return
	}
	resp, err := h.client.NotifyPayment(outgoingContext(c), &shopv1.NotifyPaymentRequest{
		Channel: c.Query("channel"),
		Form:    string(body),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop pay notify proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()), zap.String("status", resp.GetStatus()))
	httpx.OK(c, resp)
}

// AlipayNotify 处理支付宝异步通知公网回调 /aLiPay。
// cpolar 穿透将公网路径原样透传到本地，channel 固定为 alipay；
// 支付宝要求验签成功且已落库后返回纯文本 "success"，失败返回 "failure"。
func (h *ShopHandler) AlipayNotify(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.String(200, "failure")
		return
	}
	resp, err := h.client.NotifyPayment(outgoingContext(c), &shopv1.NotifyPaymentRequest{
		Channel: "alipay",
		Form:    string(body),
	})
	if err != nil {
		h.log.Warn("alipay notify proxied failed", zap.String("request_id", httpx.RequestID(c)), zap.Error(err))
		c.String(200, "failure")
		return
	}
	h.log.Info("gateway alipay notify proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()), zap.String("status", resp.GetStatus()))
	c.String(200, "success")
}

// PayQuery 将 GET /api/v1/shops/pay/query 代理到 QueryPayment。
func (h *ShopHandler) PayQuery(c *gin.Context) {
	resp, err := h.client.QueryPayment(outgoingContext(c), &shopv1.QueryPaymentRequest{
		OrderNo: c.Query("order_no"),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop pay query proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()), zap.String("status", resp.GetStatus()))
	httpx.OK(c, resp)
}

// Pay 将 POST /api/v1/shops/pay 代理到 ShopService.Pay。
func (h *ShopHandler) Pay(c *gin.Context) {
	var req request.PayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Pay(outgoingContext(c), &shopv1.PayRequest{
		OrderNo:     req.OrderNo,
		Subject:     req.Subject,
		TotalAmount: req.TotalAmount,
		Body:        req.Body,
		Scene:       req.Scene,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop pay proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}

// ScanPay 将 POST /api/v1/shops/scan-pay 代理到 ShopService.ScanPay。
func (h *ShopHandler) ScanPay(c *gin.Context) {
	var req request.ScanPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ScanPay(outgoingContext(c), &shopv1.ScanPayRequest{
		OrderNo:     req.OrderNo,
		Subject:     req.Subject,
		TotalAmount: req.TotalAmount,
		Body:        req.Body,
		Channel:     req.Channel,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop scan-pay proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}

// BarCodePay 将 POST /api/v1/shops/bar-code-pay 代理到 ShopService.BarCodePay。
func (h *ShopHandler) BarCodePay(c *gin.Context) {
	var req request.BarCodePayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.BarCodePay(outgoingContext(c), &shopv1.BarCodePayRequest{
		OrderNo:     req.OrderNo,
		Subject:     req.Subject,
		TotalAmount: req.TotalAmount,
		AuthCode:    req.AuthCode,
		Body:        req.Body,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop bar-code-pay proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}

// Refund 将 POST /api/v1/shops/refund 代理到 ShopService.Refund。
func (h *ShopHandler) Refund(c *gin.Context) {
	var req request.RefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Refund(outgoingContext(c), &shopv1.RefundRequest{
		OrderNo:      req.OrderNo,
		TradeNo:      req.TradeNo,
		RefundAmount: req.RefundAmount,
		RefundReason: req.RefundReason,
		OutRequestNo: req.OutRequestNo,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway shop refund proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}
