package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

type fakeSafetyRepo struct {
	events map[string]*entity.SafetyEvent
	byID   map[string]*entity.SafetyEvent
	logs   map[string]string
	// beforeEscalate 在升级条件更新前触发，用于 S03 交错：预检暂停 → 领取 → 恢复。
	beforeEscalate func()
}

func newFakeSafetyRepo() *fakeSafetyRepo {
	return &fakeSafetyRepo{
		events: map[string]*entity.SafetyEvent{},
		byID:   map[string]*entity.SafetyEvent{},
		logs:   map[string]string{},
	}
}

// CreateReport 模拟 (driverId, clientRequestId) 唯一索引上的 $setOnInsert。
func (f *fakeSafetyRepo) CreateReport(_ context.Context, event entity.SafetyEvent) (bool, error) {
	key := event.DriverID + "|" + event.Raw.ClientRequestID
	if _, ok := f.events[key]; ok {
		return false, nil
	}
	stored := event
	f.events[key] = &stored
	f.byID[event.EventID] = &stored
	return true, nil
}

func (f *fakeSafetyRepo) ByEventID(_ context.Context, eventID string) (*entity.SafetyEvent, error) {
	return f.byID[eventID], nil
}

func (f *fakeSafetyRepo) ByRequest(_ context.Context, driverID, requestID string) (*entity.SafetyEvent, error) {
	return f.events[driverID+"|"+requestID], nil
}

func (f *fakeSafetyRepo) AddSupplement(_ context.Context, eventID string, sup entity.SafetySupplement) (*entity.SafetyEvent, error) {
	event, ok := f.byID[eventID]
	if !ok {
		return nil, nil
	}
	event.Supplements = append(event.Supplements, sup)
	return event, nil
}

// AddAttachment 模拟 $addToSet：重复绑定不产生第二条。
func (f *fakeSafetyRepo) AddAttachment(_ context.Context, eventID, fileID string) (*entity.SafetyEvent, error) {
	event, ok := f.byID[eventID]
	if !ok {
		return nil, nil
	}
	for _, id := range event.Attachments {
		if id == fileID {
			return event, nil
		}
	}
	event.Attachments = append(event.Attachments, fileID)
	return event, nil
}

func (f *fakeSafetyRepo) Pending(_ context.Context, _ string, _ string, _ int) ([]entity.SafetyEvent, error) {
	out := make([]entity.SafetyEvent, 0, len(f.events))
	for _, event := range f.events {
		out = append(out, *event)
	}
	return out, nil
}

// 处置命令：模拟带 version + 状态条件的原子更新，未命中返回 (nil, nil)。
func (f *fakeSafetyRepo) Claim(_ context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	event := f.byID[eventID]
	if event == nil || event.Version != expectedVersion || event.State != entity.SafetyStateReceived {
		return nil, nil
	}
	event.State = entity.SafetyStateClaimed
	event.Handler = handler
	event.ClaimedAt = &at
	event.Version = expectedVersion + 1
	event.Actions = append(event.Actions, action)
	return event, nil
}

func (f *fakeSafetyRepo) Start(_ context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	event := f.byID[eventID]
	if event == nil || event.Version != expectedVersion || event.State != entity.SafetyStateClaimed || event.Handler != handler {
		return nil, nil
	}
	event.State = entity.SafetyStateProcessing
	event.Version = expectedVersion + 1
	event.Actions = append(event.Actions, action)
	return event, nil
}

func (f *fakeSafetyRepo) Transfer(_ context.Context, eventID string, expectedVersion int64, fromHandler, toHandler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	event := f.byID[eventID]
	if event == nil || event.Version != expectedVersion || event.Handler != fromHandler {
		return nil, nil
	}
	if event.State != entity.SafetyStateClaimed && event.State != entity.SafetyStateProcessing {
		return nil, nil
	}
	event.Handler = toHandler
	event.Version = expectedVersion + 1
	event.Actions = append(event.Actions, action)
	return event, nil
}

func (f *fakeSafetyRepo) AddAction(_ context.Context, eventID string, expectedVersion int64, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	event := f.byID[eventID]
	if event == nil || event.Version != expectedVersion || event.State == entity.SafetyStateResolved {
		return nil, nil
	}
	event.Version = expectedVersion + 1
	event.Actions = append(event.Actions, action)
	return event, nil
}

func (f *fakeSafetyRepo) Resolve(_ context.Context, eventID string, expectedVersion int64, handler string, action entity.SafetyAction, at time.Time) (*entity.SafetyEvent, error) {
	event := f.byID[eventID]
	if event == nil || event.Version != expectedVersion || event.Handler != handler {
		return nil, nil
	}
	if event.State != entity.SafetyStateClaimed && event.State != entity.SafetyStateProcessing {
		return nil, nil
	}
	event.State = entity.SafetyStateResolved
	event.Version = expectedVersion + 1
	event.Actions = append(event.Actions, action)
	return event, nil
}

func (f *fakeSafetyRepo) EscalateCandidates(_ context.Context, now time.Time, _ int) ([]entity.SafetyEvent, error) {
	cutoff := now.Add(-time.Duration(entity.EscalationAfter) * time.Second)
	out := make([]entity.SafetyEvent, 0, len(f.events))
	for _, event := range f.events {
		if event.State == entity.SafetyStateReceived && !event.Raw.ReceivedAt.After(cutoff) {
			out = append(out, *event)
		}
	}
	return out, nil
}

func (f *fakeSafetyRepo) TryEscalate(_ context.Context, eventID string, escalation entity.SafetyEscalation, outbox entity.SafetyOutbox) (bool, error) {
	if f.beforeEscalate != nil {
		f.beforeEscalate()
	}
	event := f.byID[eventID]
	if event == nil || event.State != entity.SafetyStateReceived {
		return false, nil
	}
	for _, e := range event.Escalations {
		if e.Level == escalation.Level {
			return false, nil
		}
	}
	event.Escalations = append(event.Escalations, escalation)
	event.Outbox = append(event.Outbox, outbox)
	return true, nil
}

func (f *fakeSafetyRepo) PendingOutbox(_ context.Context, _ int) ([]entity.SafetyOutboxRef, error) {
	out := make([]entity.SafetyOutboxRef, 0, len(f.events))
	for _, event := range f.events {
		for _, entry := range event.Outbox {
			if entry.State == entity.OutboxPending {
				out = append(out, entity.SafetyOutboxRef{EventID: event.EventID, Entry: entry})
				break
			}
		}
	}
	return out, nil
}

func (f *fakeSafetyRepo) MarkOutboxSent(_ context.Context, eventID, entryID string, at time.Time) error {
	event := f.byID[eventID]
	if event == nil {
		return nil
	}
	for i := range event.Outbox {
		if event.Outbox[i].ID == entryID {
			event.Outbox[i].State = entity.OutboxSent
			event.Outbox[i].SentAt = &at
		}
	}
	return nil
}

func (f *fakeSafetyRepo) BumpOutboxAttempt(_ context.Context, eventID, entryID string) error {
	event := f.byID[eventID]
	if event == nil {
		return nil
	}
	for i := range event.Outbox {
		if event.Outbox[i].ID == entryID {
			event.Outbox[i].Attempts++
		}
	}
	return nil
}

type fakeTaskReader struct{ ids []string }

func (f *fakeTaskReader) HasActiveTask(context.Context, string) (bool, error) {
	return len(f.ids) > 0, nil
}
func (f *fakeTaskReader) ActiveTaskIDs(context.Context, string) ([]string, error) {
	return f.ids, nil
}

type fakeLocation struct {
	lng, lat float64
	at       *time.Time
}

func (f *fakeLocation) LastKnownLocation(context.Context, string) (float64, float64, *time.Time, error) {
	return f.lng, f.lat, f.at, nil
}

func newSafetyService(
	repo *fakeSafetyRepo,
	tasks *fakeTaskReader,
	location *fakeLocation,
) *SafetyService {
	return NewSafetyService(repo, tasks, location, nil, nil, nil, clock.Real{}, zap.NewNop())
}

func driverPrincipal() authn.Principal {
	return authn.Principal{UserID: "D1", Role: "driver"}
}

// errorCode 取出业务错误码，便于断言具体拒绝原因。
func errorCode(err error) string {
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code
	}
	return ""
}

func reportCmd(requestID string) dto.ReportEventCommand {
	return dto.ReportEventCommand{
		Principal: driverPrincipal(), ClientRequestID: requestID,
		EventType: "accident", Description: "车辆被追尾",
	}
}

// TestReportAcceptsUnknownPosition 校验拒绝定位也能求助并被受理。
func TestReportAcceptsUnknownPosition(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	view, err := svc.Report(context.Background(), reportCmd("REQ-1"))
	require.NoError(t, err)
	require.NotEmpty(t, view.EventID)
	require.Equal(t, entity.PositionUnknown, view.Position.Kind)
	require.Nil(t, view.Position.Lng)
	require.NotNil(t, view.ReceivedAt)
}

// TestReportRejectsZeroPositionAsEvidence 校验 0,0 不被当作位置，但求助照常受理。
func TestReportRejectsZeroPositionAsEvidence(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	cmd := reportCmd("REQ-ZERO")
	cmd.HasPosition = true
	cmd.Lng, cmd.Lat = 0, 0
	view, err := svc.Report(context.Background(), cmd)
	require.NoError(t, err)
	require.NotEmpty(t, view.EventID)
	require.Equal(t, entity.PositionUnknown, view.Position.Kind)
	require.Nil(t, view.Position.Lng)
	event := repo.events["D1|REQ-ZERO"]
	require.Contains(t, event.AuditTrail[0].Detail, "zero coordinates")
}

// TestReportPositionKinds 校验三态判定：本次采集 > 最后已知 > 未知。
func TestReportPositionKinds(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	cmd := reportCmd("REQ-FRESH")
	cmd.HasPosition = true
	cmd.Lng, cmd.Lat = 121.47, 31.23
	cmd.CapturedAt = &now
	view, err := svc.Report(context.Background(), cmd)
	require.NoError(t, err)
	require.Equal(t, entity.PositionAvailable, view.Position.Kind)
	require.Equal(t, entity.PositionSourceClient, view.Position.Source)

	// 采集时间过旧：本次坐标不作证据，回落服务端最后已知位置。
	stale := now.Add(-30 * time.Minute)
	lastAt := now.Add(-5 * time.Minute)
	repo2 := newFakeSafetyRepo()
	svc2 := newSafetyService(repo2, &fakeTaskReader{}, &fakeLocation{lng: 121.5, lat: 31.2, at: &lastAt})
	cmd2 := reportCmd("REQ-STALE")
	cmd2.HasPosition = true
	cmd2.Lng, cmd2.Lat = 121.47, 31.23
	cmd2.CapturedAt = &stale
	view, err = svc2.Report(context.Background(), cmd2)
	require.NoError(t, err)
	require.Equal(t, entity.PositionLastKnown, view.Position.Kind)
	require.Equal(t, entity.PositionSourceLastKnown, view.Position.Source)
	require.Equal(t, lastAt.Unix(), view.Position.CapturedAt.Unix(), "保留最后位置的真实采集时间")

	// 没有最后位置：仍是 UNKNOWN，不接受 0,0 填充。
	repo3 := newFakeSafetyRepo()
	svc3 := newSafetyService(repo3, &fakeTaskReader{}, &fakeLocation{})
	cmd3 := reportCmd("REQ-NONE")
	cmd3.CapturedAt = &stale
	view, err = svc3.Report(context.Background(), cmd3)
	require.NoError(t, err)
	require.Equal(t, entity.PositionUnknown, view.Position.Kind)
	require.Nil(t, view.Position.Lat)
}

// TestReportIdempotency 校验同键同内容返回同一编号，同键不同内容拒绝。
func TestReportIdempotency(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()
	first, err := svc.Report(ctx, reportCmd("REQ-SAME"))
	require.NoError(t, err)

	// 响应丢失后用原请求号重试：必须回到同一个事件。
	retry, err := svc.Report(ctx, reportCmd("REQ-SAME"))
	require.NoError(t, err)
	require.Equal(t, first.EventID, retry.EventID)
	require.Equal(t, first.ReceivedAt.Unix(), retry.ReceivedAt.Unix())

	// 按原请求号查询也能拿到同一事件。
	found, err := svc.ByRequest(ctx, dto.ByRequestCommand{
		Principal: driverPrincipal(), ClientRequestID: "REQ-SAME",
	})
	require.NoError(t, err)
	require.Equal(t, first.EventID, found.EventID)

	// 同一个请求号提交另一个险情：拒绝，且不创建新编号。
	changed := reportCmd("REQ-SAME")
	changed.EventType = "medical"
	_, err = svc.Report(ctx, changed)
	require.Error(t, err)
	require.Equal(t, "IDEMPOTENCY_CONFLICT", errorCode(err))
	require.Len(t, repo.events, 1)
}

// TestByRequestReportsNotReceived 校验未受理时明确返回 404，而不是新建。
func TestByRequestReportsNotReceived(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	_, err := svc.ByRequest(context.Background(), dto.ByRequestCommand{
		Principal: driverPrincipal(), ClientRequestID: "REQ-MISSING",
	})
	require.Error(t, err)
	require.Equal(t, "REPORT_NOT_RECEIVED", errorCode(err))
	require.Empty(t, repo.events)
}

// TestSupplementDoesNotOverwriteRaw 校验补充只追加，不覆盖最初的位置证据。
func TestSupplementDoesNotOverwriteRaw(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()
	created, err := svc.Report(ctx, reportCmd("REQ-SUP"))
	require.NoError(t, err)
	require.Equal(t, entity.PositionUnknown, created.Position.Kind)

	now := time.Now().UTC()
	updated, err := svc.AddSupplement(ctx, dto.AddSupplementCommand{
		Principal: driverPrincipal(), EventID: created.EventID, Kind: "position",
		HasPosition: true, Lng: 121.47, Lat: 31.23, CapturedAt: &now,
	})
	require.NoError(t, err)
	require.Equal(t, 1, updated.Supplements)
	require.Equal(t, entity.PositionUnknown, updated.Position.Kind, "原始证据仍是 UNKNOWN")

	event := repo.byID[created.EventID]
	require.Equal(t, entity.PositionAvailable, event.Supplements[0].Position.Kind)
	require.Equal(t, int64(1), event.Supplements[0].Seq)
}

// TestAttachmentIsAppendedAfterReport 校验附件后补：先受理，再绑定。
func TestAttachmentIsAppendedAfterReport(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()
	created, err := svc.Report(ctx, reportCmd("REQ-FILE"))
	require.NoError(t, err)
	require.Empty(t, created.Attachments)

	updated, err := svc.AddAttachment(ctx, dto.AddAttachmentCommand{
		Principal: driverPrincipal(), EventID: created.EventID, FileID: "F:1",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"F:1"}, updated.Attachments)
	// 重复绑定同一文件不产生第二条。
	updated, err = svc.AddAttachment(ctx, dto.AddAttachmentCommand{
		Principal: driverPrincipal(), EventID: created.EventID, FileID: "F:1",
	})
	require.NoError(t, err)
	require.Len(t, updated.Attachments, 1)
}

// TestTaskIDMustBelongToDriver 校验任务引用由服务端核对，无任务时允许为空。
func TestTaskIDMustBelongToDriver(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{ids: []string{"TASK-1", "507f1f77bcf86cd799439011"}}, &fakeLocation{})
	ctx := context.Background()

	// 收车时无任务：允许为空。
	cmd := reportCmd("REQ-NOTASK")
	view, err := svc.Report(ctx, cmd)
	require.NoError(t, err)
	require.Empty(t, view.TaskID)

	// 本人当前任务：接受。
	cmd = reportCmd("REQ-OWN")
	cmd.TaskID = "TASK-1"
	_, err = svc.Report(ctx, cmd)
	require.NoError(t, err)

	// 他人任务：拒绝，且不创建事件。
	cmd = reportCmd("REQ-OTHER")
	cmd.TaskID = "TASK-9"
	_, err = svc.Report(ctx, cmd)
	require.Error(t, err)
	require.Equal(t, "TASK_NOT_OWNED", errorCode(err))
	require.Len(t, repo.events, 2)
}

// TestReportRejectsUnknownEventType 校验字典外的类型直接拒绝，不静默归类。
func TestReportRejectsUnknownEventType(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	cmd := reportCmd("REQ-TYPE")
	cmd.EventType = "made_up"
	_, err := svc.Report(context.Background(), cmd)
	require.Error(t, err)
	require.Equal(t, "INVALID_EVENT_TYPE", errorCode(err))
	require.Empty(t, repo.events)
}

// TestOutboxDoesNotBlockAcceptance 校验通知只是意图：受理先落，投递另走。
func TestOutboxDoesNotBlockAcceptance(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	view, err := svc.Report(context.Background(), reportCmd("REQ-OUTBOX"))
	require.NoError(t, err)
	require.NotEmpty(t, view.EventID)

	event := repo.byID[view.EventID]
	require.Len(t, event.Outbox, 1)
	require.Equal(t, entity.OutboxPending, event.Outbox[0].State)
	require.Equal(t, entity.OutboxKindSafetyReported, event.Outbox[0].Kind)
}
