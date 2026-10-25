package handler

import (
	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/gin-gonic/gin"
)

// Verify is 乘客上车核验（接口文档 3.9.2）：
// POST /api/drv/trip/verify，入参 { order_id, code, idempotency_key }。
func (h *ArrivalHandler) Verify(c *gin.Context) {
	var r struct {
		OrderID        string `json:"order_id"`
		Code           string `json:"code"`
		IdempotencyKey string `json:"idempotency_key"`
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
	out, err := h.client.Verify(ctx, &orderv1.VerifyRequest{OrderId: r.OrderID, Code: r.Code, IdempotencyKey: key})
	arrivalResponse(c, out, err)
}

// Start is 开始行程（接口文档 3.9.3）：POST /api/drv/trip/start。
func (h *ArrivalHandler) Start(c *gin.Context) {
	var r struct {
		TaskID         string `json:"task_id"`
		IdempotencyKey string `json:"idempotency_key"`
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
	out, err := h.client.Start(ctx, &orderv1.StartRequest{TaskId: r.TaskID, IdempotencyKey: key})
	arrivalResponse(c, out, err)
}

// End is 确认送达（接口文档 3.9.4）：POST /api/drv/trip/end。
func (h *ArrivalHandler) End(c *gin.Context) {
	var r struct {
		OrderID        string `json:"order_id"`
		ActualDist     int64  `json:"actual_dist"`
		ActualDur      int64  `json:"actual_dur"`
		EndReason      string `json:"end_reason"`
		IdempotencyKey string `json:"idempotency_key"`
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
	out, err := h.client.End(ctx, &orderv1.EndRequest{
		OrderId: r.OrderID, ActualDist: r.ActualDist, ActualDur: r.ActualDur,
		EndReason: r.EndReason, IdempotencyKey: key,
	})
	arrivalResponse(c, out, err)
}

// RecordSample is 轨迹样本上报（接口文档 3.9.5 扩展）：POST /api/drv/trip/sample。
func (h *ArrivalHandler) RecordSample(c *gin.Context) {
	var r struct {
		TaskID      string   `json:"task_id"`
		SessionID   string   `json:"session_id"`
		Seq         int64    `json:"seq"`
		Lng         *float64 `json:"lng"`
		Lat         *float64 `json:"lat"`
		CRS         string   `json:"crs"`
		Accuracy    *float64 `json:"accuracy"`
		CollectedAt *int64   `json:"collected_at"`
	}
	if !decodeArrival(c, &r) {
		return
	}
	if r.Lng == nil || r.Lat == nil {
		httpx.Error(c, apperrors.InvalidArgument("INVALID_LOCATION", "lng and lat are required"))
		return
	}
	collected := int64(0)
	if r.CollectedAt != nil {
		collected = *r.CollectedAt
	}
	accuracy := 0.0
	if r.Accuracy != nil {
		accuracy = *r.Accuracy
	}
	ctx, cancel := arrivalContext(c)
	defer cancel()
	out, err := h.client.RecordSample(ctx, &orderv1.SampleRequest{
		TaskId: r.TaskID, SessionId: r.SessionID, Seq: r.Seq, Lng: *r.Lng, Lat: *r.Lat,
		Crs: r.CRS, Accuracy: accuracy, CollectedAt: collected,
	})
	arrivalResponse(c, out, err)
}
