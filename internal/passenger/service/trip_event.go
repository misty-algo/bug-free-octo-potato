package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// HandleTripEnded 消费司机域发布的 trip.ended 事件：
// 行程结束即进入待支付，乘客必须完成支付订单才算结束，司机收入也才有来源。
//
// 幂等性：状态机只允许推进一次，重复投递时 CanTransitionTo 返回 false，
// advanceTo 直接跳过，因此重复消费不产生副作用；订单已是终态同样自动跳过。
func (s *Service) HandleTripEnded(ctx context.Context, body []byte) error {
	var payload contracts.TripEndedPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		// 消息体损坏时不再重投（rabbitmqx 丢弃），返回错误只为留下日志。
		return fmt.Errorf("decode trip.ended payload: %w", err)
	}
	orderNo := strings.TrimSpace(payload.OrderID)
	if orderNo == "" {
		return nil
	}
	order, err := s.orders.FindByOrderNo(ctx, orderNo)
	if err != nil {
		// 订单不在本域（历史/其它环境订单）时丢弃，避免消息反复失败。
		if errors.Is(err, entity.ErrOrderNotFound) {
			return nil
		}
		return fmt.Errorf("find order %s: %w", orderNo, err)
	}
	if order.State.Terminal() {
		return nil
	}
	// 司机可能从"已接单"直接送达，中间态（已到达/行程中）逐级补齐留痕。
	if err := s.advanceTo(ctx, order, []entity.OrderState{
		entity.OrderStateAccepted,
		entity.OrderStateArrived,
		entity.OrderStateInService,
		entity.OrderStateCompletedUnpaid,
	}, "司机送达", s.now().UTC()); err != nil {
		return fmt.Errorf("advance order %s to unpaid: %w", orderNo, err)
	}
	return nil
}
