package model

import "time"

// RideDocument 是 trips 集合的 MongoDB 文档模型（乘客行程聚合）。
type RideDocument struct {
	ID          string `bson:"_id"`
	PassengerID string `bson:"passenger_id"`
	DriverID    string `bson:"driver_id"`
	Status      string `bson:"status"`

	Origin      GeoPoint `bson:"origin"`
	Destination GeoPoint `bson:"destination"`

	DistanceM   int64 `bson:"distance_m"`
	DurationSec int64 `bson:"duration_sec"`
	WaitSec     int64 `bson:"wait_sec"`

	StartedAt *time.Time `bson:"started_at,omitempty"`
	EndedAt   *time.Time `bson:"ended_at,omitempty"`

	BillingRequestID string `bson:"billing_request_id"`
	PassengerPaid    int64  `bson:"passenger_paid"`
	// CommissionRateBP 行程结束时由计价规则快照锁定的佣金率（万分比）。
	CommissionRateBP int64 `bson:"commission_rate_bp"`
	BillCreated      bool  `bson:"bill_created"`

	Version int64 `bson:"version"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (RideDocument) MongoCollectionName() string {
	return "trips"
}
