package entity

import (
	"context"
	"time"
)

// ListenFilter 听单候选过滤条件。
type ListenFilter struct {
	Lat, Lng   float64
	CityCode   string
	Seats      int
	CarType    string
	ExcludeIDs []string
	Limit      int
}

// DriverRepository 司机档案仓储（含资金五桶原子操作）。
type DriverRepository interface {
	Create(ctx context.Context, driver *Driver) error
	Get(ctx context.Context, id string) (*Driver, error)
	// UpdateStatus 原子更新司机状态，version 为读到的版本号（乐观锁）。
	UpdateStatus(ctx context.Context, id, status string, version int64, updatedAt time.Time) error
	// UpdateMode 原子更新模式与模式版本。
	UpdateMode(ctx context.Context, id, mode string, version, modeVersion int64, updatedAt time.Time) error
	// UpdateAreaConfig 原子更新区域开关与片区。
	UpdateAreaConfig(ctx context.Context, id string, switchOn bool, areas []AreaBox, version int64, updatedAt time.Time) error
	// UpdateTaskBinding 原子更新活动任务绑定与忙碌状态（接单/释放运力）。
	UpdateTaskBinding(ctx context.Context, id, status, activeTaskID string, version int64, updatedAt time.Time) error
	// UpdateHeartbeat 更新心跳时间、坐标与展示字段（离线后收车由服务层判定）。
	UpdateHeartbeat(ctx context.Context, id string, lastHeartbeatAt time.Time, lat, lng float64, version int64, updatedAt time.Time) error
	// UpdateOfflineAfterTrip 设置"本单后收车"。
	UpdateOfflineAfterTrip(ctx context.Context, id string, on bool, version int64, updatedAt time.Time) error
	// FindListenCandidates 听单候选：listing+idle+资质+未被禁接单+区域+车型座位匹配，
	// 排序交由服务层按评分模型计算，这里保证返回确定性顺序（driver_no 升序）。
	FindListenCandidates(ctx context.Context, f ListenFilter) ([]*Driver, error)
	// FindIdleHeartbeatExpired 空闲且心跳超时的司机（用于自动收车）。
	FindIdleHeartbeatExpired(ctx context.Context, before time.Time, limit int) ([]*Driver, error)
	// MoveFunds 资金五桶原子转移（源桶余额不足返回 ErrInsufficientFunds）。
	MoveFunds(ctx context.Context, id, fromBucket, toBucket string, amountFen int64) error
	// DeductFunds 从指定桶扣款出账（提现打款 W→平台代付，桶内余额不足返回 ErrInsufficientFunds）。
	DeductFunds(ctx context.Context, id, bucket string, amountFen int64) error
	// AddFunds 外部资金入账到指定桶（如乘客支付 → 待结算 U）。
	AddFunds(ctx context.Context, id, bucket string, amountFen int64) error
	// AddMonthlyIncome 累加本月累计已结算收入（用于派单优先级）。
	AddMonthlyIncome(ctx context.Context, id string, amountFen int64) error
	// AddSuccessRate 更新成功率统计（简单移动平均）。
	AddSuccessRate(ctx context.Context, id string, sample bool) error
	// SetAcceptPermission 后台禁接单开关（A01 联动）。
	SetAcceptPermission(ctx context.Context, id string, canAccept bool) error
	// RecordRelease 记录退单（含责任方）。
	RecordRelease(ctx context.Context, id, party string, at time.Time) error
	// CountDriverReleasesToday 当日司机责任退单次数（排除乘客取消与平台责任取消）。
	CountDriverReleasesToday(ctx context.Context, driverID string, dayStart time.Time) (int64, error)
}

// HallOrderRepository 接单大厅订单仓储。所有状态流转都走原子条件更新，
// 保证"一个任务一个司机、一个司机一个活动任务"，并且唯一性由数据库唯一约束兜底。
type HallOrderRepository interface {
	Create(ctx context.Context, order *HallOrder) error
	Get(ctx context.Context, id string) (*HallOrder, error)
	GetByOrderID(ctx context.Context, orderID string) (*HallOrder, error)
	// Assign 原子邀请/抢占：WAITING 且未被占用（或邀请已超时）才能成功。
	Assign(ctx context.Context, orderID, driverID, driverName string, version int64, acceptDeadline time.Time, distanceMeters int64) (*HallOrder, error)
	// ConfirmAccept 确认接单：WAITING+本人+未过 accept_deadline → ACCEPTED。
	ConfirmAccept(ctx context.Context, orderID, driverID string, version int64) (*HallOrder, error)
	// ReleaseToWaiting 退单/超时回收：回到 WAITING 等待重派，记录责任方与排除名单。
	ReleaseToWaiting(ctx context.Context, orderID, releasedBy, responsibleParty, reason string) (*HallOrder, error)
	// Complete 送达结束：进入 ENDED（行程结束，与付款无关）。
	Complete(ctx context.Context, orderID, driverID string, version int64, distanceMeters, durationSec int64) (*HallOrder, error)
	// StartTrip 行程开始（由行程域/司机端调用）：ACCEPTED → IN_TRIP。
	StartTrip(ctx context.Context, orderID, driverID string) (*HallOrder, error)
	// Cancel 取消订单（乘客取消/平台责任）：→ CANCELLED。
	Cancel(ctx context.Context, orderID, responsibleParty, reason string) (*HallOrder, error)
	// OpenToGrabPool 转入抢单池曝光。
	OpenToGrabPool(ctx context.Context, orderID string) (*HallOrder, error)
	// ListPool 抢单大厅列表：WAITING + 已曝光 + 未超时。
	ListPool(ctx context.Context, driver *Driver, lat, lng float64, limit int) ([]*HallOrder, error)
	// FindPendingInviteExpired 邀请超时未完成的任务。
	FindPendingInviteExpired(ctx context.Context, now time.Time, limit int) ([]*HallOrder, error)
	// FindListeningTasks 处于听单窗口内且尚未派出的任务。
	FindListeningTasks(ctx context.Context, now time.Time, limit int) ([]*HallOrder, error)
	// FindWindowSwitchable 听单窗口结束、需要转抢单池的任务。
	FindWindowSwitchable(ctx context.Context, now time.Time, limit int) ([]*HallOrder, error)
	// FindDispatchExpired 超过总派单时限的任务（平台责任取消）。
	FindDispatchExpired(ctx context.Context, now time.Time, limit int) ([]*HallOrder, error)
	// ReleasePendingForDriver 释放司机名下待确认的推送任务（模式切换/预占超时）。
	ReleasePendingForDriver(ctx context.Context, driverID string) (int64, error)
	// SumEndedByDay 按行程结束日统计：完单量与乘客数（拼车两乘客算 2 单）。
	SumEndedByDay(ctx context.Context, driverID string, dayStart, dayEnd time.Time) (orders int64, passengers int64, err error)
}

// WithdrawCreateResult 幂等创建提现的结果。
type WithdrawCreateResult struct {
	Withdrawal *Withdrawal
	Duplicate  bool
}

// IncomeRepository 收入流水、资金分录与提现仓储。
type IncomeRepository interface {
	// CreateFlow 追加流水凭证（event_id 去重）。
	CreateFlow(ctx context.Context, flow *IncomeFlow) error
	GetFlowByEventID(ctx context.Context, eventID string) (*IncomeFlow, error)
	GetFlowByID(ctx context.Context, id string) (*IncomeFlow, error)
	GetFlowByKey(ctx context.Context, driverID, idempotencyKey string) (*IncomeFlow, error)
	ListFlows(ctx context.Context, driverID string, offset, limit int) ([]*IncomeFlow, int64, error)
	// SumByAccountingDate 按记账日汇总收入与退款（退款为负值）。
	SumByAccountingDate(ctx context.Context, driverID, dateKey string) (incomeFen, refundFen int64, err error)
	// ListPendingSettlement 待结算（U 桶）流水，供财务域确认结算。
	ListPendingSettlement(ctx context.Context, limit int) ([]*IncomeFlow, error)
	// MarkSettled 将待结算流水标记为已结算（U → A）。
	MarkSettled(ctx context.Context, flowIDs []string) error
	// CreateLedger 追加资金分录。
	CreateLedger(ctx context.Context, entry *LedgerEntry) error
	// CreateWithdraw 幂等创建提现申请。
	CreateWithdraw(ctx context.Context, w *Withdrawal) (*WithdrawCreateResult, error)
	GetWithdraw(ctx context.Context, id string) (*Withdrawal, error)
	// GetWithdrawByKey 按幂等键查询提现单（提现前预检，避免重复预占额度）。
	GetWithdrawByKey(ctx context.Context, driverID, idempotencyKey string) (*Withdrawal, error)
	// UpdateWithdraw 原子更新提现单（乐观锁由 version 校验）。
	UpdateWithdraw(ctx context.Context, w *Withdrawal) error
	ListWithdraws(ctx context.Context, driverID string, offset, limit int) ([]*Withdrawal, int64, error)
	// WithdrawTodayStats 当日已受理金额与次数（不含驳回）。
	WithdrawTodayStats(ctx context.Context, driverID string, dayStart time.Time) (amountFen int64, count int64, err error)
}

// SupportRepository 可靠事件表、消费去重表与幂等命令表。
type SupportRepository interface {
	// EnqueueEvent 写入待投递事件（event_id 唯一，天然幂等）。
	EnqueueEvent(ctx context.Context, event *OutboxEvent) error
	FetchPendingEvents(ctx context.Context, limit int) ([]*OutboxEvent, error)
	MarkPublished(ctx context.Context, eventID string) error
	MarkFailed(ctx context.Context, eventID, errMsg string) error
	// ClaimInbound 外部事件去重：已处理返回 false。
	ClaimInbound(ctx context.Context, eventID, eventType, consumer string) (bool, error)
	// ClaimCommand 幂等命令：已存在返回 false 与原结果。
	ClaimCommand(ctx context.Context, rec *CommandRecord) (bool, *CommandRecord, error)
	// EnsureIndexes 创建跨模块约束所需的唯一索引。
	EnsureIndexes(ctx context.Context) error
}

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
