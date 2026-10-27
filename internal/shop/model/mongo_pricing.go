package model

import "time"

const pricingRulesMongoCollectionName = "pricing_rules"

// PricingRulesDocument 是 pricing_rules 集合（计价规则）的 MongoDB 文档模型。
type PricingRulesDocument struct {
	ID            string    `bson:"_id"`
	RuleNo        string    `bson:"rule_no"`
	Name          string    `bson:"name"`
	CityCode      string    `bson:"city_code"`
	ProductCode   string    `bson:"product_code"`
	Version       int64     `bson:"version"`
	Status        string    `bson:"status"`
	EffectiveTime time.Time `bson:"effective_time"`
	ExpireTime    time.Time `bson:"expire_time"`

	// 基础计价
	BaseFee            int64            `bson:"base_fee"`
	BaseDistance       int64            `bson:"base_distance"`
	BaseDuration       int64            `bson:"base_duration"`
	MileageSegments    []MileageSegment `bson:"mileage_segments"`
	DurationRules      []DurationRule   `bson:"duration_rules"`
	LongDistanceRule   LongDistanceRule `bson:"long_distance_rule"`
	LowSpeedRule       LowSpeedRule     `bson:"low_speed_rule"`
	NightRule          NightRule        `bson:"night_rule"`
	MinFee             int64            `bson:"min_fee"`
	MaxFee             int64            `bson:"max_fee"`
	WaitingFreeMinutes int64            `bson:"waiting_free_minutes"`
	WaitingFeePerMin   int64            `bson:"waiting_fee_per_min"`
	ExtraFeeTypes      []string         `bson:"extra_fee_types"`

	// 司机结算
	CommissionRate          float64     `bson:"commission_rate"`
	DriverMinIncomePerOrder int64       `bson:"driver_min_income_per_order"`
	SubsidyRule             SubsidyRule `bson:"subsidy_rule"`
	Remark                  string      `bson:"remark"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (PricingRulesDocument) MongoCollectionName() string {
	return pricingRulesMongoCollectionName
}

// MileageSegment 表示 pricing_rules.mileageSegments 分段里程。
type MileageSegment struct {
	FromKm     int64 `bson:"from_km"`
	ToKm       int64 `bson:"to_km"`
	PricePerKm int64 `bson:"price_per_km"`
}

// DurationRule 表示 pricing_rules.durationRules 分时段时长费。
type DurationRule struct {
	Period      string `bson:"period"`
	Start       string `bson:"start"`
	End         string `bson:"end"`
	PricePerMin int64  `bson:"price_per_min"`
}

// LongDistanceRule 表示 pricing_rules.longDistanceRule 远途费。
type LongDistanceRule struct {
	FromKm     int64 `bson:"from_km"`
	PricePerKm int64 `bson:"price_per_km"`
}

// LowSpeedRule 表示 pricing_rules.lowSpeedRule 低速费。
type LowSpeedRule struct {
	SpeedThreshold int64 `bson:"speed_threshold"`
	PricePerMin    int64 `bson:"price_per_min"`
}

// NightRule 表示 pricing_rules.nightRule 夜间费。
type NightRule struct {
	Start      string `bson:"start"`
	End        string `bson:"end"`
	ExtraPerKm int64  `bson:"extra_per_km"`
}

// SubsidyRule 表示 pricing_rules.subsidyRule 补贴规则。
type SubsidyRule struct {
	Enabled     bool   `bson:"enabled"`
	MaxSubsidy  int64  `bson:"max_subsidy"`
	Description string `bson:"description"`
}

const pricingSnapshotsMongoCollectionName = "pricing_snapshots"

// PricingSnapshotsDocument 是 pricing_snapshots 集合（计价规则快照）的 MongoDB 文档模型。
type PricingSnapshotsDocument struct {
	ID          string                 `bson:"_id"`
	OrderID     string                 `bson:"order_id"`
	RuleID      string                 `bson:"rule_id"`
	RuleVersion int64                  `bson:"rule_version"`
	Snapshot    map[string]interface{} `bson:"snapshot"`
	SurgeRatio  float64                `bson:"surge_ratio"`
	CreatedAt   time.Time              `bson:"created_at"`
	UpdatedAt   time.Time              `bson:"updated_at"`
	CreatedBy   string                 `bson:"created_by"`
	UpdatedBy   string                 `bson:"updated_by"`
	IsDeleted   bool                   `bson:"is_deleted"`
	Version     int64                  `bson:"version"`
}

func (PricingSnapshotsDocument) MongoCollectionName() string {
	return pricingSnapshotsMongoCollectionName
}

const surgeRulesMongoCollectionName = "surge_rules"

// SurgeRulesDocument 是 surge_rules 集合（动态调价规则）的 MongoDB 文档模型。
type SurgeRulesDocument struct {
	ID                string     `bson:"_id"`
	CityCode          string     `bson:"city_code"`
	ProductCode       string     `bson:"product_code"`
	GeoHash           string     `bson:"geo_hash"`
	DemandSupplyRatio float64    `bson:"demand_supply_ratio"`
	SurgeRatio        float64    `bson:"surge_ratio"`
	MaxSurgeRatio     float64    `bson:"max_surge_ratio"`
	TimeWindow        TimeWindow `bson:"time_window"`
	WeatherTrigger    []string   `bson:"weather_trigger"`
	Enabled           bool       `bson:"enabled"`
	Sort              int64      `bson:"sort"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (SurgeRulesDocument) MongoCollectionName() string {
	return surgeRulesMongoCollectionName
}

// TimeWindow 表示 surge_rules.timeWindow 生效时段。
type TimeWindow struct {
	Start string `bson:"start"`
	End   string `bson:"end"`
}

const cancelRulesMongoCollectionName = "cancel_rules"

// CancelRulesDocument 是 cancel_rules 集合（取消费规则）的 MongoDB 文档模型。
type CancelRulesDocument struct {
	ID                string `bson:"_id"`
	CityCode          string `bson:"city_code"`
	ProductCode       string `bson:"product_code"`
	CancelSide        string `bson:"cancel_side"`
	StatusFrom        string `bson:"status_from"`
	FreeWindowSeconds int64  `bson:"free_window_seconds"`
	FeeType           string `bson:"fee_type"`
	FeeValue          int64  `bson:"fee_value"`
	MaxFee            int64  `bson:"max_fee"`
	Enabled           bool   `bson:"enabled"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CancelRulesDocument) MongoCollectionName() string {
	return cancelRulesMongoCollectionName
}

const billsMongoCollectionName = "bills"

// BillsDocument 是 bills 集合（账单）的 MongoDB 文档模型。
type BillsDocument struct {
	ID            string `bson:"_id"`
	BillNo        string `bson:"bill_no"`
	OrderID       string `bson:"order_id"`
	UserID        string `bson:"user_id"`
	DriverID      string `bson:"driver_id"`
	CityCode      string `bson:"city_code"`
	ProductCode   string `bson:"product_code"`
	BillType      string `bson:"bill_type"`
	TotalFee      int64  `bson:"total_fee"`
	DiscountFee   int64  `bson:"discount_fee"`
	PayFee        int64  `bson:"pay_fee"`
	RefundFee     int64  `bson:"refund_fee"`
	DriverIncome  int64  `bson:"driver_income"`
	CommissionFee int64  `bson:"commission_fee"`
	SubsidyFee    int64  `bson:"subsidy_fee"`
	Status        string `bson:"status"`
	Version       int64  `bson:"version"`
	Settled       bool   `bson:"settled"`
	SettlementID  string `bson:"settlement_id"`
	Remark        string `bson:"remark"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (BillsDocument) MongoCollectionName() string {
	return billsMongoCollectionName
}

const billItemsMongoCollectionName = "bill_items"

// BillItemsDocument 是 bill_items 集合（账单明细项）的 MongoDB 文档模型。
type BillItemsDocument struct {
	ID        string `bson:"_id"`
	BillID    string `bson:"bill_id"`
	OrderID   string `bson:"order_id"`
	ItemType  string `bson:"item_type"`
	ItemName  string `bson:"item_name"`
	Amount    int64  `bson:"amount"`
	Formula   string `bson:"formula"`
	Quantity  int64  `bson:"quantity"`
	UnitPrice int64  `bson:"unit_price"`
	CouponID  string `bson:"coupon_id"`
	Sort      int64  `bson:"sort"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (BillItemsDocument) MongoCollectionName() string {
	return billItemsMongoCollectionName
}
