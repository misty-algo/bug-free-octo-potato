package entity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestExpiresAtFromDateIsExclusive 校验"2026-10-22 有效"在当地 10-23 00:00 失效。
func TestExpiresAtFromDateIsExclusive(t *testing.T) {
	expiresAt, err := ExpiresAtFromDate("2026-10-22")
	require.NoError(t, err)
	require.Equal(t, "2026-10-23T00:00:00+08:00", expiresAt.Format(time.RFC3339))
	require.Equal(t, "2026-10-22", LastValidDate(expiresAt), "最后有效自然日不含次日")

	require.False(t, IsExpired(time.Date(2026, 10, 22, 23, 59, 59, 0, Shanghai), expiresAt))
	require.True(t, IsExpired(time.Date(2026, 10, 23, 0, 0, 0, 0, Shanghai), expiresAt), "边界时刻即失效")
	// UTC 的 10-22 16:00 已是当地 10-23 00:00，不能按服务器时区判定。
	require.True(t, IsExpired(time.Date(2026, 10, 22, 16, 0, 0, 0, time.UTC), expiresAt))
}

// TestReminderDueAtUsesCalendarDays 校验档位按自然日减法，而不是固定 24 小时差。
func TestReminderDueAtUsesCalendarDays(t *testing.T) {
	expiresAt, err := ExpiresAtFromDate("2026-11-20")
	require.NoError(t, err)
	require.Equal(t, "2026-10-21T09:00:00+08:00", ReminderDueAt(expiresAt, 30).Format(time.RFC3339))
	require.Equal(t, "2026-11-13T09:00:00+08:00", ReminderDueAt(expiresAt, 7).Format(time.RFC3339))
	require.Equal(t, "2026-11-19T09:00:00+08:00", ReminderDueAt(expiresAt, 1).Format(time.RFC3339))
}

// TestDocumentExpiryFallsBackToLegacy 校验存量 validUntil 按同一自然日口径归一。
func TestDocumentExpiryFallsBackToLegacy(t *testing.T) {
	legacy := time.Date(2026, 10, 22, 18, 30, 0, 0, Shanghai)
	doc := DriverDocument{ValidUntil: &legacy}
	expiry := doc.Expiry()
	require.NotNil(t, expiry)
	require.Equal(t, "2026-10-23T00:00:00+08:00", expiry.Format(time.RFC3339))
	// 新口径字段优先。
	exclusive, err := ExpiresAtFromDate("2027-01-01")
	require.NoError(t, err)
	doc.ExpiresAt = &exclusive
	require.Equal(t, "2027-01-02T00:00:00+08:00", doc.Expiry().Format(time.RFC3339))
}
