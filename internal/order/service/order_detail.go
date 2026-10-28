package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// GetMyOrderDetail 是 P18 订单详情（GET /api/pax/orders/:order_id，契约 3.6.2）。
// 只读 orders 快照列（司机/车型/路线/计价），不查实时 driver 表；
// bill 出 T…N 全量 + state；timeline 按 t 升序；refunds 按时间序；不伪造状态回退（读什么出什么）。
// 他人订单与不存在订单一律 ErrOrderNotFound（归属校验 D4，不泄露存在性）。
func (s *Service) GetMyOrderDetail(ctx context.Context, cmd dto.GetMyOrderDetailCommand) (*dto.MyOrderDetail, error) {
	if cmd.PassengerID == "" || cmd.OrderID == "" {
		return nil, entity.ErrInvalidOrder
	}
	queryRepo, ok := s.repo.(entity.OrderQueryRepository)
	if !ok {
		return nil, errors.New("order query repository not available")
	}
	order, err := queryRepo.FindByOrderNo(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.PassengerID != cmd.PassengerID {
		return nil, entity.ErrOrderNotFound
	}

	names := s.resolvePOINames(ctx, []*entity.Order{order})
	detail := &dto.MyOrderDetail{
		ID:         listOrderID(order),
		TypeName:   order.TypeName,
		StartName:  names[order.StartID],
		EndName:    names[order.EndID],
		State:      order.State,
		StateName:  stateName(order.State),
		DistM:      order.DistM,
		DurS:       order.DurS,
		CreatedAt:  formatOrderTime(order.CreatedAt),
		Timeline:   timelineItems(order.Timeline),
		DriverSnap: driverSnapItem(order),
		Refunds:    refundItems(order.Refunds),
	}
	if bill := order.PrimaryBill(); bill != nil {
		detail.Bill = billDetailItem(bill)
	}
	return detail, nil
}

// driverSnapItem 输出司机快照（orders 快照列 driverSnap.name）；无快照时为 nil，不伪造。
func driverSnapItem(order *entity.Order) *dto.DriverSnap {
	if order.DriverName == "" {
		return nil
	}
	return &dto.DriverSnap{Name: order.DriverName}
}

// billDetailItem 输出账单 T…N 全量 + state/kind/paid_at（快照原样，不重算）。
func billDetailItem(bill *entity.Bill) *dto.BillDetail {
	paidAt := ""
	if bill.PaidAt != nil {
		paidAt = formatOrderTime(*bill.PaidAt)
	}
	return &dto.BillDetail{
		TotalFee:              bill.TotalFee,
		DriverGrossFee:        bill.DriverGrossFee,
		PlatformServiceFee:    bill.PlatformServiceFee,
		SurgeFee:              bill.SurgeFee,
		CouponFee:             bill.CouponFee,
		PlatformCouponBearFee: bill.PlatformCouponBearFee,
		DriverCouponBearFee:   bill.DriverCouponBearFee,
		PayableFee:            bill.PayableFee,
		DriverIncomeFee:       bill.DriverIncomeFee,
		PlatformNetFee:        bill.PlatformNetFee,
		State:                 bill.State,
		Kind:                  bill.Kind,
		PaidAt:                paidAt,
	}
}

// timelineItems 输出时间线，按 t 升序（不改变聚合内原始顺序）。
func timelineItems(items []entity.TimelineItem) []*dto.OrderTimelineItem {
	sorted := make([]entity.TimelineItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].T.Before(sorted[j].T) })
	out := make([]*dto.OrderTimelineItem, 0, len(sorted))
	for _, item := range sorted {
		out = append(out, &dto.OrderTimelineItem{Name: item.Name, T: formatOrderTime(item.T)})
	}
	return out
}

// refundItems 输出退款记录，按时间序升序。
func refundItems(items []entity.Refund) []*dto.RefundItem {
	sorted := make([]entity.Refund, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].T.Before(sorted[j].T) })
	out := make([]*dto.RefundItem, 0, len(sorted))
	for _, item := range sorted {
		out = append(out, &dto.RefundItem{
			RefundFee:       item.RefundFee,
			DriverRefundFee: item.DriverRefundFee,
			Mode:            item.Mode,
			State:           item.State,
			T:               formatOrderTime(item.T),
		})
	}
	return out
}

// formatOrderTime 时间出参统一 RFC3339；零值出空串（不伪造时间）。
func formatOrderTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
