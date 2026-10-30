package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	paymentv1 "github.com/BwCloudWeGo/bw-cli/api/gen/payment/v1"
	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// PaymentHandler 将 payment HTTP 接口适配到生成的 gRPC client。
// 支付成功回调在本层编排：乘客订单推进到已完成 → 司机域记账结算。
type PaymentHandler struct {
	client    paymentv1.PaymentServiceClient
	passenger passengerv1.PassengerServiceClient
	driver    driverv1.DriverServiceClient
	log       *zap.Logger
}

// NewPaymentHandler 注入 payment / passenger / driver gRPC client。
func NewPaymentHandler(
	client paymentv1.PaymentServiceClient,
	passenger passengerv1.PassengerServiceClient,
	driver driverv1.DriverServiceClient,
	log *zap.Logger,
) *PaymentHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &PaymentHandler{client: client, passenger: passenger, driver: driver, log: log}
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

// Notify 将 POST /api/v1/payments/notify 代理到 NotifyPayment，
// 并在支付成功后编排下游：乘客订单 待支付 → 已完成，司机域记账结算。
// 编排放网关聚合层：支付域不感知业务订单，乘客/司机域不感知支付渠道。
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
	// 支付成功即触发下游联动。duplicate=true（渠道重试）也要再触发一次：
	// 上次联动可能因乘客/司机域暂时不可用而失败，MarkOrderPaid 与司机
	// 记账均幂等，重试不会重复入账；跳过反而会让订单永远停在待支付。
	if resp.GetSuccess() {
		h.settleOrder(c, req)
	}
	h.log.Info("gateway payment notify proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("order_no", resp.GetPaymentNo()))
	httpx.OK(c, resp)
}

// settleOrder 支付成功后的下游联动。任一环节失败只记日志不回滚支付单，
// 依赖回调重试完成最终一致。
func (h *PaymentHandler) settleOrder(c *gin.Context, req request.NotifyPaymentRequest) {
	paid, err := h.passenger.MarkOrderPaid(outgoingContext(c), &passengerv1.MarkOrderPaidRequest{
		OrderNo:   req.OrderNo,
		PaymentNo: req.TradeNo,
		TradeNo:   req.TradeNo,
		PaidFen:   req.PaidAmount,
	})
	if err != nil {
		h.log.Error("mark order paid failed", zap.String("order_no", req.OrderNo), zap.Error(err))
		return
	}
	h.log.Info("mark order paid",
		zap.String("order_no", req.OrderNo),
		zap.String("driver_id", paid.GetDriverId()),
		zap.Int64("driver_income_fen", paid.GetDriverIncomeFen()))
	if paid.GetDriverId() == "" || h.driver == nil {
		h.log.Warn("skip driver income report",
			zap.Bool("driver_client_nil", h.driver == nil),
			zap.String("driver_id", paid.GetDriverId()))
		return
	}
	if _, err := h.driver.ReportPaymentSucceeded(outgoingContext(c), &driverv1.ReportPaymentRequest{
		OrderId:  paid.GetOrderId(),
		DriverId: paid.GetDriverId(),
		// 事件 ID 按订单固定（不含渠道流水号）：渠道重试的 trade_no 每次不同，
		// 拼进 event_id 会让司机域去重失效、同一订单重复入账。
		EventId:        "pay-" + paid.GetOrderNo(),
		IdempotencyKey: "pay-" + paid.GetOrderNo(),
		Amount:         &driverv1.AmountMessage{DriverIncomeFen: paid.GetDriverIncomeFen()},
	}); err != nil {
		h.log.Error("report driver income failed", zap.String("order_no", req.OrderNo), zap.Error(err))
	}
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
