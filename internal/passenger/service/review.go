package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// ErrReviewExists 同一订单重复评价。
var ErrReviewExists = errors.New("review already exists")

// SubmitReview 乘客在订单完成并支付后对司机评价或投诉。每单一条，提交后不可改。
func (s *Service) SubmitReview(ctx context.Context, cmd dto.SubmitReviewCommand) error {
	if s.reviews == nil {
		return entity.ErrInvalidStateTransition
	}
	order, err := s.orders.FindByOrderNo(ctx, cmd.OrderNo)
	if err != nil {
		return err
	}
	if order.AccountID != cmd.AccountID {
		return entity.ErrForbidden
	}
	// 只有完成并支付（COMPLETED）的订单可以评价/投诉。
	if order.State != entity.OrderStateCompleted {
		return entity.ErrInvalidStateTransition
	}
	exists, err := s.reviews.ExistsByOrderNo(ctx, order.OrderNo)
	if err != nil {
		return err
	}
	if exists {
		return ErrReviewExists
	}
	review := &entity.Review{
		ID:            s.ids.NewID(),
		OrderNo:       order.OrderNo,
		OrderID:       order.ID,
		AccountID:     cmd.AccountID,
		DriverName:    order.DriverName,
		Rating:        cmd.Rating,
		Tags:          cmd.Tags,
		Comment:       strings.TrimSpace(cmd.Comment),
		IsComplaint:   cmd.IsComplaint,
		ComplaintType: strings.TrimSpace(cmd.ComplaintType),
		ComplaintDesc: strings.TrimSpace(cmd.ComplaintDesc),
		CreatedAt:     time.Now().UTC(),
	}
	if !review.Valid() {
		return entity.ErrInvalidStateTransition
	}
	return s.reviews.Save(ctx, review)
}
