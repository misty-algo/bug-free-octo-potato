package handler

import (
	"strconv"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// SafetyHandler 是司机安全事件入口。
// 受理成功才返回 event_id 与 received_at；失败一律不带编号。
type SafetyHandler struct {
	client driverv1.SafetyServiceClient
	auth   authn.Config
}

func NewSafetyHandler(c driverv1.SafetyServiceClient, cfg authn.Config) *SafetyHandler {
	return &SafetyHandler{client: c, auth: cfg}
}

func (h *SafetyHandler) DriverGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "driver")
}

func (h *SafetyHandler) StaffGuard() gin.HandlerFunc {
	return materialGuard(h.auth, "auditor", "safety_handler")
}

// ReportEvent: POST /api/drv/safety/event
func (h *SafetyHandler) ReportEvent(c *gin.Context) {
	var r struct {
		ClientRequestID string  `json:"client_request_id"`
		EventType       string  `json:"event_type"`
		Description     string  `json:"description"`
		TaskID          string  `json:"task_id"`
		HasPosition     bool    `json:"has_position"`
		Lng             float64 `json:"lng"`
		Lat             float64 `json:"lat"`
		CapturedAt      int64   `json:"captured_at"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ReportEvent(ctx, &driverv1.ReportEventRequest{
		ClientRequestId: r.ClientRequestID, EventType: r.EventType,
		Description: r.Description, TaskId: r.TaskID, HasPosition: r.HasPosition,
		Lng: r.Lng, Lat: r.Lat, CapturedAt: r.CapturedAt,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// EventByRequest: GET /api/drv/safety/event/by-request?client_request_id=
// 响应丢失后的恢复入口：返回原事件，查不到就是明确未受理。
func (h *SafetyHandler) EventByRequest(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.EventByRequest(ctx, &driverv1.EventByRequestRequest{
		ClientRequestId: c.Query("client_request_id"),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// AddSupplement: POST /api/drv/safety/event/supplement
func (h *SafetyHandler) AddSupplement(c *gin.Context) {
	var r struct {
		EventID     string  `json:"event_id"`
		Kind        string  `json:"kind"`
		Note        string  `json:"note"`
		HasPosition bool    `json:"has_position"`
		Lng         float64 `json:"lng"`
		Lat         float64 `json:"lat"`
		CapturedAt  int64   `json:"captured_at"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.AddSupplement(ctx, &driverv1.AddSupplementRequest{
		EventId: r.EventID, Kind: r.Kind, Note: r.Note, HasPosition: r.HasPosition,
		Lng: r.Lng, Lat: r.Lat, CapturedAt: r.CapturedAt,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// AddAttachment: POST /api/drv/safety/event/attachment
func (h *SafetyHandler) AddAttachment(c *gin.Context) {
	var r struct {
		EventID string `json:"event_id"`
		FileID  string `json:"file_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.AddAttachment(ctx, &driverv1.AddAttachmentRequest{
		EventId: r.EventID, FileId: r.FileID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// Hotline: GET /api/drv/safety/hotline
func (h *SafetyHandler) Hotline(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.Hotline(ctx, &driverv1.HotlineRequest{})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// PendingEvents: GET /api/admin/safety/events?state=received&limit=20
func (h *SafetyHandler) PendingEvents(c *gin.Context) {
	limit := int32(20)
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = int32(parsed)
		}
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.PendingEvents(ctx, &driverv1.PendingEventsRequest{
		State: c.Query("state"), Limit: limit, DriverId: c.Query("driver_id"),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// DrainOutbox: POST /api/admin/safety/outbox/drain
func (h *SafetyHandler) DrainOutbox(c *gin.Context) {
	var r struct {
		Limit int64 `json:"limit"`
	}
	if c.Request.ContentLength > 0 {
		if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
			httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
			return
		}
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.DrainSafetyOutbox(ctx, &driverv1.DrainSafetyOutboxRequest{Limit: r.Limit})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// ClaimEvent: POST /api/admin/safety/event/claim
func (h *SafetyHandler) ClaimEvent(c *gin.Context) {
	var r struct {
		EventID         string `json:"event_id"`
		ExpectedVersion int64  `json:"expected_version"`
		RequestID       string `json:"request_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ClaimEvent(ctx, &driverv1.ClaimEventRequest{
		EventId: r.EventID, ExpectedVersion: r.ExpectedVersion, RequestId: r.RequestID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// StartProcessing: POST /api/admin/safety/event/start
func (h *SafetyHandler) StartProcessing(c *gin.Context) {
	var r struct {
		EventID         string `json:"event_id"`
		ExpectedVersion int64  `json:"expected_version"`
		RequestID       string `json:"request_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.StartProcessing(ctx, &driverv1.StartProcessingRequest{
		EventId: r.EventID, ExpectedVersion: r.ExpectedVersion, RequestId: r.RequestID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// TransferEvent: POST /api/admin/safety/event/transfer
func (h *SafetyHandler) TransferEvent(c *gin.Context) {
	var r struct {
		EventID         string `json:"event_id"`
		ExpectedVersion int64  `json:"expected_version"`
		To              string `json:"to"`
		Reason          string `json:"reason"`
		RequestID       string `json:"request_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.TransferEvent(ctx, &driverv1.TransferEventRequest{
		EventId: r.EventID, ExpectedVersion: r.ExpectedVersion, To: r.To, Reason: r.Reason, RequestId: r.RequestID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// AddAction: POST /api/admin/safety/event/action
func (h *SafetyHandler) AddAction(c *gin.Context) {
	var r struct {
		EventID         string `json:"event_id"`
		ExpectedVersion int64  `json:"expected_version"`
		Kind            string `json:"kind"`
		Note            string `json:"note"`
		RefID           string `json:"ref_id"`
		RequestID       string `json:"request_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.AddAction(ctx, &driverv1.AddActionRequest{
		EventId: r.EventID, ExpectedVersion: r.ExpectedVersion, Kind: r.Kind,
		Note: r.Note, RefId: r.RefID, RequestId: r.RequestID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// ResolveEvent: POST /api/admin/safety/event/resolve
func (h *SafetyHandler) ResolveEvent(c *gin.Context) {
	var r struct {
		EventID         string `json:"event_id"`
		ExpectedVersion int64  `json:"expected_version"`
		Result          string `json:"result"`
		Note            string `json:"note"`
		RequestID       string `json:"request_id"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.ResolveEvent(ctx, &driverv1.ResolveEventRequest{
		EventId: r.EventID, ExpectedVersion: r.ExpectedVersion, Result: r.Result, Note: r.Note, RequestId: r.RequestID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// GetEvidence: GET /api/admin/safety/evidence?event_id=
func (h *SafetyHandler) GetEvidence(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.GetEvidence(ctx, &driverv1.GetEvidenceRequest{EventId: c.Query("event_id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// EscalateTick: POST /api/admin/safety/escalate/tick
func (h *SafetyHandler) EscalateTick(c *gin.Context) {
	var r struct {
		Now   int64 `json:"now"`
		Limit int32 `json:"limit"`
	}
	if c.Request.ContentLength > 0 {
		if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
			httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
			return
		}
	}
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.EscalateTick(ctx, &driverv1.EscalateTickRequest{Now: r.Now, Limit: r.Limit})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, out)
}

// GetNotice: GET /api/drv/safety/notice?key=
func (h *SafetyHandler) GetNotice(c *gin.Context) {
	ctx, cancel := requestContext(c)
	defer cancel()
	out, err := h.client.GetNotice(ctx, &driverv1.GetNoticeRequest{Key: c.Query("key")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	// ETag 缓存：命中 If-None-Match 直接 304，不重发正文。
	if inm := c.GetHeader("If-None-Match"); inm != "" && inm == out.GetEtag() {
		c.Status(304)
		return
	}
	c.Header("ETag", out.GetEtag())
	c.Header("Cache-Control", "private, max-age=60, must-revalidate")
	c.Header("Vary", "Authorization")
	httpx.OK(c, out)
}
