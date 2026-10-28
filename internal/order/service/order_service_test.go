package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// Task 4 失败测试（docs/plans/2026-10-27-pax-orders-my.md）：
// P17 订单列表 —— 过滤口径（附录 E）与金额口径（附录 B）。

type fakePOIResolver struct {
	names map[string]string
}

func (f *fakePOIResolver) ResolveNames(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	for _, id := range ids {
		if name, ok := f.names[id]; ok {
			out[id] = name
		}
	}
	return out, nil
}

// ListFiltered 供 P17 列表测试使用：口径统一走 entity.MatchesFilter，
// created_at 倒序 + offset/limit 分页，total 为命中总数。
func (r *fakeRepository) ListFiltered(ctx context.Context, filter *entity.OrderFilter) ([]*entity.Order, int64, error) {
	matched := make([]*entity.Order, 0, len(r.items))
	for _, item := range r.items {
		if entity.MatchesFilter(item, filter) {
			copy := *item
			matched = append(matched, &copy)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.After(matched[j].CreatedAt) })
	total := int64(len(matched))
	if filter.Offset >= len(matched) {
		return nil, total, nil
	}
	end := filter.Offset + filter.Limit
	if end > len(matched) {
		end = len(matched)
	}
	return matched[filter.Offset:end], total, nil
}

var _ entity.OrderQueryRepository = (*fakeRepository)(nil)

func seedOrder(repo *fakeRepository, no, passengerID, state string, at time.Time, mutate func(*entity.Order)) *entity.Order {
	order := &entity.Order{
		ID:          "id-" + no,
		OrderNo:     no,
		PassengerID: passengerID,
		TypeName:    "快车",
		StartID:     "poi-start-" + no,
		EndID:       "poi-end-" + no,
		State:       state,
		CreatedAt:   at,
		UpdatedAt:   at,
	}
	if mutate != nil {
		mutate(order)
	}
	_ = repo.Save(context.Background(), order)
	return order
}

// TestListMyOrders_FilterActive：「进行中」=PENDING/ACCEPTED/IN_SERVICE/IN_TRIP。
func TestListMyOrders_FilterActive(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	oPending := seedOrder(repo, "P-ACT-1", "pax-1", entity.OrderPending, base.Add(1*time.Minute), nil)
	oTrip := seedOrder(repo, "P-ACT-2", "pax-1", entity.OrderInTrip, base.Add(2*time.Minute), nil)
	seedOrder(repo, "P-ACT-3", "pax-1", entity.OrderEnded, base.Add(3*time.Minute), nil)
	seedOrder(repo, "P-ACT-4", "pax-1", entity.OrderCancelled, base.Add(4*time.Minute), nil)
	seedOrder(repo, "P-OTHER-ACT", "pax-2", entity.OrderInTrip, base.Add(5*time.Minute), nil)

	got, err := svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{PassengerID: "pax-1", Filter: "进行中", Page: 1, Size: 20})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)
	require.Len(t, got.List, 2)
	// created_at 倒序
	require.Equal(t, oTrip.OrderNo, got.List[0].ID)
	require.Equal(t, oPending.OrderNo, got.List[1].ID)
	for _, item := range got.List {
		require.Contains(t, []string{entity.OrderPending, entity.OrderAccepted, entity.OrderInService, entity.OrderInTrip}, item.State)
	}
}

// TestListMyOrders_UnpaidIncludesCancelFee：「待支付」=UNPAID 或（CANCELLED 且取消费未付）；
// 同一「已取消+取消费未付」订单同时出现在「已取消」与「待支付」两个 filter。
func TestListMyOrders_UnpaidIncludesCancelFee(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	oCancelFee := seedOrder(repo, "P-UNP-1", "pax-1", entity.OrderCancelled, base.Add(1*time.Minute), func(o *entity.Order) {
		o.Bills = []entity.Bill{{Kind: entity.BillKindCancelFee, TotalFee: 500, PayableFee: 500, State: entity.BillStateUnpaid}}
	})
	seedOrder(repo, "P-UNP-2", "pax-1", entity.OrderCancelled, base.Add(2*time.Minute), nil) // 已取消无费
	oUnpaid := seedOrder(repo, "P-UNP-3", "pax-1", entity.OrderUnpaid, base.Add(3*time.Minute), nil)
	seedOrder(repo, "P-UNP-4", "pax-1", entity.OrderEnded, base.Add(4*time.Minute), func(o *entity.Order) {
		o.Bills = []entity.Bill{{Kind: entity.BillKindTrip, PayableFee: 1200, State: entity.BillStateUnpaid}}
	})

	unpaid, err := svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{PassengerID: "pax-1", Filter: "待支付", Page: 1, Size: 20})
	require.NoError(t, err)
	unpaidIDs := make([]string, 0, len(unpaid.List))
	for _, item := range unpaid.List {
		unpaidIDs = append(unpaidIDs, item.ID)
	}
	require.ElementsMatch(t, []string{oUnpaid.OrderNo, oCancelFee.OrderNo}, unpaidIDs)

	cancelled, err := svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{PassengerID: "pax-1", Filter: "已取消", Page: 1, Size: 20})
	require.NoError(t, err)
	cancelledIDs := make([]string, 0, len(cancelled.List))
	for _, item := range cancelled.List {
		cancelledIDs = append(cancelledIDs, item.ID)
	}
	require.ElementsMatch(t, []string{oCancelFee.OrderNo, "P-UNP-2"}, cancelledIDs)
	require.Contains(t, unpaidIDs, oCancelFee.OrderNo)
}

// TestListMyOrders_AmountLabel：金额口径（附录 B）——
// 活动单→预估(price_snap)；ENDED+UNPAID→应付(P)；PAID→实付；PAID+退款→退款后净实付(P−r)；CANCELLED 无费→0。
func TestListMyOrders_AmountLabel(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	seedOrder(repo, "P-AMT-1", "pax-1", entity.OrderInTrip, base.Add(1*time.Minute), func(o *entity.Order) {
		o.PriceSnap = &entity.PriceSnapshot{T: 1800, Q: 500, H: 200, F: 1200}
		o.CpFen = 100
		o.CdFen = 50
	})
	seedOrder(repo, "P-AMT-2", "pax-1", entity.OrderEnded, base.Add(2*time.Minute), func(o *entity.Order) {
		o.Bills = []entity.Bill{{Kind: entity.BillKindTrip, PayableFee: 2000, State: entity.BillStateUnpaid}}
	})
	seedOrder(repo, "P-AMT-3", "pax-1", entity.OrderCompleted, base.Add(3*time.Minute), func(o *entity.Order) {
		o.Bills = []entity.Bill{{Kind: entity.BillKindTrip, PayableFee: 1500, State: entity.BillStatePaid}}
	})
	seedOrder(repo, "P-AMT-4", "pax-1", entity.OrderCompleted, base.Add(4*time.Minute), func(o *entity.Order) {
		o.Bills = []entity.Bill{{Kind: entity.BillKindTrip, PayableFee: 1500, State: entity.BillStatePaid}}
		o.Refunds = []entity.Refund{{RefundFee: 300}}
	})
	seedOrder(repo, "P-AMT-5", "pax-1", entity.OrderCancelled, base.Add(5*time.Minute), nil)

	got, err := svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{PassengerID: "pax-1", Filter: "全部", Page: 1, Size: 20})
	require.NoError(t, err)
	require.Equal(t, int64(5), got.Total)
	byID := make(map[string]*dto.MyOrderListItem, len(got.List))
	for _, item := range got.List {
		byID[item.ID] = item
	}

	active := byID["P-AMT-1"]
	require.NotNil(t, active)
	require.Equal(t, int64(1050), active.Amount) // priceSnap.F − Cp − Cd
	require.Equal(t, "预估", active.AmountLabel)
	require.Equal(t, "estimated", active.AmountBasis)

	payable := byID["P-AMT-2"]
	require.NotNil(t, payable)
	require.Equal(t, int64(2000), payable.Amount)
	require.Equal(t, "应付", payable.AmountLabel)
	require.Equal(t, "payable", payable.AmountBasis)
	require.Equal(t, entity.BillStateUnpaid, payable.BillState)

	paid := byID["P-AMT-3"]
	require.NotNil(t, paid)
	require.Equal(t, int64(1500), paid.Amount)
	require.Equal(t, "实付", paid.AmountLabel)
	require.Equal(t, "paid", paid.AmountBasis)

	netPaid := byID["P-AMT-4"]
	require.NotNil(t, netPaid)
	require.Equal(t, int64(1200), netPaid.Amount) // P − Σr
	require.Equal(t, "退款后净实付", netPaid.AmountLabel)
	require.Equal(t, "net_paid", netPaid.AmountBasis)

	freeCancel := byID["P-AMT-5"]
	require.NotNil(t, freeCancel)
	require.Equal(t, int64(0), freeCancel.Amount)
}

// TestListMyOrders_OnlyMine：他人订单不出现；id=orderNo；start_name/end_name 走 POI 解析。
func TestListMyOrders_OnlyMine(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	resolver := &fakePOIResolver{names: map[string]string{
		"poi-start-M1": "虹桥火车站",
		"poi-end-M1":   "浦东机场",
	}}
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(resolver)

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	seedOrder(repo, "M1", "pax-1", entity.OrderCompleted, base.Add(1*time.Minute), nil)
	seedOrder(repo, "X1", "pax-2", entity.OrderCompleted, base.Add(2*time.Minute), nil)
	seedOrder(repo, "M2", "pax-1", entity.OrderPending, base.Add(3*time.Minute), nil)

	got, err := svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{PassengerID: "pax-1", Filter: "全部", Page: 1, Size: 20})
	require.NoError(t, err)
	require.Equal(t, int64(2), got.Total)
	require.Len(t, got.List, 2)
	require.Equal(t, "M2", got.List[0].ID)
	require.Equal(t, "M1", got.List[1].ID)

	m1 := got.List[1]
	require.Equal(t, "快车", m1.TypeName)
	require.Equal(t, "虹桥火车站", m1.StartName)
	require.Equal(t, "浦东机场", m1.EndName)
	require.Equal(t, entity.OrderCompleted, m1.State)
	require.Equal(t, "已完成", m1.StateName)
	require.NotEmpty(t, m1.CreatedAt)
}

// Task 5 失败测试（docs/plans/2026-10-27-pax-orders-my.md）：
// P18 订单详情 —— 快照不可变、账单/时间线/退款全量、归属校验 404。

// FindByOrderNo 供 P18 详情测试使用：按 orders.orderNo 业务号精确查找（契约 id 口径）。
func (r *fakeRepository) FindByOrderNo(ctx context.Context, orderNo string) (*entity.Order, error) {
	for _, item := range r.items {
		if item.OrderNo == orderNo {
			copy := *item
			return &copy, nil
		}
	}
	return nil, entity.ErrOrderNotFound
}

// TestGetMyOrderDetail_SnapshotImmutable：司机/车辆/计价快照列直接读 orders 快照，
// 不查实时 driver 表（契约 3.6.2 设计思路：司机与车辆信息、计价版本取下单时快照）。
func TestGetMyOrderDetail_SnapshotImmutable(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{names: map[string]string{
		"poi-start-D1": "虹桥火车站",
		"poi-end-D1":   "浦东机场",
	}})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	seedOrder(repo, "D1", "pax-1", entity.OrderEnded, base, func(o *entity.Order) {
		o.TypeName = "快车"
		o.DistM = 12300
		o.DurS = 1800
		o.DriverName = "王师傅" // 下单/派单时落库的快照列（driverSnap.name）
		o.PriceSnap = &entity.PriceSnapshot{T: 1836, Q: 1377, H: 459, F: 1836, K: 50}
		o.Bills = []entity.Bill{{Kind: entity.BillKindTrip, TotalFee: 1836, PayableFee: 1520, State: entity.BillStateUnpaid}}
	})

	got, err := svc.GetMyOrderDetail(ctx, dto.GetMyOrderDetailCommand{PassengerID: "pax-1", OrderID: "D1"})
	require.NoError(t, err)

	// 司机快照：只读 orders 快照列（driverSnap.name），不查实时 driver 表
	require.NotNil(t, got.DriverSnap)
	require.Equal(t, "王师傅", got.DriverSnap.Name)

	// 计价快照：账单出参来自内嵌 bills[]，不按 priceSnap 重算
	require.NotNil(t, got.Bill)
	require.Equal(t, int64(1836), got.Bill.TotalFee)
	require.Equal(t, int64(1520), got.Bill.PayableFee)

	// 路线/车型为快照出参
	require.Equal(t, "D1", got.ID)
	require.Equal(t, "快车", got.TypeName)
	require.Equal(t, "虹桥火车站", got.StartName)
	require.Equal(t, "浦东机场", got.EndName)
	require.Equal(t, int64(12300), got.DistM)
	require.Equal(t, int64(1800), got.DurS)
}

// TestGetMyOrderDetail_IncludesTimelineBillRefunds：bill 输出 T…N 全量 + state；
// refunds 按时间序；时间线按 t 升序；不伪造状态回退（读什么出什么）。
func TestGetMyOrderDetail_IncludesTimelineBillRefunds(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	paidAt := base.Add(2 * time.Hour)
	seedOrder(repo, "D2", "pax-1", entity.OrderCompleted, base, func(o *entity.Order) {
		o.Bills = []entity.Bill{{
			Kind:                  entity.BillKindTrip,
			TotalFee:              1836,
			DriverGrossFee:        1377,
			PlatformServiceFee:    459,
			SurgeFee:              100,
			CouponFee:             50,
			PlatformCouponBearFee: 30,
			DriverCouponBearFee:   20,
			PayableFee:            1520,
			DriverIncomeFee:       1300,
			PlatformNetFee:        400,
			State:                 entity.BillStatePaid,
			PaidAt:                &paidAt,
		}}
		o.Timeline = []entity.TimelineItem{
			{Name: "订单完成", T: base.Add(90 * time.Minute)},
			{Name: "司机已到达", T: base.Add(5 * time.Minute)},
			{Name: "行程开始", T: base.Add(30 * time.Minute)},
		}
		o.Refunds = []entity.Refund{
			{RefundFee: 300, DriverRefundFee: 150, Mode: "original", State: "SUCCESS", T: base.Add(3 * time.Hour)},
			{RefundFee: 100, DriverRefundFee: 50, Mode: "original", State: "SUCCESS", T: base.Add(2 * time.Hour)},
		}
	})

	got, err := svc.GetMyOrderDetail(ctx, dto.GetMyOrderDetailCommand{PassengerID: "pax-1", OrderID: "D2"})
	require.NoError(t, err)

	// 账单 T…N 全量 + state/kind/paid_at
	require.Equal(t, int64(1836), got.Bill.TotalFee)
	require.Equal(t, int64(1377), got.Bill.DriverGrossFee)
	require.Equal(t, int64(459), got.Bill.PlatformServiceFee)
	require.Equal(t, int64(100), got.Bill.SurgeFee)
	require.Equal(t, int64(50), got.Bill.CouponFee)
	require.Equal(t, int64(30), got.Bill.PlatformCouponBearFee)
	require.Equal(t, int64(20), got.Bill.DriverCouponBearFee)
	require.Equal(t, int64(1520), got.Bill.PayableFee)
	require.Equal(t, int64(1300), got.Bill.DriverIncomeFee)
	require.Equal(t, int64(400), got.Bill.PlatformNetFee)
	require.Equal(t, entity.BillStatePaid, got.Bill.State)
	require.Equal(t, entity.BillKindTrip, got.Bill.Kind)
	require.Equal(t, paidAt.Format(time.RFC3339), got.Bill.PaidAt)

	// 时间线按 t 升序
	require.Len(t, got.Timeline, 3)
	require.Equal(t, "司机已到达", got.Timeline[0].Name)
	require.Equal(t, "行程开始", got.Timeline[1].Name)
	require.Equal(t, "订单完成", got.Timeline[2].Name)
	require.Equal(t, base.Add(5*time.Minute).Format(time.RFC3339), got.Timeline[0].T)

	// refunds 按时间序（升序）
	require.Len(t, got.Refunds, 2)
	require.Equal(t, int64(100), got.Refunds[0].RefundFee)
	require.Equal(t, int64(50), got.Refunds[0].DriverRefundFee)
	require.Equal(t, "original", got.Refunds[0].Mode)
	require.Equal(t, "SUCCESS", got.Refunds[0].State)
	require.Equal(t, base.Add(2*time.Hour).Format(time.RFC3339), got.Refunds[0].T)
	require.Equal(t, int64(300), got.Refunds[1].RefundFee)

	// 读什么出什么：状态原样出参（不伪造状态回退）
	require.Equal(t, entity.OrderCompleted, got.State)
	require.Equal(t, "已完成", got.StateName)
	require.Equal(t, "D2", got.ID)
	require.Equal(t, base.Format(time.RFC3339), got.CreatedAt)
}

// TestGetMyOrderDetail_NotMineReturnsNotFound：他人订单/不存在订单一律 NotFound（不泄露存在性，D4）。
func TestGetMyOrderDetail_NotMineReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepository()
	svc := NewService(repo, nil, zap.NewNop()).WithPOINameResolver(&fakePOIResolver{})

	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	seedOrder(repo, "D3", "pax-2", entity.OrderCompleted, base, nil)

	got, err := svc.GetMyOrderDetail(ctx, dto.GetMyOrderDetailCommand{PassengerID: "pax-1", OrderID: "D3"})
	require.ErrorIs(t, err, entity.ErrOrderNotFound)
	require.Nil(t, got)

	got, err = svc.GetMyOrderDetail(ctx, dto.GetMyOrderDetailCommand{PassengerID: "pax-1", OrderID: "missing"})
	require.ErrorIs(t, err, entity.ErrOrderNotFound)
	require.Nil(t, got)
}
