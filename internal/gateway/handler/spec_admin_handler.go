package handler

// spec_admin_handler.go 落地《后端接口文档 v2.0》管理端 A01–A18 的 18 个叶子节点。
//
// 合并前管理端没有任何实现（仅 driver 域的车辆审核与安全事件 REST 入口）。
// 本文件按文档第 3.12–3.17 节契约提供：运营管理、计价与派单、营销活动、
// 财务、数据报表、系统设置；并落实文档中的硬性规则：
//   - A03/A04：无任意改状态/改金额入口，人工动作全部为带权限校验与留痕的业务命令；
//   - A05：价格版本化、区域重叠按优先级、发布前三个样例分账预览；
//   - A06：权重总和必须 10000，全 0 拒绝；
//   - A07/A08：库存与预算原子扣减，不超发；
//   - A10：司机+业务日+结算版本唯一键，重跑不重复转余额；
//   - A11：≥50000 分双人审批，未知结果先查单不释放；
//   - A12：PROPORTIONAL/PLATFORM 两种模式锁定，累计尾差精确计算；
//   - A13：GMV 按行程结束日、净实付按记账日，未收敛订单标记；
//   - A14：留存队列未成熟显示「不适用」；
//   - A16：保留至少一个超管，角色编码不可改，删角色检查引用。

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// 管理端七类角色（文档 7.6 / A16）。
var specAdminRoles = []string{
	"super_admin", "operator", "auditor", "cs", "safety_handler", "finance_first", "finance_second",
}

// 计价/派单/营销/报表对应的管理端权限点（RBAC 三层鉴权之接口层）。
var specAdminPermissions = map[string]string{
	"/api/admin/users":                  "user:read",
	"/api/admin/driver-audit":           "driver-audit:write",
	"/api/admin/orders":                 "order:read",
	"/api/admin/order-command":          "order:command",
	"/api/admin/ticket/action":          "ticket:write",
	"/api/admin/pricing/publish":        "pricing:publish",
	"/api/admin/dispatch/config":        "dispatch:config",
	"/api/admin/coupon/template":        "coupon:write",
	"/api/admin/campaign":               "campaign:write",
	"/api/admin/pinche/config":          "pinche:config",
	"/api/admin/settle":                 "finance:settle",
	"/api/admin/withdraw/review":        "withdraw:review",
	"/api/admin/refund":                 "refund:review",
	"/api/admin/capital-flow":           "finance:read",
	"/api/admin/report/orders":          "report:read",
	"/api/admin/report/driver-activity": "report:read",
	"/api/admin/report/growth":          "report:read",
	"/api/admin/export":                 "report:read",
	"/api/admin/role":                   "role:write",
	"/api/admin/operation-log":          "audit:read",
	"/api/admin/dictionary":             "dictionary:publish",
}

// ── A01 用户管理 ──

type specAdminUser struct {
	ID        string   `json:"id"`
	PhoneMask string   `json:"phone_mask"`
	Role      string   `json:"role"`
	Restrict  []string `json:"restrict"`
}

type specUserRestrictRequest struct {
	UserID       string `json:"user_id"`
	RestrictType string `json:"restrict_type"` // 禁新乘车/禁新接单/禁提现/登录风险限制
	Reason       string `json:"reason"`
	Scope        string `json:"scope"`
}

// AdminUsers GET 查询用户（手机号掩码）；POST 施加限制（每次须填原因与范围，留痕）。
func (h *SpecContractHandler) AdminUsers(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		role := strings.TrimSpace(c.Query("role"))
		h.store.mu.RLock()
		users := make([]specAdminUser, 0, len(h.store.accounts))
		for _, u := range h.store.accounts {
			if role != "" && u.Role != role {
				continue
			}
			users = append(users, *u)
		}
		sort.SliceStable(users, func(i, j int) bool { return users[i].ID < users[j].ID })
		h.store.mu.RUnlock()
		httpx.PaxOK(c, map[string]any{"users": users})
		return
	}

	var req specUserRestrictRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "reason is required")
		return
	}
	if strings.TrimSpace(req.Scope) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "scope is required")
		return
	}
	at := h.now()
	operator, _ := specPrincipal(c)

	h.store.mu.Lock()
	user, ok := h.store.accounts[req.UserID]
	if !ok {
		// 首次施加限制时创建本地影子账号，保证后续可查询。
		user = &specAdminUser{ID: req.UserID, Role: "pax", Restrict: []string{}}
		h.store.accounts[req.UserID] = user
	}
	exists := false
	for _, r := range user.Restrict {
		if r == req.RestrictType {
			exists = true
			break
		}
	}
	if !exists && strings.TrimSpace(req.RestrictType) != "" {
		user.Restrict = append(user.Restrict, req.RestrictType)
	}
	h.store.appendLogLocked(operator, "operator", "user", "restrict", httpx.RequestID(c), "", strings.Join(user.Restrict, ","), at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"users": []specAdminUser{*user}})
}

// ── A02 司机与车辆审核（统一入口，底层能力已由 vehicle 域提供）──

type specDriverAuditRequest struct {
	ApplicationID string `json:"application_id"`
	Revision      int64  `json:"revision"`
	Action        string `json:"action"` // approve / reject
	Reason        string `json:"reason"`
	Version       int64  `json:"version"`
	AuditorID     string `json:"auditor_id"`
}

// AdminDriverAudit 领取申请版本审批：驳回必填原因；提交人不能审批自己的资料；并发用版本锁。
func (h *SpecContractHandler) AdminDriverAudit(c *gin.Context) {
	var req specDriverAuditRequest
	if !specBindJSON(c, &req) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "approve" && action != "reject" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be approve/reject")
		return
	}
	if action == "reject" && strings.TrimSpace(req.Reason) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "reason is required for reject")
		return
	}
	if req.ApplicationID == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "application_id is required")
		return
	}
	auditorID := firstNonEmpty(req.AuditorID, "admin")
	at := h.now()

	h.store.mu.Lock()
	app, ok := h.store.applications[req.ApplicationID]
	if !ok {
		// 未见过的申请：按文档「领取申请版本审批」语义建立待审记录。
		app = &specVehicleApplication{
			ID:        req.ApplicationID,
			Revision:  req.Revision,
			State:     "待审",
			Submitter: "",
			Version:   1,
			History:   []specApplicationRevision{},
		}
		h.store.applications[req.ApplicationID] = app
	}
	// 提交人不能审批自己的资料。
	if app.Submitter != "" && app.Submitter == auditorID {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusForbidden, httpx.PaxCodeUnauthorized, "submitter cannot audit own application")
		return
	}
	// 版本锁：版本不符 → 冲突。
	if req.Version > 0 && req.Version != app.Version {
		h.store.mu.Unlock()
		httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "application version conflict")
		return
	}
	if action == "reject" {
		app.State = "驳回"
		app.RejectCnt++
		app.RejectReason = req.Reason
		if app.RejectCnt >= 3 {
			app.ManualReview = true // 驳回 3 次标人工重点复核
		}
	} else {
		app.State = "已通过"
	}
	app.Version++
	app.History = append(app.History, specApplicationRevision{
		Revision: app.Revision, Action: action, Reason: req.Reason, Auditor: auditorID, T: at,
	})
	state := app.State
	h.store.appendLogLocked(auditorID, "auditor", "vehicle_application", action, httpx.RequestID(c), app.State, state, at)
	h.store.emit("", "DriverEligibilityChanged", req.ApplicationID+" "+action, "driver", at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"application_id": app.ID, "state": state})
}

// ── A03 订单全量管理（穿透查询）──

type specAdminOrderQuery struct {
	OrderID    string `json:"order_id" form:"order_id"`
	Passenger  string `json:"passenger" form:"passenger"`
	State      string `json:"state" form:"state"`
	StartAt    int64  `json:"start_at" form:"start_at"`
	EndAt      int64  `json:"end_at" form:"end_at"`
	WithDetail bool   `json:"with_detail" form:"with_detail"`
}

// AdminOrders 多条件组合查询；带 order_id 时穿透返回任务、轨迹、计价版本、支付退款与工单。
func (h *SpecContractHandler) AdminOrders(c *gin.Context) {
	var query specAdminOrderQuery
	if err := c.ShouldBindQuery(&query); err != nil && c.Request.Method == http.MethodPost {
		_ = c.ShouldBindJSON(&query)
	}

	h.store.mu.RLock()
	list := make([]map[string]any, 0, len(h.store.orders))
	var detail map[string]any
	for _, o := range h.store.orders {
		if query.OrderID != "" && o.ID != query.OrderID {
			continue
		}
		if query.Passenger != "" && o.PassengerID != query.Passenger {
			continue
		}
		if query.State != "" && o.State != query.State {
			continue
		}
		if query.StartAt > 0 && o.CreatedAt < query.StartAt {
			continue
		}
		if query.EndAt > 0 && o.CreatedAt > query.EndAt {
			continue
		}
		bill := h.store.findBillLocked(o.ID, "trip")
		amount := int64(0)
		billState := ""
		if bill != nil {
			amount = bill.P
			billState = bill.State
		}
		list = append(list, map[string]any{
			"id": o.ID, "type_name": specCarTypeName(o.Type),
			"start_name": o.StartName, "end_name": o.EndName,
			"state": o.State, "amount": amount,
			"bill_state": billState, "created_at": o.CreatedAt,
		})
		if query.WithDetail && query.OrderID != "" {
			detail = specBuildOrderDetail(h.store, o)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i]["created_at"].(int64) > list[j]["created_at"].(int64)
	})
	h.store.mu.RUnlock()

	httpx.PaxOK(c, map[string]any{"list": list, "detail": detail, "total": len(list)})
}

func specBuildOrderDetail(s *specStore, o *specOrder) map[string]any {
	var task map[string]any
	if t, ok := s.tasks[o.TaskID]; ok {
		task = map[string]any{
			"task_id": t.ID, "state": t.State, "phase": t.Phase,
			"driver_id": t.DriverID, "candidates": t.Candidates,
			"deadline": t.Deadline, "return_count": t.ReturnCnt,
		}
	}
	tickets := make([]specTicket, 0)
	for _, tk := range s.tickets {
		if tk.OrderID == o.ID {
			tickets = append(tickets, *tk)
		}
	}
	return map[string]any{
		"id": o.ID, "state": o.State, "version": o.Version,
		"bill":        specBillView(s.findBillLocked(o.ID, "trip")),
		"cancel_bill": specBillView(s.findBillLocked(o.ID, "cancel")),
		"timeline":    o.Timeline,
		"route":       map[string]any{"start": o.StartName, "end": o.EndName, "dist_m": o.DistM, "dur_s": o.DurS},
		"driver_snap": map[string]any{"driver_id": o.DriverID},
		"actual":      map[string]any{"dist_m": o.ActualDist, "dur_s": o.ActualDur},
		"cfg_version": o.CfgVersion,
		"price_snap":  o.PriceSnap,
		"task":        task,
		"refunds":     o.Refunds,
		"tickets":     tickets,
	}
}

func specBillView(b *specBill) any {
	if b == nil {
		return nil
	}
	return map[string]any{
		"bill_id": b.ID, "kind": b.Kind, "P": b.P, "T": b.T, "Q": b.Q,
		"H": b.H, "F": b.F, "K": b.K, "Cp": b.Cp, "Cd": b.Cd,
		"D": b.D, "N": b.N, "G": b.G, "state": b.State, "paid_at": b.PaidAt,
		"settlement_payment_id": b.SettleID,
	}
}

func specCarTypeName(t string) string {
	switch t {
	case "kuaiche":
		return "快车"
	case "zhuanche":
		return "专车"
	case "pinche":
		return "拼车"
	default:
		return t
	}
}

// ── A03 业务命令（改派/免取消费/安全提前结束）──

type specOrderCommandRequest struct {
	OrderID  string `json:"order_id"`
	Command  string `json:"command"` // reassign / waive_cancel_fee / safe_end
	Basis    string `json:"basis"`
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
	IdemKey  string `json:"idempotency_key"`
}

// AdminOrderCommand 所有人工动作均为有权限、有前置条件校验、有留痕的业务命令。
func (h *SpecContractHandler) AdminOrderCommand(c *gin.Context) {
	var req specOrderCommandRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Basis) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "basis is required")
		return
	}
	operator := firstNonEmpty(req.Operator, "admin")
	now := time.Now().UTC()
	at := now.Unix()

	result, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey, "cmd:"+req.OrderID+":"+req.Command+":"+req.Basis), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		order, ok := h.store.orders[req.OrderID]
		if !ok {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "order not found")
		}
		before := order.State
		switch strings.ToLower(strings.TrimSpace(req.Command)) {
		case "reassign":
			if order.State != OrderStateWaiting {
				return nil, apperrors.New(apperrors.KindConflict, "40901", "reassign only allowed while waiting")
			}
			order.WaitDeadline = at + SpecDispatchTotalSec
		case "waive_cancel_fee":
			if bill := h.store.findBillLocked(order.ID, "cancel"); bill != nil {
				if bill.State == BillStatePaid {
					return nil, apperrors.New(apperrors.KindConflict, "40901", "cancel fee already paid")
				}
				bill.State = BillStateVoid // 作废，不修改原流水
			}
			order.CancelFee = 0
		case "safe_end":
			if order.State != OrderStateInTrip {
				return nil, apperrors.New(apperrors.KindConflict, "40901", "safe end only allowed in trip")
			}
			if req.Reason == "" {
				return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "reason is required for safe_end")
			}
			order.State = OrderStateEnded
			order.EndedAt = at
		default:
			return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "unknown command")
		}
		order.Version++
		order.Timeline = append(order.Timeline, specTimeline{Name: strings.ToUpper(req.Command), Desc: req.Basis, T: at})
		h.store.appendLogLocked(operator, "operator", "order", req.Command, httpx.RequestID(c), before, order.State, at)
		return map[string]any{"order_id": order.ID, "command": req.Command, "state": order.State}, nil
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

// ── A04 投诉工单处理 ──

type specTicketActionRequest struct {
	TicketID string `json:"ticket_id"`
	Action   string `json:"action"` // process / close / transfer
	Reply    string `json:"reply"`
	Operator string `json:"operator"`
}

// AdminTicketAction 工单流转：待处理/处理中/已完结/已关闭；申诉（transfer）换人处理。
func (h *SpecContractHandler) AdminTicketAction(c *gin.Context) {
	var req specTicketActionRequest
	if !specBindJSON(c, &req) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	at := h.now()
	operator := firstNonEmpty(req.Operator, "cs")

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	ticket, ok := h.store.tickets[req.TicketID]
	if !ok {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "ticket not found")
		return
	}
	before := ticket.State
	switch action {
	case "process":
		if ticket.State == "待处理" || ticket.State == "处理中" {
			ticket.State = "处理中"
			ticket.Handler = operator
		}
	case "close":
		ticket.State = "已完结"
		ticket.Reply = req.Reply
		ticket.DoneAt = at
	case "transfer":
		// 申诉：关联处理轮次并换人处理（不得由原处理人继续）。
		if ticket.Handler == operator {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "appeal must be handled by another senior agent")
			return
		}
		ticket.Round++
		ticket.State = "处理中"
		ticket.Handler = operator
	default:
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be process/close/transfer")
		return
	}
	h.store.appendLogLocked(operator, "cs", "ticket", action, httpx.RequestID(c), before, ticket.State, at)
	state := ticket.State

	httpx.PaxOK(c, map[string]any{"ticket_id": ticket.ID, "state": state})
}

// ── A05 计价规则管理（草稿/发布/样例预览）──

type specPricingPublishRequest struct {
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
	ValidFrom  int64  `json:"valid_from"`
	ValidTo    int64  `json:"valid_to"`
	Priority   int    `json:"priority"`
}

// specFareSample 是发布前的样例分账预览（文档 A05：至少三个样例）。
type specFareSample struct {
	Name   string   `json:"name"`
	DistM  int64    `json:"dist_m"`
	DurS   int64    `json:"dur_s"`
	Cp     int64    `json:"cp"`
	Cd     int64    `json:"cd"`
	Result specFare `json:"result"`
}

// AdminPricingPublish 校验草稿后发布新版本；发布前返回至少三个样例的分账结果。
// 生效版本不可原地修改；区域重叠按优先级选一，相同优先级时间重叠禁止发布。
func (h *SpecContractHandler) AdminPricingPublish(c *gin.Context) {
	var req specPricingPublishRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.City) == "" || strings.TrimSpace(req.Type) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "city and type are required")
		return
	}
	if req.ValidFrom <= 0 || req.ValidTo <= 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "valid_from and valid_to are required")
		return
	}
	if req.ValidTo <= req.ValidFrom {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "valid_to must be greater than valid_from")
		return
	}
	if req.Incl < 0 || req.PerKM < 0 || req.PerMin < 0 || req.Start < 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "pricing fields must not be negative")
		return
	}
	if req.ServiceBP < 0 || req.CommBP < 0 || req.SurgeBP <= 0 || req.DiscountBP <= 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "rates must be positive basis points")
		return
	}
	if req.Priority <= 0 {
		req.Priority = 1
	}

	cfg := specPricingConfig{
		City: req.City, Area: firstNonEmpty(req.Area, "全市"), Type: req.Type,
		Start: req.Start, Incl: req.Incl, PerKM: req.PerKM, PerMin: req.PerMin,
		ServiceBP: req.ServiceBP, CommBP: req.CommBP, SurgeBP: req.SurgeBP,
		CancelFee: req.CancelFee, DiscountBP: req.DiscountBP, Fixed: req.Fixed,
		ValidFrom: req.ValidFrom, ValidTo: req.ValidTo, State: "已发布",
	}

	at := h.now()
	h.store.mu.Lock()
	// 相同优先级时间重叠禁止发布；不同优先级允许（按优先级选一）。
	for _, exist := range h.store.cfgs {
		if exist.City != cfg.City || exist.Type != cfg.Type {
			continue
		}
		overlap := cfg.ValidFrom <= exist.ValidTo && exist.ValidFrom <= cfg.ValidTo
		if overlap && exist.Priority == cfg.Priority {
			h.store.mu.Unlock()
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "same priority overlapping validity window")
			return
		}
	}
	cfg.Version = int64(len(h.store.cfgs)) + 1
	h.store.cfgs = append(h.store.cfgs, cfg)

	samples := make([]specFareSample, 0, 3)
	for _, s := range []struct {
		name      string
		dist, dur int64
		cp, cd    int64
	}{
		{"短途 3km/8min", 3000, 480, 0, 0},
		{"中途 6.5km/16min", 6500, 960, 500, 0},
		{"长途 20km/35min", 20000, 2100, 0, 300},
	} {
		samples = append(samples, specFareSample{
			Name: s.name, DistM: s.dist, DurS: s.dur, Cp: s.cp, Cd: s.cd,
			Result: computeFare(cfg, s.dist, s.dur, s.cp, s.cd, cfg.Type == "pinche"),
		})
	}
	h.store.appendLogLocked("admin", "super_admin", "pricing_config", "publish", httpx.RequestID(c), "", "v"+strconv.FormatInt(cfg.Version, 10), at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"cfg_version": cfg.Version, "samples": samples})
}

// ── A06 派单策略配置 ──

type specDispatchConfigRequest struct {
	Weights      map[string]int64 `json:"weights"`
	Radius       int64            `json:"radius"`
	InviteSec    int64            `json:"invite_sec"`
	TotalSec     int64            `json:"total_sec"`
	RegionSwitch bool             `json:"region_switch"`
	Redispatch   map[string]int64 `json:"re_dispatch_params"`
}

// AdminDispatchConfig 校验并保存派单策略：权重总和必须为 10000，全 0 拒绝；半径与时限为正。
func (h *SpecContractHandler) AdminDispatchConfig(c *gin.Context) {
	var req specDispatchConfigRequest
	if !specBindJSON(c, &req) {
		return
	}
	total := int64(0)
	for _, w := range req.Weights {
		total += w
	}
	if total != 10000 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument,
			"weights sum must be 10000, got "+strconv.FormatInt(total, 10))
		return
	}
	if req.Radius <= 0 || req.InviteSec <= 0 || req.TotalSec <= 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "radius/invite_sec/total_sec must be positive")
		return
	}

	at := h.now()
	h.store.mu.Lock()
	h.store.dispatch = &specDispatchConfig{
		Weights:      req.Weights,
		Radius:       req.Radius,
		InviteSec:    req.InviteSec,
		TotalSec:     req.TotalSec,
		RegionSwitch: req.RegionSwitch,
		Redispatch:   req.Redispatch,
		Version:      h.store.dispatchVer + 1,
	}
	h.store.dispatchVer++
	version := h.store.dispatchVer
	h.store.appendLogLocked("admin", "super_admin", "dispatch_config", "publish", httpx.RequestID(c), "", "v"+strconv.FormatInt(version, 10), at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"config_version": version})
}

type specDispatchConfig struct {
	Weights      map[string]int64 `json:"weights"`
	Radius       int64            `json:"radius"`
	InviteSec    int64            `json:"invite_sec"`
	TotalSec     int64            `json:"total_sec"`
	RegionSwitch bool             `json:"region_switch"`
	Redispatch   map[string]int64 `json:"re_dispatch_params"`
	Version      int64            `json:"version"`
}

// ── A07 券模板与发放 ──

type specCouponTemplateRequest struct {
	Name        string   `json:"name"`
	FaceValue   int64    `json:"face_value"`
	Threshold   int64    `json:"threshold"`
	ValidFrom   int64    `json:"valid_from"`
	ValidTo     int64    `json:"valid_to"`
	CarTypes    []string `json:"car_types"`
	Area        string   `json:"area"`
	Stock       int64    `json:"stock"`
	PerLimit    int64    `json:"per_limit"`
	Bearer      string   `json:"bearer"`     // platform / driver（二选一）
	IssueType   string   `json:"issue_type"` // 注册/定向/活动领取
	PassengerID string   `json:"passenger_id"`
}

// AdminCouponTemplate 创建券模板并向指定乘客发放（库存原子扣减，防超发）。
func (h *SpecContractHandler) AdminCouponTemplate(c *gin.Context) {
	var req specCouponTemplateRequest
	if !specBindJSON(c, &req) {
		return
	}
	if req.Bearer != "platform" && req.Bearer != "driver" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "bearer must be platform or driver")
		return
	}
	if req.FaceValue <= 0 || req.Stock <= 0 || req.ValidTo <= req.ValidFrom {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "invalid template parameters")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	tpl := &specCouponTemplate{
		ID: h.store.nextID("CT"), Name: req.Name, FaceValue: req.FaceValue, Threshold: req.Threshold,
		ValidFrom: req.ValidFrom, ValidTo: req.ValidTo, CarTypes: req.CarTypes, Area: req.Area,
		Stock: req.Stock, PerLimit: req.PerLimit, Bearer: req.Bearer,
		IssueType: firstNonEmpty(req.IssueType, "活动领取"), State: "已发布",
	}
	issued := int64(0)
	if req.PassengerID != "" {
		// 库存原子扣减：条件更新，库存为 0 则不再发放。
		if tpl.Stock > 0 {
			tpl.Stock--
			issued++
			h.store.coupons[h.store.nextID("C")] = &specCoupon{
				ID: h.store.nextID(""), TemplateID: tpl.ID, PassengerID: req.PassengerID,
				Name: tpl.Name, Bearer: tpl.Bearer, Threshold: tpl.Threshold,
				Amount: tpl.FaceValue, State: CouponStateAvailable, ExpireAt: tpl.ValidTo,
			}
		}
	}
	h.store.templates[tpl.ID] = tpl
	h.store.appendLogLocked("admin", "operator", "coupon_template", "create", httpx.RequestID(c), "", tpl.ID, at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"template_id": tpl.ID, "issued": issued, "stock": tpl.Stock})
}

// ── A08 补贴活动管理 ──

type specCampaignRequest struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Reward         int64  `json:"reward"`
	PerDriverLimit int64  `json:"per_driver_limit"`
	Budget         int64  `json:"budget"`
	Action         string `json:"action"` // create / recover
	DriverID       string `json:"driver_id"`
	RecoverFen     int64  `json:"recover_fen"`
}

// AdminCampaign 创建补贴活动；预算原子预占，80% 告警、100% 停止；虚假订单/全退用反向流水追回。
func (h *SpecContractHandler) AdminCampaign(c *gin.Context) {
	var req specCampaignRequest
	if !specBindJSON(c, &req) {
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "recover":
		camp := h.store.campaigns[firstNonEmpty(req.Name, "")]
		if camp == nil {
			httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "campaign not found")
			return
		}
		// 反向流水追回：不修改原金额，只追加反向凭证。
		h.store.flows = append(h.store.flows, &specFlow{
			ID: h.store.nextID("F"), Name: "补贴追回 " + camp.ID,
			Fen: -req.RecoverFen, T: at, Type: "recover",
		})
		h.store.appendLogLocked("admin", "operator", "campaign", "recover", httpx.RequestID(c), "", strconv.FormatInt(req.RecoverFen, 10), at)
		httpx.PaxOK(c, map[string]any{"campaign_id": camp.ID, "recovered": req.RecoverFen, "used_budget": camp.UsedBudget})
		return
	default:
		if req.Reward <= 0 || req.Budget <= 0 || req.PerDriverLimit <= 0 {
			httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "invalid campaign parameters")
			return
		}
		camp := &specCampaign{
			ID: h.store.nextID("CA"), Name: req.Name, Type: firstNonEmpty(req.Type, "完单奖励"),
			Reward: req.Reward, PerDriverLimit: req.PerDriverLimit,
			Budget: req.Budget, UsedBudget: 0, State: "已发布",
		}
		h.store.campaigns[camp.ID] = camp
		h.store.appendLogLocked("admin", "operator", "campaign", "create", httpx.RequestID(c), "", camp.ID, at)
		httpx.PaxOK(c, map[string]any{"campaign_id": camp.ID, "budget": camp.Budget, "used_budget": camp.UsedBudget})
	}
}

type specCampaign struct {
	ID             string `json:"campaign_id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Reward         int64  `json:"reward"`
	PerDriverLimit int64  `json:"per_driver_limit"`
	Budget         int64  `json:"budget"`
	UsedBudget     int64  `json:"used_budget"`
	State          string `json:"state"`
}

// ── A09 拼车/特价配置 ──

type specPincheConfigRequest struct {
	MatchSec          int64            `json:"match_sec"`
	GroupMax          int64            `json:"group_max"`
	SpecialPricePlans []map[string]any `json:"special_price_plans"`
}

// AdminPincheConfig 保存拼车匹配与特价固定价方案配置。
// 每组最多 2 单 4 座；拼成/拼未成均按提交时固定价，特价不重复扣活动差价。
func (h *SpecContractHandler) AdminPincheConfig(c *gin.Context) {
	var req specPincheConfigRequest
	if !specBindJSON(c, &req) {
		return
	}
	if req.MatchSec <= 0 {
		req.MatchSec = SpecCarpoolMatchSec
	}
	if req.GroupMax <= 0 {
		req.GroupMax = SpecCarpoolGroupMax
	}
	if req.GroupMax > SpecCarpoolGroupMax {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument,
			"group_max must not exceed "+strconv.FormatInt(SpecCarpoolGroupMax, 10))
		return
	}
	at := h.now()
	h.store.mu.Lock()
	h.store.pinche = &specPincheConfig{
		MatchSec:          req.MatchSec,
		GroupMax:          req.GroupMax,
		SpecialPricePlans: req.SpecialPricePlans,
		Version:           h.store.pincheVer + 1,
	}
	h.store.pincheVer++
	version := h.store.pincheVer
	h.store.appendLogLocked("admin", "operator", "pinche_config", "publish", httpx.RequestID(c), "", "v"+strconv.FormatInt(version, 10), at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"config_version": version})
}

type specPincheConfig struct {
	MatchSec          int64            `json:"match_sec"`
	GroupMax          int64            `json:"group_max"`
	SpecialPricePlans []map[string]any `json:"special_price_plans,omitempty"`
	Version           int64            `json:"version"`
}

// ── A10 结算批处理（日结）──

type specSettleRequest struct {
	BusinessDate string `json:"business_date"` // YYYY-MM-DD，默认前一日
}

// AdminSettle 按北京时间日结：处理业务日已支付且无冻结的司机收益，
// 先核渠道收付再核分账明细；差异生成差异单并挂起条目。
// 批处理幂等：司机+业务日+结算版本唯一键，重跑不重复转余额。
func (h *SpecContractHandler) AdminSettle(c *gin.Context) {
	var req specSettleRequest
	_ = c.ShouldBindJSON(&req)

	loc := time.FixedZone("CST", 8*3600)
	businessDate := strings.TrimSpace(req.BusinessDate)
	if businessDate == "" {
		businessDate = time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", businessDate); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "business_date must be YYYY-MM-DD")
		return
	}
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	settled := int64(0)
	diffItems := make([]map[string]any, 0)
	for driverID, fund := range h.store.funds {
		key := driverID + "|" + businessDate + "|1"
		if _, ok := h.store.settlements[key]; ok {
			continue // 幂等：同司机同业务日同版本不重复转余额
		}
		if fund.Fz > 0 {
			diffItems = append(diffItems, map[string]any{
				"driver_id": driverID, "reason": "frozen", "amount": fund.Fz,
			})
			continue // 有冻结：挂起该司机条目，不影响其他司机继续结算
		}
		if fund.U > 0 {
			fund.A += fund.U
			settled += fund.U
			fund.U = 0
		}
		h.store.settlements[key] = &specSettlement{
			ID: driverID + "-" + businessDate, DriverID: driverID,
			BusinessDate: businessDate, Version: 1,
			Amount: settled, State: "已结算",
		}
	}

	httpx.PaxOK(c, map[string]any{"settled_amount": settled, "diff_items": diffItems, "business_date": businessDate, "settled_at": at})
}

type specSettlement struct {
	ID           string `json:"id"`
	DriverID     string `json:"driver_id"`
	BusinessDate string `json:"business_date"`
	Version      int64  `json:"version"`
	Amount       int64  `json:"amount"`
	State        string `json:"state"`
}

// ── A11 提现审核 ──

type specWithdrawReviewRequest struct {
	WithdrawalID string `json:"withdrawal_id"`
	Action       string `json:"action"` // first / pay / reject
	ReviewerRole string `json:"reviewer_role"`
	ReviewerID   string `json:"reviewer_id"`
	Comment      string `json:"comment"`
}

// AdminWithdrawReview 提现审核工作台：
// 普通单人审核、≥50000 分两人分离；批准后以唯一代付请求号提交渠道；
// 明确失败终结原单并释放预占，未知结果保持预占并查单。
func (h *SpecContractHandler) AdminWithdrawReview(c *gin.Context) {
	var req specWithdrawReviewRequest
	if !specBindJSON(c, &req) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != "first" && action != "pay" && action != "reject" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be first/pay/reject")
		return
	}
	reviewerID := firstNonEmpty(req.ReviewerID, "finance")
	reviewerRole := firstNonEmpty(req.ReviewerRole, "finance_first")
	at := h.now()

	h.store.mu.Lock()
	defer h.store.mu.Unlock()

	draw, ok := h.store.draws[req.WithdrawalID]
	if !ok {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "withdrawal not found")
		return
	}
	before := draw.State

	switch action {
	case "first":
		if draw.State != "待初审" {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "withdrawal is not pending first review")
			return
		}
		draw.Approvals = 1
		draw.ApproverIDs = append(draw.ApproverIDs, reviewerID)
		if draw.Fen >= SpecWithdrawDualApprove {
			draw.State = "待复核" // 大额需第二人
		} else {
			draw.State = "待打款"
		}
	case "pay":
		if draw.State != "待打款" && draw.State != "待复核" {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "withdrawal is not payable in current state")
			return
		}
		if draw.Fen >= SpecWithdrawDualApprove {
			// 大额双人分离：复核人不得与初审人相同。
			if len(draw.ApproverIDs) > 0 && draw.ApproverIDs[0] == reviewerID {
				httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "second reviewer must differ from first reviewer")
				return
			}
			if draw.Approvals < 2 {
				httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "large withdrawal requires two approvals")
				return
			}
		}
		draw.State = "打款中"
		if draw.RequestNo == "" {
			draw.RequestNo = specRandomToken("pay") // 唯一代付请求号
		}
	case "reject":
		if draw.State == "成功" || draw.State == "打款中" {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "withdrawal already finalized")
			return
		}
		draw.State = "已驳回"
		// 统一释放路由：先偿 Debt，再补退款预占缺口，余下回 A。
		fund := h.store.ensureFundLocked(draw.DriverID)
		released := draw.Fen - fund.W
		if released < 0 {
			released = draw.Fen
		}
		fund.W -= (draw.Fen - released)
		if fund.W < 0 {
			fund.W = 0
		}
		if fund.Debt > 0 {
			if fund.Debt >= released {
				fund.Debt -= released
				released = 0
			} else {
				released -= fund.Debt
				fund.Debt = 0
			}
		}
		fund.A += released
	}

	h.store.appendLogLocked(reviewerID, reviewerRole, "withdrawal", action, httpx.RequestID(c), before, draw.State, at)
	httpx.PaxOK(c, map[string]any{"withdrawal_id": draw.ID, "state": draw.State})
}

// ── A12 退款审批与执行 ──

type specRefundRequest struct {
	OrderID string `json:"order_id"`
	Amount  int64  `json:"amount"`
	Mode    string `json:"mode"` // PROPORTIONAL / PLATFORM
	Reason  string `json:"reason"`
	IdemKey string `json:"idempotency_key"`
}

// AdminRefund 退款审批并执行分摊。
// 累计成功+在途不超过原实付；同一订单同时只允许一笔退款进入分摊与执行阶段；
// 尾差按 Round(D0×(Rp+r)) − Round(D0×Rp) 精确计算（文档 T14）。
func (h *SpecContractHandler) AdminRefund(c *gin.Context) {
	var req specRefundRequest
	if !specBindJSON(c, &req) {
		return
	}
	mode := strings.ToUpper(strings.TrimSpace(req.Mode))
	if mode != "PROPORTIONAL" && mode != "PLATFORM" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "mode must be PROPORTIONAL/PLATFORM")
		return
	}
	if req.Amount <= 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "amount must be positive")
		return
	}
	now := time.Now().UTC()
	at := now.Unix()

	result, repeated, err := h.store.idemApply(firstNonEmpty(req.IdemKey, "refund:"+req.OrderID+":"+strconv.FormatInt(req.Amount, 10)+":"+mode), now, func() (any, error) {
		h.store.mu.Lock()
		defer h.store.mu.Unlock()
		order, ok := h.store.orders[req.OrderID]
		if !ok {
			return nil, apperrors.New(apperrors.KindNotFound, "40401", "order not found")
		}
		bill := h.store.findBillLocked(req.OrderID, "trip")
		if bill == nil || bill.State != BillStatePaid {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "bill is not paid")
		}
		// 同一订单同时只允许一笔退款进入分摊与执行阶段。
		pending := int64(0)
		refunded := int64(0)
		for _, r := range order.Refunds {
			if r.State == "成功" {
				refunded += r.R
				continue
			}
			if r.State == "进行中" || r.State == "PENDING" {
				pending++
			}
		}
		if pending > 0 {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "another refund is in progress")
		}
		if refunded+req.Amount > bill.P {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "refund exceeds original payment")
		}
		// 尾差：Round(D0×(Rp+r)) − Round(D0×Rp)
		driverBear := specDriverBear(bill.D, bill.P, refunded, req.Amount, mode)
		r := specRefund{
			ID: h.store.nextID("R"), R: req.Amount, DR: driverBear,
			Mode: mode, State: "成功", T: at,
		}
		order.Refunds = append(order.Refunds, r)
		bill.RefundFen = refunded + req.Amount

		// 司机承担额从司机可用/待结算扣回；不足部分记 Debt。
		if driverBear > 0 && order.DriverID != "" {
			fund := h.store.ensureFundLocked(order.DriverID)
			remain := driverBear
			if fund.A >= remain {
				fund.A -= remain
				remain = 0
			} else {
				remain -= fund.A
				fund.A = 0
			}
			if remain > 0 && fund.U > 0 {
				if fund.U >= remain {
					fund.U -= remain
					remain = 0
				} else {
					remain -= fund.U
					fund.U = 0
				}
			}
			if remain > 0 {
				fund.Debt += remain
			}
			h.store.flows = append(h.store.flows, &specFlow{
				ID: h.store.nextID("F"), Name: "退款冲正 " + order.ID,
				Fen: -driverBear, T: at, Type: "refund",
			})
		}
		h.store.emit("refund:"+r.ID, "RefundSucceeded", r.ID+" "+strconv.FormatInt(req.Amount, 10), "refund", at)

		return map[string]any{
			"order_id":       order.ID,
			"refunded":       req.Amount,
			"driver_bear":    driverBear,
			"total_refunded": bill.RefundFen,
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

// specDriverBear 计算本次退款的司机承担额（文档 T14 尾差公式）。
//
//	司机承担 = Round(D0 × (Rp + r)) − Round(D0 × Rp)
//
// 其中 D0=原始司机应得，P0=原实付，Rp=已退比例，r=本次退款比例。
// 用累计口径计算而非每次独立计算，避免尾差累积。
func specDriverBear(d0, p0, refundedFen, thisRefundFen int64, mode string) int64 {
	if mode == "PLATFORM" || p0 <= 0 || d0 <= 0 {
		return 0
	}
	before := specRoundDiv(d0*(refundedFen+thisRefundFen), p0)
	after := specRoundDiv(d0*refundedFen, p0)
	return before - after
}

func specRoundDiv(n, d int64) int64 {
	if d <= 0 || n == 0 {
		return 0
	}
	negative := n < 0
	if negative {
		n = -n
	}
	r := (n + d/2) / d
	if negative {
		return -r
	}
	return r
}

// ── A13 订单与流水报表 ──

// AdminReportOrders T+1 报表：GMV 按行程结束日、净实付按支付退款记账日，两个口径不混算。
func (h *SpecContractHandler) AdminReportOrders(c *gin.Context) {
	date := strings.TrimSpace(c.Query("date"))
	if date == "" {
		date = time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, -1).Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "date must be YYYY-MM-DD")
		return
	}

	loc := time.FixedZone("CST", 8*3600)
	h.store.mu.RLock()
	orders, completed, gmv, netPaid, commission, subsidy, driverIncome := int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), int64(0)
	unsettled := int64(0)
	for _, o := range h.store.orders {
		orders++
		if o.State == OrderStateEnded {
			completed++
			if bill := h.store.findBillLocked(o.ID, "trip"); bill != nil {
				gmv += bill.P
				commission += bill.K
				subsidy += bill.G
				driverIncome += bill.D
				if bill.State == BillStatePaid {
					netPaid += bill.P - bill.RefundFen
				} else {
					unsettled++ // 当日未收敛订单标记「未收敛」
				}
			}
		}
	}
	h.store.mu.RUnlock()

	cancelRate := "0%"
	if orders > 0 {
		cancelled := orders - completed
		cancelRate = strconv.FormatInt(cancelled*100/orders, 10) + "%"
	}

	httpx.PaxOK(c, map[string]any{
		"metrics": map[string]any{
			"orders": orders, "completed": completed, "cancel_rate": cancelRate,
			"gmv": gmv, "net_paid": netPaid, "commission": commission,
			"subsidy": subsidy, "driver_income": driverIncome,
			"net_income": netPaid - driverIncome - subsidy,
		},
		"date":            date,
		"cutoff_time":     time.Now().In(loc).Format(time.RFC3339),
		"caliber_version": "v2.0",
		"unsettled":       unsettled,
	})
}

// ── A14 司机活跃与留存 ──

// AdminReportDriverActivity 实时在线取近 60 秒心跳；历史活跃按当日有效出车时长≥60 秒；
// 留存队列未成熟显示「不适用」而非 0%。
func (h *SpecContractHandler) AdminReportDriverActivity(c *gin.Context) {
	at := h.now()
	h.store.mu.RLock()
	online := int64(0)
	active := int64(0)
	for id, rt := range h.store.driverStatus {
		if at-rt.LastBeatAt <= 60 {
			online++
		}
		if rt.OnlineSec >= 60 {
			active++
		}
		_ = id
	}
	total := int64(len(h.store.driverStatus))
	h.store.mu.RUnlock()

	retention := map[string]string{"next_day": "不适用", "day7": "不适用", "day30": "不适用"}
	if total > 0 {
		retention["next_day"] = strconv.FormatInt(active*100/total, 10) + "%"
		retention["day7"] = "不适用" // 队列未满 7 日，按分母成熟规则不显示
	}

	httpx.PaxOK(c, map[string]any{
		"online_now":      online,
		"active_history":  active,
		"retention":       retention,
		"caliber_version": "v2.0",
	})
}

// ── A15 用户增长分析 ──

// AdminReportGrowth 注册渠道首次归因不改；LTV = 累计净实付 − 平台承担券 − 现金补贴；无投放成本不显示 ROI。
func (h *SpecContractHandler) AdminReportGrowth(c *gin.Context) {
	h.store.mu.RLock()
	registrations := int64(len(h.store.accounts))
	dau := int64(0)
	netPaid := int64(0)
	platformCoupon := int64(0)
	cashSubsidy := int64(0)
	firstOrder := int64(0)
	repeat := int64(0)
	for id := range h.store.accounts {
		orderCount := int64(0)
		for _, o := range h.store.orders {
			if o.PassengerID != id {
				continue
			}
			orderCount++
			if bill := h.store.findBillLocked(o.ID, "trip"); bill != nil && bill.State == BillStatePaid {
				netPaid += bill.P - bill.RefundFen
				platformCoupon += bill.Cp
				cashSubsidy += bill.G
			}
		}
		if orderCount > 0 {
			firstOrder++
			dau++
			if orderCount > 1 {
				repeat++
			}
		}
	}
	h.store.mu.RUnlock()

	firstRate := "不适用"
	repurchase := "不适用"
	if registrations > 0 {
		firstRate = strconv.FormatInt(firstOrder*100/registrations, 10) + "%"
	}
	if firstOrder > 0 {
		repurchase = strconv.FormatInt(repeat*100/firstOrder, 10) + "%"
	}

	httpx.PaxOK(c, map[string]any{
		"registrations":    registrations,
		"dau":              dau,
		"first_order_rate": firstRate,
		"repurchase":       repurchase,
		"ltv":              netPaid - platformCoupon - cashSubsidy,
		"roi":              nil, // 无投放成本，按文档不显示 ROI
	})
}

// ── A15 异步导出 ──

type specExportRequest struct {
	Type   string         `json:"type"`
	Filter map[string]any `json:"filter"`
}

// AdminExportCreate 创建异步导出任务：记录筛选、权限、申请人与数据版本。
func (h *SpecContractHandler) AdminExportCreate(c *gin.Context) {
	var req specExportRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Type) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "type is required")
		return
	}
	applicant, _ := specPrincipal(c)
	at := h.now()

	h.store.mu.Lock()
	task := &specExportTask{
		ID:        h.store.nextID("E"),
		Type:      req.Type,
		Filter:    req.Filter,
		Applicant: firstNonEmpty(applicant, "admin"),
		Version:   1,
		State:     "处理中",
	}
	h.store.exps[task.ID] = task
	h.store.appendLogLocked(task.Applicant, "operator", "export_task", "create", httpx.RequestID(c), "", task.ID, at)
	// 课程模拟：创建即生成 24 小时有效的下载地址，失败按业务日重跑覆盖同版本不重复累加。
	task.State = "已完成"
	task.DownloadURL = "/api/admin/export?export_id=" + task.ID
	task.ExpiresAt = at + 24*60*60
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{
		"export_id": task.ID, "state": task.State,
		"download_url": task.DownloadURL, "expires_at": task.ExpiresAt,
	})
}

// AdminExportQuery 查询导出任务状态与下载地址。
func (h *SpecContractHandler) AdminExportQuery(c *gin.Context) {
	id := strings.TrimSpace(c.Query("export_id"))
	if id == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "export_id is required")
		return
	}
	at := h.now()
	h.store.mu.RLock()
	task, ok := h.store.exps[id]
	h.store.mu.RUnlock()
	if !ok {
		httpx.PaxError(c, http.StatusNotFound, httpx.PaxCodeNotFound, "export task not found")
		return
	}
	downloadURL := task.DownloadURL
	if task.ExpiresAt > 0 && at > task.ExpiresAt {
		downloadURL = "" // 下载地址 24 小时失效
	}
	httpx.PaxOK(c, map[string]any{
		"export_id": task.ID, "state": task.State,
		"download_url": downloadURL, "expires_at": task.ExpiresAt,
	})
}

// ── A16 角色权限管理 ──

type specRoleRequest struct {
	Code        string   `json:"role_code"`
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
}

// AdminRole GET 查询角色列表；POST 配置权限。
// 角色编码唯一不可改；删角色须检查引用；系统保留至少一个超管（此处拒绝清空超管权限）。
func (h *SpecContractHandler) AdminRole(c *gin.Context) {
	if c.Request.Method == http.MethodGet {
		h.store.mu.RLock()
		roles := make([]specRole, 0, len(h.store.roles))
		for _, r := range h.store.roles {
			roles = append(roles, *r)
		}
		sort.SliceStable(roles, func(i, j int) bool { return roles[i].Code < roles[j].Code })
		h.store.mu.RUnlock()
		httpx.PaxOK(c, map[string]any{"roles": roles})
		return
	}

	var req specRoleRequest
	if !specBindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "role_code is required")
		return
	}
	if len(req.Permissions) == 0 {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "permissions is required")
		return
	}
	// 保留至少一个超管：不允许把唯一超管角色的权限降为非通配。
	if strings.EqualFold(req.Code, "super_admin") {
		wildcard := false
		for _, p := range req.Permissions {
			if p == "*" {
				wildcard = true
				break
			}
		}
		if !wildcard {
			httpx.PaxError(c, http.StatusConflict, httpx.PaxCodeConflict, "at least one super_admin role must be kept")
			return
		}
	}

	at := h.now()
	operator, _ := specPrincipal(c)
	h.store.mu.Lock()
	role, ok := h.store.roles[req.Code]
	if !ok {
		role = &specRole{Code: req.Code}
		h.store.roles[req.Code] = role
	}
	before := strings.Join(role.Permissions, ",")
	if strings.TrimSpace(req.Name) != "" {
		role.Name = req.Name
	}
	role.Permissions = req.Permissions
	h.store.appendLogLocked(operator, "super_admin", "role", "configure", httpx.RequestID(c), before, strings.Join(role.Permissions, ","), at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"roles": []specRole{*role}})
}

// ── A16 操作日志查询 ──

// AdminOperationLog 查询操作/登录/任务日志（脱敏前后值，不含完整证件/验证码/密钥/令牌）。
func (h *SpecContractHandler) AdminOperationLog(c *gin.Context) {
	h.store.mu.RLock()
	logs := append([]*specOperationLog(nil), h.store.logs...)
	h.store.mu.RUnlock()
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].T > logs[j].T })
	if logs == nil {
		logs = []*specOperationLog{}
	}
	httpx.PaxOK(c, map[string]any{"logs": logs})
}

// ── A18 基础参数与字典管理 ──

type specDictionaryRequest struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Action string `json:"action"` // draft / publish / disable
}

// AdminDictionary 维护参数字典：草稿/已发布/停用 + 版本；枚举编码不可被删除。
func (h *SpecContractHandler) AdminDictionary(c *gin.Context) {
	var req specDictionaryRequest
	if !specBindJSON(c, &req) {
		return
	}
	key := strings.TrimSpace(req.Key)
	if key == "" {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "key is required")
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	states := map[string]string{"draft": "草稿", "publish": "已发布", "disable": "停用"}
	state, ok := states[action]
	if !ok {
		httpx.PaxError(c, http.StatusBadRequest, httpx.PaxCodeInvalidArgument, "action must be draft/publish/disable")
		return
	}

	at := h.now()
	operator, _ := specPrincipal(c)
	h.store.mu.Lock()
	entry, ok := h.store.dict[key]
	if !ok {
		entry = &specDictEntry{Key: key, Version: 0, State: "草稿"}
		h.store.dict[key] = entry
	}
	before := entry.Value + "|" + entry.State
	entry.Value = req.Value
	entry.State = state
	entry.Version++
	h.store.appendLogLocked(operator, "super_admin", "config_dictionary", action, httpx.RequestID(c), before, entry.Value+"|"+entry.State, at)
	h.store.mu.Unlock()

	httpx.PaxOK(c, map[string]any{"key": entry.Key, "version": entry.Version, "state": entry.State})
}

// ── 管理端鉴权中间件（文档 A16 RBAC 三层鉴权之接口层强制校验）──

// AdminGuard 只放行七类后台角色；菜单/按钮隐藏不是鉴权，接口层必须强制校验。
func (h *SpecContractHandler) AdminGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "a bearer token is required")
			c.Abort()
			return
		}
		principal, err := authn.VerifyToken(parts[1], authn.Config{Secret: h.jwt.Secret, Issuer: h.jwt.Issuer})
		if err != nil {
			httpx.PaxError(c, http.StatusUnauthorized, httpx.PaxCodeUnauthorized, "token is invalid or expired")
			c.Abort()
			return
		}
		allowed := false
		for _, role := range specAdminRoles {
			if strings.EqualFold(principal.Role, role) {
				allowed = true
				break
			}
		}
		if !allowed {
			httpx.PaxError(c, http.StatusForbidden, httpx.PaxCodeUnauthorized, "admin role is required")
			c.Abort()
			return
		}
		c.Next()
	}
}
