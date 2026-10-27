package request

// CreatePaymentRequest 是 POST /api/v1/payments 使用的 JSON 载荷，金额单位分。
type CreatePaymentRequest struct {
	OrderNo       string `json:"order_no" binding:"required"`
	Subject       string `json:"subject"`
	Amount        int64  `json:"amount" binding:"required,gt=0"`
	Channel       string `json:"channel"`
	UserID        string `json:"user_id"`
	IdempotentKey string `json:"idempotent_key"`
}

// NotifyPaymentRequest 是 POST /api/v1/payments/notify 使用的 JSON 载荷，金额单位分。
type NotifyPaymentRequest struct {
	OrderNo    string `json:"order_no" binding:"required"`
	TradeNo    string `json:"trade_no"`
	Channel    string `json:"channel"`
	PaidAmount int64  `json:"paid_amount" binding:"required,gt=0"`
	PaidAtUnix int64  `json:"paid_at_unix"`
}

// QueryPaymentRequest 是 GET /api/v1/payments/:order_no 使用的查询参数载荷。
type QueryPaymentRequest struct {
	OrderNo string `json:"order_no"`
}

// RefundPaymentRequest 是 POST /api/v1/payments/refund 使用的 JSON 载荷，金额单位分。
type RefundPaymentRequest struct {
	OrderNo      string `json:"order_no" binding:"required"`
	RefundAmount int64  `json:"refund_amount" binding:"required,gt=0"`
	Reason       string `json:"reason"`
	OutRequestNo string `json:"out_request_no"`
}
