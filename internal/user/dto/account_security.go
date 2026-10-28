package dto

// ChannelBindingStatus 是渠道绑定展示条目（wechat/qq/sms，契约 3.7.2）。
type ChannelBindingStatus struct {
	Channel string // wechat / qq / sms
	Bound   bool
	BoundAt int64 // 绑定时间（unix 秒）；未绑定为 0
}

// AccountSecurityStatus 是 P21 GET /api/pax/account-security 出参。
type AccountSecurityStatus struct {
	Bindings      []ChannelBindingStatus
	ProtocolVer   string
	RealNameState string // 未认证/认证中/已认证/认证失败
}

// AgreementRecord 是 P21 协议同意记录出参（POST /api/pax/account-security）。
type AgreementRecord struct {
	ProtocolVer string
}
