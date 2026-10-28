package dto

// ListMyOrdersCommand 是 P17 乘客订单列表入参（GET /api/pax/orders）。
// Filter 为契约过滤视图（空=全部）；Page/Size 由 service 层归一化（默认每页 20）。
type ListMyOrdersCommand struct {
	PassengerID string
	Filter      string
	Page        int32
	Size        int32
}

// MyOrderListItem 是订单列表项（契约 3.6.1 出参）。
// ID 用 orders.orderNo 业务号（契约 id）；Amount* 为金额口径三字段。
type MyOrderListItem struct {
	ID          string
	TypeName    string
	StartName   string
	EndName     string
	State       string
	StateName   string
	Amount      int64
	AmountLabel string
	AmountBasis string
	BillState   string
	CreatedAt   string
}

// ListMyOrdersResult 是 P17 列表出参 {list, total}。
type ListMyOrdersResult struct {
	List  []*MyOrderListItem
	Total int64
}

// GetMyOrderDetailCommand 是 P18 订单详情入参（GET /api/pax/orders/:order_id）。
// OrderID 用 orders.orderNo 业务号（契约 id）；PassengerID 为归属校验（D4）。
type GetMyOrderDetailCommand struct {
	PassengerID string
	OrderID     string
}

// BillDetail 是账单全量费用明细（T…N + state，契约 3.6.2 bill）。
type BillDetail struct {
	TotalFee              int64
	DriverGrossFee        int64
	PlatformServiceFee    int64
	SurgeFee              int64
	CouponFee             int64
	PlatformCouponBearFee int64
	DriverCouponBearFee   int64
	PayableFee            int64
	DriverIncomeFee       int64
	PlatformNetFee        int64
	State                 string
	Kind                  string
	PaidAt                string
}

// OrderTimelineItem 是时间线元素（出参按 t 升序）。
type OrderTimelineItem struct {
	Name string
	T    string
}

// DriverSnap 是司机快照（只读 orders 快照列 driverSnap.name，不查实时 driver 表）。
type DriverSnap struct {
	Name string
}

// RefundItem 是退款元素（出参按时间序升序）。
type RefundItem struct {
	RefundFee       int64
	DriverRefundFee int64
	Mode            string
	State           string
	T               string
}

// SubmitRatingCommand 是 P19 提交评价入参（POST /api/pax/order/rate）。
// Rating 1–5；Anonymous 仅存标志（仅对司机隐藏身份）。
type SubmitRatingCommand struct {
	PassengerID string
	OrderID     string
	Rating      int32
	Anonymous   bool
}

// SubmitRatingResult 是 P19 出参 {order_id, rating}。
type SubmitRatingResult struct {
	OrderID string
	Rating  int32
}

// SubmitComplaintCommand 是 P19 提交投诉入参（POST /api/pax/order/complaint）。
// Type 为契约中文五类：绕路/服务态度/危险驾驶/费用异议/其他。
// IdempotencyKey 必填，ticketNo 由其派生（同键同号，重试 upsert 同一文档）。
type SubmitComplaintCommand struct {
	PassengerID    string
	OrderID        string
	Type           string
	Desc           string
	Images         []string
	IdempotencyKey string
}

// SubmitComplaintResult 是 P19 出参 {ticket_id, state, state_name}。
type SubmitComplaintResult struct {
	TicketID  string
	State     string
	StateName string
}

// TicketTimelineItem 是工单时间线元素（P22，出参按 seq 升序）。
type TicketTimelineItem struct {
	Seq     int32
	Actor   string
	Action  string
	From    string
	To      string
	Message string
	At      string
}

// MyTicketProgress 是我的工单进度项（P22，契约 3.7.3 my_tickets 元素）。
// TicketID 取工单号（与 P19 出参 ticket_id 同口径）。
type MyTicketProgress struct {
	TicketID  string
	TicketNo  string
	OrderNo   string
	Type      string
	State     string
	StateName string
	Round     int32
	CreatedAt string
	Timeline  []*TicketTimelineItem
}

// ListMyTicketsResult 是 P22 出参 {list, total}。
type ListMyTicketsResult struct {
	List  []*MyTicketProgress
	Total int64
}

// MyOrderDetail 是 P18 订单详情出参（契约 3.6.2）。
type MyOrderDetail struct {
	ID         string
	TypeName   string
	StartName  string
	EndName    string
	State      string
	StateName  string
	DistM      int64
	DurS       int64
	CreatedAt  string
	Bill       *BillDetail
	Timeline   []*OrderTimelineItem
	DriverSnap *DriverSnap
	Refunds    []*RefundItem
}
