package entity

import (
	"context"
	"time"
)

// HallTaskView 是司机域派单任务中乘客域关心的视图。
type HallTaskView struct {
	TaskID      string
	Status      string // WAITING/ACCEPTED/ARRIVED/IN_TRIP/ENDED/CANCELLED
	DriverID    string
	DriverName  string
	DriverPlate string
	// DriverLat / DriverLng / DriverLocatedAt 是接单司机最近一次心跳位置（WGS84），
	// 乘客端行程地图据此画司机实时位置；未接单或没有心跳时为零值。
	DriverLat       float64
	DriverLng       float64
	DriverLocatedAt time.Time
	// DestChange 司机侧任务上记录的目的地变更裁决状态（nil 表示无请求）。
	DestChange *DestinationChange
}

// HallSettlementView 是行程结束后的结算视图：支付成功回调据此通知司机域入账。
type HallSettlementView struct {
	// DriverID 为接单司机标识；空表示任务未被接单，无从结算。
	DriverID string
	// PassengerPaidFen / DriverIncomeFen 单位分：乘客实付与司机净得（已扣平台佣金）。
	PassengerPaidFen int64
	DriverIncomeFen  int64
}

// DispatchTask 是乘客下单后投进司机域派单池的任务描述。
type DispatchTask struct {
	// OrderNo 为乘客域订单号，司机域以它做幂等与回查键。
	OrderNo       string
	PassengerName string
	OriginName    string
	OriginLat     float64
	OriginLng     float64
	DestName      string
	DestLat       float64
	DestLng       float64
	// PaidFen / PlatformCouponFen / CommissionFen 单位分：
	// 乘客实付、平台承担券与平台佣金（司机收入 = 实付 - 佣金）。
	PaidFen           int64
	PlatformCouponFen int64
	CommissionFen     int64
}

// HallOrderBridge 是乘客域与司机域派单池（hall_orders 集合）之间的桥。
// 共库部署下由乘客域直写司机域集合，替代跨服务事件；接口保持抽象，
// 后续切换为事件/HTTP 时只换仓储实现。
type HallOrderBridge interface {
	// CreateDispatchTask 把订单投进派单池。同一订单重复投单幂等（不产生第二任务）。
	CreateDispatchTask(ctx context.Context, task DispatchTask) error
	// FindTaskByOrderNo 查订单对应的派单任务；不存在返回 ErrOrderNotFound。
	FindTaskByOrderNo(ctx context.Context, orderNo string) (*HallTaskView, error)
	// FindSettlementByOrderNo 查订单的结算要素（接单司机与分账金额）；
	// 任务不存在返回 ErrOrderNotFound。
	FindSettlementByOrderNo(ctx context.Context, orderNo string) (*HallSettlementView, error)
	// CancelWaitingTask 取消尚未被司机接单的任务，返回是否取消成功；
	// 任务已被接单时返回 false，由双方线下协商或走客服流程。
	CancelWaitingTask(ctx context.Context, orderNo string, reason string) (bool, error)
	// RequestDestinationChange 把乘客的更改下车点请求写到派单任务上，
	// 由司机端裁决；仅行程中（已接单未送达）的任务接受请求。
	RequestDestinationChange(ctx context.Context, orderNo string, change DestinationChange) error
}
