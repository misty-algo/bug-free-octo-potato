package entity

import (
	"errors"
	"time"
)

// 取消报价规则（P12）。
const (
	// CancelQuoteValidWindow 取消报价 30 秒有效，过期需重新报价。
	CancelQuoteValidWindow = 30 * time.Second
	// FreeCancelAfterAccept 司机接单后 120 秒内取消免费。
	FreeCancelAfterAccept = 120 * time.Second
	// StandardCancelFee 免费期后标准取消费（500 分）。
	StandardCancelFee = int64(500)
)

// CancelQuote 是取消报价聚合（P12：报价 30 秒有效并绑定订单版本；状态变化则重新报价）。
type CancelQuote struct {
	ID           string
	OrderID      string
	QuoteNo      string
	CancelFee    int64 // 取消报价金额（分）
	BoundVersion int64 // 绑定订单版本；状态变化则重新报价
	ReasonCode   string
	ExpiresAt    time.Time
	Used         bool

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatedBy string
	UpdatedBy string
	IsDeleted bool
}

// NewCancelQuote 生成取消报价：司机接单后 freeWindow 内免费，否则按标准取消费。
func NewCancelQuote(orderID, quoteNo string, orderVersion int64, acceptedAt time.Time, now time.Time, reasonCode string) (*CancelQuote, error) {
	if orderID == "" || quoteNo == "" {
		return nil, ErrInvalidCancelQuote
	}
	fee := int64(0)
	if now.Sub(acceptedAt) > FreeCancelAfterAccept {
		fee = StandardCancelFee
	}
	return &CancelQuote{
		OrderID:      orderID,
		QuoteNo:      quoteNo,
		CancelFee:    fee,
		BoundVersion: orderVersion,
		ReasonCode:   reasonCode,
		ExpiresAt:    now.Add(CancelQuoteValidWindow),
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// IsExpired 判断报价是否已过 30 秒有效期。
func (q *CancelQuote) IsExpired(now time.Time) bool {
	return now.After(q.ExpiresAt)
}

// Consume 确认消费报价：仅未使用且未过期且版本一致时允许。
func (q *CancelQuote) Consume(orderVersion int64, now time.Time) error {
	if q.Used {
		return ErrCancelQuoteUsed
	}
	if q.IsExpired(now) {
		return ErrCancelQuoteExpired
	}
	if q.BoundVersion != orderVersion {
		return ErrCancelQuoteVersionMismatch
	}
	q.Used = true
	q.UpdatedAt = now
	return nil
}

// 领域错误。
var (
	ErrInvalidCancelQuote       = errors.New("invalid cancel quote")
	ErrCancelQuoteUsed          = errors.New("cancel quote already used")
	ErrCancelQuoteExpired       = errors.New("cancel quote expired")
	ErrCancelQuoteVersionMismatch = errors.New("cancel quote bound version mismatch")
)
