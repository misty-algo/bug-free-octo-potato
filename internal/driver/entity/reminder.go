package entity

import (
	"context"
	"strconv"
	"time"
)

// 提醒任务的档位：Level 越小越紧迫，expired 用 0。
// 合并时取最小 Level 一条发送，其余标记 merged。
const (
	ReminderLevelExpired = 0
	ReminderLevel1Day    = 1
	ReminderLevel7Day    = 7
	ReminderLevel30Day   = 30
)

// BeforeExpiryLevels 是需要提前提醒的档位（日历天）。
var BeforeExpiryLevels = []int{ReminderLevel30Day, ReminderLevel7Day, ReminderLevel1Day}

const (
	ReminderKindBefore  = "before"
	ReminderKindExpired = "expired"
)

// 任务状态。leased 靠租约回收，进程重启后自然补跑。
const (
	ReminderPending   = "pending"
	ReminderLeased    = "leased"
	ReminderSent      = "sent"
	ReminderMerged    = "merged"
	ReminderCancelled = "cancelled"
	ReminderFailed    = "failed"
)

const (
	// ReminderLeaseTTL 是单次领取的租约时长，超时后可被其他进程重领。
	ReminderLeaseTTL = 2 * time.Minute
	// ReminderMaxAttempts 超过后转 failed 终态，不再阻塞后续档位。
	ReminderMaxAttempts = 5
	// NoticeKindDocumentExpiring 是站内通知类型。
	NoticeKindDocumentExpiring = "document_expiring"
	NoticeKindDocumentExpired  = "document_expired"
)

// ReminderTask 是一条到期提醒任务。
// (driverId, documentId, documentVersion, level) 唯一：同一证件版本每档只生成一次。
type ReminderTask struct {
	TaskID          string     `bson:"taskId"`
	DriverID        string     `bson:"driverId"`
	DocumentID      string     `bson:"documentId"`
	DocType         string     `bson:"docType"`
	DocumentVersion int64      `bson:"documentVersion"`
	Level           int        `bson:"level"`
	Kind            string     `bson:"kind"`
	ExpiresAt       time.Time  `bson:"expiresAt"`
	DueAt           time.Time  `bson:"dueAt"`
	State           string     `bson:"state"`
	LeaseOwner      string     `bson:"leaseOwner"`
	LeaseUntil      *time.Time `bson:"leaseUntil"`
	Attempts        int        `bson:"attempts"`
	MergedInto      string     `bson:"mergedInto"`
	NoticeID        string     `bson:"noticeId"`
	NotifiedAt      *time.Time `bson:"notifiedAt"`
	CreatedAt       time.Time  `bson:"createdAt"`
	UpdatedAt       time.Time  `bson:"updatedAt"`
}

// SlotKey 是幂等键，与唯一索引保持一致。
func (t ReminderTask) SlotKey() string {
	return t.DriverID + "|" + t.DocumentID + "|" + strconv.FormatInt(t.DocumentVersion, 10) +
		"|" + strconv.Itoa(t.Level)
}

// NoticeIDFor 让通知写入天然幂等：同一任务重复投递不会产生第二条。
func NoticeIDFor(taskID string) string {
	return "N:" + taskID
}

// ReminderRepository 只做条件更新，档位判定与合并规则在 service。
type ReminderRepository interface {
	Upsert(ctx context.Context, task ReminderTask) error
	// CancelByDocument 让旧证件版本未发的提醒失效，已发通知保留。
	CancelByDocument(ctx context.Context, documentID string, exceptVersion int64, at time.Time) error
	// ClaimDue 用租约领取到期任务：重启后租约过期可被重新领取。
	ClaimDue(ctx context.Context, now time.Time, limit int, owner string) ([]ReminderTask, error)
	MarkSent(ctx context.Context, taskID, noticeID string, at time.Time) error
	MarkMerged(ctx context.Context, taskID, into string, at time.Time) error
	MarkCancelled(ctx context.Context, taskID string, at time.Time) error
	// MarkFailed 把任务退回可重试状态，并把 dueAt 后移做退避。
	MarkFailed(ctx context.Context, taskID string, retryAt time.Time) error
	// InsertNotice 写站内通知，eventID 唯一保证不重复；已存在返回 false。
	InsertNotice(ctx context.Context, eventID, name, driverID, detail string, at time.Time) (bool, error)
}
