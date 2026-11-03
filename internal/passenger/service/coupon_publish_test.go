package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// TestPublishPlatformCoupons 验证管理端发布券真正落到平台券库：
// 发布后领券中心可见、可按先到先得领取，且非法参数不会写入死券。
func TestPublishPlatformCoupons(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	expireAt := now.Add(48 * time.Hour).Unix()

	t.Run("发布后领券中心可见并可按先到先得领取", func(t *testing.T) {
		svc, platform, _ := newClaimTestService(t, now)

		templateNo, issued, err := svc.PublishPlatformCoupons(ctx, dto.PublishPlatformCouponsCommand{
			TemplateNo:   "TPL-1",
			Name:         "满20减5",
			FaceValueFen: 500,
			ThresholdFen: 2000,
			ExpireAtUnix: expireAt,
			Stock:        2,
		})
		if err != nil {
			t.Fatalf("PublishPlatformCoupons() error = %v, want nil", err)
		}
		if got, want := templateNo, "TPL-1"; got != want {
			t.Errorf("templateNo: got %q, want %q", got, want)
		}
		if got, want := issued, 2; got != want {
			t.Errorf("issued: got %d, want %d", got, want)
		}

		list, err := svc.ListClaimableCoupons(ctx, "account-1", 10)
		if err != nil {
			t.Fatalf("ListClaimableCoupons() error = %v, want nil", err)
		}
		if got, want := len(list), 2; got != want {
			t.Fatalf("可领券数量: got %d, want %d", got, want)
		}
		for _, item := range list {
			if got, want := item.FaceValue, int64(500); got != want {
				t.Errorf("面值: got %d, want %d", got, want)
			}
			if got, want := item.Threshold, int64(2000); got != want {
				t.Errorf("门槛: got %d, want %d", got, want)
			}
		}

		// 第一张领走后从未领取集合里消失，平台券状态转 CLAIMED。
		first := list[0].CouponID
		if _, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: first}); err != nil {
			t.Fatalf("ClaimCoupon() error = %v, want nil", err)
		}
		coupon, err := platform.FindByCouponNo(ctx, first)
		if err != nil {
			t.Fatalf("FindByCouponNo() error = %v, want nil", err)
		}
		if got, want := coupon.State, entity.PlatformCouponStateClaimed; got != want {
			t.Errorf("领取后状态: got %q, want %q", got, want)
		}
	})

	t.Run("券模板号为空时自动生成", func(t *testing.T) {
		svc, _, _ := newClaimTestService(t, now)

		templateNo, _, err := svc.PublishPlatformCoupons(ctx, dto.PublishPlatformCouponsCommand{
			Name:         "新人券",
			FaceValueFen: 300,
			ExpireAtUnix: expireAt,
			Stock:        1,
		})
		if err != nil {
			t.Fatalf("PublishPlatformCoupons() error = %v, want nil", err)
		}
		if !strings.HasPrefix(templateNo, "TPL") {
			t.Errorf("自动模板号应以 TPL 开头: got %q", templateNo)
		}
	})

	t.Run("发布数量超出上限时拒绝", func(t *testing.T) {
		svc, platform, _ := newClaimTestService(t, now)

		_, issued, err := svc.PublishPlatformCoupons(ctx, dto.PublishPlatformCouponsCommand{
			FaceValueFen: 300,
			ExpireAtUnix: expireAt,
			Stock:        entity.MaxPublishCouponStock + 1,
		})
		if !errors.Is(err, entity.ErrInvalidCouponStock) {
			t.Fatalf("错误: got %v, want %v", err, entity.ErrInvalidCouponStock)
		}
		if issued != 0 {
			t.Errorf("拒绝时不应发放券: got %d, want 0", issued)
		}
		if list, _ := platform.ListAvailable(ctx, 10, now); len(list) != 0 {
			t.Errorf("拒绝时不应写入任何券: got %d 张", len(list))
		}
	})

	t.Run("过期时间早于当前时间时拒绝", func(t *testing.T) {
		svc, _, _ := newClaimTestService(t, now)

		_, _, err := svc.PublishPlatformCoupons(ctx, dto.PublishPlatformCouponsCommand{
			FaceValueFen: 300,
			ExpireAtUnix: now.Add(-time.Minute).Unix(),
			Stock:        1,
		})
		if !errors.Is(err, entity.ErrPlatformCouponUnavailable) {
			t.Fatalf("错误: got %v, want %v", err, entity.ErrPlatformCouponUnavailable)
		}
	})
}
