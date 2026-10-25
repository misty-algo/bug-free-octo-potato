package entity

import (
	"context"
	"strconv"
	"time"
)

// 位置证据的三态。UNKNOWN 不是错误，无定位也必须能求助。
const (
	PositionAvailable = "AVAILABLE"
	PositionLastKnown = "LAST_KNOWN"
	PositionUnknown   = "UNKNOWN"

	PositionSourceClient    = "client"
	PositionSourceLastKnown = "driver_last_location"
	PositionSourceNone      = "none"
)

// 状态常量见 safety_dict.go（RECEIVED/CLAIMED/PROCESSING/RESOLVED）。
// 通知意图的状态复用审核域既有常量：全仓只有一套 outbox 语义。
const (
	OutboxKindSafetyReported  = "SafetyEventReported"
	OutboxKindSafetyEscalated = "SafetyEventEscalated"
)

// 事件类型字典。字典本身受控，未知类型直接拒绝而不是静默归类。
var knownEventTypes = map[string]bool{
	"accident":   true,
	"medical":    true,
	"harassment": true,
	"fire":       true,
	"other":      true,
}

// IsKnownEventType 是字典校验入口。
func IsKnownEventType(t string) bool { return knownEventTypes[t] }

// 受控限制：附件与补充条数有上限，避免无限膨胀。
const (
	MaxSafetyAttachments = 6
	MaxSafetySupplements = 20
	MaxSafetyDescription = 500
)

// PositionFreshWindow 是本次采集位置被认定为 AVAILABLE 的最大滞留时间。
// 过旧的采集时间不作证据，回落到服务端最后已知位置。
const PositionFreshWindow = 2 * time.Minute

// Hotline 是受控发布的紧急电话指引条目。
type Hotline struct {
	Name   string
	Number string
	Note   string
}

// 仓库无动态配置下发机制，因此紧急电话以代码发布为版本：
// 客户端可离线缓存 HotlineVersion 对应的内容，但不得据此替代后台受理。
const HotlineVersion = "2026.10-demo"

// HotlinePublishedAt 是该版本生效时间。
var HotlinePublishedAt = time.Date(2026, 10, 1, 0, 0, 0, 0, Shanghai)

// PublishedHotlines 是已发布内容。课程环境明确不自动拨号。
var PublishedHotlines = []Hotline{
	{Name: "报警", Number: "110", Note: "人身安全受威胁或发生事故时优先报警"},
	{Name: "急救", Number: "120", Note: "人员受伤或突发疾病"},
	{Name: "交通事故", Number: "122", Note: "道路交通事故处理"},
	{Name: "平台安全值班", Number: "400-000-0000", Note: "演示用号码，仅作指引，不替代后台受理"},
}

// SafetyPosition 是位置证据。UNKNOWN 时坐标为 nil，绝不用 0,0 填充。
type SafetyPosition struct {
	Kind       string     `bson:"kind"`
	Lng        *float64   `bson:"lng"`
	Lat        *float64   `bson:"lat"`
	CapturedAt *time.Time `bson:"capturedAt"`
	Source     string     `bson:"source"`
}

// SafetyRaw 是受理瞬间冻结的原始内容。
// 写入后没有任何修改入口：后续补充只追加到 supplements，不回改这里。
type SafetyRaw struct {
	EventType       string         `bson:"eventType"`
	Description     string         `bson:"description"`
	TaskID          string         `bson:"taskId"`
	Position        SafetyPosition `bson:"position"`
	ClientRequestID string         `bson:"clientRequestId"`
	RequestHash     string         `bson:"requestHash"`
	ReceivedAt      time.Time      `bson:"receivedAt"`
}

type SafetySupplement struct {
	Seq      int64          `bson:"seq"`
	Kind     string         `bson:"kind"`
	Note     string         `bson:"note"`
	Position SafetyPosition `bson:"position"`
	ActorID  string         `bson:"actorId"`
	At       time.Time      `bson:"at"`
}

type SafetyOutbox struct {
	ID       string     `bson:"id"`
	Kind     string     `bson:"kind"`
	Payload  string     `bson:"payload"`
	State    string     `bson:"state"`
	Attempts int        `bson:"attempts"`
	SentAt   *time.Time `bson:"sentAt,omitempty"`
}

type SafetyAudit struct {
	Seq     int64     `bson:"seq"`
	Action  string    `bson:"action"`
	ActorID string    `bson:"actorId"`
	Role    string    `bson:"role"`
	Detail  string    `bson:"detail"`
	At      time.Time `bson:"at"`
}

// SafetyAction 是处置动作的时间线条目。原始求助与后续动作只追加：
// 客户端补充、后台处置、附证据与办结结果都记录 actor/时间/version/requestId。
type SafetyAction struct {
	Seq       int64     `bson:"seq"`
	Type      string    `bson:"type"`
	Note      string    `bson:"note"`
	Result    string    `bson:"result,omitempty"`
	Target    string    `bson:"target,omitempty"` // 转派目标处理人
	RefID     string    `bson:"refId,omitempty"`  // 附证据/附件的引用
	ActorID   string    `bson:"actorId"`
	ActorRole string    `bson:"actorRole"`
	RequestID string    `bson:"requestId"`
	Version   int64     `bson:"version"`
	At        time.Time `bson:"at"`
}

// SafetyEscalation 是升级事实。一旦写入即保留，不因事后被领取而删除。
type SafetyEscalation struct {
	Level int64     `bson:"level"`
	At    time.Time `bson:"at"`
}

// SafetyEvent 是安全事件聚合根。
// standalone Mongo 没有多文档事务，因此原始内容、附件、补充、动作、审计、
// 通知意图全部内嵌，一次条件更新就是一次原子处置。
type SafetyEvent struct {
	EventID  string `bson:"eventId"`
	DriverID string `bson:"driverId"`
	State    string `bson:"state"`
	// Version 是处置命令的乐观锁：受理为 1，每次处置命令 +1。
	// 升级不推进 Version，避免与并发领取互相干扰（见 safety_dict.go 说明）。
	Version     int64              `bson:"version"`
	Handler     string             `bson:"handler"`
	ClaimedAt   *time.Time         `bson:"claimedAt,omitempty"`
	Raw         SafetyRaw          `bson:"raw"`
	Attachments []string           `bson:"attachments"`
	Supplements []SafetySupplement `bson:"supplements"`
	Actions     []SafetyAction     `bson:"actions"`
	Escalations []SafetyEscalation `bson:"escalations"`
	AuditTrail  []SafetyAudit      `bson:"auditTrail"`
	Outbox      []SafetyOutbox     `bson:"outbox"`
	CreatedAt   time.Time          `bson:"createdAt"`
	UpdatedAt   time.Time          `bson:"updatedAt"`
}

// NextSupplementSeq 用已有条数推进序号，避免另存计数器。
func (e SafetyEvent) NextSupplementSeq() int64 {
	return int64(len(e.Supplements)) + 1
}

// NextAuditSeq 同上。
func (e SafetyEvent) NextAuditSeq() int64 {
	return int64(len(e.AuditTrail)) + 1
}

// NextActionSeq 同上。
func (e SafetyEvent) NextActionSeq() int64 {
	return int64(len(e.Actions)) + 1
}

// NextOutboxID 同上。
func (e SafetyEvent) NextOutboxID() string {
	return strconv.Itoa(len(e.Outbox) + 1)
}

// SafetyOutboxRef 定位一条待投递的意图。
type SafetyOutboxRef struct {
	EventID string
	Entry   SafetyOutbox
}

type SafetyRepository interface {
	// CreateReport 以 (driverId, clientRequestId) 为幂等键条件 upsert。
	// created=false 表示命中已有请求号，调用方据此比较内容哈希。
	CreateReport(ctx context.Context, event SafetyEvent) (created bool, err error)
	ByEventID(ctx context.Context, eventID string) (*SafetyEvent, error)
	ByRequest(ctx context.Context, driverID, clientRequestID string) (*SafetyEvent, error)
	AddSupplement(ctx context.Context, eventID string, sup SafetySupplement) (*SafetyEvent, error)
	AddAttachment(ctx context.Context, eventID, fileID string) (*SafetyEvent, error)
	Pending(ctx context.Context, driverID, state string, limit int) ([]SafetyEvent, error)
	PendingOutbox(ctx context.Context, limit int) ([]SafetyOutboxRef, error)
	MarkOutboxSent(ctx context.Context, eventID, entryID string, at time.Time) error
	BumpOutboxAttempt(ctx context.Context, eventID, entryID string) error

	// 以下处置命令共享 safety_event 行的原子边界：filter 同时带 version 与
	// 状态条件，匹配不到就返回 (nil, nil)，调用方据此判定版本冲突或状态不对。
	Claim(ctx context.Context, eventID string, expectedVersion int64, handler string, action SafetyAction, at time.Time) (*SafetyEvent, error)
	Start(ctx context.Context, eventID string, expectedVersion int64, handler string, action SafetyAction, at time.Time) (*SafetyEvent, error)
	Transfer(ctx context.Context, eventID string, expectedVersion int64, fromHandler, toHandler string, action SafetyAction, at time.Time) (*SafetyEvent, error)
	AddAction(ctx context.Context, eventID string, expectedVersion int64, action SafetyAction, at time.Time) (*SafetyEvent, error)
	Resolve(ctx context.Context, eventID string, expectedVersion int64, handler string, action SafetyAction, at time.Time) (*SafetyEvent, error)
	// EscalateCandidates 返回已超过升级阈值仍未领取的事件。
	EscalateCandidates(ctx context.Context, now time.Time, limit int) ([]SafetyEvent, error)
	// TryEscalate 带 state=RECEIVED 与档位不存在的条件写入；false 表示已被领取或已升级。
	TryEscalate(ctx context.Context, eventID string, escalation SafetyEscalation, outbox SafetyOutbox) (bool, error)
}

// DriverLocationReader 读司机的最后已知位置（由 D05 上报写入）。
type DriverLocationReader interface {
	LastKnownLocation(ctx context.Context, driverID string) (lng, lat float64, at *time.Time, err error)
}

// TracePointView 是轨迹证据的最小视图。
type TracePointView struct {
	Seq         int64
	CollectedAt time.Time
	Lng         float64
	Lat         float64
	Accuracy    float64
}

// MessageView 是联系消息证据。
type MessageView struct {
	Seq  int64
	From string
	Text string
	At   time.Time
}

// VirtualCallView 是虚拟号元数据。
type VirtualCallView struct {
	Number    string
	ExpiresAt time.Time
	RevokedAt *time.Time
	Simulated bool
}

// SafetyEvidenceReader 只读必要证据；实现直接读订单域集合（行程域集合）。
// 读取必须由调用方写审计：证据读取本身也是受控动作。
type SafetyEvidenceReader interface {
	TracePoints(ctx context.Context, taskNo string, from, to time.Time, limit int) ([]TracePointView, error)
	Messages(ctx context.Context, taskNo string, limit int) ([]MessageView, error)
	VirtualCall(ctx context.Context, taskNo string) (*VirtualCallView, error)
}
