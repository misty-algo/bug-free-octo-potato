package model

import "time"

const reviewsMongoCollectionName = "reviews"

// ReviewsDocument 是 reviews 集合（评价）的 MongoDB 文档模型。
type ReviewsDocument struct {
	ID           string   `bson:"_id"`
	OrderID      string   `bson:"order_id"`
	ReviewerType string   `bson:"reviewer_type"`
	ReviewerID   string   `bson:"reviewer_id"`
	TargetID     string   `bson:"target_id"`
	Rating       int64    `bson:"rating"`
	Tags         []string `bson:"tags"`
	Content      string   `bson:"content"`
	IsAnonymous  bool     `bson:"is_anonymous"`
	IsVisible    bool     `bson:"is_visible"`
	AppealStatus string   `bson:"appeal_status"`
	AppealReason string   `bson:"appeal_reason"`
	ReplyContent string   `bson:"reply_content"`
	ScoreDelta   int64    `bson:"score_delta"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (ReviewsDocument) MongoCollectionName() string {
	return reviewsMongoCollectionName
}

const ticketsMongoCollectionName = "tickets"

// TicketsDocument 是 tickets 集合（客服工单）的 MongoDB 文档模型。
type TicketsDocument struct {
	ID              string             `bson:"_id"`
	TicketNo        string             `bson:"ticket_no"`
	OrderID         string             `bson:"order_id"`
	UserID          string             `bson:"user_id"`
	DriverID        string             `bson:"driver_id"`
	ApplicantType   string             `bson:"applicant_type"`
	Category        string             `bson:"category"`
	SubCategory     string             `bson:"sub_category"`
	Title           string             `bson:"title"`
	Content         string             `bson:"content"`
	Attachments     []string           `bson:"attachments"`
	Priority        string             `bson:"priority"`
	Status          string             `bson:"status"`
	AssigneeID      string             `bson:"assignee_id"`
	SkillGroup      string             `bson:"skill_group"`
	SlaDeadline     time.Time          `bson:"sla_deadline"`
	FirstResponseAt time.Time          `bson:"first_response_at"`
	ResolvedAt      time.Time          `bson:"resolved_at"`
	Resolution      string             `bson:"resolution"`
	Compensation    TicketCompensation `bson:"compensation"`
	Satisfaction    int64              `bson:"satisfaction"`
	IsEscalated     bool               `bson:"is_escalated"`
	EscalateTo      string             `bson:"escalate_to"`
	Tags            []string           `bson:"tags"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (TicketsDocument) MongoCollectionName() string {
	return ticketsMongoCollectionName
}

// TicketCompensation 表示 tickets.compensation 赔付信息。
type TicketCompensation struct {
	Amount int64  `bson:"amount"`
	Type   string `bson:"type"`
	Status string `bson:"status"`
}

const ticketLogsMongoCollectionName = "ticket_logs"

// TicketLogsDocument 是 ticket_logs 集合（工单处理日志）的 MongoDB 文档模型。
type TicketLogsDocument struct {
	ID           string   `bson:"_id"`
	TicketID     string   `bson:"ticket_id"`
	Action       string   `bson:"action"`
	OperatorID   string   `bson:"operator_id"`
	OperatorType string   `bson:"operator_type"`
	Content      string   `bson:"content"`
	FromStatus   string   `bson:"from_status"`
	ToStatus     string   `bson:"to_status"`
	Attachments  []string `bson:"attachments"`
	IsInternal   bool     `bson:"is_internal"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (TicketLogsDocument) MongoCollectionName() string {
	return ticketLogsMongoCollectionName
}

const lostItemsMongoCollectionName = "lost_items"

// LostItemsDocument 是 lost_items 集合（遗失物品）的 MongoDB 文档模型。
type LostItemsDocument struct {
	ID              string    `bson:"_id"`
	OrderID         string    `bson:"order_id"`
	UserID          string    `bson:"user_id"`
	DriverID        string    `bson:"driver_id"`
	ItemName        string    `bson:"item_name"`
	ItemCategory    string    `bson:"item_category"`
	Description     string    `bson:"description"`
	Images          []string  `bson:"images"`
	ContactPhoneEnc string    `bson:"contact_phone_enc"`
	Status          string    `bson:"status"`
	FoundAt         time.Time `bson:"found_at"`
	ReturnedAt      time.Time `bson:"returned_at"`
	HandlerID       string    `bson:"handler_id"`
	Remark          string    `bson:"remark"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (LostItemsDocument) MongoCollectionName() string {
	return lostItemsMongoCollectionName
}
