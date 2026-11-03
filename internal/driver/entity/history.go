package entity

import (
	"context"
	"time"
)

// HistoryRating 订单内嵌评价的精简投影。
type HistoryRating struct {
	Score     int32     `json:"score"`
	Content   string    `json:"content"`
	Anonymous bool      `json:"anonymous"`
	RatedAt   time.Time `json:"ratedAt"`
}

// HistoryComplaint 投诉工单的精简投影。
type HistoryComplaint struct {
	Type    string    `json:"type"`
	Content string    `json:"content"`
	State   string    `json:"state"`
	At      time.Time `json:"at"`
}

// HistoryOrder 司机接单记录（含乘客评价/投诉）。
type HistoryOrder struct {
	OrderID        string           `json:"orderId"`
	PassengerName  string           `json:"passengerName"`
	PassengerCount int              `json:"passengerCount"`
	Origin         Location         `json:"origin"`
	Destination    Location         `json:"destination"`
	Status         string           `json:"status"`
	DriverIncomeFen int64           `json:"driverIncomeFen"`
	CreatedAt      time.Time        `json:"createdAt"`
	EndedAt        time.Time        `json:"endedAt"`
	Rating         *HistoryRating   `json:"rating,omitempty"`
	Complaint      *HistoryComplaint `json:"complaint,omitempty"`
}

// DriverNotice 司机收到的反馈通知（评价/投诉）。
type DriverNotice struct {
	ID      string    `json:"id"`
	Type    string    `json:"type"` // rating / complaint
	OrderID string    `json:"orderId"`
	Title   string    `json:"title"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// HistoryRepository 司机接单记录与乘客反馈查询（跨域只读，复用同一物理库）。
type HistoryRepository interface {
	// ListTakenOrders 返回该司机接过的订单（按创建时间倒序），并附带评价与投诉。
	ListTakenOrders(ctx context.Context, driverID string, limit int) ([]*HistoryOrder, error)
}
