package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ArrivalHandler independently authenticates HTTP and forwards the original bearer.
type ArrivalHandler struct {
	client orderv1.ArrivalServiceClient
	auth   authn.Config
	writes bool
	mu     sync.Mutex
	window time.Time
	count  int
	now    func() time.Time
}

func NewArrivalHandler(c orderv1.ArrivalServiceClient, cfg authn.Config, writes bool) *ArrivalHandler {
	return &ArrivalHandler{client: c, auth: cfg, writes: writes, now: time.Now}
}

// Guard applies to every arrival read and write; its bounded global bucket avoids
// attacker-controlled identity/IP maps. Replicas each enforce their own limit.
func (h *ArrivalHandler) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		values := c.Request.Header.Values("Authorization")
		if len(values) != 1 {
			h.unauthorized(c)
			return
		}
		parts := strings.Fields(values[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			h.unauthorized(c)
			return
		}
		p, err := authn.VerifyToken(parts[1], h.auth)
		if err != nil {
			h.unauthorized(c)
			return
		}
		switch p.Role {
		case "driver", "passenger", "dispatcher", "reviewer":
		default:
			httpx.Error(c, apperrors.Forbidden("FORBIDDEN", "role cannot access arrival APIs"))
			c.Abort()
			return
		}
		if c.Request.Method == http.MethodPost && !h.writes {
			httpx.Error(c, apperrors.Unavailable("TEMPORARILY_UNAVAILABLE", "arrival writes are disabled"))
			c.Abort()
			return
		}
		now := h.now()
		h.mu.Lock()
		if h.window.IsZero() || now.Sub(h.window) >= time.Second {
			h.window = now
			h.count = 0
		}
		h.count++
		allowed := h.count <= 100
		h.mu.Unlock()
		if !allowed {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, httpx.Response{RequestID: httpx.RequestID(c), Error: &httpx.ErrorBody{Code: "RATE_LIMITED", Message: "arrival request limit exceeded"}})
			return
		}
		c.Next()
	}
}
func (h *ArrivalHandler) unauthorized(c *gin.Context) {
	httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "valid bearer token required"))
	c.Abort()
}
func arrivalContext(c *gin.Context) (context.Context, context.CancelFunc) {
	ctx := metadata.AppendToOutgoingContext(outgoingContext(c), "authorization", c.GetHeader("Authorization"))
	return context.WithTimeout(ctx, time.Second)
}
func invalidArrival(c *gin.Context, err error) {
	httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
}
func decodeArrival(c *gin.Context, out any) bool {
	if err := request.DecodeArrivalJSON(c.Request.Body, out); err != nil {
		invalidArrival(c, err)
		return false
	}
	return true
}
func arrivalKey(c *gin.Context) (string, bool) {
	values := c.Request.Header.Values("Idempotency-Key")
	key := c.GetHeader("Idempotency-Key")
	if len(values) != 1 {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", "exactly one Idempotency-Key required"))
		return "", false
	}
	if err := request.ValidateIdempotencyKey(key); err != nil {
		invalidArrival(c, err)
		return "", false
	}
	return key, true
}
func version(c *gin.Context, s string) (uint64, bool) {
	n, err := request.ParseVersion(s)
	if err != nil {
		invalidArrival(c, err)
		return 0, false
	}
	return n, true
}
func arrivalResponse(c *gin.Context, result proto.Message, err error) {
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	raw, marshalErr := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(result)
	if marshalErr != nil {
		httpx.Error(c, apperrors.Internal("INTERNAL", "encode arrival response"))
		return
	}
	httpx.OK(c, json.RawMessage(raw))
}

// Arrive is 到达上车点确认（接口文档 3.9.1）：
// POST /api/drv/trip/arrive，入参 { task_id, idempotency_key, reason }。
func (h *ArrivalHandler) Arrive(c *gin.Context) {
	var r struct {
		TaskID         string `json:"task_id"`
		IdempotencyKey string `json:"idempotency_key"`
		Reason         string `json:"reason"`
	}
	if !decodeArrival(c, &r) {
		return
	}
	key := r.IdempotencyKey
	if key == "" {
		var ok bool
		if key, ok = arrivalKey(c); !ok {
			return
		}
	} else if err := request.ValidateIdempotencyKey(key); err != nil {
		invalidArrival(c, err)
		return
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.Arrive(ctx, &orderv1.ArriveRequest{TaskId: r.TaskID, IdempotencyKey: key, Reason: r.Reason})
	arrivalResponse(c, out, err)
}

// RecordLocation is 司机位置上报（接口文档 3.9.5）：
// POST /api/drv/position，入参 { lng, lat, task_id }。
func (h *ArrivalHandler) RecordLocation(c *gin.Context) {
	var r struct {
		Lng    *float64 `json:"lng"`
		Lat    *float64 `json:"lat"`
		TaskID string   `json:"task_id"`
	}
	if !decodeArrival(c, &r) {
		return
	}
	if r.Lng == nil || r.Lat == nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_LOCATION", "lng and lat are required"))
		return
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.RecordLocation(ctx, &orderv1.RecordLocationRequest{TaskId: r.TaskID, Lng: *r.Lng, Lat: *r.Lat})
	arrivalResponse(c, out, err)
}
