package service

import (
	"context"

	"go.uber.org/zap"

	settlementv1 "github.com/BwCloudWeGo/bw-cli/api/gen/settlement/v1"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/shop/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/gopay"
	"github.com/BwCloudWeGo/bw-cli/pkg/wechatpayx"
)

// Service 编排 shop 用例。
type Service struct {
	repo     entity.Repository
	log      *zap.Logger
	alipay   *gopay.Client
	wechat   *wechatpayx.Client
	payments entity.PaymentRepository

	settlement settlementv1.SettlementServiceClient

	trajectories    entity.TrajectoryRepository
	virtualNumbers  entity.VirtualNumberRepository
	destinationChg  entity.DestinationChangeRepository
	tripShares      entity.TripShareRepository
	cancelQuotes    entity.CancelQuoteRepository
	userCoupons     entity.UserCouponRepository
}

// NewService 创建 shop 用例服务。
func NewService(repo entity.Repository, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, log: log}
}

// SetPaymentRepository 注入支付单仓储。支付用例落库前必须注入。
func (s *Service) SetPaymentRepository(payments entity.PaymentRepository) {
	s.payments = payments
}

// SetSettlementClient 注入结算服务客户端。退款成功后用于触发司机入账冲正；
// 未注入时退款仍正常完成，仅跳过冲正（由 refund_success 事件消费端兜底）。
func (s *Service) SetSettlementClient(client settlementv1.SettlementServiceClient) {
	s.settlement = client
}

// SetTrajectoryRepository 注入实时轨迹仓储（P08）。
func (s *Service) SetTrajectoryRepository(r entity.TrajectoryRepository) {
	s.trajectories = r
}

// SetVirtualNumberRepository 注入虚拟号仓储（P09）。
func (s *Service) SetVirtualNumberRepository(r entity.VirtualNumberRepository) {
	s.virtualNumbers = r
}

// SetDestinationChangeRepository 注入中途改点仓储（P10）。
func (s *Service) SetDestinationChangeRepository(r entity.DestinationChangeRepository) {
	s.destinationChg = r
}

// SetTripShareRepository 注入行程分享仓储（P11）。
func (s *Service) SetTripShareRepository(r entity.TripShareRepository) {
	s.tripShares = r
}

// SetCancelQuoteRepository 注入取消报价仓储（P12）。
func (s *Service) SetCancelQuoteRepository(r entity.CancelQuoteRepository) {
	s.cancelQuotes = r
}

// SetUserCouponRepository 注入用户券仓储（P14）。
func (s *Service) SetUserCouponRepository(r entity.UserCouponRepository) {
	s.userCoupons = r
}

// SetWechatClient 注入微信支付客户端。为 nil 时微信支付用例返回 wechat not configured。
func (s *Service) SetWechatClient(client *wechatpayx.Client) {
	s.wechat = client
}

// Create 创建 shop 记录。
func (s *Service) Create(ctx context.Context, cmd dto.CreateCommand) (*dto.ShopDTO, error) {
	item, err := entity.NewShop(cmd.Name, cmd.Description)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}
	s.log.Info("shop created", zap.String("aggregate_id", item.ID), zap.String("use_case", "CreateShop"))
	return dto.FromShop(item), nil
}

// Get 根据 ID 返回一条 shop 记录。
func (s *Service) Get(ctx context.Context, id string) (*dto.ShopDTO, error) {
	item, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.FromShop(item), nil
}

// List 返回分页的 shop 记录。
func (s *Service) List(ctx context.Context, cmd dto.ListCommand) (*dto.ListShopDTO, error) {
	offset, limit := normalizePagination(cmd.Page, cmd.PageSize)
	items, total, err := s.repo.List(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	output := &dto.ListShopDTO{Items: make([]*dto.ShopDTO, 0, len(items)), Total: total}
	for _, item := range items {
		output.Items = append(output.Items, dto.FromShop(item))
	}
	return output, nil
}

// Update 根据 ID 修改一条 shop 记录。
func (s *Service) Update(ctx context.Context, cmd dto.UpdateCommand) (*dto.ShopDTO, error) {
	item, err := s.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, err
	}
	if err := item.Update(cmd.Name, cmd.Description); err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}
	s.log.Info("shop updated", zap.String("aggregate_id", cmd.ID), zap.String("use_case", "UpdateShop"))
	return dto.FromShop(item), nil
}

// Delete 根据 ID 删除一条 shop 记录。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("shop deleted", zap.String("aggregate_id", id), zap.String("use_case", "DeleteShop"))
	return nil
}

func normalizePagination(page int32, pageSize int32) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return int((page - 1) * pageSize), int(pageSize)
}
