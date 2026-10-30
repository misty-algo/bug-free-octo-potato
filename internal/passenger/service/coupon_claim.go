package service

import (
	"context"
	"errors"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// platformCouponID 把「券码 + 账号」映射为本域券 ID：
// 同一乘客对同一券码重复领取命中同一条记录，领取天然幂等；
// 不同乘客各得一张，互不影响（公开投放券的每人限领一张语义）。
func platformCouponID(couponNo, accountID string) string {
	return "plat-" + couponNo + "-" + accountID
}

// ListClaimableCoupons 列出领券中心当前可领的平台券。
// 券由管理端营销中心发放到共享库 coupons 集合，这里只做只读展示。
func (s *Service) ListClaimableCoupons(ctx context.Context, accountID string, limit int) ([]*dto.CouponDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	items, err := s.platform.ListAvailable(ctx, limit, s.now().UTC())
	if err != nil {
		return nil, err
	}
	// 已领取判断以本域券包为准（不限状态：被锁定/已使用的券同样算已领取）。
	claimed := make(map[string]struct{})
	if ids, err := s.coupons.ListIDsByAccount(ctx, accountID); err == nil {
		for _, id := range ids {
			claimed[id] = struct{}{}
		}
	}
	list := make([]*dto.CouponDTO, 0, len(items))
	for _, item := range items {
		entry := platformCouponToDTO(item)
		if _, ok := claimed[platformCouponID(item.CouponNo, accountID)]; ok {
			entry.Claimed = true
		}
		list = append(list, entry)
	}
	return list, nil
}

// ClaimCoupon 领取一张平台券到本人名下。
//
// 幂等：同券码重复领取返回已领取的券，不重复入账；
// 平台侧 state 必须仍为 AVAILABLE 且未过期。领取在本域生成一张
// available 状态的券，之后的估价匹配、下单锁定复用既有链路。
// 并发边界：平台侧的领减由营销中心管理；本域以固定券 ID 的唯一记录
// 保证同一乘客对同一券码只入账一次，跨乘客互不冲突。
func (s *Service) ClaimCoupon(ctx context.Context, cmd dto.ClaimCouponCommand) (*dto.CouponDTO, error) {
	if cmd.AccountID == "" {
		return nil, entity.ErrUnauthorized
	}
	couponNo := strings.TrimSpace(cmd.CouponNo)
	if couponNo == "" {
		return nil, entity.ErrPlatformCouponNotFound
	}
	id := platformCouponID(couponNo, cmd.AccountID)
	if existing, err := s.coupons.FindByID(ctx, id); err == nil && existing != nil {
		return couponToDTO(existing), nil
	} else if err != nil && !errors.Is(err, entity.ErrCouponNotFound) {
		return nil, err
	}

	platform, err := s.platform.FindByCouponNo(ctx, couponNo)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if platform.State != entity.PlatformCouponStateAvailable {
		return nil, entity.ErrPlatformCouponUnavailable
	}
	if !now.Before(platform.ExpireAt) {
		return nil, entity.ErrCouponUnavailable
	}
	coupon := &entity.Coupon{
		ID:         id,
		AccountID:  cmd.AccountID,
		TemplateID: platform.CouponNo,
		Name:       "打车券",
		FaceValue:  platform.FaceValueFen,
		Threshold:  platform.ThresholdFen,
		State:      entity.CouponStateAvailable,
		ExpiresAt:  platform.ExpireAt,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.coupons.Save(ctx, coupon); err != nil {
		return nil, err
	}
	return couponToDTO(coupon), nil
}

// platformCouponToDTO 把平台券映射为领券中心的展示条目。
func platformCouponToDTO(item *entity.PlatformCoupon) *dto.CouponDTO {
	return &dto.CouponDTO{
		CouponID:  item.CouponNo,
		Name:      "打车券",
		FaceValue: item.FaceValueFen,
		Threshold: item.ThresholdFen,
		ExpiresAt: item.ExpireAt,
	}
}

// couponToDTO 把本域券映射为出参。
func couponToDTO(coupon *entity.Coupon) *dto.CouponDTO {
	return &dto.CouponDTO{
		CouponID:  coupon.ID,
		Name:      coupon.Name,
		FaceValue: coupon.FaceValue,
		Threshold: coupon.Threshold,
		ExpiresAt: coupon.ExpiresAt,
	}
}
