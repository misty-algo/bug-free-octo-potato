package entity

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
	"github.com/BwCloudWeGo/bw-cli/pkg/geo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// 司机四态（与统一技术底座一致：收车 / 空闲 / 忙碌 / 接单预占）。
const (
	DriverStatusOffline     = contracts.DriverStatusOffline
	DriverStatusIdle        = contracts.DriverStatusIdle
	DriverStatusBusy        = contracts.DriverStatusBusy
	DriverStatusPreOccupied = contracts.DriverStatusPreOccupied
)

// 接单模式枚举：听单、抢单互斥。
const (
	ModeListen = "listen" // 听单：系统派单
	ModeGrab   = "grab"   // 抢单：司机在大厅自抢
)

var (
	ErrDriverNotFound       = errors.New("driver not found")
	ErrInvalidDriver        = errors.New("invalid driver")
	ErrInvalidStatus        = errors.New("invalid driver status")
	ErrStatusNotAllowed     = errors.New("status transition not allowed")
	ErrInvalidMode          = errors.New("invalid driver mode")
	ErrBusyCannotOffline    = errors.New("busy driver cannot go offline, complete current order first")
	ErrPreOccupiedBusy      = errors.New("driver has a pending order invitation, confirm or wait for timeout first")
	ErrModeSwitchNotAllowed = errors.New("mode can only be switched when driver is idle")
	ErrAreaChangeNotAllowed = errors.New("area config can only be changed when driver is idle")
	ErrDriverCannotAccept   = errors.New("driver is banned from accepting orders")
	ErrDriverUnqualified    = errors.New("driver qualification is invalid")
	ErrInsufficientFunds    = errors.New("insufficient funds in source bucket")
	ErrInvalidBucket        = errors.New("invalid funds bucket")
	ErrVersionConflict      = errors.New("entity version conflict, retry the command")
)

// 资金五桶键名（司机侧资金互斥模型）。
const (
	BucketPending    = "U"    // 待结算（行程已完成但尚未结算到账）
	BucketAvailable  = "A"    // 可用（可提现金额就是 A）
	BucketFrozen     = "Fz"   // 冻结
	BucketWithdraw   = "W"    // 提现预占
	BucketDebt       = "Debt" // 待追偿
)

// ReleaseRecord 退单记录（含责任方）：乘客取消与平台责任取消不计入每日 3 次上限。
type ReleaseRecord struct {
	At    time.Time
	Party string
}

// Funds 司机资金五桶，单位统一为分（int64），互斥且总和守恒。
type Funds struct {
	Pending   int64 `json:"pending_fen"`
	Available int64 `json:"available_fen"`
	Frozen    int64 `json:"frozen_fen"`
	Withdraw  int64 `json:"withdraw_fen"`
	Debt      int64 `json:"debt_fen"`
}

// Get 按桶名取余额。
func (f Funds) Get(bucket string) (int64, error) {
	switch bucket {
	case BucketPending:
		return f.Pending, nil
	case BucketAvailable:
		return f.Available, nil
	case BucketFrozen:
		return f.Frozen, nil
	case BucketWithdraw:
		return f.Withdraw, nil
	case BucketDebt:
		return f.Debt, nil
	}
	return 0, ErrInvalidBucket
}

// Add 按桶名增减余额（内部使用，外部一律走仓储原子操作）。
func (f *Funds) Add(bucket string, delta int64) error {
	switch bucket {
	case BucketPending:
		f.Pending += delta
	case BucketAvailable:
		f.Available += delta
	case BucketFrozen:
		f.Frozen += delta
	case BucketWithdraw:
		f.Withdraw += delta
	case BucketDebt:
		f.Debt += delta
	default:
		return ErrInvalidBucket
	}
	return nil
}

// CanTransfer 判断源桶是否有足够余额。
func (f Funds) CanTransfer(from string, amount int64) bool {
	cur, err := f.Get(from)
	if err != nil || amount <= 0 {
		return false
	}
	return cur >= amount
}

// AreaBox 是一个矩形接单片区（初版用包围盒近似多边形，边界点算区域内）。
type AreaBox struct {
	Name   string  `json:"name"`
	MinLat float64 `json:"min_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLat float64 `json:"max_lat"`
	MaxLng float64 `json:"max_lng"`
}

// Driver 是司机档案聚合根：状态机、接单模式、接单区域、资质与资金五桶。
type Driver struct {
	ID    string
	DriverNo string // 司机编号：排序并列时的稳定打破依据
	Name  string
	Phone string
	// Status 四态：offline / idle / busy / pre_occupied
	Status   string
	Mode     string
	ModeVersion int64 // 模式版本：切换时递增，使旧邀请立即失效
	AreaLimitSwitch bool
	Areas    []AreaBox
	CityCode string
	CarType  string
	Seats    int
	Qualified bool // 资质有效（含证照未过期）
	CanAccept bool // 后台禁接单开关（A01）
	OfflineAfterTrip bool // 本单后收车
	LastHeartbeatAt time.Time
	LastLat float64 // 最近一次心跳上报的坐标（GCJ02，用于派单距离）
	LastLng float64
	ActiveTaskID    string // 当前活动任务（唯一约束兜底：一司机一个活动任务）
	OnlineSinceAt   time.Time
	RecentReleases  []ReleaseRecord // 最近退单记录（含责任方，用於每日 3 次上限）
	MonthlyIncomeFen int64
	Rating      float64 // 好评率 0–5
	SuccessRate float64 // 成功率 0–1
	Funds       Funds
	Version     int64 // 乐观锁版本
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// newDriverID 生成司机档案标识。
//
// 司机标识要与行程域、派单域共用同一个取值：那两个域以 Mongo ObjectId 作为
// 司机主键（dispatch_tasks.driverId、drivers._id），并在入口按 ObjectId 格式
// 校验令牌里的司机号。因此这里直接产出 24 位十六进制的 ObjectId 串，避免
// 司机档案与行程数据各用一套标识而对接不上。
func newDriverID() string { return bson.NewObjectID().Hex() }

// NewDriver 注册司机，默认空闲 + 抢单模式。
func NewDriver(name, phone string) (*Driver, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidDriver
	}
	now := biztime.UTC()
	return &Driver{
		ID:              newDriverID(),
		DriverNo:        newDriverNo(),
		Name:            name,
		Phone:           strings.TrimSpace(phone),
		Status:          DriverStatusIdle,
		Mode:            ModeGrab,
		ModeVersion:     1,
		Areas:           []AreaBox{},
		CityCode:        defaultCityCode,
		CarType:         "comfort",
		Seats:           4,
		Qualified:       true,
		CanAccept:       true,
		LastHeartbeatAt: now,
		OnlineSinceAt:   now,
		Rating:          5.0,
		SuccessRate:     1.0,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

const defaultCityCode = "510100" // 课程演示默认城市：成都

// SetStatus 司机状态流转。忙碌/预占状态只能通过订单生命周期（接单或释放）回到空闲。
func (d *Driver) SetStatus(status string) error {
	switch status {
	case DriverStatusOffline:
		switch d.Status {
		case DriverStatusBusy:
			return ErrBusyCannotOffline
		case DriverStatusPreOccupied:
			return ErrPreOccupiedBusy
		}
	case DriverStatusIdle:
		if d.Status == DriverStatusBusy || d.Status == DriverStatusPreOccupied {
			return ErrStatusNotAllowed
		}
	default:
		return ErrInvalidStatus
	}
	d.Status = status
	d.bump()
	return nil
}

// ToBusy 由确认接单触发：空闲/接单预占 → 忙碌，绑定活动任务。
// 抢单与邀请会先进入"接单预占"，因此预占 → 忙碌是合法流转。
func (d *Driver) ToBusy(taskID string) error {
	if d.Status != DriverStatusIdle && d.Status != DriverStatusPreOccupied {
		return ErrStatusNotAllowed
	}
	if taskID == "" {
		return ErrInvalidStatus
	}
	d.Status = DriverStatusBusy
	d.ActiveTaskID = taskID
	d.bump()
	return nil
}

// ToPreOccupied 由邀请/抢占触发：空闲 → 接单预占，绑定活动任务。
func (d *Driver) ToPreOccupied(taskID string) error {
	if d.Status != DriverStatusIdle {
		return ErrStatusNotAllowed
	}
	d.Status = DriverStatusPreOccupied
	d.ActiveTaskID = taskID
	d.bump()
	return nil
}

// ReleaseTask 释放运力：忙碌/预占 → 空闲（行程送达即释放，与付款无关）。
// 返回 true 表示司机设置了"本单后收车"，需要自动收车。
func (d *Driver) ReleaseTask() bool {
	if d.Status == DriverStatusBusy || d.Status == DriverStatusPreOccupied {
		d.Status = DriverStatusIdle
		d.ActiveTaskID = ""
		d.bump()
	}
	offline := d.OfflineAfterTrip
	if offline {
		d.Status = DriverStatusOffline
		d.OfflineAfterTrip = false
		d.bump()
	}
	return offline
}

// SetMode 切换听单/抢单模式：仅空闲允许，切换时递增模式版本使旧邀请失效。
func (d *Driver) SetMode(mode string) error {
	if mode != ModeListen && mode != ModeGrab {
		return ErrInvalidMode
	}
	if d.Status != DriverStatusIdle {
		return ErrModeSwitchNotAllowed
	}
	if d.Mode == mode {
		return nil
	}
	d.Mode = mode
	d.ModeVersion++
	d.bump()
	return nil
}

// UpdateAreaConfig 修改接单区域：仅空闲允许。
func (d *Driver) UpdateAreaConfig(switchOn bool, areas []AreaBox) error {
	if d.Status != DriverStatusIdle {
		return ErrAreaChangeNotAllowed
	}
	for _, a := range areas {
		// 片区包围盒必须合法，否则区域过滤会出现"看不见任何单"的静默失效。
		if a.MinLat >= a.MaxLat || a.MinLng >= a.MaxLng {
			return ErrInvalidDriver
		}
		if a.MinLat < -90 || a.MaxLat > 90 || a.MinLng < -180 || a.MaxLng > 180 {
			return ErrInvalidDriver
		}
	}
	d.AreaLimitSwitch = switchOn
	if areas == nil {
		areas = []AreaBox{}
	}
	d.Areas = areas
	d.bump()
	return nil
}

// TouchHeartbeat 记录心跳时间与最新坐标。
func (d *Driver) TouchHeartbeat(at time.Time, lat, lng float64) {
	d.LastHeartbeatAt = at
	if lat != 0 || lng != 0 {
		d.LastLat, d.LastLng = lat, lng
	}
	d.bump()
}

// SetOfflineAfterTrip 设置"本单后收车"。
func (d *Driver) SetOfflineAfterTrip(on bool) {
	d.OfflineAfterTrip = on
	d.bump()
}

// InArea 判断订单起点是否在接单范围内：开关关闭时全城可见。
func (d *Driver) InArea(lat, lng float64) bool {
	if !d.AreaLimitSwitch || len(d.Areas) == 0 {
		return true
	}
	p := geo.Point{Lat: lat, Lng: lng}
	for _, area := range d.Areas {
		if geo.InBox(p, area.MinLat, area.MinLng, area.MaxLat, area.MaxLng) {
			return true
		}
	}
	return false
}

// MatchRequirement 车型/座位/城市是否满足任务要求（0 表示无要求）。
func (d *Driver) MatchRequirement(seats int, carType, cityCode string) bool {
	if cityCode != "" && d.CityCode != "" && cityCode != d.CityCode {
		return false
	}
	if carType != "" && d.CarType != "" && carType != d.CarType {
		return false
	}
	if seats > 0 && d.Seats > 0 && seats > d.Seats {
		return false
	}
	return true
}

// CanReceiveOrder 是否可接单：空闲 + 资质有效 + 未被禁接单。
func (d *Driver) CanReceiveOrder() error {
	if !d.Qualified {
		return ErrDriverUnqualified
	}
	if !d.CanAccept {
		return ErrDriverCannotAccept
	}
	if d.Status != DriverStatusIdle {
		return ErrDriverNotAvailable
	}
	return nil
}

// EnsureVersion 校验乐观锁版本（服务端读到的版本须与命令携带的一致）。
func (d *Driver) EnsureVersion(version int64) error {
	if version > 0 && version != d.Version {
		return ErrVersionConflict
	}
	return nil
}

func (d *Driver) bump() {
	d.Version++
	d.UpdatedAt = biztime.UTC()
}

var noSeq int64

func newDriverNo() string {
	noSeq++
	if noSeq == 0 {
		noSeq = rand.Int63n(9000) + 1000
	}
	return fmt.Sprintf("D%06d", noSeq)
}

// PrimeDriverNoSequence 用库里已有的最大司机编号推进序号起点。
// 进程重启后内存序号归零，不校准就会发出与历史重复的司机编号。
func PrimeDriverNoSequence(existingMax string) {
	var value int64
	for _, r := range strings.TrimSpace(existingMax) {
		if r < '0' || r > '9' {
			continue
		}
		value = value*10 + int64(r-'0')
	}
	if value > noSeq {
		noSeq = value
	}
}
