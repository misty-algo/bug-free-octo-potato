package model

import "time"

const productsMongoCollectionName = "products"

// ProductsDocument 是 products 集合（产品线）的 MongoDB 文档模型。
type ProductsDocument struct {
	ID           string   `bson:"_id"`
	Code         string   `bson:"code"`
	Name         string   `bson:"name"`
	IconURL      string   `bson:"icon_url"`
	SeatCapacity int64    `bson:"seat_capacity"`
	PriceRatio   float64  `bson:"price_ratio"`
	IsFixedPrice bool     `bson:"is_fixed_price"`
	CityCodes    []string `bson:"city_codes"`
	Status       string   `bson:"status"`
	Sort         int64    `bson:"sort"`
	Description  string   `bson:"description"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (ProductsDocument) MongoCollectionName() string {
	return productsMongoCollectionName
}

const ordersMongoCollectionName = "orders"

// OrdersDocument 是 orders 集合（订单，核心集合）的 MongoDB 文档模型。
type OrdersDocument struct {
	ID          string `bson:"_id"`
	OrderNo     string `bson:"order_no"`
	BizType     string `bson:"biz_type"`
	ProductCode string `bson:"product_code"`
	CityCode    string `bson:"city_code"`

	// 乘客侧
	UserID            string `bson:"user_id"`
	PassengerPhoneEnc string `bson:"passenger_phone_enc"`
	PassengerName     string `bson:"passenger_name"`
	IsProxy           bool   `bson:"is_proxy"`

	// 起终点
	StartLocation    GeoPoint `bson:"start_location"`
	StartAddress     string   `bson:"start_address"`
	StartPoiID       string   `bson:"start_poi_id"`
	StartHouseNumber string   `bson:"start_house_number"`
	EndLocation      GeoPoint `bson:"end_location"`
	EndAddress       string   `bson:"end_address"`
	EndPoiID         string   `bson:"end_poi_id"`
	EndHouseNumber   string   `bson:"end_house_number"`
	EstimateDistance int64    `bson:"estimate_distance"`
	EstimateDuration int64    `bson:"estimate_duration"`
	EstimateFee      int64    `bson:"estimate_fee"`
	FixedPrice       int64    `bson:"fixed_price"`

	// 司机侧
	DriverID              string    `bson:"driver_id"`
	VehicleID             string    `bson:"vehicle_id"`
	PlateNo               string    `bson:"plate_no"`
	DriverPhoneEnc        string    `bson:"driver_phone_enc"`
	VirtualNumber         string    `bson:"virtual_number"`
	VirtualNumberExpireAt time.Time `bson:"virtual_number_expire_at"`

	// 状态机
	Status           string `bson:"status"`
	Version          int64  `bson:"version"`
	CancelSide       string `bson:"cancel_side"`
	CancelReasonCode string `bson:"cancel_reason_code"`
	CancelFee        int64  `bson:"cancel_fee"`

	// 时间节点
	ReserveTime    time.Time `bson:"reserve_time"`
	CreateTime     time.Time `bson:"create_time"`
	AcceptTime     time.Time `bson:"accept_time"`
	ArriveTime     time.Time `bson:"arrive_time"`
	BoardTime      time.Time `bson:"board_time"`
	EndServiceTime time.Time `bson:"end_service_time"`
	PayTime        time.Time `bson:"pay_time"`
	CancelTime     time.Time `bson:"cancel_time"`
	FinishTime     time.Time `bson:"finish_time"`

	// 实际里程时长
	PickupDistance   int64 `bson:"pickup_distance"`
	PickupDuration   int64 `bson:"pickup_duration"`
	ActualDistance   int64 `bson:"actual_distance"`
	ActualDuration   int64 `bson:"actual_duration"`
	WaitDuration     int64 `bson:"wait_duration"`
	PaidWaitDuration int64 `bson:"paid_wait_duration"`

	// 计价
	PricingRuleID     string  `bson:"pricing_rule_id"`
	PricingSnapshotID string  `bson:"pricing_snapshot_id"`
	SurgeRatio        float64 `bson:"surge_ratio"`
	SurgeFee          int64   `bson:"surge_fee"`
	CouponID          string  `bson:"coupon_id"`
	DiscountFee       int64   `bson:"discount_fee"`
	ExtraFee          int64   `bson:"extra_fee"`
	TotalFee          int64   `bson:"total_fee"`
	PayFee            int64   `bson:"pay_fee"`
	DriverIncome      int64   `bson:"driver_income"`
	DriverNetIncome   int64   `bson:"driver_net_income"`
	CommissionFee     int64   `bson:"commission_fee"`
	SubsidyFee        int64   `bson:"subsidy_fee"`

	// 支付
	PayStatus   string `bson:"pay_status"`
	PayChannel  string `bson:"pay_channel"`
	PaymentID   string `bson:"payment_id"`
	IsNoPassPay bool   `bson:"is_no_pass_pay"`

	// 服务与安全
	PassengerRemark string   `bson:"passenger_remark"`
	RecordURL       string   `bson:"record_url"`
	IsShared        bool     `bson:"is_shared"`
	SafetyFlags     []string `bson:"safety_flags"`
	RiskLevel       string   `bson:"risk_level"`

	// 拼车/预约扩展
	FlightNo         string `bson:"flight_no"`
	CarpoolGroupID   string `bson:"carpool_group_id"`
	CarpoolSeatCount int64  `bson:"carpool_seat_count"`

	// 其他
	Source        string `bson:"source"`
	DeviceID      string `bson:"device_id"`
	InvoiceStatus string `bson:"invoice_status"`
	IsReviewed    bool   `bson:"is_reviewed"`
	PartnerID     string `bson:"partner_id"`

	// 16.6 增补字段
	QueueID     string   `bson:"queue_id"`
	RewardRpIDs []string `bson:"reward_rp_ids"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
}

func (OrdersDocument) MongoCollectionName() string {
	return ordersMongoCollectionName
}

const orderStatusLogsMongoCollectionName = "order_status_logs"

// OrderStatusLogsDocument 是 order_status_logs 集合（订单状态流转日志）的 MongoDB 文档模型。
type OrderStatusLogsDocument struct {
	ID           string                 `bson:"_id"`
	OrderID      string                 `bson:"order_id"`
	FromStatus   string                 `bson:"from_status"`
	ToStatus     string                 `bson:"to_status"`
	OperatorType string                 `bson:"operator_type"`
	OperatorID   string                 `bson:"operator_id"`
	Reason       string                 `bson:"reason"`
	Extra        map[string]interface{} `bson:"extra"`
	OccurredAt   time.Time              `bson:"occurred_at"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (OrderStatusLogsDocument) MongoCollectionName() string {
	return orderStatusLogsMongoCollectionName
}

const orderCancelsMongoCollectionName = "order_cancels"

// OrderCancelsDocument 是 order_cancels 集合（取消记录）的 MongoDB 文档模型。
type OrderCancelsDocument struct {
	ID                  string    `bson:"_id"`
	OrderID             string    `bson:"order_id"`
	CancelSide          string    `bson:"cancel_side"`
	ReasonCode          string    `bson:"reason_code"`
	ReasonText          string    `bson:"reason_text"`
	OrderStatusAtCancel string    `bson:"order_status_at_cancel"`
	FreeCancel          bool      `bson:"free_cancel"`
	CancelFee           int64     `bson:"cancel_fee"`
	ActualChargeFee     int64     `bson:"actual_charge_fee"`
	Charged             bool      `bson:"charged"`
	AffectCredit        bool      `bson:"affect_credit"`
	ScoreDelta          int64     `bson:"score_delta"`
	OperatorID          string    `bson:"operator_id"`
	CancelTime          time.Time `bson:"cancel_time"`

	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (OrderCancelsDocument) MongoCollectionName() string {
	return orderCancelsMongoCollectionName
}

const dispatchesMongoCollectionName = "dispatches"

// DispatchesDocument 是 dispatches 集合（派单记录）的 MongoDB 文档模型。
type DispatchesDocument struct {
	ID               string              `bson:"_id"`
	OrderID          string              `bson:"order_id"`
	Round            int64               `bson:"round"`
	Strategy         string              `bson:"strategy"`
	SearchRadius     int64               `bson:"search_radius"`
	CandidateCount   int64               `bson:"candidate_count"`
	Candidates       []DispatchCandidate `bson:"candidates"`
	Broadcasts       []DispatchBroadcast `bson:"broadcasts"`
	AcceptedDriverID string              `bson:"accepted_driver_id"`
	Result           string              `bson:"result"`
	ElapsedMs        int64               `bson:"elapsed_ms"`
	StartedAt        time.Time           `bson:"started_at"`
	EndedAt          time.Time           `bson:"ended_at"`

	// 16.6 增补字段
	OperatorID   string `bson:"operator_id"`
	OperatorType string `bson:"operator_type"`
	QueueID      string `bson:"queue_id"`

	// 全局审计字段
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
	CreatedBy string    `bson:"created_by"`
	UpdatedBy string    `bson:"updated_by"`
	IsDeleted bool      `bson:"is_deleted"`
	Version   int64     `bson:"version"`
}

func (DispatchesDocument) MongoCollectionName() string {
	return dispatchesMongoCollectionName
}

// DispatchCandidate 表示 dispatches.candidates 候选司机明细。
type DispatchCandidate struct {
	DriverID string  `bson:"driver_id"`
	Distance int64   `bson:"distance"`
	Duration int64   `bson:"duration"`
	Score    float64 `bson:"score"`
	Rank     int64   `bson:"rank"`
	Reason   string  `bson:"reason"`
}

// DispatchBroadcast 表示 dispatches.broadcasts 推送明细。
type DispatchBroadcast struct {
	DriverID    string    `bson:"driver_id"`
	PushedAt    time.Time `bson:"pushed_at"`
	RespondedAt time.Time `bson:"responded_at"`
	Result      string    `bson:"result"`
	Score       float64   `bson:"score"`
}
