package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
)

// DocumentReminder 是证件变更时维护提醒档位的最小接口。
// 实现失败只记日志，不影响证件保存本身（提醒不是有效期检查的前置条件）。
type DocumentReminder interface {
	EnsureForDocument(ctx context.Context, doc *entity.DriverDocument, now time.Time) error
}

// documentLoader 是发送前重验需要的窄能力，便于用假实现覆盖合并与重验分支。
type documentLoader interface {
	DocumentsBySubject(ctx context.Context, subjectID string) ([]entity.DriverDocument, error)
}

// ReminderService 只做三件事：生成档位、按租约领取、发送前重验。
type ReminderService struct {
	tasks    entity.ReminderRepository
	vehicles documentLoader
	clock    clock.Clock
	log      *zap.Logger
	owner    string
}

func NewReminderService(tasks entity.ReminderRepository, vehicles documentLoader, clk clock.Clock, log *zap.Logger) *ReminderService {
	if log == nil {
		log = zap.NewNop()
	}
	return &ReminderService{
		tasks: tasks, vehicles: vehicles, clock: clk, log: log,
		owner: "worker:" + uuid.NewString()[:8],
	}
}

var _ DocumentReminder = (*ReminderService)(nil)

// EnsureForDocument 为新版本生成 30/7/1/expired 四档，并作废旧版本未发提醒。
// 档位由唯一索引 (driverId, documentId, documentVersion, level) 保证只生成一次。
func (s *ReminderService) EnsureForDocument(ctx context.Context, doc *entity.DriverDocument, now time.Time) error {
	if err := s.tasks.CancelByDocument(ctx, doc.DocumentID, doc.Version, now); err != nil {
		return err
	}
	expiry := doc.Expiry()
	if expiry == nil || doc.State != entity.DocStateActive {
		return nil
	}
	for _, level := range entity.BeforeExpiryLevels {
		if err := s.tasks.Upsert(ctx, s.newTask(doc, *expiry, level, entity.ReminderKindBefore, now)); err != nil {
			return err
		}
	}
	return s.tasks.Upsert(ctx, s.newTask(doc, *expiry, entity.ReminderLevelExpired, entity.ReminderKindExpired, now))
}

func (s *ReminderService) newTask(doc *entity.DriverDocument, expiry time.Time, level int, kind string, now time.Time) entity.ReminderTask {
	dueAt := expiry
	if kind == entity.ReminderKindBefore {
		dueAt = entity.ReminderDueAt(expiry, level)
	}
	return entity.ReminderTask{
		TaskID:          fmt.Sprintf("RT:%s:%d:%d", doc.DocumentID, doc.Version, level),
		DriverID:        doc.SubjectID,
		DocumentID:      doc.DocumentID,
		DocType:         doc.DocType,
		DocumentVersion: doc.Version,
		Level:           level,
		Kind:            kind,
		ExpiresAt:       expiry,
		DueAt:           dueAt,
		State:           entity.ReminderPending,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// Tick 领取到期任务并发送。now 允许传入模拟时间用于演练。
func (s *ReminderService) Tick(ctx context.Context, now time.Time, limit int) (*dto.ReminderTickResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	tasks, err := s.tasks.ClaimDue(ctx, now, limit, s.owner)
	if err != nil {
		return nil, fmt.Errorf("claim reminders: %w", err)
	}
	res := &dto.ReminderTickResult{Claimed: int32(len(tasks))}

	// 同一证件版本的多档一起到达时合并成一条最紧迫提醒。
	groups := make(map[string][]entity.ReminderTask, len(tasks))
	order := make([]string, 0, len(tasks))
	for _, task := range tasks {
		key := fmt.Sprintf("%s|%d", task.DocumentID, task.DocumentVersion)
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], task)
	}

	for _, key := range order {
		group := groups[key]
		sort.Slice(group, func(i, j int) bool { return group[i].Level < group[j].Level })
		primary := group[0]

		doc, err := s.documentOf(ctx, primary)
		if err != nil {
			s.log.Error("load document for reminder failed",
				zap.String("document_id", primary.DocumentID), zap.Error(err))
			s.failGroup(ctx, group, now, res)
			continue
		}
		expiry := expiryOf(doc, primary)
		// 发送前重验：证件已换版本、已归档或已无有效期，就不发。
		if doc == nil || doc.Version != primary.DocumentVersion ||
			doc.State != entity.DocStateActive || expiry == nil {
			s.cancelGroup(ctx, group, now, res)
			continue
		}
		// 已经过期就不再补发 30/7/1 天的过时提醒，只发到期限制通知。
		if entity.IsExpired(now, *expiry) && primary.Kind != entity.ReminderKindExpired {
			s.cancelGroup(ctx, group, now, res)
			continue
		}

		for _, task := range group[1:] {
			if err := s.tasks.MarkMerged(ctx, task.TaskID, primary.TaskID, now); err != nil {
				s.log.Error("mark reminder merged failed", zap.String("task_id", task.TaskID), zap.Error(err))
				continue
			}
			res.Merged++
		}

		name := entity.NoticeKindDocumentExpiring
		if primary.Kind == entity.ReminderKindExpired {
			name = entity.NoticeKindDocumentExpired
		}
		detail := fmt.Sprintf(
			`{"driver_id":%q,"document_id":%q,"doc_type":%q,"expires_on":%q,"level":%d,"merged":%d}`,
			primary.DriverID, primary.DocumentID, primary.DocType,
			entity.LastValidDate(*expiry), primary.Level, len(group)-1)
		noticeID := entity.NoticeIDFor(primary.TaskID)
		// eventId 唯一：重启补跑不会给司机生成第二条相同通知。
		if _, err := s.tasks.InsertNotice(ctx, noticeID, name, primary.DriverID, detail, now); err != nil {
			s.log.Error("insert document notice failed",
				zap.String("task_id", primary.TaskID), zap.Error(err))
			s.failTask(ctx, primary, now, res)
			continue
		}
		if err := s.tasks.MarkSent(ctx, primary.TaskID, noticeID, now); err != nil {
			s.log.Error("mark reminder sent failed", zap.String("task_id", primary.TaskID), zap.Error(err))
			res.Failed++
			continue
		}
		res.Sent++
	}
	return res, nil
}

func (s *ReminderService) documentOf(ctx context.Context, task entity.ReminderTask) (*entity.DriverDocument, error) {
	docs, err := s.vehicles.DocumentsBySubject(ctx, task.DriverID)
	if err != nil {
		return nil, err
	}
	for i := range docs {
		if docs[i].DocumentID == task.DocumentID {
			return &docs[i], nil
		}
	}
	return nil, nil
}

func expiryOf(doc *entity.DriverDocument, task entity.ReminderTask) *time.Time {
	if doc == nil {
		return nil
	}
	if expiry := doc.Expiry(); expiry != nil {
		return expiry
	}
	return &task.ExpiresAt
}

func (s *ReminderService) cancelGroup(ctx context.Context, group []entity.ReminderTask, now time.Time, res *dto.ReminderTickResult) {
	for _, task := range group {
		if err := s.tasks.MarkCancelled(ctx, task.TaskID, now); err != nil {
			s.log.Error("cancel reminder failed", zap.String("task_id", task.TaskID), zap.Error(err))
			continue
		}
		res.Cancelled++
	}
}

func (s *ReminderService) failGroup(ctx context.Context, group []entity.ReminderTask, now time.Time, res *dto.ReminderTickResult) {
	for _, task := range group {
		s.failTask(ctx, task, now, res)
	}
}

// failTask 用 attempts 做线性退避，避免失败任务在下一轮立刻重试打爆日志。
func (s *ReminderService) failTask(ctx context.Context, task entity.ReminderTask, now time.Time, res *dto.ReminderTickResult) {
	retryAt := now.Add(time.Duration(task.Attempts) * time.Minute)
	if err := s.tasks.MarkFailed(ctx, task.TaskID, retryAt); err != nil {
		s.log.Error("mark reminder failed", zap.String("task_id", task.TaskID), zap.Error(err))
	}
	res.Failed++
}
