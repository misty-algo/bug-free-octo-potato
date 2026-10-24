package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// Server 是司机端 gRPC 适配器：把 pb 请求翻译成用例命令，并把实体错误翻译成统一错误码。
type Server struct {
	driverv1.UnimplementedDriverServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建司机端 gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	return &Server{svc: svc, log: log}
}

// ─────────────────────────── 司机档案 ───────────────────────────

// RegisterDriver 处理司机注册。
func (s *Server) RegisterDriver(ctx context.Context, req *driverv1.RegisterDriverRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.RegisterDriver(ctx, dto.RegisterDriverCommand{Name: req.GetName(), Phone: req.GetPhone()})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// GetDriverProfile 处理档案查询。
func (s *Server) GetDriverProfile(ctx context.Context, req *driverv1.GetDriverProfileRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.GetDriverProfile(ctx, req.GetDriverId())
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// UpdateDriverStatus 处理收车/出车。
func (s *Server) UpdateDriverStatus(ctx context.Context, req *driverv1.UpdateDriverStatusRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.UpdateStatus(ctx, dto.UpdateStatusCommand{
		DriverID: req.GetDriverId(), Status: req.GetStatus(), Version: req.GetVersion(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// UpdateDriverMode 处理听单/抢单切换。
func (s *Server) UpdateDriverMode(ctx context.Context, req *driverv1.UpdateDriverModeRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.UpdateMode(ctx, dto.UpdateModeCommand{
		DriverID: req.GetDriverId(), Mode: req.GetMode(), Version: req.GetVersion(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// UpdateAreaConfig 处理接单区域设置。
func (s *Server) UpdateAreaConfig(ctx context.Context, req *driverv1.UpdateAreaConfigRequest) (*driverv1.DriverProfileResponse, error) {
	areas := make([]entity.AreaBox, 0, len(req.GetAreas()))
	for _, a := range req.GetAreas() {
		areas = append(areas, entity.AreaBox{Name: a.GetName(), MinLat: a.GetMinLat(), MinLng: a.GetMinLng(), MaxLat: a.GetMaxLat(), MaxLng: a.GetMaxLng()})
	}
	profile, err := s.svc.UpdateAreaConfig(ctx, dto.UpdateAreaCommand{
		DriverID: req.GetDriverId(), SwitchOn: req.GetAreaLimitSwitch(), Areas: areas, Version: req.GetVersion(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// DriverHeartbeat 处理心跳上报（空闲超时自动收车）。
func (s *Server) DriverHeartbeat(ctx context.Context, req *driverv1.HeartbeatRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.Heartbeat(ctx, dto.HeartbeatCommand{
		DriverID: req.GetDriverId(), Lat: req.GetLat(), Lng: req.GetLng(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// UpdateOfflineAfterTrip 处理"本单后收车"设置。
func (s *Server) UpdateOfflineAfterTrip(ctx context.Context, req *driverv1.UpdateOfflineAfterTripRequest) (*driverv1.DriverProfileResponse, error) {
	profile, err := s.svc.SetOfflineAfterTrip(ctx, dto.OfflineAfterTripCommand{
		DriverID: req.GetDriverId(), On: req.GetOn(), Version: req.GetVersion(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProfileProto(profile), nil
}

// ─────────────────────────── 接单大厅 ───────────────────────────

// CreateHallOrder 处理任务创建（模拟乘客下单 / 乘客域事件重放）。
func (s *Server) CreateHallOrder(ctx context.Context, req *driverv1.CreateHallOrderRequest) (*driverv1.HallOrderResponse, error) {
	cmd := dto.CreateTaskCommand{
		OrderID: req.GetOrderId(),
		PassengerName: req.GetPassengerName(),
		PassengerCount: int(req.GetPassengerCount()),
		CityCode: req.GetCityCode(), OrderType: req.GetOrderType(),
		RequireSeats: int(req.GetRequireSeats()), RequireCarType: req.GetRequireCarType(),
		OriginName: req.GetOrigin().GetName(), OriginLat: req.GetOrigin().GetLat(), OriginLng: req.GetOrigin().GetLng(),
		DestName: req.GetDestination().GetName(), DestLat: req.GetDestination().GetLat(), DestLng: req.GetDestination().GetLng(),
		OrigFareFen: req.GetOrigFareFen(), PassengerPaidFen: req.GetPassengerPaidFen(),
		PlatformCouponFen: req.GetPlatformCouponFen(), DriverCouponFen: req.GetDriverCouponFen(),
		SubsidyFen: req.GetSubsidyFen(),
	}
	order, err := s.svc.CreateTask(ctx, cmd)
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// ListHallOrders 处理大厅列表查询。
func (s *Server) ListHallOrders(ctx context.Context, req *driverv1.ListHallOrdersRequest) (*driverv1.ListHallOrdersResponse, error) {
	items, total, err := s.svc.ListHallOrders(ctx, req.GetDriverId(), req.GetLat(), req.GetLng(), int(req.GetLimit()))
	if err != nil {
		return nil, mapDriverError(err)
	}
	out := &driverv1.ListHallOrdersResponse{Total: total}
	for _, i := range items {
		out.Items = append(out.Items, toOrderProto(i))
	}
	return out, nil
}

// GrabOrder 处理抢单。
func (s *Server) GrabOrder(ctx context.Context, req *driverv1.GrabOrderRequest) (*driverv1.HallOrderResponse, error) {
	order, err := s.svc.GrabOrder(ctx, dto.GrabOrderCommand{
		DriverID: req.GetDriverId(), OrderID: req.GetOrderId(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// AcceptOrder 处理确认接单。
func (s *Server) AcceptOrder(ctx context.Context, req *driverv1.AcceptOrderRequest) (*driverv1.HallOrderResponse, error) {
	order, err := s.svc.AcceptOrder(ctx, dto.AcceptOrderCommand{
		DriverID: req.GetDriverId(), OrderID: req.GetOrderId(),
		Version: req.GetVersion(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// ReleaseOrder 处理退单。
func (s *Server) ReleaseOrder(ctx context.Context, req *driverv1.ReleaseOrderRequest) (*driverv1.HallOrderResponse, error) {
	order, err := s.svc.ReleaseOrder(ctx, dto.ReleaseOrderCommand{
		DriverID: req.GetDriverId(), OrderID: req.GetOrderId(), Reason: req.GetReason(),
		ResponsibleParty: req.GetResponsibleParty(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// StartTrip 处理行程开始（行程域/司机端）。
func (s *Server) StartTrip(ctx context.Context, req *driverv1.StartTripRequest) (*driverv1.HallOrderResponse, error) {
	order, err := s.svc.StartTrip(ctx, dto.StartTripCommand{
		DriverID: req.GetDriverId(), OrderID: req.GetOrderId(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// CompleteOrder 处理送达结束并记账（课程模拟：行程结束即视为支付成功）。
func (s *Server) CompleteOrder(ctx context.Context, req *driverv1.CompleteOrderRequest) (*driverv1.CompleteOrderResponse, error) {
	order, err := s.svc.EndTrip(ctx, dto.EndTripCommand{
		DriverID: req.GetDriverId(), OrderID: req.GetOrderId(),
		DistanceMeters: req.GetDistanceMeters(), DurationSec: req.GetDurationSec(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	flows, _, fErr := s.svc.ListIncomeFlows(ctx, dto.ListFlowsCommand{DriverID: req.GetDriverId(), Page: 1, PageSize: 1})
	res := &driverv1.CompleteOrderResponse{Order: toOrderProto(order)}
	if fErr == nil && len(flows) > 0 {
		res.Flow = toFlowProto(flows[0])
	}
	return res, nil
}

// CancelOrder 处理乘客取消/平台责任取消。
func (s *Server) CancelOrder(ctx context.Context, req *driverv1.CancelOrderRequest) (*driverv1.HallOrderResponse, error) {
	order, err := s.svc.CancelByExternal(ctx, req.GetOrderId(), req.GetResponsibleParty(), req.GetReason())
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toOrderProto(order), nil
}

// ─────────────────────────── 收益 / 结算 ───────────────────────────

// GetIncomeCompare 处理首页收益对比。
func (s *Server) GetIncomeCompare(ctx context.Context, req *driverv1.GetIncomeCompareRequest) (*driverv1.IncomeCompareResponse, error) {
	cmp, err := s.svc.GetIncomeCompare(ctx, req.GetDriverId())
	if err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.IncomeCompareResponse{
		Today: &driverv1.DayIncomeMessage{
			Date: cmp.Today.Date, OrderCount: cmp.Today.OrderCount,
			IncomeFen: cmp.Today.IncomeFen, RefundFen: cmp.Today.RefundFen,
		},
		Yesterday: &driverv1.DayIncomeMessage{
			Date: cmp.Yesterday.Date, OrderCount: cmp.Yesterday.OrderCount,
			IncomeFen: cmp.Yesterday.IncomeFen, RefundFen: cmp.Yesterday.RefundFen,
		},
		OrderDiff: cmp.OrderDiff, IncomeDiffFen: cmp.IncomeDiffFen,
		PercentText: cmp.PercentText, Summary: cmp.Summary,
		PendingIncomeFen: cmp.PendingIncomeFen, LastUpdatedAt: cmp.LastUpdatedAt,
	}, nil
}

// ListIncomeFlows 处理流水分页查询。
func (s *Server) ListIncomeFlows(ctx context.Context, req *driverv1.ListIncomeFlowsRequest) (*driverv1.ListIncomeFlowsResponse, error) {
	items, total, err := s.svc.ListIncomeFlows(ctx, dto.ListFlowsCommand{
		DriverID: req.GetDriverId(), Page: int(req.GetPage()), PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	out := &driverv1.ListIncomeFlowsResponse{Total: total}
	for _, f := range items {
		out.Items = append(out.Items, toFlowProto(f))
	}
	return out, nil
}

// GetIncomeSummary 处理收入概览。
func (s *Server) GetIncomeSummary(ctx context.Context, req *driverv1.GetIncomeSummaryRequest) (*driverv1.IncomeSummaryResponse, error) {
	sum, err := s.svc.GetIncomeSummary(ctx, req.GetDriverId())
	if err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.IncomeSummaryResponse{
		PendingFen: sum.PendingFen, AvailableFen: sum.AvailableFen, FrozenFen: sum.FrozenFen,
		WithdrawFen: sum.WithdrawFen, DebtFen: sum.DebtFen, MonthlyIncomeFen: sum.MonthlyIncomeFen,
		MinWithdrawFen: sum.MinWithdrawFen, MaxWithdrawFen: sum.MaxWithdrawFen,
		DailyLimitFen: sum.DailyLimitFen, DailyMaxCount: int32(sum.DailyMaxCount),
		DualApprovalFen: sum.DualApprovalFen,
		WithdrawnTodayFen: sum.WithdrawnTodayFen, WithdrawCountToday: sum.WithdrawCountToday,
		UpdatedAt: sum.UpdatedAt,
	}, nil
}

// RequestWithdraw 处理提现申请。
func (s *Server) RequestWithdraw(ctx context.Context, req *driverv1.RequestWithdrawRequest) (*driverv1.WithdrawResponse, error) {
	res, err := s.svc.RequestWithdraw(ctx, dto.RequestWithdrawCommand{
		DriverID: req.GetDriverId(), AmountFen: req.GetAmountFen(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toWithdrawProto(res.Withdrawal), nil
}

// ListWithdrawals 处理提现记录查询。
func (s *Server) ListWithdrawals(ctx context.Context, req *driverv1.ListWithdrawalsRequest) (*driverv1.ListWithdrawalsResponse, error) {
	items, total, err := s.svc.ListWithdrawals(ctx, dto.ListWithdrawalsCommand{
		DriverID: req.GetDriverId(), Page: int(req.GetPage()), PageSize: int(req.GetPageSize()),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	out := &driverv1.ListWithdrawalsResponse{Total: total}
	for _, w := range items {
		out.Items = append(out.Items, toWithdrawProto(w))
	}
	return out, nil
}

// ─────────────────────────── 跨模块资金接口 ───────────────────────────

// ReportPaymentSucceeded 支付域回调：记账到待结算桶 U。
func (s *Server) ReportPaymentSucceeded(ctx context.Context, req *driverv1.ReportPaymentRequest) (*driverv1.IncomeFlowResponse, error) {
	flow, err := s.svc.ReportPaymentSucceeded(ctx, dto.ReportPaymentCommand{
		OrderID: req.GetOrderId(), DriverID: req.GetDriverId(),
		AccountingDate: req.GetAccountingDate(), EventID: req.GetEventId(),
		IdempotencyKey: req.GetIdempotencyKey(), Amount: contracts.TaskAmount{
			OrigFareFen: req.GetAmount().GetOrigFareFen(),
			PassengerPaidFen: req.GetAmount().GetPassengerPaidFen(),
			PlatformCouponFen: req.GetAmount().GetPlatformCouponFen(),
			DriverCouponFen: req.GetAmount().GetDriverCouponFen(),
			CommissionFen: req.GetAmount().GetCommissionFen(),
			SubsidyFen: req.GetAmount().GetSubsidyFen(),
			DriverIncomeFen: req.GetAmount().GetDriverIncomeFen(),
		},
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.IncomeFlowResponse{Flow: toFlowProto(flow)}, nil
}

// ReportRefundSucceeded 退款回调：追加反向流水。
func (s *Server) ReportRefundSucceeded(ctx context.Context, req *driverv1.ReportRefundRequest) (*driverv1.IncomeFlowResponse, error) {
	flow, err := s.svc.ReportRefundSucceeded(ctx, dto.ReportRefundCommand{
		OrderID: req.GetOrderId(), RefundFen: req.GetRefundFen(),
		ResponsibleParty: req.GetResponsibleParty(), AccountingDate: req.GetAccountingDate(),
		Reason: req.GetReason(), EventID: req.GetEventId(), IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.IncomeFlowResponse{Flow: toFlowProto(flow)}, nil
}

// SettleIncome 财务域结算确认：U → A。
func (s *Server) SettleIncome(ctx context.Context, req *driverv1.SettleIncomeRequest) (*driverv1.SettleIncomeResponse, error) {
	count, err := s.svc.SettleIncome(ctx, dto.SettleIncomeCommand{
		DriverID: req.GetDriverId(), FlowIDs: req.GetFlowIds(), EventID: req.GetEventId(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.SettleIncomeResponse{SettledDrivers: int32(count)}, nil
}

// ApproveWithdraw 后台审批通过。
func (s *Server) ApproveWithdraw(ctx context.Context, req *driverv1.ApproveWithdrawRequest) (*driverv1.WithdrawResponse, error) {
	w, err := s.svc.ApproveWithdraw(ctx, dto.ApproveWithdrawCommand{
		WithdrawalID: req.GetWithdrawalId(), ApproverID: req.GetApproverId(), Comment: req.GetComment(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toWithdrawProto(w), nil
}

// RejectWithdraw 后台驳回（返还预占额度）。
func (s *Server) RejectWithdraw(ctx context.Context, req *driverv1.RejectWithdrawRequest) (*driverv1.WithdrawResponse, error) {
	w, err := s.svc.RejectWithdraw(ctx, dto.RejectWithdrawCommand{
		WithdrawalID: req.GetWithdrawalId(), ApproverID: req.GetApproverId(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toWithdrawProto(w), nil
}

// CompleteWithdraw 打款结果回调。
func (s *Server) CompleteWithdraw(ctx context.Context, req *driverv1.CompleteWithdrawRequest) (*driverv1.WithdrawResponse, error) {
	w, err := s.svc.CompleteWithdraw(ctx, dto.CompleteWithdrawCommand{
		WithdrawalID: req.GetWithdrawalId(), Success: req.GetSuccess(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toWithdrawProto(w), nil
}

// ─────────────────────────── 类型转换 ───────────────────────────

func toProfileProto(p *dto.DriverProfileDTO) *driverv1.DriverProfileResponse {
	areas := make([]*driverv1.AreaBoxMessage, 0, len(p.Areas))
	for _, a := range p.Areas {
		areas = append(areas, &driverv1.AreaBoxMessage{Name: a.Name, MinLat: a.MinLat, MinLng: a.MinLng, MaxLat: a.MaxLat, MaxLng: a.MaxLng})
	}
	return &driverv1.DriverProfileResponse{
		Id: p.ID, DriverNo: p.DriverNo, Name: p.Name, Phone: p.Phone,
		Status: p.Status, Mode: p.Mode, ModeVersion: p.ModeVersion,
		AreaLimitSwitch: p.AreaLimitSwitch, Areas: areas,
		CityCode: p.CityCode, CarType: p.CarType, Seats: int32(p.Seats),
		Qualified: p.Qualified, CanAccept: p.CanAccept,
		OfflineAfterTrip: p.OfflineAfterTrip,
		LastHeartbeatAt:  biztime.RFC3339(p.LastHeartbeatAt), ActiveTaskId: p.ActiveTaskID,
		MonthlyIncomeFen: p.MonthlyIncomeFen, Rating: p.Rating, SuccessRate: p.SuccessRate,
		Funds: &driverv1.FundsMessage{
			PendingFen: p.Funds.PendingFen, AvailableFen: p.Funds.AvailableFen,
			FrozenFen: p.Funds.FrozenFen, WithdrawFen: p.Funds.WithdrawFen, DebtFen: p.Funds.DebtFen,
		},
		Version: p.Version, CreatedAt: biztime.RFC3339(p.CreatedAt), UpdatedAt: biztime.RFC3339(p.UpdatedAt),
	}
}

func toOrderProto(o *dto.HallOrderDTO) *driverv1.HallOrderResponse {
	return &driverv1.HallOrderResponse{
		Id: o.ID, TaskId: o.TaskID, OrderId: o.OrderID,
		PassengerName: o.PassengerName, PassengerCount: int32(o.PassengerCount),
		Origin:      &driverv1.LocationMessage{Name: o.Origin.Name, Lat: o.Origin.Lat, Lng: o.Origin.Lng},
		Destination: &driverv1.LocationMessage{Name: o.Destination.Name, Lat: o.Destination.Lat, Lng: o.Destination.Lng},
		OrigFareFen: o.OrigFareFen, PassengerPaidFen: o.PassengerPaidFen,
		PlatformCouponFen: o.PlatformCouponFen, DriverCouponFen: o.DriverCouponFen,
		CommissionFen: o.CommissionFen, SubsidyFen: o.SubsidyFen, DriverIncomeFen: o.DriverIncomeFen,
		Status: o.Status, DriverId: o.DriverID, DriverName: o.DriverName,
		DispatchChannel: o.DispatchChannel, VisibleInPool: o.VisibleInPool,
		PickupDistanceMeters: o.PickupDistanceMeters, InviteRound: o.InviteRound,
		AcceptDeadline: biztime.RFC3339(o.AcceptDeadline), ListenDeadline: biztime.RFC3339(o.ListenDeadline),
		DispatchDeadline: biztime.RFC3339(o.DispatchDeadline),
		AssignedAt: biztime.RFC3339(o.AssignedAt), AcceptedAt: biztime.RFC3339(o.AcceptedAt),
		StartedAt: biztime.RFC3339(o.StartedAt), EndedAt: biztime.RFC3339(o.EndedAt),
		CancelledAt: biztime.RFC3339(o.CancelledAt),
		DistanceMeters: o.DistanceMeters, DurationSec: o.DurationSec,
		CancelReason: o.CancelReason, ResponsibleParty: o.ResponsibleParty,
		Version: o.Version, CreatedAt: biztime.RFC3339(o.CreatedAt), UpdatedAt: biztime.RFC3339(o.UpdatedAt),
	}
}

func toFlowProto(f *dto.IncomeFlowDTO) *driverv1.IncomeFlowMessage {
	return &driverv1.IncomeFlowMessage{
		Id: f.ID, DriverId: f.DriverID, OrderId: f.OrderID, TaskId: f.TaskID,
		FlowType: f.FlowType, ReverseOf: f.ReverseOf,
		Amount: &driverv1.AmountMessage{
			OrigFareFen: f.OrigFareFen, PassengerPaidFen: f.PassengerPaidFen,
			PlatformCouponFen: f.PlatformCouponFen, DriverCouponFen: f.DriverCouponFen,
			CommissionFen: f.CommissionFen, SubsidyFen: f.SubsidyFen, DriverIncomeFen: f.DriverIncomeFen,
		},
		AccountingDate: f.AccountingDate, BucketFrom: f.BucketFrom, BucketTo: f.BucketTo,
		EventId: f.EventID, IdempotencyKey: f.IdempotencyKey, Reason: f.Reason,
		CreatedAt: biztime.RFC3339(f.CreatedAt),
	}
}

func toWithdrawProto(w *dto.WithdrawDTO) *driverv1.WithdrawResponse {
	approvals := make([]*driverv1.ApprovalMessage, 0, len(w.Approvals))
	for _, a := range w.Approvals {
		approvals = append(approvals, &driverv1.ApprovalMessage{
			ApproverId: a.ApproverID, Comment: a.Comment, ApprovedAt: biztime.RFC3339(a.ApprovedAt),
		})
	}
	return &driverv1.WithdrawResponse{
		Id: w.ID, DriverId: w.DriverID, AmountFen: w.AmountFen, Status: w.Status,
		DualApproval: w.DualApproval, Approvals: approvals,
		IdempotencyKey: w.IdempotencyKey, Reason: w.Reason,
		CreatedAt: biztime.RFC3339(w.CreatedAt), UpdatedAt: biztime.RFC3339(w.UpdatedAt),
	}
}

// mapDriverError 把领域错误翻译成统一错误码，供 gateway 透传给前端。
func mapDriverError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrInvalidDriver),
		stderrors.Is(err, entity.ErrInvalidStatus),
		stderrors.Is(err, entity.ErrInvalidMode),
		stderrors.Is(err, entity.ErrInvalidOrder),
		stderrors.Is(err, entity.ErrInvalidFlow),
		stderrors.Is(err, entity.ErrInvalidWithdraw):
		return apperrors.InvalidArgument("invalid_input", err.Error())
	case stderrors.Is(err, entity.ErrDriverNotFound):
		return apperrors.NotFound("driver_not_found", "司机不存在，请先注册")
	case stderrors.Is(err, entity.ErrOrderNotFound):
		return apperrors.NotFound("order_not_found", "订单不存在")
	case stderrors.Is(err, entity.ErrWithdrawNotFound):
		return apperrors.NotFound("withdraw_not_found", "提现单不存在")
	case stderrors.Is(err, entity.ErrOrderNotVisible):
		return apperrors.Conflict("order_not_visible", "订单尚未进入抢单池（听单窗口内或已超时）")
	case stderrors.Is(err, entity.ErrOrderNotAvailable):
		return apperrors.Conflict("order_not_available", "手慢了，订单已被其他司机占用或已释放")
	case stderrors.Is(err, entity.ErrOrderNotMine):
		return apperrors.Conflict("order_not_mine", "该订单未派给您，无法操作")
	case stderrors.Is(err, entity.ErrOrderStateDenied):
		return apperrors.Conflict("order_state_denied", "订单当前状态不允许该操作")
	case stderrors.Is(err, entity.ErrTripAlreadyStarted):
		return apperrors.Conflict("trip_already_started", "行程已开始，不能再退单")
	case stderrors.Is(err, entity.ErrBusyCannotOffline):
		return apperrors.Conflict("busy_cannot_offline", "忙碌状态不能直接收车，请先完成当前订单")
	case stderrors.Is(err, entity.ErrPreOccupiedBusy):
		return apperrors.Conflict("pre_occupied_busy", "有待确认的接单邀请，请先确认或等待超时")
	case stderrors.Is(err, entity.ErrStatusNotAllowed):
		return apperrors.Conflict("status_not_allowed", "该状态流转不被允许")
	case stderrors.Is(err, entity.ErrModeSwitchNotAllowed):
		return apperrors.Conflict("mode_switch_denied", "仅空闲状态允许切换听单/抢单模式")
	case stderrors.Is(err, entity.ErrAreaChangeNotAllowed):
		return apperrors.Conflict("area_change_denied", "仅空闲状态允许修改接单区域设置")
	case stderrors.Is(err, entity.ErrDriverNotAvailable):
		return apperrors.Conflict("driver_not_idle", "司机当前状态不可接单，请先出车（空闲）")
	case stderrors.Is(err, entity.ErrDriverUnqualified):
		return apperrors.Conflict("driver_unqualified", "司机资质无效，不能接单")
	case stderrors.Is(err, entity.ErrDriverCannotAccept):
		return apperrors.Conflict("driver_forbidden", "该司机已被禁止接单")
	case stderrors.Is(err, entity.ErrVersionConflict):
		return apperrors.Conflict("version_conflict", "数据版本已变化，请刷新后重试")
	case stderrors.Is(err, entity.ErrReleaseLimitExceeded):
		return apperrors.Conflict("release_limit_exceeded", "今日退单次数已达上限（每天最多 3 次）")
	case stderrors.Is(err, entity.ErrBelowMinWithdraw):
		return apperrors.Conflict("below_min_withdraw", "未达到最低提现门槛")
	case stderrors.Is(err, entity.ErrAboveMaxWithdraw):
		return apperrors.Conflict("above_max_withdraw", "超过单笔提现上限")
	case stderrors.Is(err, entity.ErrBalanceInsufficient), stderrors.Is(err, entity.ErrInsufficientFunds):
		return apperrors.Conflict("balance_insufficient", "可提现余额不足")
	case stderrors.Is(err, entity.ErrDailyLimitExceeded):
		return apperrors.Conflict("daily_withdraw_limit", "超过单日提现上限")
	case stderrors.Is(err, entity.ErrDailyCountExceeded):
		return apperrors.Conflict("daily_withdraw_count", "今日提现次数已达上限")
	case stderrors.Is(err, entity.ErrDualApprovalRequired):
		return apperrors.Conflict("dual_approval_required", "大额提现需双人审批")
	case stderrors.Is(err, entity.ErrDuplicateApprover):
		return apperrors.Conflict("duplicate_approver", "同一审批人不能重复审批")
	case stderrors.Is(err, entity.ErrWithdrawStateDenied):
		return apperrors.Conflict("withdraw_state_denied", "提现单当前状态不允许该操作")
	case stderrors.Is(err, entity.ErrDuplicateFlowEvent):
		return apperrors.Conflict("duplicate_event", "该事件已处理，忽略重复请求")
	default:
		return apperrors.Wrap(apperrors.KindInternal, "driver_service_error", "司机服务内部错误", err)
	}
}
