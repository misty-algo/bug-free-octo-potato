package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Server 将 shop gRPC 请求适配到 service 用例。
type Server struct {
	shopv1.UnimplementedShopServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建 shop gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	return &Server{svc: svc, log: log}
}

// CreateShop 处理创建 RPC。
func (s *Server) CreateShop(ctx context.Context, req *shopv1.CreateShopRequest) (*shopv1.ShopResponse, error) {
	item, err := s.svc.Create(ctx, dto.CreateCommand{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return toProto(item), nil
}

// GetShop 处理按 ID 查询。
func (s *Server) GetShop(ctx context.Context, req *shopv1.GetShopRequest) (*shopv1.ShopResponse, error) {
	item, err := s.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapShopError(err)
	}
	return toProto(item), nil
}

// ListShops 处理分页列表查询。
func (s *Server) ListShops(ctx context.Context, req *shopv1.ListShopsRequest) (*shopv1.ListShopsResponse, error) {
	list, err := s.svc.List(ctx, dto.ListCommand{
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	resp := &shopv1.ListShopsResponse{
		Items: make([]*shopv1.ShopResponse, 0, len(list.Items)),
		Total: list.Total,
	}
	for _, item := range list.Items {
		resp.Items = append(resp.Items, toProto(item))
	}
	return resp, nil
}

// UpdateShop 处理按 ID 更新。
func (s *Server) UpdateShop(ctx context.Context, req *shopv1.UpdateShopRequest) (*shopv1.ShopResponse, error) {
	item, err := s.svc.Update(ctx, dto.UpdateCommand{
		ID:          req.GetId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return toProto(item), nil
}

// DeleteShop 处理按 ID 删除。
func (s *Server) DeleteShop(ctx context.Context, req *shopv1.DeleteShopRequest) (*shopv1.DeleteShopResponse, error) {
	if err := s.svc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.DeleteShopResponse{Success: true}, nil
}

// NotifyPayment 处理支付渠道异步通知：验签并更新支付单状态。
func (s *Server) NotifyPayment(ctx context.Context, req *shopv1.NotifyPaymentRequest) (*shopv1.NotifyPaymentResponse, error) {
	result, err := s.svc.NotifyPayment(ctx, dto.NotifyCommand{
		Channel: req.GetChannel(),
		Form:    req.GetForm(),
		Payload: req.GetPayload(),
		Headers: req.GetHeaders(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.NotifyPaymentResponse{
		Success: true,
		OrderNo: result.OrderNo,
		TradeNo: result.TradeNo,
		Status:  result.Status,
	}, nil
}

// QueryPayment 按 order_no 查询支付单，pending 时主动刷新渠道状态。
func (s *Server) QueryPayment(ctx context.Context, req *shopv1.QueryPaymentRequest) (*shopv1.QueryPaymentResponse, error) {
	result, err := s.svc.QueryPayment(ctx, req.GetOrderNo())
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.QueryPaymentResponse{
		OrderNo:     result.OrderNo,
		Status:      result.Status,
		Channel:     result.Channel,
		TradeNo:     result.TradeNo,
		TotalAmount: result.TotalAmount,
		PaidAtUnix:  result.PaidAtUnix,
	}, nil
}

// Pay 处理 WAP/PC 网页支付下单。
func (s *Server) Pay(ctx context.Context, req *shopv1.PayRequest) (*shopv1.PayResponse, error) {
	result, err := s.svc.Pay(ctx, dto.PayCommand{
		OrderNo:     req.GetOrderNo(),
		Subject:     req.GetSubject(),
		TotalAmount: req.GetTotalAmount(),
		Body:        req.GetBody(),
		Scene:       req.GetScene(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.PayResponse{OrderNo: result.OrderNo, PayUrl: result.PayURL}, nil
}

// ScanPay 处理扫码支付下单。
func (s *Server) ScanPay(ctx context.Context, req *shopv1.ScanPayRequest) (*shopv1.ScanPayResponse, error) {
	result, err := s.svc.ScanPay(ctx, dto.ScanPayCommand{
		OrderNo:     req.GetOrderNo(),
		Subject:     req.GetSubject(),
		TotalAmount: req.GetTotalAmount(),
		Body:        req.GetBody(),
		Channel:     req.GetChannel(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.ScanPayResponse{
		OrderNo:     result.OrderNo,
		QrCode:      result.QrCode,
		TotalAmount: result.TotalAmount,
	}, nil
}

// BarCodePay 处理付款码支付（商家扫用户付款码）。
func (s *Server) BarCodePay(ctx context.Context, req *shopv1.BarCodePayRequest) (*shopv1.BarCodePayResponse, error) {
	result, err := s.svc.BarCodePay(ctx, dto.BarCodePayCommand{
		OrderNo:     req.GetOrderNo(),
		Subject:     req.GetSubject(),
		TotalAmount: req.GetTotalAmount(),
		AuthCode:    req.GetAuthCode(),
		Body:        req.GetBody(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.BarCodePayResponse{
		TradeNo:      result.TradeNo,
		OrderNo:      result.OrderNo,
		TotalAmount:  result.TotalAmount,
		BuyerLogonId: result.BuyerLogonID,
	}, nil
}

// Refund 处理退款。
func (s *Server) Refund(ctx context.Context, req *shopv1.RefundRequest) (*shopv1.RefundResponse, error) {
	result, err := s.svc.Refund(ctx, dto.RefundCommand{
		OrderNo:      req.GetOrderNo(),
		TradeNo:      req.GetTradeNo(),
		RefundAmount: req.GetRefundAmount(),
		RefundReason: req.GetRefundReason(),
		OutRequestNo: req.GetOutRequestNo(),
		Channel:      req.GetChannel(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.RefundResponse{
		Success:      result.Success,
		TradeNo:      result.TradeNo,
		OrderNo:      result.OrderNo,
		RefundAmount: result.RefundAmount,
	}, nil
}

func toProto(item *dto.ShopDTO) *shopv1.ShopResponse {
	return &shopv1.ShopResponse{
		Id:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func mapShopError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrInvalidShop):
		return apperrors.InvalidArgument("invalid_shop", "invalid shop input")
	case stderrors.Is(err, entity.ErrShopNotFound):
		return apperrors.NotFound("shop_not_found", "shop not found")
	case stderrors.Is(err, entity.ErrInvalidPaymentRequest):
		return apperrors.InvalidArgument("invalid_payment_request", "invalid payment request")
	case stderrors.Is(err, entity.ErrAlipayNotConfigured):
		return apperrors.Internal("alipay_not_configured", "alipay not configured")
	case stderrors.Is(err, entity.ErrWechatNotConfigured):
		return apperrors.Internal("wechat_not_configured", "wechat not configured")
	case stderrors.Is(err, entity.ErrPaymentNotFound):
		return apperrors.NotFound("payment_not_found", "payment not found")
	case stderrors.Is(err, entity.ErrPaymentExists):
		return apperrors.InvalidArgument("payment_exists", "payment order already exists")
	case stderrors.Is(err, entity.ErrPaymentChannelUnsupported):
		return apperrors.InvalidArgument("payment_channel_unsupported", "unsupported payment channel")
	case stderrors.Is(err, entity.ErrTrajectoryNotFound):
		return apperrors.NotFound("trajectory_not_found", "trajectory not found")
	case stderrors.Is(err, entity.ErrInvalidVirtualNumber):
		return apperrors.InvalidArgument("invalid_virtual_number", "invalid virtual number")
	case stderrors.Is(err, entity.ErrVirtualNumberNotFound):
		return apperrors.NotFound("virtual_number_not_found", "virtual number not found")
	case stderrors.Is(err, entity.ErrVirtualNumberAlreadyReleased):
		return apperrors.InvalidArgument("virtual_number_already_released", "virtual number already released")
	case stderrors.Is(err, entity.ErrInvalidDestinationChange):
		return apperrors.InvalidArgument("invalid_destination_change", "invalid destination change")
	case stderrors.Is(err, entity.ErrDestinationChangeNotFound):
		return apperrors.NotFound("destination_change_not_found", "destination change not found")
	case stderrors.Is(err, entity.ErrDestinationChangeNotPending):
		return apperrors.InvalidArgument("destination_change_not_pending", "destination change is not pending")
	case stderrors.Is(err, entity.ErrDestinationChangeExpired):
		return apperrors.InvalidArgument("destination_change_expired", "destination change confirm window expired")
	case stderrors.Is(err, entity.ErrDestinationChangeLimitHit):
		return apperrors.InvalidArgument("destination_change_limit_hit", "destination change limit reached")
	case stderrors.Is(err, entity.ErrDestinationChangeVersionConflict):
		return apperrors.Conflict("destination_change_version_conflict", "destination change version conflict")
	case stderrors.Is(err, entity.ErrInvalidTripShare):
		return apperrors.InvalidArgument("invalid_trip_share", "invalid trip share")
	case stderrors.Is(err, entity.ErrTripShareNotFound):
		return apperrors.NotFound("trip_share_not_found", "trip share not found")
	case stderrors.Is(err, entity.ErrTripShareExpired):
		return apperrors.InvalidArgument("trip_share_expired", "trip share expired or revoked")
	case stderrors.Is(err, entity.ErrInvalidCancelQuote):
		return apperrors.InvalidArgument("invalid_cancel_quote", "invalid cancel quote")
	case stderrors.Is(err, entity.ErrCancelQuoteNotFound):
		return apperrors.NotFound("cancel_quote_not_found", "cancel quote not found")
	case stderrors.Is(err, entity.ErrCancelQuoteUsed):
		return apperrors.InvalidArgument("cancel_quote_used", "cancel quote already used")
	case stderrors.Is(err, entity.ErrCancelQuoteExpired):
		return apperrors.InvalidArgument("cancel_quote_expired", "cancel quote expired")
	case stderrors.Is(err, entity.ErrCancelQuoteVersionMismatch):
		return apperrors.Conflict("cancel_quote_version_mismatch", "cancel quote bound version mismatch")
	default:
		return apperrors.Wrap(apperrors.KindInternal, "shop_service_error", "shop service error", err)
	}
}
