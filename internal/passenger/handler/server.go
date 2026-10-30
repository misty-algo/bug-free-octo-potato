// Package handler 把乘客端 gRPC 请求适配到乘客端业务用例。
package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Server 将乘客端 gRPC 请求适配到业务用例。
type Server struct {
	passengerv1.UnimplementedPassengerServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建乘客端 gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	return &Server{svc: svc, log: log}
}

// SendCode 处理下发短信验证码。
func (s *Server) SendCode(ctx context.Context, req *passengerv1.SendCodeRequest) (*passengerv1.SendCodeResponse, error) {
	out, err := s.svc.SendCode(ctx, dto.SendCodeCommand{
		Phone:    req.GetPhone(),
		DeviceID: req.GetDeviceId(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	s.log.Info("passenger verification code sent", zap.String("use_case", "SendCode"))
	return &passengerv1.SendCodeResponse{
		ExpiresInSeconds:   int32(out.ExpiresInSeconds),
		ResendAfterSeconds: int32(out.ResendAfterSeconds),
		DebugCode:          out.DebugCode,
	}, nil
}

// Login 处理登录请求，覆盖短信、微信与 QQ 三种渠道。
func (s *Server) Login(ctx context.Context, req *passengerv1.LoginRequest) (*passengerv1.LoginResponse, error) {
	out, err := s.svc.Login(ctx, dto.LoginCommand{
		Channel:  entity.Channel(req.GetChannel()),
		Phone:    req.GetPhone(),
		Code:     req.GetCode(),
		Password: req.GetPassword(),
		Role:     entity.Role(req.GetRole()),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	s.log.Info("passenger logged in", zap.String("use_case", "Login"), zap.String("aggregate_id", out.AccountID))
	return toLoginProto(out), nil
}

// SetPassword 处理设置登录密码。
func (s *Server) SetPassword(ctx context.Context, req *passengerv1.SetPasswordRequest) (*passengerv1.SetPasswordResponse, error) {
	err := s.svc.SetPassword(ctx, dto.SetPasswordCommand{
		AccountID: req.GetAccountId(),
		Phone:     req.GetPhone(),
		Code:      req.GetCode(),
		Password:  req.GetPassword(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.SetPasswordResponse{Success: true}, nil
}

// BindPhone 处理绑定手机号。
func (s *Server) BindPhone(ctx context.Context, req *passengerv1.BindPhoneRequest) (*passengerv1.BindPhoneResponse, error) {
	out, err := s.svc.BindPhone(ctx, req.GetAccountId(), req.GetPhone())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	// 手机号以掩码形式返回，真实号码不出现在响应里。
	return &passengerv1.BindPhoneResponse{Phone: out.Phone}, nil
}

// Refresh 处理刷新令牌。
func (s *Server) Refresh(ctx context.Context, req *passengerv1.RefreshRequest) (*passengerv1.LoginResponse, error) {
	out, err := s.svc.Refresh(ctx, dto.RefreshCommand{
		RefreshToken: req.GetRefreshToken(),
		Audience:     entity.Audience(req.GetAudience()),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toLoginProto(out), nil
}

// Logout 处理退出登录，撤销该账号全部刷新令牌。
func (s *Server) Logout(ctx context.Context, req *passengerv1.LogoutRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.Logout(ctx, dto.LogoutCommand{AccountID: req.GetAccountId()}); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// UnbindChannel 处理解绑第三方渠道。
func (s *Server) UnbindChannel(ctx context.Context, req *passengerv1.UnbindChannelRequest) (*passengerv1.LogoutResponse, error) {
	err := s.svc.UnbindChannel(ctx, req.GetAccountId(), entity.Channel(req.GetChannel()))
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// CancelAccount 处理注销账号。
func (s *Server) CancelAccount(ctx context.Context, req *passengerv1.CancelAccountRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.CancelAccount(ctx, req.GetAccountId()); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// WithdrawCancelAccount 处理撤回注销申请。
func (s *Server) WithdrawCancelAccount(ctx context.Context, req *passengerv1.CancelAccountRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.WithdrawCancelAccount(ctx, req.GetAccountId()); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// Home 处理首页合并数据。
func (s *Server) Home(ctx context.Context, req *passengerv1.HomeRequest) (*passengerv1.HomeResponse, error) {
	out, err := s.svc.Home(ctx, dto.HomeCommand{
		AccountID:   req.GetAccountId(),
		Lng:         req.GetLng(),
		Lat:         req.GetLat(),
		HasLocation: req.GetHasLocation(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	response := &passengerv1.HomeResponse{
		HasActiveOrder:   out.HasActiveOrder,
		ActiveOrderId:    out.ActiveOrderID,
		RestoreTarget:    out.RestoreTarget,
		CarTypes:         make([]*passengerv1.CarType, 0, len(out.CarTypes)),
		QuickEntries:     make([]*passengerv1.QuickEntry, 0, len(out.QuickEntries)),
		CommonAddresses:  make([]*passengerv1.Address, 0, len(out.CommonAddresses)),
		HistoryAddresses: make([]*passengerv1.Address, 0, len(out.HistoryAddresses)),
	}
	for _, item := range out.CarTypes {
		response.CarTypes = append(response.CarTypes, &passengerv1.CarType{
			CarType:           int32(item.CarType),
			Name:              item.Name,
			Available:         item.Available,
			UnavailableReason: item.UnavailableReason,
			MinFee:            item.MinFee,
		})
	}
	for _, item := range out.QuickEntries {
		response.QuickEntries = append(response.QuickEntries, &passengerv1.QuickEntry{
			EntryId: item.EntryID,
			Title:   item.Title,
			Icon:    item.Icon,
			Action:  item.Action,
			Target:  item.Target,
		})
	}
	for _, item := range out.CommonAddresses {
		response.CommonAddresses = append(response.CommonAddresses, toAddressProto(&item))
	}
	for _, item := range out.HistoryAddresses {
		response.HistoryAddresses = append(response.HistoryAddresses, toAddressProto(&item))
	}
	return response, nil
}

// SearchPOI 处理地点检索。
func (s *Server) SearchPOI(ctx context.Context, req *passengerv1.SearchPOIRequest) (*passengerv1.SearchPOIResponse, error) {
	out, err := s.svc.SearchPOI(ctx, dto.SearchPOICommand{
		AccountID: req.GetAccountId(),
		Keyword:   req.GetKeyword(),
		City:      req.GetCity(),
		Lng:       req.GetLng(),
		Lat:       req.GetLat(),
		HasCoord:  req.GetHasCoord(),
		Limit:     int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.SearchPOIResponse{List: toPOIList(out)}, nil
}

// RecommendPickup 处理上车点推荐。
func (s *Server) RecommendPickup(ctx context.Context, req *passengerv1.RecommendPickupRequest) (*passengerv1.SearchPOIResponse, error) {
	out, err := s.svc.RecommendPickup(ctx, req.GetAccountId(), req.GetLng(), req.GetLat(), int(req.GetLimit()))
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.SearchPOIResponse{List: toPOIList(out)}, nil
}

// ListAddresses 处理地址列表查询。
func (s *Server) ListAddresses(ctx context.Context, req *passengerv1.ListAddressesRequest) (*passengerv1.AddressListResponse, error) {
	out, err := s.svc.ListAddresses(ctx, req.GetAccountId(), entity.AddressType(req.GetType()))
	if err != nil {
		return nil, mapPassengerError(err)
	}
	list := make([]*passengerv1.Address, 0, len(out))
	for _, item := range out {
		list = append(list, toAddressProto(item))
	}
	return &passengerv1.AddressListResponse{List: list}, nil
}

// SaveAddress 处理新增或更新常用地址。
func (s *Server) SaveAddress(ctx context.Context, req *passengerv1.SaveAddressRequest) (*passengerv1.AddressResponse, error) {
	out, err := s.svc.SaveAddress(ctx, req.GetAccountId(), dto.SaveAddressCommand{
		ID:       req.GetId(),
		AddrType: req.GetType(),
		Tag:      req.GetTag(),
		Name:     req.GetName(),
		Addr:     req.GetAddr(),
		POIID:    req.GetPoiId(),
		Lng:      req.GetLng(),
		Lat:      req.GetLat(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.AddressResponse{Address: toAddressProto(out)}, nil
}

// DeleteAddress 处理删除常用地址。
func (s *Server) DeleteAddress(ctx context.Context, req *passengerv1.DeleteAddressRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.DeleteAddress(ctx, req.GetAccountId(), req.GetAddressId()); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// RecordHistory 处理上报一次选中的地点。
func (s *Server) RecordHistory(ctx context.Context, req *passengerv1.RecordHistoryRequest) (*passengerv1.LogoutResponse, error) {
	err := s.svc.RecordHistory(ctx, req.GetAccountId(), dto.RecordHistoryCommand{
		POIID: req.GetPoiId(),
		Name:  req.GetName(),
		Addr:  req.GetAddr(),
		Lng:   req.GetLng(),
		Lat:   req.GetLat(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// ClearHistory 处理清空历史地址。
func (s *Server) ClearHistory(ctx context.Context, req *passengerv1.ClearHistoryRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.ClearHistory(ctx, req.GetAccountId()); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// UseQuickEntry 处理记录入口已用过。
func (s *Server) UseQuickEntry(ctx context.Context, req *passengerv1.UseQuickEntryRequest) (*passengerv1.LogoutResponse, error) {
	if err := s.svc.UseQuickEntry(ctx, req.GetAccountId(), req.GetEntryId()); err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.LogoutResponse{Success: true}, nil
}

// ListCoupons 处理可用券列表。
func (s *Server) ListCoupons(ctx context.Context, req *passengerv1.ListCouponsRequest) (*passengerv1.CouponListResponse, error) {
	out, err := s.svc.ListCoupons(ctx, req.GetAccountId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	list := make([]*passengerv1.Coupon, 0, len(out))
	for _, item := range out {
		list = append(list, &passengerv1.Coupon{
			CouponId:  item.CouponID,
			Name:      item.Name,
			FaceValue: item.FaceValue,
			Threshold: item.Threshold,
			ExpireAt:  item.ExpiresAt.Unix(),
		})
	}
	return &passengerv1.CouponListResponse{List: list}, nil
}

// SubmitRealName 处理实名提交（P20）。
func (s *Server) SubmitRealName(ctx context.Context, req *passengerv1.SubmitRealNameRequest) (*passengerv1.RealNameStatusResponse, error) {
	out, err := s.svc.SubmitRealName(ctx, dto.SubmitRealNameCommand{
		AccountID:      req.GetAccountId(),
		Name:           req.GetName(),
		IDNo:           req.GetIdNo(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toRealNameProto(out), nil
}

// GetRealNameStatus 处理实名状态查询。
func (s *Server) GetRealNameStatus(ctx context.Context, req *passengerv1.GetRealNameStatusRequest) (*passengerv1.RealNameStatusResponse, error) {
	out, err := s.svc.GetRealNameStatus(ctx, req.GetAccountId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toRealNameProto(out), nil
}

// toRealNameProto 把实名状态出参转成 proto 响应。
func toRealNameProto(out *dto.RealNameStatusDTO) *passengerv1.RealNameStatusResponse {
	return &passengerv1.RealNameStatusResponse{
		State:    out.State,
		NameMask: out.NameMask,
		IdNoMask: out.IDNoMask,
	}
}

// ListClaimableCoupons 处理领券中心列表。
func (s *Server) ListClaimableCoupons(ctx context.Context, req *passengerv1.ListClaimableCouponsRequest) (*passengerv1.CouponListResponse, error) {
	out, err := s.svc.ListClaimableCoupons(ctx, req.GetAccountId(), int(req.GetLimit()))
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toCouponListProto(out), nil
}

// ClaimCoupon 处理领取平台券。
func (s *Server) ClaimCoupon(ctx context.Context, req *passengerv1.ClaimCouponRequest) (*passengerv1.Coupon, error) {
	out, err := s.svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{
		AccountID: req.GetAccountId(),
		CouponNo:  req.GetCouponNo(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.Coupon{
		CouponId:  out.CouponID,
		Name:      out.Name,
		FaceValue: out.FaceValue,
		Threshold: out.Threshold,
		ExpireAt:  out.ExpiresAt.Unix(),
	}, nil
}

// toCouponListProto 把券列表出参转成 proto 响应。
func toCouponListProto(items []*dto.CouponDTO) *passengerv1.CouponListResponse {
	list := make([]*passengerv1.Coupon, 0, len(items))
	for _, item := range items {
		list = append(list, &passengerv1.Coupon{
			CouponId:  item.CouponID,
			Name:      item.Name,
			FaceValue: item.FaceValue,
			Threshold: item.Threshold,
			ExpireAt:  item.ExpiresAt.Unix(),
			Claimed:   item.Claimed,
		})
	}
	return &passengerv1.CouponListResponse{List: list}
}

// Estimate 处理生成估价单。
func (s *Server) Estimate(ctx context.Context, req *passengerv1.EstimateRequest) (*passengerv1.EstimateResponse, error) {
	out, err := s.svc.Estimate(ctx, dto.EstimateCommand{
		AccountID:      req.GetAccountId(),
		CarType:        int(req.GetCarType()),
		StartName:      req.GetStartName(),
		StartAddr:      req.GetStartAddr(),
		StartLng:       req.GetStartLng(),
		StartLat:       req.GetStartLat(),
		EndName:        req.GetEndName(),
		EndAddr:        req.GetEndAddr(),
		EndLng:         req.GetEndLng(),
		EndLat:         req.GetEndLat(),
		CouponID:       req.GetCouponId(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toEstimateProto(out), nil
}

// QuotePrice 处理游客看价。游客拿不到估价单。
func (s *Server) QuotePrice(ctx context.Context, req *passengerv1.EstimateRequest) (*passengerv1.EstimateResponse, error) {
	out, err := s.svc.QuotePrice(ctx, dto.EstimateCommand{
		AccountID: req.GetAccountId(),
		CarType:   int(req.GetCarType()),
		StartLng:  req.GetStartLng(),
		StartLat:  req.GetStartLat(),
		EndLng:    req.GetEndLng(),
		EndLat:    req.GetEndLat(),
		CouponID:  req.GetCouponId(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return toEstimateProto(out), nil
}

// CreateOrder 处理创建订单。
func (s *Server) CreateOrder(ctx context.Context, req *passengerv1.CreateOrderRequest) (*passengerv1.OrderResponse, error) {
	out, err := s.svc.CreateOrder(ctx, dto.CreateOrderCommand{
		AccountID:      req.GetAccountId(),
		EstimateNo:     req.GetEstimateNo(),
		CarType:        int(req.GetCarType()),
		StartName:      req.GetStartName(),
		StartAddr:      req.GetStartAddr(),
		StartLng:       req.GetStartLng(),
		StartLat:       req.GetStartLat(),
		EndName:        req.GetEndName(),
		EndAddr:        req.GetEndAddr(),
		EndLng:         req.GetEndLng(),
		EndLat:         req.GetEndLat(),
		Remark:         req.GetRemark(),
		IdempotencyKey: req.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	s.log.Info("passenger order created", zap.String("use_case", "CreateOrder"), zap.String("aggregate_id", out.OrderID))
	return &passengerv1.OrderResponse{Order: toOrderProto(out)}, nil
}

// GetOrder 处理订单详情。
func (s *Server) GetOrder(ctx context.Context, req *passengerv1.GetOrderRequest) (*passengerv1.OrderResponse, error) {
	out, err := s.svc.GetOrder(ctx, req.GetAccountId(), req.GetOrderId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.OrderResponse{Order: toOrderProto(out)}, nil
}

// CurrentOrder 处理进行中订单查询。
func (s *Server) CurrentOrder(ctx context.Context, req *passengerv1.CurrentOrderRequest) (*passengerv1.OrderResponse, error) {
	out, err := s.svc.CurrentOrder(ctx, req.GetAccountId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.OrderResponse{Order: toOrderProto(out)}, nil
}

// ListOrders 处理订单列表。
func (s *Server) ListOrders(ctx context.Context, req *passengerv1.ListOrdersRequest) (*passengerv1.OrderListResponse, error) {
	out, err := s.svc.ListOrders(ctx, req.GetAccountId(), entity.OrderState(req.GetState()), int(req.GetPageNo()), int(req.GetPageSize()))
	if err != nil {
		return nil, mapPassengerError(err)
	}
	list := make([]*passengerv1.Order, 0, len(out.List))
	for _, item := range out.List {
		list = append(list, toOrderProto(item))
	}
	return &passengerv1.OrderListResponse{Total: int32(out.Total), List: list}, nil
}

// CancelQuote 处理取消报价，两段式取消的第一步。
func (s *Server) CancelQuote(ctx context.Context, req *passengerv1.CancelQuoteRequest) (*passengerv1.CancelQuoteResponse, error) {
	out, err := s.svc.CancelQuote(ctx, req.GetAccountId(), req.GetOrderId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.CancelQuoteResponse{
		NeedConfirm: out.NeedConfirm,
		CancelFee:   out.CancelFee,
		State:       out.State,
		ExpiresAt:   out.ExpiresAt.Unix(),
	}, nil
}

// CancelOrder 处理取消订单。
func (s *Server) CancelOrder(ctx context.Context, req *passengerv1.CancelOrderRequest) (*passengerv1.CancelQuoteResponse, error) {
	out, err := s.svc.CancelOrder(ctx, dto.CancelOrderCommand{
		AccountID:  req.GetAccountId(),
		OrderID:    req.GetOrderId(),
		Reason:     req.GetReason(),
		ConfirmFee: req.GetConfirmFee(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.CancelQuoteResponse{
		NeedConfirm: out.NeedConfirm,
		CancelFee:   out.CancelFee,
		State:       out.State,
	}, nil
}

// DriverLocation 处理司机实时位置查询。
// MarkOrderPaid 支付成功回调：订单推进到已完成并返回司机结算要素。
func (s *Server) MarkOrderPaid(ctx context.Context, req *passengerv1.MarkOrderPaidRequest) (*passengerv1.MarkOrderPaidResponse, error) {
	out, err := s.svc.MarkOrderPaid(ctx, dto.MarkOrderPaidCommand{
		OrderNo:   req.GetOrderNo(),
		PaymentNo: req.GetPaymentNo(),
		TradeNo:   req.GetTradeNo(),
		PaidFen:   req.GetPaidFen(),
	})
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.MarkOrderPaidResponse{
		OrderId:         out.OrderID,
		OrderNo:         out.OrderNo,
		State:           out.State,
		DriverId:        out.DriverID,
		PaidFen:         out.PaidFen,
		DriverIncomeFen: out.DriverIncomeFen,
	}, nil
}

func (s *Server) DriverLocation(ctx context.Context, req *passengerv1.DriverLocationRequest) (*passengerv1.DriverLocationResponse, error) {
	out, err := s.svc.DriverLocation(ctx, req.GetAccountId(), req.GetOrderId())
	if err != nil {
		return nil, mapPassengerError(err)
	}
	return &passengerv1.DriverLocationResponse{
		DriverId:   out.DriverID,
		DriverName: out.DriverName,
		PlateNo:    out.PlateNo,
		CarModel:   out.CarModel,
		Rating:     out.Rating,
		Lng:        out.Lng,
		Lat:        out.Lat,
		EtaMin:     int32(out.ETAMin),
		Delayed:    out.Delayed,
		ReportedAt: out.ReportedAt.Unix(),
	}, nil
}

// toLoginProto 把登录出参转成 proto 结构。
func toLoginProto(out *dto.LoginDTO) *passengerv1.LoginResponse {
	return &passengerv1.LoginResponse{
		AccessToken:        out.AccessToken,
		RefreshToken:       out.RefreshToken,
		AccountId:          out.AccountID,
		IsNewUser:          out.IsNewUser,
		HasPassword:        out.HasPassword,
		RestoreTarget:      out.RestoreTarget,
		NeedDriverRegister: out.NeedDriverRegister,
		ExpiresInSeconds:   int32(out.ExpiresInSeconds),
	}
}

// toEstimateProto 把估价出参转成 proto 结构。
func toEstimateProto(out *dto.EstimateDTO) *passengerv1.EstimateResponse {
	response := &passengerv1.EstimateResponse{
		EstimateNo:  out.EstimateNo,
		CarType:     int32(out.CarType),
		BaseFee:     out.BaseFee,
		ServiceFee:  out.ServiceFee,
		DiscountFee: out.DiscountFee,
		PayFee:      out.PayFee,
		DistanceM:   out.DistanceM,
		DurationS:   out.DurationS,
	}
	// 游客看价没有过期时间。零值时间转 Unix 会得到一个无意义的负数，
	// 这里保持零值，前端按「无过期时间」处理。
	if !out.ExpiresAt.IsZero() {
		response.ExpiresAt = out.ExpiresAt.Unix()
	}
	return response
}

// toOrderProto 把订单出参转成 proto 结构。
func toOrderProto(out *dto.OrderDTO) *passengerv1.Order {
	if out == nil {
		return nil
	}
	return &passengerv1.Order{
		OrderId:     out.OrderID,
		OrderNo:     out.OrderNo,
		State:       out.State,
		CarType:     int32(out.CarType),
		StartName:   out.StartName,
		StartAddr:   out.StartAddr,
		EndName:     out.EndName,
		EndAddr:     out.EndAddr,
		TotalFee:    out.TotalFee,
		CancelFee:   out.CancelFee,
		Remark:      out.Remark,
		DriverName:  out.DriverName,
		DriverPlate: out.DriverPlate,
		CreatedAt:   out.CreatedAt.Unix(),
	}
}

// toAddressProto 把地址出参转成 proto 结构。
func toAddressProto(out *dto.AddressDTO) *passengerv1.Address {
	if out == nil {
		return nil
	}
	return &passengerv1.Address{
		Id:       out.ID,
		Type:     out.Type,
		Tag:      out.Tag,
		Name:     out.Name,
		Addr:     out.Addr,
		PoiId:    out.POIID,
		Lng:      out.Lng,
		Lat:      out.Lat,
		UseCount: int32(out.UseCount),
	}
}

// toPOIList 把地点出参列表转成 proto 结构。
func toPOIList(out []*dto.POIDTO) []*passengerv1.POI {
	list := make([]*passengerv1.POI, 0, len(out))
	for _, item := range out {
		list = append(list, &passengerv1.POI{
			Id:          item.ID,
			Name:        item.Name,
			Addr:        item.Addr,
			City:        item.City,
			Lng:         item.Lng,
			Lat:         item.Lat,
			HasDistance: item.HasDistance,
			DistanceM:   item.DistanceM,
		})
	}
	return list
}

// mapPassengerError 把领域错误映射成统一的应用错误。
func mapPassengerError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrInvalidPhone),
		stderrors.Is(err, entity.ErrInvalidPassword),
		stderrors.Is(err, entity.ErrInvalidRole),
		stderrors.Is(err, entity.ErrInvalidChannel),
		stderrors.Is(err, entity.ErrInvalidCoordinate),
		stderrors.Is(err, entity.ErrInvalidCode),
		stderrors.Is(err, entity.ErrCodeExpired),
		stderrors.Is(err, entity.ErrCodeUsed),
		stderrors.Is(err, entity.ErrTooClose),
		stderrors.Is(err, entity.ErrCodeRateLimited),
		stderrors.Is(err, entity.ErrPasswordNotSet):
		return apperrors.InvalidArgument("invalid_passenger_request", err.Error())
	case stderrors.Is(err, entity.ErrInvalidCredentials):
		return apperrors.Unauthorized("invalid_credentials", "手机号或密码不正确")
	case stderrors.Is(err, entity.ErrUnauthorized):
		return apperrors.Unauthorized("unauthorized", "未登录或登录已过期")
	case stderrors.Is(err, entity.ErrForbidden):
		return apperrors.New(apperrors.KindUnauthorized, "forbidden", "无权限访问该资源")
	case stderrors.Is(err, entity.ErrAccountDisabled):
		return apperrors.New(apperrors.KindUnauthorized, "account_disabled", "账号已被禁用")
	case stderrors.Is(err, entity.ErrAccountNotFound),
		stderrors.Is(err, entity.ErrEstimateNotFound),
		stderrors.Is(err, entity.ErrAddressNotFound),
		stderrors.Is(err, entity.ErrOrderNotFound),
		stderrors.Is(err, entity.ErrCouponNotFound),
		stderrors.Is(err, entity.ErrDriverNotRegistered),
		stderrors.Is(err, entity.ErrDriverNotAssigned),
		stderrors.Is(err, entity.ErrChannelNotBound):
		return apperrors.NotFound("passenger_resource_not_found", err.Error())
	case stderrors.Is(err, entity.ErrPhoneAlreadyUsed),
		stderrors.Is(err, entity.ErrOrderStateConflict),
		stderrors.Is(err, entity.ErrEstimateUsed),
		stderrors.Is(err, entity.ErrEstimateExpired),
		stderrors.Is(err, entity.ErrCouponLocked),
		stderrors.Is(err, entity.ErrCouponUnavailable),
		stderrors.Is(err, entity.ErrCancelNotAllowed):
		return apperrors.Conflict("passenger_state_conflict", err.Error())
	default:
		return apperrors.Wrap(apperrors.KindInternal, "passenger_service_error", "乘客端服务错误", err)
	}
}
