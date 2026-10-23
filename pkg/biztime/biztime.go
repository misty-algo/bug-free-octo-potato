// Package biztime 提供全局统一的业务时间口径。
//
// 依据《各模块技术点与实行方案》1.2 统一技术底座：
//   - 业务时间一律取服务端可信时间；
//   - 报表/统计的自然日统一 Asia/Shanghai；
//   - 单体实地时间用于跨日、到期与 grace 期的判断。
//
// 所有模块（乘客端、司机端、行程域、财务域、后台）都必须走本包的 Now，
// 禁止直接 time.Now()，否则跨模块的时间口径会不一致。
package biztime

import "time"

// Location 是业务口径所在时区（中国标准时间，固定 UTC+8，无夏令时）。
var Location = time.FixedZone("Asia/Shanghai", 8*3600)

// Now 返回业务当前时间（服务端可信时间）。
func Now() time.Time { return time.Now().In(Location) }

// UTC 返回业务当前时间对应的 UTC 时刻，便于统一入库。
func UTC() time.Time { return Now().UTC() }

// In shanghai 将任意时间转换到业务时区。
func In(t time.Time) time.Time { return t.In(Location) }

// DayStart 返回 t 所在自然日的起点（业务时区当日 00:00:00 对应的 UTC 时刻）。
func DayStart(t time.Time) time.Time {
	local := In(t)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Location).UTC()
}

// DayEnd 返回 t 所在自然日的下一日起点（半开区间上界）。
func DayEnd(t time.Time) time.Time { return DayStart(t).AddDate(0, 0, 1) }

// AddDays 在自然日维度平移天数（用于今日/昨日统计）。
func AddDays(t time.Time, days int) time.Time {
	local := In(t).AddDate(0, 0, days)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, Location).UTC()
}

// MonthStart 返回 t 所在自然月的起点。
func MonthStart(t time.Time) time.Time {
	local := In(t)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, Location).UTC()
}

const dateLayout = "2006-01-02"

// DateKey 返回记账日/统计日 key（yyyy-mm-dd，业务时区）。
func DateKey(t time.Time) string { return In(t).Format(dateLayout) }

// ParseDateKey 解析 yyyy-mm-dd 到当日起点（业务时区，UTC 时刻）。
func ParseDateKey(key string) (time.Time, error) {
	t, err := time.ParseInLocation(dateLayout, key, Location)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

const stampLayout = "2006-01-02 15:04:05"

// FormatStamp 输出可读时间（业务时区），用于接口出参与日志。
func FormatStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return In(t).Format(stampLayout)
}

// RFC3339 输出带时区的时间字符串（跨模块事件统一格式）。
func RFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return In(t).Format(time.RFC3339)
}
