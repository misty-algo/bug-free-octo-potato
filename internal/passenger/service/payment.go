package service

import (
	"context"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// ───────────────── 支付成功联动：待支付 → 已完成 → 司机入账 ─────────────────

// MarkOrderPaid 支付成功回调落单：
// 1. 把订单从 COMPLETED_UNPAID 推进到 COMPLETED（幂等：已完成直接返回原单）；
// 2. 从司机域任务读取结算要素（接单司机与净得金额），供网关接着调用司机域记账。
// 司机入账本身由网关编排（MarkOrderPaid → driver.ReportPaymentSucceeded），
// 本域不跨服务直写司机资金账本。
func (s *Service) MarkOrderPaid(ctx context.Context, cmd dto.MarkOrderPaidCommand) (*dto.OrderPaidResult, error) {
	order, err := s.orders.FindByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	// 幂等：重复支付通知不重复推进、不报错。
	if order.State == entity.OrderStateCompleted {
		return s.orderPaidResult(ctx, order), nil
	}
	if order.State != entity.OrderStateCompletedUnpaid {
		return nil, entity.ErrInvalidStateTransition
	}
	if err := s.advanceTo(ctx, order, []entity.OrderState{entity.OrderStateCompleted}, "支付成功", s.now().UTC()); err != nil {
		return nil, err
	}
	return s.orderPaidResult(ctx, order), nil
}

// orderPaidResult 汇总支付联动结果；司机结算要素缺失时对应字段留空，
// 由调用方（网关）决定是否跳过司机域入账。
func (s *Service) orderPaidResult(ctx context.Context, order *entity.Order) *dto.OrderPaidResult {
	result := &dto.OrderPaidResult{
		OrderID: order.ID,
		OrderNo: order.OrderNo,
		State:   string(order.State),
		PaidFen: order.TotalFee,
	}
	if s.hall == nil {
		return result
	}
	settle, err := s.hall.FindSettlementByOrderNo(ctx, order.OrderNo)
	if err != nil {
		return result
	}
	result.DriverID = settle.DriverID
	result.PaidFen = settle.PassengerPaidFen
	result.DriverIncomeFen = settle.DriverIncomeFen
	return result
}

// platformCommissionFen 按基点计算平台佣金，四舍五入到分。
func (s *Service) platformCommissionFen(paidFen int64) int64 {
	bp := s.commissionBP
	if bp <= 0 {
		bp = 2000
	}
	return (paidFen*bp + 5000) / 10000
}
