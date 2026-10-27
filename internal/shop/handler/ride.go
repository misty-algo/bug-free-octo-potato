package handler

import (
	"context"
	"time"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// CreateTrip 处理乘客下单 RPC。
func (s *Server) CreateTrip(ctx context.Context, req *shopv1.CreateTripRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.CreateTrip(ctx, dto.CreateTripCommand{
		TripID:      req.GetTripId(),
		PassengerID: req.GetPassengerId(),
		Origin:      protoToGeoPoint(req.GetOrigin()),
		Destination: protoToGeoPoint(req.GetDestination()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// GetTrip 处理查询单个行程 RPC。
func (s *Server) GetTrip(ctx context.Context, req *shopv1.GetTripRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.GetTrip(ctx, req.GetTripId())
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// ListTrips 处理查询乘客行程列表 RPC。
func (s *Server) ListTrips(ctx context.Context, req *shopv1.ListTripsRequest) (*shopv1.ListTripsResponse, error) {
	out, err := s.svc.ListTrips(ctx, dto.ListTripsCommand{
		PassengerID: req.GetPassengerId(),
		Limit:       int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	items := make([]*shopv1.TripResponse, 0, len(out.Items))
	for _, item := range out.Items {
		items = append(items, tripToProto(item))
	}
	return &shopv1.ListTripsResponse{Items: items}, nil
}

// AssignDriver 处理司机接单 RPC。
func (s *Server) AssignDriver(ctx context.Context, req *shopv1.AssignDriverRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.AssignDriver(ctx, dto.AssignDriverCommand{
		TripID:   req.GetTripId(),
		DriverID: req.GetDriverId(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// DriverArrived 处理司机到达上车点 RPC。
func (s *Server) DriverArrived(ctx context.Context, req *shopv1.TripActionRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.DriverArrived(ctx, tripActionCommand(req))
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// BoardTrip 处理乘客上车 RPC。
func (s *Server) BoardTrip(ctx context.Context, req *shopv1.TripActionRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.BoardTrip(ctx, tripActionCommand(req))
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// CancelTrip 处理取消行程 RPC。
func (s *Server) CancelTrip(ctx context.Context, req *shopv1.TripActionRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.CancelTrip(ctx, tripActionCommand(req))
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// EndTrip 处理送达 RPC：计价并按锁定费率建账。
func (s *Server) EndTrip(ctx context.Context, req *shopv1.EndTripRequest) (*shopv1.TripResponse, error) {
	item, err := s.svc.EndTrip(ctx, dto.EndTripCommand{
		TripID:      req.GetTripId(),
		DriverID:    req.GetDriverId(),
		DistanceM:   req.GetDistanceM(),
		DurationSec: req.GetDurationSec(),
		WaitSec:     req.GetWaitSec(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return tripToProto(item), nil
}

// PayTrip 处理行程支付 RPC：金额取计价结果，支付单号复用行程号。
func (s *Server) PayTrip(ctx context.Context, req *shopv1.PayTripRequest) (*shopv1.PayTripResponse, error) {
	result, err := s.svc.PayTrip(ctx, dto.PayTripCommand{
		TripID: req.GetTripId(),
		Scene:  req.GetScene(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.PayTripResponse{
		TripId:  result.TripID,
		OrderNo: result.OrderNo,
		Amount:  result.Amount,
		PayUrl:  result.PayURL,
	}, nil
}

// tripActionCommand 把 RPC 入参转为行程动作命令。
func tripActionCommand(req *shopv1.TripActionRequest) dto.TripActionCommand {
	return dto.TripActionCommand{
		TripID:   req.GetTripId(),
		DriverID: req.GetDriverId(),
	}
}

// protoToGeoPoint 把 RPC 的 GeoJSON 点转为实体坐标。
func protoToGeoPoint(point *shopv1.GeoPoint) entity.GeoPoint {
	if point == nil {
		return entity.GeoPoint{}
	}
	return entity.GeoPoint{Type: point.GetType(), Coordinates: point.GetCoordinates()}
}

// tripToProto 把行程 DTO 转为 RPC 出参。
func tripToProto(item *dto.TripDTO) *shopv1.TripResponse {
	if item == nil {
		return nil
	}
	return &shopv1.TripResponse{
		TripId:           item.TripID,
		PassengerId:      item.PassengerID,
		DriverId:         item.DriverID,
		Status:           item.Status,
		Origin:           geoPointToProto(item.Origin),
		Destination:      geoPointToProto(item.Destination),
		DistanceM:        item.DistanceM,
		DurationSec:      item.DurationSec,
		WaitSec:          item.WaitSec,
		StartedAt:        timePtrToRFC3339(item.StartedAt),
		EndedAt:          timePtrToRFC3339(item.EndedAt),
		BillingRequestId: item.BillingRequestID,
		PassengerPaid:    item.PassengerPaid,
		CommissionRateBp: item.CommissionRateBP,
		BillCreated:      item.BillCreated,
		CreatedAt:        item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:        item.UpdatedAt.Format(time.RFC3339),
	}
}

// timePtrToRFC3339 把可选时间格式化为 RFC3339 串；nil 返回空串。
func timePtrToRFC3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
