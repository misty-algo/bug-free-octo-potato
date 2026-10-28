package entity

import (
	"context"
	"time"
)

// ChannelBinding 渠道绑定条目（accounts.channelBindings[]，绑定/解绑由登录模块 3.1.6 写入）。
type ChannelBinding struct {
	Channel    string
	ChannelUID string
	BoundAt    time.Time
}

// AccountSecurity 账号安全信息（P21，契约 3.7.2）。
type AccountSecurity struct {
	Bindings      []ChannelBinding // 渠道绑定列表
	ProtocolVer   string           // 协议版本
	RealNameState string           // 实名状态（未认证/认证中/已认证/认证失败）
}

// AccountSecurityRepository 账号安全读写端口（P21，落 accounts 集合）。
// 绑定/解绑本体是登录模块 3.1.6 的用例，本端口只读列表、记协议版本。
type AccountSecurityRepository interface {
	// GetAccountSecurity 读取绑定列表/协议版本/实名状态；无记录返回 (nil, nil)。
	GetAccountSecurity(ctx context.Context, accountID string) (*AccountSecurity, error)
	// SaveProtocolVer 记录协议同意版本（幂等由用例层保证，同版本不重复写）。
	SaveProtocolVer(ctx context.Context, accountID, protocolVer string) error
}
