package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// Task 6 失败测试（docs/plans/2026-10-27-pax-orders-my.md）：
// P19 提交评价 —— 72h 窗口（41001）/每单一次（40901）/幂等重试/评分 1–5/匿名仅存标志。

// fixedClock 模拟时钟（业务时间取服务端时间，支持过期测试）。
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

// MarkRated 模拟原子标记已评（_id+version 乐观锁 + rated!=true 前置）。
func (r *fakeRepository) MarkRated(ctx context.Context, id string, version int64, rating *entity.Rating) (bool, error) {
	item, ok := r.items[id]
	if !ok || item.Version != version || item.Rated {
		return false, nil
	}
	copy := *item
	copy.Rated = true
	snap := *rating
	copy.Rating = &snap
	copy.Version = version + 1
	r.items[id] = &copy
	return true, nil
}

// rejectRepository 模拟唯一键/乐观锁兜底：MarkRated 一律 matched=false（并发下被抢先写）。
type rejectRepository struct{ *fakeRepository }

func (r *rejectRepository) MarkRated(ctx context.Context, id string, version int64, rating *entity.Rating) (bool, error) {
	return false, nil
}

func seedRatingOrder(repo *fakeRepository, no string, endedAt time.Time) {
	seedOrder(repo, no, "pax-1", entity.OrderEnded, endedAt.Add(-time.Hour), func(o *entity.Order) {
		o.EndedAt = &endedAt
	})
}

func requireAppCode(t *testing.T, err error, code string) {
	t.Helper()
	appErr, ok := apperrors.As(err)
	require.True(t, ok, "期望应用错误，得到 %v", err)
	require.Equal(t, code, appErr.Code)
}

// TestSubmitRating_Window：ENDED 后 72h 内允许，超时 41001。
func TestSubmitRating_Window(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	// 窗口内（结束后第 71 小时）允许
	repo := newFakeRepository()
	seedRatingOrder(repo, "R-W1", base)
	svc := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(71 * time.Hour)})
	got, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-W1", Rating: 5})
	require.NoError(t, err)
	require.Equal(t, "R-W1", got.OrderID)
	require.Equal(t, int32(5), got.Rating)

	// 超窗（结束后第 73 小时）→ 41001
	repo2 := newFakeRepository()
	seedRatingOrder(repo2, "R-W2", base)
	svc2 := NewService(repo2, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(73 * time.Hour)})
	_, err = svc2.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-W2", Rating: 5})
	require.Error(t, err)
	requireAppCode(t, err, "41001")
}

// TestSubmitRating_Once：每单一次 —— 异参重复评价 40901；唯一键/乐观锁兜底同为 40901。
func TestSubmitRating_Once(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newFakeRepository()
	seedRatingOrder(repo, "R-O1", base)
	svc := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(time.Hour)})

	got, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-O1", Rating: 5})
	require.NoError(t, err)
	require.Equal(t, int32(5), got.Rating)

	// 再评 3 分（异参）→ 40901
	_, err = svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-O1", Rating: 3})
	require.Error(t, err)
	requireAppCode(t, err, "40901")

	// 唯一键兜底：前置判断漏过（模拟并发抢先）时，原子写 matched=false 同为 40901
	rejectRepo := &rejectRepository{fakeRepository: newFakeRepository()}
	seedRatingOrder(rejectRepo.fakeRepository, "R-O2", base)
	svc2 := NewService(rejectRepo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(time.Hour)})
	_, err = svc2.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-O2", Rating: 4})
	require.Error(t, err)
	requireAppCode(t, err, "40901")
}

// TestSubmitRating_IdempotentRetry：同参重试返回同一结果不重复建评（超窗后重试仍幂等）。
func TestSubmitRating_IdempotentRetry(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newFakeRepository()
	seedRatingOrder(repo, "R-I1", base)
	svc := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(time.Hour)})

	got, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-I1", Rating: 5, Anonymous: true})
	require.NoError(t, err)
	require.Equal(t, int32(5), got.Rating)

	// 同参重试（含超窗之后）→ 返回同一结果
	svcLate := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(73 * time.Hour)})
	got2, err := svcLate.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-I1", Rating: 5, Anonymous: true})
	require.NoError(t, err)
	require.Equal(t, "R-I1", got2.OrderID)
	require.Equal(t, int32(5), got2.Rating)

	// 不重复建评：仍只有一份 rating，ratedAt 未被覆盖
	order, err := repo.FindByID(ctx, "id-R-I1")
	require.NoError(t, err)
	require.True(t, order.Rated)
	require.Equal(t, base.Add(time.Hour).UTC(), order.Rating.RatedAt.UTC())
}

// TestSubmitRating_Range：评分 1–5 校验，越界拒绝。
func TestSubmitRating_Range(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newFakeRepository()
	seedRatingOrder(repo, "R-R1", base)
	seedRatingOrder(repo, "R-R2", base)
	seedRatingOrder(repo, "R-R3", base)
	svc := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(time.Hour)})

	for _, score := range []int32{0, 6, -1} {
		_, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-R1", Rating: score})
		require.ErrorIs(t, err, entity.ErrInvalidOrder, "评分 %d 应被拒绝", score)
	}

	// 边界 1 与 5 允许
	for _, tc := range []struct {
		no    string
		score int32
	}{{"R-R2", 1}, {"R-R3", 5}} {
		got, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: tc.no, Rating: tc.score})
		require.NoError(t, err)
		require.Equal(t, tc.score, got.Rating)
	}
}

// TestSubmitRating_AnonymousFlag：anonymous 仅存标志（仅对司机隐藏身份），不改写其余快照。
func TestSubmitRating_AnonymousFlag(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)

	repo := newFakeRepository()
	seedRatingOrder(repo, "R-A1", base)
	svc := NewService(repo, nil, zap.NewNop()).WithClock(fixedClock{t: base.Add(time.Hour)})

	_, err := svc.SubmitRating(ctx, dto.SubmitRatingCommand{PassengerID: "pax-1", OrderID: "R-A1", Rating: 4, Anonymous: true})
	require.NoError(t, err)

	order, err := repo.FindByID(ctx, "id-R-A1")
	require.NoError(t, err)
	require.True(t, order.Rated)
	require.Equal(t, int32(4), order.Rating.Score)
	require.True(t, order.Rating.Anonymous)
	require.Equal(t, base.Add(time.Hour).UTC(), order.Rating.RatedAt.UTC())
}
