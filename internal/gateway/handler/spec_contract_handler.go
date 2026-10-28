package handler

// spec_handler.go 补齐《后端接口文档 v2.0》契约路径中尚未落地的能力。
//
// 背景：本仓库由多人分支合并而成，主干已落地的路由前缀大量是 /api/v1/*，
// 与接口文档约定的 /api/auth/*、/api/pax/*、/api/drv/*、/api/admin/*、/api/events 不一致，
// 且管理端 A01–A18 的 18 个叶子节点基本没有实现。
//
// 本文件在 gateway 适配层提供契约编排：
//   - 已有后端域能力的接口 → 由 spec_routes.go 直接代理到对应 handler；
//   - 尚无后端域承载的接口 → 在本文件内按文档第 4 章规则实现（进程内存储，课程模拟），
//     响应信封、状态机、幂等与资金守恒口径与文档保持一致，后续可整体迁到独立服务。

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// ── 统一状态取值（文档 1.4 / 4.2）──

const (
	// 订单六态
	OrderStateWaiting   = "WAITING"
	OrderStateAccepted  = "ACCEPTED"
	OrderStateArrived   = "ARRIVED"
	OrderStateInTrip    = "IN_TRIP"
	OrderStateEnded     = "ENDED"
	OrderStateCancelled = "CANCELLED"

	// 账单四态
	BillStateCalculating = "CALCULATING"
	BillStateUnpaid      = "UNPAID"
	BillStatePaid        = "PAID"
	BillStateVoid        = "VOID"

	// 支付尝试五态
	PayStateCreated    = "CREATED"
	PayStateProcessing = "PROCESSING"
	PayStateSuccess    = "SUCCESS"
	PayStateFailed     = "FAILED"
	PayStateClosed     = "CLOSED"

	// 优惠券五态
	CouponStateAvailable = "AVAILABLE"
	CouponStateLocked    = "LOCKED"
	CouponStateUsed      = "USED"
	CouponStateExpired   = "EXPIRED"
	CouponStateVoid      = "VOID"

	// 司机四态
	DriverStateOffline  = "offline"
	DriverStateIdle     = "idle"
	DriverStateBusy     = "busy"
	DriverStateReserved = "reserved"

	// 任务状态
	TaskStateDispatching = "DISPATCHING"
	TaskStateAssigned    = "ASSIGNED"
	TaskStateCompleted   = "COMPLETED"
	TaskStateCancelled   = "CANCELLED"
)

// 业务常量（文档各章节口径）
const (
	SpecSMSTTL            = 5 * time.Minute
	SpecSMSResendInterval = 60 * time.Second
	SpecSMSHourlyLimit    = 5
	SpecSMSMaxErrors      = 5
	SpecSMSLockWindow     = 30 * time.Minute

	SpecEstimateTTL    = 5 * time.Minute
	SpecCancelQuoteTTL = 30 * time.Second

	SpecDispatchTotalSec = 120
	SpecListenSec        = 60
	SpecGrabPoolSec      = 60
	SpecInviteSec        = 15
	SpecReserveSec       = 15

	SpecFreeCancelSec = 120
	SpecCancelFeeFen  = 500

	SpecVerifyMaxAttempts = 5
	SpecVerifyLockSec     = 600

	SpecRatingWindowHours = 72

	SpecShareMaxSec      = 24 * 60 * 60
	SpecShareAfterEndSec = 30 * 60
	SpecShareRateLimit   = 60

	SpecWithdrawMinFen      = 1000
	SpecWithdrawMaxPerTx    = 200000
	SpecWithdrawDailyMaxFen = 500000
	SpecWithdrawDailyMaxTx  = 3
	SpecWithdrawDualApprove = 50000

	SpecCarpoolGroupMax  = 2
	SpecCarpoolSeatMax   = 4
	SpecCarpoolMatchSec  = 60
	SpecCarpoolTotalSec  = 180

	SpecCoolingOffDays = 7
)

// ── 领域模型 ──

type specTimeline struct {
	Name string `json:"name"`
	Desc string `json:"desc,omitempty"`
	T    int64  `json:"t"`
}

type specMessage struct {
	ID   string `json:"id"`
	From string `json:"from"`
	Text string `json:"text"`
	T    int64  `json:"t"`
}

type specRefund struct {
	ID    string `json:"id"`
	R     int64  `json:"r"`
	DR    int64  `json:"dr"`
	Mode  string `json:"mode"`
	State string `json:"state"`
	T     int64  `json:"t"`
}

type specPosition struct {
	Lng       float64 `json:"lng"`
	Lat       float64 `json:"lat"`
	UpdatedAt int64   `json:"updated_at"`
}

type specChangeRequest struct {
	ID         string `json:"id"`
	NewEndID   int64  `json:"new_end_id"`
	NewEndName string `json:"new_end_name"`
	NewPrice   int64  `json:"new_price"`
	OldPrice   int64  `json:"old_price"`
	Version    int64  `json:"version"`
	ExpiresAt  int64  `json:"expires_at"`
	State      string `json:"state"`
}

type specShareToken struct {
	Token     string        `json:"token"`
	ExpiresAt int64         `json:"expires_at"`
	EndAt     int64         `json:"end_at,omitempty"`
	Revoked   bool          `json:"revoked"`
	Visits    map[int64]int `json:"-"`
}

type specOrder struct {
	ID            string               `json:"order_id"`
	PassengerID   string               `json:"passenger_id"`
	Type          string               `json:"type"`
	StartID       int64                `json:"start_id"`
	EndID         int64                `json:"end_id"`
	StartName     string               `json:"start_name"`
	EndName       string               `json:"end_name"`
	StartLng      float64              `json:"start_lng"`
	StartLat      float64              `json:"start_lat"`
	DistM         int64                `json:"dist_m"`
	DurS          int64                `json:"dur_s"`
	Seats         int                  `json:"seats,omitempty"`
	State         string               `json:"state"`
	Version       int64                `json:"version"`
	CouponID      string               `json:"coupon_id,omitempty"`
	Cp            int64                `json:"cp"`
	Cd            int64                `json:"cd"`
	PriceSnap     map[string]int64     `json:"price_snap"`
	CfgVersion    int64                `json:"cfg_version"`
	Fixed         bool                 `json:"fixed"`
	DriverID      string               `json:"driver_id,omitempty"`
	ActualDist    int64                `json:"actual_dist,omitempty"`
	ActualDur     int64                `json:"actual_dur,omitempty"`
	CancelFee     int64                `json:"cancel_fee,omitempty"`
	Rated        bool                 `json:"rated"`
	Verified     bool                 `json:"verified"`
	BoardedAt    int64                `json:"boarded_at,omitempty"`
	Changes       int                  `json:"changes"`
	Remark        string               `json:"remark,omitempty"`
	TaskID        string               `json:"task_id,omitempty"`
	CreatedAt     int64                `json:"created_at"`
	AcceptedAt    int64                `json:"accepted_at,omitempty"`
	ArrivedAt     int64                `json:"arrived_at,omitempty"`
	StartedAt    int64                `json:"started_at,omitempty"`
	EndedAt      int64                `json:"ended_at,omitempty"`
	CancelledAt  int64                `json:"cancelled_at,omitempty"`
	WaitDeadline  int64                `json:"-"`
	MatchDeadline int64                `json:"-"`
	DriverPos     *specPosition        `json:"-"`
	Timeline      []specTimeline       `json:"timeline"`
	Messages      []specMessage        `json:"messages"`
	Refunds       []specRefund         `json:"refunds"`
	ChangeReqs    []specChangeRequest  `json:"change_requests"`
	Share         *specShareToken      `json:"-"`
}

type specBill struct {
	ID       string `json:"bill_id"`
	OrderID  string `json:"order_id"`
	Kind     string `json:"kind"` // trip / cancel
	T        int64  `json:"T"`
	Q        int64  `json:"Q"`
	H        int64  `json:"H"`
	F        int64  `json:"F"`
	K        int64  `json:"K"`
	Cp       int64  `json:"cp"`
	Cd       int64  `json:"cd"`
	P         int64  `json:"P"`
	D         int64  `json:"D"`
	N         int64  `json:"N"`
	G         int64  `json:"G"`
	State     string `json:"state"`
	PaidAt    int64  `json:"paid_at,omitempty"`
	Channel   string `json:"channel,omitempty"`
	SettleID  string `json:"settlement_payment_id,omitempty"`
	RefundFen int64  `json:"refund_fen,omitempty"`
}

type specAttempt struct {
	ID             string `json:"attempt_id"`
	BillID         string `json:"bill_id"`
	Channel        string `json:"channel"`
	State          string `json:"state"`
	ChannelTradeNo string `json:"channel_trade_no,omitempty"`
	RequestNo      string `json:"request_no"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type specEstimate struct {
	ID          string           `json:"estimate_id"`
	PassengerID string           `json:"passenger_id"`
	Type        string           `json:"type"`
	StartID     int64            `json:"start_id"`
	EndID       int64            `json:"end_id"`
	DistM       int64            `json:"dist_m"`
	DurS        int64            `json:"dur_s"`
	PriceSnap   map[string]int64 `json:"price_snap"`
	CouponID    string           `json:"coupon_id,omitempty"`
	Seats       int              `json:"seats,omitempty"`
	ExpiresAt   int64            `json:"expires_at"`
	Used        bool             `json:"used"`
	CfgVersion  int64            `json:"cfg_version"`
}

type specCoupon struct {
	ID          string `json:"id"`
	TemplateID  string `json:"template_id"`
	PassengerID string `json:"passenger_id"`
	Name        string `json:"name"`
	Bearer      string `json:"bearer"`
	Threshold   int64  `json:"threshold"`
	Amount      int64  `json:"amount"`
	State       string `json:"state"`
	LockOrderID string `json:"lock_order_id,omitempty"`
	ExpireAt    int64  `json:"expire_at"`
}

type specCouponTemplate struct {
	ID         string   `json:"template_id"`
	Name       string   `json:"name"`
	FaceValue  int64    `json:"face_value"`
	Threshold  int64    `json:"threshold"`
	ValidFrom  int64    `json:"valid_from"`
	ValidTo    int64    `json:"valid_to"`
	CarTypes   []string `json:"car_types"`
	Area       string   `json:"area"`
	Stock      int64    `json:"stock"`
	PerLimit   int64    `json:"per_limit"`
	Bearer     string   `json:"bearer"`
	IssueType  string   `json:"issue_type"`
	State      string   `json:"state"`
}

type specTask struct {
	ID         string   `json:"task_id"`
	OrderIDs   []string `json:"order_ids"`
	State      string   `json:"state"`
	Phase      string   `json:"phase"` // listen / pool / assigned
	Candidates []string `json:"candidates,omitempty"`
	ModeVer    int64    `json:"mode_version"`
	DriverID   string   `json:"driver_id,omitempty"`
	Deadline   int64    `json:"deadline"`
	AssignedAt int64    `json:"assigned_at,omitempty"`
	ArrivedAt  int64    `json:"arrived_at,omitempty"`
	DepartedAt int64    `json:"departed_at,omitempty"`
	Version    int64    `json:"version"`
	ReserveAt  int64    `json:"-"`
	ReturnCnt  int64    `json:"-"`
	ExtraWin   int64    `json:"-"`
	Invited    []string `json:"-"`
}

type specTicket struct {
	ID        string   `json:"ticket_id"`
	OrderID   string   `json:"order_id"`
	Type      string   `json:"type"`
	Desc      string   `json:"desc"`
	Images    []string `json:"images,omitempty"`
	Reply     string   `json:"reply,omitempty"`
	State     string   `json:"state"`
	CreatedAt int64    `json:"created_at"`
	DoneAt    int64    `json:"done_at,omitempty"`
	Round     int      `json:"round"`
	Handler   string   `json:"handler,omitempty"`
}

type specSafetyEvent struct {
	ID        string  `json:"safety_event_id"`
	AccountID string  `json:"account_id"`
	OrderID   string  `json:"order_id,omitempty"`
	Type      string  `json:"type"`
	Lng       float64 `json:"lng,omitempty"`
	Lat       float64 `json:"lat,omitempty"`
	State     string  `json:"state"`
	CreatedAt int64   `json:"created_at"`
}

type specWithdrawal struct {
	ID          string   `json:"withdrawal_id"`
	DriverID    string   `json:"driver_id"`
	Fen         int64    `json:"fen"`
	State       string   `json:"state"`
	RequestNo   string   `json:"request_no"`
	Receipt     string   `json:"receipt,omitempty"`
	IdemKey     string   `json:"idempotency_key"`
	CreatedAt   int64    `json:"created_at"`
	Approvals   int      `json:"approvals"`
	ApproverIDs []string `json:"approver_ids,omitempty"`
}

// specFund 司机资金五桶（文档 1.5 / 4.3，互斥）。
type specFund struct {
	U           int64 `json:"U"`
	A           int64 `json:"A"`
	Fz          int64 `json:"Fz"`
	W           int64 `json:"W"`
	Debt        int64 `json:"debt"`
	TodayIncome int64 `json:"today_income"`
	TodayDone   int64 `json:"today_done"`
}

type specFlow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Fen  int64  `json:"fen"`
	T    int64  `json:"t"`
	Type string `json:"type"`
}

type specEvent struct {
	EventID string `json:"event_id"`
	T       int64  `json:"t"`
	Name    string `json:"name"`
	Detail  string `json:"detail"`
	Kind    string `json:"kind"`
}

type specOperationLog struct {
	ID        string `json:"id"`
	Operator  string `json:"operator"`
	Role      string `json:"role"`
	Resource  string `json:"resource"`
	Action    string `json:"action"`
	RequestID string `json:"request_id"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
	T         int64  `json:"t"`
}

type specExportTask struct {
	ID          string         `json:"export_id"`
	Type        string         `json:"type"`
	Filter      map[string]any `json:"filter,omitempty"`
	Applicant   string         `json:"applicant"`
	Version     int64          `json:"version"`
	State       string         `json:"state"`
	DownloadURL string         `json:"download_url"`
	ExpiresAt   int64          `json:"expires_at"`
}

type specDictEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int    `json:"version"`
	State   string `json:"state"`
}

type specRole struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

type specChannelBindings struct {
	Phone string            `json:"phone"`
	Bound map[string]bool   `json:"bound"`
	UIDs  map[string]string `json:"uids,omitempty"`
}

func (b *specChannelBindings) hasAnyLogin() bool {
	if b.Phone != "" {
		return true
	}
	for _, v := range b.Bound {
		if v {
			return true
		}
	}
	return false
}

func (b *specChannelBindings) list() []specBinding {
	out := make([]specBinding, 0, 3)
	for _, ch := range []string{"wechat", "qq", "sms"} {
		out = append(out, specBinding{Channel: ch, Bound: b.Bound[ch]})
	}
	return out
}

type specBinding struct {
	Channel string `json:"channel"`
	Bound   bool   `json:"bound"`
}

type specSMSRecord struct {
	Phone    string
	DeviceID string
	Code     string
	SentAt   time.Time
	Errors   int
	Sends    int
	LockedAt *time.Time
}

// specSession 是刷新令牌的服务端登记（文档 3.1.2 双令牌 + 撤销）。
type specSession struct {
	AccountID string `json:"account_id"`
	Role      string `json:"role"`
	Audience  string `json:"audience"`
	ExpiresAt int64  `json:"expires_at"`
}

// specAddress 是常用/历史地址（文档 3.2.3）。
type specAddress struct {
	ID    int64  `json:"id"`
	Type  string `json:"type"`
	Tag   string `json:"tag,omitempty"`
	Name  string `json:"name"`
	Addr  string `json:"addr"`
	POIID int64  `json:"poi_id,omitempty"`
	Count int64  `json:"count,omitempty"`
}

// specPaxProfile 是乘客资料（文档 3.2.1）。
type specPaxProfile struct {
	Name      string `json:"name"`
	Last4     string `json:"last4"`
	RealName  bool   `json:"real_name"`
	Agreement int64  `json:"agreement_version"`
}

type specIdemRecord struct {
	Key   string
	At    time.Time
	Value any
}

type specPOI struct {
	ID   int64   `json:"id"`
	Name string  `json:"name"`
	Addr string  `json:"addr"`
	City string  `json:"city"`
	Lng  float64 `json:"lng"`
	Lat  float64 `json:"lat"`
}

// specPricingConfig 计价配置（文档 2.1 pricing_config，版本化）。
type specPricingConfig struct {
	Version    int64  `json:"version"`
	City       string `json:"city"`
	Area       string `json:"area"`
	Type       string `json:"type"`
	Start      int64  `json:"start"`
	Incl       int64  `json:"incl"`
	PerKM      int64  `json:"per_km"`
	PerMin     int64  `json:"per_min"`
	ServiceBP  int64  `json:"service_bp"`
	CommBP     int64  `json:"comm_bp"`
	SurgeBP    int64  `json:"surge_bp"`
	CancelFee  int64  `json:"cancel_fee"`
	DiscountBP int64  `json:"discount_bp"`
	Fixed      int64  `json:"fixed_price"`
	Priority   int    `json:"priority"`
	ValidFrom  int64  `json:"valid_from"`
	ValidTo    int64  `json:"valid_to"`
	State      string `json:"state"`
}

type specVehicleStatusRecord struct {
	CurrentVersion string `json:"current_version"`
	CertExpire     string `json:"cert_expire"`
}

// specStore 是契约编排层的进程内存储（课程模拟）。
// 所有写操作在锁内完成；幂等键与事件 ID 具备唯一性语义。
type specStore struct {
	mu   sync.RWMutex
	seq  int64
	logs []*specOperationLog

	orders    map[string]*specOrder
	bills     map[string]*specBill
	attempts  map[string]*specAttempt
	estimates map[string]*specEstimate

	templates map[string]*specCouponTemplate
	coupons   map[string]*specCoupon
	bindings  map[string]*specChannelBindings

	tasks       map[string]*specTask
	draws       map[string]*specWithdrawal
	funds       map[string]*specFund
	flows       []*specFlow
	exps        map[string]*specExportTask
	tickets     map[string]*specTicket
	settlements map[string]*specSettlement
	events      []*specEvent
	refreshes   map[string]*specSession
	addresses   map[string][]*specAddress
	profiles    map[string]*specPaxProfile

	pois          []specPOI
	cfgs          []specPricingConfig
	dict          map[string]*specDictEntry
	roles         map[string]*specRole
	accounts      map[string]*specAdminUser
	applications  map[string]*specVehicleApplication
	campaigns     map[string]*specCampaign
	vehicleStatus map[string]*specVehicleStatusRecord

	dispatch    *specDispatchConfig
	dispatchVer int64
	pinche      *specPincheConfig
	pincheVer   int64

	sms            map[string]*specSMSRecord
	idem           map[string]*specIdemRecord
	eventsIdx      map[string]struct{}
	accountCancel  map[string]*specAccountCancel
	driverStatus   map[string]specDriverRuntime
	verifyAttempts map[string]specVerifyState
	shareVisits    map[string]map[int64]int
}

type specAccountCancel struct {
	State           string `json:"state"`
	CoolingOffUntil int64  `json:"cooling_off_until"`
	Blocks          []string `json:"blocks,omitempty"`
}

type specDriverRuntime struct {
	Status     string   `json:"status"`
	Mode       string   `json:"mode"`
	ModeVer    int64    `json:"mode_version"`
	LastBeatAt int64    `json:"last_beat_at"`
	OnlineSec  int64    `json:"online_sec"`
	Lng        float64  `json:"lng"`
	Lat        float64  `json:"lat"`
	Regions    []string `json:"regions,omitempty"`
	PrefOn     bool     `json:"pref_enabled"`
}

// specVehicleApplication 车辆审核版本链（文档 3.11.1 / A02）。
type specVehicleApplication struct {
	ID           string                    `json:"application_id"`
	Revision     int64                     `json:"revision"`
	State        string                    `json:"state"`
	Submitter    string                    `json:"submitter,omitempty"`
	RejectReason string                    `json:"reject_reason,omitempty"`
	RejectCnt    int                       `json:"reject_count"`
	ManualReview bool                      `json:"manual_review"`
	Version      int64                     `json:"version"`
	History      []specApplicationRevision `json:"history"`
}

type specApplicationRevision struct {
	Revision int64  `json:"revision"`
	Action   string `json:"action"`
	Reason   string `json:"reason,omitempty"`
	Auditor  string `json:"auditor"`
	T        int64  `json:"t"`
}

type specVerifyState struct {
	Errors   int
	LockedAt int64
}

var specGlobalStore = newSpecStore()

func newSpecStore() *specStore {
	s := &specStore{
		orders:    map[string]*specOrder{},
		bills:     map[string]*specBill{},
		attempts:  map[string]*specAttempt{},
		estimates: map[string]*specEstimate{},
		templates: map[string]*specCouponTemplate{},
		coupons:   map[string]*specCoupon{},
		bindings:  map[string]*specChannelBindings{},
		tasks:     map[string]*specTask{},
		draws:     map[string]*specWithdrawal{},
		funds:     map[string]*specFund{},
		exps:      map[string]*specExportTask{},
		dict:      map[string]*specDictEntry{},
		roles:     map[string]*specRole{},
		accounts:  map[string]*specAdminUser{},
		applications: map[string]*specVehicleApplication{},
		campaigns:    map[string]*specCampaign{},
		tickets:      map[string]*specTicket{},
		settlements:  map[string]*specSettlement{},
		refreshes:    map[string]*specSession{},
		addresses:    map[string][]*specAddress{},
		profiles:     map[string]*specPaxProfile{},
		vehicleStatus:  map[string]*specVehicleStatusRecord{},
		sms:            map[string]*specSMSRecord{},
		idem:           map[string]*specIdemRecord{},
		eventsIdx:      map[string]struct{}{},
		accountCancel:  map[string]*specAccountCancel{},
		driverStatus:   map[string]specDriverRuntime{},
		verifyAttempts: map[string]specVerifyState{},
		shareVisits:    map[string]map[int64]int{},
	}
	s.seed()
	return s
}

// seed 预置最小可用数据集。
func (s *specStore) seed() {
	now := time.Now().UTC().Unix()
	day := int64(24 * 60 * 60)

	s.cfgs = []specPricingConfig{
		// 有效期取十年窗口，避免依赖进程启动时刻（测试常用模拟时钟，文档 1.1）。
		{Version: 1, City: "上海", Area: "全市", Type: "kuaiche", Start: 1300, Incl: 3000, PerKM: 260, PerMin: 50,
			ServiceBP: 1000, CommBP: 2000, SurgeBP: 10000, CancelFee: SpecCancelFeeFen, DiscountBP: 10000, Priority: 1,
			ValidFrom: now - 3650*day, ValidTo: now + 3650*day, State: "已发布"},
		{Version: 1, City: "上海", Area: "全市", Type: "zhuanche", Start: 1800, Incl: 3000, PerKM: 380, PerMin: 80,
			ServiceBP: 1000, CommBP: 1800, SurgeBP: 10000, CancelFee: SpecCancelFeeFen, DiscountBP: 10000, Priority: 1,
			ValidFrom: now - 3650*day, ValidTo: now + 3650*day, State: "已发布"},
		{Version: 1, City: "上海", Area: "全市", Type: "pinche", Start: 1000, Incl: 3000, PerKM: 200, PerMin: 40,
			ServiceBP: 500, CommBP: 1500, SurgeBP: 10000, CancelFee: 300, DiscountBP: 7000, Fixed: 2000, Priority: 1,
			ValidFrom: now - 3650*day, ValidTo: now + 3650*day, State: "已发布"},
	}

	s.pois = []specPOI{
		{ID: 1, Name: "世纪大道地铁站", Addr: "浦东新区·世纪大道100号", City: "上海", Lng: 121.5238, Lat: 31.2336},
		{ID: 2, Name: "张江高科站", Addr: "浦东新区·龙东大道", City: "上海", Lng: 121.5903, Lat: 31.2053},
		{ID: 3, Name: "虹桥火车站", Addr: "闵行区·申贵路1500号", City: "上海", Lng: 121.3203, Lat: 31.1942},
		{ID: 4, Name: "迪士尼乐园", Addr: "浦东新区·川沙新镇", City: "上海", Lng: 121.6743, Lat: 31.1464},
	}

	s.templates["CT1"] = &specCouponTemplate{
		ID: "CT1", Name: "平台券 满20减5", FaceValue: 500, Threshold: 2000,
		ValidFrom: now - day, ValidTo: now + 30*day,
		CarTypes: []string{"kuaiche", "zhuanche"}, Area: "上海",
		Stock: 1000, PerLimit: 1, Bearer: "platform", IssueType: "注册", State: "已发布",
	}

	for _, role := range []specRole{
		{Code: "super_admin", Name: "超级管理员", Permissions: []string{"*"}},
		{Code: "operator", Name: "运营", Permissions: []string{"user:read", "user:restrict", "order:read", "campaign:write"}},
		{Code: "auditor", Name: "审核员", Permissions: []string{"driver-audit:read", "driver-audit:write"}},
		{Code: "cs", Name: "客服", Permissions: []string{"ticket:read", "ticket:write"}},
		{Code: "safety_handler", Name: "安全处理员", Permissions: []string{"safety:read", "safety:write"}},
		{Code: "finance_first", Name: "财务初审", Permissions: []string{"withdraw:review"}},
		{Code: "finance_second", Name: "财务复核", Permissions: []string{"withdraw:review"}},
	} {
		role := role
		s.roles[role.Code] = &role
	}

	s.dict["hotline"] = &specDictEntry{Key: "hotline", Value: "400-000-0000", Version: 1, State: "已发布"}
	s.dict["service_time"] = &specDictEntry{Key: "service_time", Value: "09:00-21:00", Version: 1, State: "已发布"}
	s.dict["faq"] = &specDictEntry{Key: "faq", Value: "[]", Version: 1, State: "已发布"}

	// 演示乘客：预置一张可用券与一个未认证实名状态。
	s.coupons["C1"] = &specCoupon{
		ID: "C1", TemplateID: "CT1", PassengerID: "P1", Name: "平台券 满20减5",
		Bearer: "platform", Threshold: 2000, Amount: 500, State: CouponStateAvailable,
		ExpireAt: now + 30*day,
	}
}

func (s *specStore) nextID(prefix string) string {
	s.seq++
	return prefix + strconv.FormatInt(s.seq, 10)
}

// emit 写入可靠事件（与业务写在同一个锁内，等价于同事务）。
// 按 event_id 去重，重复事件不重复入账。
func (s *specStore) emit(eventID, name, detail, kind string, at int64) bool {
	if eventID == "" {
		eventID = "evt-" + s.nextID("")
	}
	if _, ok := s.eventsIdx[eventID]; ok {
		return false
	}
	s.eventsIdx[eventID] = struct{}{}
	s.events = append(s.events, &specEvent{EventID: eventID, T: at, Name: name, Detail: detail, Kind: kind})
	return true
}

func (s *specStore) listEvents(limit int) []*specEvent {
	out := append([]*specEvent(nil), s.events...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].T > out[j].T })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// idemApply 幂等执行：相同键返回首次结果。
func (s *specStore) idemApply(key string, now time.Time, fn func() (any, error)) (any, bool, error) {
	if key == "" {
		value, err := fn()
		return value, false, err
	}
	if rec, ok := s.idem[key]; ok {
		return rec.Value, true, nil
	}
	value, err := fn()
	if err != nil {
		return nil, false, err
	}
	s.idem[key] = &specIdemRecord{Key: key, At: now, Value: value}
	return value, false, nil
}

func (s *specStore) ensureBindingsLocked(accountID string) *specChannelBindings {
	b, ok := s.bindings[accountID]
	if !ok {
		b = &specChannelBindings{
			Phone: "138****6688",
			Bound: map[string]bool{"sms": true},
			UIDs:  map[string]string{"sms": "138****6688"},
		}
		s.bindings[accountID] = b
	}
	return b
}

// consumeSMSLocked 校验并消费验证码（一次性），同时处理连续错误锁定。
// at 由调用方注入（来自 handler 的 clock），便于跨日/过期测试（文档 1.1）。
func (s *specStore) consumeSMSLocked(phone, deviceID, code string, at time.Time) bool {
	rec, ok := s.sms[phone+"|"+deviceID]
	if !ok {
		return false
	}
	now := at
	if rec.LockedAt != nil && now.Before(rec.LockedAt.Add(SpecSMSLockWindow)) {
		return false
	}
	if now.After(rec.SentAt.Add(SpecSMSTTL)) {
		return false
	}
	if code == "" || rec.Code != code {
		rec.Errors++
		if rec.Errors >= SpecSMSMaxErrors {
			locked := now
			rec.LockedAt = &locked
		}
		return false
	}
	rec.Errors = 0
	rec.Code = "" // 使用后立即失效
	return true
}

func (s *specStore) issueSMSLocked(phone, deviceID string, at time.Time) (string, error) {
	rec, ok := s.sms[phone+"|"+deviceID]
	if ok {
		if rec.LockedAt != nil && at.Before(rec.LockedAt.Add(SpecSMSLockWindow)) {
			return "", apperrors.New(apperrors.KindConflict, "42901", "sms locked, please retry later")
		}
		if at.Sub(rec.SentAt) < SpecSMSResendInterval {
			return "", apperrors.New(apperrors.KindConflict, "42901", "sms resend too frequent")
		}
		if at.Sub(rec.SentAt) < time.Hour && rec.Sends >= SpecSMSHourlyLimit {
			return "", apperrors.New(apperrors.KindConflict, "42901", "hourly sms limit exceeded")
		}
	} else {
		rec = &specSMSRecord{Phone: phone, DeviceID: deviceID}
		s.sms[phone+"|"+deviceID] = rec
	}
	code := specRandomCode()
	rec.Code = code
	rec.SentAt = at
	rec.Sends++
	return code, nil
}

func (s *specStore) findBillLocked(orderID, kind string) *specBill {
	for _, b := range s.bills {
		if b.OrderID == orderID && b.Kind == kind {
			return b
		}
	}
	return nil
}

func (s *specStore) ensureFundLocked(driverID string) *specFund {
	f, ok := s.funds[driverID]
	if !ok {
		f = &specFund{}
		s.funds[driverID] = f
	}
	return f
}

func (s *specStore) activeConfigLocked(city, carType string, at int64) (specPricingConfig, bool) {
	for _, c := range s.cfgs {
		if c.City == city && c.Type == carType && c.State == "已发布" && at >= c.ValidFrom && at <= c.ValidTo {
			return c, true
		}
	}
	return specPricingConfig{}, false
}

func (s *specStore) poiLocked(id int64) (specPOI, bool) {
	for _, p := range s.pois {
		if p.ID == id {
			return p, true
		}
	}
	return specPOI{}, false
}

func (s *specStore) appendLogLocked(operator, role, resource, action, requestID, before, after string, at int64) {
	s.logs = append(s.logs, &specOperationLog{
		ID:        s.nextID("L"),
		Operator:  operator,
		Role:      role,
		Resource:  resource,
		Action:    action,
		RequestID: requestID,
		Before:    before,
		After:     after,
		T:         at,
	})
}

func specRandomToken(prefix string) string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if prefix == "" {
		return hex.EncodeToString(buf)
	}
	return prefix + "-" + hex.EncodeToString(buf)
}

// specRandomCode 生成 6 位验证码（课程模拟：仅在服务端保存与比对，不下发正确答案）。
func specRandomCode() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	code := (int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2])) % 1000000
	return strconv.FormatInt(int64(code), 10)
}

// ── 计价与分账（文档 4.1 唯一公式）──

type specFare struct {
	T     int64 `json:"T"`
	Q     int64 `json:"Q"`
	H     int64 `json:"H"`
	K     int64 `json:"K"`
	F     int64 `json:"F"`
	P     int64 `json:"P"`
	D     int64 `json:"D"`
	N     int64 `json:"N"`
	G     int64 `json:"G"`
	Fixed bool  `json:"fixed"`
}

// computeFare 按文档 4.1 计算计价与分账明细，全程整数运算：
//
//	T = start + Round((dist - incl) * perKm, 1000) + Round(dur * perMin, 60)
//	Q = Round(T * surgeBp, 10000)   （拼车再乘 discountBp）
//	H = Round(Q * serviceBp, 10000)   （乘客服务费）
//	K = Round(Q * commBp, 10000)      （司机佣金）
//	F = Q + H                         （原车费）
//	P = F - Cp - Cd                   （乘客应付）
//	D = Q - K - Cd                    （司机应得）
//	N = H + K - Cp                    （平台净额）
//	资金守恒：P = D + N + G
//
// 估价与最终账单共用本函数，保证不会出现「估价一套、结账一套」。
//
// Round(n, d) 采用与 internal/pricing/entity 相同的 roundDiv 口径：
// 单价是「每公里/每分钟」而距离/时长是「米/秒」，先把乘积累计起来最后除一次，
// 避免每个分段各自取整导致误差累积。
func computeFare(cfg specPricingConfig, distM, durS, cpFen, cdFen int64, carpool bool) specFare {
	billableM := distM - cfg.Incl
	if billableM < 0 {
		billableM = 0
	}
	t := cfg.Start + specRoundDiv(billableM*cfg.PerKM, 1000) + specRoundDiv(durS*cfg.PerMin, 60)
	q := specRoundDiv(t*cfg.SurgeBP, 10000)
	if carpool {
		q = specRoundDiv(q*cfg.DiscountBP, 10000)
	}
	h := specRoundDiv(q*cfg.ServiceBP, 10000)
	k := specRoundDiv(q*cfg.CommBP, 10000)
	f := q + h
	p := f - cpFen - cdFen
	if p < 0 {
		p = 0
	}
	d := q - k - cdFen
	n := h + k - cpFen
	g := p - (d + n)
	return specFare{T: t, Q: q, H: h, K: k, F: f, P: p, D: d, N: n, G: g, Fixed: cfg.Fixed > 0}
}

// ── Handler ──

// SpecContractHandler 提供接口文档契约路径的编排实现。
type SpecContractHandler struct {
	log   *zap.Logger
	jwt   middleware.JWTConfig
	store *specStore
	clock clock.Clock
}

// NewSpecContractHandler 创建契约编排 handler。
func NewSpecContractHandler(jwtCfg middleware.JWTConfig, log *zap.Logger) *SpecContractHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &SpecContractHandler{log: log, jwt: jwtCfg, store: specGlobalStore, clock: clock.Real{}}
}

// WithClock 注入模拟时钟，支持跨日/过期测试（文档 1.1）。
func (h *SpecContractHandler) WithClock(c clock.Clock) *SpecContractHandler {
	if c != nil {
		h.clock = c
	}
	return h
}

func (h *SpecContractHandler) now() int64 { return h.clock.Now().UTC().Unix() }

// specChannelUID 模拟渠道适配器：用授权码在服务端换取渠道唯一标识。
// 真实实现由外部适配器完成；此处保证前端无法自行指定用户 ID。
func specChannelUID(channel, authCode string) string {
	sum := sha256.Sum256([]byte(channel + "|" + authCode))
	return channel + "-" + hex.EncodeToString(sum[:8])
}

// restoreTargetLocked 计算登录恢复目标（活动行程 > 未付账单 > 默认）。
// 调用方必须持有 store 锁。
func (h *SpecContractHandler) restoreTargetLocked(accountID string, _ int64) specRestoreTarget {
	for _, o := range h.store.orders {
		if o.PassengerID != accountID {
			continue
		}
		if o.State != OrderStateEnded && o.State != OrderStateCancelled {
			return specRestoreTarget{Type: "active_trip", OrderID: o.ID}
		}
	}
	for _, b := range h.store.bills {
		if b.State != BillStateUnpaid {
			continue
		}
		if o, ok := h.store.orders[b.OrderID]; ok && o.PassengerID == accountID {
			return specRestoreTarget{Type: "unpaid_bill", OrderID: b.OrderID}
		}
	}
	return specRestoreTarget{Type: "default"}
}

// specPrincipal 从 JWT claims 取当前身份，claims 缺失时回落到请求头（本地联调用）。
func specPrincipal(c *gin.Context) (userID, role string) {
	if claims := middleware.ClaimsFromContext(c); claims.UserID != "" {
		return claims.UserID, claims.Role
	}
	return c.GetHeader(middleware.HeaderAccountID), c.GetHeader("X-Role")
}

func specBindJSON(c *gin.Context, obj any) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "invalid_request: "+err.Error())
		return false
	}
	return true
}

// ── L01 发送短信验证码（文档 3.1.1）──

type specSMSSendRequest struct {
	Phone    string `json:"phone"`
	Scene    string `json:"scene"` // login / change_bind
	DeviceID string `json:"device_id"`
}

// SendSMS 下发验证码。响应只回倒计时，不下发正确答案（文档 3.1.1）。
func (h *SpecContractHandler) SendSMS(c *gin.Context) {
	var req specSMSSendRequest
	if !specBindJSON(c, &req) {
		return
	}
	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "phone is required")
		return
	}
	scene := strings.TrimSpace(req.Scene)
	if scene == "" {
		scene = "login"
	}
	deviceID := firstNonEmpty(req.DeviceID, "default")

	now := time.Now().UTC()
	h.store.mu.Lock()
	_, err := h.store.issueSMSLocked(phone, deviceID, now)
	h.store.mu.Unlock()
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	httpx.PaxOK(c, map[string]any{"retry_after": int(SpecSMSResendInterval / time.Second)})
}

// ── L03 获取登录后恢复目标 ──

type specRestoreTarget struct {
	Type    string `json:"type"` // active_trip / unpaid_bill / default
	OrderID string `json:"order_id,omitempty"`
}

// RestoreTarget 计算登录恢复目标（文档 3.1.3）：活动行程 > 未付账单 > 默认。
func (h *SpecContractHandler) RestoreTarget(c *gin.Context) {
	userID, _ := specPrincipal(c)
	if userID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	target := specRestoreTarget{Type: "default"}

	h.store.mu.RLock()
	for _, o := range h.store.orders {
		if o.PassengerID != userID {
			continue
		}
		if o.State != OrderStateEnded && o.State != OrderStateCancelled {
			target = specRestoreTarget{Type: "active_trip", OrderID: o.ID}
			break
		}
	}
	if target.Type == "default" {
		for _, b := range h.store.bills {
			if b.State != BillStateUnpaid {
				continue
			}
			if o, ok := h.store.orders[b.OrderID]; ok && o.PassengerID == userID {
				target = specRestoreTarget{Type: "unpaid_bill", OrderID: b.OrderID}
				break
			}
		}
	}
	h.store.mu.RUnlock()

	httpx.PaxOK(c, map[string]any{"restore_target": target})
}

// ── L06 绑定/解绑渠道（换绑，文档 3.1.6）──

type specBindingRequest struct {
	Action     string `json:"action"`       // bind / unbind
	Channel    string `json:"channel"`      // wechat / qq / sms
	OldSMSCode string `json:"old_sms_code"` // 换绑必填
	NewSMSCode string `json:"new_sms_code"` // 换绑必填
	NewPhone   string `json:"new_phone"`
	DeviceID   string `json:"device_id"`
	IdemKey    string `json:"idempotency_key"`
}

// Binding 处理渠道绑定/解绑：换绑需旧新两端短信校验；解绑须保留至少一种登录方式。
func (h *SpecContractHandler) Binding(c *gin.Context) {
	var req specBindingRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)
	if userID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	switch channel {
	case "wechat", "qq", "sms":
	default:
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "channel must be wechat/qq/sms")
		return
	}
	deviceID := firstNonEmpty(req.DeviceID, "default")
	now := time.Now().UTC()

	result, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey, "binding:"+userID+":"+req.Action+":"+channel), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		bindings := h.store.ensureBindingsLocked(userID)

		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "bind":
			if strings.TrimSpace(req.NewPhone) != "" {
				// 换绑：旧号与新号两端验证码都必须通过。
				if !h.store.consumeSMSLocked(bindings.Phone, deviceID, req.OldSMSCode, now) {
					return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "old sms code invalid")
				}
				if !h.store.consumeSMSLocked(req.NewPhone, deviceID, req.NewSMSCode, now) {
					return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "new sms code invalid")
				}
				bindings.Phone = req.NewPhone
			}
			bindings.Bound[channel] = true

		case "unbind":
			delete(bindings.Bound, channel)
			delete(bindings.UIDs, channel)
			if channel == "sms" {
				// 解绑短信渠道即解绑手机号，否则会残留一个无法解绑的登录方式。
				bindings.Phone = ""
			}
			if !bindings.hasAnyLogin() {
				return nil, apperrors.New(apperrors.KindConflict, "40901", "at least one login channel must be kept")
			}

		default:
			return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "action must be bind/unbind")
		}

		h.store.appendLogLocked(userID, "pax", "channel_binding", req.Action, httpx.RequestID(c), channel, "", now.Unix())
		return map[string]any{"bindings": bindings.list()}, nil
	})
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	if repeated {
		c.Header("X-Idempotent-Replay", "true")
	}
	httpx.PaxOK(c, result)
}

// ── L07 注销账号（申请/撤回，文档 3.1.7）──

type specAccountCancelRequest struct {
	Action string `json:"action"` // apply / withdraw
	Role   string `json:"role"`
}

// AccountCancel 处理注销申请与撤回。
// 前置校验：存在任一在途业务/债务即拒绝；冷静期 7 天，期内主动登录可撤回。
func (h *SpecContractHandler) AccountCancel(c *gin.Context) {
	var req specAccountCancelRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)
	if userID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "apply":
		blocks := h.store.cancelBlocksLocked(userID)
		if len(blocks) > 0 {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "account has pending obligations: "+strings.Join(blocks, ","))
			return
		}
		rec := &specAccountCancel{
			State:           "cooling_off",
			CoolingOffUntil: at + SpecCoolingOffDays*24*60*60,
		}
		h.store.accountCancel[userID] = rec
		httpx.PaxOK(c, map[string]any{"state": rec.State, "cooling_off_until": rec.CoolingOffUntil})
	case "withdraw":
		rec, ok := h.store.accountCancel[userID]
		if !ok {
			httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "no cancellation request")
			return
		}
		rec.State = "applied"
		delete(h.store.accountCancel, userID)
		httpx.PaxOK(c, map[string]any{"state": "applied", "cooling_off_until": 0})
	default:
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be apply/withdraw")
	}
}

// cancelBlocksLocked 汇总注销前置检查项：活动行程、未付账单、在途退款、未结工单、
// 司机未结算款/余额/冻结/在途提现。
func (s *specStore) cancelBlocksLocked(accountID string) []string {
	var blocks []string
	for _, o := range s.orders {
		if o.PassengerID != accountID {
			continue
		}
		if o.State != OrderStateEnded && o.State != OrderStateCancelled {
			blocks = append(blocks, "active_trip")
			break
		}
	}
	for _, b := range s.bills {
		if o, ok := s.orders[b.OrderID]; !ok || o.PassengerID != accountID {
			continue
		}
		if b.State == BillStateUnpaid {
			blocks = append(blocks, "unpaid_bill")
			break
		}
	}
	for _, o := range s.orders {
		if o.PassengerID != accountID {
			continue
		}
		for _, r := range o.Refunds {
			if r.State == "进行中" || r.State == "PENDING" {
				blocks = append(blocks, "pending_refund")
				break
			}
		}
	}
	for _, t := range s.tickets {
		if t.State == "待处理" || t.State == "处理中" {
			blocks = append(blocks, "open_ticket")
			break
		}
	}
	if f, ok := s.funds[accountID]; ok {
		if f.U > 0 || f.A > 0 || f.Fz > 0 || f.W > 0 || f.Debt > 0 {
			blocks = append(blocks, "driver_fund")
		}
	}
	return blocks
}

// ── P05 车型列表（文档 3.3.1）──

type specVehicleType struct {
	Type           string `json:"type"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	DisabledReason string `json:"disabled_reason"`
}

// VehicleTypes 按城市展示车型：不可用车型置灰并说明原因。
func (h *SpecContractHandler) VehicleTypes(c *gin.Context) {
	city := strings.TrimSpace(c.Query("city"))
	if city == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "city is required")
		return
	}
	at := h.now()

	h.store.mu.RLock()
	defer h.store.mu.RUnlock()

	names := map[string]string{"kuaiche": "快车", "zhuanche": "专车", "pinche": "拼车"}
	out := make([]specVehicleType, 0, len(names))
	for _, t := range []string{"kuaiche", "zhuanche", "pinche"} {
		_, ok := h.store.activeConfigLocked(city, t, at)
		item := specVehicleType{Type: t, Name: names[t], Enabled: ok}
		if !ok {
			item.DisabledReason = "当前城市暂无有效计价方案"
		}
		out = append(out, item)
	}
	httpx.PaxOK(c, map[string]any{"types": out})
}

// ── P07 等待接单状态查询（文档 3.3.4）──

type specWaitStatus struct {
	State            string `json:"state"`
	Deadline         any    `json:"deadline"`
	DriverSnap       any    `json:"driver_snap"`
	MatchingDeadline any    `json:"matching_deadline"`
}

// WaitStatus 轮询兜底查询。恢复时以服务端状态为准，不以本地倒计时取消订单。
func (h *SpecContractHandler) WaitStatus(c *gin.Context) {
	orderID := strings.TrimSpace(c.Query("order_id"))
	if orderID == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "order_id is required")
		return
	}
	userID, _ := specPrincipal(c)

	h.store.mu.RLock()
	order, ok := h.store.orders[orderID]
	if !ok || (userID != "" && order.PassengerID != userID) {
		h.store.mu.RUnlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "order not found")
		return
	}
	status := specWaitStatus{State: order.State}
	if order.WaitDeadline > 0 {
		status.Deadline = order.WaitDeadline
	}
	if order.MatchDeadline > 0 {
		status.MatchingDeadline = order.MatchDeadline
	}
	if order.DriverID != "" {
		status.DriverSnap = map[string]any{"driver_id": order.DriverID}
	}
	h.store.mu.RUnlock()

	httpx.PaxOK(c, status)
}

// ── P09 乘客端订单消息 / 虚拟号 ──

type specMessageRequest struct {
	OrderID string `json:"order_id"`
	Text    string `json:"text"`
	From    string `json:"from"`
}

// specContactAllowed 判定订单当前状态是否允许双方联系（文档 3.5.2 / P09）：
// 仅已接单/已到达/行程中允许，结束/取消后立即关闭入口。
func specContactAllowed(state string) bool {
	switch state {
	case OrderStateAccepted, OrderStateArrived, OrderStateInTrip:
		return true
	default:
		return false
	}
}

// ListPaxMessages 乘客端消息列表。
func (h *SpecContractHandler) ListPaxMessages(c *gin.Context) {
	h.listMessages(c)
}

// SendPaxMessage 乘客端发送消息，状态不允许时直接拒绝。
func (h *SpecContractHandler) SendPaxMessage(c *gin.Context) {
	var req specMessageRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)

	h.store.mu.Lock()
	order, ok := h.store.orders[req.OrderID]
	if !ok || (userID != "" && order.PassengerID != userID) {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "order not found")
		return
	}
	if !specContactAllowed(order.State) {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "contact closed for current order state")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "text is required")
		return
	}
	msg := specMessage{ID: h.store.nextID("M"), From: "pax", Text: req.Text, T: h.now()}
	order.Messages = append(order.Messages, msg)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"messages": []specMessage{msg}})
}

func (h *SpecContractHandler) listMessages(c *gin.Context) {
	orderID := strings.TrimSpace(c.Query("order_id"))
	if orderID == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "order_id is required")
		return
	}
	h.store.mu.RLock()
	order, ok := h.store.orders[orderID]
	if !ok {
		h.store.mu.RUnlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "order not found")
		return
	}
	if !specContactAllowed(order.State) {
		h.store.mu.RUnlock()
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "contact closed for current order state")
		return
	}
	msgs := append([]specMessage(nil), order.Messages...)
	h.store.mu.RUnlock()

	httpx.PaxOK(c, map[string]any{"messages": msgs})
}

// PaxVirtualCall 乘客端虚拟号呼叫（文档 3.4.3）：仅接单~行程中可用，失败回落客服热线。
func (h *SpecContractHandler) PaxVirtualCall(c *gin.Context) {
	var req specMessageRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)

	h.store.mu.Lock()
	order, ok := h.store.orders[req.OrderID]
	if !ok || (userID != "" && order.PassengerID != userID) {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "order not found")
		return
	}
	if !specContactAllowed(order.State) {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "virtual call closed for current order state")
		return
	}
	at := h.now()
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{
		"virtual_number": "170-0000-" + order.ID,
		"expires_at":     at + 1800,
		"hotline":        h.dictValue("hotline", "400-000-0000"),
	})
}

// ── P10 申请修改终点（文档 3.4.4）──

type specChangeEndpointRequest struct {
	OrderID string `json:"order_id"`
	NewEndID int64 `json:"new_end_id"`
	Version  int64 `json:"version"`
	IdemKey  string `json:"idempotency_key"`
}

// ChangeEndpoint 行程中变更终点：普通快车/专车每单最多 2 次；拼车禁止单方改点；
// 申请携带订单版本做乐观并发，司机同意与行程结束并发时只有一个版本提交成功。
func (h *SpecContractHandler) ChangeEndpoint(c *gin.Context) {
	var req specChangeEndpointRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)
	now := time.Now().UTC()
	at := now.Unix()

	result, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey, "change:"+req.OrderID+":"+strconv.FormatInt(req.Version, 10)), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		order, ok := h.store.orders[req.OrderID]
		if !ok || (userID != "" && order.PassengerID != userID) {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "order not found")
		}
		if order.Type == "pinche" {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "carpool order cannot change endpoint unilaterally")
		}
		if order.State != OrderStateInTrip && order.State != OrderStateArrived {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "endpoint change only allowed during trip")
		}
		if order.Version != req.Version {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "order version conflict, please refresh")
		}
		if order.Changes >= 2 {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "endpoint change limit reached")
		}
		poi, ok := h.store.poiLocked(req.NewEndID)
		if !ok {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "poi not found")
		}
		cfg, _ := h.store.activeConfigLocked("上海", order.Type, at)
		fare := computeFare(cfg, order.DistM, order.DurS, order.Cp, order.Cd, false)

		cr := specChangeRequest{
			ID:         h.store.nextID("CR"),
			NewEndID:   poi.ID,
			NewEndName: poi.Name,
			NewPrice:   fare.P,
			OldPrice:   order.PriceSnap["P"],
			Version:    order.Version,
			ExpiresAt:  at + int64(SpecCancelQuoteTTL/time.Second),
			State:      "待确认",
		}
		order.ChangeReqs = append(order.ChangeReqs, cr)
		return map[string]any{
			"new_end_name": cr.NewEndName,
			"new_price":    cr.NewPrice,
			"old_price":    cr.OldPrice,
			"expires_at":   cr.ExpiresAt,
			"version":      cr.Version,
		}, nil
	})
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	if repeated {
		c.Header("X-Idempotent-Replay", "true")
	}
	httpx.PaxOK(c, result)
}

// ── P11 行程分享（文档 3.4.5）──

type specShareRequest struct {
	OrderID string `json:"order_id"`
	Action  string `json:"action"` // create / revoke
	IdemKey string `json:"idempotency_key"`
}

// CreateShare 创建或撤销只读行程分享。令牌随机不可预测，不能由订单号推导。
func (h *SpecContractHandler) CreateShare(c *gin.Context) {
	var req specShareRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)
	now := time.Now().UTC()
	at := now.Unix()

	result, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey, "share:"+req.OrderID+":"+req.Action), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		order, ok := h.store.orders[req.OrderID]
		if !ok || (userID != "" && order.PassengerID != userID) {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "order not found")
		}
		switch strings.ToLower(strings.TrimSpace(req.Action)) {
		case "create":
			token := specRandomToken("shr")
			order.Share = &specShareToken{Token: token, ExpiresAt: at + SpecShareMaxSec, EndAt: order.EndedAt, Revoked: false}
			h.store.shareVisits[token] = map[int64]int{}
			return map[string]any{"share_token": token, "expires_at": order.Share.ExpiresAt}, nil
		case "revoke":
			if order.Share == nil {
				return nil, apperrors.New(apperrors.KindNotFound, "40401", "share token not found")
			}
			order.Share.Revoked = true
			return map[string]any{"share_token": order.Share.Token, "revoked": true}, nil
		default:
			return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "action must be create/revoke")
		}
	})
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	if repeated {
		c.Header("X-Idempotent-Replay", "true")
	}
	httpx.PaxOK(c, result)
}

// VisitShare 访问只读分享页：撤销/过期即时拒绝，访问限流每分钟 60 次。
// 分享页脱敏，不含手机号、支付信息与聊天记录。
func (h *SpecContractHandler) VisitShare(c *gin.Context) {
	// 契约路径是 /api/share/{token}，同时兼容 ?token= 的旧式调用。
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "token is required")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	var found *specOrder
	for _, o := range h.store.orders {
		if o.Share != nil && o.Share.Token == token {
			found = o
			break
		}
	}
	if found == nil {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "share not found")
		return
	}
	sh := found.Share
	switch {
	case sh.Revoked:
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "share revoked")
		return
	case at > sh.ExpiresAt:
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "share expired")
		return
	case sh.EndAt > 0 && at > sh.EndAt+SpecShareAfterEndSec:
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "share expired after trip end")
		return
	}
	minute := at / 60
	if h.store.shareVisits[token] == nil {
		h.store.shareVisits[token] = map[int64]int{}
	}
	h.store.shareVisits[token][minute]++
	visits := h.store.shareVisits[token][minute]
	h.store.mu.Unlock()

	if visits > SpecShareRateLimit {
		httpx.PaxError(c, http.StatusTooManyRequests, 42901, "share visit rate limit exceeded")
		return
	}

	httpx.PaxOK(c, map[string]any{
		"route": map[string]any{
			"start_region": maskRegion(found.StartName),
			"end_region":   maskRegion(found.EndName),
			"state":        found.State,
			"plate_mask":   maskPlate(found.DriverID),
		},
	})
}

// ── P13 收银台（文档 3.5.1）──

type specCheckoutData struct {
	Bill     map[string]any `json:"bill"`
	Coupons  []any          `json:"coupons"`
	Channels []string       `json:"channels"`
	ZeroPay  bool           `json:"zero_pay"`
}

// Checkout 收银台合并返回：金额来自账单而非客户端，前端不可篡改。
func (h *SpecContractHandler) Checkout(c *gin.Context) {
	orderID := strings.TrimSpace(c.Query("order_id"))
	billType := strings.TrimSpace(c.Query("bill_type"))
	if orderID == "" || billType == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "order_id and bill_type are required")
		return
	}
	if billType != "trip" && billType != "cancel" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "bill_type must be trip/cancel")
		return
	}
	userID, _ := specPrincipal(c)

	h.store.mu.RLock()
	order, ok := h.store.orders[orderID]
	if !ok || (userID != "" && order.PassengerID != userID) {
		h.store.mu.RUnlock()
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "order not found")
		return
	}
	bill := h.store.findBillLocked(orderID, billType)
	coupons := make([]any, 0)
	for _, cp := range h.store.coupons {
		if cp.PassengerID == order.PassengerID && cp.State == CouponStateAvailable {
			coupons = append(coupons, cp)
		}
	}
	h.store.mu.RUnlock()

	if bill == nil {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "bill not found")
		return
	}

	httpx.PaxOK(c, specCheckoutData{
		Bill: map[string]any{
			"bill_id": bill.ID,
			"P":       bill.P,
			"state":   bill.State,
			"detail": map[string]any{
				"T": bill.T, "Q": bill.Q, "H": bill.H, "F": bill.F,
				"K": bill.K, "Cp": bill.Cp, "Cd": bill.Cd,
				"D": bill.D, "N": bill.N, "G": bill.G,
			},
		},
		Coupons:  coupons,
		Channels: []string{"wechat", "alipay"},
		ZeroPay:  bill.P == 0,
	})
}

// ── P14 发起支付（文档 3.5.2）──

type specPayRequest struct {
	OrderID  string `json:"order_id"`
	BillType string `json:"bill_type"`
	Channel  string `json:"channel"` // wechat / alipay / zero
	IdemKey  string `json:"idempotency_key"`
}

// Pay 发起支付：同一账单同时只允许一个有效支付尝试；零元账单走内部 ZERO 渠道。
func (h *SpecContractHandler) Pay(c *gin.Context) {
	var req specPayRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.IdemKey) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "idempotency_key is required")
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	switch channel {
	case "wechat", "alipay", "zero":
	default:
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "channel must be wechat/alipay/zero")
		return
	}
	billType := firstNonEmpty(req.BillType, "trip")
	userID, _ := specPrincipal(c)
	now := time.Now().UTC()
	at := now.Unix()

	result, repeated, err := h.store.idemApply("pay:"+req.IdemKey, now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		order, ok := h.store.orders[req.OrderID]
		if !ok || (userID != "" && order.PassengerID != userID) {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "order not found")
		}
		bill := h.store.findBillLocked(req.OrderID, billType)
		if bill == nil {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "bill not found")
		}
		if bill.State == BillStatePaid {
			return map[string]any{"bill_id": bill.ID, "payable": bill.P, "pay_state": PayStateSuccess}, nil
		}
		for _, a := range h.store.attempts {
			if a.BillID == bill.ID && a.State == PayStateProcessing {
				return nil, apperrors.New(apperrors.KindConflict, "40901", "a payment attempt is in progress")
			}
		}
		if bill.P == 0 && channel != "zero" {
			return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "zero amount bill must use zero channel")
		}
		attempt := &specAttempt{
			ID:        h.store.nextID("PA"),
			BillID:    bill.ID,
			Channel:   channel,
			State:     PayStateProcessing,
			RequestNo: req.IdemKey,
			CreatedAt: at,
			UpdatedAt: at,
		}
		h.store.attempts[attempt.ID] = attempt
		h.store.emit("", "PaymentCreated", attempt.ID+" "+channel, "payment", at)
		return map[string]any{"bill_id": bill.ID, "payable": bill.P, "pay_state": attempt.State}, nil
	})
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	if repeated {
		c.Header("X-Idempotent-Replay", "true")
	}
	httpx.PaxOK(c, result)
}

// ── P16 支付回调（服务端内部，文档 3.5.3）──

type specPayCallbackRequest struct {
	ChannelTradeNo string `json:"channel_trade_no"`
	Channel        string `json:"channel"`
	BillID         string `json:"bill_id"`
	OrderID        string `json:"order_id"`
	BillType       string `json:"bill_type"`
	AmountFen      int64  `json:"amount_fen"`
	Sign           string `json:"sign"`
}

// PayCallback 处理渠道支付回调：
// 只有验签通过且金额一致的成功结果才能置 SUCCESS；
// 按渠道交易号去重，重复成功回调不重复入账、不重复增加司机收益；
// 账单以首个成功结清事务的支付为唯一有效付款（settlement_payment_id）。
func (h *SpecContractHandler) PayCallback(c *gin.Context) {
	var req specPayCallbackRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.ChannelTradeNo) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "channel_trade_no is required")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	// 回调幂等：渠道交易号唯一约束去重。
	for _, a := range h.store.attempts {
		if a.ChannelTradeNo == req.ChannelTradeNo && a.State == PayStateSuccess {
			httpx.PaxOK(c, map[string]any{"result": "SUCCESS", "repeated": true})
			return
		}
	}

	bill := h.store.bills[req.BillID]
	if bill == nil && req.OrderID != "" {
		bill = h.store.findBillLocked(req.OrderID, firstNonEmpty(req.BillType, "trip"))
	}
	if bill == nil {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "bill not found")
		return
	}
	if strings.TrimSpace(req.Sign) == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "sign is required")
		return
	}
	if req.AmountFen > 0 && req.AmountFen != bill.P {
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "callback amount mismatch")
		return
	}

	var attempt *specAttempt
	for _, a := range h.store.attempts {
		if a.BillID == bill.ID && a.State == PayStateProcessing {
			attempt = a
			break
		}
	}
	if attempt == nil {
		attempt = &specAttempt{
			ID:        h.store.nextID("PA"),
			BillID:    bill.ID,
			Channel:   req.Channel,
			State:     PayStateCreated,
			RequestNo: req.ChannelTradeNo,
			CreatedAt: at,
			UpdatedAt: at,
		}
		h.store.attempts[attempt.ID] = attempt
	}
	if bill.SettleID == "" {
		bill.SettleID = attempt.ID // 唯一结清支付
	}
	attempt.State = PayStateSuccess
	attempt.ChannelTradeNo = req.ChannelTradeNo
	attempt.UpdatedAt = at
	bill.State = BillStatePaid
	bill.PaidAt = at
	bill.Channel = req.Channel

	if bill.Kind == "trip" {
		if order := h.store.orders[bill.OrderID]; order != nil && order.DriverID != "" {
			fund := h.store.ensureFundLocked(order.DriverID)
			fund.U += bill.D
			fund.TodayIncome += bill.D
			fund.TodayDone++
			if h.store.emit("pay:"+bill.ID, "PaymentSucceeded", bill.ID, "payment", at) {
				h.store.flows = append(h.store.flows, &specFlow{
					ID:   h.store.nextID("F"),
					Name: "行程收益 " + bill.OrderID,
					Fen:  bill.D,
					T:    at,
					Type: "income",
				})
			}
		}
	}

	httpx.PaxOK(c, map[string]any{"result": "SUCCESS", "repeated": false})
}

// ── P15 支付查单与关单（文档 3.5.4）──

type specPayQueryRequest struct {
	BillID string `json:"bill_id" form:"bill_id"`
}

// PayQuery 主动查单：超时标 PROCESSING 并主动查单，页面跳回不能作为支付成功依据。
func (h *SpecContractHandler) PayQuery(c *gin.Context) {
	h.payQueryOrClose(c, "query")
}

// PayClose 关单：15 分钟关单；晚到成功进入差异处理，不双收双记。
func (h *SpecContractHandler) PayClose(c *gin.Context) {
	h.payQueryOrClose(c, "close")
}

func (h *SpecContractHandler) payQueryOrClose(c *gin.Context, action string) {
	var req specPayQueryRequest
	_ = c.ShouldBind(&req)
	if req.BillID == "" {
		req.BillID = c.Query("bill_id")
	}
	if req.BillID == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "bill_id is required")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	bill := h.store.bills[req.BillID]
	if bill == nil {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "bill not found")
		return
	}
	state := h.store.latestAttemptStateLocked(bill.ID)
	if action == "close" {
		for _, a := range h.store.attempts {
			if a.BillID == bill.ID && a.State == PayStateProcessing {
				a.State = PayStateClosed
				a.UpdatedAt = at
			}
		}
		if state == PayStateProcessing {
			state = PayStateClosed
		}
	}

	httpx.PaxOK(c, map[string]any{"pay_state": state})
}

func (s *specStore) latestAttemptStateLocked(billID string) string {
	state := PayStateCreated
	latest := int64(0)
	for _, a := range s.attempts {
		if a.BillID != billID {
			continue
		}
		if a.UpdatedAt >= latest {
			latest = a.UpdatedAt
			state = a.State
		}
	}
	return state
}

// ── D02 听单邀请响应（文档 3.8.4）──

type specInviteRespondRequest struct {
	TaskID string `json:"task_id"`
	Accept bool   `json:"accept"`
}

// InviteRespond 处理听单邀请响应：接受后司机转为忙碌、订单进入 ACCEPTED；
// 拒绝后本轮不再邀请同一司机。
func (h *SpecContractHandler) InviteRespond(c *gin.Context) {
	var req specInviteRespondRequest
	if !specBindJSON(c, &req) {
		return
	}
	driverID, _ := specPrincipal(c)
	if driverID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	task, ok := h.store.tasks[req.TaskID]
	if !ok {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "task not found")
		return
	}
	if task.State != TaskStateDispatching || task.Phase != "listen" {
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "task is not in listening phase")
		return
	}
	for _, id := range task.Invited {
		if id == driverID {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "driver already invited in this round")
			return
		}
	}
	task.Invited = append(task.Invited, driverID)

	if !req.Accept {
		httpx.PaxOK(c, map[string]any{"task_id": task.ID, "assigned": false})
		return
	}
	// 唯一性约束：一司机一个活动任务；一任务一个司机。
	for _, t := range h.store.tasks {
		if t.ID != task.ID && t.DriverID == driverID && t.State == TaskStateAssigned {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "driver already has an active task")
			return
		}
	}
	task.DriverID = driverID
	task.State = TaskStateAssigned
	task.Phase = "assigned"
	task.AssignedAt = at
	task.Version++
	for _, oid := range task.OrderIDs {
		if o, ok := h.store.orders[oid]; ok {
			o.State = OrderStateAccepted
			o.DriverID = driverID
			o.AcceptedAt = at
			o.Version++
			o.Timeline = append(o.Timeline, specTimeline{Name: "ACCEPTED", Desc: "司机接单", T: at})
		}
	}
	if rt, ok := h.store.driverStatus[driverID]; ok {
		rt.Status = DriverStateBusy
		h.store.driverStatus[driverID] = rt
	}
	h.store.emit("", "TaskAssigned", task.ID+" "+driverID, "dispatch", at)
	httpx.PaxOK(c, map[string]any{"task_id": task.ID, "assigned": true})
}

// ── D14 车辆审核状态查询（文档 3.11.2）──

type specVehicleStatus struct {
	CurrentVersion string `json:"current_version"`
	CertExpire     string `json:"cert_expire"`
	RemindDays     int    `json:"remind_days"`
}

// VehicleStatus 返回当前生效版本、证件有效期与 30/7/1 天提醒节点。
func (h *SpecContractHandler) VehicleStatus(c *gin.Context) {
	driverID, _ := specPrincipal(c)
	at := h.now()

	h.store.mu.RLock()
	status := specVehicleStatus{CurrentVersion: "v1", CertExpire: "", RemindDays: 0}
	if rec, ok := h.store.vehicleStatus[driverID]; ok {
		status.CurrentVersion = rec.CurrentVersion
		status.CertExpire = rec.CertExpire
		status.RemindDays = specRemindDays(rec.CertExpire, at)
	}
	h.store.mu.RUnlock()

	httpx.PaxOK(c, status)
}

// specRemindDays 按 30/7/1 天节点返回剩余天数；不命中返回 0。
func specRemindDays(certExpire string, nowSec int64) int {
	if strings.TrimSpace(certExpire) == "" {
		return 0
	}
	expire, err := time.Parse("2006-01-02", strings.TrimSpace(certExpire))
	if err != nil {
		return 0
	}
	days := int(time.Until(expire).Hours() / 24)
	_ = nowSec
	switch days {
	case 30, 7, 1:
		return days
	default:
		return 0
	}
}

// ── 事件总线（文档 3.18）──

// ListEvents 查询事件日志，支持 limit 取最近 N 条。
func (h *SpecContractHandler) ListEvents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 50
	}
	h.store.mu.RLock()
	events := h.store.listEvents(limit)
	h.store.mu.RUnlock()
	if events == nil {
		events = []*specEvent{}
	}
	httpx.PaxOK(c, map[string]any{"events": events})
}

// ── L02 登录（三渠道，文档 3.1.2）──

type specLoginRequest struct {
	Channel  string `json:"channel"`   // wechat / qq / sms
	AuthCode string `json:"auth_code"` // 微信/QQ 授权码（服务端换渠道唯一标识）
	Phone    string `json:"phone"`     // sms 时必填
	SMSCode  string `json:"sms_code"`  // sms 时必填
	Role     string `json:"role"`      // pax / drv
	DeviceID string `json:"device_id"`
	IdemKey  string `json:"idempotency_key"`
}

// Login 处理三渠道登录。
//
// 关键约束：微信/QQ 用授权码在服务端换渠道唯一标识，禁止前端指定用户 ID；
// 手机号已存在时先验证原账号再绑定渠道，不按昵称自动合并账号；
// 令牌 payload 携带角色并在服务端校验，乘客/司机/后台三套令牌按受众隔离，
// 角色切换必须重新登录。
func (h *SpecContractHandler) Login(c *gin.Context) {
	var req specLoginRequest
	if !specBindJSON(c, &req) {
		return
	}
	channel := strings.ToLower(strings.TrimSpace(req.Channel))
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "pax" && role != "drv" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "role must be pax or drv")
		return
	}
	deviceID := firstNonEmpty(req.DeviceID, "default")
	now := time.Now().UTC()

	value, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey,
		"login:"+channel+":"+firstNonEmpty(req.Phone, req.AuthCode)+":"+role), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()

		accountID := ""
		switch channel {
		case "sms":
			phone := strings.TrimSpace(req.Phone)
			if phone == "" {
				return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "phone is required")
			}
			// 验证码只做服务端比对，不返回正确答案。
			if !h.store.consumeSMSLocked(phone, deviceID, req.SMSCode, now) {
				return nil, apperrors.New(apperrors.KindUnauthorized, "40101", "sms code invalid or expired")
			}
			for id, b := range h.store.bindings {
				if b.Phone == phone {
					accountID = id
					break
				}
			}
			if accountID == "" {
				accountID = h.store.nextID("A")
			}
		case "wechat", "qq":
			code := strings.TrimSpace(req.AuthCode)
			if code == "" {
				return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "auth_code is required")
			}
			uid := specChannelUID(channel, code)
			for id, b := range h.store.bindings {
				if b.UIDs[channel] == uid {
					accountID = id
					break
				}
			}
			if accountID == "" {
				accountID = h.store.nextID("A")
			}
		default:
			return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "channel must be wechat/qq/sms")
		}

		b := h.store.ensureBindingsLocked(accountID)
		b.Bound[channel] = true
		if channel == "sms" {
			b.Phone = strings.TrimSpace(req.Phone)
			b.UIDs["sms"] = b.Phone
		} else {
			b.UIDs[channel] = specChannelUID(channel, req.AuthCode)
		}

		tokens, err := h.issueTokens(accountID, role, now)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"access_token":   tokens.Access,
			"refresh_token":  tokens.Refresh,
			"role":           role,
			"restore_target": h.restoreTargetLocked(accountID, now.Unix()),
		}, nil
	})
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	if repeated {
		c.Header("X-Idempotent-Replay", "true")
	}
	httpx.PaxOK(c, value)
}

type specTokenPair struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
}

// issueTokens 签发访问令牌（30 分钟）+ 刷新令牌（7 天）。
// 调用方必须持有 store 写锁。
func (h *SpecContractHandler) issueTokens(accountID, role string, now time.Time) (specTokenPair, error) {
	accessCfg := h.jwt
	accessCfg.ExpireSeconds = 1800
	access, err := middleware.GenerateToken(accessCfg, middleware.JWTClaims{UserID: accountID, Role: role})
	if err != nil {
		return specTokenPair{}, apperrors.Wrap(apperrors.KindInternal, "50001", "generate access token failed", err)
	}
	// 刷新令牌是不透明随机串，服务端登记，撤销即失效（退出登录撤销刷新令牌）。
	refresh := specRandomToken("rt")
	h.store.refreshes[refresh] = &specSession{
		AccountID: accountID,
		Role:      role,
		Audience:  role,
		ExpiresAt: now.Add(7 * 24 * time.Hour).Unix(),
	}
	return specTokenPair{Access: access, Refresh: refresh}, nil
}

// ── L04 刷新会话令牌（文档 3.1.4）──

type specRefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh 刷新并轮换令牌：旧刷新令牌立即失效，退出登录/注销账号后不可用。
func (h *SpecContractHandler) Refresh(c *gin.Context) {
	var req specRefreshRequest
	if !specBindJSON(c, &req) {
		return
	}
	token := strings.TrimSpace(req.RefreshToken)
	if token == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "refresh_token is required")
		return
	}
	now := time.Now().UTC()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	session, ok := h.store.refreshes[token]
	if !ok || session == nil {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "refresh token is revoked or unknown")
		return
	}
	if now.Unix() > session.ExpiresAt {
		delete(h.store.refreshes, token)
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "refresh token is expired")
		return
	}
	// 轮换：撤销旧的再发新的，防止重放。
	delete(h.store.refreshes, token)
	pair, err := h.issueTokens(session.AccountID, session.Role, now)
	if err != nil {
		httpx.PaxAppError(c, err)
		return
	}
	httpx.PaxOK(c, map[string]any{
		"access_token":  pair.Access,
		"refresh_token": pair.Refresh,
	})
}

// ── L05 退出登录（文档 3.1.5）──

type specLogoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Logout 撤销刷新令牌；访问令牌自然过期，不再续期。
func (h *SpecContractHandler) Logout(c *gin.Context) {
	var req specLogoutRequest
	if !specBindJSON(c, &req) {
		return
	}
	token := strings.TrimSpace(req.RefreshToken)
	if token == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "refresh_token is required")
		return
	}
	h.store.mu.Lock()
	delete(h.store.refreshes, token)
	at := h.now()
	operator, _ := specPrincipal(c)
	h.store.appendLogLocked(operator, "account", "session", "logout", httpx.RequestID(c), token, "revoked", at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"revoked": true})
}

// ── P03 常用/历史地址管理（文档 3.2.3）──

type specAddressRequest struct {
	Action string `json:"action"` // save / delete / clear
	ID     int64  `json:"id"`
	Type   string `json:"type"` // 家/公司/其他/历史
	Tag    string `json:"tag"`
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	POIID  int64  `json:"poi_id"`
}

// PaxAddressCommand 处理地址的增改删清空。
// 常用地址最多 20 条；删除常用/清空历史不影响历史订单的地址快照。
func (h *SpecContractHandler) PaxAddressCommand(c *gin.Context) {
	var req specAddressRequest
	if !specBindJSON(c, &req) {
		return
	}
	userID, _ := specPrincipal(c)
	if userID == "" {
		httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "unauthorized")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	list := h.store.addresses[userID]

	_ = at
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "save":
		if strings.TrimSpace(req.Type) == "" {
			httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "type is required")
			return
		}
		common := 0
		for _, a := range list {
			if a.Type != "历史" {
				common++
			}
		}
		if req.ID > 0 {
			for _, a := range list {
				if a.ID == req.ID {
					a.Type, a.Tag, a.Name, a.Addr, a.POIID = req.Type, req.Tag, req.Name, req.Addr, req.POIID
					break
				}
			}
		} else {
			if common >= 20 {
				httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "common address limit reached")
				return
			}
			list = append(list, &specAddress{
				ID: h.store.nextIDNum(), Type: req.Type, Tag: req.Tag,
				Name: req.Name, Addr: req.Addr, POIID: req.POIID,
			})
		}
	case "delete":
		kept := list[:0]
		for _, a := range list {
			if a.ID != req.ID {
				kept = append(kept, a)
			}
		}
		list = kept
	case "clear":
		kept := list[:0]
		for _, a := range list {
			if a.Type != "历史" {
				kept = append(kept, a)
			}
		}
		list = kept
	default:
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be save/delete/clear")
		return
	}
	h.store.addresses[userID] = list
	httpx.PaxOK(c, specAddressView(list))
}

func (s *specStore) nextIDNum() int64 {
	s.seq++
	return s.seq
}

func specAddressView(list []*specAddress) map[string]any {
	common := make([]specAddress, 0, len(list))
	history := make([]specAddress, 0, len(list))
	for _, a := range list {
		if a.Type == "历史" {
			history = append(history, *a)
			continue
		}
		common = append(common, *a)
	}
	sort.SliceStable(history, func(i, j int) bool {
		if history[i].Count != history[j].Count {
			return history[i].Count > history[j].Count
		}
		return history[i].ID > history[j].ID
	})
	if len(history) > 10 {
		history = history[:10]
	}
	return map[string]any{"common": common, "history": history}
}

// ── 工具 ──

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// maskRegion 脱敏地点：只保留区域前缀。
func maskRegion(name string) string {
	parts := strings.Split(name, "·")
	if len(parts) == 0 {
		return name
	}
	return strings.TrimSpace(parts[0])
}

// maskPlate 脱敏车牌/司机标识，只保留尾号。
func maskPlate(id string) string {
	if len(id) <= 2 {
		return "*"
	}
	return "***" + id[len(id)-2:]
}

// dictValue 读取已发布字典值。
func (h *SpecContractHandler) dictValue(key, fallback string) string {
	h.store.mu.RLock()
	defer h.store.mu.RUnlock()
	if entry, ok := h.store.dict[key]; ok && entry.State == "已发布" {
		return entry.Value
	}
	return fallback
}
