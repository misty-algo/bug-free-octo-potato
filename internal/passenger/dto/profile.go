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

// ClaimCouponCommand 是领取平台券的入参。
type ClaimCouponCommand struct {
	AccountID string
	// CouponNo 为平台侧券码，同券码重复领取返回已有券（幂等）。
	CouponNo string
}
