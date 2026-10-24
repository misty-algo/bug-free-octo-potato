// Package geo 提供统一坐标口径与距离计算。
//
// 依据《各模块技术点与实行方案》1.2 统一技术底座：
//   - 坐标统一 GCJ02；
//   - 距离用整数米、时长用整数秒，禁止在跨模块契约里出现浮点公里。
//
// 司机端（接单距离排序、接单区域判定）、乘客端（预估）、行程域（轨迹）
// 共用同一套算法，保证同一对点位算出的距离在任何模块都相等。
package geo

import "math"

// Point 是 GCJ02 坐标点。
type Point struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

const earthRadiusM = 6371000.0

// DistanceMeters 计算两个 GCJ02 坐标点的球面距离（米，向下取整）。
func DistanceMeters(a, b Point) int64 {
	return int64(math.Floor(haversine(a, b)))
}

// DistanceMetersRound 四舍五入到整数米（展示/计费口径）。
func DistanceMetersRound(a, b Point) int64 {
	return int64(math.Round(haversine(a, b)))
}

func haversine(a, b Point) float64 {
	const rad = math.Pi / 180
	dLat := (b.Lat - a.Lat) * rad
	dLng := (b.Lng - a.Lng) * rad
	lat1 := a.Lat * rad
	lat2 := b.Lat * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}

// InBox 判断坐标是否落在矩形包围盒内（边界点算区域内：司机端单片区判定规则）。
func InBox(p Point, minLat, minLng, maxLat, maxLng float64) bool {
	return p.Lat >= minLat && p.Lat <= maxLat && p.Lng >= minLng && p.Lng <= maxLng
}
