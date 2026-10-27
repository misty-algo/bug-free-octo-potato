package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/settlement/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/settlement/entity"
)

// Service 编排 settlement 用例：账单生成与分账。
type Service struct {
	repo     entity.Repository
	txRunner entity.WithTransaction // 可选；nil 时退化为直接执行（无真实事务）
	log      *zap.Logger
}

// NewService 创建 settlement 用例服务。
func NewService(repo entity.Repository, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, log: log}
}

// SetTxRunner 注入事务执行器（MongoDB session 事务）。
func (s *Service) SetTxRunner(runner entity.WithTransaction) {
	s.txRunner = runner
}

// withTransaction 在事务内执行 fn；未配置事务执行器时直接执行。
func (s *Service) withTransaction(ctx context.Context, fn func(context.Context, entity.Repository) error) error {
	if s.txRunner == nil {
		return fn(ctx, s.repo)
	}
	return s.txRunner(ctx, func(tctx context.Context, repo entity.Repository) error {
		return fn(tctx, repo)
	})
}

// CreateBill 行程结束生成不可变乘客账单。
// 账单只追加：后续分账仅在 bill_transactions 增加流水，不改动账单本身。
func (s *Service) CreateBill(ctx context.Context, cmd dto.CreateBillCommand) (*dto.BillDTO, error) {
	if cmd.OrderNo == "" || cmd.DriverID == "" {
		return nil, fmt.Errorf("%w: order_no and driver_id are required", entity.ErrInvalidSplitRequest)
	}
	if cmd.TotalAmount <= 0 {
		return nil, fmt.Errorf("%w: positive total_amount is required", entity.ErrInvalidSplitRequest)
	}
	if cmd.CommissionRateBP < 0 || cmd.CommissionRateBP > 10000 {
		return nil, fmt.Errorf("%w: commission_rate_bp must be within [0, 10000]", entity.ErrInvalidSplitRequest)
	}
	if existing, err := s.repo.FindBillByOrderNo(ctx, cmd.OrderNo); err == nil {
		return toBillDTO(existing), nil
	} else if !errors.Is(err, entity.ErrBillNotFound) {
		return nil, err
	}

	now := time.Now()
	bill := &entity.Bill{
		ID:               "BILL-" + cmd.OrderNo,
		OrderNo:          cmd.OrderNo,
		UserID:           cmd.UserID,
		DriverID:         cmd.DriverID,
		TotalAmount:      cmd.TotalAmount,
		PayFee:           cmd.TotalAmount,
		DiscountFee:      cmd.DiscountFee,
		PlatformSubsidy:  cmd.PlatformSubsidy,
		CommissionRateBP: cmd.CommissionRateBP,
		Status:           entity.BillStatusUnpaid,
		Remark:           cmd.Remark,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.repo.SaveBill(ctx, bill); err != nil {
		return nil, err
	}
	s.log.Info("bill created", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo), zap.Int64("total_amount", bill.TotalAmount))
	return toBillDTO(bill), nil
}

// FindBillByNo 按账单号查询账单。
func (s *Service) FindBillByNo(ctx context.Context, billNo string) (*dto.BillDTO, error) {
	if billNo == "" {
		return nil, fmt.Errorf("%w: bill_no is required", entity.ErrInvalidSplitRequest)
	}
	bill, err := s.repo.FindBillByNo(ctx, billNo)
	if err != nil {
		return nil, err
	}
	return toBillDTO(bill), nil
}

// SplitBill 分账：按佣金比例拆分司机收入与平台佣金，幂等可重试。
//
// 验收等式：乘客实付 = 平台佣金 + 司机收入 + 平台补贴。
// 说明：乘客实付金额包含用户侧补贴，平台另按订单补贴司机；等式把平台补贴单独
// 记账为 platform_subsidy 流水，因此分账流水合计恒等于乘客实付 + 平台补贴。
// 实现：
//  1. 幂等锚点：分账流水号 tx_no 由 bill_no + category 派生，重复调用命中已有流水直接返回；
//  2. 佣金 = floor(实付 * 万分比)，剩余全给司机，保证 实付 - 佣金 - 司机收入 = 补贴差额 恒为 0 的尾差闭合；
//  3. 司机钱包只追加更新余额与累计入账，不修改历史流水。
func (s *Service) SplitBill(ctx context.Context, cmd dto.SplitBillCommand) (*dto.SplitResult, error) {
	if cmd.OrderNo == "" {
		return nil, fmt.Errorf("%w: order_no is required", entity.ErrInvalidSplitRequest)
	}
	bill, err := s.repo.FindBillByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	// 费率取值：自动分账按建账时锁定的值，手动分账沿用命令传入的值。
	// 两者区间相同，只是来源不同——不锁定的话规则变更会让分账漂移。
	if cmd.UseBillRate {
		cmd.CommissionRate = bill.CommissionRateBP
	}
	if cmd.CommissionRate < 0 || cmd.CommissionRate > 10000 {
		return nil, fmt.Errorf("%w: commission_rate must be in [0,10000]", entity.ErrInvalidSplitRequest)
	}
	// 账单状态机前置校验：只有已支付账单才可分账（资金安全收紧）。
	// 原实现允许 UNPAID 直接分账，存在"未收款先分账"的资金风险；
	// 现在分账唯一入口是支付成功事件链路 MarkBillPaid → SplitBill，
	// 手动/异常路径必须先 MarkBillPaid 置为 PAID 才能分账。
	switch bill.Status {
	case entity.BillStatusPaid:
		// 可继续
	case entity.BillStatusCalculating, entity.BillStatusUnpaid, entity.BillStatusVoid:
		return nil, fmt.Errorf("%w: bill status %s cannot be split, bill must be paid", entity.ErrInvalidSplitRequest, bill.Status)
	default:
		return nil, fmt.Errorf("%w: unknown bill status %s", entity.ErrInvalidSplitRequest, bill.Status)
	}

	passengerPaid := bill.TotalAmount
	platformSubsidy := bill.PlatformSubsidy
	// P9：透传平台补贴，命令值大于 0 时以订单侧权威值覆盖账单补贴。
	if cmd.PlatformSubsidy > 0 {
		platformSubsidy = cmd.PlatformSubsidy
	}
	// P9：幂等键透传仅用于兼容记录；分账幂等锚点仍以账单号派生 tx_no 为准。
	if cmd.IdempotentKey != "" {
		s.log.Info("split bill idempotent key provided", zap.String("order_no", cmd.OrderNo), zap.String("idempotent_key", cmd.IdempotentKey))
	}
	// 分账等式（验收）：乘客实付 P = 平台佣金 C + 司机收入 D + 平台补贴 S。
	// C = floor(P * rate / 10000)；D = P - C - S（尾差全归司机）；
	// 平台补贴 S 单独记为 platform_subsidy 流水，且补贴随司机收入一并入司机钱包，
	// 因此司机钱包实际入账 = D + S，司机侧到账与流水一致、等式严格闭合。
	platformCommission := passengerPaid * cmd.CommissionRate / 10000
	driverIncome := passengerPaid - platformCommission - platformSubsidy
	if driverIncome < 0 {
		return nil, fmt.Errorf("%w: subsidy exceeds payable after commission, passenger_paid=%d commission=%d subsidy=%d",
			entity.ErrInvalidSplitRequest, passengerPaid, platformCommission, platformSubsidy)
	}
	driverSettlementIncome := driverIncome + platformSubsidy

	now := time.Now()
	txs := []*entity.BillTransaction{
		{
			ID:        txNo(bill.ID, entity.TxCategoryPlatformCommission),
			BillNo:    bill.ID,
			OrderNo:   bill.OrderNo,
			Category:  entity.TxCategoryPlatformCommission,
			OwnerType: entity.OwnerTypePlatform,
			OwnerID:   "platform",
			Amount:    platformCommission,
			CreatedAt: now,
		},
		{
			ID:        txNo(bill.ID, entity.TxCategoryDriverIncome),
			BillNo:    bill.ID,
			OrderNo:   bill.OrderNo,
			Category:  entity.TxCategoryDriverIncome,
			OwnerType: entity.OwnerTypeDriver,
			OwnerID:   bill.DriverID,
			Amount:    driverIncome,
			CreatedAt: now,
		},
	}
	if platformSubsidy > 0 {
		txs = append(txs, &entity.BillTransaction{
			ID:        txNo(bill.ID, entity.TxCategoryPlatformSubsidy),
			BillNo:    bill.ID,
			OrderNo:   bill.OrderNo,
			Category:  entity.TxCategoryPlatformSubsidy,
			OwnerType: entity.OwnerTypePlatform,
			OwnerID:   "platform",
			Amount:    platformSubsidy,
			CreatedAt: now,
		})
	}

	// 幂等：任一流水已存在说明该订单已分账，直接返回历史结果，不重复入账。
	for _, tx := range txs {
		existing, err := s.repo.FindTransactionByTxNo(ctx, tx.ID)
		if err != nil && !errors.Is(err, entity.ErrTransactionNotFound) {
			return nil, err
		}
		if existing != nil {
			return s.buildRepeatedSplit(bill), nil
		}
	}

	// 追加流水 + 司机入账 + 账单置已结算：同一事务提交（P1：SplitBill 原子性）。
	err = s.withTransaction(ctx, func(tctx context.Context, txRepo entity.Repository) error {
		for _, tx := range txs {
			if err := txRepo.AppendTransaction(tctx, tx); err != nil {
				return err
			}
		}
		// 司机收入入钱包：司机收入 D 与平台补贴 S 一并入账，到账 = D + S。
		// 钱包更新走乐观锁版本化（P1：条件更新防并发覆盖）。
		wallet, walletErr := txRepo.GetWallet(tctx, bill.DriverID)
		switch {
		case walletErr == nil:
			if err := txRepo.UpdateWalletBalance(tctx, bill.DriverID, driverSettlementIncome, driverSettlementIncome, wallet.Version); err != nil {
				return err
			}
		case errors.Is(walletErr, entity.ErrWalletNotFound):
			// 首次分账：新建钱包（版本从 0 起，落库即 1），幂等锚点已挡并发重复分账。
			wallet = &entity.Wallet{
				ID:          bill.DriverID,
				DriverID:    bill.DriverID,
				Balance:     driverSettlementIncome,
				TotalIncome: driverSettlementIncome,
				Version:     1,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := txRepo.UpsertWallet(tctx, wallet); err != nil {
				return err
			}
		default:
			return walletErr
		}

		bill.Status = entity.BillStatusPaid
		bill.DriverIncome = driverIncome
		bill.CommissionFee = platformCommission
		bill.SubsidyFee = platformSubsidy
		bill.Settled = true
		bill.UpdatedAt = now
		return txRepo.SaveBill(tctx, bill)
	})
	if err != nil {
		return nil, err
	}

	result := &dto.SplitResult{
		BillNo:             bill.ID,
		OrderNo:            bill.OrderNo,
		PassengerPaid:      passengerPaid,
		PlatformCommission: platformCommission,
		DriverIncome:       driverIncome,
		PlatformSubsidy:    platformSubsidy,
		Repeated:           false,
	}
	s.log.Info("bill split", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo),
		zap.Int64("commission", platformCommission), zap.Int64("driver_income", driverIncome), zap.Int64("subsidy", platformSubsidy))
	return result, nil
}

// ApplyRefund 退款冲正（P1：退款幂等 + 三账冲正）。
//
// 三账指：bill.RefundFee（账单已退金额）、bill_transactions 冲正流水、司机钱包余额。
// 规则：
//  1. 幂等锚点：冲正流水号 = BILL-{order_no}-refund_reversal，重复调用直接返回历史结果；
//  2. 冲正上限：退款金额不得超过司机已结算入账（DriverIncome + SubsidyFee）；
//  3. 已结算（Settled）账单：流水 + 钱包扣减 + 账单 RefundFee 在同一事务内提交，
//     钱包扣减走乐观锁版本化条件更新，冲突则整单回滚重试；
//  4. 未结算账单：只记冲正流水与 RefundFee，不动钱包。
func (s *Service) ApplyRefund(ctx context.Context, cmd dto.ApplyRefundCommand) (*dto.RefundSettlementResult, error) {
	if cmd.OrderNo == "" || cmd.RefundNo == "" {
		return nil, fmt.Errorf("%w: order_no and refund_no are required", entity.ErrInvalidSplitRequest)
	}
	if cmd.RefundAmount <= 0 {
		return nil, fmt.Errorf("%w: positive refund_amount is required", entity.ErrInvalidSplitRequest)
	}
	bill, err := s.repo.FindBillByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return nil, err
	}

	reversalTxID := txNo(bill.ID, entity.TxCategoryRefundReversal)
	existing, err := s.repo.FindTransactionByTxNo(ctx, reversalTxID)
	if err != nil && !errors.Is(err, entity.ErrTransactionNotFound) {
		return nil, err
	}
	if existing != nil {
		// 幂等命中：返回历史冲正结果。
		s.log.Info("refund settlement repeated", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo),
			zap.Int64("amount", existing.Amount))
		// 冲正流水以负数存储（扣减语义），对外统一暴露为正的冲正金额，
		// 与首次冲正的返回口径一致——否则调用方会拿到 -3000 这种反向值。
		return &dto.RefundSettlementResult{
			BillNo:         bill.ID,
			OrderNo:        bill.OrderNo,
			ReversedAmount: -existing.Amount,
			Repeated:       true,
		}, nil
	}

	// 冲正上限：司机已结算入账（司机收入 + 平台补贴）。
	settledIncome := bill.DriverIncome + bill.SubsidyFee
	if cmd.RefundAmount > settledIncome {
		return nil, fmt.Errorf("%w: refund=%d settled_income=%d",
			entity.ErrRefundSettlementExceeds, cmd.RefundAmount, settledIncome)
	}

	now := time.Now()
	reversal := &entity.BillTransaction{
		ID:        reversalTxID,
		BillNo:    bill.ID,
		OrderNo:   bill.OrderNo,
		Category:  entity.TxCategoryRefundReversal,
		OwnerType: entity.OwnerTypeDriver,
		OwnerID:   bill.DriverID,
		Amount:    -cmd.RefundAmount,
		CreatedAt: now,
	}

	if bill.Settled {
		// 已结算：流水 + 钱包扣减 + RefundFee 同一事务提交。
		err = s.withTransaction(ctx, func(tctx context.Context, txRepo entity.Repository) error {
			if err := txRepo.AppendTransaction(tctx, reversal); err != nil {
				return err
			}
			wallet, walletErr := txRepo.GetWallet(tctx, bill.DriverID)
			if walletErr != nil {
				return walletErr
			}
			if err := txRepo.UpdateWalletBalance(tctx, bill.DriverID, -cmd.RefundAmount, -cmd.RefundAmount, wallet.Version); err != nil {
				return err
			}
			bill.RefundFee += cmd.RefundAmount
			bill.UpdatedAt = now
			return txRepo.SaveBill(tctx, bill)
		})
	} else {
		// 未结算：只记流水与账单 RefundFee。
		if err := s.repo.AppendTransaction(ctx, reversal); err != nil {
			return nil, err
		}
		bill.RefundFee += cmd.RefundAmount
		bill.UpdatedAt = now
		if err := s.repo.SaveBill(ctx, bill); err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}

	s.log.Info("refund settlement applied", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo),
		zap.String("refund_no", cmd.RefundNo), zap.Int64("amount", cmd.RefundAmount))
	return &dto.RefundSettlementResult{
		BillNo:         bill.ID,
		OrderNo:        bill.OrderNo,
		ReversedAmount: cmd.RefundAmount,
	}, nil
}

// GetWalletBalance 查询司机钱包，钱包不存在时返回零值视图。
func (s *Service) GetWalletBalance(ctx context.Context, driverID string) (*dto.WalletDTO, error) {
	if driverID == "" {
		return nil, fmt.Errorf("%w: driver_id is required", entity.ErrInvalidSplitRequest)
	}
	wallet, err := s.repo.GetWallet(ctx, driverID)
	if errors.Is(err, entity.ErrWalletNotFound) {
		return &dto.WalletDTO{DriverID: driverID}, nil
	}
	if err != nil {
		return nil, err
	}
	return &dto.WalletDTO{
		DriverID:      wallet.DriverID,
		Balance:       wallet.Balance,
		FrozenBalance: wallet.FrozenBalance,
		TotalIncome:   wallet.TotalIncome,
		TotalWithdraw: wallet.TotalWithdraw,
	}, nil
}

// MarkBillPaid 支付成功回调：UNPAID → PAID，幂等可重试。
// 已 PAID 时直接返回当前账单；CALCULATING/VOID 拒绝流转。
func (s *Service) MarkBillPaid(ctx context.Context, cmd dto.MarkBillPaidCommand) (*dto.BillDTO, error) {
	bill, err := s.findBill(ctx, cmd.BillNo, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if err := bill.MarkPaid(time.Now()); err != nil {
		return nil, err
	}
	if cmd.PayFee > 0 {
		bill.PayFee = cmd.PayFee
		bill.TotalAmount = cmd.PayFee
	}
	if cmd.DiscountFee >= 0 {
		bill.DiscountFee = cmd.DiscountFee
	}
	if cmd.RefundFee > 0 {
		bill.RefundFee = cmd.RefundFee
	}
	if err := s.repo.SaveBill(ctx, bill); err != nil {
		return nil, err
	}
	s.log.Info("bill marked paid", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo))
	return toBillDTO(bill), nil
}

// VoidBill 作废账单：CALCULATING/UNPAID → VOID，幂等可重试。
func (s *Service) VoidBill(ctx context.Context, cmd dto.VoidBillCommand) (*dto.BillDTO, error) {
	bill, err := s.findBill(ctx, cmd.BillNo, cmd.OrderNo)
	if err != nil {
		return nil, err
	}
	if err := bill.MarkVoid(time.Now()); err != nil {
		return nil, err
	}
	if cmd.Reason != "" {
		bill.Remark = cmd.Reason
	}
	if err := s.repo.SaveBill(ctx, bill); err != nil {
		return nil, err
	}
	s.log.Info("bill voided", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo))
	return toBillDTO(bill), nil
}

// ListUnpaidBillsByUser 查询用户全部待支付账单。
func (s *Service) ListUnpaidBillsByUser(ctx context.Context, query dto.ListUnpaidBillsQuery) ([]*dto.BillDTO, error) {
	if query.UserID == "" {
		return nil, fmt.Errorf("%w: user_id is required", entity.ErrInvalidSplitRequest)
	}
	bills, err := s.repo.ListUnpaidBillsByUser(ctx, query.UserID)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.BillDTO, 0, len(bills))
	for _, bill := range bills {
		out = append(out, toBillDTO(bill))
	}
	return out, nil
}

// CheckUnpaidBan 未付禁叫车校验：存在待支付账单则禁止发起新行程。
func (s *Service) CheckUnpaidBan(ctx context.Context, query dto.ListUnpaidBillsQuery) (*dto.UnpaidBanDTO, error) {
	bills, err := s.ListUnpaidBillsByUser(ctx, query)
	if err != nil {
		return nil, err
	}
	result := &dto.UnpaidBanDTO{
		UnpaidCount: len(bills),
	}
	for _, bill := range bills {
		result.UnpaidAmountSum += bill.PayFee - bill.RefundFee
	}
	result.Banned = result.UnpaidCount > 0
	return result, nil
}

// ReconcilePendingSettlements 掉单补偿：扫描已支付但未结算的账单并补分账，返回本批补偿成功的单数。
//
// 补偿的是支付链路 MarkBillPaid 成功但 SplitBill 失败/未执行的账单——
// 该链路是 fire-and-forget，失败只记日志，没有它钱就会永远滞留在账单里。
// 每单按建账锁定的费率分账（UseBillRate），与在线链路口径一致。
//
// 单笔失败不中断整批：跳过并记日志，下一轮继续扫。分账本身幂等，重复补偿不会重复入账。
func (s *Service) ReconcilePendingSettlements(ctx context.Context, limit int64) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("%w: positive limit is required", entity.ErrInvalidSplitRequest)
	}
	bills, err := s.repo.ListPendingSettleBills(ctx, limit)
	if err != nil {
		return 0, err
	}
	compensated := 0
	for _, bill := range bills {
		if _, err := s.SplitBill(ctx, dto.SplitBillCommand{
			OrderNo:     bill.OrderNo,
			UseBillRate: true,
		}); err != nil {
			// 单笔失败不阻断整批：记录后继续，下一轮扫描会再带到它。
			s.log.Warn("reconcile split bill failed", zap.String("bill_no", bill.ID),
				zap.String("order_no", bill.OrderNo), zap.Error(err))
			continue
		}
		compensated++
		s.log.Info("reconcile split bill compensated", zap.String("bill_no", bill.ID), zap.String("order_no", bill.OrderNo))
	}
	return compensated, nil
}

// findBill 按账单号或订单号查询账单，二者必传其一。
func (s *Service) findBill(ctx context.Context, billNo string, orderNo string) (*entity.Bill, error) {
	switch {
	case billNo != "":
		return s.repo.FindBillByNo(ctx, billNo)
	case orderNo != "":
		return s.repo.FindBillByOrderNo(ctx, orderNo)
	default:
		return nil, fmt.Errorf("%w: bill_no or order_no is required", entity.ErrInvalidSplitRequest)
	}
}

// buildRepeatedSplit 构造重复分账返回值。
func (s *Service) buildRepeatedSplit(bill *entity.Bill) *dto.SplitResult {
	return &dto.SplitResult{
		BillNo:   bill.ID,
		OrderNo:  bill.OrderNo,
		Status:   bill.Status,
		Repeated: true,
	}
}

func txNo(billNo string, category string) string {
	return billNo + "-" + category
}

func toBillDTO(bill *entity.Bill) *dto.BillDTO {
	return &dto.BillDTO{
		ID:              bill.ID,
		OrderNo:         bill.OrderNo,
		UserID:          bill.UserID,
		DriverID:        bill.DriverID,
		TotalAmount:     bill.TotalAmount,
		PayFee:          bill.PayFee,
		DiscountFee:     bill.DiscountFee,
		RefundFee:       bill.RefundFee,
		DriverIncome:    bill.DriverIncome,
		CommissionFee:   bill.CommissionFee,
		SubsidyFee:      bill.SubsidyFee,
		Settled:         bill.Settled,
		PlatformSubsidy: bill.PlatformSubsidy,
		Status:          bill.Status,
		Remark:          bill.Remark,
		CreatedAt:       bill.CreatedAt,
		UpdatedAt:       bill.UpdatedAt,
	}
}
