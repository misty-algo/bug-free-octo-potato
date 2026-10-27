package entity

import (
	"time"
)

// Estimate 是估价单。它同时是价格承诺凭证与防重复下单凭证，
// MUST 归属账号，游客只能看到即时价格，拿不到估价单。
type Estimate struct {
	ID        string
	No       string
	AccountID string
	CarType  int
	// BaseFee 为起步价加里程费加时长费的合计，单位分。
	BaseFee int64
	// ServiceFee 为平台服务费，单位分。
	ServiceFee int64
	// DiscountFee 为优惠抵扣，单位分。
	DiscountFee int64
	// PayFee 为应付总额，单位分。
	PayFee int64
	// DistanceM 为距离，单位米。
	DistanceM int64
	// DurationS 为时长，单位秒。
	DurationS int64
	// PricingVersion 冻结命中的计价规则版本。
	PricingVersion string
	// CouponID 为选中的优惠券标识，可空。
	CouponID string
	// Used 标记估价单是否已被消费。
	Used bool
	ExpiresAt time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// NewEstimate 校验并创建估价单。
func NewEstimate(id, no, accountID string, carType int, now time.Time) (*Estimate, error) {
	if accountID == "" || no == "" {
		return nil, ErrInvalidStateTransition
	}
	now = now.UTC()
	return &Estimate{
		ID:        id,
		No:        no,
		AccountID: accountID,
		CarType:   carType,
		ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Expired 报告估价单是否已过期。有效期由服务端时间判断，
// 存储层的 TTL 清理只回收空间，不充当校验依据。
func (e *Estimate) Expired(now time.Time) bool {
	return e != nil && !now.UTC().Before(e.ExpiresAt)
}

// Available 报告估价单此刻能否用于下单。
func (e *Estimate) Available(now time.Time) bool {
	return e != nil && !e.Used && !e.Expired(now)
}

// AddressType 表示地址类型。
type AddressType string

// 地址类型常量。
const (
	AddressTypeHome    AddressType = "home"
	AddressTypeCompany AddressType = "company"
	AddressTypeOther   AddressType = "other"
	AddressTypeHistory AddressType = "history"
)

// Valid 报告地址类型是否合法。
func (t AddressType) Valid() bool {
	return t == AddressTypeHome || t == AddressTypeCompany || t == AddressTypeOther || t == AddressTypeHistory
}

// Address 是常用地址与历史地址。两者用途不同但同属地址簿，
// 均按账号隔离做对象级授权。
type Address struct {
	ID        string
	AccountID string
	Type      AddressType
	Tag       string
	Name      string
	Addr      string
	POIID     string
	Lng       float64
	Lat       float64
	// UseCount 为选择次数，历史地址按它与最近使用时间排序。
	UseCount    int
	LastUsedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AddressLimit 为账号可保存的常用地址上限。
const AddressLimit = 20

// NewAddress 校验并创建地址。
func NewAddress(id, accountID string, addrType AddressType, tag, name, addr, poiID string, lng, lat float64, now time.Time) (*Address, error) {
	if accountID == "" || !addrType.Valid() {
		return nil, ErrInvalidStateTransition
	}
	now = now.UTC()
	return &Address{
		ID:         id,
		AccountID:  accountID,
		Type:       addrType,
		Tag:        tag,
		Name:       name,
		Addr:       addr,
		POIID:      poiID,
		Lng:        lng,
		Lat:        lat,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// POI 是地点。搜索返回前十条，带坐标时按距离由近到远排序，
// 无坐标时仍返回但不展示虚假距离。
type POI struct {
	ID   string
	Name string
	Addr string
	City string
	Lng  float64
	Lat  float64
	// DistanceM 为距当前位置的距离，单位米。仅在有坐标时有效。
	DistanceM int64
}

// QuickEntry 是首页首次快速入口的展示项。条目由后台配置，
// 乘客端只读；点过之后对该账号不再展示。
type QuickEntry struct {
	EntryID string
	Title   string
	Icon    string
	// Action 为 scan 或 navigate，navigate 时用 Target 指向页面。
	Action string
	Target string
	Sort   int
}

// 快速入口动作常量。
const (
	QuickEntryActionScan     = "scan"
	QuickEntryActionNavigate = "navigate"
)
