package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// RestrictionView 是准入限制原因，前端直接展示给司机。
type RestrictionView struct {
	Code      string     `json:"code"`
	Source    string     `json:"source"`
	TargetKey string     `json:"target_key"`
	Detail    string     `json:"detail"`
	Until     *time.Time `json:"until,omitempty"`
}

// EligibilityView 是实时计算结果，调用方不得缓存后当作长期授权。
type EligibilityView struct {
	DriverID         string            `json:"driver_id"`
	CanAcceptNewTask bool              `json:"can_accept_new_task"`
	Restrictions     []RestrictionView `json:"restrictions"`
	SourceRevision   int64             `json:"source_revision"`
	ValidUntil       *time.Time        `json:"valid_until,omitempty"`
	AllowedActions   []string          `json:"allowed_actions"`
	BlockedActions   []string          `json:"blocked_actions"`
}

// ReminderTickResult 是一次调度扫描的计数，失败项不会阻塞后续档位。
type ReminderTickResult struct {
	Claimed   int32 `json:"claimed"`
	Sent      int32 `json:"sent"`
	Merged    int32 `json:"merged"`
	Cancelled int32 `json:"cancelled"`
	Failed    int32 `json:"failed"`
}

// AssignTaskResult 是分配成功的证据：绑定哪一版准入、哪辆车。
type AssignTaskResult struct {
	TaskID         string                  `json:"task_id"`
	TaskNo         string                  `json:"task_no"`
	SourceRevision int64                   `json:"source_revision"`
	VehicleID      string                  `json:"vehicle_id"`
	Snapshot       *entity.VehicleSnapshot `json:"snapshot,omitempty"`
}

// AssignTaskCommand 是任务分配的权威入口。expected_source_revision <= 0 表示不校验。
type AssignTaskCommand struct {
	Principal              authn.Principal
	DriverID               string
	OrderIDs               []string
	ExpectedSourceRevision int64
	TaskNo                 string
}
