package service

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/order/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/order/entity"
)

// OrderCreatedEvent 是下单成功后的领域事件 routing key。
const OrderCreatedEvent = "order.created"

// EventPublisher 发布订单领域事件（当前由 RabbitMQ 实现）。
// 发布失败不应影响下单主流程，只记录日志。
type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, body []byte) error
}

// Service 编排 order 用例。
type Service struct {
	repo      entity.Repository
	publisher EventPublisher
	log       *zap.Logger
}

// NewService 创建 order 用例服务。publisher 可为 nil，表示不发布事件。
func NewService(repo entity.Repository, publisher EventPublisher, log *zap.Logger) *Service {
	if log == nil {
		log = zap.NewNop()
	}
	return &Service{repo: repo, publisher: publisher, log: log}
}

// Create 创建 order 记录，并向 RabbitMQ 发布下单成功事件。
func (s *Service) Create(ctx context.Context, cmd dto.CreateCommand) (*dto.OrderDTO, error) {
	item, err := entity.NewOrder(cmd.Name, cmd.Description)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Save(ctx, item); err != nil {
		return nil, err
	}
	s.publishOrderCreated(ctx, item)
	s.log.Info("order created", zap.String("aggregate_id", item.ID), zap.String("use_case", "CreateOrder"))
	return dto.FromOrder(item), nil
}

// Get 根据 ID 返回一条 order 记录。
func (s *Service) Get(ctx context.Context, id string) (*dto.OrderDTO, error) {
	item, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return dto.FromOrder(item), nil
}

// List 返回分页 order 记录。
func (s *Service) List(ctx context.Context, cmd dto.ListCommand) (*dto.ListOrderDTO, error) {
	offset, limit := normalizePagination(cmd.Page, cmd.PageSize)
	items, total, err := s.repo.List(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	output := &dto.ListOrderDTO{Items: make([]*dto.OrderDTO, 0, len(items)), Total: total}
	for _, item := range items {
		output.Items = append(output.Items, dto.FromOrder(item))
	}
	return output, nil
}

// Update 根据 ID 修改一条 order 记录。
func (s *Service) Update(ctx context.Context, cmd dto.UpdateCommand) (*dto.OrderDTO, error) {
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
	s.log.Info("order updated", zap.String("aggregate_id", item.ID), zap.String("use_case", "UpdateOrder"))
	return dto.FromOrder(item), nil
}

// Delete 根据 ID 删除一条 order 记录。
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.log.Info("order deleted", zap.String("aggregate_id", id), zap.String("use_case", "DeleteOrder"))
	return nil
}

// publishOrderCreated 序列化订单信息并发布到消息队列。
func (s *Service) publishOrderCreated(ctx context.Context, item *entity.Order) {
	if s.publisher == nil {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"event":       OrderCreatedEvent,
		"order_id":    item.ID,
		"name":        item.Name,
		"description": item.Description,
		"created_at":  item.CreatedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		s.log.Error("marshal order created event failed", zap.String("aggregate_id", item.ID), zap.Error(err))
		return
	}
	if err := s.publisher.Publish(ctx, OrderCreatedEvent, payload); err != nil {
		s.log.Error("publish order created event failed", zap.String("aggregate_id", item.ID), zap.Error(err))
		return
	}
	s.log.Info("order created event published", zap.String("aggregate_id", item.ID), zap.String("routing_key", OrderCreatedEvent))
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
