package dto

import "github.com/BwCloudWeGo/bw-cli/pkg/authn"

// ArriveCommand is the 到达上车点确认 command (接口文档 3.9.1).
// 身份来自令牌，请求不携带 driver_id。
type ArriveCommand struct {
	RequestID      string          `json:"-"`
	Principal      authn.Principal `json:"-"`
	TaskID         string          `json:"task_id"`
	IdempotencyKey string          `json:"-"`
	Reason         string          `json:"reason"`
}

// ArrivalResult matches the documented response data exactly:
// { task_id, state: "ACTIVE", abnormal: bool }.
type ArrivalResult struct {
	TaskID   string `json:"task_id"`
	State    string `json:"state"`
	Abnormal bool   `json:"abnormal"`
}

// RecordLocationCommand is the 司机位置上报 command (接口文档 3.9.5).
type RecordLocationCommand struct {
	Principal authn.Principal `json:"-"`
	TaskID    string          `json:"task_id"`
	Lng       float64         `json:"lng"`
	Lat       float64         `json:"lat"`
}

// LocationReceipt matches the documented response: { accepted: true }.
type LocationReceipt struct {
	Accepted bool `json:"accepted"`
}
