package request

// CreateTripRequest 是 POST /api/pax/trips 使用的 JSON 载荷。
type CreateTripRequest struct {
	TripID      string   `json:"trip_id" binding:"required"`
	PassengerID string   `json:"passenger_id" binding:"required"`
	Origin      GeoPoint `json:"origin" binding:"required"`
	Destination GeoPoint `json:"destination" binding:"required"`
}

// GeoPoint 是 GeoJSON Point 载荷，坐标顺序 [lng, lat]。
type GeoPoint struct {
	Type        string    `json:"type" binding:"required"`
	Coordinates []float64 `json:"coordinates" binding:"required,len=2"`
}

// TripActionRequest 是到达 / 上车 / 取消动作使用的 JSON 载荷。
type TripActionRequest struct {
	TripID   string `json:"trip_id" binding:"required"`
	DriverID string `json:"driver_id"`
}

// AssignDriverRequest 是司机接单使用的 JSON 载荷。
type AssignDriverRequest struct {
	TripID   string `json:"trip_id" binding:"required"`
	DriverID string `json:"driver_id" binding:"required"`
}

// PayTripRequest 是行程支付使用的 JSON 载荷。
// 刻意不含金额字段——金额必须等于计价结果，不接受调用方指定。
type PayTripRequest struct {
	TripID string `json:"trip_id" binding:"required"`
	Scene  string `json:"scene" binding:"required,oneof=wap page"`
}

// EndTripRequest 是送达使用的 JSON 载荷。三个实测值是正式计价的唯一输入。
type EndTripRequest struct {
	TripID      string `json:"trip_id" binding:"required"`
	DriverID    string `json:"driver_id"`
	DistanceM   int64  `json:"distance_m" binding:"min=0"`
	DurationSec int64  `json:"duration_sec" binding:"min=0"`
	WaitSec     int64  `json:"wait_sec" binding:"min=0"`
}
