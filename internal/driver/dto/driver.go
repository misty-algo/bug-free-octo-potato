package dto

import (
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
)

// DriverProfileDTO 司机档案出参。
type DriverProfileDTO struct {
	ID    string
	DriverNo string
	Name  string
	Phone string
	Status   string
	Mode     string
	ModeVersion int64
	AreaLimitSwitch bool
	Areas    []entity.AreaBox
	CityCode string
	CarType  string
	Seats    int
	Qualified  bool
	CanAccept  bool
	OfflineAfterTrip bool
	LastHeartbeatAt time.Time
	ActiveTaskID    string
	MonthlyIncomeFen int64
	Rating      float64
	SuccessRate float64
	Funds FundsDTO
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FundsDTO 司机资金五桶（单位：分）。
type FundsDTO struct {
	PendingFen   int64
	AvailableFen int64
	FrozenFen    int64
	WithdrawFen  int64
	DebtFen      int64
}

// FromDriver 实体转 DTO。
func FromDriver(d *entity.Driver) *DriverProfileDTO {
	if d.Areas == nil {
		d.Areas = []entity.AreaBox{}
	}
	return &DriverProfileDTO{
		ID: d.ID, DriverNo: d.DriverNo, Name: d.Name, Phone: d.Phone,
		Status: d.Status, Mode: d.Mode, ModeVersion: d.ModeVersion,
		AreaLimitSwitch: d.AreaLimitSwitch, Areas: d.Areas,
		CityCode: d.CityCode, CarType: d.CarType, Seats: d.Seats,
		Qualified: d.Qualified, CanAccept: d.CanAccept,
		OfflineAfterTrip: d.OfflineAfterTrip,
		LastHeartbeatAt:  d.LastHeartbeatAt, ActiveTaskID: d.ActiveTaskID,
		MonthlyIncomeFen: d.MonthlyIncomeFen, Rating: d.Rating, SuccessRate: d.SuccessRate,
		Funds: FundsDTO{
			PendingFen: d.Funds.Pending, AvailableFen: d.Funds.Available,
			FrozenFen: d.Funds.Frozen, WithdrawFen: d.Funds.Withdraw, DebtFen: d.Funds.Debt,
		},
		Version: d.Version, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

// HallOrderDTO 大厅订单出参。
type HallOrderDTO struct {
	ID   string
	TaskID string
	OrderID string
	PassengerName string
	PassengerCount int
	CityCode      string
	OrderType     string
	RequireSeats  int
	RequireCarType string
	Origin        entity.Location
	Destination   entity.Location
	OrigFareFen       int64
	PassengerPaidFen  int64
	PlatformCouponFen int64
	DriverCouponFen   int64
	CommissionFen     int64
	SubsidyFen        int64
	DriverIncomeFen   int64
	Status    string
	DriverID  string
	DriverName string
	DispatchChannel string
	VisibleInPool   bool
	PickupDistanceMeters int64
	InviteRound      int64
	AcceptDeadline   time.Time
	ListenDeadline   time.Time
	DispatchDeadline time.Time
	AssignedAt  time.Time
	AcceptedAt  time.Time
	StartedAt   time.Time
	EndedAt     time.Time
	CancelledAt time.Time
	DurationSec    int64
	DistanceMeters int64
	CancelReason     string
	ResponsibleParty string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FromHallOrder 实体转 DTO。
func FromHallOrder(o *entity.HallOrder) *HallOrderDTO {
	return &HallOrderDTO{
		ID: o.ID, TaskID: o.TaskID, OrderID: o.OrderID,
		PassengerName: o.PassengerName, PassengerCount: o.PassengerCount,
		CityCode: o.CityCode, OrderType: o.OrderType,
		RequireSeats: o.RequireSeats, RequireCarType: o.RequireCarType,
		Origin: o.Origin, Destination: o.Destination,
		OrigFareFen: o.OrigFareFen, PassengerPaidFen: o.PassengerPaidFen,
		PlatformCouponFen: o.PlatformCouponFen, DriverCouponFen: o.DriverCouponFen,
		CommissionFen: o.CommissionFen, SubsidyFen: o.SubsidyFen,
		DriverIncomeFen: o.DriverIncomeFen,
		Status: o.Status, DriverID: o.DriverID, DriverName: o.DriverName,
		DispatchChannel: o.DispatchChannel, VisibleInPool: o.VisibleInPool,
		PickupDistanceMeters: o.PickupDistanceMeters, InviteRound: o.InviteRound,
		AcceptDeadline: o.AcceptDeadline, ListenDeadline: o.ListenDeadline,
		DispatchDeadline: o.DispatchDeadline,
		AssignedAt: o.AssignedAt, AcceptedAt: o.AcceptedAt, StartedAt: o.StartedAt,
		EndedAt: o.EndedAt, CancelledAt: o.CancelledAt,
		DurationSec: o.DurationSec, DistanceMeters: o.DistanceMeters,
		CancelReason: o.CancelReason, ResponsibleParty: o.ResponsibleParty,
		Version: o.Version, CreatedAt: o.CreatedAt, UpdatedAt: o.UpdatedAt,
	}
}

// IncomeFlowDTO 收入流水出参（含反向流水）。
type IncomeFlowDTO struct {
	ID   string
	DriverID string
	OrderID  string
	TaskID   string
	FlowType string
	ReverseOf string
	OrigFareFen       int64
	PassengerPaidFen  int64
	PlatformCouponFen int64
	DriverCouponFen   int64
	CommissionFen     int64
	SubsidyFen        int64
	DriverIncomeFen   int64
	AccountingDate string
	BucketFrom     string
	BucketTo       string
	EventID        string
	IdempotencyKey string
	Reason         string
	CreatedAt      time.Time
}

// FromIncomeFlow 实体转 DTO。
func FromIncomeFlow(f *entity.IncomeFlow) *IncomeFlowDTO {
	return &IncomeFlowDTO{
		ID: f.ID, DriverID: f.DriverID, OrderID: f.OrderID, TaskID: f.TaskID,
		FlowType: f.FlowType, ReverseOf: f.ReverseOf,
		OrigFareFen: f.OrigFareFen, PassengerPaidFen: f.PassengerPaidFen,
		PlatformCouponFen: f.PlatformCouponFen, DriverCouponFen: f.DriverCouponFen,
		CommissionFen: f.CommissionFen, SubsidyFen: f.SubsidyFen,
		DriverIncomeFen: f.DriverIncomeFen,
		AccountingDate: f.AccountingDate, BucketFrom: f.BucketFrom, BucketTo: f.BucketTo,
		EventID: f.EventID, IdempotencyKey: f.IdempotencyKey, Reason: f.Reason,
		CreatedAt: f.CreatedAt,
	}
}

// ApprovalDTO 提现审批留痕。
type ApprovalDTO struct {
	ApproverID string
	Comment    string
	ApprovedAt time.Time
}

// WithdrawDTO 提现记录出参。
type WithdrawDTO struct {
	ID        string
	DriverID  string
	AmountFen int64
	Status    string
	DualApproval bool
	Approvals []ApprovalDTO
	IdempotencyKey string
	Reason    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FromWithdrawal 实体转 DTO。
func FromWithdrawal(w *entity.Withdrawal) *WithdrawDTO {
	approvals := make([]ApprovalDTO, 0, len(w.Approvals))
	for _, a := range w.Approvals {
		approvals = append(approvals, ApprovalDTO{ApproverID: a.ApproverID, Comment: a.Comment, ApprovedAt: a.ApprovedAt})
	}
	return &WithdrawDTO{
		ID: w.ID, DriverID: w.DriverID, AmountFen: w.AmountFen, Status: w.Status,
		DualApproval: w.DualApproval, Approvals: approvals,
		IdempotencyKey: w.IdempotencyKey, Reason: w.Reason,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
}

// WithdrawResultDTO 提现申请结果（幂等重复请求返回同一单据）。
type WithdrawResultDTO struct {
	Withdrawal *WithdrawDTO
	Duplicate  bool
}

// IncomeSummaryDTO 收入概览（五桶 + 提现规则 + 当日统计）。
type IncomeSummaryDTO struct {
	DriverID         string
	PendingFen       int64
	AvailableFen     int64
	FrozenFen        int64
	WithdrawFen      int64
	DebtFen          int64
	MonthlyIncomeFen int64
	MinWithdrawFen   int64
	MaxWithdrawFen   int64
	DailyLimitFen    int64
	DailyMaxCount    int
	DualApprovalFen  int64
	WithdrawnTodayFen int64
	WithdrawCountToday int64
	UpdatedAt        string
}

// DayIncomeDTO 单日收益。
type DayIncomeDTO struct {
	Date       string
	OrderCount int64
	IncomeFen  int64
	RefundFen  int64
}

// IncomeCompareDTO 首页收益对比出参。
type IncomeCompareDTO struct {
	Today          DayIncomeDTO
	Yesterday      DayIncomeDTO
	OrderDiff      int64
	IncomeDiffFen  int64
	PercentText    string
	Summary        string
	PendingIncomeFen int64
	LastUpdatedAt  string
}

// FromIncomeCompare 实体转 DTO。
func FromIncomeCompare(c *entity.IncomeCompare) *IncomeCompareDTO {
	return &IncomeCompareDTO{
		Today: DayIncomeDTO{
			Date: c.Today.Date, OrderCount: c.Today.OrderCount,
			IncomeFen: c.Today.IncomeFen, RefundFen: c.Today.RefundFen,
		},
		Yesterday: DayIncomeDTO{
			Date: c.Yesterday.Date, OrderCount: c.Yesterday.OrderCount,
			IncomeFen: c.Yesterday.IncomeFen, RefundFen: c.Yesterday.RefundFen,
		},
		OrderDiff: c.OrderDiff, IncomeDiffFen: c.IncomeDiffFen,
		PercentText: c.PercentText, Summary: c.Summary,
		PendingIncomeFen: c.PendingIncomeFen, LastUpdatedAt: c.LastUpdatedAt,
	}
}
