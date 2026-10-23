package entity

import "time"

// 消息状态：意图受理不冒充消息已保存；无已读回执不显示已读。
const (
	MessagePending   = "PENDING"   // 待发送
	MessageAccepted  = "ACCEPTED"  // 权威已受理（意图已持久化）
	MessageStored    = "STORED"    // 通讯域已保存
	MessageDelivered = "DELIVERED" // 对端已拉取
	MessageFailed    = "FAILED"
)

// AuthorizeContactCommand 的两种动作。
const (
	IntentSendMessage = "SEND_MESSAGE"
	IntentVirtualCall = "REQUEST_VIRTUAL_CALL"
)

// 有效联系窗口：仅已接单、已到达、行程中。
func ContactWindowOpen(state string) bool {
	switch state {
	case OrderAccepted, OrderArrived, OrderInTrip:
		return true
	}
	return false
}

// OrderMessage 内嵌在 orders.messages。不另建聊天真相。
type OrderMessage struct {
	Seq             int64     `bson:"seq"`
	From            string    `bson:"from"` // pax / drv
	Text            string    `bson:"text"`
	T               time.Time `bson:"t"`
	ClientMessageID string    `bson:"clientMessageId,omitempty"`
	State           string    `bson:"state"`
}

// VirtualCallBinding 内嵌在 orders.virtualCall。不保存真实手机号。
type VirtualCallBinding struct {
	Number    string     `bson:"number"`
	ExpiresAt time.Time  `bson:"expiresAt"`
	Revision  int64      `bson:"revision"`
	RequestNo string     `bson:"requestNo"`
	Simulated bool       `bson:"simulated"`
	RevokedAt *time.Time `bson:"revokedAt,omitempty"`
}

// ContactIntent 是 AuthorizeContactCommand 持久化的唯一意图事实。
// 授权判定与意图持久化在同一边界内完成，不产生可反复使用的授权票。
type ContactIntent struct {
	IntentID        string    `bson:"intentId"`
	OrderNo         string    `bson:"orderNo"`
	TaskNo          string    `bson:"taskNo"`
	DriverID        string    `bson:"driverId"`
	PassengerID     string    `bson:"passengerId"`
	Revision        int64     `bson:"revision"`
	Action          string    `bson:"action"`
	Digest          string    `bson:"digest"`
	RequestNo       string    `bson:"requestNo"`
	ClientMessageID string    `bson:"clientMessageId,omitempty"`
	Seq             int64     `bson:"seq,omitempty"`
	State           string    `bson:"state"`
	CreatedAt       time.Time `bson:"createdAt"`
}

// ContactMutation 是联系命令写入的全部事实。
type ContactMutation struct {
	Order       RideOrder
	Intent      ContactIntent
	Message     *OrderMessage
	VirtualCall *VirtualCallBinding
	Events      []EventLog
	Command     CommandResult
}
