package entity

import "strings"

// 契约 3.6.1 订单列表过滤视图（filter 参数值）。
const (
	OrderViewAll       = "全部"
	OrderViewActive    = "进行中"
	OrderViewUnpaid    = "待支付"
	OrderViewDone      = "已完成"
	OrderViewCancelled = "已取消"
)

// 账单 kind 取值（TODO(孙源)：账单流程落地后按线上实际写入值校准，见计划附录 B/D）。
const (
	BillKindTrip      = "TRIP"       // 行程主账单
	BillKindCancelFee = "CANCEL_FEE" // 取消费账单
)

// 账单状态四态（TODO(孙源)：账单流程落地后校准）。
const (
	BillStateCalculating = "CALCULATING"
	BillStateUnpaid      = "UNPAID"
	BillStatePaid        = "PAID"
	BillStateVoid        = "VOID"
)

// NormalizeOrderView 归一化契约 filter（空串视同「全部」），非法值报 ErrInvalidOrder。
func NormalizeOrderView(view string) (string, error) {
	view = strings.TrimSpace(view)
	if view == "" {
		return OrderViewAll, nil
	}
	switch view {
	case OrderViewAll, OrderViewActive, OrderViewUnpaid, OrderViewDone, OrderViewCancelled:
		return view, nil
	default:
		return "", ErrInvalidOrder
	}
}

// IsOrderActive 判断订单是否处于「进行中」（PENDING/ACCEPTED/IN_SERVICE/IN_TRIP）。
func IsOrderActive(state string) bool {
	switch state {
	case OrderPending, OrderAccepted, OrderInService, OrderInTrip:
		return true
	default:
		return false
	}
}

// MatchesFilter 判断订单是否命中列表过滤（计划附录 E 口径，与 repo.buildOrderFilter 必须保持一致）：
//
//	「进行中」=PENDING/ACCEPTED/IN_SERVICE/IN_TRIP；
//	「待支付」=UNPAID 或（CANCELLED 且取消费未付）；
//	「已完成」=ENDED/COMPLETED；
//	「已取消」=CANCELLED/NO_DRIVER。
func MatchesFilter(o *Order, filter *OrderFilter) bool {
	if o == nil || filter == nil {
		return false
	}
	if filter.PassengerID != "" && o.PassengerID != filter.PassengerID {
		return false
	}
	switch filter.View {
	case "", OrderViewAll:
		return true
	case OrderViewActive:
		return IsOrderActive(o.State)
	case OrderViewUnpaid:
		return o.State == OrderUnpaid || (o.State == OrderCancelled && hasUnpaidCancelFee(o))
	case OrderViewDone:
		return o.State == OrderEnded || o.State == OrderCompleted
	case OrderViewCancelled:
		return o.State == OrderCancelled || o.State == OrderNoDriver
	default:
		return false
	}
}

// hasUnpaidCancelFee 判断订单是否存在未付的取消费账单。
func hasUnpaidCancelFee(o *Order) bool {
	for _, bill := range o.Bills {
		if bill.Kind == BillKindCancelFee && bill.State == BillStateUnpaid {
			return true
		}
	}
	return false
}

// tripBill 返回行程主账单（非取消费账单），无则返回 nil。
func (o *Order) tripBill() *Bill {
	for i := range o.Bills {
		if o.Bills[i].Kind != BillKindCancelFee {
			return &o.Bills[i]
		}
	}
	return nil
}

// cancelFeeBill 返回取消费账单，无则返回 nil。
func (o *Order) cancelFeeBill() *Bill {
	for i := range o.Bills {
		if o.Bills[i].Kind == BillKindCancelFee {
			return &o.Bills[i]
		}
	}
	return nil
}

// PrimaryBill 返回详情主账单（P18 bill 出参）：优先行程账单，其次取消费账单，均无返回 nil。
func (o *Order) PrimaryBill() *Bill {
	if bill := o.tripBill(); bill != nil {
		return bill
	}
	return o.cancelFeeBill()
}
