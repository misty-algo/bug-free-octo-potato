package model

import "time"

const riskEventsMongoCollectionName = "risk_events"

// RiskEventsDocument 是 risk_events 集合（风控事件）的 MongoDB 文档模型。
type RiskEventsDocument struct {
	ID           string                 `bson:"_id"`
	EventNo      string                 `bson:"event_no"`
	BizScene     string                 `bson:"biz_scene"`
	BizID        string                 `bson:"biz_id"`
	UserID       string                 `bson:"user_id"`
	DriverID     string                 `bson:"driver_id"`
	DeviceID     string                 `bson:"device_id"`
	RiskType     string                 `bson:"risk_type"`
	HitRules     []RiskHitRule          `bson:"hit_rules"`
	RiskScore    int64                  `bson:"risk_score"`
	Decision     string                 `bson:"decision"`
	ActionTaken  string                 `bson:"action_taken"`
	Evidence     map[string]interface{} `bson:"evidence"`
	ReviewerID   string                 `bson:"reviewer_id"`
	ReviewResult string                 `bson:"review_result"`
	ReviewedAt   time.Time              `bson:"reviewed_at"`
	Status       string                 `bson:"status"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (RiskEventsDocument) MongoCollectionName() string {
	return riskEventsMongoCollectionName
}

// RiskHitRule 表示 risk_events.hitRules 命中规则。
type RiskHitRule struct {
	RuleID   string  `bson:"rule_id"`
	RuleName string  `bson:"rule_name"`
	Weight   float64 `bson:"weight"`
}

const riskRulesMongoCollectionName = "risk_rules"

// RiskRulesDocument 是 risk_rules 集合（风控规则）的 MongoDB 文档模型。
type RiskRulesDocument struct {
	ID        string                 `bson:"_id"`
	RuleCode  string                 `bson:"rule_code"`
	RuleName  string                 `bson:"rule_name"`
	BizScene  string                 `bson:"biz_scene"`
	RuleType  string                 `bson:"rule_type"`
	Condition map[string]interface{} `bson:"condition"`
	Weight    float64                `bson:"weight"`
	Threshold float64                `bson:"threshold"`
	Action    string                 `bson:"action"`
	Enabled   bool                   `bson:"enabled"`
	Priority  int64                  `bson:"priority"`
	CityCodes []string               `bson:"city_codes"`
	Remark    string                 `bson:"remark"`

	// 16.6 增补字段
	GrayRatio     float64  `bson:"gray_ratio"`
	GrayCityCodes []string `bson:"gray_city_codes"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (RiskRulesDocument) MongoCollectionName() string {
	return riskRulesMongoCollectionName
}

const blacklistsMongoCollectionName = "blacklists"

// BlacklistsDocument 是 blacklists 集合（黑名单）的 MongoDB 文档模型。
type BlacklistsDocument struct {
	ID                 string    `bson:"_id"`
	TargetType         string    `bson:"target_type"`
	TargetValue        string    `bson:"target_value"`
	ReasonType         string    `bson:"reason_type"`
	Reason             string    `bson:"reason"`
	Source             string    `bson:"source"`
	RelatedRiskEventID string    `bson:"related_risk_event_id"`
	Scope              string    `bson:"scope"`
	CityCodes          []string  `bson:"city_codes"`
	StartAt            time.Time `bson:"start_at"`
	EndAt              time.Time `bson:"end_at"`
	Status             string    `bson:"status"`
	OperatorID         string    `bson:"operator_id"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (BlacklistsDocument) MongoCollectionName() string {
	return blacklistsMongoCollectionName
}

const safetyIncidentsMongoCollectionName = "safety_incidents"

// SafetyIncidentsDocument 是 safety_incidents 集合（安全事件）的 MongoDB 文档模型。
type SafetyIncidentsDocument struct {
	ID               string                 `bson:"_id"`
	IncidentNo       string                 `bson:"incident_no"`
	OrderID          string                 `bson:"order_id"`
	UserID           string                 `bson:"user_id"`
	DriverID         string                 `bson:"driver_id"`
	Type             string                 `bson:"type"`
	Level            string                 `bson:"level"`
	TriggerSource    string                 `bson:"trigger_source"`
	Location         GeoPoint               `bson:"location"`
	Description      string                 `bson:"description"`
	Evidence         SafetyIncidentEvidence `bson:"evidence"`
	PoliceNotified   bool                   `bson:"police_notified"`
	PoliceNotifiedAt time.Time              `bson:"police_notified_at"`
	ContactsNotified []string               `bson:"contacts_notified"`
	Status           string                 `bson:"status"`
	HandlerID        string                 `bson:"handler_id"`
	HandlingLog      []SafetyHandlingLog    `bson:"handling_log"`
	ResolvedAt       time.Time              `bson:"resolved_at"`
	Conclusion       string                 `bson:"conclusion"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (SafetyIncidentsDocument) MongoCollectionName() string {
	return safetyIncidentsMongoCollectionName
}

// SafetyIncidentEvidence 表示 safety_incidents.evidence 证据。
type SafetyIncidentEvidence struct {
	RecordURL          string   `bson:"record_url"`
	TrajectorySnapshot string   `bson:"trajectory_snapshot"`
	Images             []string `bson:"images"`
}

// SafetyHandlingLog 表示 safety_incidents.handlingLog 处置记录。
type SafetyHandlingLog struct {
	OperatorID string    `bson:"operator_id"`
	Action     string    `bson:"action"`
	Content    string    `bson:"content"`
	OperatedAt time.Time `bson:"operated_at"`
}
