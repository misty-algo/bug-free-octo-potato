package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"
)

// 申请主流程状态（D15）。聚合根的 state 始终跟随 activeRevision 的状态。
// draft 不是申请状态：只有 vehicle_drafts 存在时才叫草稿。
const (
	AppStatePending   = "pending"
	AppStateReviewing = "reviewing"
	AppStateApproved  = "approved"
	AppStateRejected  = "rejected"
	AppStateWithdrawn = "withdrawn"
)

// 受控动作。审核状态只能由这些命令推进，每次推进都追加审计记录。
const (
	ActionSubmit   = "submit"
	ActionClaim    = "claim"
	ActionTransfer = "transfer"
	ActionWithdraw = "withdraw"
	ActionApprove  = "approve"
	ActionReject   = "reject"
)

// 驳回原因码。分组与重置规则是技术补充，需与 A02 数据字典统一后替换此处常量。
const (
	RejectCodeBlurry      = "DOC_BLURRY"
	RejectCodeExpired     = "DOC_EXPIRED"
	RejectCodeMismatch    = "DOC_MISMATCH"
	RejectCodeMissing     = "DOC_MISSING"
	RejectCodeVehicleInfo = "VEHICLE_INFO_MISMATCH"
)

// 连续驳回计数的目标：证件按类型（与证件号无关），车辆按 vehicleId
// （由 VIN 派生），因此改证件号、换牌或撤回都无法绕过分组计数。
const (
	CounterTargetDoc     = "doc_type"
	CounterTargetVehicle = "vehicle"
)

const (
	// MaxRevisionsPerApplication 限制单申请版本链长度，避免单文档无限增长。
	MaxRevisionsPerApplication = 50
	// RepeatRejectThreshold 是连续驳回次数阈值，达到即打重点复核标记。
	RepeatRejectThreshold = 3
	RejectFlagRepeat      = "REPEAT_REJECT_3"
	// 内嵌通知 outbox 的状态：与业务命令同文档写入，投递后置 sent。
	OutboxPending = "pending"
	OutboxSent    = "sent"
	// OutboxKindQualification 是批准后广播的资格变更事件。
	OutboxKindQualification = "QualificationChanged"
	OutboxKindApplication   = "VehicleApplicationChanged"
	OutboxKindCounterSync   = "RejectionCounterSync"
)

// RevisionPayload 是提交时冻结的业务正文。写入后不提供任何修改入口，
// 被驳回的版本只能通过追加新 revision 重来。
type RevisionPayload struct {
	VehicleID    string   `bson:"vehicleId"`
	VinMask      string   `bson:"vinMask"`
	Plate        string   `bson:"plate"`
	CityCode     string   `bson:"cityCode"`
	ModelCode    string   `bson:"modelCode"`
	Seats        int      `bson:"seats"`
	Color        string   `bson:"color"`
	DocumentIDs  []string `bson:"documentIds"`
	FileIDs      []string `bson:"fileIds"`
	DraftID      string   `bson:"draftId"`
	DraftVersion int64    `bson:"draftVersion"`
}

// RejectItem 是逐条审核原因：哪个证件/车辆对象、什么问题。
type RejectItem struct {
	TargetType string `bson:"targetType"`
	TargetKey  string `bson:"targetKey"`
	Code       string `bson:"code"`
	Reason     string `bson:"reason"`
}

// Decision 是终态裁决记录，每 revision 至多一条。
type Decision struct {
	Action        string       `bson:"action"`
	ActorID       string       `bson:"actorId"`
	ReviewVersion int64        `bson:"reviewVersion"`
	Summary       string       `bson:"summary"`
	Items         []RejectItem `bson:"items"`
	At            time.Time    `bson:"at"`
}

// Assignment 记录领取与转派。转派只追加这里，不伪造另一次裁决。
type Assignment struct {
	Action        string    `bson:"action"`
	FromID        string    `bson:"fromId"`
	ToID          string    `bson:"toId"`
	Reason        string    `bson:"reason"`
	ReviewVersion int64     `bson:"reviewVersion"`
	At            time.Time `bson:"at"`
}

// AuditEntry 是受控命令的审计条目，随状态变更同一次写入，不会丢。
type AuditEntry struct {
	Seq           int64     `bson:"seq"`
	Action        string    `bson:"action"`
	ActorID       string    `bson:"actorId"`
	ActorRole     string    `bson:"actorRole"`
	ReviewVersion int64     `bson:"reviewVersion"`
	Detail        string    `bson:"detail"`
	At            time.Time `bson:"at"`
}

// OutboxEntry 是内嵌通知事件。与业务写入同一文档，投递侧按 id 幂等重放。
type OutboxEntry struct {
	ID        string     `bson:"id"`
	Kind      string     `bson:"kind"`
	State     string     `bson:"state"`
	Attempts  int        `bson:"attempts"`
	Payload   string     `bson:"payload"`
	CreatedAt time.Time  `bson:"createdAt"`
	SentAt    *time.Time `bson:"sentAt,omitempty"`
}

// ApplicationRevision 是一条不可变审核版本。
type ApplicationRevision struct {
	Revision      int64           `bson:"revision"`
	PayloadDigest string          `bson:"payloadDigest"`
	Payload       RevisionPayload `bson:"payload"`
	State         string          `bson:"state"`
	Decisions     []Decision      `bson:"decisions"`
	Assignments   []Assignment    `bson:"assignments"`
	CreatedAt     time.Time       `bson:"createdAt"`
	UpdatedAt     time.Time       `bson:"updatedAt"`
}

// VehicleApplication 是申请聚合根。standalone Mongo 没有多文档事务，
// 因此 revision、附件引用、裁决、审计、outbox 全部内嵌，
// 一次 UpdateOne 就是一次原子命令。
type VehicleApplication struct {
	ApplicationID  string `bson:"applicationId"`
	DriverID       string `bson:"driverId"`
	VehicleID      string `bson:"vehicleId"`
	State          string `bson:"state"`
	ActiveRevision int64  `bson:"activeRevision"`
	Revision       int64  `bson:"revision"`
	ReviewVersion  int64  `bson:"reviewVersion"`
	ReviewerID     string `bson:"reviewerId"`
	// ActiveSlot 只在申请处于 pending/reviewing 时等于 driverId，
	// 由唯一索引保证同一司机只有一个活动变更；进入终态即 $unset。
	ActiveSlot  string                `bson:"activeSlot,omitempty"`
	Revisions   []ApplicationRevision `bson:"revisions"`
	AuditTrail  []AuditEntry          `bson:"auditTrail"`
	ReviewFlags []string              `bson:"reviewFlags"`
	Outbox      []OutboxEntry         `bson:"outbox"`
	CreatedAt   time.Time             `bson:"createdAt"`
	UpdatedAt   time.Time             `bson:"updatedAt"`
}

// PayloadDigest 是冻结正文的指纹，用于证明无法复用被驳回版本改正文。
func PayloadDigest(p RevisionPayload) string {
	parts := []string{
		p.VehicleID, p.VinMask, p.Plate, p.CityCode, p.ModelCode,
		strconv.Itoa(p.Seats), p.Color,
		strings.Join(p.DocumentIDs, ","), strings.Join(p.FileIDs, ","),
		p.DraftID, strconv.FormatInt(p.DraftVersion, 10),
	}
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// RevisionByNumber 返回指定 revision，不存在时返回 nil。
func (a *VehicleApplication) RevisionByNumber(n int64) *ApplicationRevision {
	for i := range a.Revisions {
		if a.Revisions[i].Revision == n {
			return &a.Revisions[i]
		}
	}
	return nil
}

// ActiveRevisionView 返回当前生效的 revision 视图。
func (a *VehicleApplication) ActiveRevisionView() *ApplicationRevision {
	return a.RevisionByNumber(a.ActiveRevision)
}

// IsActive 判断申请是否仍占用"每司机一个活动变更"的名额。
func (a *VehicleApplication) IsActive() bool {
	return a.State == AppStatePending || a.State == AppStateReviewing
}

// NextAuditSeq 是下一条审计序号，追加式且不修改历史。
func (a *VehicleApplication) NextAuditSeq() int64 {
	return int64(len(a.AuditTrail)) + 1
}

// RejectionCounter 按 driver + 目标分组统计连续驳回。
// 通过后 consecutive 清零，历史保留在 History 里，不清空痕迹。
type RejectionCounter struct {
	DriverID    string         `bson:"driverId"`
	TargetType  string         `bson:"targetType"`
	TargetKey   string         `bson:"targetKey"`
	Consecutive int            `bson:"consecutive"`
	Flagged     bool           `bson:"flagged"`
	Processed   []string       `bson:"processed"`
	History     []CounterEvent `bson:"history"`
	UpdatedAt   time.Time      `bson:"updatedAt"`
}

// CounterEvent 是计数变更痕迹：驳回累计与通过清零都留一条。
type CounterEvent struct {
	Action        string    `bson:"action"`
	ApplicationID string    `bson:"applicationId"`
	Revision      int64     `bson:"revision"`
	Value         int       `bson:"value"`
	At            time.Time `bson:"at"`
}

// Ref 是幂等键：同一 revision 的驳回只累计一次。
func (c CounterEvent) Ref() string {
	return c.ApplicationID + ":" + strconv.FormatInt(c.Revision, 10)
}

// DriverAdmission 是司机准入/绑定的协调点，与任务分配共用同一把锁。
// 派单侧写入 ActiveTaskID 后，换车批准才能与分配互斥。
type DriverAdmission struct {
	DriverID         string     `bson:"driverId"`
	AdmissionVersion int64      `bson:"admissionVersion"`
	ActiveTaskID     string     `bson:"activeTaskId"`
	LockedBy         string     `bson:"lockedBy"`
	LockedAt         *time.Time `bson:"lockedAt"`
	UpdatedAt        time.Time  `bson:"updatedAt"`
}
