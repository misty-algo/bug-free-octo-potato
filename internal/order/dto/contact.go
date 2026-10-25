package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

// SendMessageCommand 是司机发消息。身份来自令牌，拼车按 order_id 选择目标。
type SendMessageCommand struct {
	Principal       authn.Principal `json:"-"`
	OrderID         string          `json:"order_id"`
	Text            string          `json:"text"`
	ClientMessageID string          `json:"client_message_id"`
	IdempotencyKey  string          `json:"-"`
}

type SendMessageResult struct {
	IntentID string `json:"intent_id"`
	Seq      int64  `json:"seq"`
	State    string `json:"state"`
}

type ListMessagesCommand struct {
	Principal authn.Principal `json:"-"`
	OrderID   string          `json:"order_id"`
	Cursor    int64           `json:"cursor"`
}

type ChatMessage struct {
	Seq             int64     `json:"seq"`
	From            string    `json:"from"`
	Text            string    `json:"text"`
	T               time.Time `json:"t"`
	State           string    `json:"state"`
	ClientMessageID string    `json:"client_message_id,omitempty"`
}

type ListMessagesResult struct {
	Messages   []ChatMessage `json:"messages"`
	NextCursor int64         `json:"next_cursor"`
}

type VirtualCallCommand struct {
	Principal      authn.Principal `json:"-"`
	OrderID        string          `json:"order_id"`
	IdempotencyKey string          `json:"-"`
}

// VirtualCallResult 不返回真实手机号；模拟通道必须显式标记。
type VirtualCallResult struct {
	VirtualNumber string    `json:"virtual_number"`
	ExpiresAt     time.Time `json:"expires_at"`
	Simulated     bool      `json:"simulated"`
	Notice        string    `json:"notice,omitempty"`
}

// MessageReceipt 是通讯域回写的结果回执。
type MessageReceipt struct {
	OrderID string `json:"order_id"`
	Seq     int64  `json:"seq"`
	State   string `json:"state"`
}
