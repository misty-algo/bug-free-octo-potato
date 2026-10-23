// Package contracts 定义跨模块的事件契约与 interfaces 常量。
//
// 各成员负责的模块之间只通过「事件 + 幂等契约」交互，不直接读对方库表。
// 本包只放常量与载荷结构体，不放业务逻辑，任何模块都可以安全依赖。
//
// 分工与调用方向：
//   - 樊凯军（乘客端下单 P01–P07）：发布 EventOrderCreated，司机端消费后生成抢单任务；
//   - 孙源（支付 P12–P14）：发布 EventPaymentSucceeded / EventRefundSucceeded，司机端据此记账；
//   - 李嘉峰（行程 D05–D08）：发布 EventTripEnded，司机端据此释放运力并结算收益；
//   - 杨轶麟（后台 A10/A11/A12 结算/提现审批/账务）：消费司机端的 EventIncomeRecorded、
//     EventWithdrawalRequested，并回调司机端的结算/提现结果接口；
//   - 陈春博（司机端 D01–D04、D09–D13）：发布上述事件并接收外部命令。
package contracts

// 事件类型常量（对应可靠事件表的 event_type）。
const (
	EventOrderCreated        = "order.created"
	EventOrderCancelled      = "order.cancelled"
	EventTaskAssigned        = "task.assigned"
	EventTaskAccepted        = "task.accepted"
	EventTaskReleased        = "task.released"
	EventTaskExpired         = "task.expired"
	EventTripEnded           = "trip.ended"
	EventPaymentSucceeded    = "payment.succeeded"
	EventRefundSucceeded     = "refund.succeeded"
	EventSettlementConfirmed = "driver.settlement.confirmed"
	EventIncomeRecorded      = "driver.income.recorded"
	EventWithdrawalRequested = "driver.withdrawal.requested"
	EventDriverStatusChanged = "driver.status.changed"
)

// 责任方常量：用于退单/取消是否计入司机退单次数，以及反向流水的承担方。
const (
	PartyPassenger = "passenger"
	PartyDriver    = "driver"
	PartyPlatform  = "platform"
)

// 订单六态（统一状态机，所有模块共用字符串取值）。
const (
	OrderStatusWaiting   = "WAITING"
	OrderStatusAccepted  = "ACCEPTED"
	OrderStatusArrived   = "ARRIVED"
	OrderStatusInTrip    = "IN_TRIP"
	OrderStatusEnded     = "ENDED"
	OrderStatusCancelled = "CANCELLED"
)

// 司机四态（司机端运力状态机）。
const (
	DriverStatusOffline      = "offline"       // 收车
	DriverStatusIdle         = "idle"          // 空闲
	DriverStatusBusy         = "busy"          // 忙碌（行程中）
	DriverStatusPreOccupied  = "pre_occupied"  // 接单预占（已邀请/已抢待确认）
)

// 接单通道常量：单通道分发采用的阶段。
const (
	DispatchChannelListen   = "listen"      // 听单专属窗口
	DispatchChannelGrabPool = "grab_pool"   // 转抢单池
)

// TaskAmount 任务金额拆分（单位：分，整数。1 元 = 100 分）。
// 分账守恒：PassengerPaidFen + PlatformCouponFen = OrigFareFen，
// 司机应得 DriverIncomeFen = PassengerPaidFen - CommissionFen - DriverCouponFen + SubsidyFen。
type TaskAmount struct {
	OrigFareFen       int64 `json:"orig_fare_fen"`
	PassengerPaidFen  int64 `json:"passenger_paid_fen"`
	PlatformCouponFen int64 `json:"platform_coupon_fen"`
	DriverCouponFen   int64 `json:"driver_coupon_fen"`
	CommissionFen     int64 `json:"commission_fen"`
	SubsidyFen        int64 `json:"subsidy_fen"`
	DriverIncomeFen   int64 `json:"driver_income_fen"`
}

// OrderCreatedPayload 樊凯军（乘客端）下单成功后投递的事件。
// 司机端消费后生成抢单任务；缺省字段由司机端给出课程演示默认值。
type OrderCreatedPayload struct {
	EventID       string  `json:"event_id"`
	OrderID       string  `json:"order_id"`
	TaskID        string  `json:"task_id"`
	CityCode      string  `json:"city_code"`
	OrderType     string  `json:"order_type"`
	PassengerName string  `json:"passenger_name"`
	Pickup        LatLng  `json:"pickup"`
	Dropoff       LatLng  `json:"dropoff"`
	RequireSeats  int     `json:"require_seats"`
	RequireCarType string  `json:"require_car_type"`
	PassengerCount int    `json:"passenger_count"`
	Amount        TaskAmount `json:"amount"`
	CreatedAt     string  `json:"created_at"`
}

// LatLng GCJ02 坐标。
type LatLng struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// OrderCancelledPayload 乘客端取消订单事件（不计司机退单次数）。
type OrderCancelledPayload struct {
	EventID         string `json:"event_id"`
	OrderID         string `json:"order_id"`
	Reason          string `json:"reason"`
	ResponsibleParty string `json:"responsible_party"`
	CancelledAt     string `json:"cancelled_at"`
}

// TaskAssignedPayload 司机端成功邀请/抢占司机后发布，供行程域与乘客端感知。
type TaskAssignedPayload struct {
	EventID        string `json:"event_id"`
	TaskID         string `json:"task_id"`
	OrderID        string `json:"order_id"`
	DriverID       string `json:"driver_id"`
	Channel        string `json:"channel"`
	InviteRound    int64  `json:"invite_round"`
	AcceptDeadline string `json:"accept_deadline"`
	OccurredAt     string `json:"occurred_at"`
}

// TaskAcceptedPayload 司机确认接单后发布（此时才真正占用运力，行程域可开始接驾）。
type TaskAcceptedPayload struct {
	EventID    string `json:"event_id"`
	TaskID     string `json:"task_id"`
	OrderID    string `json:"order_id"`
	DriverID   string `json:"driver_id"`
	DriverName string `json:"driver_name"`
	AcceptedAt string `json:"accepted_at"`
}

// TaskReleasedPayload 退单/改派后发布，行程域据此撤销对应权限与联系能力。
type TaskReleasedPayload struct {
	EventID          string `json:"event_id"`
	TaskID           string `json:"task_id"`
	OrderID          string `json:"order_id"`
	DriverID         string `json:"driver_id"`
	Reason           string `json:"reason"`
	ResponsibleParty string `json:"responsible_party"`
	ReleasedAt       string `json:"released_at"`
}

// TripEndedPayload 李嘉峰（行程域）送达后发布，司机端据此释放运力并生成收益凭证草稿。
type TripEndedPayload struct {
	EventID       string     `json:"event_id"`
	OrderID       string     `json:"order_id"`
	TaskID        string     `json:"task_id"`
	DriverID      string     `json:"driver_id"`
	EndedAt       string     `json:"ended_at"`
	DurationSec   int64      `json:"duration_sec"`
	DistanceMeters int64     `json:"distance_meters"`
	Amount        TaskAmount `json:"amount"`
}

// PaymentSucceededPayload 孙源（支付域）支付成功后发布，司机端据此记账。
type PaymentSucceededPayload struct {
	EventID          string     `json:"event_id"`
	OrderID          string     `json:"order_id"`
	DriverID         string     `json:"driver_id"`
	PaidAt           string     `json:"paid_at"`
	AccountingDate   string     `json:"accounting_date"`
	Amount           TaskAmount `json:"amount"`
	IdempotencyKey   string     `json:"idempotency_key"`
}

// RefundSucceededPayload 退款成功后发布，司机端生成反向流水（不修改原始凭证）。
type RefundSucceededPayload struct {
	EventID          string `json:"event_id"`
	OrderID          string `json:"order_id"`
	DriverID         string `json:"driver_id"`
	RefundFen        int64  `json:"refund_fen"`
	ResponsibleParty string `json:"responsible_party"`
	AccountingDate   string `json:"accounting_date"`
	RefundedAt       string `json:"refunded_at"`
}

// IncomeRecordedPayload 司机端记账成功后发布，供后台财务（杨轶麟 A10/A12）对账。
type IncomeRecordedPayload struct {
	EventID      string `json:"event_id"`
	FlowID       string `json:"flow_id"`
	DriverID     string `json:"driver_id"`
	OrderID      string `json:"order_id"`
	AccountingDate string `json:"accounting_date"`
	DriverIncomeFen int64 `json:"driver_income_fen"`
	BucketTo     string `json:"bucket_to"`
	OccurredAt   string `json:"occurred_at"`
}

// WithdrawalRequestedPayload 司机申请提现后发布，进入后台审核队列（A11）。
type WithdrawalRequestedPayload struct {
	EventID         string `json:"event_id"`
	WithdrawalID    string `json:"withdrawal_id"`
	DriverID        string `json:"driver_id"`
	AmountFen       int64  `json:"amount_fen"`
	DualApproval    bool   `json:"dual_approval_required"`
	IdempotencyKey  string `json:"idempotency_key"`
	RequestedAt     string `json:"requested_at"`
}

// DriverStatusChangedPayload 司机出收车/状态变化后发布，后台据此处置禁接单策略。
type DriverStatusChangedPayload struct {
	EventID    string `json:"event_id"`
	DriverID   string `json:"driver_id"`
	FromStatus string `json:"from_status"`
	ToStatus   string `json:"to_status"`
	Reason     string `json:"reason"`
	OccurredAt string `json:"occurred_at"`
}
