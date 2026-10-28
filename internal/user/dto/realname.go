package dto

// SubmitRealNameCommand 是 P20 提交实名入参（POST /api/pax/realname，契约 3.7.1）。
type SubmitRealNameCommand struct {
	AccountID      string
	Name           string
	IDNo           string
	IdempotencyKey string
}

// RealNameStatus 是 P20 出参：state + 脱敏姓名/证件号（响应不回真值）。
type RealNameStatus struct {
	State    string // 未认证/认证中/已认证/认证失败
	NameMask string // 脱敏姓名
	IDNoMask string // 脱敏证件号
}
