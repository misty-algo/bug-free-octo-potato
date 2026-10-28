package handler

import (
	"context"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// 乘客端 P17-P22 RPC 编译骨架（Task 3）。
// 各方法自 Task 4 起逐一落地；未实现的仍返回 Unimplemented，保证接口完整可注册。

// ListMyOrders P17 订单列表（GET /api/pax/orders）。
// passenger_id 只接受网关从 claims 注入的本人身份（D4 归属校验）。
func (s *Server) ListMyOrders(ctx context.Context, req *orderv1.ListMyOrdersRequest) (*orderv1.ListMyOrdersResponse, error) {
	if req == nil || req.PassengerId == "" {
		return nil, mapOrderError(entity.ErrInvalidOrder)
	}
	result, err := s.svc.ListMyOrders(ctx, dto.ListMyOrdersCommand{
		PassengerID: req.PassengerId,
		Filter:      req.Filter,
		Page:        req.Page,
		Size:        req.Size,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	items := make([]*orderv1.MyOrderListItem, 0, len(result.List))
	for _, item := range result.List {
		items = append(items, &orderv1.MyOrderListItem{
			Id:          item.ID,
			TypeName:    item.TypeName,
			StartName:   item.StartName,
			EndName:     item.EndName,
			State:       item.State,
			StateName:   item.StateName,
			Amount:      item.Amount,
			AmountLabel: item.AmountLabel,
			AmountBasis: item.AmountBasis,
			BillState:   item.BillState,
			CreatedAt:   item.CreatedAt,
		})
	}
	return &orderv1.ListMyOrdersResponse{List: items, Total: result.Total}, nil
}

// GetMyOrderDetail P18 订单详情（GET /api/pax/orders/:order_id）。
// passenger_id 只接受网关从 claims 注入的本人身份（D4 归属校验）。
func (s *Server) GetMyOrderDetail(ctx context.Context, req *orderv1.GetMyOrderDetailRequest) (*orderv1.MyOrderDetailResponse, error) {
	if req == nil || req.PassengerId == "" || req.OrderId == "" {
		return nil, mapOrderError(entity.ErrInvalidOrder)
	}
	detail, err := s.svc.GetMyOrderDetail(ctx, dto.GetMyOrderDetailCommand{
		PassengerID: req.PassengerId,
		OrderID:     req.OrderId,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	resp := &orderv1.MyOrderDetailResponse{
		Id:        detail.ID,
		TypeName:  detail.TypeName,
		StartName: detail.StartName,
		EndName:   detail.EndName,
		State:     detail.State,
		StateName: detail.StateName,
		DistM:     detail.DistM,
		DurS:      detail.DurS,
		CreatedAt: detail.CreatedAt,
		Timeline:  make([]*orderv1.OrderTimelineItem, 0, len(detail.Timeline)),
		Refunds:   make([]*orderv1.RefundItem, 0, len(detail.Refunds)),
	}
	for _, item := range detail.Timeline {
		resp.Timeline = append(resp.Timeline, &orderv1.OrderTimelineItem{Name: item.Name, T: item.T})
	}
	for _, item := range detail.Refunds {
		resp.Refunds = append(resp.Refunds, &orderv1.RefundItem{
			RefundFee:       item.RefundFee,
			DriverRefundFee: item.DriverRefundFee,
			Mode:            item.Mode,
			State:           item.State,
			T:               item.T,
		})
	}
	if detail.Bill != nil {
		resp.Bill = &orderv1.BillDetail{
			TotalFee:              detail.Bill.TotalFee,
			DriverGrossFee:        detail.Bill.DriverGrossFee,
			PlatformServiceFee:    detail.Bill.PlatformServiceFee,
			SurgeFee:              detail.Bill.SurgeFee,
			CouponFee:             detail.Bill.CouponFee,
			PlatformCouponBearFee: detail.Bill.PlatformCouponBearFee,
			DriverCouponBearFee:   detail.Bill.DriverCouponBearFee,
			PayableFee:            detail.Bill.PayableFee,
			DriverIncomeFee:       detail.Bill.DriverIncomeFee,
			PlatformNetFee:        detail.Bill.PlatformNetFee,
			State:                 detail.Bill.State,
			Kind:                  detail.Bill.Kind,
			PaidAt:                detail.Bill.PaidAt,
		}
	}
	if detail.DriverSnap != nil {
		resp.DriverSnap = &orderv1.DriverSnap{Name: detail.DriverSnap.Name}
	}
	return resp, nil
}

// SubmitRating P19 提交评价（POST /api/pax/order/rate）。
// passenger_id 只接受网关从 claims 注入的本人身份（D4 归属校验）。
func (s *Server) SubmitRating(ctx context.Context, req *orderv1.SubmitRatingRequest) (*orderv1.SubmitRatingResponse, error) {
	if req == nil || req.PassengerId == "" || req.OrderId == "" {
		return nil, mapOrderError(entity.ErrInvalidOrder)
	}
	result, err := s.svc.SubmitRating(ctx, dto.SubmitRatingCommand{
		PassengerID: req.PassengerId,
		OrderID:     req.OrderId,
		Rating:      req.Rating,
		Anonymous:   req.Anonymous,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &orderv1.SubmitRatingResponse{OrderId: result.OrderID, Rating: result.Rating}, nil
}

// SubmitComplaint P19 提交投诉（POST /api/pax/order/complaint）。
// passenger_id 只接受网关从 claims 注入的本人身份（D4 归属校验）。
func (s *Server) SubmitComplaint(ctx context.Context, req *orderv1.SubmitComplaintRequest) (*orderv1.SubmitComplaintResponse, error) {
	if req == nil || req.PassengerId == "" || req.OrderId == "" {
		return nil, mapOrderError(entity.ErrInvalidOrder)
	}
	result, err := s.svc.SubmitComplaint(ctx, dto.SubmitComplaintCommand{
		PassengerID:    req.PassengerId,
		OrderID:        req.OrderId,
		Type:           req.Type,
		Desc:           req.Desc,
		Images:         req.Images,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &orderv1.SubmitComplaintResponse{
		TicketId:  result.TicketID,
		State:     result.State,
		StateName: result.StateName,
	}, nil
}

// ListMyTickets 查询乘客本人投诉工单进度（P22，契约 3.7.3）。
func (s *Server) ListMyTickets(ctx context.Context, req *orderv1.ListMyTicketsRequest) (*orderv1.ListMyTicketsResponse, error) {
	result, err := s.svc.ListMyTickets(ctx, req.GetPassengerId())
	if err != nil {
		return nil, mapOrderError(err)
	}
	resp := &orderv1.ListMyTicketsResponse{Total: result.Total, List: []*orderv1.MyTicketProgress{}}
	for _, item := range result.List {
		ticket := &orderv1.MyTicketProgress{
			TicketId:  item.TicketID,
			TicketNo:  item.TicketNo,
			OrderNo:   item.OrderNo,
			Type:      item.Type,
			State:     item.State,
			StateName: item.StateName,
			Round:     item.Round,
			CreatedAt: item.CreatedAt,
			Timeline:  []*orderv1.TicketTimelineItem{},
		}
		for _, tl := range item.Timeline {
			ticket.Timeline = append(ticket.Timeline, &orderv1.TicketTimelineItem{
				Seq: tl.Seq, Actor: tl.Actor, Action: tl.Action,
				From: tl.From, To: tl.To, Message: tl.Message, At: tl.At,
			})
		}
		resp.List = append(resp.List, ticket)
	}
	return resp, nil
}
