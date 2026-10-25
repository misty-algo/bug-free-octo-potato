package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
)

// fakeReminderRepo 用内存实现模拟唯一索引与租约领取。
type fakeReminderRepo struct {
	tasks   map[string]entity.ReminderTask
	order   []string
	notices map[string]string
	merged  map[string]string
	sent    map[string]string
}

func newFakeReminderRepo() *fakeReminderRepo {
	return &fakeReminderRepo{
		tasks:   map[string]entity.ReminderTask{},
		notices: map[string]string{},
		merged:  map[string]string{},
		sent:    map[string]string{},
	}
}

func (f *fakeReminderRepo) Upsert(_ context.Context, task entity.ReminderTask) error {
	if _, ok := f.tasks[task.SlotKey()]; ok {
		return nil
	}
	f.tasks[task.SlotKey()] = task
	f.order = append(f.order, task.SlotKey())
	return nil
}

func (f *fakeReminderRepo) CancelByDocument(_ context.Context, documentID string, exceptVersion int64, _ time.Time) error {
	for k, t := range f.tasks {
		if t.DocumentID != documentID || t.DocumentVersion == exceptVersion {
			continue
		}
		if t.State == entity.ReminderPending || t.State == entity.ReminderFailed {
			t.State = entity.ReminderCancelled
			f.tasks[k] = t
		}
	}
	return nil
}

func (f *fakeReminderRepo) ClaimDue(_ context.Context, now time.Time, limit int, owner string) ([]entity.ReminderTask, error) {
	out := make([]entity.ReminderTask, 0, limit)
	for _, k := range f.order {
		t := f.tasks[k]
		if t.State != entity.ReminderPending && t.State != entity.ReminderFailed {
			continue
		}
		if t.DueAt.After(now) || t.Attempts >= entity.ReminderMaxAttempts {
			continue
		}
		if t.LeaseUntil != nil && t.LeaseUntil.After(now) {
			continue
		}
		t.State = entity.ReminderLeased
		t.LeaseOwner = owner
		lease := now.Add(entity.ReminderLeaseTTL)
		t.LeaseUntil = &lease
		t.Attempts++
		f.tasks[k] = t
		out = append(out, t)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (f *fakeReminderRepo) MarkSent(_ context.Context, taskID, noticeID string, _ time.Time) error {
	f.sent[taskID] = noticeID
	return f.setState(taskID, entity.ReminderSent)
}

func (f *fakeReminderRepo) MarkMerged(_ context.Context, taskID, into string, _ time.Time) error {
	f.merged[taskID] = into
	return f.setState(taskID, entity.ReminderMerged)
}

func (f *fakeReminderRepo) MarkCancelled(_ context.Context, taskID string, _ time.Time) error {
	return f.setState(taskID, entity.ReminderCancelled)
}

func (f *fakeReminderRepo) MarkFailed(_ context.Context, taskID string, retryAt time.Time) error {
	for k, t := range f.tasks {
		if t.TaskID == taskID {
			t.DueAt = retryAt
			f.tasks[k] = t
		}
	}
	return f.setState(taskID, entity.ReminderFailed)
}

func (f *fakeReminderRepo) InsertNotice(_ context.Context, eventID, _, _, detail string, _ time.Time) (bool, error) {
	if _, ok := f.notices[eventID]; ok {
		return false, nil
	}
	f.notices[eventID] = detail
	return true, nil
}

func (f *fakeReminderRepo) setState(taskID, state string) error {
	for k, t := range f.tasks {
		if t.TaskID == taskID {
			t.State = state
			f.tasks[k] = t
		}
	}
	return nil
}

type fakeDocLoader struct{ docs []entity.DriverDocument }

func (l *fakeDocLoader) DocumentsBySubject(context.Context, string) ([]entity.DriverDocument, error) {
	return l.docs, nil
}

func testDoc(driverID string, version int64, expiry time.Time) *entity.DriverDocument {
	return &entity.DriverDocument{
		DocumentID: "DOC:1", SubjectType: "driver", SubjectID: driverID,
		DocType: entity.DocTypeDrivingLicense, ExpiresAt: &expiry,
		ExpiresOn: entity.LastValidDate(expiry),
		Version:   version, State: entity.DocStateActive,
	}
}

// TestReminderLevelsUseCalendarDays 校验档位按自然日计算，且不重复生成。
func TestReminderLevelsUseCalendarDays(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	repo := newFakeReminderRepo()
	doc := testDoc("D1", 1, expiry)
	svc := NewReminderService(repo, &fakeDocLoader{docs: []entity.DriverDocument{*doc}}, clock.Real{}, zap.NewNop())

	now := time.Date(2026, 10, 1, 0, 0, 0, 0, entity.Shanghai)
	require.NoError(t, svc.EnsureForDocument(context.Background(), doc, now))
	// 再次调用应被档位唯一键挡住，不生成第二条。
	require.NoError(t, svc.EnsureForDocument(context.Background(), doc, now))
	require.Len(t, repo.tasks, 4, "30/7/1/expired 四个档位")

	due := map[int]string{}
	for _, task := range repo.tasks {
		due[task.Level] = task.DueAt.In(entity.Shanghai).Format("2006-01-02 15:04")
	}
	require.Equal(t, "2026-10-21 09:00", due[30], "最后有效日 11-20 减 30 个日历天")
	require.Equal(t, "2026-11-13 09:00", due[7])
	require.Equal(t, "2026-11-19 09:00", due[1])
	require.Equal(t, "2026-11-21 00:00", due[0], "expired 档在失效时刻触发")
}

// TestTickMergesLevelsIntoOne 校验停机错过多档时只发一条最紧迫提醒。
func TestTickMergesLevelsIntoOne(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	repo := newFakeReminderRepo()
	doc := testDoc("D1", 1, expiry)
	svc := NewReminderService(repo, &fakeDocLoader{docs: []entity.DriverDocument{*doc}}, clock.Real{}, zap.NewNop())
	ctx := context.Background()

	now := time.Date(2026, 10, 1, 0, 0, 0, 0, entity.Shanghai)
	require.NoError(t, svc.EnsureForDocument(ctx, doc, now))
	// 模拟停机：30/7/1 三档同时错过。
	missed := time.Date(2026, 11, 19, 10, 0, 0, 0, entity.Shanghai)
	for k, task := range repo.tasks {
		if task.Level > 0 {
			task.DueAt = missed
			repo.tasks[k] = task
		}
	}
	res, err := svc.Tick(ctx, missed, 50)
	require.NoError(t, err)
	require.Equal(t, int32(3), res.Claimed)
	require.Equal(t, int32(1), res.Sent, "只发当前最紧迫的一条")
	require.Equal(t, int32(2), res.Merged)
	require.Len(t, repo.notices, 1)
	require.Len(t, repo.merged, 2)
}

// TestTickDoesNotReplayStaleLevels 校验已过期时不补发 30/7/1 的过时提醒。
func TestTickDoesNotReplayStaleLevels(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	repo := newFakeReminderRepo()
	doc := testDoc("D1", 1, expiry)
	svc := NewReminderService(repo, &fakeDocLoader{docs: []entity.DriverDocument{*doc}}, clock.Real{}, zap.NewNop())
	ctx := context.Background()

	require.NoError(t, svc.EnsureForDocument(ctx, doc, time.Date(2026, 10, 1, 0, 0, 0, 0, entity.Shanghai)))
	// 到期时刻四个档位同时到期：只发到期限制通知，30/7/1 合并进它。
	expiredAt := expiry
	res, err := svc.Tick(ctx, expiredAt, 50)
	require.NoError(t, err)
	require.Equal(t, int32(4), res.Claimed)
	require.Equal(t, int32(1), res.Sent, "只发一条到期限制通知")
	require.Equal(t, int32(3), res.Merged)

	// 停机补跑场景：30 天档从未被处理，此刻已过期 → 取消而不是补发过时提醒。
	for k, task := range repo.tasks {
		if task.Level == 30 {
			task.State = entity.ReminderPending
			task.LeaseUntil = nil
			task.Attempts = 0
			repo.tasks[k] = task
		}
	}
	res, err = svc.Tick(ctx, expiredAt.Add(time.Hour), 50)
	require.NoError(t, err)
	require.Equal(t, int32(1), res.Claimed)
	require.Equal(t, int32(1), res.Cancelled, "过时的提前提醒不再补发")
	require.Equal(t, int32(0), res.Sent)
	require.Len(t, repo.notices, 1)
}

// TestTickRechecksDocumentVersion 校验发送前重验：证件换版本后旧档位取消。
func TestTickRechecksDocumentVersion(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	repo := newFakeReminderRepo()
	doc := testDoc("D1", 1, expiry)
	loader := &fakeDocLoader{docs: []entity.DriverDocument{*doc}}
	svc := NewReminderService(repo, loader, clock.Real{}, zap.NewNop())
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, entity.Shanghai)
	require.NoError(t, svc.EnsureForDocument(ctx, doc, now))

	// 新证生效：旧版本未发档位作废。
	newExpiry := expiry.AddDate(1, 0, 0)
	renewed := testDoc("D1", 2, newExpiry)
	require.NoError(t, svc.EnsureForDocument(ctx, renewed, now))
	loader.docs = []entity.DriverDocument{*renewed}
	for _, task := range repo.tasks {
		if task.DocumentVersion == 1 && task.Level == 30 {
			require.Equal(t, entity.ReminderCancelled, task.State, "旧版本档位取消")
		}
	}
	// 旧版本不再发送；推进到新版本的 30 天档后正常发一条。
	res, err := svc.Tick(ctx, now, 50)
	require.NoError(t, err)
	require.Equal(t, int32(0), res.Sent, "只按当前有效版本发")

	res, err = svc.Tick(ctx, entity.ReminderDueAt(newExpiry, 30), 50)
	require.NoError(t, err)
	require.Equal(t, int32(1), res.Sent)
	require.Len(t, repo.notices, 1)
}

// TestTickIsIdempotent 校验重启补跑不会给司机生成第二条相同通知。
func TestTickIsIdempotent(t *testing.T) {
	expiry, err := entity.ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	repo := newFakeReminderRepo()
	doc := testDoc("D1", 1, expiry)
	svc := NewReminderService(repo, &fakeDocLoader{docs: []entity.DriverDocument{*doc}}, clock.Real{}, zap.NewNop())
	ctx := context.Background()
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, entity.Shanghai)
	require.NoError(t, svc.EnsureForDocument(ctx, doc, time.Date(2026, 10, 1, 0, 0, 0, 0, entity.Shanghai)))

	first, err := svc.Tick(ctx, now, 50)
	require.NoError(t, err)
	require.Equal(t, int32(1), first.Sent)
	second, err := svc.Tick(ctx, now, 50)
	require.NoError(t, err)
	require.Equal(t, int32(0), second.Claimed)
	require.Len(t, repo.notices, 1, "通知按 taskId 幂等")
}
