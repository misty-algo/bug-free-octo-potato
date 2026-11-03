package repo

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/mongox"
)

// 与司机域 internal/driver/model/hall_order.go 的文档结构保持一致：
// 共库部署下乘客域直写 hall_orders 集合完成投单，司机域调度器照常消费。
const hallOrderCollection = "hall_orders"

// driverProfileCollection 与司机域 internal/driver/model 的集合名保持一致，
// 用于读取接单司机的心跳位置。
const driverProfileCollection = "driver_profiles"

// hallOrderDocument 是司机域派单任务文档在乘客域的只写视图。
// 只填派单必需字段，其余（邀请轮次等）由司机域调度器维护。
type hallOrderDocument struct {
	ID      string `bson:"_id"`
	TaskID  string `bson:"task_id"`
	OrderID string `bson:"order_id"`

	PassengerName  string `bson:"passenger_name"`
	PassengerCount int    `bson:"passenger_count"`
	CityCode       string `bson:"city_code"`
	OrderType      string `bson:"order_type"`
	RequireSeats   int    `bson:"require_seats"`
	// 车型不按字符串过滤（传空），由听单司机自行抢单，避免两端车型词表不一致导致无单可派。
	RequireCarType string     `bson:"require_car_type"`
	Origin         hallDocLoc `bson:"origin"`
	Destination    hallDocLoc `bson:"destination"`

	OrigFareFen       int64 `bson:"orig_fare_fen"`
	PassengerPaidFen  int64 `bson:"passenger_paid_fen"`
	PlatformCouponFen int64 `bson:"platform_coupon_fen"`
	DriverCouponFen   int64 `bson:"driver_coupon_fen"`
	CommissionFen     int64 `bson:"commission_fen"`
	SubsidyFen        int64 `bson:"subsidy_fen"`
	DriverIncomeFen   int64 `bson:"driver_income_fen"`

	Status          string `bson:"status"`
	DriverID        string `bson:"driver_id"`
	DriverName      string `bson:"driver_name"`
	DispatchChannel string `bson:"dispatch_channel"`
	VisibleInPool   bool   `bson:"visible_in_pool"`

	// 乘客发起的更改下车点请求（nil 表示无请求）。
	DestChange *hallDestChange `bson:"destination_change,omitempty"`

	InviteRound      int64     `bson:"invite_round"`
	RoundInvited     []string  `bson:"round_invited"`
	AssignExclude    []string  `bson:"assign_exclude"`
	AcceptDeadline   time.Time `bson:"accept_deadline"`
	ListenDeadline   time.Time `bson:"listen_deadline"`
	PoolVisibleAt    time.Time `bson:"pool_visible_at"`
	DispatchDeadline time.Time `bson:"dispatch_deadline"`

	AssignedAt  time.Time `bson:"assigned_at"`
	AcceptedAt  time.Time `bson:"accepted_at"`
	EndedAt     time.Time `bson:"ended_at"`
	CancelledAt time.Time `bson:"cancelled_at"`

	CancelReason     string `bson:"cancel_reason"`
	ResponsibleParty string `bson:"responsible_party"`

	Version   int64     `bson:"version"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

type hallDocLoc struct {
	Name string  `bson:"name"`
	Lat  float64 `bson:"lat"`
	Lng  float64 `bson:"lng"`
}

// hallDestChange 派单任务上的目的地变更请求（乘客域写入，司机域裁决）。
type hallDestChange struct {
	NewName     string    `bson:"new_name"`
	NewAddr     string    `bson:"new_addr"`
	NewLat      float64   `bson:"new_lat"`
	NewLng      float64   `bson:"new_lng"`
	State       string    `bson:"state"` // pending / approved / rejected
	RequestedAt time.Time `bson:"requested_at"`
}

// driverPositionDocument 是司机域 driver_profiles 的只读投影：
// 只关心接单司机最近一次心跳位置，共库部署下乘客端行程地图直接读这里。
type driverPositionDocument struct {
	ID              string    `bson:"_id"`
	LastLat         float64   `bson:"last_lat"`
	LastLng         float64   `bson:"last_lng"`
	LastHeartbeatAt time.Time `bson:"last_heartbeat_at"`
}

// HallOrderBridge 把派单任务写进司机域 hall_orders 集合并回查状态。
type HallOrderBridge struct {
	tasks    *mongox.Collection[hallOrderDocument]
	profiles *mongox.Collection[driverPositionDocument]
}

// NewHallOrderBridge 构造派单池桥。
func NewHallOrderBridge(db *mongo.Database) *HallOrderBridge {
	return &HallOrderBridge{
		tasks:    mongox.NewCollection[hallOrderDocument](db, hallOrderCollection),
		profiles: mongox.NewCollection[driverPositionDocument](db, driverProfileCollection),
	}
}

// CreateDispatchTask 投单。同一 order_id 已存在任务时幂等返回。
func (b *HallOrderBridge) CreateDispatchTask(ctx context.Context, task entity.DispatchTask) error {
	if task.OrderNo == "" {
		return errors.New("hall bridge: empty order no")
	}
	if _, err := b.tasks.FindOne(ctx, bson.M{"order_id": task.OrderNo}); err == nil {
		return nil
	} else if !errors.Is(err, mongox.ErrNotFound) {
		return err
	}
	now := time.Now().UTC()
	listenWindow := 90 * time.Second
	totalWindow := 300 * time.Second
	document := &hallOrderDocument{
		ID:                task.OrderNo, // 以乘客订单号为主键，天然防重复投单
		TaskID:            "task-" + task.OrderNo,
		OrderID:           task.OrderNo,
		PassengerName:     task.PassengerName,
		PassengerCount:    1,
		Origin:            hallDocLoc{Name: task.OriginName, Lat: task.OriginLat, Lng: task.OriginLng},
		Destination:       hallDocLoc{Name: task.DestName, Lat: task.DestLat, Lng: task.DestLng},
		OrigFareFen:       task.PaidFen + task.PlatformCouponFen,
		PassengerPaidFen:  task.PaidFen,
		PlatformCouponFen: task.PlatformCouponFen,
		CommissionFen:     task.CommissionFen,
		// 司机净得 = 乘客实付 - 平台佣金（平台券由平台承担，不扣司机）。
		DriverIncomeFen: task.PaidFen - task.CommissionFen,
		Status:            "WAITING",
		DispatchChannel:   "listen",
		RoundInvited:      []string{},
		AssignExclude:     []string{},
		AcceptDeadline:    now,
		ListenDeadline:    now.Add(listenWindow),
		PoolVisibleAt:     now.Add(listenWindow),
		DispatchDeadline:  now.Add(totalWindow),
		Version:           1,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	_, err := b.tasks.Insert(ctx, document)
	// 并发重复投单时主键冲突：任务已存在视为成功（幂等）。
	if err != nil && !strings.Contains(err.Error(), "duplicate key") {
		return err
	}
	return nil
}

// FindTaskByOrderNo 回查订单对应的派单任务状态。
func (b *HallOrderBridge) FindTaskByOrderNo(ctx context.Context, orderNo string) (*entity.HallTaskView, error) {
	document, err := b.tasks.FindOne(ctx, bson.M{"order_id": orderNo})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	view := &entity.HallTaskView{
		TaskID:      document.TaskID,
		Status:      document.Status,
		DriverID:    document.DriverID,
		DriverName:  document.DriverName,
		DriverPlate: "",
		DestChange:  toEntityDestChange(document.DestChange),
	}
	// 已接单时顺带带上司机最近心跳位置，乘客端地图据此画车。
	// 心跳查不到不算错误：地图少一个点比接口整体失败更合理。
	if document.DriverID != "" {
		if profile, err := b.profiles.FindOne(ctx, bson.M{"_id": document.DriverID}); err == nil {
			view.DriverLat = profile.LastLat
			view.DriverLng = profile.LastLng
			view.DriverLocatedAt = profile.LastHeartbeatAt
		}
	}
	return view, nil
}

// toEntityDestChange 桥视图 → 实体。
func toEntityDestChange(doc *hallDestChange) *entity.DestinationChange {
	if doc == nil {
		return nil
	}
	return &entity.DestinationChange{
		NewEndName:  doc.NewName,
		NewEndAddr:  doc.NewAddr,
		NewEndLng:   doc.NewLng,
		NewEndLat:   doc.NewLat,
		State:       doc.State,
		RequestedAt: doc.RequestedAt,
	}
}

// RequestDestinationChange 把更改下车点请求写到派单任务上。
// 仅行程中（已接单未送达）的任务接受请求；新请求覆盖旧请求。
func (b *HallOrderBridge) RequestDestinationChange(ctx context.Context, orderNo string, change entity.DestinationChange) error {
	now := time.Now().UTC()
	result, err := b.tasks.UpdateOne(ctx,
		bson.M{"order_id": orderNo, "status": bson.M{"$in": bson.A{"ACCEPTED", "ARRIVED", "IN_TRIP"}}},
		bson.M{"$set": bson.M{
			"destination_change": hallDestChange{
				NewName:     change.NewEndName,
				NewAddr:     change.NewEndAddr,
				NewLat:      change.NewEndLat,
				NewLng:      change.NewEndLng,
				State:       change.State,
				RequestedAt: now,
			},
			"updated_at": now,
		}},
	)
	if err != nil {
		return err
	}
	if result == nil || result.MatchedCount == 0 {
		return entity.ErrOrderNotFound
	}
	return nil
}

// FindSettlementByOrderNo 查订单的结算要素：接单司机与分账金额。
func (b *HallOrderBridge) FindSettlementByOrderNo(ctx context.Context, orderNo string) (*entity.HallSettlementView, error) {
	document, err := b.tasks.FindOne(ctx, bson.M{"order_id": orderNo})
	if err != nil {
		if errors.Is(err, mongox.ErrNotFound) {
			return nil, entity.ErrOrderNotFound
		}
		return nil, err
	}
	return &entity.HallSettlementView{
		DriverID:         document.DriverID,
		PassengerPaidFen: document.PassengerPaidFen,
		DriverIncomeFen:  document.DriverIncomeFen,
	}, nil
}

// CancelWaitingTask 取消尚未被接单的任务；已被接单返回 false。
func (b *HallOrderBridge) CancelWaitingTask(ctx context.Context, orderNo string, reason string) (bool, error) {
	now := time.Now().UTC()
	result, err := b.tasks.UpdateOne(ctx,
		bson.M{"order_id": orderNo, "status": "WAITING"},
		bson.M{"$set": bson.M{
			"status":            "CANCELLED",
			"cancel_reason":     reason,
			"responsible_party": "passenger",
			"cancelled_at":      now,
			"updated_at":        now,
		}},
	)
	if err != nil {
		return false, err
	}
	return result.MatchedCount > 0, nil
}
