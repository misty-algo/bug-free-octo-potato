// Package dto 定义乘客端的入参命令与出参结构。
package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// SendCodeCommand 是发送短信验证码的入参。
type SendCodeCommand struct {
	Phone string
	// DeviceID 用于同设备小时级限流。
	DeviceID string
}

// SendCodeDTO 是发送验证码的出参。
type SendCodeDTO struct {
	ExpiresInSeconds   int `json:"expiresInSeconds"`
	ResendAfterSeconds int `json:"resendAfterSeconds"`
	// DebugCode 仅在本地 Mock 通道下回传，接入真实网关后为空。
	DebugCode string `json:"debugCode,omitempty"`
}

// LoginCommand 是登录的入参。
type LoginCommand struct {
	// Channel 决定用哪种凭据：sms 走手机号验证码，wechat/qq 走授权码。
	Channel entity.Channel
	Phone   string
	// Code 为短信验证码或第三方授权码。
	Code string
	// Password 为密码登录的明文密码。
	Password string
	// Role 决定本次会话进入乘客端还是司机端。
	Role entity.Role
}

// LoginDTO 是登录的出参。
type LoginDTO struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	AccountID    string `json:"accountId"`
	IsNewUser    bool   `json:"isNewUser"`
	HasPassword  bool   `json:"hasPassword"`
	// RestoreTarget 为服务端计算的恢复目标，客户端只按它跳转。
	RestoreTarget string `json:"restoreTarget"`
	// NeedDriverRegister 为选司机身份但账号无司机资料时为真。
	NeedDriverRegister bool `json:"needDriverRegister"`
	// ExpiresInSeconds 为访问令牌剩余有效期。
	ExpiresInSeconds int `json:"expiresInSeconds"`
}

// SetPasswordCommand 是设置登录密码的入参。
type SetPasswordCommand struct {
	AccountID string
	Phone     string
	Code      string
	Password  string
}

// RefreshCommand 是刷新令牌的入参。
type RefreshCommand struct {
	RefreshToken string
	// Audience 声明刷新方所属受众，服务端校验，防止跨端刷新。
	Audience entity.Audience
}

// LogoutCommand 是退出登录的入参。
type LogoutCommand struct {
	AccountID string
}

// HomeCommand 是获取首页数据的入参。
type HomeCommand struct {
	AccountID string
	// Lng / Lat 为当前定位，可空。
	Lng float64
	Lat float64
	// HasLocation 标识前端是否成功取得定位。
	HasLocation bool
}

// CarTypeDTO 是可选车型的出参。
type CarTypeDTO struct {
	CarType int    `json:"carType"`
	Name    string `json:"name"`
	// Available 标识该车型当前是否可用。
	Available bool `json:"available"`
	// UnavailableReason 在不可用时说明原因，前端据此置灰。
	UnavailableReason string `json:"unavailableReason,omitempty"`
	// MinFee 为起步价，单位分。
	MinFee int64 `json:"minFee"`
}

// QuickEntryDTO 是首次快速入口的出参。
type QuickEntryDTO struct {
	EntryID string `json:"entryId"`
	Title   string `json:"title"`
	Icon    string `json:"icon"`
	Action  string `json:"action"`
	Target  string `json:"target"`
}

// AddressDTO 是地址的出参。
type AddressDTO struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Tag      string  `json:"tag"`
	Name     string  `json:"name"`
	Addr     string  `json:"addr"`
	POIID    string  `json:"poiId"`
	Lng      float64 `json:"lng"`
	Lat      float64 `json:"lat"`
	UseCount int     `json:"useCount"`
}

// POIDTO 是地点的出参。
type POIDTO struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Addr string  `json:"addr"`
	City string  `json:"city"`
	Lng  float64 `json:"lng"`
	Lat  float64 `json:"lat"`
	// HasDistance 标识距离是否可信。无坐标时距离不可算，MUST NOT 返回虚假距离。
	HasDistance bool  `json:"hasDistance"`
	DistanceM   int64 `json:"distanceM"`
}

// HomeDTO 是首页合并数据的出参，一次返回避免前端多次请求拼装。
type HomeDTO struct {
	// CarTypes 为当前城市可用车型。
	CarTypes []CarTypeDTO `json:"carTypes"`
	// QuickEntries 为该账号没用过的首次快速入口。
	QuickEntries []QuickEntryDTO `json:"quickEntries"`
	// CommonAddresses 为常用地址，最多二十条。
	CommonAddresses []AddressDTO `json:"commonAddresses"`
	// HistoryAddresses 为历史地址，按选择次数与最近使用时间排序，最多十条。
	HistoryAddresses []AddressDTO `json:"historyAddresses"`
	// HasActiveOrder 标识是否存在进行中的订单。
	HasActiveOrder bool `json:"hasActiveOrder"`
	// ActiveOrderID 为进行中订单标识，可空。
	ActiveOrderID string `json:"activeOrderId"`
	// RestoreTarget 为恢复目标：home 表示无进行中流程。
	RestoreTarget string `json:"restoreTarget"`
}

// SearchPOICommand 是地点搜索的入参。
type SearchPOICommand struct {
	AccountID string
	Keyword   string
	City      string
	Lng       float64
	Lat       float64
	HasCoord  bool
	Limit     int
}

// EstimateCommand 是预估计价的入参。
type EstimateCommand struct {
	AccountID string
	CarType   int
	StartName string
	StartAddr string
	StartLng  float64
	StartLat  float64
	EndName   string
	EndAddr   string
	EndLng    float64
	EndLat    float64
	CouponID  string
	// IdempotencyKey 保证重复提交只产生一张估价单。
	IdempotencyKey string
}

// EstimateDTO 是预估计价的出参。
type EstimateDTO struct {
	// EstimateNo 为估价单号，游客为空。估价单必须归属账号。
	EstimateNo string `json:"estimateNo"`
	CarType    int    `json:"carType"`
	// BaseFee / ServiceFee / DiscountFee / PayFee 单位均为分。
	BaseFee     int64 `json:"baseFee"`
	ServiceFee  int64 `json:"serviceFee"`
	DiscountFee int64 `json:"discountFee"`
	PayFee      int64 `json:"payFee"`
	DistanceM   int64 `json:"distanceM"`
	DurationS   int64 `json:"durationS"`
	// ExpiresAt 为估价单过期时间，游客为空。
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}

// CreateOrderCommand 是创建订单的入参。
type CreateOrderCommand struct {
	AccountID  string
	EstimateNo string
	CarType    int
	StartName  string
	StartAddr  string
	StartLng   float64
	StartLat   float64
	EndName    string
	EndAddr    string
	EndLng     float64
	EndLat     float64
	Remark     string
	// IdempotencyKey 为业务幂等键，相同键重试返回同一订单。
	IdempotencyKey string
}

// OrderDTO 是订单的出参。
type OrderDTO struct {
	OrderID   string `json:"orderId"`
	OrderNo   string `json:"orderNo"`
	State     string `json:"state"`
	CarType   int    `json:"carType"`
	StartName string `json:"startName"`
	StartAddr string `json:"startAddr"`
	EndName   string `json:"endName"`
	EndAddr   string `json:"endAddr"`
	// TotalFee / CancelFee 单位为分。
	TotalFee  int64  `json:"totalFee"`
	CancelFee int64  `json:"cancelFee"`
	Remark    string `json:"remark"`
	// DriverName / DriverPlate 已接单后才有值。
	DriverName  string    `json:"driverName"`
	DriverPlate string    `json:"driverPlate"`
	CreatedAt   time.Time `json:"createdAt"`
}

// CancelOrderCommand 是取消订单的入参。
type CancelOrderCommand struct {
	AccountID string
	OrderID   string
	Reason    string
	// ConfirmFee 为带消费费的二次确认标记。
	ConfirmFee bool
}

// CancelQuoteDTO 是取消报价的出参，两段式取消的第一步。
type CancelQuoteDTO struct {
	// NeedConfirm 为真时前端弹确认框，带 confirmFee 再调一次才真正取消。
	NeedConfirm bool   `json:"needConfirm"`
	CancelFee   int64  `json:"cancelFee"`
	State       string `json:"state"`
	// ExpiresAt 为报价有效期，过期后需要重新取价。
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}

// OrderListDTO 是订单列表的出参。
type OrderListDTO struct {
	Total int         `json:"total"`
	List  []*OrderDTO `json:"list"`
}

// CouponDTO 是优惠券的出参。
// MarkOrderPaidCommand 是支付成功回调的入参：按订单号定位订单。
type MarkOrderPaidCommand struct {
	// OrderNo 为乘客域订单号（支付单以业务订单号关联）。
	OrderNo string
	// PaymentNo / TradeNo 为支付域支付单号与渠道流水号，仅留痕。
	PaymentNo string
	TradeNo   string
	// PaidFen 为渠道实付金额，单位分；0 表示未提供，以订单应付为准。
	PaidFen int64
}

// OrderPaidResult 是支付成功联动结果：订单推进到已完成，
// 附带司机域结算要素，供网关接着调用司机域记账。
type OrderPaidResult struct {
	OrderID string `json:"orderId"`
	OrderNo string `json:"orderNo"`
	// State 推进后的订单状态（COMPLETED）。
	State string `json:"state"`
	// DriverID 为接单司机；任务不存在时为空，调用方应跳过司机入账。
	DriverID string `json:"driverId,omitempty"`
	// PaidFen / DriverIncomeFen 单位分：乘客实付与司机净得（已扣平台佣金）。
	PaidFen         int64 `json:"paidFen"`
	DriverIncomeFen int64 `json:"driverIncomeFen"`
}

type CouponDTO struct {
	CouponID string `json:"couponId"`
	Name     string `json:"name"`
	// FaceValue 与 Threshold 单位为分。
	FaceValue int64     `json:"faceValue"`
	Threshold int64     `json:"threshold"`
	ExpiresAt time.Time `json:"expiresAt"`
	// Claimed 标记当前账号是否已领取过（领券中心置灰用，仅 claimable 列表填充）。
	Claimed bool `json:"claimed,omitempty"`
}

// SaveAddressCommand 是新增或更新常用地址的入参。
type SaveAddressCommand struct {
	ID       string
	AddrType string
	Tag      string
	Name     string
	Addr     string
	POIID    string
	Lng      float64
	Lat      float64
}

// RecordHistoryCommand 是上报一次选中的地点的入参。
type RecordHistoryCommand struct {
	POIID string
	Name  string
	Addr  string
	Lng   float64
	Lat   float64
}

// BindPhoneDTO 是绑定手机号的出参。手机号一律脱敏返回。
type BindPhoneDTO struct {
	Phone string `json:"phone"`
}

// DriverLocationDTO 是司机实时位置的出参。
type DriverLocationDTO struct {
	DriverID   string  `json:"driverId"`
	DriverName string  `json:"driverName"`
	PlateNo    string  `json:"plateNo"`
	CarModel   string  `json:"carModel"`
	Rating     string  `json:"rating"`
	Lng        float64 `json:"lng"`
	Lat        float64 `json:"lat"`
	// ETAMin 为预计到达分钟数。
	ETAMin int `json:"etaMin"`
	// Delayed 为真时前端提示「位置更新延迟」。
	Delayed bool `json:"delayed"`
	// ReportedAt 为该位置的上报时间。
	ReportedAt time.Time `json:"reportedAt"`
}
