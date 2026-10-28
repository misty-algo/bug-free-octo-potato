package entity

import (
	"strings"
	"time"
)

// Ticket states（线上 tickets.state 枚举）。
const (
	TicketPending    = "PENDING"
	TicketProcessing = "PROCESSING"
	TicketResolved   = "RESOLVED"
)

// Ticket types（契约 2.3 投诉五类）。
const (
	TicketTypeDriverService    = "DRIVER_SERVICE"
	TicketTypeRouteDetour      = "ROUTE_DETOUR"
	TicketTypePriceDispute     = "PRICE_DISPUTE"
	TicketTypeDangerousDriving = "DANGEROUS_DRIVING"
	TicketTypeLostItem         = "LOST_ITEM" // 线上容错值
	TicketTypeOther            = "OTHER"     // 契约 3.6.4「其他」
)

// Ticket 投诉工单实体（P19）。
type Ticket struct {
	ID         string
	TicketNo   string
	OrderID    string
	OrderNo    string
	ComplainID string
	DriverID   string
	Type       string
	Content    string
	Evidences  []string
	State      string
	Round      int32 // 处理轮次（线上 tickets.round，工单进度展示）
	Timeline   []TicketEvent
	CreatedAt  time.Time
	DoneAt     *time.Time
}

// SafetyEventReport 危险驾驶投诉派生的安全事件（P19，写 safety_events）。
// 处置流转归司机端安全域（D17，state=RECEIVED 入值班队列），本域只发起。
type SafetyEventReport struct {
	EventID         string // 与事件日志共用，按工单号派生去重
	TicketNo        string
	OrderNo         string
	DriverID        string // ObjectID hex，未知为空
	Description     string
	ClientRequestID string // 幂等键
	ReceivedAt      time.Time
}

// TicketEvent 工单时间线事件（出参 timeline 元素）。
type TicketEvent struct {
	Seq     int32
	Actor   string
	Action  string
	From    string
	To      string
	Message string
	At      time.Time
}

// NewTicket 创建投诉工单（state=PENDING，初始时间线一条 create 事件）。
func NewTicket(id, ticketNo, orderID, orderNo, complainID, driverID, typ, content string, evidences []string, now time.Time) (*Ticket, error) {
	if !ValidTicketType(typ) {
		return nil, newTicketInvalidType(typ)
	}
	if content == "" {
		return nil, newTicketEmptyContent()
	}
	if len(evidences) > 6 {
		return nil, newTicketTooManyEvidences()
	}
	t := &Ticket{
		ID:         id,
		TicketNo:   ticketNo,
		OrderID:    orderID,
		OrderNo:    orderNo,
		ComplainID: complainID,
		DriverID:   driverID,
		Type:       typ,
		Content:    content,
		Evidences:  evidences,
		State:      TicketPending,
		CreatedAt:  now,
	}
	t.Timeline = []TicketEvent{{
		Seq: 1, Actor: "pax", Action: "create", To: TicketPending, Message: content, At: now,
	}}
	return t, nil
}

// ValidTicketType 判断投诉类型是否合法（契约五类 + 线上容错值，大小写不敏感）。
func ValidTicketType(typ string) bool {
	switch strings.ToUpper(typ) {
	case TicketTypeDriverService, TicketTypeRouteDetour, TicketTypePriceDispute,
		TicketTypeDangerousDriving, TicketTypeLostItem, TicketTypeOther:
		return true
	}
	return false
}

// ValidTicketState 判断工单状态是否合法（线上三态）。
func ValidTicketState(state string) bool {
	switch state {
	case TicketPending, TicketProcessing, TicketResolved:
		return true
	}
	return false
}

// TicketStateName 状态 → 契约中文（附录 E）。
func TicketStateName(state string) string {
	switch state {
	case TicketPending:
		return "待处理"
	case TicketProcessing:
		return "处理中"
	case TicketResolved:
		return "已完结"
	}
	return ""
}

// 小错误构造（避免在 entity 引入 apperrors 依赖）。
type ticketError struct{ msg string }

func (e *ticketError) Error() string { return e.msg }

func newTicketInvalidType(typ string) error { return &ticketError{"invalid ticket type: " + typ} }
func newTicketEmptyContent() error          { return &ticketError{"ticket content is empty"} }
func newTicketTooManyEvidences() error      { return &ticketError{"too many evidences (max 6)"} }
