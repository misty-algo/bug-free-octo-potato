package model

import "time"

// IncomeFlowDocument 收入流水凭证（只追加，不可变）。
type IncomeFlowDocument struct {
	ID       string `bson:"_id"`
	DriverID string `bson:"driver_id"`
	OrderID  string `bson:"order_id"`
	TaskID   string `bson:"task_id"`
	FlowType string `bson:"flow_type"`
	ReverseOf string `bson:"reverse_of"`

	OrigFareFen       int64 `bson:"orig_fare_fen"`
	PassengerPaidFen  int64 `bson:"passenger_paid_fen"`
	PlatformCouponFen int64 `bson:"platform_coupon_fen"`
	DriverCouponFen   int64 `bson:"driver_coupon_fen"`
	CommissionFen     int64 `bson:"commission_fen"`
	SubsidyFen        int64 `bson:"subsidy_fen"`
	DriverIncomeFen   int64 `bson:"driver_income_fen"`

	AccountingDate string `bson:"accounting_date"`
	BucketFrom     string `bson:"bucket_from"`
	BucketTo       string `bson:"bucket_to"`
	EventID        string `bson:"event_id"`
	IdempotencyKey string `bson:"idempotency_key"`
	Reason         string `bson:"reason"`
	CreatedAt      time.Time `bson:"created_at"`
}

func (IncomeFlowDocument) MongoCollectionName() string { return IncomeFlowCollectionName }

// ApprovalDoc 提现审批留痕。
type ApprovalDoc struct {
	ApproverID string    `bson:"approver_id"`
	Comment    string    `bson:"comment"`
	ApprovedAt time.Time `bson:"approved_at"`
}

// WithdrawDocument 提现申请单。
type WithdrawDocument struct {
	ID             string        `bson:"_id"`
	DriverID       string        `bson:"driver_id"`
	AmountFen      int64         `bson:"amount_fen"`
	Status         string        `bson:"status"`
	DualApproval   bool          `bson:"dual_approval"`
	Approvals      []ApprovalDoc `bson:"approvals"`
	IdempotencyKey string        `bson:"idempotency_key"`
	Reason         string        `bson:"reason"`
	CreatedAt      time.Time     `bson:"created_at"`
	UpdatedAt      time.Time     `bson:"updated_at"`
}

func (WithdrawDocument) MongoCollectionName() string { return WithdrawCollectionName }

// LedgerDocument 资金分录（借贷平衡的追加记录）。
type LedgerDocument struct {
	ID             string    `bson:"_id"`
	DriverID       string    `bson:"driver_id"`
	FromBucket     string    `bson:"from_bucket"`
	ToBucket       string    `bson:"to_bucket"`
	AmountFen      int64     `bson:"amount_fen"`
	RefType        string    `bson:"ref_type"`
	RefID          string    `bson:"ref_id"`
	IdempotencyKey string    `bson:"idempotency_key"`
	CreatedAt      time.Time `bson:"created_at"`
}

func (LedgerDocument) MongoCollectionName() string { return LedgerCollectionName }

// OutboxEventDocument 可靠事件表。
type OutboxEventDocument struct {
	EventID     string    `bson:"_id"`
	EventType   string    `bson:"event_type"`
	Payload     string    `bson:"payload"`
	Version     int64     `bson:"version"`
	Status      string    `bson:"status"`
	ErrorMsg    string    `bson:"error_msg"`
	RetryTimes  int       `bson:"retry_times"`
	CreatedAt   time.Time `bson:"created_at"`
	PublishedAt time.Time `bson:"published_at"`
}

func (OutboxEventDocument) MongoCollectionName() string { return OutboxCollectionName }

// InboundEventDocument 已消费外部事件表（去重）。
type InboundEventDocument struct {
	EventID     string    `bson:"_id"`
	EventType   string    `bson:"event_type"`
	Consumer    string    `bson:"consumer"`
	ProcessedAt time.Time `bson:"processed_at"`
}

func (InboundEventDocument) MongoCollectionName() string { return InboundCollectionName }

// CommandDocument 幂等命令表。
type CommandDocument struct {
	Key       string    `bson:"_id"`
	Scope     string    `bson:"scope"`
	TargetID  string    `bson:"target_id"`
	ResultRef string    `bson:"result_ref"`
	CreatedAt time.Time `bson:"created_at"`
}

func (CommandDocument) MongoCollectionName() string { return IdempotencyCollectionName }
