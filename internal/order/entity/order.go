package entity

import (
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrInvalidOrder  = errors.New("invalid order")
)

// Order 是 order 业务服务的聚合根（P17-P22 读侧 + 评价/工单写侧）。
// 字段对齐线上 huazhu.orders（bson 映射见 repo 层）；Name/Description 为脚手架遗留字段，Gorm 路径仍用。
type Order struct {
	ID          string
	Name        string
	Description string

	OrderNo     string
	PassengerID string
	Type        int32
	TypeName    string
	StartID     string
	EndID       string
	StartLoc    *GeoPoint
	DistM       int64
	DurS        int64
	Seats       int32
	State       string
	Version     int64
	CpFen       int64
	CdFen       int64
	Fixed       bool
	PriceSnap   *PriceSnapshot // 下单锁价估价快照（orders.priceSnap）
	DriverID    string
	DriverName  string
	Rated       bool
	Changes     int32
	CityCode    string
	SurgeRatio  float64
	DispatchID  string
	BizType     int32
	PricingRule string
	PricingSnap string
	PricingVer  string
	RiskLevel   string
	Matching    map[string]any
	AcceptedAt  *time.Time
	ArrivedAt   *time.Time
	EndedAt     *time.Time // ENDED/COMPLETED 时间（P19 评价窗口基准，线上 endedAt 列）
	Timeline    []TimelineItem
	Refunds     []Refund
	Bills       []Bill
	Rating      *Rating

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Rating 订单内嵌评价（P19）。
type Rating struct {
	Score     int32
	Content   string
	Anonymous bool
	RatedAt   time.Time
}

// Bill 订单内嵌账单元素（字段名待账单流程落地后校准）。
type Bill struct {
	Kind                  string
	TotalFee              int64
	DriverGrossFee        int64
	PlatformServiceFee    int64
	SurgeFee              int64
	CouponFee             int64
	PlatformCouponBearFee int64
	DriverCouponBearFee   int64
	PayableFee            int64
	DriverIncomeFee       int64
	PlatformNetFee        int64
	State                 string
	PaidAt                *time.Time
	Channel               string
}

// Refund 订单内嵌退款元素（字段名待退款流程落地后校准）。
type Refund struct {
	RefundFee       int64
	DriverRefundFee int64
	Mode            string
	State           string
	T               time.Time
}

// Order 缺失的线上状态枚举补充（ACCEPTED/ARRIVED/IN_TRIP/ENDED/CANCELLED 见 ride.go）。
const (
	OrderPending   = "PENDING"
	OrderInService = "IN_SERVICE"
	OrderCompleted = "COMPLETED"
	OrderUnpaid    = "UNPAID"
	OrderNoDriver  = "NO_DRIVER"
)

// NewOrder 校验输入，并创建带框架管理身份字段的聚合。
func NewOrder(name string, description string) (*Order, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, ErrInvalidOrder
	}
	now := time.Now().UTC()
	return &Order{
		ID:          bson.NewObjectID().Hex(),
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Update 修改可变字段，并把校验保留在业务实体内部。
func (item *Order) Update(name string, description string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if item == nil || item.ID == "" || name == "" {
		return ErrInvalidOrder
	}
	item.Name = name
	item.Description = description
	item.UpdatedAt = time.Now().UTC()
	return nil
}
