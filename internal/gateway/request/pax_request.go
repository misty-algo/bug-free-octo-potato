package request

// ListPaxOrderRequest 是 GET /api/pax/orders 使用的查询参数载荷（契约 3.6.1）。
// filter 取值：全部/进行中/待支付/已完成/已取消（可省略，省略等价全部）；每页默认 20。
type ListPaxOrderRequest struct {
	Filter string `form:"filter"`
	Page   int32  `form:"page"`
	Size   int32  `form:"size"`
}

// RatePaxOrderRequest 是 POST /api/pax/order/rate 使用的请求体（契约 3.6.3）。
// rating 1–5；anonymous 可省略（仅对司机隐藏身份）。
type RatePaxOrderRequest struct {
	OrderID   string `json:"order_id"`
	Rating    int32  `json:"rating"`
	Anonymous bool   `json:"anonymous"`
}

// ComplaintPaxOrderRequest 是 POST /api/pax/order/complaint 使用的请求体（契约 3.6.4）。
// type 为绕路/服务态度/危险驾驶/费用异议/其他；images ≤6 张；idempotency_key 必填。
type ComplaintPaxOrderRequest struct {
	OrderID        string   `json:"order_id"`
	Type           string   `json:"type"`
	Desc           string   `json:"desc"`
	Images         []string `json:"images"`
	IdempotencyKey string   `json:"idempotency_key"`
}

// RealNamePaxRequest 是 POST /api/pax/realname 使用的请求体（契约 3.7.1）。
// idempotency_key 必填；响应只回脱敏姓名/证件号。
type RealNamePaxRequest struct {
	Name           string `json:"name"`
	IDNo           string `json:"id_no"`
	IdempotencyKey string `json:"idempotency_key"`
}

// AccountSecurityPaxRequest 是 POST /api/pax/account-security 使用的请求体（契约 3.7.2）。
// 记录协议同意版本；同版本幂等不重复记。
type AccountSecurityPaxRequest struct {
	ProtocolVer string `json:"protocol_ver"`
}
