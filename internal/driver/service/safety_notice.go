package service

import (
	"context"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// NoticeService 提供受控说明文本（事故上报说明 / 录音说明 / 安全须知）。
// 优先读 A18 已发布条目，缺失时回落到内置默认文案；两者都带 ETag 缓存。
type NoticeService struct {
	repo entity.SafetyNoticeReader
}

func NewNoticeService(repo entity.SafetyNoticeReader) *NoticeService {
	return &NoticeService{repo: repo}
}

func (s *NoticeService) resolve(ctx context.Context, key string) (*entity.SafetyNotice, error) {
	if s.repo != nil {
		notice, err := s.repo.PublishedNotice(ctx, key)
		if err != nil {
			return nil, err
		}
		if notice != nil {
			return notice, nil
		}
	}
	builtin, ok := entity.BuiltinSafetyNotices[key]
	if !ok {
		return nil, apperrors.NotFound("NOTICE_NOT_FOUND", "notice does not exist")
	}
	return &builtin, nil
}

func (s *NoticeService) Notice(ctx context.Context, key string) (*dto.NoticeView, error) {
	notice, err := s.resolve(ctx, key)
	if err != nil {
		return nil, err
	}
	view := &dto.NoticeView{
		Key: notice.Key, Title: notice.Title, Version: notice.Version, ETag: notice.ETag(),
	}
	for _, b := range notice.Blocks {
		view.Blocks = append(view.Blocks, dto.NoticeBlockView{Kind: b.Kind, Text: b.Text})
	}
	return view, nil
}

func (s *NoticeService) Manifest(ctx context.Context) ([]dto.NoticeItem, error) {
	keys := entity.BuiltinNoticeKeys()
	out := make([]dto.NoticeItem, 0, len(keys))
	for _, key := range keys {
		notice, err := s.resolve(ctx, key)
		if err != nil {
			return nil, err
		}
		out = append(out, dto.NoticeItem{
			Key: notice.Key, Title: notice.Title, Version: notice.Version, ETag: notice.ETag(),
		})
	}
	return out, nil
}
