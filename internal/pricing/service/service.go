package service

import (
	"context"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/pricing/entity"
)

// RuleSnapshotRepository 提供订单下单时锁定的计价规则快照。
type RuleSnapshotRepository interface {
	SnapshotByOrder(ctx context.Context, orderNo string) (entity.PricingRule, error)
}

// CalculateBillCommand 是正式计价的入参。
type CalculateBillCommand struct {
	OrderNo          string
	BillingRequestID string
	DistanceM        int64
	DurationSec      int64
	WaitSec          int64
	StartAt          time.Time
}

// CalculateBillResult 是正式计价的结果。
type CalculateBillResult struct {
	OrderNo          string
	BillingRequestID string
	// PassengerPaid 是乘客实付，单位「分」，已按规则上下限钳制。
	PassengerPaid    int64
	Breakdown        entity.FareBreakdown
	CommissionRateBP int64
}

// Service 是计价应用服务。
type Service struct {
	snapshots RuleSnapshotRepository
}

// New 创建计价服务。
func New(snapshots RuleSnapshotRepository) *Service {
	return &Service{snapshots: snapshots}
}

// CalculateBill 按订单下单时锁定的规则快照计算车费。
// 必须取快照而非当前规则：否则规则在行程期间被修改，会导致实付与下单时的预估不一致。
func (s *Service) CalculateBill(ctx context.Context, cmd CalculateBillCommand) (CalculateBillResult, error) {
	rule, err := s.snapshots.SnapshotByOrder(ctx, cmd.OrderNo)
	if err != nil {
		return CalculateBillResult{}, err
	}
	breakdown := entity.Calculate(rule, entity.FareInput{
		DistanceM:   cmd.DistanceM,
		DurationSec: cmd.DurationSec,
		WaitSec:     cmd.WaitSec,
		StartAt:     cmd.StartAt,
	})
	return CalculateBillResult{
		OrderNo:          cmd.OrderNo,
		BillingRequestID: cmd.BillingRequestID,
		PassengerPaid:    breakdown.Total,
		Breakdown:        breakdown,
		// 佣金率只透传，分账由 settlement 域负责，避免两处各算一遍导致口径漂移。
		CommissionRateBP: rule.CommissionRateBP,
	}, nil
}
