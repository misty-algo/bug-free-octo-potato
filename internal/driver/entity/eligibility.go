package entity

import (
	"context"
	"time"
)

// 限制原因码。source 说明这条限制来自哪类权威事实。
const (
	RestrictionDocMissing = "DOC_MISSING"
	RestrictionDocExpired = "DOC_EXPIRED"
	RestrictionAccount    = "ACCOUNT_RESTRICTED"
	RestrictionVehicle    = "VEHICLE_INVALID"
	RestrictionBinding    = "BINDING_MISSING"

	SourceDocType = "doc_type"
	SourceAccount = "account"
	SourceVehicle = "vehicle"
	SourceBinding = "binding"
)

// 可执行动作：只描述"能不能做"，不改变忙闲状态。
const (
	ActionAcceptTask = "accept_task"
	ActionGoOnline   = "go_online"
)

// Restriction 是拒绝接单的具体原因，前端据此展示，司机能看到明确理由。
type Restriction struct {
	Code      string
	Source    string
	TargetKey string
	Detail    string
	Until     *time.Time
}

// Eligibility 是实时计算结果。没有投影表，因此不存在"按旧缓存放行"的路径。
type Eligibility struct {
	DriverID         string
	CanAcceptNewTask bool
	Restrictions     []Restriction
	// SourceRevision 复用 driver_admission.admissionVersion，全仓只有一个版本序列。
	SourceRevision int64
	// ValidUntil 不晚于最近一张必需证件的 expires_at。
	ValidUntil     *time.Time
	AllowedActions []string
	BlockedActions []string
}

// DispatchRepository 是分配入口的写入侧。
// 真实派单域接入后应替换为服务间调用，本域实现仅用于打通权威边界。
type DispatchRepository interface {
	CreateAssignment(ctx context.Context, driverID string, orderIDs []string, revision int64, taskNo string, at time.Time) (string, error)
}
