package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// myTicketsRepo 在 fakeRepository 上补 P22 我的工单查询端口（按 complainantId 过滤，createdAt 倒序）。
type myTicketsRepo struct {
	*fakeRepository
	tickets map[string]*entity.Ticket
	listed  string
}

var _ entity.TicketQueryRepository = (*myTicketsRepo)(nil)

func newMyTicketsRepo() *myTicketsRepo {
	return &myTicketsRepo{fakeRepository: newFakeRepository(), tickets: map[string]*entity.Ticket{}}
}

func (r *myTicketsRepo) ListTicketsByComplainant(_ context.Context, complainantID string) ([]*entity.Ticket, error) {
	r.listed = complainantID
	out := []*entity.Ticket{}
	for _, tk := range r.tickets {
		if tk.ComplainID != complainantID {
			continue
		}
		snap := *tk
		out = append(out, &snap)
	}
	// 模拟 Mongo sort({createdAt: -1})
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// TestListMyTickets_Progress：工单进度映射（state/state_name/round/timeline），归属按 complainantId 过滤。
func TestListMyTickets_Progress(t *testing.T) {
	ctx := context.Background()
	repo := newMyTicketsRepo()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	repo.tickets["TK2610270001"] = &entity.Ticket{
		ID: "id-1", TicketNo: "TK2610270001", OrderNo: "C1", Type: "绕路",
		State: entity.TicketProcessing, ComplainID: "pax-1",
		Round: 2, CreatedAt: base,
		Timeline: []entity.TicketEvent{
			{Seq: 2, Actor: "系统", Action: "重新分配", At: base.Add(2 * time.Hour)},
			{Seq: 1, Actor: "系统", Action: "创建", At: base},
		},
	}
	repo.tickets["TK2610270002"] = &entity.Ticket{
		ID: "id-2", TicketNo: "TK2610270002", OrderNo: "C2", Type: "其他",
		State: entity.TicketResolved, ComplainID: "pax-1",
		Round: 1, CreatedAt: base.Add(time.Hour),
		Timeline: []entity.TicketEvent{{Seq: 1, Actor: "系统", Action: "创建", At: base}},
	}
	// 他人单不出现（D4 归属过滤）
	repo.tickets["TK2610270003"] = &entity.Ticket{
		ID: "id-3", TicketNo: "TK2610270003", OrderNo: "C3", State: entity.TicketProcessing, ComplainID: "pax-2",
	}

	svc := NewService(repo, nil, zap.NewNop())
	got, err := svc.ListMyTickets(ctx, "pax-1")
	require.NoError(t, err)
	require.Equal(t, "pax-1", repo.listed)
	require.EqualValues(t, 2, got.Total)
	require.Len(t, got.List, 2)

	// createdAt 倒序：新单在前
	first := got.List[0]
	require.Equal(t, "TK2610270002", first.TicketID)
	require.Equal(t, "TK2610270002", first.TicketNo)
	require.Equal(t, entity.TicketResolved, first.State)
	require.Equal(t, "已完结", first.StateName)
	require.Equal(t, "其他", first.Type)
	require.EqualValues(t, 1, first.Round)
	require.Equal(t, base.Add(time.Hour).Format(time.RFC3339), first.CreatedAt)

	// 带进度：state/round/timeline 齐全，timeline 按 seq 升序
	second := got.List[1]
	require.Equal(t, "TK2610270001", second.TicketID)
	require.Equal(t, entity.TicketProcessing, second.State)
	require.Equal(t, "处理中", second.StateName)
	require.EqualValues(t, 2, second.Round)
	require.Equal(t, "绕路", second.Type)
	require.Equal(t, "C1", second.OrderNo)
	require.Len(t, second.Timeline, 2)
	require.EqualValues(t, 1, second.Timeline[0].Seq)
	require.Equal(t, "创建", second.Timeline[0].Action)
	require.Equal(t, base.Format(time.RFC3339), second.Timeline[0].At)
	require.EqualValues(t, 2, second.Timeline[1].Seq)

	// 无单：total=0，列表空（不 500）
	none, err := svc.ListMyTickets(ctx, "pax-9")
	require.NoError(t, err)
	require.EqualValues(t, 0, none.Total)
	require.Empty(t, none.List)
}
