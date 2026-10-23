package model

import "time"

// LocationDoc GCJ02 地点。
type LocationDoc struct {
	Name string  `bson:"name"`
	Lat  float64 `bson:"lat"`
	Lng  float64 `bson:"lng"`
}

// HallOrderDocument 是 hall_orders 集合文档（同时也是派单任务）。
type HallOrderDocument struct {
	ID   string `bson:"_id"`
	TaskID string `bson:"task_id"`
	OrderID string `bson:"order_id"`
	PassengerName  string `bson:"passenger_name"`
	PassengerCount int    `bson:"passenger_count"`
	CityCode       string `bson:"city_code"`
	OrderType      string `bson:"order_type"`
	RequireSeats   int    `bson:"require_seats"`
	RequireCarType string `bson:"require_car_type"`
	Origin      LocationDoc `bson:"origin"`
	Destination LocationDoc `bson:"destination"`

	// 金额一律整数分
	OrigFareFen       int64 `bson:"orig_fare_fen"`
	PassengerPaidFen  int64 `bson:"passenger_paid_fen"`
	PlatformCouponFen int64 `bson:"platform_coupon_fen"`
	DriverCouponFen   int64 `bson:"driver_coupon_fen"`
	CommissionFen     int64 `bson:"commission_fen"`
	SubsidyFen        int64 `bson:"subsidy_fen"`
	DriverIncomeFen   int64 `bson:"driver_income_fen"`

	Status      string `bson:"status"` // WAITING/ACCEPTED/ARRIVED/IN_TRIP/ENDED/CANCELLED
	DriverID    string `bson:"driver_id"`
	DriverName  string `bson:"driver_name"`
	DispatchChannel string `bson:"dispatch_channel"`
	VisibleInPool   bool   `bson:"visible_in_pool"`
	PickupDistanceMeters int64 `bson:"pickup_distance_meters"`

	InviteRound      int64    `bson:"invite_round"`
	RoundInvited     []string `bson:"round_invited"`
	AssignExclude    []string `bson:"assign_exclude"`
	AcceptDeadline   time.Time `bson:"accept_deadline"`
	ListenDeadline   time.Time `bson:"listen_deadline"`
	PoolVisibleAt    time.Time `bson:"pool_visible_at"`
	DispatchDeadline time.Time `bson:"dispatch_deadline"`

	AssignedAt  time.Time `bson:"assigned_at"`
	AcceptedAt  time.Time `bson:"accepted_at"`
	ArrivedAt   time.Time `bson:"arrived_at"`
	StartedAt   time.Time `bson:"started_at"`
	EndedAt     time.Time `bson:"ended_at"`
	CancelledAt time.Time `bson:"cancelled_at"`
	DurationSec    int64 `bson:"duration_sec"`
	DistanceMeters int64 `bson:"distance_meters"`

	CancelReason     string `bson:"cancel_reason"`
	ResponsibleParty string `bson:"responsible_party"`

	Version   int64     `bson:"version"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func (HallOrderDocument) MongoCollectionName() string { return HallOrderCollectionName }
