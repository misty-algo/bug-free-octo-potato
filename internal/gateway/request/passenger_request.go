// request 包存放 gateway 层的 HTTP 请求 DTO。
package request

// SendCodeRequest 是 POST /api/v1/passenger/auth/sms-code 使用的 JSON 载荷。
type SendCodeRequest struct {
	Phone string `json:"phone" binding:"required"`
	// DeviceID 用于同设备小时级限流，可空。
	DeviceID string `json:"deviceId"`
}

// PassengerLoginRequest 是 POST /api/v1/passenger/auth/login 使用的 JSON 载荷。
type PassengerLoginRequest struct {
	// Channel 决定凭据类型：sms / wechat / qq。
	Channel string `json:"channel" binding:"required"`
	Phone   string `json:"phone"`
	// Code 为短信验证码或第三方授权码。
	Code string `json:"code"`
	// Password 为密码登录的明文密码。
	Password string `json:"password"`
	// Role 决定进入乘客端还是司机端。
	Role string `json:"role" binding:"required"`
}

// SetPasswordRequest 是 POST /api/v1/passenger/auth/set-password 使用的 JSON 载荷。
type SetPasswordRequest struct {
	Phone    string `json:"phone" binding:"required"`
	Code     string `json:"code" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// BindPhoneRequest 是 POST /api/v1/passenger/auth/bind-phone 使用的 JSON 载荷。
type BindPhoneRequest struct {
	Phone string `json:"phone" binding:"required"`
}

// RefreshRequest 是 POST /api/v1/passenger/auth/refresh 使用的 JSON 载荷。
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
	// Audience 声明刷新方所属受众，服务端校验，防止跨端刷新。
	Audience string `json:"audience"`
}

// UnbindChannelRequest 是 POST /api/v1/passenger/auth/unbind-channel 使用的 JSON 载荷。
type UnbindChannelRequest struct {
	Channel string `json:"channel" binding:"required"`
}

// HomeRequest 是 GET /api/v1/passenger/home 使用的查询参数。
type HomeRequest struct {
	AccountID string  `form:"accountId"`
	Lng       float64 `form:"lng"`
	Lat       float64 `form:"lat"`
	// HasLocation 标识前端是否成功取得定位。
	HasLocation bool `form:"hasLocation"`
}

// SearchPOIRequest 是 GET /api/v1/passenger/poi/search 使用的查询参数。
type SearchPOIRequest struct {
	AccountID string  `form:"accountId"`
	Keyword   string  `form:"keyword" binding:"required"`
	City      string  `form:"city"`
	Lng       float64 `form:"lng"`
	Lat       float64 `form:"lat"`
	HasCoord  bool    `form:"hasCoord"`
	Limit     int     `form:"limit"`
}

// RecommendPickupRequest 是 GET /api/v1/passenger/poi/recommend 使用的查询参数。
type RecommendPickupRequest struct {
	AccountID string  `form:"accountId"`
	Lng       float64 `form:"lng" binding:"required"`
	Lat       float64 `form:"lat" binding:"required"`
	Limit     int     `form:"limit"`
}

// ListAddressesRequest 是 GET /api/v1/passenger/addresses 使用的查询参数。
type ListAddressesRequest struct {
	AccountID string `form:"accountId"`
	Type      string `form:"type"`
}

// SaveAddressRequest 是 POST /api/v1/passenger/addresses 使用的 JSON 载荷。
type SaveAddressRequest struct {
	ID   string `json:"id"`
	Type string `json:"type" binding:"required"`
	Tag  string `json:"tag"`
	Name string `json:"name" binding:"required"`
	Addr string `json:"addr" binding:"required"`
	// PoiID 可空。
	PoiID string  `json:"poiId"`
	Lng   float64 `json:"lng"`
	Lat   float64 `json:"lat"`
}

// RecordHistoryRequest 是 POST /api/v1/passenger/addresses/history 使用的 JSON 载荷。
type RecordHistoryRequest struct {
	POIID string  `json:"poiId"`
	Name  string  `json:"name" binding:"required"`
	Addr  string  `json:"addr"`
	Lng   float64 `json:"lng"`
	Lat   float64 `json:"lat"`
}

// UseQuickEntryRequest 是 POST /api/v1/passenger/home/quick-entries/use 使用的 JSON 载荷。
type UseQuickEntryRequest struct {
	EntryID string `json:"entryId" binding:"required"`
}

// PassengerEstimateRequest 是 POST /api/v1/passenger/estimate 使用的 JSON 载荷。
type PassengerEstimateRequest struct {
	AccountID string  `json:"accountId"`
	CarType   int     `json:"carType" binding:"required"`
	StartLng  float64 `json:"startLng" binding:"required"`
	StartLat  float64 `json:"startLat" binding:"required"`
	EndLng    float64 `json:"endLng" binding:"required"`
	EndLat    float64 `json:"endLat" binding:"required"`
	// CouponID 可空。
	CouponID string `json:"couponId"`
	// IdempotencyKey 保证重复提交只产生一张估价单。
	IdempotencyKey string `json:"idempotencyKey"`
}

// PassengerCreateOrderRequest 是 POST /api/v1/passenger/orders 使用的 JSON 载荷。
type PassengerCreateOrderRequest struct {
	AccountID  string  `json:"accountId" binding:"required"`
	EstimateNo string  `json:"estimateNo" binding:"required"`
	CarType    int     `json:"carType" binding:"required"`
	StartName  string  `json:"startName" binding:"required"`
	StartAddr  string  `json:"startAddr" binding:"required"`
	StartLng   float64 `json:"startLng" binding:"required"`
	StartLat   float64 `json:"startLat" binding:"required"`
	EndName    string  `json:"endName" binding:"required"`
	EndAddr    string  `json:"endAddr" binding:"required"`
	EndLng     float64 `json:"endLng" binding:"required"`
	EndLat     float64 `json:"endLat" binding:"required"`
	// Remark 为乘客备注，五十字以内。
	Remark string `json:"remark"`
	// IdempotencyKey 为业务幂等键，相同键重试返回同一订单。
	IdempotencyKey string `json:"idempotencyKey" binding:"required"`
}

// ListOrdersRequest 是 GET /api/v1/passenger/orders 使用的查询参数。
type ListOrdersRequest struct {
	AccountID string `form:"accountId"`
	State     string `form:"state"`
	PageNo    int    `form:"pageNo"`
	PageSize  int    `form:"pageSize"`
}

// PassengerCancelOrderRequest 是 POST /api/v1/passenger/orders/cancel 使用的 JSON 载荷。
//
// 命名带 Passenger 前缀：driver_request.go 里也有一个 CancelOrderRequest
// （司机端取消订单），两者同在 request 包内，不能重名。
type PassengerCancelOrderRequest struct {
	AccountID string `json:"accountId" binding:"required"`
	OrderID   string `json:"orderId" binding:"required"`
	Reason    string `json:"reason" binding:"required"`
	// ConfirmFee 为带消费费的二次确认标记。
	ConfirmFee bool `json:"confirmFee"`
}

// OrderQueryRequest 是按订单标识查询时使用的查询参数。
type OrderQueryRequest struct {
	AccountID string `json:"accountId"`
	OrderID   string `json:"orderId"`
}
