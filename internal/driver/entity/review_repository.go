package entity

import (
	"context"
	"errors"
	"time"
)

// ErrDuplicateKey 表示唯一约束已被占用：并发下的唯一真相是索引，不是先查询再插入。
var ErrDuplicateKey = errors.New("driver: duplicate key is taken")

// EventWriter 写可靠事件日志（eventId 唯一去重），供下游消费方拉取。
type EventWriter interface {
	AppendEvent(ctx context.Context, eventID, name, kind, detail string, at time.Time) error
}

// AppendRevisionInput 是"提交/追加 revision"这条命令的完整输入。
// 并发凭据是 ExpectedRevision + ExpectedReviewVersion，两者必须同时匹配。
type AppendRevisionInput struct {
	ApplicationID         string
	DriverID              string
	ExpectedRevision      int64
	ExpectedReviewVersion int64
	Revision              ApplicationRevision
	Entry                 AuditEntry
	Outbox                []OutboxEntry
}

// TransitionInput 是审核命令的并发凭据：当前 revision 与 review 代数必须同时匹配，
// 旧审核请求无法操作新版本。
type TransitionInput struct {
	ApplicationID string
	Revision      int64
	ReviewVersion int64
	ActorID       string
	ActorRole     string
	At            time.Time
	Detail        string
	Entry         AuditEntry
	Outbox        []OutboxEntry
}

// DecideInput 是一次终态裁决。ToState 只能是 approved 或 rejected。
type DecideInput struct {
	TransitionInput
	ToState  string
	Decision Decision
	Flags    []string
}

// OutboxRef 指向某个申请文档内嵌的一条待投递事件。
type OutboxRef struct {
	ApplicationID string
	DriverID      string
	Entry         OutboxEntry
}

// ApplicationRepository 是申请聚合的读写能力。所有状态迁移都是
// 单文档条件更新：MatchedCount==1 才算抢到，这是无事务下的行锁等效。
type ApplicationRepository interface {
	// Create 只在 applicationId 与 activeSlot 都未被占用时成功。
	Create(ctx context.Context, app *VehicleApplication) error
	ByID(ctx context.Context, applicationID string) (*VehicleApplication, error)
	ActiveByDriver(ctx context.Context, driverID string) (*VehicleApplication, error)
	LatestByDriver(ctx context.Context, driverID string) (*VehicleApplication, error)
	AppendRevision(ctx context.Context, in AppendRevisionInput) (bool, error)
	Claim(ctx context.Context, in TransitionInput) (bool, error)
	Transfer(ctx context.Context, in TransitionInput, to, reason string) (bool, error)
	Withdraw(ctx context.Context, in TransitionInput) (bool, error)
	Decide(ctx context.Context, in DecideInput) (bool, error)
	ListByState(ctx context.Context, states []string) ([]VehicleApplication, error)
	PendingOutbox(ctx context.Context, limit int64) ([]OutboxRef, error)
	MarkOutboxSent(ctx context.Context, applicationID, outboxID string, at time.Time) (bool, error)
	BumpOutboxAttempt(ctx context.Context, applicationID, outboxID string) error
}

// RejectionCounterRepository 维护"同证连续驳回"计数。
// 计数键与证件号无关，改号、撤回、新建申请都无法绕过。
type RejectionCounterRepository interface {
	ByDriver(ctx context.Context, driverID string) ([]RejectionCounter, error)
	// ApplyReject 对同一 revision 幂等：重复调用不会重复累计。
	ApplyReject(ctx context.Context, driverID string, items []RejectItem, applicationID string, revision int64, at time.Time) error
	// Clear 在批准后清零，保留历史痕迹。
	Clear(ctx context.Context, driverID, targetType, targetKey, applicationID string, revision int64, at time.Time) error
}

// AdmissionRepository 是准入/绑定协调点，与任务分配共用同一把锁。
type AdmissionRepository interface {
	ByDriver(ctx context.Context, driverID string) (*DriverAdmission, error)
	// TryLock 仅在没有活动任务且未被他人持锁时成功。
	TryLock(ctx context.Context, driverID, lockedBy string, at time.Time) (bool, error)
	Unlock(ctx context.Context, driverID, lockedBy string) (bool, error)
	BumpVersion(ctx context.Context, driverID string, at time.Time) (bool, error)
	// SetActiveTask 由派单域调用；本域只读它做换车前的互斥检查。
	SetActiveTask(ctx context.Context, driverID, taskID string, at time.Time) (bool, error)
}

// TaskReader 判断司机是否有未结束的任务。派单域尚未提供接口，
// 当前实现对 dispatch_tasks 做只读检查（见 repo.DispatchTaskReader）。
type TaskReader interface {
	HasActiveTask(ctx context.Context, driverID string) (bool, error)
	// ActiveTaskIDs 返回本人当前活动的任务标识，用于核对求助引用的 task_id。
	ActiveTaskIDs(ctx context.Context, driverID string) ([]string, error)
}
