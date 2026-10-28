package service

import (
	"context"
	"errors"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// ratingWindow 是 ENDED/COMPLETED 后可评价窗口（契约 3.6.3：72 小时）。
const ratingWindow = 72 * time.Hour

// WithClock 注入模拟时钟（业务时间取服务端可信时间，支持跨日/过期测试）。
func (s *Service) WithClock(c clock.Clock) *Service {
	s.clock = c
	return s
}

// SubmitRating 是 P19 提交评价（POST /api/pax/order/rate，契约 3.6.3）。
// ENDED/COMPLETED 后 72h 内可评（超时 41001）；每单一次，异参重复 40901（唯一键兜底）；
// 同参重试幂等返回既有结果不重复建评；评分 1–5；anonymous 仅存标志（仅对司机隐藏身份）。
// TODO(孙源): 司机评分均分更新属 drivers 域，此处不更新。
func (s *Service) SubmitRating(ctx context.Context, cmd dto.SubmitRatingCommand) (*dto.SubmitRatingResult, error) {
	if cmd.PassengerID == "" || cmd.OrderID == "" {
		return nil, entity.ErrInvalidOrder
	}
	if cmd.Rating < 1 || cmd.Rating > 5 {
		return nil, entity.ErrInvalidOrder // 评分 1–5
	}
	queryRepo, ok := s.repo.(entity.OrderQueryRepository)
	if !ok {
		return nil, errors.New("order query repository not available")
	}
	order, err := queryRepo.FindByOrderNo(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.PassengerID != cmd.PassengerID {
		return nil, entity.ErrOrderNotFound // 归属校验，不泄露存在性（D4）
	}

	// 幂等前置判断：已评同参返回既有结果；异参重复评价 40901。
	if order.Rating != nil {
		if order.Rating.Score == cmd.Rating && order.Rating.Anonymous == cmd.Anonymous {
			return &dto.SubmitRatingResult{OrderID: order.OrderNo, Rating: order.Rating.Score}, nil
		}
		return nil, apperrors.New(apperrors.KindConflict, "40901", "order already rated")
	}

	// 窗口：ENDED/COMPLETED 时间 + 72h 与服务端时间比较。
	if order.EndedAt == nil {
		return nil, entity.ErrInvalidOrder // 未结束不可评
	}
	now := s.clock.Now().UTC()
	if now.After(order.EndedAt.UTC().Add(ratingWindow)) {
		return nil, apperrors.New(apperrors.KindConflict, "41001", "rating window expired")
	}

	ratingRepo, ok := s.repo.(entity.RatingRepository)
	if !ok {
		return nil, errors.New("rating repository not available")
	}
	rating := &entity.Rating{Score: cmd.Rating, Anonymous: cmd.Anonymous, RatedAt: now}
	matched, err := ratingRepo.MarkRated(ctx, order.ID, order.Version, rating)
	if err != nil {
		return nil, err
	}
	if !matched {
		// 唯一键/乐观锁兜底：并发下已被抢先写入，按重复评价处理。
		return nil, apperrors.New(apperrors.KindConflict, "40901", "order already rated")
	}
	return &dto.SubmitRatingResult{OrderID: order.OrderNo, Rating: cmd.Rating}, nil
}
