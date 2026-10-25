package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// SettlementHandler 将 settlement HTTP 接口适配到生成的 gRPC client。
type SettlementHandler struct {
	client settlementv1.SettlementServiceClient
	log    *zap.Logger
}

// NewSettlementHandler 将 settlement gRPC client 注入 HTTP handler 方法。
func NewSettlementHandler(client settlementv1.SettlementServiceClient, log *zap.Logger) *SettlementHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SettlementHandler{client: client, log: log}
}

// CreateBill 将 POST /api/v1/settlements/bills 代理到 CreateBill。
func (h *SettlementHandler) CreateBill(c *gin.Context) {
	var req request.CreateBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreateBill(outgoingContext(c), &settlementv1.CreateBillRequest{
		OrderNo:         req.OrderNo,
		UserId:          req.UserID,
		DriverId:        req.DriverID,
		TotalAmount:     req.TotalAmount,
		PlatformSubsidy: req.PlatformSubsidy,
		Remark:          req.Remark,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway settlement create bill proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("bill_no", resp.GetBillNo()))
	httpx.Created(c, resp)
}

// Split 将 POST /api/v1/settlements/split 代理到 SplitBill。
func (h *SettlementHandler) Split(c *gin.Context) {
	var req request.SplitBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.SplitBill(outgoingContext(c), &settlementv1.SplitBillRequest{
		OrderNo:           req.OrderNo,
		CommissionRateBps: req.CommissionRateBps,
		PlatformSubsidy:   req.PlatformSubsidy,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway settlement split proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("bill_no", resp.GetBillNo()))
	httpx.OK(c, resp)
}

// GetWalletBalance 将 GET /api/v1/settlements/wallets/:driver_id 代理到 GetWalletBalance。
func (h *SettlementHandler) GetWalletBalance(c *gin.Context) {
	resp, err := h.client.GetWalletBalance(outgoingContext(c), &settlementv1.GetWalletBalanceRequest{DriverId: c.Param("driver_id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// MarkBillPaid 将 POST /api/v1/settlements/mark-bill-paid 代理到 MarkBillPaid（支付成功回调置账单 PAID）。
func (h *SettlementHandler) MarkBillPaid(c *gin.Context) {
	var req request.MarkBillPaidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.MarkBillPaid(outgoingContext(c), &settlementv1.MarkBillPaidRequest{
		BillNo:      req.BillNo,
		OrderNo:     req.OrderNo,
		PayFee:      req.PayFee,
		DiscountFee: req.DiscountFee,
		RefundFee:   req.RefundFee,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway settlement mark bill paid proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("bill_no", resp.GetBillNo()))
	httpx.OK(c, resp)
}

// ApplyRefund 将 POST /api/v1/settlements/apply-refund 代理到 ApplyRefund（退款冲正司机钱包）。
func (h *SettlementHandler) ApplyRefund(c *gin.Context) {
	var req request.ApplyRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ApplyRefund(outgoingContext(c), &settlementv1.ApplyRefundRequest{
		OrderNo:      req.OrderNo,
		RefundNo:     req.RefundNo,
		RefundAmount: req.RefundAmount,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway settlement apply refund proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetOrderNo()))
	httpx.OK(c, resp)
}

// CheckUnpaidBan 将 GET /api/v1/settlements/check-unpaid-ban/:user_id 代理到 CheckUnpaidBan（未付禁叫车校验）。
func (h *SettlementHandler) CheckUnpaidBan(c *gin.Context) {
	resp, err := h.client.CheckUnpaidBan(outgoingContext(c), &settlementv1.CheckUnpaidBanRequest{UserId: c.Param("user_id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// VoidBill 将 POST /api/v1/settlements/void-bill 代理到 VoidBill（作废账单）。
func (h *SettlementHandler) VoidBill(c *gin.Context) {
	var req request.VoidBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.VoidBill(outgoingContext(c), &settlementv1.VoidBillRequest{
		BillNo:  req.BillNo,
		OrderNo: req.OrderNo,
		Reason:  req.Reason,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway settlement void bill proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("bill_no", resp.GetBillNo()))
	httpx.OK(c, resp)
}
