package handler

import (
	"context"
	stderrors "errors"

	"go.uber.org/zap"

	driverv1 "github.com/BwCloudWeGo/bw-cli/api/gen/driver/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/service"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Server 将 driver gRPC 请求适配到 service 用例。
type Server struct {
	driverv1.UnimplementedDriverServiceServer
	svc *service.Service
	log *zap.Logger
}

// NewServer 创建 driver gRPC 服务端适配器。
func NewServer(svc *service.Service, log *zap.Logger) *Server {
	if log == nil {
		log = zap.NewNop()
	}
	return &Server{svc: svc, log: log}
}

// CreateDriver 处理创建 RPC。
func (s *Server) CreateDriver(ctx context.Context, req *driverv1.CreateDriverRequest) (*driverv1.DriverResponse, error) {
	item, err := s.svc.Create(ctx, dto.CreateCommand{
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProto(item), nil
}

// GetDriver 处理按 ID 查询。
func (s *Server) GetDriver(ctx context.Context, req *driverv1.GetDriverRequest) (*driverv1.DriverResponse, error) {
	item, err := s.svc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProto(item), nil
}

// ListDrivers 处理分页列表查询。
func (s *Server) ListDrivers(ctx context.Context, req *driverv1.ListDriversRequest) (*driverv1.ListDriversResponse, error) {
	list, err := s.svc.List(ctx, dto.ListCommand{
		Page:     req.GetPage(),
		PageSize: req.GetPageSize(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	resp := &driverv1.ListDriversResponse{
		Items: make([]*driverv1.DriverResponse, 0, len(list.Items)),
		Total: list.Total,
	}
	for _, item := range list.Items {
		resp.Items = append(resp.Items, toProto(item))
	}
	return resp, nil
}

// UpdateDriver 处理按 ID 更新。
func (s *Server) UpdateDriver(ctx context.Context, req *driverv1.UpdateDriverRequest) (*driverv1.DriverResponse, error) {
	item, err := s.svc.Update(ctx, dto.UpdateCommand{
		ID:          req.GetId(),
		Name:        req.GetName(),
		Description: req.GetDescription(),
	})
	if err != nil {
		return nil, mapDriverError(err)
	}
	return toProto(item), nil
}

// DeleteDriver 处理按 ID 删除。
func (s *Server) DeleteDriver(ctx context.Context, req *driverv1.DeleteDriverRequest) (*driverv1.DeleteDriverResponse, error) {
	if err := s.svc.Delete(ctx, req.GetId()); err != nil {
		return nil, mapDriverError(err)
	}
	return &driverv1.DeleteDriverResponse{Success: true}, nil
}

func toProto(item *dto.DriverDTO) *driverv1.DriverResponse {
	return &driverv1.DriverResponse{
		Id:          item.ID,
		Name:        item.Name,
		Description: item.Description,
		CreatedAt:   item.CreatedAt,
		UpdatedAt:   item.UpdatedAt,
	}
}

func mapDriverError(err error) error {
	switch {
	case stderrors.Is(err, entity.ErrInvalidDriver):
		return apperrors.InvalidArgument("invalid_driver", "invalid driver input")
	case stderrors.Is(err, entity.ErrDriverNotFound):
		return apperrors.NotFound("driver_not_found", "driver not found")
	default:
		return apperrors.Wrap(apperrors.KindInternal, "driver_service_error", "driver service error", err)
	}
}
