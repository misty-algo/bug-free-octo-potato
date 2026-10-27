package dto

// NotifyPaymentResult 通知处理结果。
type NotifyPaymentResult struct {
	Success   bool
	PaymentNo string
	Status    string
	Duplicate bool // 是否重复通知（幂等命中）
}

// RefundResult 退款结果。
type RefundResult struct {
	Success      bool
	RefundNo     string
	PaymentNo    string
	OrderNo      string
	RefundAmount int64
	Status       string
	Duplicate    bool // 是否命中退款幂等（out_request_no 已存在）
}
