package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

// DriverDTO 由用例返回，并由 handler 转换。
type DriverDTO struct {
	ID          string
	Name        string
	Description string
	CreatedAt   string
	UpdatedAt   string
}

// ListDriverDTO 包含分页列表出参。
type ListDriverDTO struct {
	Items []*DriverDTO
	Total int64
}

// FromDriver 将 driver 聚合转换为 service 响应 DTO。
func FromDriver(item *entity.Driver) *DriverDTO {
	return &DriverDTO{
		ID:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		CreatedAt:   formatTime(item.CreatedAt),
		UpdatedAt:   formatTime(item.UpdatedAt),
	}
}

// formatTime 让零值时间保持为空，并用稳定 API 格式序列化真实时间。
func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339Nano)
}
