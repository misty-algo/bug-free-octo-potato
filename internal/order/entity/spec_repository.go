package entity

import "context"

// SpecRepository 是 A18 config_dictionary 的只读门面。
// 司机只读由服务端权限兜底：这里没有任何写入方法。
type SpecRepository interface {
	// PublishedEntries 只返回 state=published 的条目。
	// 草稿与停用版本必须被过滤掉，绝不返回给司机端。
	PublishedEntries(ctx context.Context, key string) ([]SpecEntry, error)
}
