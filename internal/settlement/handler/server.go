package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/settlement/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/settlement/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/settlement/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Server 将 settlement gRPC 请求适配到 service 用例。
type Server struct {
	settlementv1.UnimplementedSettlementServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建 settlement gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	return &Server{svc: svc, log: log}
}

// CreateBill 处理生成乘客账单。
func (s *Server) CreateBill(ctx context.Context, req *settlementv1.CreateBillRequest) (*settlementv1.BillResponse, error) {
	bill, err := s.svc.CreateBill(ctx, dto.CreateBillCommand{
		OrderNo:          req.GetOrderNo(),
		UserID:           req.GetUserId(),
		DriverID:         req.GetDriverId(),
		TotalAmount:      req.GetTotalAmount(),
		DiscountFee:      req.GetDiscountFee(),
		PlatformSubsidy:  req.GetPlatformSubsidy(),
		Remark:           req.GetRemark(),
		CommissionRateBP: req.GetCommissionRateBp(),
	})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return toBillProto(bill), nil
}

// SplitBill 处理分账。
func (s *Server) SplitBill(ctx context.Context, req *settlementv1.SplitBillRequest) (*settlementv1.SplitBillResponse, error) {
	orderNo := req.GetOrderNo()
	if orderNo == "" && req.GetBillNo() != "" {
		bill, err := s.svc.FindBillByNo(ctx, req.GetBillNo())
		if err != nil {
			return nil, mapSettlementError(err)
		}
		orderNo = bill.OrderNo
	}
	result, err := s.svc.SplitBill(ctx, dto.SplitBillCommand{
		OrderNo:         orderNo,
		CommissionRate:  req.GetCommissionRateBps(),
		DriverSubsidy:   0, // 司机补贴不在本阶段 proto 暴露，如需可扩展
		PlatformSubsidy: req.GetPlatformSubsidy(),
		IdempotentKey:   req.GetIdempotentKey(),
		UseBillRate:     req.GetUseBillRate(),
	})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	// P9：按真实分账结果回填状态，不再硬编码 PAID；重复分账命中时返回历史状态。
	status := result.Status
	if status == "" {
		status = entity.BillStatusPaid
	}
	return &settlementv1.SplitBillResponse{
		BillNo:             result.BillNo,
		PassengerAmount:    result.PassengerPaid,
		PlatformCommission: result.PlatformCommission,
		DriverIncome:       result.DriverIncome,
		PlatformSubsidy:    result.PlatformSubsidy,
		Status:             status,
		Duplicate:          result.Repeated,
	}, nil
}

// MarkBillPaid 处理支付成功回调，账单 UNPAID → PAID。
func (s *Server) MarkBillPaid(ctx context.Context, req *settlementv1.MarkBillPaidRequest) (*settlementv1.BillResponse, error) {
	bill, err := s.svc.MarkBillPaid(ctx, dto.MarkBillPaidCommand{
		BillNo:      req.GetBillNo(),
		OrderNo:     req.GetOrderNo(),
		PayFee:      req.GetPayFee(),
		DiscountFee: req.GetDiscountFee(),
		RefundFee:   req.GetRefundFee(),
	})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return toBillProto(bill), nil
}

// VoidBill 处理作废账单，CALCULATING/UNPAID → VOID。
func (s *Server) VoidBill(ctx context.Context, req *settlementv1.VoidBillRequest) (*settlementv1.BillResponse, error) {
	bill, err := s.svc.VoidBill(ctx, dto.VoidBillCommand{
		BillNo:  req.GetBillNo(),
		OrderNo: req.GetOrderNo(),
		Reason:  req.GetReason(),
	})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return toBillProto(bill), nil
}

// ListUnpaidBills 处理查询用户待支付账单。
func (s *Server) ListUnpaidBills(ctx context.Context, req *settlementv1.ListUnpaidBillsRequest) (*settlementv1.ListUnpaidBillsResponse, error) {
	bills, err := s.svc.ListUnpaidBillsByUser(ctx, dto.ListUnpaidBillsQuery{UserID: req.GetUserId()})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	out := &settlementv1.ListUnpaidBillsResponse{}
	for _, bill := range bills {
		out.Bills = append(out.Bills, toBillProto(bill))
	}
	return out, nil
}

// CheckUnpaidBan 处理未付禁叫车校验。
func (s *Server) CheckUnpaidBan(ctx context.Context, req *settlementv1.CheckUnpaidBanRequest) (*settlementv1.CheckUnpaidBanResponse, error) {
	ban, err := s.svc.CheckUnpaidBan(ctx, dto.ListUnpaidBillsQuery{UserID: req.GetUserId()})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return &settlementv1.CheckUnpaidBanResponse{
		Banned:          ban.Banned,
		UnpaidCount:     int32(ban.UnpaidCount),
		UnpaidAmountSum: ban.UnpaidAmountSum,
	}, nil
}

// toBillProto 将账单 DTO 转为 proto 视图。
func toBillProto(bill *dto.BillDTO) *settlementv1.BillResponse {
	return &settlementv1.BillResponse{
		BillNo:          bill.ID,
		OrderNo:         bill.OrderNo,
		UserId:          bill.UserID,
		DriverId:        bill.DriverID,
		TotalAmount:     bill.TotalAmount,
		PlatformSubsidy: bill.PlatformSubsidy,
		Status:          bill.Status,
		CreatedAt:       bill.CreatedAt.Format("2006-01-02 15:04:05"),
		PayFee:          bill.PayFee,
		DiscountFee:     bill.DiscountFee,
		RefundFee:       bill.RefundFee,
		DriverIncome:    bill.DriverIncome,
		CommissionFee:   bill.CommissionFee,
		SubsidyFee:      bill.SubsidyFee,
		Settled:         bill.Settled,
	}
}

// GetWalletBalance 处理查询司机钱包。
func (s *Server) GetWalletBalance(ctx context.Context, req *settlementv1.GetWalletBalanceRequest) (*settlementv1.WalletBalanceResponse, error) {
	wallet, err := s.svc.GetWalletBalance(ctx, req.GetDriverId())
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return &settlementv1.WalletBalanceResponse{
		DriverId:      wallet.DriverID,
		Balance:       wallet.Balance,
		FrozenBalance: wallet.FrozenBalance,
		TotalIncome:   wallet.TotalIncome,
		TotalWithdraw: wallet.TotalWithdraw,
	}, nil
}

// ApplyRefund 处理退款冲正（幂等可重试）。
func (s *Server) ApplyRefund(ctx context.Context, req *settlementv1.ApplyRefundRequest) (*settlementv1.ApplyRefundResponse, error) {
	result, err := s.svc.ApplyRefund(ctx, dto.ApplyRefundCommand{
		OrderNo:      req.GetOrderNo(),
		RefundNo:     req.GetRefundNo(),
		RefundAmount: req.GetRefundAmount(),
	})
	if err != nil {
		return nil, mapSettlementError(err)
	}
	return &settlementv1.ApplyRefundResponse{
		BillNo:         result.BillNo,
		OrderNo:        result.OrderNo,
		ReversedAmount: result.ReversedAmount,
		Repeated:       result.Repeated,
	}, nil
}

func mapSettlementError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrBillNotFound):
		return apperrors.NotFound("bill_not_found", "bill not found")
	case stderrors.Is(err, entity.ErrBillAlreadyExists):
		return apperrors.Conflict("bill_already_exists", "bill already exists")
	case stderrors.Is(err, entity.ErrBillAlreadySplit):
		return apperrors.Conflict("bill_already_split", "bill already split")
	case stderrors.Is(err, entity.ErrBillStateTransition):
		return apperrors.Conflict("bill_state_transition", "invalid bill state transition")
	case stderrors.Is(err, entity.ErrInvalidSplitRequest):
		return apperrors.InvalidArgument("invalid_split_request", "invalid split request")
	case stderrors.Is(err, entity.ErrWalletNotFound):
		return apperrors.NotFound("wallet_not_found", "wallet not found")
	case stderrors.Is(err, entity.ErrWalletVersionConflict):
		return apperrors.Conflict("wallet_version_conflict", "wallet version conflict, retry later")
	case stderrors.Is(err, entity.ErrRefundSettlementExceeds):
		return apperrors.InvalidArgument("refund_exceeds_settled_income", "refund amount exceeds driver settled income")
	default:
		return apperrors.Wrap(apperrors.KindInternal, "settlement_service_error", "settlement service error", err)
	}
}
