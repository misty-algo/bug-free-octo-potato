package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// newExtendedService 构造带优惠券与司机仓储的服务。
func newExtendedService(t *testing.T, now time.Time) (*Service, *fakeAccountRepository, *fakeOrderRepository, *fakeEstimateRepository, *fakeCouponRepository, *fakeDriverRepository) {
	t.Helper()
	accounts := newFakeAccountRepository()
	orders := newFakeOrderRepository()
	estimates := newFakeEstimateRepository()
	coupons := newFakeCouponRepository()
	drivers := newFakeDriverRepository()

	svc, _, _, _, _ := newTestService(t, now)
	// 用同一批内存依赖重建服务，保证测试拿到的仓储与服务内部一致。
	svc = NewService(
		accounts,
		newFakeChannelRepository(),
		newFakeSessionRepository(),
		newFakeCodeRepository(),
		estimates,
		newFakeAddressRepository(),
		&fakePOIRepository{},
		orders,
		newFakeQuickEntryRepository(),
		&fakeTokenIssuer{},
		fakePasswordHasher{},
		&fakeCodeSender{},
		fakeChannelAuthenticator{},
		&fakeIDGenerator{},
	)
	svc.WithClock(func() time.Time { return now })
	svc.WithCouponRepository(coupons)
	svc.WithDriverRepository(drivers)
	svc.WithCancelFee(500, 1000)
	return svc, accounts, orders, estimates, coupons, drivers
}

func TestSaveAddressEnforcesTheLimit(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	// 常用地址上限二十条。
	for i := 0; i < entity.AddressLimit; i++ {
		if _, err := svc.SaveAddress(context.Background(), "account-1", dto.SaveAddressCommand{
			AddrType: string(entity.AddressTypeHome),
			Name:     "家",
			Addr:     "地址",
		}); err != nil {
			t.Fatalf("SaveAddress() error = %v at %d", err, i)
		}
	}
	if _, err := svc.SaveAddress(context.Background(), "account-1", dto.SaveAddressCommand{
		AddrType: string(entity.AddressTypeHome),
		Name:     "家",
		Addr:     "地址",
	}); err == nil {
		t.Fatal("SaveAddress() error = nil, want the twenty-address limit enforced")
	}
}

func TestSaveAddressRejectsEditingAnotherAccountAddress(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	created, err := svc.SaveAddress(context.Background(), "account-1", dto.SaveAddressCommand{
		AddrType: string(entity.AddressTypeHome),
		Name:     "家",
		Addr:     "地址",
	})
	if err != nil {
		t.Fatalf("SaveAddress() error = %v", err)
	}
	// 换个账号来改别人的地址，必须被拒。
	if _, err := svc.SaveAddress(context.Background(), "account-2", dto.SaveAddressCommand{
		ID:       created.ID,
		AddrType: string(entity.AddressTypeHome),
		Name:     "家",
		Addr:     "地址",
	}); !errors.Is(err, entity.ErrForbidden) {
		t.Fatalf("SaveAddress() error = %v, want ErrForbidden", err)
	}
}

func TestRecordHistoryIncrementsTheUseCount(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	for i := 0; i < 3; i++ {
		if err := svc.RecordHistory(context.Background(), "account-1", dto.RecordHistoryCommand{
			POIID: "poi_1", Name: "张江", Addr: "科苑路",
		}); err != nil {
			t.Fatalf("RecordHistory() error = %v", err)
		}
	}
	list, err := svc.ListAddresses(context.Background(), "account-1", entity.AddressTypeHistory)
	if err != nil {
		t.Fatalf("ListAddresses() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("history has %d entries, want 1", len(list))
	}
	if list[0].UseCount != 3 {
		t.Fatalf("UseCount = %d, want 3", list[0].UseCount)
	}
}

func TestBestCouponPicksTheLargestDiscount(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, coupons, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	coupons.coupons["c1"] = &entity.Coupon{
		ID: "c1", AccountID: "account-1", Name: "小券", FaceValue: 300, Threshold: 1000,
		State: entity.CouponStateAvailable, ExpiresAt: now.Add(time.Hour),
	}
	coupons.coupons["c2"] = &entity.Coupon{
		ID: "c2", AccountID: "account-1", Name: "大券", FaceValue: 1400, Threshold: 2000,
		State: entity.CouponStateAvailable, ExpiresAt: now.Add(time.Hour),
	}
	// 金额不达门槛的券不参与匹配。
	coupons.coupons["c3"] = &entity.Coupon{
		ID: "c3", AccountID: "account-1", Name: "门槛高", FaceValue: 5000, Threshold: 99999,
		State: entity.CouponStateAvailable, ExpiresAt: now.Add(time.Hour),
	}

	best, err := svc.BestCoupon(context.Background(), "account-1", "", CarTypeExpress, 5000)
	if err != nil {
		t.Fatalf("BestCoupon() error = %v", err)
	}
	if best == nil || best.ID != "c2" {
		t.Fatalf("best coupon = %+v, want c2", best)
	}
}

func TestBestCouponRejectsAnUnusableSpecifiedCoupon(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, coupons, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	coupons.coupons["c1"] = &entity.Coupon{
		ID: "c1", AccountID: "account-1", Name: "门槛高", FaceValue: 5000, Threshold: 99999,
		State: entity.CouponStateAvailable, ExpiresAt: now.Add(time.Hour),
	}
	// 指定券不满足门槛时直接报不可用，不做静默降级。
	if _, err := svc.BestCoupon(context.Background(), "account-1", "c1", CarTypeExpress, 1000); !errors.Is(err, entity.ErrCouponUnavailable) {
		t.Fatalf("BestCoupon() error = %v, want ErrCouponUnavailable", err)
	}
}

func TestCancelOrderIsTwoStage(t *testing.T) {
	now := time.Now()
	svc, accounts, orders, estimates, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	// 推进到已接单，此时取消费不为零。
	stored, err := orders.FindByID(context.Background(), order.OrderID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if err := stored.ApplyTransition(entity.OrderStateAccepted, now); err != nil {
		t.Fatalf("ApplyTransition() error = %v", err)
	}
	if err := orders.UpdateState(context.Background(), stored); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}

	// 第一次调用只报价不取消。
	quote, err := svc.CancelOrder(context.Background(), dto.CancelOrderCommand{
		AccountID: "account-1", OrderID: order.OrderID, Reason: "临时有事",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if !quote.NeedConfirm {
		t.Fatal("NeedConfirm = false, want the two-stage confirmation")
	}
	if quote.CancelFee != 500 {
		t.Fatalf("CancelFee = %d, want 500", quote.CancelFee)
	}
	after, err := orders.FindByID(context.Background(), order.OrderID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if after.State != entity.OrderStateAccepted {
		t.Fatalf("state = %q, want the order untouched before confirmation", after.State)
	}

	// 带上确认标记才真正取消。
	final, err := svc.CancelOrder(context.Background(), dto.CancelOrderCommand{
		AccountID: "account-1", OrderID: order.OrderID, Reason: "临时有事", ConfirmFee: true,
	})
	if err != nil {
		t.Fatalf("second CancelOrder() error = %v", err)
	}
	if final.NeedConfirm {
		t.Fatal("NeedConfirm = true, want the cancellation to be executed")
	}
	done, err := orders.FindByID(context.Background(), order.OrderID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if done.State != entity.OrderStateCancelled {
		t.Fatalf("state = %q, want CANCELLED", done.State)
	}
	if len(orders.logs) < 2 {
		t.Fatalf("status logs = %d, want the cancel transition recorded", len(orders.logs))
	}
}

func TestCancelOrderIsFreeWhilePending(t *testing.T) {
	now := time.Now()
	svc, accounts, _, estimates, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	// 待接单取消免费，不需要二次确认。
	quote, err := svc.CancelOrder(context.Background(), dto.CancelOrderCommand{
		AccountID: "account-1", OrderID: order.OrderID, Reason: "临时有事",
	})
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if quote.NeedConfirm {
		t.Fatal("NeedConfirm = true, want a free cancel while pending")
	}
	if quote.CancelFee != 0 {
		t.Fatalf("CancelFee = %d, want 0", quote.CancelFee)
	}
}

func TestCancelAccountRejectsAnAccountWithAnActiveOrder(t *testing.T) {
	now := time.Now()
	svc, accounts, _, estimates, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	}); err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	// 还有进行中的订单时注销必须被拒，否则乘客会被扔在半路。
	if err := svc.CancelAccount(context.Background(), "account-1"); !errors.Is(err, entity.ErrOrderStateConflict) {
		t.Fatalf("CancelAccount() error = %v, want ErrOrderStateConflict", err)
	}
}

func TestUnbindChannelKeepsOneLoginWay(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _, _ := newExtendedService(t, now)

	// 账号只有一个渠道时不允许解绑，否则账号将无法登录。
	channels := svc.channels.(*fakeChannelRepository)
	channels.seed(&entity.ChannelBinding{
		ID: "b1", AccountID: "account-1", Channel: entity.ChannelWeChat, ChannelUID: "wx-1",
	})
	if err := svc.UnbindChannel(context.Background(), "account-1", entity.ChannelWeChat); !errors.Is(err, entity.ErrOrderStateConflict) {
		t.Fatalf("UnbindChannel() error = %v, want ErrOrderStateConflict", err)
	}

	// 有两个渠道时允许解绑掉一个。
	channels.seed(&entity.ChannelBinding{
		ID: "b2", AccountID: "account-1", Channel: entity.ChannelQQ, ChannelUID: "qq-1",
	})
	if err := svc.UnbindChannel(context.Background(), "account-1", entity.ChannelWeChat); err != nil {
		t.Fatalf("UnbindChannel() error = %v, want the unbind to succeed", err)
	}
}

func TestDriverLocationMarksDelayedPositions(t *testing.T) {
	now := time.Now()
	svc, accounts, orders, estimates, _, drivers := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	order, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	})
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	stored, err := orders.FindByID(context.Background(), order.OrderID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if err := stored.ApplyTransition(entity.OrderStateAccepted, now); err != nil {
		t.Fatalf("ApplyTransition() error = %v", err)
	}
	if err := orders.UpdateState(context.Background(), stored); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}

	// 司机位置是六十秒前的，超过三十秒阈值应标延迟。
	// 司机的标识与订单标识一致，表示这单派给了他。
	drivers.drivers["d1"] = &entity.DriverProfile{
		ID: order.OrderID, AccountID: "driver-1", Name: "王师傅", PlateNo: "沪A·8Z6Q2",
		AuditState: entity.DriverAuditApproved, LastLng: 121.60, LastLat: 31.20,
		LastReportAt: now.Add(-60 * time.Second),
	}
	out, err := svc.DriverLocation(context.Background(), "account-1", order.OrderID)
	if err != nil {
		t.Fatalf("DriverLocation() error = %v", err)
	}
	if !out.Delayed {
		t.Fatal("Delayed = false, want the stale position marked")
	}
}

func TestListOrdersRespectsThePageSizeLimit(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	out, err := svc.ListOrders(context.Background(), "account-1", "", 1, 500)
	if err != nil {
		t.Fatalf("ListOrders() error = %v", err)
	}
	if out == nil {
		t.Fatal("ListOrders() returned nil")
	}
}

func TestBindPhoneRejectsAPhoneOwnedByAnotherAccount(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	other, err := entity.NewAccount("account-2", "13900000000", now)
	if err != nil {
		t.Fatalf("NewAccount() error = %v", err)
	}
	if err := accounts.Save(context.Background(), other); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// 绑定别人占用的手机号必须被拒。
	if _, err := svc.BindPhone(context.Background(), "account-1", "13900000000"); !errors.Is(err, entity.ErrPhoneAlreadyUsed) {
		t.Fatalf("BindPhone() error = %v, want ErrPhoneAlreadyUsed", err)
	}
}

func TestBindPhoneReturnsAMaskedNumber(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, _, _ := newExtendedService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	out, err := svc.BindPhone(context.Background(), "account-1", "13712345678")
	if err != nil {
		t.Fatalf("BindPhone() error = %v", err)
	}
	// 接口返回必须是掩码手机号，真实号码不能出现。
	if out.Phone != "137****5678" {
		t.Fatalf("Phone = %q, want the masked form", out.Phone)
	}
}
