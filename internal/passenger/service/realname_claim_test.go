package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

func TestSubmitRealName(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)

	t.Run("提交成功写回实名状态并保存脱敏值", func(t *testing.T) {
		svc, accounts, _, _, _ := newTestService(t, now)
		seedVerifiedAccount(t, accounts, "13800000000", now)

		out, err := svc.SubmitRealName(ctx, dto.SubmitRealNameCommand{
			AccountID: "account-1", Name: "王小明", IDNo: "330110199001011234",
		})
		if err != nil {
			t.Fatalf("SubmitRealName() error = %v", err)
		}
		if out.State != dto.RealNameStateVerified {
			t.Fatalf("SubmitRealName() state = %q, want %q", out.State, dto.RealNameStateVerified)
		}
		if strings.Contains(out.NameMask, "王小明") {
			t.Fatalf("name mask leaked real name: %q", out.NameMask)
		}
		if strings.Contains(out.IDNoMask, "1234") {
			t.Fatalf("id no mask leaked full digits: %q", out.IDNoMask)
		}
		saved, err := accounts.FindByID(ctx, "account-1")
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if !saved.RealNameVerified {
			t.Fatalf("got RealNameVerified = false, want true")
		}
	})

	t.Run("重复提交幂等返回当前状态", func(t *testing.T) {
		svc, accounts, _, _, _ := newTestService(t, now)
		seedVerifiedAccount(t, accounts, "13800000000", now)
		cmd := dto.SubmitRealNameCommand{AccountID: "account-1", Name: "王小明", IDNo: "330110199001011234"}
		if _, err := svc.SubmitRealName(ctx, cmd); err != nil {
			t.Fatalf("first SubmitRealName() error = %v", err)
		}
		second, err := svc.SubmitRealName(ctx, cmd)
		if err != nil {
			t.Fatalf("second SubmitRealName() error = %v", err)
		}
		if second.State != dto.RealNameStateVerified {
			t.Fatalf("got state = %q, want %q", second.State, dto.RealNameStateVerified)
		}
	})

	t.Run("非法证件号或空姓名返回错误", func(t *testing.T) {
		cases := []struct {
			name string
			cmd  dto.SubmitRealNameCommand
			want error
		}{
			{name: "证件号格式非法", cmd: dto.SubmitRealNameCommand{AccountID: "account-1", Name: "王小明", IDNo: "abc"}, want: entity.ErrInvalidRealName},
			{name: "姓名为空", cmd: dto.SubmitRealNameCommand{AccountID: "account-1", Name: "", IDNo: "330110199001011234"}, want: entity.ErrInvalidRealName},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, accounts, _, _, _ := newTestService(t, now)
				seedVerifiedAccount(t, accounts, "13800000000", now)
				if _, err := svc.SubmitRealName(ctx, tc.cmd); !errors.Is(err, tc.want) {
					t.Fatalf("SubmitRealName() error = %v, want %v", err, tc.want)
				}
			})
		}
	})

	t.Run("未登录", func(t *testing.T) {
		svc, _, _, _, _ := newTestService(t, now)
		if _, err := svc.SubmitRealName(ctx, dto.SubmitRealNameCommand{Name: "王小明", IDNo: "330110199001011234"}); !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("SubmitRealName() error = %v, want ErrUnauthorized", err)
		}
	})
}

func TestGetRealNameStatus(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)

	svc, accounts, _, _, _ := newTestService(t, now)
	// 未实名的账号返回 UNVERIFIED。
	account, err := entity.NewAccount("account-1", "13800000000", now)
	if err != nil {
		t.Fatalf("NewAccount() error = %v", err)
	}
	if err := accounts.Save(ctx, account); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	out, err := svc.GetRealNameStatus(ctx, "account-1")
	if err != nil {
		t.Fatalf("GetRealNameStatus() error = %v", err)
	}
	if out.State != dto.RealNameStateUnverified {
		t.Fatalf("got state = %q, want %q", out.State, dto.RealNameStateUnverified)
	}
}

// fakePlatformCouponRepository 是平台券只读仓储的内存实现。
type fakePlatformCouponRepository struct {
	mutex  sync.Mutex
	items  map[string]*entity.PlatformCoupon
	errNow bool
}

func newFakePlatformCouponRepository() *fakePlatformCouponRepository {
	return &fakePlatformCouponRepository{items: map[string]*entity.PlatformCoupon{}}
}

func (r *fakePlatformCouponRepository) seed(coupon *entity.PlatformCoupon) {
	r.items[coupon.CouponNo] = coupon
}

func (r *fakePlatformCouponRepository) ListAvailable(ctx context.Context, limit int, now time.Time) ([]*entity.PlatformCoupon, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	list := make([]*entity.PlatformCoupon, 0, len(r.items))
	for _, item := range r.items {
		if item.State == entity.PlatformCouponStateAvailable && now.Before(item.ExpireAt) {
			list = append(list, item)
		}
	}
	return list, nil
}

func (r *fakePlatformCouponRepository) FindByCouponNo(ctx context.Context, couponNo string) (*entity.PlatformCoupon, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r.errNow {
		return nil, errors.New("mongo down")
	}
	item, ok := r.items[couponNo]
	if !ok {
		return nil, entity.ErrPlatformCouponNotFound
	}
	return item, nil
}

func newClaimTestService(t *testing.T, now time.Time) (*Service, *fakePlatformCouponRepository, *fakeCouponRepository) {
	t.Helper()
	svc, accounts, _, _, _ := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)
	coupons := newFakeCouponRepository()
	svc.WithCouponRepository(coupons)
	platform := newFakePlatformCouponRepository()
	svc.WithPlatformCouponRepository(platform)
	return svc, platform, coupons
}

func TestClaimCoupon(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)
	valid := &entity.PlatformCoupon{
		CouponNo: "HZ-C-001", TemplateNo: "tpl-1", State: entity.PlatformCouponStateAvailable,
		FaceValueFen: 500, ThresholdFen: 2000, ExpireAt: now.Add(72 * time.Hour),
	}

	t.Run("领取成功入账且可再次查询", func(t *testing.T) {
		svc, platform, coupons := newClaimTestService(t, now)
		platform.seed(valid)
		out, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: "HZ-C-001"})
		if err != nil {
			t.Fatalf("ClaimCoupon() error = %v", err)
		}
		if out.CouponID == "" || out.FaceValue != 500 {
			t.Fatalf("ClaimCoupon() = %+v, want face value 500", out)
		}
		saved, err := coupons.FindByID(ctx, "plat-HZ-C-001-account-1")
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if saved.AccountID != "account-1" || saved.FaceValue != 500 {
			t.Fatalf("saved coupon = %+v, want account-1 with face value 500", saved)
		}
		// 另一个乘客领同一券码应各得一张，互不影响。
		other, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-2", CouponNo: "HZ-C-001"})
		if err != nil {
			t.Fatalf("ClaimCoupon() by another account error = %v", err)
		}
		if other.CouponID == out.CouponID {
			t.Fatalf("got same coupon id %q for two accounts, want distinct records", other.CouponID)
		}
	})

	t.Run("同券码重复领取幂等返回已有券", func(t *testing.T) {
		svc, platform, coupons := newClaimTestService(t, now)
		platform.seed(valid)
		cmd := dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: "HZ-C-001"}
		first, err := svc.ClaimCoupon(ctx, cmd)
		if err != nil {
			t.Fatalf("first ClaimCoupon() error = %v", err)
		}
		second, err := svc.ClaimCoupon(ctx, cmd)
		if err != nil {
			t.Fatalf("second ClaimCoupon() error = %v", err)
		}
		if first.CouponID != second.CouponID {
			t.Fatalf("got coupon ids %q and %q, want equal", first.CouponID, second.CouponID)
		}
		if got := len(coupons.coupons); got != 1 {
			t.Fatalf("got %d coupons, want 1", got)
		}
	})

	t.Run("平台侧不可用时拒绝", func(t *testing.T) {
		svc, platform, _ := newClaimTestService(t, now)
		locked := *valid
		locked.State = "LOCKED"
		platform.seed(&locked)
		if _, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: "HZ-C-001"}); !errors.Is(err, entity.ErrPlatformCouponUnavailable) {
			t.Fatalf("ClaimCoupon() error = %v, want ErrPlatformCouponUnavailable", err)
		}
	})

	t.Run("过期券拒绝", func(t *testing.T) {
		svc, platform, _ := newClaimTestService(t, now)
		expired := *valid
		expired.ExpireAt = now.Add(-time.Hour)
		platform.seed(&expired)
		if _, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: "HZ-C-001"}); !errors.Is(err, entity.ErrCouponUnavailable) {
			t.Fatalf("ClaimCoupon() error = %v, want ErrCouponUnavailable", err)
		}
	})

	t.Run("券码不存在", func(t *testing.T) {
		svc, _, _ := newClaimTestService(t, now)
		if _, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{AccountID: "account-1", CouponNo: "NOPE"}); !errors.Is(err, entity.ErrPlatformCouponNotFound) {
			t.Fatalf("ClaimCoupon() error = %v, want ErrPlatformCouponNotFound", err)
		}
	})

	t.Run("未登录", func(t *testing.T) {
		svc, _, _, _, _ := newTestService(t, now)
		if _, err := svc.ClaimCoupon(ctx, dto.ClaimCouponCommand{CouponNo: "HZ-C-001"}); !errors.Is(err, entity.ErrUnauthorized) {
			t.Fatalf("ClaimCoupon() error = %v, want ErrUnauthorized", err)
		}
	})
}

func TestListClaimableCoupons(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)

	svc, platform, _ := newClaimTestService(t, now)
	platform.seed(&entity.PlatformCoupon{
		CouponNo: "HZ-C-001", State: entity.PlatformCouponStateAvailable,
		FaceValueFen: 500, ThresholdFen: 2000, ExpireAt: now.Add(72 * time.Hour),
	})

	out, err := svc.ListClaimableCoupons(ctx, "account-1", 0)
	if err != nil {
		t.Fatalf("ListClaimableCoupons() error = %v", err)
	}
	if len(out) != 1 || out[0].CouponID != "HZ-C-001" {
		t.Fatalf("ListClaimableCoupons() = %+v, want one coupon HZ-C-001", out)
	}
}

func TestEstimateAppliesCouponDiscount(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.Local)

	t.Run("估价带券返回抵扣后应付金额", func(t *testing.T) {
		svc, _, coupons := newClaimTestService(t, now)
		coupons.coupons["plat-HZ-C-002"] = &entity.Coupon{
			ID: "plat-HZ-C-002", AccountID: "account-1", Name: "打车券",
			FaceValue: 500, Threshold: 0,
			State: entity.CouponStateAvailable, ExpiresAt: now.Add(72 * time.Hour),
		}
		out, err := svc.Estimate(ctx, dto.EstimateCommand{
			AccountID: "account-1", CarType: CarTypeExpress, CouponID: "plat-HZ-C-002",
			StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		})
		if err != nil {
			t.Fatalf("Estimate() error = %v", err)
		}
		if out.DiscountFee != 500 {
			t.Fatalf("got DiscountFee = %d, want 500", out.DiscountFee)
		}
		if out.PayFee != out.BaseFee+out.ServiceFee-500 {
			t.Fatalf("got PayFee = %d, want BaseFee+ServiceFee-500 = %d", out.PayFee, out.BaseFee+out.ServiceFee-500)
		}
	})

	t.Run("下单锁定券，取消后释放", func(t *testing.T) {
		svc, _, coupons := newClaimTestService(t, now)
		coupons.coupons["plat-HZ-C-002"] = &entity.Coupon{
			ID: "plat-HZ-C-002", AccountID: "account-1", Name: "打车券",
			FaceValue: 500, Threshold: 0,
			State: entity.CouponStateAvailable, ExpiresAt: now.Add(72 * time.Hour),
		}
		est, err := svc.Estimate(ctx, dto.EstimateCommand{
			AccountID: "account-1", CarType: CarTypeExpress, CouponID: "plat-HZ-C-002",
			StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		})
		if err != nil {
			t.Fatalf("Estimate() error = %v", err)
		}
		order, err := svc.CreateOrder(ctx, dto.CreateOrderCommand{
			AccountID: "account-1", EstimateNo: est.EstimateNo, CarType: CarTypeExpress,
			StartName: "A", StartAddr: "a", StartLng: 121.601, StartLat: 31.2054,
			EndName: "B", EndAddr: "b", EndLng: 121.3208, EndLat: 31.1979,
			IdempotencyKey: "k-coupon-1",
		})
		if err != nil {
			t.Fatalf("CreateOrder() error = %v", err)
		}
		locked, err := coupons.FindByID(ctx, "plat-HZ-C-002")
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if locked.State != entity.CouponStateLocked || locked.LockOrderID != order.OrderID {
			t.Fatalf("got coupon state = %s lock = %q, want locked by %s", locked.State, locked.LockOrderID, order.OrderID)
		}
		if _, err := svc.CancelOrder(ctx, dto.CancelOrderCommand{
			AccountID: "account-1", OrderID: order.OrderID, Reason: "测试取消",
		}); err != nil {
			t.Fatalf("CancelOrder() error = %v", err)
		}
		released, err := coupons.FindByID(ctx, "plat-HZ-C-002")
		if err != nil {
			t.Fatalf("FindByID() error = %v", err)
		}
		if released.State != entity.CouponStateAvailable {
			t.Fatalf("got coupon state = %s, want available after cancel", released.State)
		}
	})
}
