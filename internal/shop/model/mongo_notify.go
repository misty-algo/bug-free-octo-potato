package model

import "time"

const messageTemplatesMongoCollectionName = "message_templates"

// MessageTemplatesDocument 是 message_templates 集合（消息模板）的 MongoDB 文档模型。
type MessageTemplatesDocument struct {
	ID                   string     `bson:"_id"`
	Code                 string     `bson:"code"`
	Name                 string     `bson:"name"`
	Channel              string     `bson:"channel"`
	Scene                string     `bson:"scene"`
	Title                string     `bson:"title"`
	Content              string     `bson:"content"`
	Variables            []string   `bson:"variables"`
	ThirdPartyTemplateID string     `bson:"third_party_template_id"`
	JumpURL              string     `bson:"jump_url"`
	Enabled              bool       `bson:"enabled"`
	FrequencyLimit       int64      `bson:"frequency_limit"`
	QuietHours           QuietHours `bson:"quiet_hours"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (MessageTemplatesDocument) MongoCollectionName() string {
	return messageTemplatesMongoCollectionName
}

// QuietHours 表示 message_templates.quietHours 免打扰时段。
type QuietHours struct {
	Start string `bson:"start"`
	End   string `bson:"end"`
}

const notificationsMongoCollectionName = "notifications"

// NotificationsDocument 是 notifications 集合（消息发送记录）的 MongoDB 文档模型。
type NotificationsDocument struct {
	ID              string    `bson:"_id"`
	TemplateCode    string    `bson:"template_code"`
	Channel         string    `bson:"channel"`
	ReceiverType    string    `bson:"receiver_type"`
	ReceiverID      string    `bson:"receiver_id"`
	ReceiverAddress string    `bson:"receiver_address"`
	BizID           string    `bson:"biz_id"`
	Content         string    `bson:"content"`
	Status          string    `bson:"status"`
	SentAt          time.Time `bson:"sent_at"`
	DeliveredAt     time.Time `bson:"delivered_at"`
	ReadAt          time.Time `bson:"read_at"`
	FailReason      string    `bson:"fail_reason"`
	ThirdPartyMsgID string    `bson:"third_party_msg_id"`
	RetryCount      int64     `bson:"retry_count"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (NotificationsDocument) MongoCollectionName() string {
	return notificationsMongoCollectionName
}
