package request

// 司机端 HTTP 请求体。
// 约定：所有金额字段单位为「分」（int64），1 元 = 100 分；
// 坐标统一 GCJ02；距离单位为米。

// RegisterDriverRequest 注册司机请求体。
type RegisterDriverRequest struct {
	Name  string `json:"name" binding:"required"`
	Phone string `json:"phone"`
}

// GetDriverProfileRequest 司机档案查询请求体。
type GetDriverProfileRequest struct {
	IsAdmin bool `json:"is_admin"`
}

// UpdateDriverStatusRequest 收车/出车请求体（offline=收车，idle=出车）。
type UpdateDriverStatusRequest struct {
	Status  string `json:"status" binding:"required,oneof=offline idle"`
	Version int64  `json:"version"`
}

// UpdateDriverModeRequest 听单/抢单切换请求体。
type UpdateDriverModeRequest struct {
	Mode    string `json:"mode" binding:"required,oneof=listen grab"`
	Version int64  `json:"version"`
}

// UpdateOfflineAfterTripRequest 本单后收车请求体。
type UpdateOfflineAfterTripRequest struct {
	On      bool  `json:"on"`
	Version int64 `json:"version"`
}

// HeartbeatRequest 心跳上报请求体。
type HeartbeatRequest struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// AreaBoxItem 接单片区（大片区的包围盒近似）。
type AreaBoxItem struct {
	Name   string  `json:"name" binding:"required"`
	MinLat float64 `json:"min_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLat float64 `json:"max_lat"`
	MaxLng float64 `json:"max_lng"`
}

// UpdateAreaConfigRequest 接单区域偏好请求体。
type UpdateAreaConfigRequest struct {
	AreaLimitSwitch bool          `json:"area_limit_switch"`
	Areas           []AreaBoxItem `json:"areas" binding:"omitempty,dive"`
	Version         int64         `json:"version"`
}

// LocationItem GCJ02 地点。
type LocationItem struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// CreateHallOrderRequest 创建派单任务请求体（金额单位：分）。
type CreateHallOrderRequest struct {
	PassengerName     string       `json:"passenger_name" binding:"required"`
	Origin            LocationItem `json:"origin" binding:"required"`
	Destination       LocationItem `json:"destination" binding:"required"`
	PassengerPaidFen  int64        `json:"passenger_paid_fen" binding:"required,gt=0"`
	OrigFareFen       int64        `json:"orig_fare_fen"`
	PlatformCouponFen int64        `json:"platform_coupon_fen"`
	DriverCouponFen   int64        `json:"driver_coupon_fen"`
	SubsidyFen        int64        `json:"subsidy_fen"`
	PassengerCount    int          `json:"passenger_count"`
	CityCode          string       `json:"city_code"`
	OrderType         string       `json:"order_type"`
	RequireSeats      int          `json:"require_seats"`
	RequireCarType    string       `json:"require_car_type"`
	OrderID           string       `json:"order_id"`
}

// ListHallOrdersRequest 大厅列表查询参数。
type ListHallOrdersRequest struct {
	DriverID string  `form:"driver_id" binding:"required"`
	Limit    int32   `form:"limit"`
	Lat      float64 `form:"lat"`
	Lng      float64 `form:"lng"`
}

// HallOrderActionRequest 抢单/确认接单/退单/开始行程/完成行程的请求体。
type HallOrderActionRequest struct {
	DriverID         string `json:"driver_id" binding:"required"`
	OrderID          string `json:"order_id" binding:"required"`
	Version          int64  `json:"version"`
	IdempotencyKey   string `json:"idempotency_key"`
	Reason           string `json:"reason"`
	ResponsibleParty string `json:"responsible_party"`
	DistanceMeters   int64  `json:"distance_meters"`
	DurationSec      int64  `json:"duration_sec"`
}

// CancelOrderRequest 乘客取消/平台责任取消请求体。
type CancelOrderRequest struct {
	OrderID          string `json:"order_id" binding:"required"`
	ResponsibleParty string `json:"responsible_party"`
	Reason           string `json:"reason"`
}

// IncomeQueryRequest 收入相关查询参数。
type IncomeQueryRequest struct {
	DriverID string `form:"driver_id" binding:"required"`
	Page     int32  `form:"page"`
	PageSize int32  `form:"page_size"`
}

// RequestWithdrawRequest 提现申请请求体（金额单位：分）。
type RequestWithdrawRequest struct {
	DriverID       string `json:"driver_id" binding:"required"`
	AmountFen      int64  `json:"amount_fen" binding:"required,gt=0"`
	IdempotencyKey string `json:"idempotency_key"`
}

// AmountItem 金额拆分项（单位：分）。
type AmountItem struct {
	OrigFareFen       int64 `json:"orig_fare_fen"`
	PassengerPaidFen  int64 `json:"passenger_paid_fen"`
	PlatformCouponFen int64 `json:"platform_coupon_fen"`
	DriverCouponFen   int64 `json:"driver_coupon_fen"`
	CommissionFen     int64 `json:"commission_fen"`
	SubsidyFen        int64 `json:"subsidy_fen"`
	DriverIncomeFen   int64 `json:"driver_income_fen"`
}

// ReportPaymentRequest 支付域回调请求体。
type ReportPaymentRequest struct {
	OrderID        string     `json:"order_id" binding:"required"`
	DriverID       string     `json:"driver_id" binding:"required"`
	AccountingDate string     `json:"accounting_date"`
	EventID        string     `json:"event_id"`
	IdempotencyKey string     `json:"idempotency_key"`
	Amount         AmountItem `json:"amount" binding:"required"`
}

// ReportRefundRequest 退款回调请求体。
type ReportRefundRequest struct {
	OrderID          string `json:"order_id" binding:"required"`
	RefundFen        int64  `json:"refund_fen" binding:"required,gt=0"`
	ResponsibleParty string `json:"responsible_party"`
	AccountingDate   string `json:"accounting_date"`
	Reason           string `json:"reason"`
	EventID          string `json:"event_id"`
	IdempotencyKey   string `json:"idempotency_key"`
}

// SettleIncomeRequest 财务域结算确认请求体。
type SettleIncomeRequest struct {
	DriverID string   `json:"driver_id"`
	FlowIDs  []string `json:"flow_ids"`
	EventID  string   `json:"event_id"`
}

// ApproveWithdrawRequest 提现审批请求体。
type ApproveWithdrawRequest struct {
	WithdrawalID string `json:"withdrawal_id" binding:"required"`
	ApproverID   string `json:"approver_id" binding:"required"`
	Comment      string `json:"comment"`
}

// RejectWithdrawRequest 提现驳回请求体。
type RejectWithdrawRequest struct {
	WithdrawalID string `json:"withdrawal_id" binding:"required"`
	ApproverID   string `json:"approver_id"`
	Reason       string `json:"reason"`
}

// CompleteWithdrawRequest 打款结果回调请求体。
type CompleteWithdrawRequest struct {
	WithdrawalID string `json:"withdrawal_id" binding:"required"`
	Success      bool   `json:"success"`
	Reason       string `json:"reason"`
}
