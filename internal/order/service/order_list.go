package service

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"go.uber.org/zap"
)

// POINameResolver 按 POI ID 批量解析名称（契约出参 start_name/end_name，涉及表 poi）。
// orders 仅存 startId/endId 引用，名称从 pois 表解析；不查实时 driver 表（快照原则）。
type POINameResolver interface {
	ResolveNames(ctx context.Context, ids []string) (map[string]string, error)
}

const (
	defaultListSize = 20 // 契约：每页 20 条
	maxListSize     = 100
)

// WithPOINameResolver 注入 POI 名称解析器（可选；缺省时名称出空串降级）。
func (s *Service) WithPOINameResolver(resolver POINameResolver) *Service {
	s.poiNames = resolver
	return s
}

// ListMyOrders 是 P17 乘客订单列表（GET /api/pax/orders）。
// 只读本人订单（claims 归属校验在 D4），created_at 倒序分页；
// 过滤口径见 entity.MatchesFilter（附录 E），金额口径见 entity.AmountSummary（附录 B）。
func (s *Service) ListMyOrders(ctx context.Context, cmd dto.ListMyOrdersCommand) (*dto.ListMyOrdersResult, error) {
	if cmd.PassengerID == "" {
		return nil, entity.ErrInvalidOrder
	}
	view, err := entity.NormalizeOrderView(cmd.Filter)
	if err != nil {
		return nil, err
	}
	queryRepo, ok := s.repo.(entity.OrderQueryRepository)
	if !ok {
		return nil, errors.New("order query repository not available")
	}
	size := int(cmd.Size)
	if size <= 0 {
		size = defaultListSize
	}
	if size > maxListSize {
		size = maxListSize
	}
	page := int(cmd.Page)
	if page < 1 {
		page = 1
	}

	items, total, err := queryRepo.ListFiltered(ctx, &entity.OrderFilter{
		PassengerID: cmd.PassengerID,
		View:        view,
		Limit:       size,
		Offset:      (page - 1) * size,
	})
	if err != nil {
		return nil, err
	}

	names := s.resolvePOINames(ctx, items)
	result := &dto.ListMyOrdersResult{
		List:  make([]*dto.MyOrderListItem, 0, len(items)),
		Total: total,
	}
	for _, order := range items {
		amount := order.AmountSummary()
		result.List = append(result.List, &dto.MyOrderListItem{
			ID:          listOrderID(order),
			TypeName:    order.TypeName,
			StartName:   names[order.StartID],
			EndName:     names[order.EndID],
			State:       order.State,
			StateName:   stateName(order.State),
			Amount:      amount.Amount,
			AmountLabel: amount.Label,
			AmountBasis: amount.Basis,
			BillState:   amount.BillState,
			CreatedAt:   order.CreatedAt.Format(time.RFC3339),
		})
	}
	return result, nil
}

// resolvePOINames 批量解析起终点名称；解析器缺失或失败时降级为空名称（列表不因名称失败）。
func (s *Service) resolvePOINames(ctx context.Context, items []*entity.Order) map[string]string {
	if s.poiNames == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, 2*len(items))
	seen := make(map[string]struct{}, 2*len(items))
	for _, order := range items {
		for _, id := range []string{order.StartID, order.EndID} {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	names, err := s.poiNames.ResolveNames(ctx, ids)
	if err != nil {
		s.log.Warn("resolve poi names failed, degrade to empty", zap.Error(err))
		return map[string]string{}
	}
	return names
}

// listOrderID 契约 id 出参用 orders.orderNo 业务号（计划附录/契约定案），缺省回退内部主键。
func listOrderID(order *entity.Order) string {
	if order.OrderNo != "" {
		return order.OrderNo
	}
	return order.ID
}

// stateName 是状态中文映射（计划附录 E：线上 Mongo 枚举 → 契约中文）。
func stateName(state string) string {
	switch state {
	case entity.OrderPending:
		return "待接单"
	case entity.OrderAccepted:
		return "已接单"
	case entity.OrderInService:
		return "司机已到达"
	case entity.OrderInTrip:
		return "行程中"
	case entity.OrderEnded, entity.OrderCompleted:
		return "已完成"
	case entity.OrderUnpaid:
		return "待支付"
	case entity.OrderCancelled, entity.OrderNoDriver:
		return "已取消"
	default:
		return ""
	}
}
