package handler

import (
	"context"
	"strings"
	"time"

	orderv1 "github.com/BwCloudWeGo/bw-cli/api/gen/order/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
	"google.golang.org/grpc/metadata"
)

// SpecCommands 是 D08 只读用例：版本清单与正文。没有写入方法。
type SpecCommands interface {
	Manifest(context.Context, authn.Principal) (*dto.SpecManifestResult, error)
	Content(context.Context, dto.SpecQuery) (*dto.SpecResult, error)
}

type SpecServer struct {
	orderv1.UnimplementedServiceSpecServiceServer
	spec SpecCommands
	auth authn.Config
}

func NewSpecServer(s SpecCommands, cfg authn.Config) *SpecServer {
	return &SpecServer{spec: s, auth: cfg}
}

func (s *SpecServer) principal(ctx context.Context) (authn.Principal, error) {
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
	return principal, nil
}

func unixOf(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func (s *SpecServer) GetManifest(ctx context.Context, _ *orderv1.GetManifestRequest) (*orderv1.GetManifestResponse, error) {
	principal, err := s.principal(ctx)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.spec.Manifest(ctx, principal)
	if err != nil {
		return nil, grpcError(err)
	}
	out := &orderv1.GetManifestResponse{GuideId: result.GuideID}
	for _, item := range result.Items {
		out.Items = append(out.Items, &orderv1.ManifestItem{
			GuideId: item.GuideID, Category: item.Category, Version: item.Version,
			PublishedAt: unixOf(item.PublishedAt), Etag: item.ETag,
		})
	}
	return out, nil
}

func (s *SpecServer) GetContent(ctx context.Context, r *orderv1.GetContentRequest) (*orderv1.GetContentResponse, error) {
	principal, err := s.principal(ctx)
	if err != nil {
		return nil, grpcError(err)
	}
	result, err := s.spec.Content(ctx, dto.SpecQuery{Principal: principal, Category: r.GetCategory(), Mode: r.GetMode()})
	if err != nil {
		return nil, grpcError(err)
	}
	out := &orderv1.GetContentResponse{
		GuideId: result.GuideID, Version: result.Version,
		PublishedAt: unixOf(result.PublishedAt), Etag: result.ETag,
	}
	for _, item := range result.Categories {
		ci := &orderv1.ContentItem{Category: item.Category, Version: item.Version, PublishedAt: unixOf(item.PublishedAt)}
		for _, b := range item.Blocks {
			ci.Blocks = append(ci.Blocks, &orderv1.SpecBlock{Type: b.Type, Text: b.Text, Items: b.Items})
		}
		out.Categories = append(out.Categories, ci)
	}
	for _, b := range result.Content {
		out.Content = append(out.Content, &orderv1.SpecBlock{Type: b.Type, Text: b.Text, Items: b.Items})
	}
	return out, nil
}
