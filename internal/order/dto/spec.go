package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// SpecQuery 是司机端只读查询。身份来自令牌，司机角色由服务端兜底。
type SpecQuery struct {
	Principal authn.Principal `json:"-"`
	Category  string          `json:"category"`
	Mode      string          `json:"mode"` // brief 行驶短提示 / full 停车后全文
}

// SpecBlock 是受限结构化块，不含 HTML。
type SpecBlock struct {
	Type  string   `json:"type"`
	Text  string   `json:"text,omitempty"`
	Items []string `json:"items,omitempty"`
}

type SpecItem struct {
	Category    string      `json:"category"`
	Version     int64       `json:"version"`
	PublishedAt *time.Time  `json:"published_at"`
	Blocks      []SpecBlock `json:"blocks"`
}

// SpecResult 兼容接口文档 3.9.6 的 { version, content }，
// 并补充 guide_id、published_at、etag 以支持离线版本标注。
type SpecResult struct {
	GuideID     string      `json:"guide_id"`
	Version     int64       `json:"version"`
	PublishedAt *time.Time  `json:"published_at"`
	ETag        string      `json:"etag"`
	Categories  []SpecItem  `json:"categories"`
	Content     []SpecBlock `json:"content,omitempty"`
}

type SpecManifestItem struct {
	GuideID     string     `json:"guide_id"`
	Category    string     `json:"category"`
	Version     int64      `json:"version"`
	PublishedAt *time.Time `json:"published_at"`
	ETag        string     `json:"etag"`
}

// SpecManifestResult 是离线恢复后先比对再增量更新的版本清单。
type SpecManifestResult struct {
	GuideID string             `json:"guide_id"`
	Items   []SpecManifestItem `json:"items"`
}
