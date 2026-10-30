package entity

import (
	"errors"
	"time"
)

// OrderState 是订单状态。状态枚举全大写，且流转关系固定，
// MUST NOT 增删状态或自造流转。
type OrderState string

// 订单状态常量。
const (
	OrderStatePending         OrderState = "PENDING"
	OrderStateAccepted        OrderState = "ACCEPTED"
	OrderStateArrived         OrderState = "ARRIVED"
	OrderStateInService       OrderState = "IN_SERVICE"
	OrderStateCompletedUnpaid OrderState = "COMPLETED_UNPAID"
	OrderStateUnpaid          OrderState = "UNPAID"
	OrderStateCompleted       OrderState = "COMPLETED"
	OrderStateNoDriver        OrderState = "NO_DRIVER"
	OrderStateCancelled       OrderState = "CANCELLED"
	OrderStateSuspended       OrderState = "SUSPENDED"
)

// 状态流转错误。
var (
	ErrInvalidStateTransition = errors.New("invalid order state transition")
	ErrTerminalState          = errors.New("order already in terminal state")
)

// 订单终态集合。终态订单不允许再流转。
var orderTerminalStates = map[OrderState]bool{
	OrderStateCompleted: true,
	OrderStateCancelled: true,
}

// orderTransitions 定义允许的流转。键为当前状态，值为可流转到的状态集合。
var orderTransitions = map[OrderState]map[OrderState]bool{
	OrderStatePending: {
		OrderStateAccepted:  true,
		OrderStateNoDriver:  true,
		OrderStateCancelled: true,
		OrderStateSuspended: true,
	},
	OrderStateNoDriver: {
		OrderStatePending:   true,
		OrderStateCancelled: true,
	},
	OrderStateAccepted: {
		OrderStateArrived:   true,
		OrderStateCancelled: true,
		OrderStateSuspended: true,
	},
	OrderStateArrived: {
		OrderStateInService:       true,
		OrderStateCompletedUnpaid: true,
		OrderStateCancelled:       true,
		OrderStateSuspended:       true,
	},
	OrderStateInService: {
		OrderStateCompletedUnpaid: true,
		OrderStateSuspended:       true,
	},
	// 行程结束即待支付；支付成功直接完成。UNPAID 保留给"支付中"中间态语义。
	OrderStateCompletedUnpaid: {
		OrderStateUnpaid:    true,
		OrderStateCompleted: true,
		OrderStateCancelled: true,
	},
	OrderStateUnpaid: {
		OrderStateCompleted: true,
	},
}

// Valid 报告状态是否是已知状态。
func (s OrderState) Valid() bool {
	_, ok := orderTransitions[s]
	if ok {
		return true
	}
	return orderTerminalStates[s]
}

// Terminal 报告状态是否为终态。
func (s OrderState) Terminal() bool {
	return orderTerminalStates[s]
}

// CanTransitionTo 报告能否从当前状态流转到目标状态。
func (s OrderState) CanTransitionTo(next OrderState) bool {
	if s.Terminal() {
		return false
	}
	allowed, ok := orderTransitions[s]
	if !ok {
		return false
	}
	return allowed[next]
}

// Order 是订单聚合。金额一律以分为单位的整数，全链路禁止浮点。
type Order struct {
	ID         string
	OrderNo    string
	AccountID  string
	State      OrderState
	CarType    int
	StartName  string
	StartAddr  string
	StartLng   float64
	StartLat   float64
	EndName    string
	EndAddr    string
	EndLng     float64
	EndLat     float64
	// EstimateNo 为来源估价单号，同时承担防重复下单职责。
	EstimateNo string
	// TotalFee 为应付总额，单位分。
	TotalFee int64
	// CouponFee 为优惠券抵扣，单位分。
	CouponFee int64
	// CancelFee 为取消费，单位分。
	CancelFee int64
	Remark    string
	// PricingSnapshot 冻结下单时命中的计价规则，保证历史订单金额可复算。
	PricingSnapshot string
	// IdempotencyKey 保证重复提交不产生第二单。
	IdempotencyKey string
	// DriverName / DriverPlate 司机接单后由派单任务回传，仅用于乘客端展示。
	DriverName  string
	DriverPlate string
	// Version 为乐观锁版本号，并发写集合必带。
	Version int64
	// CreatedAt / UpdatedAt 统一 UTC 存储。
	CreatedAt time.Time
	UpdatedAt time.Time
}

// NewOrder 校验并创建订单。
func NewOrder(id, orderNo, accountID string, now time.Time) (*Order, error) {
	if accountID == "" || orderNo == "" {
		return nil, ErrInvalidStateTransition
	}
	now = now.UTC()
	return &Order{
		ID:        id,
		OrderNo:   orderNo,
		AccountID: accountID,
		State:     OrderStatePending,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// CanTransition 报告订单能否流转到目标状态。
func (o *Order) CanTransition(next OrderState) bool {
	return o != nil && o.State.CanTransitionTo(next)
}

// ApplyTransition 校验并执行状态流转。非法流转直接拒绝，不做静默兼容。
//
// 这里不自增 Version：版本号由仓储层的乐观锁更新统一自增，
// 两处都自增会让过滤条件与实际版本错位，导致更新永远匹配不上。
func (o *Order) ApplyTransition(next OrderState, now time.Time) error {
	if o.State.Terminal() {
		return ErrTerminalState
	}
	if !o.State.CanTransitionTo(next) {
		return ErrInvalidStateTransition
	}
	o.State = next
	o.UpdatedAt = now.UTC()
	return nil
}

// OrderStatusLog 是订单状态留痕。每次状态变更写一条，保证任意一单可完整回放。
type OrderStatusLog struct {
	ID        string
	OrderID   string
	From      OrderState
	To        OrderState
	Operator  string
	Reason    string
	CreatedAt time.Time
}

// NewOrderStatusLog 创建一条状态留痕。
func NewOrderStatusLog(id, orderID string, from, to OrderState, operator, reason string, now time.Time) *OrderStatusLog {
	return &OrderStatusLog{
		ID:        id,
		OrderID:   orderID,
		From:      from,
		To:        to,
		Operator:  operator,
		Reason:    reason,
		CreatedAt: now.UTC(),
	}
}
