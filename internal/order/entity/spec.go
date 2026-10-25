package entity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// 服务规范状态。后台草稿与停用版本永不返回给司机。
const (
	SpecStateDraft     = "draft"
	SpecStatePublished = "published"
	SpecStateDisabled  = "disabled"
)

// SpecKey 是 A18 后台 config_dictionary 中服务规范的条目键。
const SpecKey = "drv.service-spec"

// 内容分类：接人话术、禁止行为、红线行为、违规处置。
const (
	CategoryPickupScript = "pickup_script"
	CategoryProhibited   = "prohibited"
	CategoryRedline      = "redline"
	CategoryPenalty      = "penalty"
)

// 受限结构化正文：仅允许这四种块，禁止内联 HTML 或任意富文本。
var allowedSpecBlockTypes = map[string]bool{
	"heading": true, "paragraph": true, "list": true, "warning": true,
}

// ContentBlock 是服务规范正文的最小结构。渲染白名单在前端，
// 服务端负责不返回允许类型之外的任何内容。
type ContentBlock struct {
	Type  string   `bson:"type"`
	Text  string   `bson:"text"`
	Items []string `bson:"items,omitempty"`
}

func (b ContentBlock) allowed() bool { return allowedSpecBlockTypes[b.Type] }

// SpecEntry 是 config_dictionary 的只读投影。该集合属 A18 后台配置域，
// 行程域只读消费，不写、不发布、不建立第二份真相。
type SpecEntry struct {
	Key         string         `bson:"key"`
	GuideID     string         `bson:"guideId"`
	Category    string         `bson:"category"`
	Version     int64          `bson:"version"`
	State       string         `bson:"state"`
	PublishedAt *time.Time     `bson:"publishedAt"`
	Brief       []ContentBlock `bson:"brief"`
	Content     []ContentBlock `bson:"content"`
}

// htmlTag 匹配任意 HTML 标签。规范正文是受限结构化文本，
// 服务端必须先剥离后台可能带入的标签，再由前端白名单渲染。
var htmlTag = regexp.MustCompile(`<[^>]*>`)

// stripHTML 去掉标签内容后返回纯文本，避免后台任意 HTML 被插进页面。
func stripHTML(s string) string {
	return strings.TrimSpace(htmlTag.ReplaceAllString(s, ""))
}

func sanitized(blocks []ContentBlock) []ContentBlock {
	out := make([]ContentBlock, 0, len(blocks))
	for _, b := range blocks {
		if !b.allowed() {
			continue
		}
		clean := ContentBlock{Type: b.Type, Text: stripHTML(b.Text)}
		if clean.Text == "" && b.Text != "" {
			// 整段都是标签，没有可读正文，丢弃而不是留空节点。
			continue
		}
		for _, item := range b.Items {
			if text := stripHTML(item); text != "" {
				clean.Items = append(clean.Items, text)
			}
		}
		out = append(out, clean)
	}
	return out
}

// Blocks 按模式返回净化后的正文。brief 未配置时退化为正文前两块，
// 便于行驶场景短提示，但不改变「短提示来自 brief」的语义。
func (e SpecEntry) Blocks(mode string) []ContentBlock {
	if mode == "brief" {
		if blocks := sanitized(e.Brief); len(blocks) > 0 {
			return blocks
		}
		blocks := sanitized(e.Content)
		if len(blocks) > 2 {
			blocks = blocks[:2]
		}
		return blocks
	}
	return sanitized(e.Content)
}

// canonical 是参与摘要计算的规范化内容，用于弱校验器。
func (e SpecEntry) canonical() []byte {
	body, _ := json.Marshal(struct {
		Key      string         `json:"key"`
		Category string         `json:"category"`
		Version  int64          `json:"version"`
		Blocks   []ContentBlock `json:"blocks"`
	}{Key: e.Key, Category: e.Category, Version: e.Version, Blocks: e.Blocks("full")})
	return body
}

// Digest 是参与校验器计算的原始摘要，不含 W/ 与引号，便于多分类聚合。
func (e SpecEntry) Digest() string {
	sum := sha256.Sum256(e.canonical())
	return fmt.Sprintf("%d-%s", e.Version, hex.EncodeToString(sum[:8]))
}

// ETag 是单个条目的弱校验器：版本 + 正文摘要。
func (e SpecEntry) ETag() string {
	return `W/"` + e.Digest() + `"`
}
