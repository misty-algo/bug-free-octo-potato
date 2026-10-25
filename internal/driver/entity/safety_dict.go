package entity

// 安全事件处置闭环的数据字典。
// 状态编码全仓统一：RECEIVED → CLAIMED → PROCESSING → RESOLVED。
// 未知值一律拒绝，不静默归类。

// 事件状态机。RECEIVED 受理；CLAIMED 已领取（尚未行动）；PROCESSING 处理中；RESOLVED 已办结。
const (
	SafetyStateReceived   = "RECEIVED"
	SafetyStateClaimed    = "CLAIMED"
	SafetyStateProcessing = "PROCESSING"
	SafetyStateResolved   = "RESOLVED"
)

var knownSafetyStates = map[string]bool{
	SafetyStateReceived:   true,
	SafetyStateClaimed:    true,
	SafetyStateProcessing: true,
	SafetyStateResolved:   true,
}

// IsKnownSafetyState 是状态字典校验入口。
func IsKnownSafetyState(s string) bool { return knownSafetyStates[s] }

// 处置动作类型。全部只追加，不改动受理瞬间的原始内容。
// 前缀 Safety 避免与审核域既有 ActionClaim/ActionTransfer 冲突。
const (
	SafetyActionClaim          = "claim"
	SafetyActionTransfer       = "transfer"
	SafetyActionStart          = "start"
	SafetyActionNote           = "note"
	SafetyActionAttachEvidence = "attach_evidence"
	SafetyActionResolve        = "resolve"
)

// 办结结果。办结必填，缺 result 拒绝；值必须是字典内已知项。
const (
	ResultAssisted   = "assisted"    // 已协助处置
	ResultEscalated  = "escalated"   // 升级到外部/更高层级
	ResultFalseAlarm = "false_alarm" // 核实为误报
	ResultNoAction   = "no_action"   // 核实无需处置
)

var knownResolveResults = map[string]bool{
	ResultAssisted:   true,
	ResultEscalated:  true,
	ResultFalseAlarm: true,
	ResultNoAction:   true,
}

// IsKnownResolveResult 是办结结果字典校验入口。
func IsKnownResolveResult(r string) bool { return knownResolveResults[r] }

// 升级档位。从 received_at 起算，与重分配/服务重启无关。
const EscalationLevelUnclaimed int64 = 1

// 未领取升级的时限：受理后超过该时长仍无人领取则升级。
const (
	// EscalationAfter 是升级阈值（5 分钟），tick 时按 received_at 判定。
	EscalationAfter = 5 * 60 // 秒
	// QueueNotifyAfter 是正常提示窗口（1 分钟），用于值班队列提醒语义。
	QueueNotifyAfter = 60 // 秒
)

// 必要证据类型。录音在课程环境默认未开启：无录音即"无证据"，绝不伪造。
const (
	EvidenceTrace       = "trace"
	EvidenceMessage     = "message"
	EvidenceVirtualCall = "virtual_call"
	EvidenceRecording   = "recording"
)
