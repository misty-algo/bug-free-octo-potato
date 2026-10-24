package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// dispatchByListen 单通道分发的听单阶段：按候选评分邀请司机，同轮不重复邀请同一司机。
// 若当前没有可用听单运力，则立即把任务曝光到抢单池，避免运力空转。
func (s *Service) dispatchByListen(ctx context.Context, order *entity.HallOrder) {
	now := biztime.UTC()
	if order.Status != entity.OrderStatusWaiting {
		return
	}
	if now.After(order.DispatchDeadline) {
		return
	}
	if order.DriverID != "" && !order.InviteExpired(now) {
		return // 已有生效邀请
	}
	candidates, err := s.drivers.FindListenCandidates(ctx, entity.ListenFilter{
		Lat: order.Origin.Lat, Lng: order.Origin.Lng,
		CityCode: order.CityCode, Seats: order.RequireSeats, CarType: order.RequireCarType,
		ExcludeIDs: append(append([]string{}, order.RoundInvited...), order.AssignExclude...),
		Limit:      50,
	})
	if err != nil {
		s.log.Warn("find listen candidates failed", zap.String("order_id", order.ID), zap.Error(err))
		return
	}
	usable := make([]*entity.Driver, 0, len(candidates))
	for _, d := range candidates {
		if d.ActiveTaskID != "" {
			continue // 唯一约束兜底：已有活动任务不再派单
		}
		if d.CanReceiveOrder() != nil {
			continue
		}
		if !s.filterCandidate(d, order) {
			continue
		}
		usable = append(usable, d)
	}
	if len(usable) == 0 {
		if !order.VisibleInPool {
			if _, err := s.orders.OpenToGrabPool(ctx, order.ID); err == nil {
				if fresh, gErr := s.orders.Get(ctx, order.ID); gErr == nil {
					*order = *fresh
				}
			}
		}
		return
	}
	s.rankCandidates(usable, order)
	picked := usable[0]
	var pickupMeters int64
	if picked.LastLat != 0 || picked.LastLng != 0 {
		pickupMeters = order.DistanceTo(picked.LastLat, picked.LastLng)
	}
	assigned, err := s.orders.Assign(ctx, order.ID, picked.ID, picked.Name, 0, now.Add(s.acceptTimeout()), pickupMeters)
	if err != nil {
		return // 并发场景：任务已被他人占用，交还下一轮处理
	}
	if assigned.DispatchChannel == entity.DispatchChannelListen && assigned.PickupDistanceMeters == 0 {
		assigned.PickupDistanceMeters = assigned.DistanceTo(picked.LastLat, picked.LastLng)
	}
	if err := s.bindPreOccupied(ctx, picked, assigned.TaskID); err != nil {
		_, _ = s.orders.ReleaseToWaiting(ctx, assigned.ID, picked.ID, contracts.PartyPlatform, "driver_bind_failed")
		return
	}
	*order = *assigned
	s.emit(ctx, &contracts.TaskAssignedPayload{
		EventID: fmt.Sprintf("tas-%s-%d", assigned.ID, assigned.InviteRound),
		TaskID: assigned.TaskID, OrderID: assigned.OrderID, DriverID: picked.ID,
		Channel: assigned.DispatchChannel, InviteRound: assigned.InviteRound,
		AcceptDeadline: biztime.RFC3339(assigned.AcceptDeadline), OccurredAt: biztime.RFC3339(now),
	}, contracts.EventTaskAssigned)
}

// TickResult 是调度周期的处理统计（用于日志与排障）。
type TickResult struct {
	InviteExpired   int
	Dispatched      int
	SwitchedToPool  int
	CancelExpired   int
	HeartbeatOffline int
}

// DispatchTick 后台调度周期（建议 1 秒执行一次）：
//  1. 邀请 15 秒超时未确认 → 释放司机运力，同轮更换下一候选；
//  2. 听单通道任务继续派发（继承剩余预算）；
//  3. 听单窗口结束 → 转入抢单池曝光；
//  4. 超过总派单时限 → 平台责任取消；
//  5. 空闲司机心跳超时 → 自动收车（忙碌掉线保留任务）。
func (s *Service) DispatchTick(ctx context.Context) TickResult {
	now := biztime.UTC()
	var res TickResult

	expired, err := s.orders.FindPendingInviteExpired(ctx, now, 100)
	if err == nil {
		for _, o := range expired {
			if _, rErr := s.orders.ReleaseToWaiting(ctx, o.ID, o.DriverID, contracts.PartyPlatform, "invite_timeout"); rErr != nil {
				continue
			}
			if d, dErr := s.drivers.Get(ctx, o.DriverID); dErr == nil {
				_ = s.releaseDriver(ctx, d, false)
			}
			res.InviteExpired++
			if fresh, gErr := s.orders.Get(ctx, o.ID); gErr == nil {
				s.dispatchByListen(ctx, fresh)
				res.Dispatched++
			}
		}
	}

	listening, err := s.orders.FindListeningTasks(ctx, now, 100)
	if err == nil {
		for _, o := range listening {
			before := o.DriverID
			s.dispatchByListen(ctx, o)
			if before == "" && o.DriverID != "" {
				res.Dispatched++
			}
		}
	}

	switchable, err := s.orders.FindWindowSwitchable(ctx, now, 100)
	if err == nil {
		for _, o := range switchable {
			if _, err := s.orders.OpenToGrabPool(ctx, o.ID); err == nil {
				res.SwitchedToPool++
			}
		}
	}

	overdue, err := s.orders.FindDispatchExpired(ctx, now, 100)
	if err == nil {
		for _, o := range overdue {
			if _, err := s.orders.Cancel(ctx, o.ID, contracts.PartyPlatform, "dispatch_window_expired"); err != nil {
				continue
			}
			if o.DriverID != "" {
				if d, dErr := s.drivers.Get(ctx, o.DriverID); dErr == nil {
					_ = s.releaseDriver(ctx, d, false)
				}
			}
			res.CancelExpired++
		}
	}

	timeoutSec := s.cfg.HeartbeatTimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	idleExpired, err := s.drivers.FindIdleHeartbeatExpired(ctx, now.Add(-time.Duration(timeoutSec)*time.Second), 100)
	if err == nil {
		for _, d := range idleExpired {
			if err := d.SetStatus(entity.DriverStatusOffline); err != nil {
				continue
			}
			if err := s.drivers.UpdateStatus(ctx, d.ID, d.Status, 0, d.UpdatedAt); err != nil {
				continue
			}
			res.HeartbeatOffline++
			s.emit(ctx, &contracts.DriverStatusChangedPayload{
				EventID: fmt.Sprintf("dsc-%s-hb-%d", d.ID, now.UnixNano()),
				DriverID: d.ID, FromStatus: entity.DriverStatusIdle, ToStatus: entity.DriverStatusOffline,
				Reason: "heartbeat_timeout", OccurredAt: biztime.RFC3339(now),
			}, contracts.EventDriverStatusChanged)
		}
	}
	return res
}
