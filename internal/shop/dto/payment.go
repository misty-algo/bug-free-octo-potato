package dto

// PayCommand 包含发起 WAP/PC 网页支付的入参，金额 int64 分。
type PayCommand struct {
	OrderNo     string
	Subject     string
	TotalAmount int64
	Body        string
	Scene       string
}

// ScanPayCommand 包含发起扫码支付的入参，金额 int64 分。
// Channel 取值 alipay / wechat，空值默认 alipay。
type ScanPayCommand struct {
	OrderNo     string
	Subject     string
	TotalAmount int64
	Body        string
	Channel     string
}

// BarCodePayCommand 包含发起付款码支付的入参，金额 int64 分。
type BarCodePayCommand struct {
	OrderNo     string
	Subject     string
	TotalAmount int64
	AuthCode    string
	Body        string
}

// RefundCommand 包含发起退款的入参，退款金额 int64 分。
// Channel 取值 alipay / wechat，空值默认 alipay。
type RefundCommand struct {
	OrderNo      string
	TradeNo      string
	RefundAmount int64
	RefundReason string
	OutRequestNo string
	Channel      string
}

// NotifyCommand 透传支付渠道异步通知原文。
// alipay 回调为 x-www-form-urlencoded 表单字符串（Form 字段）；
// wechat 回调为 JSON 原文（Payload 字段）并携带 Wechatpay-* 验签请求头。
type NotifyCommand struct {
	Channel string
	Form    string
	Payload string
	Headers map[string]string
}

// PaymentQueryResult 描述本地支付单的当前状态。
type PaymentQueryResult struct {
	OrderNo     string
	Status      string
	Channel     string
	TradeNo     string
	TotalAmount int64
	PaidAtUnix  int64
}
