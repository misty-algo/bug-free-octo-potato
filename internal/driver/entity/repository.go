package entity

import (
	"context"
	"time"
)

// VehicleRepository 是车辆资料、草稿、证件、审核版本与绑定的读写能力。
// 这里只提供底层能力，审核流程界面属 A02 域。
type VehicleRepository interface {
	DraftByDriver(ctx context.Context, driverID string) (*VehicleDraft, error)
	SaveDraft(ctx context.Context, draft *VehicleDraft) error
	DeleteDraft(ctx context.Context, driverID string) error

	ProfileByID(ctx context.Context, vehicleID string) (*VehicleProfile, error)
	UpsertProfile(ctx context.Context, profile *VehicleProfile) error

	PendingAuditByDriver(ctx context.Context, driverID string) (*VehicleAudit, error)
	NextRevision(ctx context.Context, applicationID string) (int64, error)
	InsertAudit(ctx context.Context, audit *VehicleAudit) error
	AuditByKey(ctx context.Context, applicationID string, revision int64) (*VehicleAudit, error)
	ListPendingAudits(ctx context.Context) ([]VehicleAudit, error)
	// SetAuditDecision 只接受 pending → 终态，保证终态裁决唯一。
	SetAuditDecision(ctx context.Context, applicationID string, revision int64, state, reason, decidedBy string, at time.Time) (bool, error)

	ActiveDocument(ctx context.Context, subjectID, docType string) (*DriverDocument, error)
	DocumentsByIDs(ctx context.Context, ids []string) ([]DriverDocument, error)
	DocumentsBySubject(ctx context.Context, subjectID string) ([]DriverDocument, error)
	SaveDocument(ctx context.Context, doc *DriverDocument) error
	// ArchiveDocument 把当前有效证件版本归档，历史独立保留。
	ArchiveDocument(ctx context.Context, subjectID, docType string) error

	ActiveBindingByDriver(ctx context.Context, driverID string) (*VehicleBinding, error)
	ActiveBindingByVehicle(ctx context.Context, vehicleID string) (*VehicleBinding, error)
	// SaveBinding 撞 uk_binding_driver/uk_binding_vehicle 时返回 repo.ErrDuplicateKey，
	// 调用方据此返回 VEHICLE_ALREADY_BOUND，而不是先查询再插入。
	SaveBinding(ctx context.Context, binding *VehicleBinding) error
	// DeleteBinding 用于批准失败时把已写入的新绑定回退。
	DeleteBinding(ctx context.Context, driverID string, bindingVersion int64) error
	ArchiveBinding(ctx context.Context, binding *VehicleBinding) error
	// DeleteHistory 配合 DeleteBinding 回退刚归档的历史行，避免同一版本两处存在。
	DeleteHistory(ctx context.Context, driverID string, bindingVersion int64) error
	HistoryByDriver(ctx context.Context, driverID string) ([]VehicleBinding, error)

	// UpdateDriverVehicleProjection 把当前有效车投影到 drivers。
	// 投影是只读消费方的缓存，不是第二份真相。
	UpdateDriverVehicleProjection(ctx context.Context, driverID string, snap VehicleSnapshot) error
}

// FileRepository 是文件元数据与删除任务的读写能力。
// 所有状态迁移都必须在同一行上做条件更新，这是无事务下的行锁等效替代。
type FileRepository interface {
	Insert(ctx context.Context, rec *FileRecord) error
	ByID(ctx context.Context, fileID string) (*FileRecord, error)
	ByKey(ctx context.Context, key string) (*FileRecord, error)
	// MarkUploaded 只在 UPLOADING 状态上成功，携带服务端实测字节数。
	MarkUploaded(ctx context.Context, fileID, provider, bucket, contentType, checksum string, size int64) (bool, error)
	Fail(ctx context.Context, fileID, reason string) error
	// Attach 抢占 READY 行并绑定到审核版本；已被清理抢占的行不会被绑定。
	Attach(ctx context.Context, ownerID, attachedTo string, ids []string) (int64, error)
	// Release 在提交落库失败时把本次绑定退回 READY，不留下指向不存在版本的孤儿。
	Release(ctx context.Context, attachedTo string) (int64, error)
	CountByOwnerPurpose(ctx context.Context, ownerID, attachedTo string) (int64, error)
	ListByIDs(ctx context.Context, ids []string) ([]FileRecord, error)
	// ClaimStale 把暂存到期且未被引用的 READY 行抢占为 DELETING。
	ClaimStale(ctx context.Context, now time.Time, limit int64) ([]FileRecord, error)
	InsertDeleteTask(ctx context.Context, task *FileDeleteTask) error
	PendingDeleteTasks(ctx context.Context, limit int64) ([]FileDeleteTask, error)
	BumpDeleteAttempt(ctx context.Context, taskID string) error
	MarkDeleteTaskDone(ctx context.Context, taskID string, at time.Time) error
	MarkFileDeleted(ctx context.Context, fileID string, at time.Time) error
}
