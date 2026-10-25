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

// ContactCommands are the D07 use cases: 发消息、拉取消息、申请虚拟号。
type ContactCommands interface {
	SendMessage(context.Context, dto.SendMessageCommand) (*dto.SendMessageResult, error)
	ListMessages(context.Context, dto.ListMessagesCommand) (*dto.ListMessagesResult, error)
	RequestVirtualCall(context.Context, dto.VirtualCallCommand) (*dto.VirtualCallResult, error)
}

type ContactServer struct {
	orderv1.UnimplementedContactServiceServer
	contact ContactCommands
	auth    authn.Config
	writes  bool
}

func NewContactServer(c ContactCommands, cfg authn.Config, writes bool) *ContactServer {
	return &ContactServer{contact: c, auth: cfg, writes: writes}
}

func (s *ContactServer) principal(ctx context.Context, write bool) (authn.Principal, error) {
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
		return authn.Principal{}, apperrors.Unavailable("WRITES_DISABLED", "contact writes are disabled")
	}
	return principal, nil
}

func (s *ContactServer) SendMessage(ctx context.Context, r *orderv1.SendMessageRequest) (*orderv1.SendMessageResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.contact.SendMessage(ctx, dto.SendMessageCommand{
		Principal:       principal,
		OrderID:         r.GetOrderId(),
		Text:            r.GetText(),
		ClientMessageID: r.GetClientMessageId(),
		IdempotencyKey:  r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.SendMessageResult{IntentId: result.IntentID, Seq: result.Seq, State: result.State}, nil
}

func (s *ContactServer) ListMessages(ctx context.Context, r *orderv1.ListMessagesRequest) (*orderv1.ListMessagesResult, error) {
	principal, err := s.principal(ctx, false)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.contact.ListMessages(ctx, dto.ListMessagesCommand{Principal: principal, OrderID: r.GetOrderId(), Cursor: r.GetCursor()})
	if err != nil {
		return nil, grpcError(err)
	}
	out := &orderv1.ListMessagesResult{NextCursor: result.NextCursor}
	for _, m := range result.Messages {
		out.Messages = append(out.Messages, &orderv1.ChatMessage{
			Seq: m.Seq, From: m.From, Text: m.Text, T: m.T.Unix(), State: m.State, ClientMessageId: m.ClientMessageID,
		})
	}
	return out, nil
}

func (s *ContactServer) RequestVirtualCall(ctx context.Context, r *orderv1.VirtualCallRequest) (*orderv1.VirtualCallResult, error) {
	principal, err := s.principal(ctx, true)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.contact.RequestVirtualCall(ctx, dto.VirtualCallCommand{
		Principal: principal, OrderID: r.GetOrderId(), IdempotencyKey: r.GetIdempotencyKey(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &orderv1.VirtualCallResult{
		VirtualNumber: result.VirtualNumber, ExpiresAt: result.ExpiresAt.Unix(),
		Simulated: result.Simulated, Notice: result.Notice,
	}, nil
}
