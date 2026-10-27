package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// CreateTripCommand 乘客下单入参。
type CreateTripCommand struct {
	TripID      string          // 行程号，由调用方生成（幂等锚点）
	PassengerID string          // 乘客 ID
	Origin      entity.GeoPoint // 上车点
	Destination entity.GeoPoint // 目的地
}

// EndTripCommand 司机送达入参。三个实测值是正式计价的唯一输入。
type EndTripCommand struct {
	TripID      string
	DriverID    string
	DistanceM   int64 // 行驶里程（米）
	DurationSec int64 // 行驶时长（秒）
	WaitSec     int64 // 等待时长（秒）
}

// AssignDriverCommand 司机接单入参。
type AssignDriverCommand struct {
	TripID   string
	DriverID string
}

// TripActionCommand 针对单个行程的简单动作（到达/上车/取消）。
type TripActionCommand struct {
	TripID   string
	DriverID string
}

// PayTripCommand 乘客为已结束行程付款的入参。
// 刻意不提供金额字段：金额必须等于计价结果，否则调用方可以自己定价。
type PayTripCommand struct {
	TripID string
	Scene  string // wap（手机网站）/ page（电脑网站）
}

// PayTripResult 行程支付结果。
type PayTripResult struct {
	TripID  string
	OrderNo string // 与行程号同值，支付回调据此关联回账单
	Amount  int64  // 实付金额（分），来自计价结果
	PayURL  string
}

// ListTripsCommand 查询乘客行程列表的入参。
type ListTripsCommand struct {
	PassengerID string
	Limit       int
}

// TripDTO 行程出参。
type TripDTO struct {
	TripID           string
	PassengerID      string
	DriverID         string
	Status           string
	Origin           entity.GeoPoint
	Destination      entity.GeoPoint
	DistanceM        int64
	DurationSec      int64
	WaitSec          int64
	StartedAt        *time.Time
	EndedAt          *time.Time
	BillingRequestID string
	PassengerPaid    int64
	CommissionRateBP int64
	BillCreated      bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ListTripsDTO 行程列表出参。
type ListTripsDTO struct {
	Items []*TripDTO
}

// FromTrip 把行程聚合转换为出参 DTO。
func FromTrip(t *entity.Trip) *TripDTO {
	if t == nil {
		return nil
	}
	return &TripDTO{
		TripID:           t.ID,
		PassengerID:      t.PassengerID,
		DriverID:         t.DriverID,
		Status:           t.Status,
		Origin:           t.Origin,
		Destination:      t.Destination,
		DistanceM:        t.DistanceM,
		DurationSec:      t.DurationSec,
		WaitSec:          t.WaitSec,
		StartedAt:        t.StartedAt,
		EndedAt:          t.EndedAt,
		BillingRequestID: t.BillingRequestID,
		PassengerPaid:    t.PassengerPaid,
		CommissionRateBP: t.CommissionRateBP,
		BillCreated:      t.BillCreated,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
}
