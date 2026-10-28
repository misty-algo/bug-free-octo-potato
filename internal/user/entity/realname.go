package entity

import (
	"context"
	"time"
)

// 实名认证状态（契约 3.7.1 状态机，D5 中文枚举落库；线上存量 = 已认证）。
const (
	RealNameStateNone    = "未认证"
	RealNameStatePending = "认证中"
	RealNameStatePassed  = "已认证"
	RealNameStateFailed  = "认证失败"
)

// RealNameAuth 账号实名认证信息（落 accounts.realNameAuth，P20）。
// 新写入只存三件套（Enc/Hash/Mask），不落姓名/证件真值；
// 存量明文由映射层兼容读，不回写本结构。
type RealNameAuth struct {
	NameEnc       string
	NameHash      string
	NameMask      string
	IDNoEnc       string
	IDNoHash      string
	IDNoMask      string
	State         string
	SubmitCount   int      // 日限计数（每账号每日 3 次有效提交）
	SubmitDay     string   // 计数归属日 YYYY-MM-DD，跨日清零
	IdemKeyHashes []string // 当日已用幂等键哈希，同键同内容重试不计次
	AuthAt        *time.Time
}

// RealNameAccountRepository 实名认证读写端口（P20，落 accounts 集合）。
type RealNameAccountRepository interface {
	// GetRealNameAuth 返回账号实名子文档；未认证时返回 (nil, nil)。
	GetRealNameAuth(ctx context.Context, accountID string) (*RealNameAuth, error)
	// SaveRealNameAuth 覆盖写账号实名子文档。
	SaveRealNameAuth(ctx context.Context, accountID string, auth *RealNameAuth) error
	// FindValidHolderByIDNo 返回持有同证件的其他有效主账号 ID（同证件唯一，D5）。
	// idNoHash 比对新写入的哈希字段；idNoPlain 仅用于存量明文兼容比对，不落库。
	// 有效 = 非「认证失败」；excludeAccountID 排除本人重提。
	FindValidHolderByIDNo(ctx context.Context, idNoHash, idNoPlain, excludeAccountID string) (string, error)
}

// RealNameVerdict 二要素校验适配器结论。
type RealNameVerdict string

const (
	RealNameVerdictPending RealNameVerdict = "PENDING"
	RealNameVerdictPassed  RealNameVerdict = "PASSED"
	RealNameVerdictFailed  RealNameVerdict = "FAILED"
)

// RealNameVerifier 二要素校验适配器（契约 1.6 外部适配层）。
// 超时/错误由用例层兜底为「认证中」，再查询结果。
type RealNameVerifier interface {
	Verify(ctx context.Context, name, idNo string) (RealNameVerdict, error)
}
