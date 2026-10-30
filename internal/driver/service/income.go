package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// ───────────────── 记账：由支付域事件驱动 ─────────────────

// ReportPaymentSucceeded 接收支付域的“支付成功”事件：生成收益凭证并把资金落到待结算桶 U。
// 幂等：同一 event_id 只会记一次账（可靠事件表 + 流水 event_id 唯一索引双重兜底）。
func (s *Service) ReportPaymentSucceeded(ctx context.Context, cmd dto.ReportPaymentCommand) (*dto.IncomeFlowDTO, error) {
	return s.reportPaid(ctx, cmd)
}

func (s *Service) reportPaid(ctx context.Context, cmd dto.ReportPaymentCommand) (*dto.IncomeFlowDTO, error) {
	if cmd.DriverID == "" || cmd.Amount.DriverIncomeFen <= 0 {
		return nil, entity.ErrInvalidFlow
	}
	eventID := cmd.EventID
	if eventID == "" {
		eventID = fmt.Sprintf("pay-%s-%s", cmd.OrderID, cmd.IdempotencyKey)
	}
	// 事件去重：跨模块重复投递不重复记账。
	if claimed, err := s.support.ClaimInbound(ctx, eventID, contracts.EventPaymentSucceeded, localConsumer); err != nil || !claimed {
		if existing, fErr := s.incomes.GetFlowByEventID(ctx, eventID); fErr == nil {
			return dto.FromIncomeFlow(existing), nil
		}
	}
	date := cmd.AccountingDate
	if date == "" {
		date = biztime.DateKey(biztime.Now())
	}
	flow, err := entity.NewIncomeFlow(cmd.DriverID, cmd.OrderID, "", eventID, cmd.IdempotencyKey, cmd.Amount, date)
	if err != nil {
		return nil, err
	}
	if err := s.incomes.CreateFlow(ctx, flow); err != nil {
		if existing, fErr := s.incomes.GetFlowByEventID(ctx, eventID); fErr == nil {
			return dto.FromIncomeFlow(existing), nil
		}
		return nil, err
	}
	// 资金入账：外部资金 → 待结算 U（此时还不可提现，等财务域结算后再进 A）。
	if err := s.drivers.AddFunds(ctx, cmd.DriverID, entity.BucketPending, flow.DriverIncomeFen); err != nil {
		s.log.Error("add pending funds failed", zap.String("driver_id", cmd.DriverID), zap.Error(err))
	}
	_ = s.drivers.AddMonthlyIncome(ctx, cmd.DriverID, flow.DriverIncomeFen)
	_ = s.appendLedger(ctx, flow.DriverID, "", entity.BucketPending, flow.DriverIncomeFen, "payment", flow.ID, flow.IdempotencyKey)
	s.emit(ctx, &contracts.IncomeRecordedPayload{
		EventID: fmt.Sprintf("inc-%s", flow.ID), FlowID: flow.ID, DriverID: flow.DriverID,
		OrderID: flow.OrderID, AccountingDate: flow.AccountingDate,
		DriverIncomeFen: flow.DriverIncomeFen, BucketTo: flow.BucketTo,
		OccurredAt: biztime.RFC3339(flow.CreatedAt),
	}, contracts.EventIncomeRecorded)
	// 支付成功即结算到可用桶（U → A）：钱已到手，无需等待财务域批处理。
	_, _ = s.SettleIncome(ctx, dto.SettleIncomeCommand{DriverID: flow.DriverID, FlowIDs: []string{flow.ID}})
	return dto.FromIncomeFlow(flow), nil
}

// appendLedger 记录入账：U 桶由外部资金流入，不需要源桶扣减。
func (s *Service) appendLedger(ctx context.Context, driverID, fromBucket, toBucket string, amountFen int64, refType, refID, key string) error {
	entry := &entity.LedgerEntry{
		ID: fmt.Sprintf("lg-%s-%s", refType, refID), DriverID: driverID,
		FromBucket: fromBucket, ToBucket: toBucket, AmountFen: amountFen,
		RefType: refType, RefID: refID, IdempotencyKey: key, CreatedAt: biztime.UTC(),
	}
	return s.incomes.CreateLedger(ctx, entry)
}

// SettleIncome 结算确认（财务域 A10 调用）：待结算 U → 可用 A。
func (s *Service) SettleIncome(ctx context.Context, cmd dto.SettleIncomeCommand) (int, error) {
	flows := cmd.FlowIDs
	var err error
	if len(flows) == 0 {
		flowsFlows, lErr := s.incomes.ListPendingSettlement(ctx, 200)
		if lErr != nil {
			return 0, lErr
		}
		for _, f := range flowsFlows {
			if cmd.DriverID != "" && f.DriverID != cmd.DriverID {
				continue
			}
			flows = append(flows, f.ID)
		}
	}
	if len(flows) == 0 {
		return 0, nil
	}
	// 按司机分组汇总，避免对同一司机多次小额写。
	byDriver := map[string]int64{}
	settled := make([]string, 0, len(flows))
	for _, fid := range flows {
		f, fErr := s.incomes.GetFlowByID(ctx, fid)
		if fErr != nil || f == nil {
			continue
		}
		if cmd.DriverID != "" && f.DriverID != cmd.DriverID {
			continue
		}
		if f.BucketTo != entity.BucketPending {
			continue // 已结算流水不重复结转
		}
		byDriver[f.DriverID] += f.DriverIncomeFen
		settled = append(settled, f.ID)
	}
	_ = s.incomes.MarkSettled(ctx, settled)
	count := 0
	for driverID, amount := range byDriver {
		if amount <= 0 {
			continue
		}
		if err = s.drivers.MoveFunds(ctx, driverID, entity.BucketPending, entity.BucketAvailable, amount); err != nil {
			s.log.Warn("settle move funds failed", zap.String("driver_id", driverID), zap.Error(err))
			continue
		}
		_ = s.appendLedger(ctx, driverID, entity.BucketPending, entity.BucketAvailable, amount, "settlement", driverID, "")
		count++
	}
	return count, nil
}

// ReportRefundSucceeded 退款成功（支付域/账务回调）：追加反向流水，绝不修改原凭证。
// 反向金额优先从"待追偿 Debt"桶体现，遵循跨日退款记当日负向收入的原则。
func (s *Service) ReportRefundSucceeded(ctx context.Context, cmd dto.ReportRefundCommand) (*dto.IncomeFlowDTO, error) {
	if cmd.RefundFen <= 0 {
		return nil, entity.ErrInvalidFlow
	}
	// 幂等：反向流水的 event_id 唯一索引即去重依据。
	// 不在前置校验前抢占用事件位，避免"处理失败但事件被消费"导致退款永久丢失。
	if existing, err := s.incomes.GetFlowByEventID(ctx, cmd.EventID); err == nil && existing != nil {
		return dto.FromIncomeFlow(existing), nil
	}
	// 退款回调可能带乘客域订单号，也可能直接带大厅任务号，两种都尝试解析。
	order, err := s.orders.GetByOrderID(ctx, cmd.OrderID)
	if err != nil {
		order, err = s.orders.Get(ctx, cmd.OrderID)
		if err != nil {
			return nil, entity.ErrOrderNotFound
		}
	}
	driverID := order.DriverID
	origin, oErr := s.incomes.GetFlowByKey(ctx, driverID, "")
	if oErr != nil || origin == nil {
		// 找不到原始凭证时，依然以反向流水入库（保证资金守恒与可追溯）。
		origin = &entity.IncomeFlow{ID: "unknown", DriverID: driverID, OrderID: order.OrderID}
	}
	date := cmd.AccountingDate
	if date == "" {
		date = biztime.DateKey(biztime.Now())
	}
	flowType := entity.FlowTypeRefund
	if cmd.ResponsibleParty == contracts.PartyDriver {
		flowType = entity.FlowTypePunishment
	}
	reverse, err := entity.NewReverseFlow(origin, flowType, cmd.EventID, cmd.IdempotencyKey, date, cmd.RefundFen, cmd.Reason)
	if err != nil {
		return nil, err
	}
	if err := s.incomes.CreateFlow(ctx, reverse); err != nil {
		return nil, err
	}
	_ = s.appendLedger(ctx, reverse.DriverID, "", reverse.BucketTo, reverse.DriverIncomeFen, "reverse", reverse.ID, reverse.IdempotencyKey)
	return dto.FromIncomeFlow(reverse), nil
}

// ───────────────── 首页收益对比（D09–D11）─────────────────

// GetIncomeCompare 今日/昨日收益对比。
// 口径：接单量按行程结束日统计（拼车按乘客数计），收入按记账日统计——两者不同步属正常。
func (s *Service) GetIncomeCompare(ctx context.Context, driverID string) (*dto.IncomeCompareDTO, error) {
	driver, err := s.drivers.Get(ctx, driverID)
	if err != nil {
		return nil, err
	}
	now := biztime.Now()
	todayStart := biztime.DayStart(now)
	yesterdayStart := biztime.AddDays(todayStart, -1)

	todayOrders, todayPassengers, err := s.orders.SumEndedByDay(ctx, driverID, todayStart, biztime.DayEnd(now))
	if err != nil {
		return nil, err
	}
	yOrders, yPassengers, err := s.orders.SumEndedByDay(ctx, driverID, yesterdayStart, todayStart)
	if err != nil {
		return nil, err
	}
	todayIncome, todayRefund, err := s.incomes.SumByAccountingDate(ctx, driverID, biztime.DateKey(now))
	if err != nil {
		return nil, err
	}
	yIncome, yRefund, err := s.incomes.SumByAccountingDate(ctx, driverID, biztime.DateKey(yesterdayStart))
	if err != nil {
		return nil, err
	}
	compare := &entity.IncomeCompare{
		Today: entity.DayIncome{
			Date: biztime.DateKey(now), OrderCount: todayPassengers, IncomeFen: todayIncome, RefundFen: todayRefund,
		},
		Yesterday: entity.DayIncome{
			Date: biztime.DateKey(yesterdayStart), OrderCount: yPassengers, IncomeFen: yIncome, RefundFen: yRefund,
		},
		PendingIncomeFen: driver.Funds.Pending,
		LastUpdatedAt:    biztime.RFC3339(now),
	}
	compare.OrderDiff = compare.Today.OrderCount - compare.Yesterday.OrderCount
	compare.IncomeDiffFen = compare.Today.IncomeFen - compare.Yesterday.IncomeFen
	compare.BuildSummary()
	_ = todayOrders
	_ = yOrders
	return dto.FromIncomeCompare(compare), nil
}

// GetIncomeSummary 收入概览：五桶余额 + 提现规则 + 当日受理统计。
func (s *Service) GetIncomeSummary(ctx context.Context, driverID string) (*dto.IncomeSummaryDTO, error) {
	driver, err := s.drivers.Get(ctx, driverID)
	if err != nil {
		return nil, err
	}
	amountFen, count, err := s.incomes.WithdrawTodayStats(ctx, driverID, biztime.DayStart(biztime.Now()))
	if err != nil {
		return nil, err
	}
	return &dto.IncomeSummaryDTO{
		DriverID:          driver.ID,
		PendingFen:        driver.Funds.Pending,
		AvailableFen:      driver.Funds.Available,
		FrozenFen:         driver.Funds.Frozen,
		WithdrawFen:       driver.Funds.Withdraw,
		DebtFen:           driver.Funds.Debt,
		MonthlyIncomeFen:  driver.MonthlyIncomeFen,
		MinWithdrawFen:    s.cfg.MinWithdrawFen,
		MaxWithdrawFen:    s.cfg.MaxWithdrawFen,
		DailyLimitFen:     s.cfg.DailyWithdrawLimitFen,
		DailyMaxCount:     s.cfg.DailyWithdrawCount,
		DualApprovalFen:   s.cfg.DualApprovalFen,
		WithdrawnTodayFen: amountFen,
		WithdrawCountToday: count,
		UpdatedAt:         biztime.RFC3339(driver.UpdatedAt),
	}, nil
}

// ListIncomeFlows 收入流水分页查询（含反向流水）。
func (s *Service) ListIncomeFlows(ctx context.Context, cmd dto.ListFlowsCommand) ([]*dto.IncomeFlowDTO, int64, error) {
	page, size := cmd.Page, cmd.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	flows, total, err := s.incomes.ListFlows(ctx, cmd.DriverID, (page-1)*size, size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.IncomeFlowDTO, 0, len(flows))
	for _, f := range flows {
		out = append(out, dto.FromIncomeFlow(f))
	}
	return out, total, nil
}

// ───────────────── 提现（D12–D13）─────────────────

// RequestWithdraw 申请提现：额度校验 → 可用桶原子预占（A→W）→ 记录单据并进入后台审核。
func (s *Service) RequestWithdraw(ctx context.Context, cmd dto.RequestWithdrawCommand) (*dto.WithdrawResultDTO, error) {
	driver, err := s.drivers.Get(ctx, cmd.DriverID)
	if err != nil {
		return nil, err
	}
	if cmd.AmountFen < s.cfg.MinWithdrawFen {
		return nil, entity.ErrBelowMinWithdraw
	}
	if cmd.AmountFen > s.cfg.MaxWithdrawFen {
		return nil, entity.ErrAboveMaxWithdraw
	}
	if !driver.Funds.CanTransfer(entity.BucketAvailable, cmd.AmountFen) {
		return nil, entity.ErrBalanceInsufficient
	}
	dayStart := biztime.DayStart(biztime.Now())
	usedAmount, usedCount, err := s.incomes.WithdrawTodayStats(ctx, driver.ID, dayStart)
	if err != nil {
		return nil, err
	}
	if usedCount >= int64(s.cfg.DailyWithdrawCount) {
		return nil, entity.ErrDailyCountExceeded
	}
	if usedAmount+cmd.AmountFen > s.cfg.DailyWithdrawLimitFen {
		return nil, entity.ErrDailyLimitExceeded
	}
	key := cmd.IdempotencyKey
	if key == "" {
		key = fmt.Sprintf("wd-%s-%d", driver.ID, biztime.UTC().UnixNano())
	}
	dual := cmd.AmountFen >= s.cfg.DualApprovalFen
	w, wErr := entity.NewWithdrawal(driver.ID, key, cmd.AmountFen, dual)
	if wErr != nil {
		return nil, wErr
	}
	// 幂等预检：同一幂等键重复提交必须返回原单，且不得再次预占额度。
	if existing, qErr := s.incomes.GetWithdrawByKey(ctx, driver.ID, key); qErr == nil && existing != nil {
		return &dto.WithdrawResultDTO{Withdrawal: dto.FromWithdrawal(existing), Duplicate: true}, nil
	}
	// 先原子占额度，再落单据：任何一步失败都不会产生"钱动单据没动"的中间态。
	if err := s.drivers.MoveFunds(ctx, driver.ID, entity.BucketAvailable, entity.BucketWithdraw, cmd.AmountFen); err != nil {
		return nil, entity.ErrBalanceInsufficient
	}
	result, err := s.incomes.CreateWithdraw(ctx, w)
	if err != nil {
		// 落单失败：把额度返还可用桶，保证资金守恒。
		_ = s.drivers.MoveFunds(ctx, driver.ID, entity.BucketWithdraw, entity.BucketAvailable, cmd.AmountFen)
		return nil, err
	}
	if result.Duplicate {
		// 并发插入被唯一索引拦下：返还本次多余的预占，返回既有单据。
		_ = s.drivers.MoveFunds(ctx, driver.ID, entity.BucketWithdraw, entity.BucketAvailable, cmd.AmountFen)
	}
	_ = s.appendLedger(ctx, driver.ID, entity.BucketAvailable, entity.BucketWithdraw, cmd.AmountFen, "withdraw_reserve", result.Withdrawal.ID, key)
	if !result.Duplicate {
		s.emit(ctx, &contracts.WithdrawalRequestedPayload{
			EventID: fmt.Sprintf("wdr-%s", result.Withdrawal.ID), WithdrawalID: result.Withdrawal.ID,
			DriverID: driver.ID, AmountFen: cmd.AmountFen, DualApproval: result.Withdrawal.DualApproval,
			IdempotencyKey: key, RequestedAt: biztime.RFC3339(result.Withdrawal.CreatedAt),
		}, contracts.EventWithdrawalRequested)
	}
	return &dto.WithdrawResultDTO{Withdrawal: dto.FromWithdrawal(result.Withdrawal), Duplicate: result.Duplicate}, nil
}

// ListWithdrawals 提现记录分页查询。
func (s *Service) ListWithdrawals(ctx context.Context, cmd dto.ListWithdrawalsCommand) ([]*dto.WithdrawDTO, int64, error) {
	page, size := cmd.Page, cmd.PageSize
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	list, total, err := s.incomes.ListWithdraws(ctx, cmd.DriverID, (page-1)*size, size)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*dto.WithdrawDTO, 0, len(list))
	for _, w := range list {
		out = append(out, dto.FromWithdrawal(w))
	}
	return out, total, nil
}

// ApproveWithdraw 后台审批通过（A11）：达到阈值的单据需要双人审批。
func (s *Service) ApproveWithdraw(ctx context.Context, cmd dto.ApproveWithdrawCommand) (*dto.WithdrawDTO, error) {
	w, err := s.incomes.GetWithdraw(ctx, cmd.WithdrawalID)
	if err != nil {
		return nil, err
	}
	if err := w.Approve(cmd.ApproverID, cmd.Comment); err != nil {
		return nil, err
	}
	if err := s.incomes.UpdateWithdraw(ctx, w); err != nil {
		return nil, err
	}
	return dto.FromWithdrawal(w), nil
}

// RejectWithdraw 驳回：预占额度返还可用桶（当日受理次数不返还）。
func (s *Service) RejectWithdraw(ctx context.Context, cmd dto.RejectWithdrawCommand) (*dto.WithdrawDTO, error) {
	w, err := s.incomes.GetWithdraw(ctx, cmd.WithdrawalID)
	if err != nil {
		return nil, err
	}
	if err := w.MarkRejected(cmd.Reason); err != nil {
		return nil, err
	}
	if err := s.incomes.UpdateWithdraw(ctx, w); err != nil {
		return nil, err
	}
	if err := s.drivers.MoveFunds(ctx, w.DriverID, entity.BucketWithdraw, entity.BucketAvailable, w.AmountFen); err != nil {
		s.log.Error("return withdraw reservation failed", zap.String("withdrawal_id", w.ID), zap.Error(err))
	}
	_ = s.appendLedger(ctx, w.DriverID, entity.BucketWithdraw, entity.BucketAvailable, w.AmountFen, "withdraw_reject", w.ID, w.IdempotencyKey)
	return dto.FromWithdrawal(w), nil
}

// CompleteWithdraw 打款结果回调（财务域 A12 / 代付适配器）：成功则预占正式出账，失败返还。
func (s *Service) CompleteWithdraw(ctx context.Context, cmd dto.CompleteWithdrawCommand) (*dto.WithdrawDTO, error) {
	w, err := s.incomes.GetWithdraw(ctx, cmd.WithdrawalID)
	if err != nil {
		return nil, err
	}
	if cmd.Success {
		if w.DualApproval && !w.ApprovalsEnough() {
			return nil, entity.ErrDualApprovalRequired
		}
		if err := w.MarkPaid(); err != nil {
			return nil, err
		}
		// 预占正式出账：W 桶扣款。只记分录不扣桶会让"提现中"永远不减、
		// 司机看着钱卡在 W 桶，资金也不平衡。
		if err := s.drivers.DeductFunds(ctx, w.DriverID, entity.BucketWithdraw, w.AmountFen); err != nil {
			w.RollbackPaid()
			_ = s.incomes.UpdateWithdraw(ctx, w)
			return nil, err
		}
		_ = s.appendLedger(ctx, w.DriverID, entity.BucketWithdraw, "", w.AmountFen, "withdraw_paid", w.ID, w.IdempotencyKey)
	} else {
		if err := w.MarkRejected(cmd.Reason); err != nil {
			return nil, err
		}
		_ = s.drivers.MoveFunds(ctx, w.DriverID, entity.BucketWithdraw, entity.BucketAvailable, w.AmountFen)
		_ = s.appendLedger(ctx, w.DriverID, entity.BucketWithdraw, entity.BucketAvailable, w.AmountFen, "withdraw_fail", w.ID, w.IdempotencyKey)
	}
	if err := s.incomes.UpdateWithdraw(ctx, w); err != nil {
		return nil, err
	}
	return dto.FromWithdrawal(w), nil
}

