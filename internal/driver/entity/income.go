package entity

import (
	"errors"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
)

// 收入流水类型：正向收益记录与反向（退款/处罚）记录。
// 规则：凭证不可变，退款与处罚一律追加反向流水，绝不修改原凭证金额。
const (
	FlowTypeIncome     = "income"
	FlowTypeSubsidy    = "subsidy"
	FlowTypeRefund     = "refund"
	FlowTypePunishment = "punishment"
)

// 提现申请状态流转：待审核 → 已核准(待打款) → 已成功 / 已驳回。
const (
	WithdrawStatusReviewing = "reviewing"
	WithdrawStatusApproved  = "approved"
	WithdrawStatusPaying    = "paying"
	WithdrawStatusSuccess   = "success"
	WithdrawStatusRejected  = "rejected"
)

var (
	ErrInvalidFlow            = errors.New("invalid income flow")
	ErrInvalidWithdraw        = errors.New("invalid withdraw request")
	ErrBalanceInsufficient    = errors.New("balance below withdraw amount")
	ErrBelowMinWithdraw       = errors.New("amount below minimum withdraw threshold")
	ErrAboveMaxWithdraw       = errors.New("amount above single withdraw limit")
	ErrDailyLimitExceeded     = errors.New("daily withdraw limit exceeded")
	ErrDailyCountExceeded     = errors.New("daily withdraw count exceeded")
	ErrWithdrawNotFound       = errors.New("withdraw request not found")
	ErrWithdrawStateDenied    = errors.New("withdraw state transition not allowed")
	ErrDualApprovalRequired   = errors.New("dual approval required before payout")
	ErrDuplicateApprover      = errors.New("same approver cannot approve twice")
	ErrDuplicateFlowEvent     = errors.New("income flow event already recorded")
)

// IncomeFlow 司机收入流水凭证（只追加，不修改）。
type IncomeFlow struct {
	ID   string
	DriverID string
	OrderID  string
	TaskID   string
	FlowType string // income / subsidy / refund / punishment
	ReverseOf string // 反向流水的原凭证 ID

	// 分账明细：一律整数分
	OrigFareFen       int64
	PassengerPaidFen  int64
	PlatformCouponFen int64
	DriverCouponFen   int64
	CommissionFen     int64
	SubsidyFen        int64
	DriverIncomeFen   int64 // 反向流水为负数

	AccountingDate string // 记账日（业务时区 yyyy-mm-dd）
	BucketFrom     string // 来源桶（空表示外部入账）
	BucketTo       string // 落账桶：income → U（待结算）
	EventID        string // 事件去重键
	IdempotencyKey string // 命令幂等键
	Reason         string
	CreatedAt      time.Time
}

// NewIncomeFlow 生成一条正向收益凭证，金额在创建时锁定。
func NewIncomeFlow(driverID, orderID, taskID, eventID, idempotencyKey string, amount contractAmount, accountingDate string) (*IncomeFlow, error) {
	if driverID == "" || orderID == "" || amount.DriverIncomeFen == 0 {
		return nil, ErrInvalidFlow
	}
	now := biztime.UTC()
	date := strings.TrimSpace(accountingDate)
	if date == "" {
		date = biztime.DateKey(now)
	}
	return &IncomeFlow{
		ID:                newID(),
		DriverID:          driverID,
		OrderID:           orderID,
		TaskID:            taskID,
		FlowType:          FlowTypeIncome,
		OrigFareFen:       amount.OrigFareFen,
		PassengerPaidFen:  amount.PassengerPaidFen,
		PlatformCouponFen: amount.PlatformCouponFen,
		DriverCouponFen:   amount.DriverCouponFen,
		CommissionFen:     amount.CommissionFen,
		SubsidyFen:        amount.SubsidyFen,
		DriverIncomeFen:   amount.DriverIncomeFen,
		AccountingDate:    date,
		BucketTo:          BucketPending,
		EventID:           eventID,
		IdempotencyKey:    idempotencyKey,
		CreatedAt:         now,
	}, nil
}

// NewReverseFlow 生成反向流水（退款/处罚），金额为负。
func NewReverseFlow(origin *IncomeFlow, flowType, eventID, idempotencyKey, accountingDate string, amountFen int64, reason string) (*IncomeFlow, error) {
	if origin == nil || amountFen <= 0 {
		return nil, ErrInvalidFlow
	}
	if flowType != FlowTypeRefund && flowType != FlowTypePunishment {
		return nil, ErrInvalidFlow
	}
	now := biztime.UTC()
	date := strings.TrimSpace(accountingDate)
	if date == "" {
		date = biztime.DateKey(now)
	}
	return &IncomeFlow{
		ID:              newID(),
		DriverID:        origin.DriverID,
		OrderID:         origin.OrderID,
		TaskID:          origin.TaskID,
		FlowType:        flowType,
		ReverseOf:       origin.ID,
		DriverIncomeFen: -amountFen,
		AccountingDate:  date,
		BucketTo:        BucketDebt,
		EventID:         eventID,
		IdempotencyKey:  idempotencyKey,
		Reason:          reason,
		CreatedAt:       now,
	}, nil
}

// LedgerEntry 资金分录（只追加，借贷平衡）。
type LedgerEntry struct {
	ID             string
	DriverID       string
	FromBucket     string
	ToBucket       string
	AmountFen      int64
	RefType        string
	RefID          string
	IdempotencyKey string
	CreatedAt      time.Time
}

// ApprovalRecord 提现审批留痕（≥500 元需双人审批）。
type ApprovalRecord struct {
	ApproverID string
	Comment    string
	ApprovedAt time.Time
}

// Withdrawal 提现申请单。
type Withdrawal struct {
	ID        string
	DriverID  string
	AmountFen int64
	Status    string
	// DualApproval 金额达到阈值时需要双人审批（由服务按 A11 配置判定）。
	DualApproval bool
	Approvals    []ApprovalRecord
	IdempotencyKey string
	Reason       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewWithdrawal 创建提现申请，初始状态待审核。
func NewWithdrawal(driverID, idempotencyKey string, amountFen int64, dualApproval bool) (*Withdrawal, error) {
	if driverID == "" || amountFen <= 0 {
		return nil, ErrInvalidWithdraw
	}
	now := biztime.UTC()
	return &Withdrawal{
		ID:             newID(),
		DriverID:       driverID,
		AmountFen:      amountFen,
		Status:         WithdrawStatusReviewing,
		DualApproval:   dualApproval,
		Approvals:      []ApprovalRecord{},
		IdempotencyKey: idempotencyKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// Approve 审批通过：单人/双人审批到达阈值后置为已核准。
func (w *Withdrawal) Approve(approverID, comment string) error {
	if approverID == "" {
		return ErrInvalidWithdraw
	}
	if w.Status != WithdrawStatusReviewing {
		return ErrWithdrawStateDenied
	}
	for _, a := range w.Approvals {
		if a.ApproverID == approverID {
			return ErrDuplicateApprover
		}
	}
	w.Approvals = append(w.Approvals, ApprovalRecord{ApproverID: approverID, Comment: comment, ApprovedAt: biztime.UTC()})
	if !w.DualApproval || len(w.Approvals) >= 2 {
		w.Status = WithdrawStatusApproved
	}
	w.UpdatedAt = biztime.UTC()
	return nil
}

// ApprovalsEnough 是否已集齐所需审批。
func (w *Withdrawal) ApprovalsEnough() bool {
	if !w.DualApproval {
		return len(w.Approvals) >= 1
	}
	return len(w.Approvals) >= 2
}

// MarkPaying 进入打款中（外部代付适配器调用）。
func (w *Withdrawal) MarkPaying() error {
	if w.Status != WithdrawStatusApproved {
		return ErrWithdrawStateDenied
	}
	w.Status = WithdrawStatusPaying
	w.UpdatedAt = biztime.UTC()
	return nil
}

// MarkPaid 打款成功：预占资金正式出账。
func (w *Withdrawal) MarkPaid() error {
	if w.Status != WithdrawStatusApproved && w.Status != WithdrawStatusPaying {
		return ErrWithdrawStateDenied
	}
	w.Status = WithdrawStatusSuccess
	w.UpdatedAt = biztime.UTC()
	return nil
}

// RollbackPaid 出账失败时把已置 success 的单据回滚到已核准：
// 状态与资金必须一致，不能出现"单据已到账但 W 桶没扣"的中间态。
func (w *Withdrawal) RollbackPaid() {
	if w.Status == WithdrawStatusSuccess {
		w.Status = WithdrawStatusApproved
		w.UpdatedAt = biztime.UTC()
	}
}

// MarkRejected 驳回：预占额度需返还可用桶（由服务层执行返还）。
func (w *Withdrawal) MarkRejected(reason string) error {
	if w.Status == WithdrawStatusSuccess || w.Status == WithdrawStatusRejected {
		return ErrWithdrawStateDenied
	}
	w.Status = WithdrawStatusRejected
	w.Reason = reason
	w.UpdatedAt = biztime.UTC()
	return nil
}

// DayIncome 单日收益统计。
// 口径说明：接单量按行程结束日统计且拼车按乘客数计，收入按记账日统计，两者不同步属正常。
type DayIncome struct {
	Date         string `json:"date"`
	OrderCount   int64  `json:"order_count"`
	IncomeFen    int64  `json:"income_fen"`
	RefundFen    int64  `json:"refund_fen"`
}

// IncomeCompare 首页收益对比结果（D09–D11）。
type IncomeCompare struct {
	Today          DayIncome `json:"today"`
	Yesterday      DayIncome `json:"yesterday"`
	OrderDiff      int64     `json:"order_diff"`
	IncomeDiffFen  int64     `json:"income_diff_fen"`
	PercentText    string    `json:"percent_text"` // 昨日为 0 时不展示百分比
	Summary        string    `json:"summary"`
	PendingIncomeFen int64   `json:"pending_income_fen"` // 待入账（U 桶）预估
	LastUpdatedAt  string    `json:"last_updated_at"`
}

// BuildSummary 生成差额文案：多/少/持平，昨日为 0 元不显示百分比。
func (c *IncomeCompare) BuildSummary() {
	diff := c.IncomeDiffFen
	switch {
	case diff > 0:
		c.Summary = fmtFen("今天比昨天多挣 %s 元", diff)
	case diff < 0:
		c.Summary = fmtFen("今天比昨天少挣 %s 元", -diff)
	default:
		c.Summary = "今天与昨天收益持平"
	}
	if c.Yesterday.IncomeFen <= 0 {
		c.PercentText = "-"
		return
	}
	pct := float64(diff) / float64(c.Yesterday.IncomeFen) * 100
	c.PercentText = trimF(pct) + "%"
}

func trimF(v float64) string {
	s := trimRightZeros(v)
	return s
}
