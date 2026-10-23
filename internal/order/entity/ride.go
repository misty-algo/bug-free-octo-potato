package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Order states: WAITING → ACCEPTED → ARRIVED → IN_TRIP → ENDED, plus CANCELLED.
const (
	OrderAccepted  = "ACCEPTED"
	OrderArrived   = "ARRIVED"
	OrderInTrip    = "IN_TRIP"
	OrderEnded     = "ENDED"
	OrderCancelled = "CANCELLED"
)

// Dispatch task states. 到达确认后任务进入 ACTIVE。
const (
	TaskAssigned = "ASSIGNED"
	TaskActive   = "ACTIVE"
	TaskFinished = "FINISHED"
)

// Coordinate is the domain value used by distance calculations. It is never
// persisted; documents store GeoPoint in GeoJSON [lng, lat] order.
type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	CRS       string  `json:"crs"`
}

// GeoPoint is the persisted GeoJSON point (GCJ-02, [longitude, latitude]).
type GeoPoint struct {
	Type        string    `bson:"type,omitempty"`
	Coordinates []float64 `bson:"coordinates,omitempty"`
}

// Coordinate converts the point into the domain value, reporting false when the
// stored coordinate is unusable.
func (g *GeoPoint) Coordinate() (Coordinate, bool) {
	if g == nil || len(g.Coordinates) < 2 {
		return Coordinate{}, false
	}
	lng, lat := g.Coordinates[0], g.Coordinates[1]
	if lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return Coordinate{}, false
	}
	return Coordinate{Latitude: lat, Longitude: lng, CRS: "GCJ02"}, true
}

// TimelineItem is one embedded entry of orders.timeline（文档中的 order_timeline）。
type TimelineItem struct {
	Name string    `bson:"name"`
	Desc string    `bson:"desc,omitempty"`
	T    time.Time `bson:"t"`
}

// DispatchTask mirrors the dispatch_tasks collection.
type DispatchTask struct {
	ID        bson.ObjectID   `bson:"_id"`
	TaskNo    string          `bson:"taskNo"`
	State     string          `bson:"state"`
	DriverID  bson.ObjectID   `bson:"driverId"`
	OrderIDs  []bson.ObjectID `bson:"orderIds"`
	ArrivedAt *time.Time      `bson:"arrivedAt"`
	// AssignmentRevision 是派单轮次：每次派单/改派 +1，退单/改派/结束与
	// 联系授权共用它作为同一把分配锁。
	AssignmentRevision int64 `bson:"assignmentRevision"`
}

// PriceSnapshot is the fare snapshot locked when the order was placed
// （mongodb.js: priceSnap { T,Q,H,F,K,fixed }，单位分）。行程开始只引用它，
// 缺失时保持 ARRIVED 并报服务异常，不伪造计价版本。
type PriceSnapshot struct {
	T     int64 `bson:"T"`
	Q     int64 `bson:"Q"`
	H     int64 `bson:"H"`
	F     int64 `bson:"F"`
	K     int64 `bson:"K"`
	Fixed bool  `bson:"fixed"`
}

// VehicleSnapshot 是分配任务时锁定的车辆版本快照。
// 写入订单后，续证、改车或注销都不再改写旧订单。
type VehicleSnapshot struct {
	VehicleID      string `bson:"vehicleId"`
	RevisionID     string `bson:"revisionId"`
	Plate          string `bson:"plate"`
	ModelCode      string `bson:"modelCode"`
	Seats          int    `bson:"seats"`
	Color          string `bson:"color"`
	BindingVersion int64  `bson:"bindingVersion"`
}

// RideOrder mirrors the orders collection fields owned by trip execution.
// The order document is the aggregate root: its timeline is embedded so one
// document update remains atomic on a standalone server.
type RideOrder struct {
	ID            bson.ObjectID  `bson:"_id"`
	OrderNo       string         `bson:"orderNo"`
	PassengerID   bson.ObjectID  `bson:"passengerId"`
	State         string         `bson:"state"`
	Version       int64          `bson:"version"`
	DriverID      bson.ObjectID  `bson:"driverId"`
	DispatchID    bson.ObjectID  `bson:"dispatchId"`
	ArrivedAt     *time.Time     `bson:"arrivedAt"`
	StartID       bson.ObjectID  `bson:"startId"`
	StartLocation *GeoPoint      `bson:"startLocation"`
	Timeline      []TimelineItem `bson:"timeline"`
	// D06 上车核验：计数与锁定在订单文档内原子更新，不另建授权票据。
	Verified          bool       `bson:"verified"`
	VerifyFailures    int        `bson:"verifyFailures"`
	VerifyLockedUntil *time.Time `bson:"verifyLockedUntil"`
	// D06 开始与送达
	BoardedAt  *time.Time     `bson:"boardedAt"`
	StartedAt  *time.Time     `bson:"startedAt"`
	EndedAt    *time.Time     `bson:"endedAt"`
	ActualDist *int64         `bson:"actualDist"`
	ActualDur  *int64         `bson:"actualDur"`
	PriceSnap  *PriceSnapshot `bson:"priceSnap"`
	CfgVersion *int64         `bson:"cfgVersion"`
	Fixed      bool           `bson:"fixed"`
	Changes    int            `bson:"changes"`
	// D06 计费两阶段：COLLECTING → READY → 计价域按 billingRequestId 去重消费。
	BillingRequestID  string `bson:"billingRequestId,omitempty"`
	BillingInputState string `bson:"billingInputState,omitempty"`
	// D07 乘客联系：消息正文与历史内嵌，不另建聊天真相。
	Messages    []OrderMessage      `bson:"messages"`
	VirtualCall *VirtualCallBinding `bson:"virtualCall,omitempty"`
	// D14 分配任务时锁定的车辆版本快照；写入后不因续证、改车或注销而改写。
	VehicleSnap *VehicleSnapshot `bson:"vehicleSnap,omitempty"`
}

// Driver mirrors the drivers collection fields used by trip execution. The
// latest position is embedded so the arrival decision and the write share one
// document (接口文档 3.9.5 涉及 driver)。
type Driver struct {
	ID             bson.ObjectID `bson:"_id"`
	LastLocation   *GeoPoint     `bson:"lastLocation"`
	LastLocationAt *time.Time    `bson:"lastLocationAt"`
}

// EventLog mirrors the event_logs reliable event bus (eventId 唯一去重)。
type EventLog struct {
	EventID string    `bson:"eventId"`
	T       time.Time `bson:"t"`
	Name    string    `bson:"name"`
	Detail  string    `bson:"detail"`
	Kind    string    `bson:"kind"`
}

// CommandKey identifies one business command for idempotent replay.
type CommandKey struct {
	ActorID        string `bson:"actor_id"`
	CommandType    string `bson:"command_type"`
	ResourceID     string `bson:"resource_id"`
	IdempotencyKey string `bson:"idempotency_key"`
}

// CommandResult stores the outcome of a completed command so a repeated key
// answers with the original fact instead of applying the command twice.
type CommandResult struct {
	CommandKey  `bson:",inline"`
	RequestHash string    `bson:"request_hash"`
	ResultJSON  []byte    `bson:"result_json"`
	FactID      string    `bson:"fact_id"`
	CreatedAt   time.Time `bson:"created_at"`
}

// ArrivalMutation is everything the arrival command writes. Because the
// deployed MongoDB is standalone, each part is applied as a single-document
// atomic update guarded by version; there is no multi-document transaction.
type ArrivalMutation struct {
	TaskNo    string
	Orders    []RideOrder
	ArrivedAt time.Time
	Abnormal  bool
	Reason    string
	Distance  *float64
	Timeline  TimelineItem
	Events    []EventLog
	Command   CommandResult
}
