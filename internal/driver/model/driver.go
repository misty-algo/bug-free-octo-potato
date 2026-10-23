package model

import "time"

// ── MongoDB 集合名（day1 库） ──
// 说明：day1 是已有练习库，旧的 drivers / withdrawals 等集合带严格 $jsonSchema 校验器，
// 司机端使用独立命名的新集合避免冲突。集合名命名遵循《技术方案》中的域划分。

const (
	DriverCollectionName      = "driver_profiles"
	HallOrderCollectionName   = "hall_orders"
	IncomeFlowCollectionName  = "income_flows"
	WithdrawCollectionName    = "withdraw_requests"
	LedgerCollectionName      = "driver_ledgers"
	OutboxCollectionName      = "driver_outbox_events"
	InboundCollectionName     = "driver_inbound_events"
	IdempotencyCollectionName = "driver_idempotency_keys"
)

// AreaBoxDoc 片区包围盒。
type AreaBoxDoc struct {
	Name   string  `bson:"name"`
	MinLat float64 `bson:"min_lat"`
	MinLng float64 `bson:"min_lng"`
	MaxLat float64 `bson:"max_lat"`
	MaxLng float64 `bson:"max_lng"`
}

// ReleaseRecordDoc 退单记录。
type ReleaseRecordDoc struct {
	At    time.Time `bson:"at"`
	Party string    `bson:"party"`
}

// FundsDoc 司机资金五桶（单位：分）。
type FundsDoc struct {
	PendingFen   int64 `bson:"pending_fen"`
	AvailableFen int64 `bson:"available_fen"`
	FrozenFen    int64 `bson:"frozen_fen"`
	WithdrawFen  int64 `bson:"withdraw_fen"`
	DebtFen      int64 `bson:"debt_fen"`
}

// DriverDocument 是 driver_profiles 集合文档。
type DriverDocument struct {
	ID          string       `bson:"_id"`
	DriverNo    string       `bson:"driver_no"`
	Name        string       `bson:"name"`
	Phone       string       `bson:"phone"`
	Status      string       `bson:"status"`       // offline / idle / busy / pre_occupied
	Mode        string       `bson:"mode"`         // listen / grab
	ModeVersion int64        `bson:"mode_version"` // 模式版本：切换递增，使旧邀请失效
	AreaLimitSwitch bool         `bson:"area_limit_switch"`
	Areas          []AreaBoxDoc `bson:"areas"`
	CityCode       string       `bson:"city_code"`
	CarType        string       `bson:"car_type"`
	Seats          int          `bson:"seats"`
	Qualified      bool         `bson:"qualified"`
	CanAccept      bool         `bson:"can_accept"`
	OfflineAfterTrip bool       `bson:"offline_after_trip"`
	LastHeartbeatAt  time.Time  `bson:"last_heartbeat_at"`
	LastLat          float64    `bson:"last_lat"`
	LastLng          float64    `bson:"last_lng"`
	ActiveTaskID     string     `bson:"active_task_id"`
	OnlineSinceAt    time.Time  `bson:"online_since_at"`
	RecentReleases    []ReleaseRecordDoc `bson:"recent_releases"`
	MonthlyIncomeFen int64      `bson:"monthly_income_fen"`
	Rating           float64   `bson:"rating"`
	SuccessRate      float64   `bson:"success_rate"`
	Funds            FundsDoc  `bson:"funds"`
	Version          int64     `bson:"version"`
	CreatedAt        time.Time `bson:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

func (DriverDocument) MongoCollectionName() string { return DriverCollectionName }
