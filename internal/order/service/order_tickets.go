package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// ListMyTickets 返回我的工单进度（P22，契约 3.7.3 my_tickets）。
// 归属按 complainantId 过滤（D4：只出本人单）；每项含 state/round/timeline 进度。
func (s *Service) ListMyTickets(ctx context.Context, passengerID string) (*dto.ListMyTicketsResult, error) {
	repo, ok := s.repo.(entity.TicketQueryRepository)
	if !ok {
		return nil, errors.New("ticket query repository not available")
	}
	tickets, err := repo.ListTicketsByComplainant(ctx, passengerID)
	if err != nil {
		return nil, err
	}
	result := &dto.ListMyTicketsResult{List: []*dto.MyTicketProgress{}, Total: int64(len(tickets))}
	for _, tk := range tickets {
		result.List = append(result.List, toTicketProgress(tk))
	}
	return result, nil
}

// toTicketProgress 映射工单进度；timeline 按 seq 升序，时间为 RFC3339（全 pax 口径一致）。
func toTicketProgress(tk *entity.Ticket) *dto.MyTicketProgress {
	item := &dto.MyTicketProgress{
		TicketID:  tk.TicketNo,
		TicketNo:  tk.TicketNo,
		OrderNo:   tk.OrderNo,
		Type:      tk.Type,
		State:     tk.State,
		StateName: entity.TicketStateName(tk.State),
		Round:     tk.Round,
		CreatedAt: tk.CreatedAt.Format(time.RFC3339),
		Timeline:  []*dto.TicketTimelineItem{},
	}
	events := append([]entity.TicketEvent{}, tk.Timeline...)
	sort.SliceStable(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
	for _, ev := range events {
		item.Timeline = append(item.Timeline, &dto.TicketTimelineItem{
			Seq:     ev.Seq,
			Actor:   ev.Actor,
			Action:  ev.Action,
			From:    ev.From,
			To:      ev.To,
			Message: ev.Message,
			At:      ev.At.Format(time.RFC3339),
		})
	}
	return item
}
