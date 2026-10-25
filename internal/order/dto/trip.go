package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// VerifyCommand 是乘客上车核验命令（接口文档 3.9.2）。
// 身份来自令牌，请求不携带 driver_id。
type VerifyCommand struct {
	RequestID      string          `json:"-"`
	Principal      authn.Principal `json:"-"`
	OrderID        string          `json:"order_id"`
	Code           string          `json:"code"`
	IdempotencyKey string          `json:"-"`
}

// VerifyResult 只回传是否已核验、剩余次数与锁定时长，不回显正确答案。
type VerifyResult struct {
	OrderID           string     `json:"order_id"`
	Verified          bool       `json:"verified"`
	RemainingAttempts int        `json:"remaining_attempts"`
	LockUntil         *time.Time `json:"lock_until,omitempty"`
}

// StartCommand 是开始行程命令（接口文档 3.9.3）。
type StartCommand struct {
	Principal      authn.Principal `json:"-"`
	TaskID         string          `json:"task_id"`
	IdempotencyKey string          `json:"-"`
}

type StartResult struct {
	TaskID     string    `json:"task_id"`
	State      string    `json:"state"`
	DepartedAt time.Time `json:"departed_at"`
}

// EndCommand 是确认送达命令（接口文档 3.9.4）。
type EndCommand struct {
	Principal      authn.Principal `json:"-"`
	OrderID        string          `json:"order_id"`
	ActualDist     int64           `json:"actual_dist"`
	ActualDur      int64           `json:"actual_dur"`
	EndReason      string          `json:"end_reason"`
	IdempotencyKey string          `json:"-"`
}

// BillProjection 是行程域持有的账单投影，不是第二份正式账单真相。
type BillProjection struct {
	T     int64  `json:"T"`
	P     int64  `json:"P"`
	State string `json:"state"`
}

type EndResult struct {
	OrderID string         `json:"order_id"`
	State   string         `json:"state"`
	Bill    BillProjection `json:"bill"`
}

// SampleCommand 是轨迹样本上报（接口文档 3.9.5 的扩展）。
type SampleCommand struct {
	Principal   authn.Principal `json:"-"`
	TaskID      string          `json:"task_id"`
	SessionID   string          `json:"session_id"`
	Seq         int64           `json:"seq"`
	Lng         float64         `json:"lng"`
	Lat         float64         `json:"lat"`
	CRS         string          `json:"crs"`
	Accuracy    *float64        `json:"accuracy,omitempty"`
	CollectedAt *time.Time      `json:"-"`
}

// SampleResult 区分已持久化确认与缺失序号，供前端按批次补传。
type SampleResult struct {
	Accepted   bool    `json:"accepted"`
	TaskID     string  `json:"task_id"`
	SessionID  string  `json:"session_id"`
	Seq        int64   `json:"seq"`
	Watermark  int64   `json:"watermark"`
	Missing    []int64 `json:"missing,omitempty"`
	Quality    string  `json:"quality"`
	Converted  bool    `json:"converted"`
	Duplicated bool    `json:"duplicated,omitempty"`
}
