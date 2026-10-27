package handler

import (
	"context"
	"time"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// AppendTrajectory 处理 P08 轨迹上报 RPC。
func (s *Server) AppendTrajectory(ctx context.Context, req *shopv1.AppendTrajectoryRequest) (*shopv1.TrajectoryResponse, error) {
	item, err := s.svc.AppendTrajectory(ctx, dto.AppendTrajectoryCommand{
		OrderID:    req.GetOrderId(),
		DriverID:   req.GetDriverId(),
		Location:   dtoGeoPoint(req.GetLocation()),
		Speed:      req.GetSpeed(),
		Bearing:    req.GetBearing(),
		Accuracy:   req.GetAccuracy(),
		Mileage:    req.GetMileage(),
		PointType:  req.GetPointType(),
		ReportedAt: unixToTime(req.GetReportedAtUnix()),
		Source:     req.GetSource(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return trajectoryToProto(item), nil
}

// ListTrajectories 处理 P08 轨迹查询 RPC。
func (s *Server) ListTrajectories(ctx context.Context, req *shopv1.ListTrajectoriesRequest) (*shopv1.ListTrajectoriesResponse, error) {
	list, err := s.svc.ListTrajectories(ctx, dto.ListTrajectoriesCommand{
		OrderID: req.GetOrderId(),
		Limit:   int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	resp := &shopv1.ListTrajectoriesResponse{
		Items: make([]*shopv1.TrajectoryResponse, 0, len(list.Items)),
		Total: list.Total,
	}
	for _, item := range list.Items {
		resp.Items = append(resp.Items, trajectoryToProto(item))
	}
	return resp, nil
}

// BindVirtualNumber 处理 P09 虚拟号绑定 RPC。
func (s *Server) BindVirtualNumber(ctx context.Context, req *shopv1.BindVirtualNumberRequest) (*shopv1.VirtualNumberResponse, error) {
	item, err := s.svc.BindVirtualNumber(ctx, dto.BindVirtualNumberCommand{
		OrderID:     req.GetOrderId(),
		PassengerID: req.GetPassengerId(),
		DriverID:    req.GetDriverId(),
		VirtualNo:   req.GetVirtualNo(),
		OrderStatus: req.GetOrderStatus(),
		ExpireAt:    unixToTime(req.GetExpireAtUnix()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return virtualNumberToProto(item), nil
}

// GetVirtualNumber 处理 P09 虚拟号查询 RPC。
func (s *Server) GetVirtualNumber(ctx context.Context, req *shopv1.GetVirtualNumberRequest) (*shopv1.VirtualNumberResponse, error) {
	item, err := s.svc.GetVirtualNumber(ctx, dto.GetVirtualNumberCommand{OrderID: req.GetOrderId()})
	if err != nil {
		return nil, mapShopError(err)
	}
	return virtualNumberToProto(item), nil
}

// ReleaseVirtualNumber 处理 P09 虚拟号释放 RPC。
func (s *Server) ReleaseVirtualNumber(ctx context.Context, req *shopv1.ReleaseVirtualNumberRequest) (*shopv1.VirtualNumberResponse, error) {
	item, err := s.svc.ReleaseVirtualNumber(ctx, dto.ReleaseVirtualNumberCommand{OrderID: req.GetOrderId()})
	if err != nil {
		return nil, mapShopError(err)
	}
	return virtualNumberToProto(item), nil
}

// RequestDestinationChange 处理 P10 发起中途改点 RPC。
func (s *Server) RequestDestinationChange(ctx context.Context, req *shopv1.RequestDestinationChangeRequest) (*shopv1.DestinationChangeResponse, error) {
	item, err := s.svc.RequestDestinationChange(ctx, dto.RequestDestinationChangeCommand{
		OrderID:        req.GetOrderId(),
		NewEndLocation: dtoGeoPoint(req.GetNewEndLocation()),
		NewEndAddress:  req.GetNewEndAddress(),
		NewEndPoiID:    req.GetNewEndPoiId(),
		NewEstimateFee: req.GetNewEstimateFee(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return destinationChangeToProto(item), nil
}

// ConfirmDestinationChange 处理 P10 司机确认改点 RPC。
func (s *Server) ConfirmDestinationChange(ctx context.Context, req *shopv1.ConfirmDestinationChangeRequest) (*shopv1.DestinationChangeResponse, error) {
	item, err := s.svc.ConfirmDestinationChange(ctx, dto.ConfirmDestinationChangeCommand{ID: req.GetId()})
	if err != nil {
		return nil, mapShopError(err)
	}
	return destinationChangeToProto(item), nil
}

// RejectDestinationChange 处理 P10 司机拒绝改点 RPC。
func (s *Server) RejectDestinationChange(ctx context.Context, req *shopv1.RejectDestinationChangeRequest) (*shopv1.DestinationChangeResponse, error) {
	if err := s.svc.RejectDestinationChange(ctx, dto.RejectDestinationChangeCommand{ID: req.GetId()}); err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.DestinationChangeResponse{Id: req.GetId()}, nil
}

// CancelDestinationChange 处理 P10 乘客撤销改点 RPC。
func (s *Server) CancelDestinationChange(ctx context.Context, req *shopv1.CancelDestinationChangeRequest) (*shopv1.DestinationChangeResponse, error) {
	if err := s.svc.CancelDestinationChange(ctx, dto.CancelDestinationChangeCommand{ID: req.GetId()}); err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.DestinationChangeResponse{Id: req.GetId()}, nil
}

// ListDestinationChanges 处理 P10 改点申请列表 RPC。
func (s *Server) ListDestinationChanges(ctx context.Context, req *shopv1.ListDestinationChangesRequest) (*shopv1.ListDestinationChangesResponse, error) {
	list, err := s.svc.ListDestinationChanges(ctx, dto.ListDestinationChangesCommand{OrderID: req.GetOrderId()})
	if err != nil {
		return nil, mapShopError(err)
	}
	resp := &shopv1.ListDestinationChangesResponse{
		Items: make([]*shopv1.DestinationChangeResponse, 0, len(list.Items)),
		Total: list.Total,
	}
	for _, item := range list.Items {
		resp.Items = append(resp.Items, destinationChangeToProto(item))
	}
	return resp, nil
}

// CreateTripShare 处理 P11 创建行程分享 RPC。
func (s *Server) CreateTripShare(ctx context.Context, req *shopv1.CreateTripShareRequest) (*shopv1.TripShareResponse, error) {
	item, err := s.svc.CreateTripShare(ctx, dto.CreateTripShareCommand{
		OrderID:      req.GetOrderId(),
		CreatorID:    req.GetCreatorId(),
		OrderEndedAt: unixToTimePtr(req.GetOrderEndedAtUnix()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripShareToProto(item), nil
}

// AccessTripShare 处理 P11 访问行程分享 RPC。
func (s *Server) AccessTripShare(ctx context.Context, req *shopv1.AccessTripShareRequest) (*shopv1.TripShareResponse, error) {
	item, err := s.svc.AccessTripShare(ctx, dto.AccessTripShareCommand{Token: req.GetShareToken()})
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripShareToProto(item), nil
}

// RevokeTripShare 处理 P11 撤销行程分享 RPC。
func (s *Server) RevokeTripShare(ctx context.Context, req *shopv1.RevokeTripShareRequest) (*shopv1.TripShareResponse, error) {
	if err := s.svc.RevokeTripShare(ctx, dto.RevokeTripShareCommand{Token: req.GetShareToken()}); err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.TripShareResponse{ShareToken: req.GetShareToken(), Revoked: true}, nil
}

// QuoteCancelFee 处理 P12 生成取消报价 RPC。
func (s *Server) QuoteCancelFee(ctx context.Context, req *shopv1.QuoteCancelFeeRequest) (*shopv1.CancelQuoteResponse, error) {
	item, err := s.svc.QuoteCancelFee(ctx, dto.QuoteCancelCommand{
		OrderID:      req.GetOrderId(),
		QuoteNo:      req.GetQuoteNo(),
		OrderVersion: req.GetOrderVersion(),
		AcceptedAt:   unixToTime(req.GetAcceptedAtUnix()),
		ReasonCode:   req.GetReasonCode(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return cancelQuoteToProto(item), nil
}

// GetLatestCancelQuote 处理 P12 查询最新取消报价 RPC。
func (s *Server) GetLatestCancelQuote(ctx context.Context, req *shopv1.GetLatestCancelQuoteRequest) (*shopv1.CancelQuoteResponse, error) {
	item, err := s.svc.GetLatestCancelQuote(ctx, dto.GetLatestCancelQuoteCommand{OrderID: req.GetOrderId()})
	if err != nil {
		return nil, mapShopError(err)
	}
	return cancelQuoteToProto(item), nil
}

// ConsumeCancelQuote 处理 P12 消费取消报价 RPC。
func (s *Server) ConsumeCancelQuote(ctx context.Context, req *shopv1.ConsumeCancelQuoteRequest) (*shopv1.CancelQuoteResponse, error) {
	item, err := s.svc.ConsumeCancelQuote(ctx, dto.ConsumeCancelQuoteCommand{
		QuoteNo:      req.GetQuoteNo(),
		OrderVersion: req.GetOrderVersion(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return cancelQuoteToProto(item), nil
}

func trajectoryToProto(item *dto.TrajectoryDTO) *shopv1.TrajectoryResponse {
	return &shopv1.TrajectoryResponse{
		Id:              item.ID,
		OrderId:         item.OrderID,
		DriverId:        item.DriverID,
		Location:        geoPointToProto(item.Location),
		Speed:           item.Speed,
		Bearing:         item.Bearing,
		Accuracy:        item.Accuracy,
		Mileage:         item.Mileage,
		PointType:       item.PointType,
		ReportedAtUnix:  item.ReportedAt.Unix(),
		CollectedAtUnix: item.CollectedAt.Unix(),
		Source:          item.Source,
		Quality:         item.Quality,
	}
}

func virtualNumberToProto(item *dto.VirtualNumberDTO) *shopv1.VirtualNumberResponse {
	return &shopv1.VirtualNumberResponse{
		Id:             item.ID,
		OrderId:        item.OrderID,
		PassengerId:    item.PassengerID,
		DriverId:       item.DriverID,
		VirtualNo:      item.VirtualNo,
		Status:         item.Status,
		BoundAtUnix:    item.BoundAt.Unix(),
		ReleasedAtUnix: item.ReleasedAt.Unix(),
		ExpireAtUnix:   item.ExpireAt.Unix(),
	}
}

func destinationChangeToProto(item *dto.DestinationChangeDTO) *shopv1.DestinationChangeResponse {
	return &shopv1.DestinationChangeResponse{
		Id:              item.ID,
		OrderId:         item.OrderID,
		NewEndLocation:  geoPointToProto(item.NewEndLocation),
		NewEndAddress:   item.NewEndAddress,
		NewEndPoiId:     item.NewEndPoiID,
		NewEstimateFee:  item.NewEstimateFee,
		Status:          item.Status,
		RequestedAtUnix: item.RequestedAt.Unix(),
		ConfirmedAtUnix: item.ConfirmedAt.Unix(),
		ExpiresAtUnix:   item.ExpiresAt.Unix(),
		Version:         item.Version,
	}
}

func tripShareToProto(item *dto.TripShareDTO) *shopv1.TripShareResponse {
	return &shopv1.TripShareResponse{
		Id:            item.ID,
		OrderId:       item.OrderID,
		ShareToken:    item.ShareToken,
		CreatorId:     item.CreatorID,
		ExpireAtUnix:  item.ExpireAt.Unix(),
		Revoked:       item.Revoked,
		AccessCount:   item.AccessCount,
		CreatedAtUnix: item.CreatedAt.Unix(),
	}
}

func cancelQuoteToProto(item *dto.CancelQuoteDTO) *shopv1.CancelQuoteResponse {
	return &shopv1.CancelQuoteResponse{
		Id:            item.ID,
		OrderId:       item.OrderID,
		QuoteNo:       item.QuoteNo,
		CancelFee:     item.CancelFee,
		BoundVersion:  item.BoundVersion,
		ReasonCode:    item.ReasonCode,
		ExpiresAtUnix: item.ExpiresAt.Unix(),
		Used:          item.Used,
		CreatedAtUnix: item.CreatedAt.Unix(),
	}
}

func dtoGeoPoint(point *shopv1.GeoPoint) entity.GeoPoint {
	if point == nil {
		return entity.GeoPoint{}
	}
	return entity.GeoPoint{Type: point.GetType(), Coordinates: point.GetCoordinates()}
}

func geoPointToProto(point entity.GeoPoint) *shopv1.GeoPoint {
	return &shopv1.GeoPoint{Type: point.Type, Coordinates: point.Coordinates}
}

func unixToTime(seconds int64) time.Time {
	if seconds <= 0 {
		return time.Time{}
	}
	return time.Unix(seconds, 0).UTC()
}

func unixToTimePtr(seconds int64) *time.Time {
	if seconds <= 0 {
		return nil
	}
	t := time.Unix(seconds, 0).UTC()
	return &t
}
