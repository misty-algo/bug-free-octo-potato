package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	biz "github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// DriverHandler 处理司机端 REST 请求，转发到 driver-service 的 gRPC。
type DriverHandler struct {
	client driverv1.DriverServiceClient
	log    *zap.Logger
}

// NewDriverHandler 创建司机端 HTTP handler。
func NewDriverHandler(client driverv1.DriverServiceClient, log *zap.Logger) *DriverHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &DriverHandler{client: client, log: log}
}

// ─────────────────────────── 司机账号 ───────────────────────────

// SendDriverCode 下发司机登录验证码（登录前可用）。
func (h *DriverHandler) SendDriverCode(c *gin.Context) {
	var req biz.DriverSendCodeRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.SendDriverCode(outgoingContext(c), &driverv1.SendDriverCodeRequest{
		Phone: req.Phone,
	})
	handleDriverResponse(c, resp, err)
}

// DriverLogin 司机登录：号码未建档时自动建档，返回司机令牌与档案。
func (h *DriverHandler) DriverLogin(c *gin.Context) {
	var req biz.DriverLoginRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.DriverLogin(outgoingContext(c), &driverv1.DriverLoginRequest{
		Phone: req.Phone, Code: req.Code, Name: req.Name,
	})
	handleDriverResponse(c, resp, err)
}

// Register 注册司机。
func (h *DriverHandler) Register(c *gin.Context) {
	var req biz.RegisterDriverRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.RegisterDriver(outgoingContext(c), &driverv1.RegisterDriverRequest{
		Name: req.Name, Phone: req.Phone,
	})
	handleDriverResponse(c, resp, err)
}

// GetProfile 查询司机档案（含资金五桶）。
func (h *DriverHandler) GetProfile(c *gin.Context) {
	resp, err := h.client.GetDriverProfile(outgoingContext(c), &driverv1.GetDriverProfileRequest{
		DriverId: c.Param("id"),
	})
	handleDriverResponse(c, resp, err)
}

// UpdateStatus 收车/出车。
func (h *DriverHandler) UpdateStatus(c *gin.Context) {
	var req biz.UpdateDriverStatusRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.UpdateDriverStatus(outgoingContext(c), &driverv1.UpdateDriverStatusRequest{
		DriverId: c.Param("id"), Status: req.Status, Version: req.Version,
	})
	handleDriverResponse(c, resp, err)
}

// UpdateMode 听单/抢单切换。
func (h *DriverHandler) UpdateMode(c *gin.Context) {
	var req biz.UpdateDriverModeRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.UpdateDriverMode(outgoingContext(c), &driverv1.UpdateDriverModeRequest{
		DriverId: c.Param("id"), Mode: req.Mode, Version: req.Version,
	})
	handleDriverResponse(c, resp, err)
}

// UpdateOfflineAfterTrip 设置"本单后收车"。
func (h *DriverHandler) UpdateOfflineAfterTrip(c *gin.Context) {
	var req biz.UpdateOfflineAfterTripRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.UpdateOfflineAfterTrip(outgoingContext(c), &driverv1.UpdateOfflineAfterTripRequest{
		DriverId: c.Param("id"), On: req.On, Version: req.Version,
	})
	handleDriverResponse(c, resp, err)
}

// Heartbeat 司机心跳上报（空闲连续 60 秒无心跳自动收车）。
func (h *DriverHandler) Heartbeat(c *gin.Context) {
	var req biz.HeartbeatRequest
	if c.Request.ContentLength > 0 {
		if !bindJSON(c, &req) {
			return
		}
	}
	resp, err := h.client.DriverHeartbeat(outgoingContext(c), &driverv1.HeartbeatRequest{
		DriverId: c.Param("id"), Lat: req.Lat, Lng: req.Lng,
	})
	handleDriverResponse(c, resp, err)
}

// UpdateAreaConfig 修改接单区域偏好。
func (h *DriverHandler) UpdateAreaConfig(c *gin.Context) {
	var req biz.UpdateAreaConfigRequest
	if !bindJSON(c, &req) {
		return
	}
	areas := make([]*driverv1.AreaBoxMessage, 0, len(req.Areas))
	for _, a := range req.Areas {
		areas = append(areas, &driverv1.AreaBoxMessage{
			Name: a.Name, MinLat: a.MinLat, MinLng: a.MinLng, MaxLat: a.MaxLat, MaxLng: a.MaxLng,
		})
	}
	resp, err := h.client.UpdateAreaConfig(outgoingContext(c), &driverv1.UpdateAreaConfigRequest{
		DriverId: c.Param("id"), AreaLimitSwitch: req.AreaLimitSwitch, Areas: areas, Version: req.Version,
	})
	handleDriverResponse(c, resp, err)
}

// CreateHallOrder 创建派单任务（模拟乘客下单）。
func (h *DriverHandler) CreateHallOrder(c *gin.Context) {
	var req biz.CreateHallOrderRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.CreateHallOrder(outgoingContext(c), &driverv1.CreateHallOrderRequest{
		PassengerName: req.PassengerName,
		Origin:        &driverv1.LocationMessage{Name: req.Origin.Name, Lat: req.Origin.Lat, Lng: req.Origin.Lng},
		Destination:   &driverv1.LocationMessage{Name: req.Destination.Name, Lat: req.Destination.Lat, Lng: req.Destination.Lng},
		OrigFareFen:   req.OrigFareFen, PassengerPaidFen: req.PassengerPaidFen,
		PlatformCouponFen: req.PlatformCouponFen, DriverCouponFen: req.DriverCouponFen,
		SubsidyFen: req.SubsidyFen, PassengerCount: int32(req.PassengerCount),
		CityCode: req.CityCode, OrderType: req.OrderType,
		RequireSeats: int32(req.RequireSeats), RequireCarType: req.RequireCarType,
		OrderId: req.OrderID,
	})
	handleDriverResponse(c, resp, err)
}

// ListHallOrders 大厅列表。
func (h *DriverHandler) ListHallOrders(c *gin.Context) {
	var req biz.ListHallOrdersRequest
	if !bindQuery(c, &req) {
		return
	}
	resp, err := h.client.ListHallOrders(outgoingContext(c), &driverv1.ListHallOrdersRequest{
		DriverId: req.DriverID, Limit: req.Limit, Lat: req.Lat, Lng: req.Lng,
	})
	handleDriverResponse(c, resp, err)
}

// GrabOrder 抢单（15 秒预占）。
func (h *DriverHandler) GrabOrder(c *gin.Context) {
	var req biz.HallOrderActionRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.GrabOrder(outgoingContext(c), &driverv1.GrabOrderRequest{
		DriverId: req.DriverID, OrderId: req.OrderID, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// AcceptOrder 确认接单。
func (h *DriverHandler) AcceptOrder(c *gin.Context) {
	var req biz.HallOrderActionRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.AcceptOrder(outgoingContext(c), &driverv1.AcceptOrderRequest{
		DriverId: req.DriverID, OrderId: req.OrderID, Version: req.Version, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// ReleaseOrder 退单（仅未开始行程可退，司机责任计入每日 3 次）。
func (h *DriverHandler) ReleaseOrder(c *gin.Context) {
	var req biz.HallOrderActionRequest
	if !bindJSON(c, &req) {
		return
	}
	party := req.ResponsibleParty
	if party == "" {
		party = contracts.PartyDriver
	}
	resp, err := h.client.ReleaseOrder(outgoingContext(c), &driverv1.ReleaseOrderRequest{
		DriverId: req.DriverID, OrderId: req.OrderID, Reason: req.Reason,
		ResponsibleParty: party, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// StartTrip 开始行程。
func (h *DriverHandler) StartTrip(c *gin.Context) {
	var req biz.HallOrderActionRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.StartTrip(outgoingContext(c), &driverv1.StartTripRequest{
		DriverId: req.DriverID, OrderId: req.OrderID, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// CompleteOrder 送达结束并记账。
func (h *DriverHandler) CompleteOrder(c *gin.Context) {
	var req biz.HallOrderActionRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.CompleteOrder(outgoingContext(c), &driverv1.CompleteOrderRequest{
		DriverId: req.DriverID, OrderId: req.OrderID,
		DistanceMeters: req.DistanceMeters, DurationSec: req.DurationSec,
		IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// CancelOrder 乘客取消/平台责任取消。
func (h *DriverHandler) CancelOrder(c *gin.Context) {
	var req biz.CancelOrderRequest
	if !bindJSON(c, &req) {
		return
	}
	party := req.ResponsibleParty
	if party == "" {
		party = contracts.PartyPassenger
	}
	resp, err := h.client.CancelOrder(outgoingContext(c), &driverv1.CancelOrderRequest{
		OrderId: req.OrderID, ResponsibleParty: party, Reason: req.Reason,
	})
	handleDriverResponse(c, resp, err)
}

// GetIncomeCompare 首页收益对比。
func (h *DriverHandler) GetIncomeCompare(c *gin.Context) {
	var req biz.IncomeQueryRequest
	if !bindQuery(c, &req) {
		return
	}
	resp, err := h.client.GetIncomeCompare(outgoingContext(c), &driverv1.GetIncomeCompareRequest{
		DriverId: req.DriverID,
	})
	handleDriverResponse(c, resp, err)
}

// GetFlows 收入流水分页。
func (h *DriverHandler) GetFlows(c *gin.Context) {
	var req biz.IncomeQueryRequest
	if !bindQuery(c, &req) {
		return
	}
	resp, err := h.client.ListIncomeFlows(outgoingContext(c), &driverv1.ListIncomeFlowsRequest{
		DriverId: req.DriverID, Page: req.Page, PageSize: req.PageSize,
		From: req.From, To: req.To,
	})
	handleDriverResponse(c, resp, err)
}

// GetSummary 收入概览（五桶 + 提现规则）。
func (h *DriverHandler) GetSummary(c *gin.Context) {
	var req biz.IncomeQueryRequest
	if !bindQuery(c, &req) {
		return
	}
	resp, err := h.client.GetIncomeSummary(outgoingContext(c), &driverv1.GetIncomeSummaryRequest{
		DriverId: req.DriverID,
	})
	handleDriverResponse(c, resp, err)
}

// Withdraw 申请提现。
func (h *DriverHandler) Withdraw(c *gin.Context) {
	var req biz.RequestWithdrawRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.RequestWithdraw(outgoingContext(c), &driverv1.RequestWithdrawRequest{
		DriverId: req.DriverID, AmountFen: req.AmountFen, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// ListWithdrawals 提现记录。
func (h *DriverHandler) ListWithdrawals(c *gin.Context) {
	var req biz.IncomeQueryRequest
	if !bindQuery(c, &req) {
		return
	}
	resp, err := h.client.ListWithdrawals(outgoingContext(c), &driverv1.ListWithdrawalsRequest{
		DriverId: req.DriverID, Page: req.Page, PageSize: req.PageSize,
		From: req.From, To: req.To, Status: req.Status,
	})
	handleDriverResponse(c, resp, err)
}

// ── 跨模块回调接口（供支付域 / 财务域 / 后台集成）──

// ReportPayment 支付成功回调。
func (h *DriverHandler) ReportPayment(c *gin.Context) {
	var req biz.ReportPaymentRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.ReportPaymentSucceeded(outgoingContext(c), &driverv1.ReportPaymentRequest{
		OrderId: req.OrderID, DriverId: req.DriverID, AccountingDate: req.AccountingDate,
		EventId: req.EventID, IdempotencyKey: req.IdempotencyKey,
		Amount: toAmountMessage(req.Amount),
	})
	handleDriverResponse(c, resp, err)
}

// ReportRefund 退款成功回调（生成反向流水）。
func (h *DriverHandler) ReportRefund(c *gin.Context) {
	var req biz.ReportRefundRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.ReportRefundSucceeded(outgoingContext(c), &driverv1.ReportRefundRequest{
		OrderId: req.OrderID, RefundFen: req.RefundFen, ResponsibleParty: req.ResponsibleParty,
		AccountingDate: req.AccountingDate, Reason: req.Reason,
		EventId: req.EventID, IdempotencyKey: req.IdempotencyKey,
	})
	handleDriverResponse(c, resp, err)
}

// SettleIncome 结算确认（财务域）。
func (h *DriverHandler) SettleIncome(c *gin.Context) {
	var req biz.SettleIncomeRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.SettleIncome(outgoingContext(c), &driverv1.SettleIncomeRequest{
		DriverId: req.DriverID, FlowIds: req.FlowIDs, EventId: req.EventID,
	})
	handleDriverResponse(c, resp, err)
}

// ApproveWithdraw 提现审批通过。
func (h *DriverHandler) ApproveWithdraw(c *gin.Context) {
	var req biz.ApproveWithdrawRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.ApproveWithdraw(outgoingContext(c), &driverv1.ApproveWithdrawRequest{
		WithdrawalId: req.WithdrawalID, ApproverId: req.ApproverID, Comment: req.Comment,
	})
	handleDriverResponse(c, resp, err)
}

// RejectWithdraw 提现驳回（返还预占额度）。
func (h *DriverHandler) RejectWithdraw(c *gin.Context) {
	var req biz.RejectWithdrawRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.RejectWithdraw(outgoingContext(c), &driverv1.RejectWithdrawRequest{
		WithdrawalId: req.WithdrawalID, ApproverId: req.ApproverID, Reason: req.Reason,
	})
	handleDriverResponse(c, resp, err)
}

// CompleteWithdraw 打款结果回调。
func (h *DriverHandler) CompleteWithdraw(c *gin.Context) {
	var req biz.CompleteWithdrawRequest
	if !bindJSON(c, &req) {
		return
	}
	resp, err := h.client.CompleteWithdraw(outgoingContext(c), &driverv1.CompleteWithdrawRequest{
		WithdrawalId: req.WithdrawalID, Success: req.Success, Reason: req.Reason,
	})
	handleDriverResponse(c, resp, err)
}

// ── 通用适配 ──

// bindJSON 绑定 JSON 请求体，失败统一返回 invalid_request。
func bindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return false
	}
	return true
}

// bindQuery 绑定 query 参数，失败统一返回 invalid_request。
func bindQuery(c *gin.Context, obj any) bool {
	if err := c.ShouldBindQuery(obj); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return false
	}
	return true
}

func handleDriverResponse(c *gin.Context, resp any, err error) {
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

func toAmountMessage(a biz.AmountItem) *driverv1.AmountMessage {
	return &driverv1.AmountMessage{
		OrigFareFen: a.OrigFareFen, PassengerPaidFen: a.PassengerPaidFen,
		PlatformCouponFen: a.PlatformCouponFen, DriverCouponFen: a.DriverCouponFen,
		CommissionFen: a.CommissionFen, SubsidyFen: a.SubsidyFen, DriverIncomeFen: a.DriverIncomeFen,
	}
}
