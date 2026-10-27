package request

// CreateShopRequest 是 POST /api/v1/shops 使用的 JSON 载荷。
type CreateShopRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateShopRequest 是 PUT /api/v1/shops/:id 使用的 JSON 载荷。
type UpdateShopRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// ListShopRequest 是 GET /api/v1/shops 使用的查询参数载荷。
type ListShopRequest struct {
	Page     int32 `form:"page"`
	PageSize int32 `form:"page_size"`
}

// PayRequest 是 POST /api/v1/shops/pay 使用的 JSON 载荷，金额单位分。
type PayRequest struct {
	OrderNo     string `json:"order_no"`
	Subject     string `json:"subject" binding:"required"`
	TotalAmount int64  `json:"total_amount" binding:"required,gt=0"`
	Body        string `json:"body"`
	Scene       string `json:"scene" binding:"required,oneof=wap page"`
}

// ScanPayRequest 是 POST /api/v1/shops/scan-pay 使用的 JSON 载荷，金额单位分。
// Channel 取值 alipay / wechat，空值默认 alipay。
type ScanPayRequest struct {
	OrderNo     string `json:"order_no"`
	Subject     string `json:"subject" binding:"required"`
	TotalAmount int64  `json:"total_amount" binding:"required,gt=0"`
	Body        string `json:"body"`
	Channel     string `json:"channel"`
}

// BarCodePayRequest 是 POST /api/v1/shops/bar-code-pay 使用的 JSON 载荷，金额单位分。
type BarCodePayRequest struct {
	OrderNo     string `json:"order_no"`
	Subject     string `json:"subject" binding:"required"`
	TotalAmount int64  `json:"total_amount" binding:"required,gt=0"`
	AuthCode    string `json:"auth_code" binding:"required"`
	Body        string `json:"body"`
}

// RefundRequest 是 POST /api/v1/shops/refund 使用的 JSON 载荷，金额单位分。
// Channel 取值 alipay / wechat，空值默认 alipay。
type RefundRequest struct {
	OrderNo      string `json:"order_no"`
	TradeNo      string `json:"trade_no"`
	RefundAmount int64  `json:"refund_amount" binding:"required,gt=0"`
	RefundReason string `json:"refund_reason"`
	OutRequestNo string `json:"out_request_no"`
	Channel      string `json:"channel"`
}
