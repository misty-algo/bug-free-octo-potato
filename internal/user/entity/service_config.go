package entity

import "context"

// ServiceConfigRepository 客服配置读取端口（P22，读线上 config_dictionaries）。
// 绑定/协议/工单之外的客服页配置都取自参数字典现值（A18）。
type ServiceConfigRepository interface {
	// GetPublishedValues 按 dictKey 批量读取现值，state=已发布 才生效；
	// 无记录 key 不出现在结果里（调用方按空值处理，不 500）。
	GetPublishedValues(ctx context.Context, keys []string) (map[string]string, error)
}
