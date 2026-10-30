package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	paymentv1 "github.com/BwCloudWeGo/bw-cli/api/gen/payment/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/payment/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/payment/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/payment/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Server 将 payment gRPC 请求适配到 service 用例。
type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建 payment gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	return &Server{svc: svc, log: log}
}

// CreatePayment 处理创建支付单。
func (s *Server) CreatePayment(ctx context.Context, req *paymentv1.CreatePaymentRequest) (*paymentv1.PaymentResponse, error) {
	item, err := s.svc.CreatePayment(ctx, dto.CreatePaymentCommand{
		OrderNo:       req.GetOrderNo(),
		Subject:       req.GetSubject(),
		Amount:        req.GetAmount(),
		Channel:       req.GetChannel(),
		UserID:        req.GetUserId(),
		IdempotentKey: req.GetIdempotentKey(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return toPaymentProto(item), nil
}

// BeginPayment 渠道预支付受理回调：CREATED → PROCESSING（幂等）。
func (s *Server) BeginPayment(ctx context.Context, req *paymentv1.BeginPaymentRequest) (*paymentv1.PaymentResponse, error) {
	item, err := s.svc.BeginPayment(ctx, dto.BeginPaymentCommand{
		OrderNo:  req.GetOrderNo(),
		PrepayID: req.GetPrepayId(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return toPaymentProto(item), nil
}

// FailPayment 渠道支付失败通知：CREATED/PROCESSING → FAILED。
func (s *Server) FailPayment(ctx context.Context, req *paymentv1.FailPaymentRequest) (*paymentv1.PaymentResponse, error) {
	item, err := s.svc.FailPayment(ctx, dto.FailPaymentCommand{
		OrderNo:    req.GetOrderNo(),
		FailReason: req.GetFailReason(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return toPaymentProto(item), nil
}

// ClosePayment 关单：CREATED/PROCESSING → CLOSED（幂等）。
func (s *Server) ClosePayment(ctx context.Context, req *paymentv1.ClosePaymentRequest) (*paymentv1.PaymentResponse, error) {
	item, err := s.svc.ClosePayment(ctx, dto.ClosePaymentCommand{
		OrderNo: req.GetOrderNo(),
		Reason:  req.GetReason(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return toPaymentProto(item), nil
}

// NotifyPayment 处理异步支付成功通知。
func (s *Server) NotifyPayment(ctx context.Context, req *paymentv1.NotifyPaymentRequest) (*paymentv1.NotifyPaymentResponse, error) {
	result, err := s.svc.NotifyPayment(ctx, dto.NotifyPaymentCommand{
		OrderNo:    req.GetOrderNo(),
		TradeNo:    req.GetTradeNo(),
		Channel:    req.GetChannel(),
		PaidAmount: req.GetPaidAmount(),
		PaidAt:     req.GetPaidAtUnix(),
	})
	if err != nil {
		s.log.Error("notify payment failed", zap.String("order_no", req.GetOrderNo()), zap.Error(err))
		return nil, mapPaymentError(err)
	}
	return &paymentv1.NotifyPaymentResponse{
		Success:   result.Success,
		PaymentNo: result.PaymentNo,
		Status:    result.Status,
		Duplicate: result.Duplicate,
	}, nil
}

// QueryPayment 处理按订单号查单。
func (s *Server) QueryPayment(ctx context.Context, req *paymentv1.QueryPaymentRequest) (*paymentv1.PaymentResponse, error) {
	item, err := s.svc.QueryPayment(ctx, req.GetOrderNo())
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return toPaymentProto(item), nil
}

// RefundPayment 处理退款。
func (s *Server) RefundPayment(ctx context.Context, req *paymentv1.RefundPaymentRequest) (*paymentv1.RefundPaymentResponse, error) {
	result, err := s.svc.RefundPayment(ctx, dto.RefundPaymentCommand{
		OrderNo:      req.GetOrderNo(),
		RefundAmount: req.GetRefundAmount(),
		Reason:       req.GetReason(),
		OutRequestNo: req.GetOutRequestNo(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return &paymentv1.RefundPaymentResponse{
		Success:      result.Success,
		RefundNo:     result.RefundNo,
		PaymentNo:    result.PaymentNo,
		OrderNo:      result.OrderNo,
		RefundAmount: result.RefundAmount,
		Status:       result.Status,
		Duplicate:    result.Duplicate,
	}, nil
}

// ReviewRefund 审核退款单：通过后提交渠道 / 拒绝（幂等由退款单状态机守卫）。
func (s *Server) ReviewRefund(ctx context.Context, req *paymentv1.ReviewRefundRequest) (*paymentv1.RefundPaymentResponse, error) {
	result, err := s.svc.ReviewRefund(ctx, dto.ReviewRefundCommand{
		RefundNo:     req.GetRefundNo(),
		Approved:     req.GetApproved(),
		RejectReason: req.GetRejectReason(),
	})
	if err != nil {
		return nil, mapPaymentError(err)
	}
	return &paymentv1.RefundPaymentResponse{
		Success:      result.Success,
		RefundNo:     result.RefundNo,
		PaymentNo:    result.PaymentNo,
		OrderNo:      result.OrderNo,
		RefundAmount: result.RefundAmount,
		Status:       result.Status,
		Duplicate:    result.Duplicate,
	}, nil
}

func toPaymentProto(item *dto.PaymentDTO) *paymentv1.PaymentResponse {
	resp := &paymentv1.PaymentResponse{
		PaymentNo:      item.ID,
		OrderNo:        item.OrderNo,
		UserId:         item.UserID,
		Amount:         item.Amount,
		PaidAmount:     item.PaidAmount,
		Status:         item.Status,
		Channel:        item.Channel,
		TradeNo:        item.TradeNo,
		RefundedAmount: item.RefundedAmount,
		PrepayId:       item.PrepayID,
		FailReason:     item.FailReason,
		NotifyCount:    int32(item.NotifyCount),
		CreatedAt:      item.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:      item.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
	if item.PaidAt != nil {
		resp.PaidAtUnix = item.PaidAt.Unix()
	}
	if item.ExpireAt != nil {
		resp.ExpireAtUnix = item.ExpireAt.Unix()
	}
	return resp
}

func mapPaymentError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrPaymentNotFound):
		return apperrors.NotFound("payment_not_found", "payment not found")
	case stderrors.Is(err, entity.ErrPaymentAlreadyExists):
		return apperrors.Conflict("payment_already_exists", "payment already exists")
	case stderrors.Is(err, entity.ErrPaymentAlreadyPaid):
		return apperrors.Conflict("payment_already_paid", "payment already paid")
	case stderrors.Is(err, entity.ErrPaymentClosed):
		return apperrors.Conflict("payment_closed", "payment closed")
	case stderrors.Is(err, entity.ErrInvalidPaymentRequest):
		return apperrors.InvalidArgument("invalid_payment_request", "invalid payment request")
	case stderrors.Is(err, entity.ErrRefundNotFound):
		return apperrors.NotFound("refund_not_found", "refund not found")
	case stderrors.Is(err, entity.ErrRefundAlreadyExists):
		return apperrors.Conflict("refund_already_exists", "refund already exists")
	case stderrors.Is(err, entity.ErrRefundChannelFailed):
		return apperrors.Internal("refund_channel_failed", "refund channel failed")
	case stderrors.Is(err, entity.ErrRefundExceedsPaid):
		return apperrors.InvalidArgument("refund_exceeds_paid", "refund amount exceeds paid amount")
	case stderrors.Is(err, entity.ErrRefundStateTransition):
		return apperrors.Conflict("refund_state_transition", "invalid refund state transition")
	default:
		// 未映射错误记录明细（消息仍不外泄内部实现），否则排障时只见笼统的
		// payment_service_error 无从下手。
		zap.L().Error("unmapped payment service error", zap.Error(err))
		return apperrors.Wrap(apperrors.KindInternal, "payment_service_error", "payment service error", err)
	}
}
