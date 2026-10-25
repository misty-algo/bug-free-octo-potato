package handler

import (
	"net/http"
	"strconv"
	"strings"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// ContactHandler 是司机端乘客联系入口：只做鉴权、参数校验与转发，
// 消息正文、虚拟号与可验证授权由通讯安全域维护，这里不另建聊天真相。
type ContactHandler struct {
	client orderv1.ContactServiceClient
	auth   authn.Config
	writes bool
}

func NewContactHandler(c orderv1.ContactServiceClient, cfg authn.Config, writes bool) *ContactHandler {
	return &ContactHandler{client: c, auth: cfg, writes: writes}
}

func (h *ContactHandler) Guard() gin.HandlerFunc {
	return func(c *gin.Context) {
		parts := strings.Fields(c.GetHeader("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required"))
			c.Abort()
			return
		}
		if _, err := authn.VerifyToken(parts[1], h.auth); err != nil {
			httpx.Error(c, apperrors.Unauthorized("UNAUTHENTICATED", "token is invalid or expired"))
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *ContactHandler) writeEnabled(c *gin.Context) bool {
	if !h.writes {
		httpx.Error(c, apperrors.Unavailable("WRITES_DISABLED", "contact writes are disabled"))
		return false
	}
	return true
}

// SendMessage: POST /api/drv/order/message
func (h *ContactHandler) SendMessage(c *gin.Context) {
	if !h.writeEnabled(c) {
		return
	}
	var r struct {
		OrderID         string `json:"order_id"`
		Text            string `json:"text"`
		ClientMessageID string `json:"client_message_id"`
		IdempotencyKey  string `json:"idempotency_key"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	key := r.IdempotencyKey
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	if err := request.ValidateIdempotencyKey(key); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.SendMessage(ctx, &orderv1.SendMessageRequest{
		OrderId: r.OrderID, Text: r.Text, ClientMessageId: r.ClientMessageID, IdempotencyKey: key,
	})
	arrivalResponse(c, out, err)
}

// ListMessages: GET /api/drv/order/message?order_id=&cursor=
func (h *ContactHandler) ListMessages(c *gin.Context) {
	orderID := strings.TrimSpace(c.Query("order_id"))
	if orderID == "" {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", "order_id is required"))
		return
	}
	cursor, _ := strconv.ParseInt(c.Query("cursor"), 10, 64)
	if cursor < 0 {
		cursor = 0
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.ListMessages(ctx, &orderv1.ListMessagesRequest{OrderId: orderID, Cursor: cursor})
	arrivalResponse(c, out, err)
}

// RequestVirtualCall: POST /api/drv/order/virtual-call
func (h *ContactHandler) RequestVirtualCall(c *gin.Context) {
	if !h.writeEnabled(c) {
		return
	}
	var r struct {
		OrderID        string `json:"order_id"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := request.DecodeArrivalJSON(c.Request.Body, &r); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	key := r.IdempotencyKey
	if key == "" {
		key = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	}
	if err := request.ValidateIdempotencyKey(key); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_REQUEST", err.Error()))
		return
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.RequestVirtualCall(ctx, &orderv1.VirtualCallRequest{OrderId: r.OrderID, IdempotencyKey: key})
	arrivalResponse(c, out, err)
}

var _ = http.StatusOK
