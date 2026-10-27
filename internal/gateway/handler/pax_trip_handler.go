package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// TripHandler 把 /api/pax/trips 的乘客行程请求代理到 shop 服务。
//
// 与 PassengerHandler 是两个独立处理器：后者对接 dev 主干的 passenger-service
// （登录、首页、下单），本处理器对接 shop-service 的行程六态链路
// （下单 → 接单 → 到达 → 上车 → 送达计价建账 → 支付）。
// 两者职责不重叠，因此各自持有自己的 gRPC client。
//
// 文件名刻意取 pax_trip_handler：handler 包下已有 trip_handler.go（司机端到达核验），
// 避免与其重名覆盖。
type TripHandler struct {
	client shopv1.ShopServiceClient
	log    *zap.Logger
}

// NewTripHandler 创建乘客行程处理器。
func NewTripHandler(client shopv1.ShopServiceClient, log *zap.Logger) *TripHandler {
	return &TripHandler{client: client, log: log}
}

// CreateTrip 将 POST /api/pax/trips 代理到 CreateTrip。
func (h *TripHandler) CreateTrip(c *gin.Context) {
	var req request.CreateTripRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreateTrip(outgoingContext(c), &shopv1.CreateTripRequest{
		TripId:      req.TripID,
		PassengerId: req.PassengerID,
		Origin:      toPaxTripGeoPoint(req.Origin),
		Destination: toPaxTripGeoPoint(req.Destination),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway trip created", zap.String("request_id", httpx.RequestID(c)), zap.String("trip_id", resp.GetTripId()))
	httpx.Created(c, resp)
}

// GetTrip 将 GET /api/pax/trips/:trip_id 代理到 GetTrip。
func (h *TripHandler) GetTrip(c *gin.Context) {
	resp, err := h.client.GetTrip(outgoingContext(c), &shopv1.GetTripRequest{TripId: c.Param("trip_id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// ListTrips 将 GET /api/pax/trips 代理到 ListTrips。
func (h *TripHandler) ListTrips(c *gin.Context) {
	resp, err := h.client.ListTrips(outgoingContext(c), &shopv1.ListTripsRequest{
		PassengerId: c.Query("passenger_id"),
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// AssignDriver 将 POST /api/pax/trips/assign 代理到 AssignDriver。
func (h *TripHandler) AssignDriver(c *gin.Context) {
	var req request.AssignDriverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.AssignDriver(outgoingContext(c), &shopv1.AssignDriverRequest{
		TripId:   req.TripID,
		DriverId: req.DriverID,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// DriverArrived 将 POST /api/pax/trips/arrive 代理到 DriverArrived。
func (h *TripHandler) DriverArrived(c *gin.Context) { h.paxTripAction(c, "arrive") }

// BoardTrip 将 POST /api/pax/trips/board 代理到 BoardTrip。
func (h *TripHandler) BoardTrip(c *gin.Context) { h.paxTripAction(c, "board") }

// CancelTrip 将 POST /api/pax/trips/cancel 代理到 CancelTrip。
func (h *TripHandler) CancelTrip(c *gin.Context) { h.paxTripAction(c, "cancel") }

// paxTripAction 统一处理到达/上车/取消三个结构相同的动作。
func (h *TripHandler) paxTripAction(c *gin.Context, action string) {
	var req request.TripActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	in := &shopv1.TripActionRequest{TripId: req.TripID, DriverId: req.DriverID}
	var (
		resp *shopv1.TripResponse
		err  error
	)
	switch action {
	case "arrive":
		resp, err = h.client.DriverArrived(outgoingContext(c), in)
	case "board":
		resp, err = h.client.BoardTrip(outgoingContext(c), in)
	case "cancel":
		resp, err = h.client.CancelTrip(outgoingContext(c), in)
	}
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// EndTrip 将 POST /api/pax/trips/end 代理到 EndTrip（计价并按锁定费率建账）。
func (h *TripHandler) EndTrip(c *gin.Context) {
	var req request.EndTripRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.EndTrip(outgoingContext(c), &shopv1.EndTripRequest{
		TripId:      req.TripID,
		DriverId:    req.DriverID,
		DistanceM:   req.DistanceM,
		DurationSec: req.DurationSec,
		WaitSec:     req.WaitSec,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway trip ended", zap.String("request_id", httpx.RequestID(c)),
		zap.String("trip_id", resp.GetTripId()), zap.Int64("passenger_paid", resp.GetPassengerPaid()),
		zap.Int64("commission_rate_bp", resp.GetCommissionRateBp()))
	httpx.OK(c, resp)
}

// PayTrip 将 POST /api/pax/trips/pay 代理到 PayTrip，返回支付宝支付链接。
func (h *TripHandler) PayTrip(c *gin.Context) {
	var req request.PayTripRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.PayTrip(outgoingContext(c), &shopv1.PayTripRequest{
		TripId: req.TripID,
		Scene:  req.Scene,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway trip pay requested", zap.String("request_id", httpx.RequestID(c)),
		zap.String("trip_id", resp.GetTripId()), zap.Int64("amount", resp.GetAmount()))
	httpx.OK(c, resp)
}

// toPaxTripGeoPoint 把 JSON 载荷的 GeoJSON 点转为 proto 表示。
func toPaxTripGeoPoint(p request.GeoPoint) *shopv1.GeoPoint {
	return &shopv1.GeoPoint{Type: p.Type, Coordinates: p.Coordinates}
}
