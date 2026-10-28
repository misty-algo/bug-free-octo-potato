package model

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// MongoCollectionName 供 mongox.NewDocumentStore[TicketDocument] 定位 tickets 集合。
func (TicketDocument) MongoCollectionName() string { return "tickets" }

// TicketDocument 对齐线上 huazhu.tickets 的 29 个字段（2026-10-27 实测）。
type TicketDocument struct {
	ID          bson.ObjectID `bson:"_id"`
	TicketNo    string        `bson:"ticketNo"`
	OrderID     bson.ObjectID `bson:"orderId"`
	OrderNo     string        `bson:"orderNo"`
	ComplainID  bson.ObjectID `bson:"complainantId"`
	DriverID    bson.ObjectID `bson:"driverId"`
	Type        string        `bson:"type"`
	Group       string        `bson:"group,omitempty"`
	Content     string        `bson:"content"`
	Evidences   []string      `bson:"evidences"`
	State       string        `bson:"state"`
	Urgent      bool          `bson:"urgent"`
	Assignee    string        `bson:"assignee,omitempty"`
	Round       int32         `bson:"round"`
	Rounds      []TicketRound `bson:"rounds"`
	FirstRespAt *time.Time    `bson:"firstRespondedAt,omitempty"`
	FirstDueAt  *time.Time    `bson:"firstResponseDueAt,omitempty"`
	UrgentDueAt *time.Time    `bson:"urgentResponseDueAt,omitempty"`
	PausedMs    int64         `bson:"pausedMs"`
	PausedAt    *time.Time    `bson:"pausedAt,omitempty"`
	Escalated   bool          `bson:"escalated"`
	EscalatedAt *time.Time    `bson:"escalatedAt,omitempty"`
	EscalateRsn string        `bson:"escalateReason,omitempty"`
	DutyRemind  *time.Time    `bson:"dutyRemindedAt,omitempty"`
	MergedInto  string        `bson:"mergedInto,omitempty"`
	MergedNos   []string      `bson:"mergedTicketNos"`
	Timeline    []TicketEvent `bson:"timeline"`
	CreatedAt   time.Time     `bson:"createdAt"`

	// IdempotencyKey 幂等键（本模块新增字段；同键重试 upsert 同一文档）。
	IdempotencyKey string `bson:"idempotencyKey,omitempty"`
}

// TicketRound 工单处理轮次元素（线上暂无样本，字段名待客服域落地后校准）。
type TicketRound struct {
	Seq        int32      `bson:"seq"`
	Actor      string     `bson:"actor,omitempty"`
	Action     string     `bson:"action,omitempty"`
	Content    string     `bson:"content,omitempty"`
	Reply      string     `bson:"reply,omitempty"`
	Attachment []string   `bson:"attachment,omitempty"`
	T          *time.Time `bson:"t,omitempty"`
}

// TicketEvent 工单时间线元素（线上实测字段：seq/actor/action/from/to/message/at）。
type TicketEvent struct {
	Seq     int32      `bson:"seq"`
	Actor   string     `bson:"actor,omitempty"`
	Action  string     `bson:"action,omitempty"`
	From    string     `bson:"from,omitempty"`
	To      string     `bson:"to,omitempty"`
	Message string     `bson:"message,omitempty"`
	At      *time.Time `bson:"at,omitempty"`
}
