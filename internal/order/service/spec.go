package service

import (
	"context"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// SpecService 是司机端规范只读用例。服务内不存在任何创建/编辑/发布路径，
// 司机只读由服务端权限兜底；文案处罚不在此形成任何扣款动作。
type SpecService struct{ repo entity.SpecRepository }

func NewSpecService(r entity.SpecRepository) *SpecService { return &SpecService{repo: r} }

// published 只返回已发布条目；为空表示后台尚无发布版本。
func (s *SpecService) published(ctx context.Context, principal authn.Principal) ([]entity.SpecEntry, error) {
	if err := requireRole(principal, "driver"); err != nil {
		return nil, err
	}
	entries, err := s.repo.PublishedEntries(ctx, entity.SpecKey)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, apperrors.NotFound("SPEC_NOT_PUBLISHED", "暂无已发布服务规范")
	}
	return entries, nil
}

// Manifest 返回版本清单，供离线恢复后先比对再更新。
func (s *SpecService) Manifest(ctx context.Context, principal authn.Principal) (*dto.SpecManifestResult, error) {
	entries, err := s.published(ctx, principal)
	if err != nil {
		return nil, err
	}
	result := &dto.SpecManifestResult{GuideID: guideIDOf(entries)}
	for _, e := range entries {
		result.Items = append(result.Items, dto.SpecManifestItem{
			GuideID: e.GuideID, Category: e.Category, Version: e.Version, PublishedAt: e.PublishedAt, ETag: e.ETag(),
		})
	}
	return result, nil
}

// Content 返回全文或短提示。未指定分类时返回全部已发布分类。
func (s *SpecService) Content(ctx context.Context, q dto.SpecQuery) (*dto.SpecResult, error) {
	entries, err := s.published(ctx, q.Principal)
	if err != nil {
		return nil, err
	}
	mode := strings.TrimSpace(q.Mode)
	if mode == "" {
		mode = "full"
	}
	if mode != "brief" && mode != "full" {
		return nil, apperrors.InvalidArgument("INVALID_MODE", "mode must be brief or full")
	}
	category := strings.TrimSpace(q.Category)
	result := &dto.SpecResult{GuideID: guideIDOf(entries)}
	for _, e := range entries {
		if category != "" && e.Category != category {
			continue
		}
		result.Categories = append(result.Categories, dto.SpecItem{
			Category: e.Category, Version: e.Version, PublishedAt: e.PublishedAt, Blocks: toSpecBlocks(e.Blocks(mode)),
		})
	}
	if len(result.Categories) == 0 {
		return nil, apperrors.NotFound("SPEC_CATEGORY_NOT_FOUND", "该分类暂无已发布内容")
	}
	// 与接口文档 3.9.6 的 { version, content } 对齐：扁平化当前结果。
	for _, item := range result.Categories {
		result.Content = append(result.Content, item.Blocks...)
		if item.Version > result.Version {
			result.Version = item.Version
		}
		if result.PublishedAt == nil || (item.PublishedAt != nil && item.PublishedAt.After(*result.PublishedAt)) {
			result.PublishedAt = item.PublishedAt
		}
	}
	result.ETag = etagOf(entries, category)
	return result, nil
}

func toSpecBlocks(blocks []entity.ContentBlock) []dto.SpecBlock {
	out := make([]dto.SpecBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, dto.SpecBlock{Type: b.Type, Text: b.Text, Items: b.Items})
	}
	return out
}

func guideIDOf(entries []entity.SpecEntry) string {
	for _, e := range entries {
		if e.GuideID != "" {
			return e.GuideID
		}
	}
	return entity.SpecKey
}

// etagOf 聚合所选条目的摘要，任一分类更新即整体失效。
func etagOf(entries []entity.SpecEntry, category string) string {
	var parts []string
	for _, e := range entries {
		if category != "" && e.Category != category {
			continue
		}
		parts = append(parts, e.Digest())
	}
	if len(parts) == 0 {
		return ""
	}
	// 直接使用条目的弱校验器拼接，不再二次 W/ 包装。
	return `W/"` + strings.Join(parts, ",") + `"`
}
