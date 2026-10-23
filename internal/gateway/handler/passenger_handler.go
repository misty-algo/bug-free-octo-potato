package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// PassengerHandler 把乘客端 HTTP 接口适配到内部 passenger gRPC client。
type PassengerHandler struct {
	client passengerv1.PassengerServiceClient
	log    *zap.Logger
}

// NewPassengerHandler 把 passenger gRPC client 注入 HTTP handler。
func NewPassengerHandler(client passengerv1.PassengerServiceClient, log *zap.Logger) *PassengerHandler {
	return &PassengerHandler{client: client, log: log}
}

// SendCode 处理下发短信验证码。
func (h *PassengerHandler) SendCode(c *gin.Context) {
	var req request.SendCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.SendCode(outgoingContext(c), &passengerv1.SendCodeRequest{
		Phone:    req.Phone,
		DeviceId: req.DeviceID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Login 处理乘客端登录。
func (h *PassengerHandler) Login(c *gin.Context) {
	var req request.PassengerLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Login(outgoingContext(c), &passengerv1.LoginRequest{
		Channel:  req.Channel,
		Phone:    req.Phone,
		Code:     req.Code,
		Password: req.Password,
		Role:     req.Role,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// SetPassword 处理设置登录密码。
func (h *PassengerHandler) SetPassword(c *gin.Context) {
	var req request.SetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	accountID := passengerAccountID(c)
	resp, err := h.client.SetPassword(outgoingContext(c), &passengerv1.SetPasswordRequest{
		AccountId: accountID,
		Phone:     req.Phone,
		Code:      req.Code,
		Password:  req.Password,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// BindPhone 处理绑定手机号。
func (h *PassengerHandler) BindPhone(c *gin.Context) {
	var req request.BindPhoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.BindPhone(outgoingContext(c), &passengerv1.BindPhoneRequest{
		AccountId: passengerAccountID(c),
		Phone:     req.Phone,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Refresh 处理刷新令牌。
func (h *PassengerHandler) Refresh(c *gin.Context) {
	var req request.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Refresh(outgoingContext(c), &passengerv1.RefreshRequest{
		RefreshToken: req.RefreshToken,
		Audience:     req.Audience,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Logout 处理退出登录。
func (h *PassengerHandler) Logout(c *gin.Context) {
	resp, err := h.client.Logout(outgoingContext(c), &passengerv1.LogoutRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// UnbindChannel 处理解绑第三方渠道。
func (h *PassengerHandler) UnbindChannel(c *gin.Context) {
	var req request.UnbindChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.UnbindChannel(outgoingContext(c), &passengerv1.UnbindChannelRequest{
		AccountId: passengerAccountID(c),
		Channel:   req.Channel,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// CancelAccount 处理注销账号。
func (h *PassengerHandler) CancelAccount(c *gin.Context) {
	resp, err := h.client.CancelAccount(outgoingContext(c), &passengerv1.CancelAccountRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// WithdrawCancelAccount 处理撤回注销申请。
func (h *PassengerHandler) WithdrawCancelAccount(c *gin.Context) {
	resp, err := h.client.WithdrawCancelAccount(outgoingContext(c), &passengerv1.CancelAccountRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Home 处理首页合并数据。
func (h *PassengerHandler) Home(c *gin.Context) {
	var req request.HomeRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Home(outgoingContext(c), &passengerv1.HomeRequest{
		AccountId:   req.AccountID,
		Lng:         req.Lng,
		Lat:         req.Lat,
		HasLocation: req.HasLocation,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// SearchPOI 处理地点检索。
func (h *PassengerHandler) SearchPOI(c *gin.Context) {
	var req request.SearchPOIRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.SearchPOI(outgoingContext(c), &passengerv1.SearchPOIRequest{
		AccountId: req.AccountID,
		Keyword:   req.Keyword,
		City:      req.City,
		Lng:       req.Lng,
		Lat:       req.Lat,
		HasCoord:  req.HasCoord,
		Limit:     int32(req.Limit),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// RecommendPickup 处理上车点推荐。
func (h *PassengerHandler) RecommendPickup(c *gin.Context) {
	var req request.RecommendPickupRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.RecommendPickup(outgoingContext(c), &passengerv1.RecommendPickupRequest{
		AccountId: req.AccountID,
		Lng:       req.Lng,
		Lat:       req.Lat,
		Limit:     int32(req.Limit),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// ListAddresses 处理地址列表查询。
func (h *PassengerHandler) ListAddresses(c *gin.Context) {
	var req request.ListAddressesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ListAddresses(outgoingContext(c), &passengerv1.ListAddressesRequest{
		AccountId: req.AccountID,
		Type:      req.Type,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// SaveAddress 处理新增或更新常用地址。
func (h *PassengerHandler) SaveAddress(c *gin.Context) {
	var req request.SaveAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.SaveAddress(outgoingContext(c), &passengerv1.SaveAddressRequest{
		AccountId: passengerAccountID(c),
		Id:        req.ID,
		Type:      req.Type,
		Tag:       req.Tag,
		Name:      req.Name,
		Addr:      req.Addr,
		PoiId:     req.PoiID,
		Lng:       req.Lng,
		Lat:       req.Lat,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// DeleteAddress 处理删除常用地址。
func (h *PassengerHandler) DeleteAddress(c *gin.Context) {
	addressID := c.Query("addressId")
	if addressID == "" {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", "缺少参数 addressId"))
		return
	}
	resp, err := h.client.DeleteAddress(outgoingContext(c), &passengerv1.DeleteAddressRequest{
		AccountId: passengerAccountID(c),
		AddressId: addressID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// RecordHistory 处理上报一次选中的地点。
func (h *PassengerHandler) RecordHistory(c *gin.Context) {
	var req request.RecordHistoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.RecordHistory(outgoingContext(c), &passengerv1.RecordHistoryRequest{
		AccountId: passengerAccountID(c),
		PoiId:     req.POIID,
		Name:      req.Name,
		Addr:      req.Addr,
		Lng:       req.Lng,
		Lat:       req.Lat,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// ClearHistory 处理清空历史地址。
func (h *PassengerHandler) ClearHistory(c *gin.Context) {
	resp, err := h.client.ClearHistory(outgoingContext(c), &passengerv1.ClearHistoryRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// UseQuickEntry 处理记录入口已用过。
func (h *PassengerHandler) UseQuickEntry(c *gin.Context) {
	var req request.UseQuickEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.UseQuickEntry(outgoingContext(c), &passengerv1.UseQuickEntryRequest{
		AccountId: passengerAccountID(c),
		EntryId:   req.EntryID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// ListCoupons 处理可用券列表。
func (h *PassengerHandler) ListCoupons(c *gin.Context) {
	resp, err := h.client.ListCoupons(outgoingContext(c), &passengerv1.ListCouponsRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Estimate 处理生成估价单。
func (h *PassengerHandler) Estimate(c *gin.Context) {
	var req request.PassengerEstimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.Estimate(outgoingContext(c), &passengerv1.EstimateRequest{
		AccountId:      req.AccountID,
		CarType:        int32(req.CarType),
		StartLng:       req.StartLng,
		StartLat:       req.StartLat,
		EndLng:         req.EndLng,
		EndLat:         req.EndLat,
		CouponId:       req.CouponID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// QuotePrice 处理游客看价。游客拿不到估价单。
func (h *PassengerHandler) QuotePrice(c *gin.Context) {
	var req request.PassengerEstimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.QuotePrice(outgoingContext(c), &passengerv1.EstimateRequest{
		CarType:  int32(req.CarType),
		StartLng: req.StartLng,
		StartLat: req.StartLat,
		EndLng:   req.EndLng,
		EndLat:   req.EndLat,
		CouponId: req.CouponID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// CreateOrder 处理创建订单。
func (h *PassengerHandler) CreateOrder(c *gin.Context) {
	var req request.PassengerCreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreateOrder(outgoingContext(c), &passengerv1.CreateOrderRequest{
		AccountId:      req.AccountID,
		EstimateNo:     req.EstimateNo,
		CarType:        int32(req.CarType),
		StartName:      req.StartName,
		StartAddr:      req.StartAddr,
		StartLng:       req.StartLng,
		StartLat:       req.StartLat,
		EndName:        req.EndName,
		EndAddr:        req.EndAddr,
		EndLng:         req.EndLng,
		EndLat:         req.EndLat,
		Remark:         req.Remark,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway passenger order created", zap.String("request_id", httpx.RequestID(c)))
	httpx.OK(c, resp)
}

// GetOrder 处理订单详情。
func (h *PassengerHandler) GetOrder(c *gin.Context) {
	orderID := c.Query("orderId")
	if orderID == "" {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", "缺少参数 orderId"))
		return
	}
	resp, err := h.client.GetOrder(outgoingContext(c), &passengerv1.GetOrderRequest{
		AccountId: passengerAccountID(c),
		OrderId:   orderID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// CurrentOrder 处理进行中订单查询。
func (h *PassengerHandler) CurrentOrder(c *gin.Context) {
	resp, err := h.client.CurrentOrder(outgoingContext(c), &passengerv1.CurrentOrderRequest{
		AccountId: passengerAccountID(c),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// ListOrders 处理订单列表。
func (h *PassengerHandler) ListOrders(c *gin.Context) {
	var req request.ListOrdersRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ListOrders(outgoingContext(c), &passengerv1.ListOrdersRequest{
		AccountId: req.AccountID,
		State:     req.State,
		PageNo:    int32(req.PageNo),
		PageSize:  int32(req.PageSize),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// CancelQuote 处理取消报价，两段式取消的第一步。
func (h *PassengerHandler) CancelQuote(c *gin.Context) {
	orderID := c.Query("orderId")
	if orderID == "" {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", "缺少参数 orderId"))
		return
	}
	resp, err := h.client.CancelQuote(outgoingContext(c), &passengerv1.CancelQuoteRequest{
		AccountId: passengerAccountID(c),
		OrderId:   orderID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// CancelOrder 处理取消订单。
func (h *PassengerHandler) CancelOrder(c *gin.Context) {
	var req request.CancelOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CancelOrder(outgoingContext(c), &passengerv1.CancelOrderRequest{
		AccountId:  req.AccountID,
		OrderId:    req.OrderID,
		Reason:     req.Reason,
		ConfirmFee: req.ConfirmFee,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// DriverLocation 处理司机实时位置查询。
func (h *PassengerHandler) DriverLocation(c *gin.Context) {
	orderID := c.Query("orderId")
	if orderID == "" {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", "缺少参数 orderId"))
		return
	}
	resp, err := h.client.DriverLocation(outgoingContext(c), &passengerv1.DriverLocationRequest{
		AccountId: passengerAccountID(c),
		OrderId:   orderID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// passengerAccountID 从请求头或查询参数取账号标识。
//
// 生产环境应从 JWT 中解析，此处保留查询参数以便本地联调。
func passengerAccountID(c *gin.Context) string {
	if value := c.GetHeader("X-Account-Id"); value != "" {
		return value
	}
	return c.Query("accountId")
}
