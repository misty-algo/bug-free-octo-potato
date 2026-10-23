package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// ReportEventCommand 是核心求助。位置、描述、任务引用全部可选，
// 缺失不会阻塞受理。
type ReportEventCommand struct {
	Principal       authn.Principal
	ClientRequestID string
	EventType       string
	Description     string
	TaskID          string
	// HasPosition 区分"未采集定位"与"坐标恰好是 0,0"。
	HasPosition bool
	Lng         float64
	Lat         float64
	CapturedAt  *time.Time
}

type ByRequestCommand struct {
	Principal       authn.Principal
	ClientRequestID string
}

// AddSupplementCommand 是追加动作：只增不改，不覆盖原始位置证据。
type AddSupplementCommand struct {
	Principal   authn.Principal
	EventID     string
	Kind        string
	Note        string
	HasPosition bool
	Lng         float64
	Lat         float64
	CapturedAt  *time.Time
}

type AddAttachmentCommand struct {
	Principal authn.Principal
	EventID   string
	FileID    string
}

type PendingEventsCommand struct {
	Principal authn.Principal
	DriverID  string
	State     string
	Limit     int
}

// SafetyClaimCommand 领取事件：RECEIVED → CLAIMED，仅一个处理员成功。
type SafetyClaimCommand struct {
	Principal       authn.Principal
	EventID         string
	ExpectedVersion int64
	RequestID       string
}

// SafetyStartCommand 实际开始处理：CLAIMED → PROCESSING。
type SafetyStartCommand struct {
	Principal       authn.Principal
	EventID         string
	ExpectedVersion int64
	RequestID       string
}

// SafetyTransferCommand 转派：写原因与交接动作，不覆盖原受理/领取事实。
type SafetyTransferCommand struct {
	Principal       authn.Principal
	EventID         string
	ExpectedVersion int64
	To              string
	Reason          string
	RequestID       string
}

// SafetyAddActionCommand 追加处置：保持状态，只增不改。
type SafetyAddActionCommand struct {
	Principal       authn.Principal
	EventID         string
	ExpectedVersion int64
	Kind            string
	Note            string
	RefID           string
	RequestID       string
}

// SafetyResolveCommand 办结：必填 result 与 note。
type SafetyResolveCommand struct {
	Principal       authn.Principal
	EventID         string
	ExpectedVersion int64
	Result          string
	Note            string
	RequestID       string
}

// SafetyGetEvidenceCommand 读取必要证据：读取本身被审计。
type SafetyGetEvidenceCommand struct {
	Principal authn.Principal
	EventID   string
}

// SafetyEscalateTickCommand 扫描未领取升级：now 用于演练/补跑。
type SafetyEscalateTickCommand struct {
	Principal authn.Principal
	Now       time.Time
	Limit     int
}

// EscalateTickResult 是升级扫描的结果。Skipped 表示候选在写入前已被领取。
type EscalateTickResult struct {
	Escalated int32
	Skipped   int32
	Failed    int32
}

// SafetyEventView 只对本人与后台角色开放，附件只返回编号。
type SafetyEventView struct {
	EventID     string
	DriverID    string
	State       string
	Version     int64
	Handler     string
	ClaimedAt   *time.Time
	EventType   string
	Description string
	TaskID      string
	Position    entity.SafetyPosition
	ReceivedAt  *time.Time
	Attachments []string
	Supplements int
	Actions     int
	Escalations int
}

// EvidenceView 是必要证据的最小视图。录音在课程环境默认未开启：
// Available=false 表示无录音证据，绝不伪造"全程录音保护"。
type EvidenceView struct {
	EventID     string
	TaskID      string
	Trace       []entity.TracePointView
	Messages    []entity.MessageView
	VirtualCall *entity.VirtualCallView
	Recording   RecordingView
}

// RecordingView 表达录音证据现状：无录音时 Available=false。
type RecordingView struct {
	Available bool
	Note      string
}

// NoticeView 是 A18 说明内容的只读视图，带 ETag 供客户端缓存。
type NoticeView struct {
	Key     string
	Title   string
	Version int64
	ETag    string
	Blocks  []NoticeBlockView
}

type NoticeBlockView struct {
	Kind string
	Text string
}

// NoticeItem 是清单条目。
type NoticeItem struct {
	Key     string
	Title   string
	Version int64
	ETag    string
}

type HotlineItem struct {
	Name   string
	Number string
	Note   string
}

// HotlineView 是受控已发布配置的只读视图。
// AutoDial 恒为 false：课程环境不自动拨号；Demo 标记明确这是演示内容。
type HotlineView struct {
	Items            []HotlineItem
	Version          string
	PublishedAt      time.Time
	AutoDial         bool
	Demo             bool
	OfflineCacheable bool
}
