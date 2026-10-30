package dto

// SubmitRealNameCommand 是 P20 提交实名的入参。
type SubmitRealNameCommand struct {
	AccountID string
	Name      string
	// IDNo 为证件号；服务端只落脱敏值，不保存真值。
	IDNo string
	// IdempotencyKey 由客户端生成；提交本身幂等，重复提交返回当前状态。
	IdempotencyKey string
}

// RealNameStatusDTO 是实名状态出参，只含状态与脱敏值。
type RealNameStatusDTO struct {
	// State: UNVERIFIED / VERIFIED
	State      string `json:"state"`
	NameMask   string `json:"nameMask"`
	IDNoMask   string `json:"idNoMask"`
}

// 实名状态枚举。
const (
	RealNameStateUnverified = "UNVERIFIED"
	RealNameStateVerified   = "VERIFIED"
)

// UpdateProfileCommand 是注册引导补全资料的入参。
type UpdateProfileCommand struct {
	AccountID string
	Nickname  string
	// Avatar 为头像标识（emoji 或预设图标），可空。
	Avatar string
	// EmergencyName / EmergencyPhone 为紧急联系人，成对提交。
	EmergencyName  string
	EmergencyPhone string
}

// ProfileDTO 是个人资料出参，紧急联系电话只回传掩码值。
type ProfileDTO struct {
	AccountID          string `json:"account_id"`
	Nickname           string `json:"nickname"`
	Avatar             string `json:"avatar"`
	EmergencyName      string `json:"emergency_name"`
	EmergencyPhoneMask string `json:"emergency_phone_mask"`
	// Completed 表示注册引导是否已填完。
	Completed bool `json:"completed"`
}

// ClaimCouponCommand 是领取平台券的入参。
type ClaimCouponCommand struct {
	AccountID string
	// CouponNo 为平台侧券码，同券码重复领取返回已有券（幂等）。
	CouponNo string
}
