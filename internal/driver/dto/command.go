package dto

import (
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// ── 司机账号命令 ──

// SendDriverCodeCommand 下发司机登录验证码。
type SendDriverCodeCommand struct {
	Phone string
}

// DriverLoginCommand 司机登录（手机号 + 验证码）。
type DriverLoginCommand struct {
	Phone string
	Code  string
	// Name 仅首次登录自动建档时需要。
	Name string
}

// ── 司机档案命令 ──

// RegisterDriverCommand 注册司机。
type RegisterDriverCommand struct {
	Name  string
	Phone string
}

// UpdateStatusCommand 收车 / 出车。
type UpdateStatusCommand struct {
	DriverID string
	Status   string
	Version  int64
}

// UpdateModeCommand 听单 / 抢单切换。
type UpdateModeCommand struct {
	DriverID string
	Mode     string
	Version  int64
}

// UpdateAreaCommand 接单区域设置。
type UpdateAreaCommand struct {
	DriverID   string
	SwitchOn   bool
	Areas      []entity.AreaBox
	Version    int64
}

// HeartbeatCommand 司机心跳上报（空闲连续 60 秒无心跳自动收车）。
type HeartbeatCommand struct {
	DriverID string
	Lat      float64
	Lng      float64
}

// OfflineAfterTripCommand 设置"本单后收车"。
type OfflineAfterTripCommand struct {
	DriverID string
	On       bool
	Version  int64
}

// ── 接单大厅命令 ──

// CreateTaskCommand 创建派单任务（乘客域下单事件 / 模拟下单都走这里）。
type CreateTaskCommand struct {
	OrderID        string
	PassengerName  string
	PassengerCount int
	CityCode       string
	OrderType      string
	RequireSeats   int
	RequireCarType string
	OriginName     string
	OriginLat      float64
	OriginLng      float64
	DestName       string
	DestLat        float64
	DestLng        float64
	OrigFareFen       int64
	PassengerPaidFen  int64
	PlatformCouponFen int64
	DriverCouponFen   int64
	CommissionFen     int64
	SubsidyFen        int64
	EventID        string
}

// GrabOrderCommand 抢单。
type GrabOrderCommand struct {
	DriverID       string
	OrderID        string
	IdempotencyKey string
}

// AcceptOrderCommand 确认接单。
type AcceptOrderCommand struct {
	DriverID       string
	OrderID        string
	Version        int64
	IdempotencyKey string
}

// ReleaseOrderCommand 退单。
type ReleaseOrderCommand struct {
	DriverID       string
	OrderID        string
	Reason         string
	ResponsibleParty string
	IdempotencyKey string
}

// StartTripCommand 开始行程（行程域/司机端）。
type StartTripCommand struct {
	DriverID       string
	OrderID        string
	IdempotencyKey string
}

// EndTripCommand 送达结束（行程域回调）。
type EndTripCommand struct {
	DriverID       string
	OrderID        string
	DistanceMeters int64
	DurationSec    int64
	IdempotencyKey string
	EventID        string
}

// ── 资金类命令（跨模块对接）──

// ReportPaymentCommand 支付域回调：结算收益入 U 桶。
type ReportPaymentCommand struct {
	OrderID        string
	DriverID       string
	AccountingDate string
	EventID        string
	IdempotencyKey string
	Amount         contracts.TaskAmount
}

// ReportRefundCommand 退款回调：生成反向流水。
type ReportRefundCommand struct {
	OrderID          string
	DriverID         string
	RefundFen        int64
	ResponsibleParty string
	AccountingDate   string
	Reason           string
	EventID          string
	IdempotencyKey   string
}

// RequestWithdrawCommand 申请提现。
type RequestWithdrawCommand struct {
	DriverID       string
	AmountFen      int64
	IdempotencyKey string
}

// SettleIncomeCommand 财务域结算确认：U → A。
type SettleIncomeCommand struct {
	DriverID string
	FlowIDs  []string
	EventID  string
}

// ApproveWithdrawCommand 提现审批通过（后台 A11）。
type ApproveWithdrawCommand struct {
	WithdrawalID string
	ApproverID   string
	Comment      string
}

// RejectWithdrawCommand 提现驳回（后台 A11）。
type RejectWithdrawCommand struct {
	WithdrawalID string
	ApproverID   string
	Reason       string
}

// CompleteWithdrawCommand 提现打款结果（财务代付适配器 / 后台 A12）。
type CompleteWithdrawCommand struct {
	WithdrawalID string
	Success      bool
	Reason       string
}

// ── 查询命令 ──

// ListFlowsCommand 流水分页查询。
type ListFlowsCommand struct {
	DriverID string
	Page     int
	PageSize int
	// From/To 记账日 yyyy-mm-dd，闭区间；留空表示不限。
	From string
	To   string
}

// ListWithdrawalsCommand 提现记录分页查询。
type ListWithdrawalsCommand struct {
	DriverID string
	Page     int
	PageSize int
	// From/To 申请日 yyyy-mm-dd，闭区间；留空表示不限。
	From string
	To   string
	// Status 提现状态；留空表示全部。
	Status string
}
