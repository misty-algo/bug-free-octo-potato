package entity

import (
	"errors"
	"math"
	"time"
)

// 订单行程状态机（表结构 TRIP_STATE 六态）。
const (
	TripStatusWaiting  = "WAITING"
	TripStatusAccepted = "ACCEPTED"
	TripStatusArrived  = "ARRIVED"
	TripStatusInTrip   = "IN_TRIP"
	TripStatusEnded    = "ENDED"
	TripStatusCanceled = "CANCELLED"
)

// 轨迹点类型。
const (
	PointTypePickup  = "pickup"
	PointTypeService = "service"
)

// 轨迹上报来源。
const (
	TrajectorySourceApp    = "app"
	TrajectorySourceDevice = "device"
)

// 轨迹质量规则阈值（P08）。
const (
	// TrajectoryDelayThreshold 相邻上报超过该时长即标记延迟（司机 5 秒上报）。
	TrajectoryDelayThreshold = 30 * time.Second
	// TrajectoryJumpDistance 相邻两点距离超过该值（米）判定为跳点。
	TrajectoryJumpDistance = 500.0
)

// 轨迹点质量问题标签。
const (
	QualityNormal = "normal" // 正常
	QualityDelay  = "delay"  // 上报延迟
	QualityJump   = "jump"   // 跳点
)

// TrajectoryPoint 是轨迹上报点（值对象）。
type TrajectoryPoint struct {
	ID          string // 持久化主键，由 service 生成
	OrderID     string
	DriverID    string
	Location    GeoPoint // {type:"Point", coordinates:[lng, lat]}
	Speed       float64
	Bearing     float64
	Accuracy    float64
	Mileage     float64
	PointType   string
	ReportedAt  time.Time // 司机端上报时间
	CollectedAt time.Time // 服务端采集时间
	Source      string

	// 质量标记（由聚合推导，不入库则用于展示层）。
	Quality string
}

// GeoPoint 表示 GeoJSON Point，坐标顺序为 [lng, lat]。
type GeoPoint struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

// Lng 返回经度。GeoJSON 规定坐标顺序为 [lng, lat]，与口语习惯的「经纬度」相反，
// 用命名方法取值，避免各处直接下标访问时把经纬写反——那会让派单把司机导到错的地方。
func (g GeoPoint) Lng() float64 {
	if len(g.Coordinates) < 1 {
		return 0
	}
	return g.Coordinates[0]
}

// Lat 返回纬度。坐标顺序见 Lng 的说明。
func (g GeoPoint) Lat() float64 {
	if len(g.Coordinates) < 2 {
		return 0
	}
	return g.Coordinates[1]
}

// IsValid 校验轨迹点必填字段与坐标合法性。
func (p *TrajectoryPoint) IsValid() error {
	if p.OrderID == "" || p.DriverID == "" {
		return errors.New("order_id and driver_id are required")
	}
	if p.Location.Type != "Point" || len(p.Location.Coordinates) != 2 {
		return errors.New("location must be GeoJSON Point with [lng, lat]")
	}
	lng, lat := p.Location.Coordinates[0], p.Location.Coordinates[1]
	if lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return errors.New("location coordinates out of range")
	}
	if p.PointType != PointTypePickup && p.PointType != PointTypeService {
		return errors.New("point_type must be pickup or service")
	}
	if p.Source != TrajectorySourceApp && p.Source != TrajectorySourceDevice {
		return errors.New("source must be app or device")
	}
	return nil
}

// CheckJump 校验相邻两点是否构成跳点：距离超过阈值（米）。
func (p *TrajectoryPoint) CheckJump(prev *TrajectoryPoint) bool {
	if prev == nil {
		return false
	}
	d := HaversineMeters(prev.Location, p.Location)
	return d > TrajectoryJumpDistance
}

// CheckDelay 校验相对上一上报点的间隔是否超过 30 秒延迟阈值。
func (p *TrajectoryPoint) CheckDelay(prev *TrajectoryPoint) bool {
	if prev == nil {
		return false
	}
	return p.ReportedAt.Sub(prev.ReportedAt) > TrajectoryDelayThreshold
}

// ApplyQuality 按规则推导质量问题标记：
// 与上一上报点间隔超 30 秒标 delay；距离超阈值标 jump；两者叠加时 jump 优先。
func (p *TrajectoryPoint) ApplyQuality(prev *TrajectoryPoint) {
	p.Quality = QualityNormal
	switch {
	case p.CheckJump(prev):
		p.Quality = QualityJump
	case p.CheckDelay(prev):
		p.Quality = QualityDelay
	}
}

// HaversineMeters 计算两个 GeoJSON Point 的大圆距离（米），用于跳点校验。
func HaversineMeters(a, b GeoPoint) float64 {
	const earthRadius = 6371000.0
	if len(a.Coordinates) != 2 || len(b.Coordinates) != 2 {
		return math.MaxFloat64
	}
	lat1 := a.Coordinates[1] * math.Pi / 180
	lat2 := b.Coordinates[1] * math.Pi / 180
	dLat := (b.Coordinates[1] - a.Coordinates[1]) * math.Pi / 180
	dLng := (b.Coordinates[0] - a.Coordinates[0]) * math.Pi / 180

	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(h))
}
