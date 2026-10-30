package service

import (
	"context"
	"sync"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// fakeCouponRepository 是优惠券仓储的内存实现。
type fakeCouponRepository struct {
	mutex   sync.Mutex
	coupons map[string]*entity.Coupon
}

func newFakeCouponRepository() *fakeCouponRepository {
	return &fakeCouponRepository{coupons: map[string]*entity.Coupon{}}
}

// ListIDsByAccount 列出该账号名下所有券标识（不限状态）。
func (r *fakeCouponRepository) ListIDsByAccount(ctx context.Context, accountID string) ([]string, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	ids := make([]string, 0)
	for id, coupon := range r.coupons {
		if coupon.AccountID == accountID {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (r *fakeCouponRepository) FindByID(ctx context.Context, id string) (*entity.Coupon, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	coupon, ok := r.coupons[id]
	if !ok {
		return nil, entity.ErrCouponNotFound
	}
	copied := *coupon
	return &copied, nil
}

func (r *fakeCouponRepository) Save(ctx context.Context, coupon *entity.Coupon) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.coupons[coupon.ID] = coupon
	return nil
}

func (r *fakeCouponRepository) ListUsable(ctx context.Context, accountID string, now time.Time) ([]*entity.Coupon, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	list := make([]*entity.Coupon, 0, len(r.coupons))
	for _, coupon := range r.coupons {
		if coupon.AccountID == accountID {
			copied := *coupon
			list = append(list, &copied)
		}
	}
	return list, nil
}

func (r *fakeCouponRepository) Lock(ctx context.Context, id string, orderID string, now time.Time) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	coupon, ok := r.coupons[id]
	if !ok {
		return entity.ErrCouponNotFound
	}
	if coupon.State != entity.CouponStateAvailable {
		return entity.ErrCouponLocked
	}
	coupon.State = entity.CouponStateLocked
	coupon.LockOrderID = orderID
	return nil
}

func (r *fakeCouponRepository) Unlock(ctx context.Context, id string, now time.Time) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	coupon, ok := r.coupons[id]
	if !ok {
		return entity.ErrCouponNotFound
	}
	coupon.State = entity.CouponStateAvailable
	coupon.LockOrderID = ""
	return nil
}

// fakeDriverRepository 是司机档案仓储的内存实现。
type fakeDriverRepository struct {
	mutex   sync.Mutex
	drivers map[string]*entity.DriverProfile
}

func newFakeDriverRepository() *fakeDriverRepository {
	return &fakeDriverRepository{drivers: map[string]*entity.DriverProfile{}}
}

func (r *fakeDriverRepository) FindByAccountID(ctx context.Context, accountID string) (*entity.DriverProfile, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, driver := range r.drivers {
		if driver.AccountID == accountID {
			copied := *driver
			return &copied, nil
		}
	}
	return nil, entity.ErrDriverNotRegistered
}

func (r *fakeDriverRepository) FindByOrderID(ctx context.Context, orderID string) (*entity.DriverProfile, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, driver := range r.drivers {
		// 订单与司机的关联通过 order_id 字段，测试里直接用司机标识当关联键。
		if driver.ID == orderID || driver.AccountID == orderID {
			copied := *driver
			return &copied, nil
		}
	}
	return nil, entity.ErrDriverNotAssigned
}

// ensure the fakes keep compile-time contracts with the entity ports.
var (
	_ entity.CouponRepository = (*fakeCouponRepository)(nil)
	_ entity.DriverRepository = (*fakeDriverRepository)(nil)
)
