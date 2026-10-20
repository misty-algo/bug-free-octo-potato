package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/gateway/request"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
)

// DriverHandler 将 driver HTTP 接口适配到生成的 gRPC client。
type DriverHandler struct {
	client driverv1.DriverServiceClient
	log    *zap.Logger
}

// NewDriverHandler 将 driver gRPC client 注入 HTTP handler 方法。
func NewDriverHandler(client driverv1.DriverServiceClient, log *zap.Logger) *DriverHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &DriverHandler{
		client: client,
		log:    log,
	}
}

// Create 将 POST /api/v1/drivers 代理到 CreateDriver。
func (h *DriverHandler) Create(c *gin.Context) {
	var req request.CreateDriverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.CreateDriver(outgoingContext(c), &driverv1.CreateDriverRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway driver create proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", resp.GetId()))
	httpx.Created(c, resp)
}

// Get 将 GET /api/v1/drivers/:id 代理到 GetDriver。
func (h *DriverHandler) Get(c *gin.Context) {
	resp, err := h.client.GetDriver(outgoingContext(c), &driverv1.GetDriverRequest{Id: c.Param("id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// List 将 GET /api/v1/drivers 代理到 ListDrivers。
func (h *DriverHandler) List(c *gin.Context) {
	var req request.ListDriverRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.ListDrivers(outgoingContext(c), &driverv1.ListDriversRequest{
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	httpx.OK(c, resp)
}

// Update 将 PUT /api/v1/drivers/:id 代理到 UpdateDriver。
func (h *DriverHandler) Update(c *gin.Context) {
	var req request.UpdateDriverRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Error(c, apperrors.InvalidArgument("invalid_request", err.Error()))
		return
	}
	resp, err := h.client.UpdateDriver(outgoingContext(c), &driverv1.UpdateDriverRequest{
		Id:          c.Param("id"),
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway driver update proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", resp.GetId()))
	httpx.OK(c, resp)
}

// Delete 将 DELETE /api/v1/drivers/:id 代理到 DeleteDriver。
func (h *DriverHandler) Delete(c *gin.Context) {
	resp, err := h.client.DeleteDriver(outgoingContext(c), &driverv1.DeleteDriverRequest{Id: c.Param("id")})
	if err != nil {
		httpx.Error(c, apperrors.FromGRPC(err))
		return
	}
	h.log.Info("gateway driver delete proxied", zap.String("request_id", httpx.RequestID(c)), zap.String("aggregate_id", c.Param("id")))
	httpx.OK(c, resp)
}
