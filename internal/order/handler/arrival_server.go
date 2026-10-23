package handler

import (
	"context"
	"strings"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// ArrivalCommands and LocationCommands are the trip execution use cases exposed
// over RPC. Identity always comes from the verified token, never from the body.
type ArrivalCommands interface {
	Arrive(context.Context, dto.ArriveCommand) (*dto.ArrivalResult, error)
}
type LocationCommands interface {
	RecordLocation(context.Context, dto.RecordLocationCommand) (*dto.LocationReceipt, error)
}

// TripCommands are the D06 use cases: 核验、开始、送达、轨迹上报。
type TripCommands interface {
	Verify(context.Context, dto.VerifyCommand) (*dto.VerifyResult, error)
	Start(context.Context, dto.StartCommand) (*dto.StartResult, error)
	End(context.Context, dto.EndCommand) (*dto.EndResult, error)
	RecordSample(context.Context, dto.SampleCommand) (*dto.SampleResult, error)
}

type ArrivalServer struct {
	orderv1.UnimplementedArrivalServiceServer
	arrival  ArrivalCommands
	location LocationCommands
	trip     TripCommands
	auth     authn.Config
	writes   bool
}

func NewArrivalServer(a ArrivalCommands, l LocationCommands, t TripCommands, cfg authn.Config, writes bool) *ArrivalServer {
	return &ArrivalServer{arrival: a, location: l, trip: t, auth: cfg, writes: writes}
}

func (s *ArrivalServer) principal(ctx context.Context, write bool) (authn.Principal, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required")
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "a bearer token is required")
	}
	principal, err := authn.VerifyToken(parts[1], s.auth)
	if err != nil {
		return authn.Principal{}, apperrors.Unauthorized("UNAUTHENTICATED", "token is invalid or expired")
	}
	if write && !s.writes {
		return authn.Principal{}, apperrors.Unavailable("WRITES_DISABLED", "arrival writes are disabled")
	}
	return principal, nil
}

func grpcError(err error) error {
	if e, ok := apperrors.As(err); ok {
		return apperrors.ToGRPC(e)
	}
	return err
}

func requestID(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("x-request-id"); len(values) == 1 {
		return values[0]
	}
	return ""
}

func (s *ArrivalServer) RecordLocation(ctx context.Context, r *orderv1.RecordLocationRequest) (*orderv1.LocationReceipt, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	receipt, err := s.location.RecordLocation(ctx, dto.RecordLocationCommand{
		Principal: principal, TaskID: r.GetTaskId(), Lng: r.GetLng(), Lat: r.GetLat(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.LocationReceipt{Accepted: receipt.Accepted}, nil
}

func (s *ArrivalServer) Arrive(ctx context.Context, r *orderv1.ArriveRequest) (*orderv1.ArrivalResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.arrival.Arrive(ctx, dto.ArriveCommand{
		RequestID:      requestID(ctx),
		Principal:      principal,
		TaskID:         r.GetTaskId(),
		IdempotencyKey: r.GetIdempotencyKey(),
		Reason:         r.GetReason(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.ArrivalResult{TaskId: result.TaskID, State: result.State, Abnormal: result.Abnormal}, nil
}
