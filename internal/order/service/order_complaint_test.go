package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// Task 7 失败测试（docs/plans/2026-10-27-pax-orders-my.md）：
// P19 提交投诉 —— 幂等键同号不重复建单、desc ≤500、图片 ≤6 且文字提交不丢、五类校验、危险驾驶写安全侧记录。

// complaintRepo 在 fakeRepository 上补 P19 投诉写入端口：
// tickets 按 ticketNo 幂等（同号重复 CreateTicket 不重复建单），并记录安全侧写入。
type complaintRepo struct {
	*fakeRepository
	tickets       map[string]*entity.Ticket
	created       []*entity.Ticket
	safetyReports []*entity.SafetyEventReport
	eventLogs     []*entity.EventLog
}

func newComplaintRepo() *complaintRepo {
	return &complaintRepo{fakeRepository: newFakeRepository(), tickets: map[string]*entity.Ticket{}}
}

func (r *complaintRepo) CreateTicket(ctx context.Context, ticket *entity.Ticket) (bool, error) {
	if _, ok := r.tickets[ticket.TicketNo]; ok {
		return false, nil // 同号幂等：不重复建单
	}
	snap := *ticket
	r.tickets[ticket.TicketNo] = &snap
	r.created = append(r.created, &snap)
	return true, nil
}

func (r *complaintRepo) FindTicketByTicketNo(ctx context.Context, ticketNo string) (*entity.Ticket, error) {
	item, ok := r.tickets[ticketNo]
	if !ok {
		return nil, entity.ErrOrderNotFound
	}
	snap := *item
	return &snap, nil
}

func (r *complaintRepo) RecordDangerousDriving(ctx context.Context, report *entity.SafetyEventReport, log *entity.EventLog) error {
	r.safetyReports = append(r.safetyReports, report)
	r.eventLogs = append(r.eventLogs, log)
	return nil
}

func newComplaintService(repo entity.Repository, now time.Time) *Service {
	return NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: now})
}

func complaintCmd(orderID, typ, desc, key string, images []string) dto.SubmitComplaintCommand {
	return dto.SubmitComplaintCommand{
		PassengerID:    "pax-1",
		OrderID:        orderID,
		Type:           typ,
		Desc:           desc,
		Images:         images,
		IdempotencyKey: key,
	}
}

// TestSubmitComplaint_Idempotency：同幂等键返回同一 ticket_id（不重复建单）；不同键是新单。
func TestSubmitComplaint_Idempotency(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newComplaintRepo()
	seedOrder(repo.fakeRepository, "C1", "pax-1", entity.OrderEnded, base, nil)
	svc := newComplaintService(repo, base.Add(time.Hour))

	got, err := svc.SubmitComplaint(ctx, complaintCmd("C1", "绕路", "司机绕路", "key-1", []string{"http://img/1.jpg"}))
	require.NoError(t, err)
	require.NotEmpty(t, got.TicketID)
	require.Equal(t, entity.TicketPending, got.State)
	require.Equal(t, "待处理", got.StateName)

	got2, err := svc.SubmitComplaint(ctx, complaintCmd("C1", "绕路", "司机绕路", "key-1", []string{"http://img/1.jpg"}))
	require.NoError(t, err)
	require.Equal(t, got.TicketID, got2.TicketID)
	require.Len(t, repo.created, 1) // 不重复建单

	got3, err := svc.SubmitComplaint(ctx, complaintCmd("C1", "绕路", "司机绕路", "key-2", nil))
	require.NoError(t, err)
	require.NotEqual(t, got.TicketID, got3.TicketID)
	require.Len(t, repo.created, 2)
}

// TestSubmitComplaint_DescLimit：描述 >500 字拒绝，≤500 字放行（契约 ≤500 字）。
func TestSubmitComplaint_DescLimit(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newComplaintRepo()
	seedOrder(repo.fakeRepository, "C2", "pax-1", entity.OrderEnded, base, nil)
	svc := newComplaintService(repo, base.Add(time.Hour))

	_, err := svc.SubmitComplaint(ctx, complaintCmd("C2", "绕路", strings.Repeat("字", 501), "k1", nil))
	require.ErrorIs(t, err, entity.ErrInvalidOrder)

	got, err := svc.SubmitComplaint(ctx, complaintCmd("C2", "绕路", strings.Repeat("字", 500), "k2", nil))
	require.NoError(t, err)
	require.NotEmpty(t, got.TicketID)
}

// TestSubmitComplaint_ImagesLimit：>6 张拒绝；图片失败/缺失不丢文字提交。
func TestSubmitComplaint_ImagesLimit(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newComplaintRepo()
	seedOrder(repo.fakeRepository, "C3", "pax-1", entity.OrderEnded, base, nil)
	svc := newComplaintService(repo, base.Add(time.Hour))

	_, err := svc.SubmitComplaint(ctx, complaintCmd("C3", "服务态度", "服务差", "k1",
		[]string{"1", "2", "3", "4", "5", "6", "7"}))
	require.ErrorIs(t, err, entity.ErrInvalidOrder)

	// 图片上传失败保留文字提交：images 为空照常建单
	got, err := svc.SubmitComplaint(ctx, complaintCmd("C3", "服务态度", "服务差", "k2", nil))
	require.NoError(t, err)
	require.NotEmpty(t, got.TicketID)
	item := repo.tickets[got.TicketID]
	require.Equal(t, "服务差", item.Content)
	require.Empty(t, item.Evidences)
}

// TestSubmitComplaint_TypeValid：契约五类（中文）放行并映射机器码；非法类型拒绝。
func TestSubmitComplaint_TypeValid(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newComplaintRepo()
	seedOrder(repo.fakeRepository, "C4", "pax-1", entity.OrderEnded, base, nil)
	svc := newComplaintService(repo, base.Add(time.Hour))

	for i, typ := range []string{"绕路", "服务态度", "危险驾驶", "费用异议", "其他"} {
		got, err := svc.SubmitComplaint(ctx, complaintCmd("C4", typ, "投诉内容", "k"+typ, nil))
		require.NoError(t, err, "类型 %q 应被接受", typ)
		require.NotEmpty(t, got.TicketID)
		_ = i
	}
	require.Equal(t, entity.TicketTypeRouteDetour, repo.tickets[repo.created[0].TicketNo].Type)
	require.Equal(t, entity.TicketTypeDangerousDriving, repo.tickets[repo.created[2].TicketNo].Type)

	_, err := svc.SubmitComplaint(ctx, complaintCmd("C4", "乱写", "投诉内容", "k-bad", nil))
	require.ErrorIs(t, err, entity.ErrInvalidOrder)
}

// TestSubmitComplaint_DangerousDrivingEvent：危险驾驶额外写 safety_event+event_log 记录，其余类型不写。
func TestSubmitComplaint_DangerousDrivingEvent(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newComplaintRepo()
	seedOrder(repo.fakeRepository, "C5", "pax-1", entity.OrderEnded, base, nil)
	svc := newComplaintService(repo, base.Add(time.Hour))

	got, err := svc.SubmitComplaint(ctx, complaintCmd("C5", "危险驾驶", "司机飙车", "k-danger", nil))
	require.NoError(t, err)

	require.Len(t, repo.safetyReports, 1)
	report := repo.safetyReports[0]
	require.Equal(t, got.TicketID, report.TicketNo)
	require.Equal(t, "C5", report.OrderNo)
	require.Equal(t, "司机飙车", report.Description)
	require.Equal(t, "k-danger", report.ClientRequestID)
	require.NotEmpty(t, report.EventID)

	require.Len(t, repo.eventLogs, 1)
	ev := repo.eventLogs[0]
	require.Equal(t, report.EventID, ev.EventID)
	require.Equal(t, "safety", ev.Kind)

	// 非危险驾驶不写安全侧记录
	_, err = svc.SubmitComplaint(ctx, complaintCmd("C5", "绕路", "绕远路", "k-normal", nil))
	require.NoError(t, err)
	require.Len(t, repo.safetyReports, 1)
	require.Len(t, repo.eventLogs, 1)
}
