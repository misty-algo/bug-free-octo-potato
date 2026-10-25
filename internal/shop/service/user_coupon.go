package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
)

// requireUserCoupons 检查用户券仓储是否注入。
func (s *Service) requireUserCoupons() error {
	if s.userCoupons == nil {
		return errors.New("user coupon repository not injected")
	}
	return nil
}

// IssueUserCoupon 处理 P14 发券：券码唯一，按有效期生成 AVAILABLE 券。
func (s *Service) IssueUserCoupon(ctx context.Context, cmd dto.IssueUserCouponCommand) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	now := time.Now()
	coupon, err := entity.NewUserCoupon(cmd.CouponNo, cmd.UserID, cmd.TemplateID, cmd.BatchID,
		cmd.ValidStart, cmd.ValidEnd, cmd.Source, now)
	if err != nil {
		return nil, err
	}
	coupon.ID = uuid.NewString()
	coupon.FaceValue = cmd.FaceValue
	coupon.MinCharge = cmd.MinCharge
	coupon.MaxDiscount = cmd.MaxDiscount
	if err := s.userCoupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	s.log.Info("user coupon issued", zap.String("coupon_no", cmd.CouponNo), zap.String("user_id", cmd.UserID), zap.String("use_case", "IssueUserCoupon"))
	return dto.FromUserCoupon(coupon), nil
}

// GetUserCoupon 处理 P14 单券查询。
func (s *Service) GetUserCoupon(ctx context.Context, couponNo string) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupon, err := s.userCoupons.FindByCouponNo(ctx, couponNo)
	if err != nil {
		return nil, err
	}
	return dto.FromUserCoupon(coupon), nil
}

// ListUserCoupons 处理 P14 用户券列表查询。
func (s *Service) ListUserCoupons(ctx context.Context, cmd dto.ListUserCouponsCommand) (*dto.ListUserCouponsDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupons, err := s.userCoupons.ListByUser(ctx, cmd.UserID, cmd.Status, cmd.Limit)
	if err != nil {
		return nil, err
	}
	output := &dto.ListUserCouponsDTO{Items: make([]*dto.UserCouponDTO, 0, len(coupons)), Total: int64(len(coupons))}
	for _, coupon := range coupons {
		output.Items = append(output.Items, dto.FromUserCoupon(coupon))
	}
	return output, nil
}

// CountUsableCoupons 处理 P14 可用券计数（下单页角标）。
func (s *Service) CountUsableCoupons(ctx context.Context, userID string) (int64, error) {
	if err := s.requireUserCoupons(); err != nil {
		return 0, err
	}
	return s.userCoupons.CountUsableByUser(ctx, userID)
}

// LockUserCoupon 处理 P14 锁券：下单时 AVAILABLE → LOCKED，同单重复锁定幂等。
func (s *Service) LockUserCoupon(ctx context.Context, cmd dto.LockUserCouponCommand) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupon, err := s.userCoupons.FindByCouponNo(ctx, cmd.CouponNo)
	if err != nil {
		return nil, err
	}
	if err := coupon.Lock(cmd.OrderID, time.Now()); err != nil {
		return nil, err
	}
	if err := s.userCoupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	s.log.Info("user coupon locked", zap.String("coupon_no", cmd.CouponNo), zap.String("order_id", cmd.OrderID), zap.String("use_case", "LockUserCoupon"))
	return dto.FromUserCoupon(coupon), nil
}

// UseUserCoupon 处理 P14 核销：支付成功确认时 LOCKED → USED，记录实际抵扣。
func (s *Service) UseUserCoupon(ctx context.Context, cmd dto.UseUserCouponCommand) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupon, err := s.userCoupons.FindByCouponNo(ctx, cmd.CouponNo)
	if err != nil {
		return nil, err
	}
	if err := coupon.Use(cmd.OrderID, cmd.DiscountAmount, time.Now()); err != nil {
		return nil, err
	}
	if err := s.userCoupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	s.log.Info("user coupon used", zap.String("coupon_no", cmd.CouponNo), zap.String("order_id", cmd.OrderID), zap.Int64("discount", cmd.DiscountAmount), zap.String("use_case", "UseUserCoupon"))
	return dto.FromUserCoupon(coupon), nil
}

// ReleaseUserCoupon 处理 P14 解锁：支付失败/取消订单时 LOCKED → AVAILABLE。
func (s *Service) ReleaseUserCoupon(ctx context.Context, cmd dto.ReleaseUserCouponCommand) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupon, err := s.userCoupons.FindByCouponNo(ctx, cmd.CouponNo)
	if err != nil {
		return nil, err
	}
	if coupon.LockOrderID != "" && coupon.LockOrderID != cmd.OrderID {
		return nil, entity.ErrUserCouponInvalidState
	}
	if err := coupon.Release(time.Now()); err != nil {
		return nil, err
	}
	if err := s.userCoupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	s.log.Info("user coupon released", zap.String("coupon_no", cmd.CouponNo), zap.String("use_case", "ReleaseUserCoupon"))
	return dto.FromUserCoupon(coupon), nil
}

// VoidUserCoupon 处理 P14 作废：风控/运营介入时 AVAILABLE/LOCKED → VOID（不可恢复）。
func (s *Service) VoidUserCoupon(ctx context.Context, cmd dto.VoidUserCouponCommand) (*dto.UserCouponDTO, error) {
	if err := s.requireUserCoupons(); err != nil {
		return nil, err
	}
	coupon, err := s.userCoupons.FindByCouponNo(ctx, cmd.CouponNo)
	if err != nil {
		return nil, err
	}
	if err := coupon.Void(time.Now()); err != nil {
		return nil, err
	}
	if err := s.userCoupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	s.log.Info("user coupon voided", zap.String("coupon_no", cmd.CouponNo), zap.String("use_case", "VoidUserCoupon"))
	return dto.FromUserCoupon(coupon), nil
}

// ExpireUserCoupons 处理 P14 过期扫描：把已过有效期的 AVAILABLE/LOCKED 券批量标记 EXPIRED。
func (s *Service) ExpireUserCoupons(ctx context.Context, limit int) (int64, error) {
	if err := s.requireUserCoupons(); err != nil {
		return 0, err
	}
	if limit <= 0 {
		limit = 200
	}
	now := time.Now()
	expired, err := s.userCoupons.ListExpiredDue(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	var count int64
	for _, coupon := range expired {
		if err := coupon.MarkExpired(now); err != nil {
			continue // 已被并发改成其他状态，跳过
		}
		if err := s.userCoupons.Save(ctx, coupon); err != nil {
			continue
		}
		count++
	}
	if count > 0 {
		s.log.Info("user coupons expired", zap.Int64("count", count), zap.String("use_case", "ExpireUserCoupons"))
	}
	return count, nil
}
