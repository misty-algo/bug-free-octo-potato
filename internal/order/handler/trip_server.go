package handler

import (
	"context"
	"time"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
)

// unixOrZero 把可选的锁定时刻压成 Unix 秒，0 表示未锁定。
func unixOrZero(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func (s *ArrivalServer) Verify(ctx context.Context, r *orderv1.VerifyRequest) (*orderv1.VerifyResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.trip.Verify(ctx, dto.VerifyCommand{
		RequestID:      requestID(ctx),
		Principal:      principal,
		OrderID:        r.GetOrderId(),
		Code:           r.GetCode(),
		IdempotencyKey: r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.VerifyResult{
		OrderId:           result.OrderID,
		Verified:          result.Verified,
		RemainingAttempts: int32(result.RemainingAttempts),
		LockUntil:         unixOrZero(result.LockUntil),
	}, nil
}

func (s *ArrivalServer) Start(ctx context.Context, r *orderv1.StartRequest) (*orderv1.StartResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.trip.Start(ctx, dto.StartCommand{
		Principal:      principal,
		TaskID:         r.GetTaskId(),
		IdempotencyKey: r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.StartResult{TaskId: result.TaskID, State: result.State, DepartedAt: result.DepartedAt.Unix()}, nil
}

func (s *ArrivalServer) End(ctx context.Context, r *orderv1.EndRequest) (*orderv1.EndResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.trip.End(ctx, dto.EndCommand{
		Principal:      principal,
		OrderID:        r.GetOrderId(),
		ActualDist:     r.GetActualDist(),
		ActualDur:      r.GetActualDur(),
		EndReason:      r.GetEndReason(),
		IdempotencyKey: r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.EndResult{
		OrderId: result.OrderID,
		State:   result.State,
		Bill:    &orderv1.BillProjection{T: result.Bill.T, P: result.Bill.P, State: result.Bill.State},
	}, nil
}

func (s *ArrivalServer) RecordSample(ctx context.Context, r *orderv1.SampleRequest) (*orderv1.SampleResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	var accuracy *float64
	if r.GetAccuracy() > 0 {
		v := r.GetAccuracy()
		accuracy = &v
	}
	var collectedAt *time.Time
	if r.GetCollectedAt() > 0 {
		t := time.Unix(r.GetCollectedAt(), 0).UTC()
		collectedAt = &t
	}
	result, err := s.trip.RecordSample(ctx, dto.SampleCommand{
		Principal:   principal,
		TaskID:      r.GetTaskId(),
		SessionID:   r.GetSessionId(),
		Seq:         r.GetSeq(),
		Lng:         r.GetLng(),
		Lat:         r.GetLat(),
		CRS:         r.GetCrs(),
		Accuracy:    accuracy,
		CollectedAt: collectedAt,
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.SampleResult{
		Accepted:   result.Accepted,
		TaskId:     result.TaskID,
		SessionId:  result.SessionID,
		Seq:        result.Seq,
		Watermark:  result.Watermark,
		Missing:    result.Missing,
		Quality:    result.Quality,
		Converted:  result.Converted,
		Duplicated: result.Duplicated,
	}, nil
}
