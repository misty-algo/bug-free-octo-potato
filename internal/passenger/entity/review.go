package entity

import (
	"strings"
	"time"
)

// Review 是乘客对司机的评价或投诉记录（每单一条，提交后不可改）。
type Review struct {
	ID         string
	OrderNo    string
	OrderID    string
	AccountID  string
	DriverName string
	// Rating 1-5 星；纯投诉时可为 0。
	Rating int32
	// Tags 好评标签（如 服务好、开车稳）。
	Tags []string
	// Comment 评语。
	Comment string
	// IsComplaint 为 true 时本条记录是投诉。
	IsComplaint bool
	// ComplaintType 投诉类型（service_attitude / route_detour / unsafe_driving / overcharge / other）。
	ComplaintType string
	// ComplaintDesc 投诉描述。
	ComplaintDesc string
	CreatedAt     time.Time
}

// Valid 校验评价/投诉内容。
func (r *Review) Valid() bool {
	if r.OrderNo == "" || r.AccountID == "" {
		return false
	}
	if r.IsComplaint {
		if r.ComplaintType == "" || strings.TrimSpace(r.ComplaintDesc) == "" {
			return false
		}
		if len(r.ComplaintDesc) > 500 {
			return false
		}
		return true
	}
	return r.Rating >= 1 && r.Rating <= 5
}
