package entity

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	"github.com/BwCloudWeGo/bw-cli/pkg/geo"
)

// 订单六态（统一技术底座状态机，与乘客端/行程域/账务完全一致）。
const (
	OrderStatusWaiting   = contracts.OrderStatusWaiting
	OrderStatusAccepted  = contracts.OrderStatusAccepted
	OrderStatusArrived   = contracts.OrderStatusArrived
	OrderStatusInTrip    = contracts.OrderStatusInTrip
	OrderStatusEnded     = contracts.OrderStatusEnded
	OrderStatusCancelled = contracts.OrderStatusCancelled
)

// 接单通道常量。
const (
	DispatchChannelListen   = contracts.DispatchChannelListen
	DispatchChannelGrabPool = contracts.DispatchChannelGrabPool
)

var (
	ErrOrderNotFound        = errors.New("hall order not found")
	ErrInvalidOrder         = errors.New("invalid hall order")
	ErrOrderNotAvailable    = errors.New("order is no longer available")
	ErrOrderStateDenied     = errors.New("order state transition not allowed")
	ErrOrderNotMine         = errors.New("order is not assigned to this driver")
	ErrReleaseLimitExceeded = errors.New("daily release limit exceeded, only 3 times per day")
	ErrDriverNotAvailable   = errors.New("driver is not available for orders")
	ErrOrderNotVisible      = errors.New("order is not visible in grab pool yet")
	ErrTripAlreadyStarted   = errors.New("trip already started, release is not allowed")
)

// Location 订单地点（GCJ02）。
type Location struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

// Point 转 GCJ02 点。
func (l Location) Point() geo.Point { return geo.Point{Lat: l.Lat, Lng: l.Lng} }

// HallOrder 是接单大厅的订单聚合（同时也是派单任务实体）。
type HallOrder struct {
	ID   string
	TaskID string // 任务号（派单/抢单 actions 对外使用）
	OrderID string // 乘客域订单号
	PassengerName string
	PassengerCount int // 拼车时两乘客算 2 单（今日单量口径）
	CityCode      string
	OrderType     string
	RequireSeats  int
	RequireCarType string
	Origin      Location
	Destination Location

	// ── 金额：一律整数分 ──
	OrigFareFen       int64 // 原车费
	PassengerPaidFen  int64 // 乘客实付
	PlatformCouponFen int64 // 平台承担券
	DriverCouponFen   int64 // 司机承担券
	CommissionFen     int64 // 平台佣金
	SubsidyFen        int64 // 平台补贴
	DriverIncomeFen   int64 // 司机应得（= 乘客实付 - 佣金 - 司机券 + 补贴）

	Status    string
	DriverID  string
	DriverName string
	DispatchChannel string // listen / grab_pool
	VisibleInPool   bool   // 是否已对抢单司机可见
	PickupDistanceMeters int64

	// ── 派单预算：退单重派继承剩余预算，不重置 ──
	InviteRound      int64
	RoundInvited     []string // 本轮已邀请过的司机（同轮不重复）
	AssignExclude    []string // 明确排除的司机（退单司机本轮不再派）
	AcceptDeadline   time.Time
	ListenDeadline   time.Time // 听单窗口结束（转抢单池）
	PoolVisibleAt    time.Time
	DispatchDeadline time.Time // 总派单时限，超时平台责任取消

	AssignedAt  time.Time
	AcceptedAt  time.Time
	ArrivedAt   time.Time
	StartedAt   time.Time
	EndedAt     time.Time
	CancelledAt time.Time
	DurationSec     int64
	DistanceMeters  int64

	CancelReason     string
	ResponsibleParty string // passenger / driver / platform

	Version   int64 // 乐观锁版本
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateHallOrderParam 创建大厅订单的参数（金额单位：分）。
type CreateHallOrderParam struct {
	OrderID        string
	PassengerName  string
	PassengerCount int
	CityCode       string
	OrderType      string
	RequireSeats   int
	RequireCarType string
	Origin         Location
	Destination    Location
	OrigFareFen       int64
	PassengerPaidFen  int64
	PlatformCouponFen int64
	DriverCouponFen   int64
	CommissionFen     int64
	SubsidyFen        int64
	// 派单窗口参数
	ListenWindowSec int
	TotalWindowSec  int
	AcceptTimeoutSec int
}

// validLocation 校验 GCJ02 坐标：不允许 (0,0) 与越界值。
func validLocation(l Location) bool {
	if l.Lat == 0 || l.Lng == 0 {
		return false
	}
	return l.Lat >= -90 && l.Lat <= 90 && l.Lng >= -180 && l.Lng <= 180
}

// NewHallOrder 创建进入大厅池的订单（WAITING）。
func NewHallOrder(p CreateHallOrderParam) (*HallOrder, error) {
	if strings.TrimSpace(p.PassengerName) == "" {
		return nil, ErrInvalidOrder
	}
	paid := p.PassengerPaidFen
	if paid <= 0 {
		paid = p.OrigFareFen
	}
	if paid <= 0 {
		return nil, ErrInvalidOrder
	}
	if p.PlatformCouponFen < 0 || p.DriverCouponFen < 0 || p.CommissionFen < 0 || p.SubsidyFen < 0 {
		return nil, ErrInvalidOrder
	}
	// 坐标必须真实可用：起终点缺失或越界都会让派单距离与区域过滤失真。
	if !validLocation(p.Origin) || !validLocation(p.Destination) {
		return nil, ErrInvalidOrder
	}
	orig := p.OrigFareFen
	if orig <= 0 {
		orig = paid + p.PlatformCouponFen
	}
	if p.OrigFareFen > 0 && p.PassengerPaidFen > 0 && orig != paid+p.PlatformCouponFen {
		// 分账口恒校验：原车费 = 乘客实付 + 平台承担券，违反则以实付为准重算。
		orig = paid + p.PlatformCouponFen
	}
	if p.DriverCouponFen >= paid {
		p.DriverCouponFen = 0
	}
	income := paid - p.CommissionFen - p.DriverCouponFen + p.SubsidyFen
	if income < 0 {
		income = 0
	}
	now := biztime.UTC()
	listenSec := p.ListenWindowSec
	if listenSec <= 0 {
		listenSec = 60
	}
	totalSec := p.TotalWindowSec
	if totalSec <= 0 {
		totalSec = 120
	}
	if totalSec < listenSec {
		totalSec = listenSec
	}
	cnt := p.PassengerCount
	if cnt <= 0 {
		cnt = 1
	}
	orderID := strings.TrimSpace(p.OrderID)
	if orderID == "" {
		orderID = newID()
	}
	return &HallOrder{
		ID:                newID(),
		TaskID:            newID(),
		OrderID:           orderID,
		PassengerName:     strings.TrimSpace(p.PassengerName),
		PassengerCount:    cnt,
		CityCode:          strings.TrimSpace(p.CityCode),
		OrderType:         strings.TrimSpace(p.OrderType),
		RequireSeats:      p.RequireSeats,
		RequireCarType:    strings.TrimSpace(p.RequireCarType),
		Origin:            p.Origin,
		Destination:       p.Destination,
		OrigFareFen:       orig,
		PassengerPaidFen:  paid,
		PlatformCouponFen: p.PlatformCouponFen,
		DriverCouponFen:   p.DriverCouponFen,
		CommissionFen:     p.CommissionFen,
		SubsidyFen:        p.SubsidyFen,
		DriverIncomeFen:   income,
		Status:            OrderStatusWaiting,
		DispatchChannel:   DispatchChannelListen,
		RoundInvited:      []string{},
		AssignExclude:     []string{},
		AcceptDeadline:    now,
		ListenDeadline:    now.Add(time.Duration(listenSec) * time.Second),
		PoolVisibleAt:     now.Add(time.Duration(listenSec) * time.Second),
		DispatchDeadline:  now.Add(time.Duration(totalSec) * time.Second),
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// Amount 导出为跨模块契约的金额结构。
func (o *HallOrder) Amount() contracts.TaskAmount {
	return contracts.TaskAmount{
		OrigFareFen:       o.OrigFareFen,
		PassengerPaidFen:  o.PassengerPaidFen,
		PlatformCouponFen: o.PlatformCouponFen,
		DriverCouponFen:   o.DriverCouponFen,
		CommissionFen:     o.CommissionFen,
		SubsidyFen:        o.SubsidyFen,
		DriverIncomeFen:   o.DriverIncomeFen,
	}
}

// DistanceTo 司机到起点的距离（整数米）。
func (o *HallOrder) DistanceTo(lat, lng float64) int64 {
	return geo.DistanceMeters(o.Origin.Point(), geo.Point{Lat: lat, Lng: lng})
}

// IsWaiting 是否仍在等待接单。
func (o *HallOrder) IsWaiting() bool { return o.Status == OrderStatusWaiting }

// HasPendingInvite 是否有待确认邀请（预占中）。
func (o *HallOrder) HasPendingInvite(driverID string) bool {
	return o.Status == OrderStatusWaiting && o.DriverID != "" && o.DriverID == driverID
}

// InviteExpired 邀请是否已超时未确认。
func (o *HallOrder) InviteExpired(now time.Time) bool {
	return o.Status == OrderStatusWaiting && o.DriverID != "" && now.After(o.AcceptDeadline)
}

// VisibleToGrabPool 是否可由抢单司机抢占。
func (o *HallOrder) VisibleToGrabPool(now time.Time) bool {
	return o.Status == OrderStatusWaiting && o.VisibleInPool && now.Before(o.DispatchDeadline)
}

// CanRelease 仅未开始个人行程可退（WAITING/ACCEPTED，未到 ARRIVED/IN_TRIP）。
func (o *HallOrder) CanRelease(driverID string) error {
	if o.DriverID != driverID {
		return ErrOrderNotMine
	}
	switch o.Status {
	case OrderStatusAccepted:
		return nil
	case OrderStatusWaiting:
		if o.DriverID == "" {
			return ErrOrderNotMine
		}
		return nil
	case OrderStatusArrived, OrderStatusInTrip:
		return ErrTripAlreadyStarted
	default:
		return ErrOrderStateDenied
	}
}

// CanStartTrip 行程域可否开始行程。
func (o *HallOrder) CanStartTrip(driverID string) error {
	if o.DriverID != driverID {
		return ErrOrderNotMine
	}
	if o.Status != OrderStatusAccepted && o.Status != OrderStatusArrived {
		return ErrOrderStateDenied
	}
	return nil
}

// CanEndTrip 行程域可否送达。
func (o *HallOrder) CanEndTrip(driverID string) error {
	if o.DriverID != driverID {
		return ErrOrderNotMine
	}
	if o.Status != OrderStatusInTrip && o.Status != OrderStatusAccepted && o.Status != OrderStatusArrived {
		return ErrOrderStateDenied
	}
	return nil
}

// EnsureVersion 校验乐观锁版本。
func (o *HallOrder) EnsureVersion(version int64) error {
	if version > 0 && version != o.Version {
		return ErrVersionConflict
	}
	return nil
}

func newID() string { return uuid.NewString() }
