package errors_test

import (
	"context"
	"fmt"
	"testing"

	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestArrivalErrorMapping(t *testing.T) {
	tests := []struct {
		name string
		err  *apperrors.AppError
		http int
		grpc codes.Code
	}{
		{"invalid", apperrors.InvalidArgument("bad", "bad input"), 400, codes.InvalidArgument},
		{"unauthorized", apperrors.Unauthorized("UNAUTHENTICATED", "unauthenticated"), 401, codes.Unauthenticated},
		{"forbidden", apperrors.Forbidden("FORBIDDEN", "forbidden"), 403, codes.PermissionDenied},
		{"not found", apperrors.NotFound("TASK_NOT_FOUND", "missing"), 404, codes.NotFound},
		{"conflict", apperrors.Conflict("VERSION_CONFLICT", "conflict"), 409, codes.AlreadyExists},
		{"unavailable", apperrors.Unavailable("TEMPORARILY_UNAVAILABLE", "unavailable"), 503, codes.Unavailable},
		{"deadline", apperrors.Deadline("DEADLINE_EXCEEDED", "deadline"), 504, codes.DeadlineExceeded},
		{"internal", apperrors.Internal("internal_error", "internal"), 500, codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("operation: %w", tt.err)
			if got := apperrors.HTTPStatus(err); got != tt.http {
				t.Errorf("HTTPStatus=%d, want %d", got, tt.http)
			}
			if got := apperrors.GRPCCode(err); got != tt.grpc {
				t.Errorf("GRPCCode=%v, want %v", got, tt.grpc)
			}
			wire := apperrors.ToGRPC(err)
			if status.Code(wire) != tt.grpc {
				t.Fatalf("wire=%v", wire)
			}
			got := apperrors.FromGRPC(wire)
			if got.Kind != tt.err.Kind || got.Code != tt.err.Code || got.Message != tt.err.Message {
				t.Fatalf("round trip=%+v, want %+v", got, tt.err)
			}
			if apperrors.HTTPStatus(got) != tt.http {
				t.Fatalf("round trip HTTPStatus=%d", apperrors.HTTPStatus(got))
			}
		})
	}
}

func TestTransportFailureMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
		http int
		grpc codes.Code
	}{
		{"raw deadline", context.DeadlineExceeded, "DEADLINE_EXCEEDED", 504, codes.DeadlineExceeded},
		{"wrapped deadline", fmt.Errorf("query: %w", context.DeadlineExceeded), "DEADLINE_EXCEEDED", 504, codes.DeadlineExceeded},
		{"RPC deadline", status.Error(codes.DeadlineExceeded, "context deadline exceeded"), "DEADLINE_EXCEEDED", 504, codes.DeadlineExceeded},
		{"RPC unavailable", status.Error(codes.Unavailable, "connection unavailable"), "TEMPORARILY_UNAVAILABLE", 503, codes.Unavailable},
		{"RPC forbidden", status.Error(codes.PermissionDenied, "denied"), "FORBIDDEN", 403, codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apperrors.FromGRPC(tt.err)
			if got.Code != tt.code || apperrors.HTTPStatus(got) != tt.http {
				t.Fatalf("FromGRPC=%+v status=%d", got, apperrors.HTTPStatus(got))
			}
			if apperrors.HTTPStatus(tt.err) != tt.http || apperrors.GRPCCode(tt.err) != tt.grpc {
				t.Fatalf("raw mapping HTTP=%d GRPC=%v", apperrors.HTTPStatus(tt.err), apperrors.GRPCCode(tt.err))
			}
			if code := status.Code(apperrors.ToGRPC(tt.err)); code != tt.grpc {
				t.Fatalf("ToGRPC=%v", code)
			}
		})
	}
	if apperrors.ToGRPC(nil) != nil || apperrors.FromGRPC(nil) != nil {
		t.Fatal("nil error must remain nil")
	}
}
