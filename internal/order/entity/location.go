package entity

import (
	"math"
	"strings"
	"time"
	"unicode/utf8"

	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Arrival thresholds. 距上车点不超过 200 米且定位不超过 30 秒才允许常规确认。
const (
	ArrivalRadiusMeters    = 200
	ArrivalFreshnessWindow = 30 * time.Second
)

type ArrivalInput struct {
	Reason string
}

type ArrivalDecision struct {
	// Normal is true only when the driver is within ArrivalRadiusMeters of the
	// boarding point and the latest position is no older than
	// ArrivalFreshnessWindow.
	Normal         bool
	Abnormal       bool
	Reason         string
	DistanceMeters *float64
	AgeMillis      *int64
}

func ValidateCoordinate(c Coordinate) error {
	if c.CRS != "GCJ02" || math.IsNaN(c.Latitude) || math.IsNaN(c.Longitude) || math.IsInf(c.Latitude, 0) || math.IsInf(c.Longitude, 0) || c.Latitude < -90 || c.Latitude > 90 || c.Longitude < -180 || c.Longitude > 180 {
		return apperrors.InvalidArgument("INVALID_LOCATION", "finite GCJ02 coordinates in geographic range are required")
	}
	return nil
}

func DistanceMeters(from, to Coordinate) (float64, error) {
	if err := ValidateCoordinate(from); err != nil {
		return 0, err
	}
	if err := ValidateCoordinate(to); err != nil {
		return 0, err
	}
	lat1, lat2 := from.Latitude*math.Pi/180, to.Latitude*math.Pi/180
	dlat, dlon := lat2-lat1, (to.Longitude-from.Longitude)*math.Pi/180
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 6371008.8 * 2 * math.Asin(math.Sqrt(math.Max(0, math.Min(1, a)))), nil
}

// EvaluateArrival decides whether a 常规到达确认 is admissible. The server
// recomputes distance and freshness from its own stored evidence and never
// trusts a client-supplied "arrived" flag.
//
// 业务规则（D05）：距上车点不超过 200 米且定位不超过 30 秒才允许常规确认；
// 其余情况必须填写原因走异常到达，由服务端生成复核标记。
func EvaluateArrival(pickup Coordinate, driver *Driver, input ArrivalInput, now time.Time) (ArrivalDecision, error) {
	decision := ArrivalDecision{Abnormal: true}
	reason := strings.TrimSpace(input.Reason)
	if utf8.RuneCountInString(reason) > 500 {
		return decision, apperrors.InvalidArgument("EXCEPTION_REASON_REQUIRED", "exception reason must contain 1 to 500 characters")
	}
	if err := ValidateCoordinate(pickup); err != nil {
		decision.Reason = "订单缺少有效上车点坐标"
		return decision, err
	}
	if driver == nil || driver.LastLocationAt == nil {
		decision.Reason = "缺少司机定位，请先上报位置"
	} else if location, ok := driver.LastLocation.Coordinate(); !ok {
		decision.Reason = "司机最近定位不可用"
	} else {
		age := now.Sub(*driver.LastLocationAt)
		ageMillis := age.Milliseconds()
		decision.AgeMillis = &ageMillis
		switch {
		case ageMillis < 0:
			decision.Reason = "定位时间异常"
		case age > ArrivalFreshnessWindow:
			decision.Reason = "定位已过期，请重新上报位置"
		default:
			distance, err := DistanceMeters(pickup, location)
			if err != nil {
				return decision, err
			}
			decision.DistanceMeters = &distance
			if distance > ArrivalRadiusMeters {
				decision.Reason = "距上车点过远，请确认位置或走异常到达"
			} else {
				decision.Normal = true
				decision.Abnormal = false
			}
		}
	}
	if decision.Abnormal {
		decision.Reason = strings.TrimSpace(decision.Reason)
		if reason == "" {
			return decision, apperrors.InvalidArgument("EXCEPTION_REASON_REQUIRED", decision.Reason+"；请填写 reason 走异常到达")
		}
	}
	return decision, nil
}
