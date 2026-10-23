package entity

import "time"

// Shanghai 是业务日历时区。中国无夏令时，用固定偏移，
// 避免容器缺 tzdata 时 time.LoadLocation 失败把整条链路带崩。
var Shanghai = time.FixedZone("Asia/Shanghai", 8*3600)

// DateLayout 是"最后有效自然日"的对外格式。
const DateLayout = "2006-01-02"

// ExpiresAtFromDate 把"2026-11-20 有效"解析成当地次日 00:00 的 exclusive 边界：
// 2026-11-21 00:00:00+08:00 起失效，判定始终是 now < expires_at。
func ExpiresAtFromDate(date string) (time.Time, error) {
	day, err := time.ParseInLocation(DateLayout, date, Shanghai)
	if err != nil {
		return time.Time{}, err
	}
	return day.AddDate(0, 0, 1), nil
}

// NormalizeLegacyValidUntil 把旧口径的 unix 秒归一到自然日边界：
// 它表示"当日仍有效"，因此取当地当天 00:00 的次日作为 expires_at。
// 这是兼容路径，新写入一律用 valid_until_date。
func NormalizeLegacyValidUntil(t time.Time) time.Time {
	day := t.In(Shanghai)
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, Shanghai)
	return start.AddDate(0, 0, 1)
}

// LastValidDate 返回 expires_at 对应的最后有效自然日（当地）。
func LastValidDate(expiresAt time.Time) string {
	return expiresAt.In(Shanghai).AddDate(0, 0, -1).Format(DateLayout)
}

// IsExpired 是唯一的有效期判定：now < expires_at 才算有效。
func IsExpired(now, expiresAt time.Time) bool {
	return !now.Before(expiresAt)
}

// ReminderDueAt 计算提醒到期时间：最后有效自然日减 daysBefore 个日历天，
// 当天 09:00（当地）。用日历减法而不是 24 小时差。
func ReminderDueAt(expiresAt time.Time, daysBefore int) time.Time {
	last := expiresAt.In(Shanghai).AddDate(0, 0, -1)
	due := last.AddDate(0, 0, -daysBefore)
	return time.Date(due.Year(), due.Month(), due.Day(), 9, 0, 0, 0, Shanghai)
}
