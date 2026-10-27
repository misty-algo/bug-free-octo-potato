package model

import "time"

const driverRewardPenaltiesMongoCollectionName = "driver_reward_penalties"

// DriverRewardPenaltiesDocument 是 driver_reward_penalties 集合（司机奖惩记录）的 MongoDB 文档模型。
type DriverRewardPenaltiesDocument struct {
	ID           string    `bson:"_id"`
	RpNo         string    `bson:"rp_no"`
	DriverID     string    `bson:"driver_id"`
	Direction    string    `bson:"direction"`
	BizType      string    `bson:"biz_type"`
	CampaignID   string    `bson:"campaign_id"`
	OrderID      string    `bson:"order_id"`
	Amount       int64     `bson:"amount"`
	ScoreDelta   int64     `bson:"score_delta"`
	Reason       string    `bson:"reason"`
	Evidence     []string  `bson:"evidence"`
	Status       string    `bson:"status"`
	EffectiveAt  time.Time `bson:"effective_at"`
	ExpireAt     time.Time `bson:"expire_at"`
	AppealReason string    `bson:"appeal_reason"`
	AppealResult string    `bson:"appeal_result"`
	AppealAt     time.Time `bson:"appeal_at"`
	ReversedAt   time.Time `bson:"reversed_at"`
	OperatorID   string    `bson:"operator_id"`
	OperatorType string    `bson:"operator_type"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverRewardPenaltiesDocument) MongoCollectionName() string {
	return driverRewardPenaltiesMongoCollectionName
}

const dispatchQueuesMongoCollectionName = "dispatch_queues"

// DispatchQueuesDocument 是 dispatch_queues 集合（高峰排队队列）的 MongoDB 文档模型。
type DispatchQueuesDocument struct {
	ID               string    `bson:"_id"`
	QueueNo          string    `bson:"queue_no"`
	OrderID          string    `bson:"order_id"`
	UserID           string    `bson:"user_id"`
	CityCode         string    `bson:"city_code"`
	ProductCode      string    `bson:"product_code"`
	QueuePosition    int64     `bson:"queue_position"`
	EstimatedWaitSec int64     `bson:"estimated_wait_sec"`
	Status           string    `bson:"status"`
	JoinedAt         time.Time `bson:"joined_at"`
	CalledAt         time.Time `bson:"called_at"`
	ExpireAt         time.Time `bson:"expire_at"`
	NotifyCount      int64     `bson:"notify_count"`
	DispatchID       string    `bson:"dispatch_id"`
	Priority         int64     `bson:"priority"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DispatchQueuesDocument) MongoCollectionName() string {
	return dispatchQueuesMongoCollectionName
}

const driverScheduleTasksMongoCollectionName = "driver_schedule_tasks"

// DriverScheduleTasksDocument 是 driver_schedule_tasks 集合（司机排班与时长任务）的 MongoDB 文档模型。
type DriverScheduleTasksDocument struct {
	ID           string    `bson:"_id"`
	TaskNo       string    `bson:"task_no"`
	DriverID     string    `bson:"driver_id"`
	CityCode     string    `bson:"city_code"`
	TaskDate     string    `bson:"task_date"`
	TaskType     string    `bson:"task_type"`
	PeriodStart  time.Time `bson:"period_start"`
	PeriodEnd    time.Time `bson:"period_end"`
	TargetValue  int64     `bson:"target_value"`
	CurrentValue int64     `bson:"current_value"`
	RewardType   string    `bson:"reward_type"`
	RewardAmount int64     `bson:"reward_amount"`
	Status       string    `bson:"status"`
	ClaimedAt    time.Time `bson:"claimed_at"`
	Source       string    `bson:"source"`
	CampaignID   string    `bson:"campaign_id"`
	RewardRpID   string    `bson:"reward_rp_id"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DriverScheduleTasksDocument) MongoCollectionName() string {
	return driverScheduleTasksMongoCollectionName
}

const invoiceTitlesMongoCollectionName = "invoice_titles"

// InvoiceTitlesDocument 是 invoice_titles 集合（常用发票抬头）的 MongoDB 文档模型。
type InvoiceTitlesDocument struct {
	ID             string `bson:"_id"`
	UserID         string `bson:"user_id"`
	TitleType      string `bson:"title_type"`
	Title          string `bson:"title"`
	TaxNo          string `bson:"tax_no"`
	CompanyAddress string `bson:"company_address"`
	CompanyPhone   string `bson:"company_phone"`
	BankName       string `bson:"bank_name"`
	BankAccountEnc string `bson:"bank_account_enc"`
	Email          string `bson:"email"`
	IsDefault      bool   `bson:"is_default"`
	Status         string `bson:"status"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (InvoiceTitlesDocument) MongoCollectionName() string {
	return invoiceTitlesMongoCollectionName
}

const carpoolGroupsMongoCollectionName = "carpool_groups"

// CarpoolGroupsDocument 是 carpool_groups 集合（拼车组）的 MongoDB 文档模型。
type CarpoolGroupsDocument struct {
	ID           string    `bson:"_id"`
	GroupNo      string    `bson:"group_no"`
	OrderIds     []string  `bson:"order_ids"`
	CityCode     string    `bson:"city_code"`
	ProductCode  string    `bson:"product_code"`
	SeatTotal    int64     `bson:"seat_total"`
	SeatUsed     int64     `bson:"seat_used"`
	RouteHash    string    `bson:"route_hash"`
	Origin       GeoPoint  `bson:"origin"`
	Destination  GeoPoint  `bson:"destination"`
	DiscountRate float64   `bson:"discount_rate"`
	Status       string    `bson:"status"`
	ExpireAt     time.Time `bson:"expire_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (CarpoolGroupsDocument) MongoCollectionName() string {
	return carpoolGroupsMongoCollectionName
}

const announcementsMongoCollectionName = "announcements"

// AnnouncementsDocument 是 announcements 集合（平台公告）的 MongoDB 文档模型。
type AnnouncementsDocument struct {
	ID             string    `bson:"_id"`
	AnnounceNo     string    `bson:"announce_no"`
	Title          string    `bson:"title"`
	Content        string    `bson:"content"`
	TargetAudience string    `bson:"target_audience"`
	CityCodes      []string  `bson:"city_codes"`
	PartnerID      string    `bson:"partner_id"`
	Channel        string    `bson:"channel"`
	Priority       int64     `bson:"priority"`
	IsPinned       bool      `bson:"is_pinned"`
	RequireConfirm bool      `bson:"require_confirm"`
	PublishAt      time.Time `bson:"publish_at"`
	OfflineAt      time.Time `bson:"offline_at"`
	Status         string    `bson:"status"`
	TargetCount    int64     `bson:"target_count"`
	ReadCount      int64     `bson:"read_count"`
	PublisherID    string    `bson:"publisher_id"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (AnnouncementsDocument) MongoCollectionName() string {
	return announcementsMongoCollectionName
}

const announcementReadsMongoCollectionName = "announcement_reads"

// AnnouncementReadsDocument 是 announcement_reads 集合（公告已读）的 MongoDB 文档模型。
type AnnouncementReadsDocument struct {
	ID           string    `bson:"_id"`
	AnnounceID   string    `bson:"announce_id"`
	ReceiverType string    `bson:"receiver_type"`
	ReceiverID   string    `bson:"receiver_id"`
	ReadAt       time.Time `bson:"read_at"`
	ConfirmedAt  time.Time `bson:"confirmed_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (AnnouncementReadsDocument) MongoCollectionName() string {
	return announcementReadsMongoCollectionName
}

const fleetsMongoCollectionName = "fleets"

// FleetsDocument 是 fleets 集合（车队）的 MongoDB 文档模型。
type FleetsDocument struct {
	ID              string    `bson:"_id"`
	FleetNo         string    `bson:"fleet_no"`
	Name            string    `bson:"name"`
	PartnerID       string    `bson:"partner_id"`
	CityCode        string    `bson:"city_code"`
	ManagerName     string    `bson:"manager_name"`
	ManagerPhoneEnc string    `bson:"manager_phone_enc"`
	DriverCount     int64     `bson:"driver_count"`
	VehicleCount    int64     `bson:"vehicle_count"`
	CommissionRate  float64   `bson:"commission_rate"`
	Status          string    `bson:"status"`
	ContractStartAt time.Time `bson:"contract_start_at"`
	ContractEndAt   time.Time `bson:"contract_end_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (FleetsDocument) MongoCollectionName() string {
	return fleetsMongoCollectionName
}

const dispatchRulesMongoCollectionName = "dispatch_rules"

// DispatchRulesDocument 是 dispatch_rules 集合（派单规则配置）的 MongoDB 文档模型。
type DispatchRulesDocument struct {
	ID                string       `bson:"_id"`
	RuleCode          string       `bson:"rule_code"`
	CityCode          string       `bson:"city_code"`
	ProductCode       string       `bson:"product_code"`
	SearchRadiusStart int64        `bson:"search_radius_start"`
	SearchRadiusStep  int64        `bson:"search_radius_step"`
	SearchRadiusMax   int64        `bson:"search_radius_max"`
	MaxRounds         int64        `bson:"max_rounds"`
	RoundTimeoutSec   int64        `bson:"round_timeout_sec"`
	ScoreWeights      ScoreWeights `bson:"score_weights"`
	RequireQualified  bool         `bson:"require_qualified"`
	MaxPickupDistance int64        `bson:"max_pickup_distance"`
	Priority          int64        `bson:"priority"`
	Enabled           bool         `bson:"enabled"`
	EffectiveStart    time.Time    `bson:"effective_start"`
	EffectiveEnd      time.Time    `bson:"effective_end"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DispatchRulesDocument) MongoCollectionName() string {
	return dispatchRulesMongoCollectionName
}

// ScoreWeights 表示 dispatch_rules.scoreWeights 打分权重。
type ScoreWeights struct {
	Distance      float64 `bson:"distance"`
	ServiceScore  float64 `bson:"service_score"`
	AcceptRate    float64 `bson:"accept_rate"`
	OnlineMinutes float64 `bson:"online_minutes"`
	Detour        float64 `bson:"detour"`
	BigOrder      float64 `bson:"big_order"`
}
