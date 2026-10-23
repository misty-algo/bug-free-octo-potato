package entity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
)

// SafetyNotice 是"事故上报说明 / 录音说明 / 安全须知"的受控说明文本。
// 复用 A18 后台 config_dictionary 发布 + 只读投影（同 D08 服务规范模式），
// 本域只读、不建第二份真相；A18 未配置时回落到内置默认文案。
type SafetyNotice struct {
	Key     string
	Version int64
	Title   string
	Blocks  []NoticeBlock
}

type NoticeBlock struct {
	Kind string // text | bullet
	Text string
}

// 三个说明条目的 key（A18 config_dictionary 中加前缀 safety.notice.）。
const (
	NoticeKeyReport    = "report"
	NoticeKeyRecording = "recording"
	NoticeKeyGuideline = "guideline"
)

// SafetyNoticeReader 读 A18 已发布条目；key 不存在返回 (nil, nil)。
type SafetyNoticeReader interface {
	PublishedNotice(ctx context.Context, key string) (*SafetyNotice, error)
}

// BuiltinSafetyNotices 是 A18 未发布时的兜底内容。课程环境：
// 录音默认未开启，无录音即"无证据"，绝不伪造"全程录音保护"。
var BuiltinSafetyNotices = map[string]SafetyNotice{
	NoticeKeyReport: {
		Key: NoticeKeyReport, Version: 1, Title: "事故上报说明",
		Blocks: []NoticeBlock{
			{Kind: "text", Text: "发生事故或人身安全受威胁时，请优先拨打 110/120，再通过安全事件入口上报。"},
			{Kind: "bullet", Text: "尽量提供位置；无法定位时可直接提交，后台会记录为未知位置，不会阻塞受理。"},
			{Kind: "bullet", Text: "描述经过时不要透露无关个人隐私。"},
		},
	},
	NoticeKeyRecording: {
		Key: NoticeKeyRecording, Version: 1, Title: "录音说明",
		Blocks: []NoticeBlock{
			{Kind: "text", Text: "行程录音默认未开启，仅在获得明确授权时才会采集。"},
			{Kind: "text", Text: "未采集录音时，事件证据中不显示录音，也不会伪造录音保护提示。"},
		},
	},
	NoticeKeyGuideline: {
		Key: NoticeKeyGuideline, Version: 1, Title: "安全须知",
		Blocks: []NoticeBlock{
			{Kind: "text", Text: "本内容为课程演示，不构成任何安全承诺。"},
			{Kind: "bullet", Text: "紧急电话指引不替代后台受理。"},
			{Kind: "bullet", Text: "处理状态以后台受理结果为准。"},
		},
	},
}

// BuiltinNoticeKeys 返回内置条目的稳定顺序，用于清单接口。
func BuiltinNoticeKeys() []string {
	keys := make([]string, 0, len(BuiltinSafetyNotices))
	for k := range BuiltinSafetyNotices {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ETag 与 D08 服务规范同一口径：W/"版本-摘要"。
func (n SafetyNotice) ETag() string {
	return fmt.Sprintf("W/\"v%d-%s\"", n.Version, n.Digest())
}

// Digest 对标题与正文做稳定摘要，供 If-None-Match 判断。
func (n SafetyNotice) Digest() string {
	h := sha256.New()
	h.Write([]byte(n.Title))
	for _, b := range n.Blocks {
		h.Write([]byte(b.Kind))
		h.Write([]byte{0})
		h.Write([]byte(b.Text))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
