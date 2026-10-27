package entity

import (
	"errors"
	"fmt"
	"time"
)

// 行程领域错误。
var (
	ErrTripNotFound        = errors.New("trip not found")
	ErrInvalidTripStatus   = errors.New("invalid trip status")
	ErrInvalidTrip         = errors.New("invalid trip")
	ErrTripVersionConflict = errors.New("trip version conflict")
)

// 乘客侧行程聚合根。生命周期：WAITING → ACCEPTED → ARRIVED → IN_TRIP → ENDED，
// 未上车前可 CANCELLED。这里是乘客付钱、司机挣钱的那条链的唯一事实来源。
//
// 金额相关的三个字段（PassengerPaid / CommissionRateBP / BillCreated）只在行程
// 结束时一次写入：费率取自计价规则快照而非调用方传参，避免规则变更导致分账漂移。
type Trip struct {
	ID          string
	PassengerID string
	DriverID    string
	Status      string

	Origin      GeoPoint
	Destination GeoPoint

	// 行程实测值，由司机端在结束时上报，正式计价的输入。
	DistanceM   int64
	DurationSec int64
	WaitSec     int64

	StartedAt *time.Time
	EndedAt   *time.Time

	// BillingRequestID 本次正式计价的幂等键，与计价域一一对应。
	BillingRequestID string
	// PassengerPaid 乘客实付（分），由计价域算出后回填。
	PassengerPaid int64
	// CommissionRateBP 行程结束时由计价规则快照锁定的平台佣金率（万分比）。
	// 建账时随账单一起落库，后续分账只认这个值。
	CommissionRateBP int64
	// BillCreated 账单已创建标记：建账与行程结束不在同一事务，
	// 靠它保证重试不会重复建账。
	BillCreated bool

	// Version 乐观锁版本。司机接单与乘客取消会并发写同一行程，
	// 靠它保证后写者覆盖不了先写者的状态流转。
	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

// 合法状态流转表。终点状态（ENDED/CANCELLED）不可再流转。
var tripTransitions = map[string][]string{
	TripStatusWaiting:  {TripStatusAccepted, TripStatusCanceled},
	TripStatusAccepted: {TripStatusArrived, TripStatusCanceled},
	TripStatusArrived:  {TripStatusInTrip, TripStatusCanceled},
	TripStatusInTrip:   {TripStatusEnded},
	TripStatusEnded:    {},
	TripStatusCanceled: {},
}

// canTransit 判断当前状态能否流转到目标状态。
func (t *Trip) canTransit(to string) bool {
	for _, next := range tripTransitions[t.Status] {
		if next == to {
			return true
		}
	}
	return false
}

// transit 执行状态流转，非法流转返回错误。
func (t *Trip) transit(to string, at time.Time) error {
	if t == nil || t.ID == "" {
		return ErrInvalidTrip
	}
	if !t.canTransit(to) {
		return fmt.Errorf("%w: cannot transit from %s to %s", ErrInvalidTripStatus, t.Status, to)
	}
	t.Status = to
	t.UpdatedAt = at
	return nil
}

// NewTrip 创建待接单行程。起点终点必须合法，乘客必填。
func NewTrip(id, passengerID string, origin, destination GeoPoint, now time.Time) (*Trip, error) {
	if id == "" || passengerID == "" {
		return nil, fmt.Errorf("%w: trip_id and passenger_id are required", ErrInvalidTrip)
	}
	if err := validatePoint(origin); err != nil {
		return nil, fmt.Errorf("%w: origin %v", ErrInvalidTrip, err)
	}
	if err := validatePoint(destination); err != nil {
		return nil, fmt.Errorf("%w: destination %v", ErrInvalidTrip, err)
	}
	return &Trip{
		ID:          id,
		PassengerID: passengerID,
		Status:      TripStatusWaiting,
		Origin:      origin,
		Destination: destination,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// validatePoint 校验 GeoJSON Point 坐标合法。
func validatePoint(p GeoPoint) error {
	if p.Type != "Point" || len(p.Coordinates) != 2 {
		return errors.New("must be GeoJSON Point with [lng, lat]")
	}
	lng, lat := p.Coordinates[0], p.Coordinates[1]
	if lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return errors.New("coordinates out of range")
	}
	return nil
}

// AssignDriver 司机接单：WAITING → ACCEPTED。
func (t *Trip) AssignDriver(driverID string, at time.Time) error {
	if driverID == "" {
		return fmt.Errorf("%w: driver_id is required", ErrInvalidTrip)
	}
	if err := t.transit(TripStatusAccepted, at); err != nil {
		return err
	}
	t.DriverID = driverID
	return nil
}

// Arrive 司机到达上车点：ACCEPTED → ARRIVED。
func (t *Trip) Arrive(at time.Time) error {
	return t.transit(TripStatusArrived, at)
}

// Board 乘客上车：ARRIVED → IN_TRIP。
func (t *Trip) Board(at time.Time) error {
	if err := t.transit(TripStatusInTrip, at); err != nil {
		return err
	}
	t.StartedAt = &at
	return nil
}

// Cancel 取消行程：未上车前的任一状态 → CANCELLED。
func (t *Trip) Cancel(at time.Time) error {
	return t.transit(TripStatusCanceled, at)
}

// End 送达：IN_TRIP → ENDED，并记录实测里程/时长/等待（正式计价输入）。
// 金额为 0 时拒绝结束——不让它静默建一张 0 元账单。
func (t *Trip) End(at time.Time, distanceM, durationSec, waitSec int64) error {
	if distanceM < 0 || durationSec < 0 || waitSec < 0 {
		return fmt.Errorf("%w: negative trip metrics", ErrInvalidTrip)
	}
	if err := t.transit(TripStatusEnded, at); err != nil {
		return err
	}
	t.EndedAt = &at
	t.DistanceM = distanceM
	t.DurationSec = durationSec
	t.WaitSec = waitSec
	return nil
}

// ApplyFare 回填计价结果与锁定的佣金率。只在行程已结束、且尚未回填时生效。
func (t *Trip) ApplyFare(billingRequestID string, passengerPaid, commissionRateBP int64, at time.Time) error {
	if t.Status != TripStatusEnded {
		return fmt.Errorf("%w: fare can only be applied on ENDED trip, current %s", ErrInvalidTripStatus, t.Status)
	}
	if commissionRateBP < 0 || commissionRateBP > 10000 {
		return fmt.Errorf("%w: commission_rate_bp must be within [0, 10000]", ErrInvalidTrip)
	}
	// 幂等：同一计价请求重复回填直接返回，避免金额被二次改写。
	if t.BillingRequestID == billingRequestID {
		return nil
	}
	if t.BillingRequestID != "" {
		return fmt.Errorf("%w: trip already billed with %s", ErrInvalidTripStatus, t.BillingRequestID)
	}
	t.BillingRequestID = billingRequestID
	t.PassengerPaid = passengerPaid
	t.CommissionRateBP = commissionRateBP
	t.UpdatedAt = at
	return nil
}

// MarkBillCreated 标记账单已创建。重复调用返回 false，调用方据此跳过重复建账。
func (t *Trip) MarkBillCreated(at time.Time) bool {
	if t.BillCreated {
		return false
	}
	t.BillCreated = true
	t.UpdatedAt = at
	return true
}
