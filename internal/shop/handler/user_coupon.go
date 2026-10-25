package handler

import (
	"context"

	shopv1 "github.com/BwCloudWeGo/bw-cli/api/gen/shop/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
)

// IssueUserCoupon 处理 P14 发券 RPC。
func (s *Server) IssueUserCoupon(ctx context.Context, req *shopv1.IssueUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.IssueUserCoupon(ctx, dto.IssueUserCouponCommand{
		CouponNo:    req.GetCouponNo(),
		UserID:      req.GetUserId(),
		TemplateID:  req.GetTemplateId(),
		BatchID:     req.GetBatchId(),
		ValidStart:  unixToTime(req.GetValidStartUnix()),
		ValidEnd:    unixToTime(req.GetValidEndUnix()),
		Source:      req.GetSource(),
		FaceValue:   req.GetFaceValue(),
		MinCharge:   req.GetMinCharge(),
		MaxDiscount: req.GetMaxDiscount(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

// GetUserCoupon 处理 P14 单券查询 RPC。
func (s *Server) GetUserCoupon(ctx context.Context, req *shopv1.GetUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.GetUserCoupon(ctx, req.GetCouponNo())
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

// ListUserCoupons 处理 P14 用户券列表 RPC。
func (s *Server) ListUserCoupons(ctx context.Context, req *shopv1.ListUserCouponsRequest) (*shopv1.ListUserCouponsResponse, error) {
	list, err := s.svc.ListUserCoupons(ctx, dto.ListUserCouponsCommand{
		UserID: req.GetUserId(),
		Status: req.GetStatus(),
		Limit:  int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	resp := &shopv1.ListUserCouponsResponse{
		Items: make([]*shopv1.UserCouponResponse, 0, len(list.Items)),
		Total: list.Total,
	}
	for _, item := range list.Items {
		resp.Items = append(resp.Items, userCouponToProto(item))
	}
	return resp, nil
}

// CountUsableCoupons 处理 P14 可用券计数 RPC。
func (s *Server) CountUsableCoupons(ctx context.Context, req *shopv1.CountUsableCouponsRequest) (*shopv1.CountUsableCouponsResponse, error) {
	count, err := s.svc.CountUsableCoupons(ctx, req.GetUserId())
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.CountUsableCouponsResponse{UserId: req.GetUserId(), Count: count}, nil
}

// LockUserCoupon 处理 P14 锁券 RPC。
func (s *Server) LockUserCoupon(ctx context.Context, req *shopv1.LockUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.LockUserCoupon(ctx, dto.LockUserCouponCommand{
		CouponNo: req.GetCouponNo(),
		OrderID:  req.GetOrderId(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

// UseUserCoupon 处理 P14 核销券 RPC。
func (s *Server) UseUserCoupon(ctx context.Context, req *shopv1.UseUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.UseUserCoupon(ctx, dto.UseUserCouponCommand{
		CouponNo:       req.GetCouponNo(),
		OrderID:        req.GetOrderId(),
		DiscountAmount: req.GetDiscountAmount(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

// ReleaseUserCoupon 处理 P14 解锁券 RPC。
func (s *Server) ReleaseUserCoupon(ctx context.Context, req *shopv1.ReleaseUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.ReleaseUserCoupon(ctx, dto.ReleaseUserCouponCommand{
		CouponNo: req.GetCouponNo(),
		OrderID:  req.GetOrderId(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

// ExpireUserCoupons 处理 P14 过期扫描 RPC。
func (s *Server) ExpireUserCoupons(ctx context.Context, req *shopv1.ExpireUserCouponsRequest) (*shopv1.ExpireUserCouponsResponse, error) {
	count, err := s.svc.ExpireUserCoupons(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, mapShopError(err)
	}
	return &shopv1.ExpireUserCouponsResponse{ExpiredCount: count}, nil
}

// VoidUserCoupon 处理 P14 作废券 RPC。
func (s *Server) VoidUserCoupon(ctx context.Context, req *shopv1.VoidUserCouponRequest) (*shopv1.UserCouponResponse, error) {
	item, err := s.svc.VoidUserCoupon(ctx, dto.VoidUserCouponCommand{
		CouponNo: req.GetCouponNo(),
	})
	if err != nil {
		return nil, mapShopError(err)
	}
	return userCouponToProto(item), nil
}

func userCouponToProto(item *dto.UserCouponDTO) *shopv1.UserCouponResponse {
	resp := &shopv1.UserCouponResponse{
		Id:             item.ID,
		CouponNo:       item.CouponNo,
		UserId:         item.UserID,
		TemplateId:     item.TemplateID,
		BatchId:        item.BatchID,
		Status:         item.Status,
		ObtainAtUnix:   item.ObtainAt.Unix(),
		ValidStartUnix: item.ValidStart.Unix(),
		ValidEndUnix:   item.ValidEnd.Unix(),
		OrderId:        item.OrderID,
		LockOrderId:    item.LockOrderID,
		DiscountAmount: item.DiscountAmount,
		Source:         item.Source,
		FaceValue:      item.FaceValue,
		MinCharge:      item.MinCharge,
		MaxDiscount:    item.MaxDiscount,
		Version:        item.Version,
	}
	if item.UsedAt != nil {
		resp.UsedAtUnix = item.UsedAt.Unix()
	}
	return resp
}
