package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// objectID converts a string identifier from an access token into the ObjectId
// shape stored by the shared database.
func objectID(id string) (bson.ObjectID, error) { return bson.ObjectIDFromHex(id) }

type ArrivalService struct {
	repo  entity.ArrivalRepository
	clock clock.Clock
}

func NewArrivalService(r entity.ArrivalRepository, c clock.Clock) *ArrivalService {
	return &ArrivalService{repo: r, clock: c}
}

func utcMillis(t time.Time) time.Time { return t.UTC().Truncate(time.Millisecond) }

func requestHash(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", apperrors.InvalidArgument("INVALID_ARGUMENT", "request cannot be encoded")
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func validateKey(key string) error {
	if len(key) < 1 || len(key) > 128 {
		return apperrors.InvalidArgument("INVALID_IDEMPOTENCY_KEY", "idempotency key must contain 1 to 128 ASCII characters")
	}
	for _, c := range key {
		if c < 33 || c > 126 {
			return apperrors.InvalidArgument("INVALID_IDEMPOTENCY_KEY", "idempotency key must use printable ASCII")
		}
	}
	return nil
}

func requireRole(p authn.Principal, role string) error {
	if p.UserID == "" {
		return apperrors.Unauthorized("UNAUTHENTICATED", "authentication required")
	}
	if p.Role != role {
		return apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "role is not permitted")
	}
	return nil
}

func replay[T any](record *entity.CommandResult, hash string) (*T, error) {
	if record == nil {
		return nil, nil
	}
	if record.RequestHash != hash {
		return nil, apperrors.Conflict("IDEMPOTENCY_CONFLICT", "idempotency key was used with different content")
	}
	var result T
	if err := json.Unmarshal(record.ResultJSON, &result); err != nil {
		return nil, fmt.Errorf("decode saved command: %w", err)
	}
	return &result, nil
}

func commandRecord(key entity.CommandKey, hash, fact string, result any, now time.Time) (entity.CommandResult, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return entity.CommandResult{}, fmt.Errorf("encode command result: %w", err)
	}
	return entity.CommandResult{CommandKey: key, RequestHash: hash, FactID: fact, ResultJSON: b, CreatedAt: now}, nil
}

// normalizeTaskState tolerates the lowercase spellings written by earlier
// dispatch prototypes so one canonical state machine is enforced.
func normalizeTaskState(state string) string {
	switch strings.ToUpper(state) {
	case "ACCEPTED":
		return entity.TaskAssigned
	case entity.TaskAssigned, entity.TaskActive, entity.TaskFinished:
		return strings.ToUpper(state)
	default:
		return strings.ToUpper(state)
	}
}

// RecordLocation stores the driver's latest position (接口文档 3.9.5). It is the
// prerequisite of 到达确认: the arrival decision reads this evidence.
func (s *ArrivalService) RecordLocation(ctx context.Context, cmd dto.RecordLocationCommand) (*dto.LocationReceipt, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if math.IsNaN(cmd.Lng) || math.IsNaN(cmd.Lat) || cmd.Lng < -180 || cmd.Lng > 180 || cmd.Lat < -90 || cmd.Lat > 90 {
		return nil, apperrors.InvalidArgument("INVALID_LOCATION", "finite GCJ02 coordinates in geographic range are required")
	}
	driverID, err := objectID(cmd.Principal.UserID)
	if err != nil {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "token driver id is not a valid ObjectId")
	}
	at := utcMillis(s.clock.Now())
	if err := s.repo.RecordPosition(ctx, driverID, entity.Coordinate{Latitude: cmd.Lat, Longitude: cmd.Lng, CRS: "GCJ02"}, at); err != nil {
		return nil, err
	}
	return &dto.LocationReceipt{Accepted: true}, nil
}

// Arrive performs 到达上车点确认（接口文档 3.9.1）。
//
// 距上车点不超过 200 米且定位不超过 30 秒才允许常规确认；否则必须提交 reason 走
// 异常到达并生成复核标记。幂等键保证重复提交只产生一条到达事件。
func (s *ArrivalService) Arrive(ctx context.Context, cmd dto.ArriveCommand) (*dto.ArrivalResult, error) {
	if err := requireRole(cmd.Principal, "driver"); err != nil {
		return nil, err
	}
	if err := validateKey(cmd.IdempotencyKey); err != nil {
		return nil, err
	}
	cmd.TaskID = strings.TrimSpace(cmd.TaskID)
	if cmd.TaskID == "" {
		return nil, apperrors.InvalidArgument("INVALID_ARGUMENT", "task_id is required")
	}
	cmd.Reason = strings.TrimSpace(cmd.Reason)
	hash, err := requestHash(cmd)
	if err != nil {
		return nil, err
	}

	task, err := s.repo.TaskByNo(ctx, cmd.TaskID)
	if err != nil {
		return nil, err
	}
	// 对象级授权：只有当前任务的接单司机可以确认到达。
	if task.DriverID.Hex() != cmd.Principal.UserID {
		return nil, apperrors.New(apperrors.Kind("FORBIDDEN"), "FORBIDDEN", "driver does not own this task")
	}
	switch normalizeTaskState(task.State) {
	case entity.TaskAssigned, entity.TaskActive:
	case entity.TaskFinished, "CANCELLED":
		return nil, apperrors.Conflict("INVALID_TASK_STATE", "task is already finished")
	default:
		return nil, apperrors.Conflict("INVALID_TASK_STATE", "task is not assigned to a driver")
	}

	key := entity.CommandKey{ActorID: cmd.Principal.UserID, CommandType: "ARRIVE", ResourceID: task.TaskNo, IdempotencyKey: cmd.IdempotencyKey}
	saved, err := s.repo.FindCommand(ctx, key)
	if err != nil {
		return nil, err
	}
	if result, err := replay[dto.ArrivalResult](saved, hash); err != nil || result != nil {
		return result, err
	}

	orders, err := s.repo.OrdersByIDs(ctx, task.OrderIDs)
	if err != nil {
		return nil, err
	}
	now := utcMillis(s.clock.Now())
	pending := make([]entity.RideOrder, 0, len(orders))
	for _, order := range orders {
		switch strings.ToUpper(order.State) {
		case entity.OrderCancelled:
			continue
		case entity.OrderAccepted:
			pending = append(pending, order)
		case entity.OrderArrived, entity.OrderInTrip, entity.OrderEnded:
			// 已到达：幂等收敛，不重复推进、不重置等待计时。
		default:
			return nil, apperrors.Conflict("INVALID_ORDER_STATE", "order is not accepted")
		}
	}
	if len(orders) == 0 || (len(pending) == 0 && len(orders) == countCancelled(orders)) {
		return nil, apperrors.Conflict("NO_ACTIVE_ORDERS", "task has no accepted orders")
	}
	pickupSource := pending
	if len(pickupSource) == 0 {
		pickupSource = nonCancelled(orders)
	}
	if len(pickupSource) == 0 {
		return nil, apperrors.Conflict("NO_ACTIVE_ORDERS", "task has no accepted orders")
	}
	pickup, err := s.repo.Pickup(ctx, &pickupSource[0])
	if err != nil {
		return nil, err
	}
	driver, err := s.repo.DriverByID(ctx, task.DriverID)
	if err != nil {
		return nil, err
	}
	decision, err := entity.EvaluateArrival(pickup, driver, entity.ArrivalInput{Reason: cmd.Reason}, now)
	if err != nil {
		return nil, err
	}

	arrivalID := uuid.NewString()
	result := &dto.ArrivalResult{TaskID: task.TaskNo, State: entity.TaskActive, Abnormal: decision.Abnormal}
	description := arrivalDescription(decision, cmd.Reason)
	mutation := entity.ArrivalMutation{
		TaskNo:    task.TaskNo,
		ArrivedAt: now,
		Abnormal:  decision.Abnormal,
		Reason:    cmd.Reason,
		Distance:  decision.DistanceMeters,
		Timeline:  entity.TimelineItem{Name: "司机已到达", Desc: description, T: now},
	}
	for _, order := range pending {
		order.Version++
		order.State = entity.OrderArrived
		order.ArrivedAt = &now
		mutation.Orders = append(mutation.Orders, order)
	}
	detail := fmt.Sprintf("%s 到达 %s", task.TaskNo, description)
	mutation.Events = append(mutation.Events, entity.EventLog{EventID: "ARRIVE:" + arrivalID, T: now, Name: "DriverArrived", Detail: detail, Kind: "trip"})
	if decision.Abnormal {
		// 异常到达：不静默放行，额外生成复核标记供后台复核。
		review := decision.Reason
		if cmd.Reason != "" {
			review = fmt.Sprintf("%s；司机说明：%s", decision.Reason, cmd.Reason)
		}
		mutation.Events = append(mutation.Events, entity.EventLog{EventID: "ARRIVAL_REVIEW:" + arrivalID, T: now, Name: "ArrivalReviewCreated", Detail: review, Kind: "review"})
	}
	mutation.Command, err = commandRecord(key, hash, arrivalID, result, now)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveArrival(ctx, mutation); err != nil {
		return nil, err
	}
	return result, nil
}

func arrivalDescription(d entity.ArrivalDecision, reason string) string {
	if d.Normal {
		return "常规到达，已通知乘客尽快上车"
	}
	if reason == "" {
		return "异常到达：" + d.Reason
	}
	return "异常到达：" + d.Reason + "；" + reason
}

func countCancelled(orders []entity.RideOrder) int {
	n := 0
	for _, o := range orders {
		if strings.ToUpper(o.State) == entity.OrderCancelled {
			n++
		}
	}
	return n
}

func nonCancelled(orders []entity.RideOrder) []entity.RideOrder {
	out := make([]entity.RideOrder, 0, len(orders))
	for _, o := range orders {
		if strings.ToUpper(o.State) != entity.OrderCancelled {
			out = append(out, o)
		}
	}
	return out
}
