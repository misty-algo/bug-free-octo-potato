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

// requireTrajectories 检查实时轨迹仓储是否注入。
func (s *Service) requireTrajectories() error {
	if s.trajectories == nil {
		return errors.New("trajectory repository not injected")
	}
	return nil
}

// requireVirtualNumbers 检查虚拟号仓储是否注入。
func (s *Service) requireVirtualNumbers() error {
	if s.virtualNumbers == nil {
		return errors.New("virtual number repository not injected")
	}
	return nil
}

// requireDestinationChanges 检查中途改点仓储是否注入。
func (s *Service) requireDestinationChanges() error {
	if s.destinationChg == nil {
		return errors.New("destination change repository not injected")
	}
	return nil
}

// requireTripShares 检查行程分享仓储是否注入。
func (s *Service) requireTripShares() error {
	if s.tripShares == nil {
		return errors.New("trip share repository not injected")
	}
	return nil
}

// requireCancelQuotes 检查取消报价仓储是否注入。
func (s *Service) requireCancelQuotes() error {
	if s.cancelQuotes == nil {
		return errors.New("cancel quote repository not injected")
	}
	return nil
}

// AppendTrajectory 处理 P08 轨迹上报：校验、与上一相邻点做质量判定后追加落库。
func (s *Service) AppendTrajectory(ctx context.Context, cmd dto.AppendTrajectoryCommand) (*dto.TrajectoryDTO, error) {
	if err := s.requireTrajectories(); err != nil {
		return nil, err
	}
	now := time.Now()
	point := &entity.TrajectoryPoint{
		ID:          uuid.NewString(),
		OrderID:     cmd.OrderID,
		DriverID:    cmd.DriverID,
		Location:    cmd.Location,
		Speed:       cmd.Speed,
		Bearing:     cmd.Bearing,
		Accuracy:    cmd.Accuracy,
		Mileage:     cmd.Mileage,
		PointType:   cmd.PointType,
		ReportedAt:  cmd.ReportedAt,
		CollectedAt: now,
		Source:      cmd.Source,
	}
	if err := point.IsValid(); err != nil {
		return nil, err
	}
	prev, err := s.trajectories.LatestByOrder(ctx, cmd.OrderID)
	if err != nil && !errors.Is(err, entity.ErrTrajectoryNotFound) {
		return nil, err
	}
	point.ApplyQuality(prev)
	if err := s.trajectories.Append(ctx, point); err != nil {
		return nil, err
	}
	s.log.Info("trajectory appended", zap.String("order_id", cmd.OrderID), zap.String("use_case", "AppendTrajectory"))
	return dto.FromTrajectoryPoint(point), nil
}

// ListTrajectories 处理 P08 轨迹查询：按上报时间升序返回。
func (s *Service) ListTrajectories(ctx context.Context, cmd dto.ListTrajectoriesCommand) (*dto.ListTrajectoriesDTO, error) {
	if err := s.requireTrajectories(); err != nil {
		return nil, err
	}
	points, err := s.trajectories.ListByOrder(ctx, cmd.OrderID, cmd.Limit)
	if err != nil {
		return nil, err
	}
	output := &dto.ListTrajectoriesDTO{Items: make([]*dto.TrajectoryDTO, 0, len(points)), Total: int64(len(points))}
	for _, point := range points {
		output.Items = append(output.Items, dto.FromTrajectoryPoint(point))
	}
	return output, nil
}

// BindVirtualNumber 处理 P09 虚拟号绑定：仅订单处于可联系状态（ACCEPTED/ARRIVED/IN_TRIP）允许。
func (s *Service) BindVirtualNumber(ctx context.Context, cmd dto.BindVirtualNumberCommand) (*dto.VirtualNumberDTO, error) {
	if err := s.requireVirtualNumbers(); err != nil {
		return nil, err
	}
	if !entity.CanContact(cmd.OrderStatus) {
		return nil, entity.ErrInvalidVirtualNumber
	}
	now := time.Now()
	virtualNumber, err := entity.NewVirtualNumber(cmd.OrderID, cmd.PassengerID, cmd.DriverID, cmd.VirtualNo, now, cmd.ExpireAt)
	if err != nil {
		return nil, err
	}
	virtualNumber.ID = uuid.NewString()
	if err := s.virtualNumbers.Save(ctx, virtualNumber); err != nil {
		return nil, err
	}
	s.log.Info("virtual number bound", zap.String("order_id", cmd.OrderID), zap.String("use_case", "BindVirtualNumber"))
	return dto.FromVirtualNumber(virtualNumber), nil
}

// GetVirtualNumber 处理 P09 虚拟号查询。
func (s *Service) GetVirtualNumber(ctx context.Context, cmd dto.GetVirtualNumberCommand) (*dto.VirtualNumberDTO, error) {
	if err := s.requireVirtualNumbers(); err != nil {
		return nil, err
	}
	virtualNumber, err := s.virtualNumbers.FindByOrder(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	return dto.FromVirtualNumber(virtualNumber), nil
}

// ReleaseVirtualNumber 处理 P09 虚拟号释放：行程结束/取消时关闭联系通道。
func (s *Service) ReleaseVirtualNumber(ctx context.Context, cmd dto.ReleaseVirtualNumberCommand) (*dto.VirtualNumberDTO, error) {
	if err := s.requireVirtualNumbers(); err != nil {
		return nil, err
	}
	virtualNumber, err := s.virtualNumbers.FindByOrder(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if err := virtualNumber.Release(time.Now()); err != nil {
		return nil, err
	}
	if err := s.virtualNumbers.Save(ctx, virtualNumber); err != nil {
		return nil, err
	}
	s.log.Info("virtual number released", zap.String("order_id", cmd.OrderID), zap.String("use_case", "ReleaseVirtualNumber"))
	return dto.FromVirtualNumber(virtualNumber), nil
}

// RequestDestinationChange 处理 P10 发起中途改点：校验订单累计成功改点次数未达上限。
func (s *Service) RequestDestinationChange(ctx context.Context, cmd dto.RequestDestinationChangeCommand) (*dto.DestinationChangeDTO, error) {
	if err := s.requireDestinationChanges(); err != nil {
		return nil, err
	}
	accepted, err := s.destinationChg.CountAcceptedByOrder(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if accepted >= entity.MaxDestinationChangePerOrder {
		return nil, entity.ErrDestinationChangeLimitHit
	}
	change, err := entity.NewDestinationChange(cmd.OrderID, cmd.NewEndLocation, cmd.NewEndAddress, cmd.NewEndPoiID, cmd.NewEstimateFee, time.Now())
	if err != nil {
		return nil, err
	}
	change.ID = uuid.NewString()
	if err := s.destinationChg.Save(ctx, change); err != nil {
		return nil, err
	}
	s.log.Info("destination change requested", zap.String("order_id", cmd.OrderID), zap.String("use_case", "RequestDestinationChange"))
	return dto.FromDestinationChange(change), nil
}

// ConfirmDestinationChange 处理 P10 司机确认改点；乐观锁冲突时重读最新聚合重试一次。
func (s *Service) ConfirmDestinationChange(ctx context.Context, cmd dto.ConfirmDestinationChangeCommand) (*dto.DestinationChangeDTO, error) {
	if err := s.requireDestinationChanges(); err != nil {
		return nil, err
	}
	return s.applyDestinationChange(ctx, cmd.ID, func(c *entity.DestinationChange, now time.Time) error {
		return c.Confirm(now)
	})
}

// RejectDestinationChange 处理 P10 司机拒绝改点。
func (s *Service) RejectDestinationChange(ctx context.Context, cmd dto.RejectDestinationChangeCommand) error {
	if err := s.requireDestinationChanges(); err != nil {
		return err
	}
	_, err := s.applyDestinationChange(ctx, cmd.ID, func(c *entity.DestinationChange, now time.Time) error {
		return c.Reject(now)
	})
	return err
}

// CancelDestinationChange 处理 P10 乘客撤销改点申请。
func (s *Service) CancelDestinationChange(ctx context.Context, cmd dto.CancelDestinationChangeCommand) error {
	if err := s.requireDestinationChanges(); err != nil {
		return err
	}
	_, err := s.applyDestinationChange(ctx, cmd.ID, func(c *entity.DestinationChange, now time.Time) error {
		return c.Cancel(now)
	})
	return err
}

// applyDestinationChange 加载改点聚合、执行状态迁移并以乐观锁保存；版本冲突时重读重试一次。
func (s *Service) applyDestinationChange(ctx context.Context, id string, apply func(*entity.DestinationChange, time.Time) error) (*dto.DestinationChangeDTO, error) {
	for attempt := 0; attempt < 2; attempt++ {
		change, err := s.destinationChg.FindByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := apply(change, time.Now()); err != nil {
			return nil, err
		}
		if err := s.destinationChg.Save(ctx, change); err != nil {
			if errors.Is(err, entity.ErrDestinationChangeVersionConflict) && attempt == 0 {
				continue
			}
			return nil, err
		}
		s.log.Info("destination change updated", zap.String("aggregate_id", id), zap.String("use_case", "ApplyDestinationChange"))
		return dto.FromDestinationChange(change), nil
	}
	return nil, entity.ErrDestinationChangeVersionConflict
}

// ListDestinationChanges 处理 P10 查询订单全部改点申请。
func (s *Service) ListDestinationChanges(ctx context.Context, cmd dto.ListDestinationChangesCommand) (*dto.ListDestinationChangesDTO, error) {
	if err := s.requireDestinationChanges(); err != nil {
		return nil, err
	}
	items, err := s.destinationChg.ListByOrder(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	output := &dto.ListDestinationChangesDTO{Items: make([]*dto.DestinationChangeDTO, 0, len(items)), Total: int64(len(items))}
	for _, item := range items {
		output.Items = append(output.Items, dto.FromDestinationChange(item))
	}
	return output, nil
}

// CreateTripShare 处理 P11 创建行程分享：生成不可推导令牌并按行程状态计算失效时间。
func (s *Service) CreateTripShare(ctx context.Context, cmd dto.CreateTripShareCommand) (*dto.TripShareDTO, error) {
	if err := s.requireTripShares(); err != nil {
		return nil, err
	}
	share, err := entity.NewTripShare(cmd.OrderID, cmd.CreatorID, cmd.OrderEndedAt, time.Now())
	if err != nil {
		return nil, err
	}
	share.ID = uuid.NewString()
	if err := s.tripShares.Save(ctx, share); err != nil {
		return nil, err
	}
	s.log.Info("trip share created", zap.String("order_id", cmd.OrderID), zap.String("use_case", "CreateTripShare"))
	return dto.FromTripShare(share), nil
}

// AccessTripShare 处理 P11 访问行程分享：校验未失效并记录访问（触发限流时仍返回数据）。
func (s *Service) AccessTripShare(ctx context.Context, cmd dto.AccessTripShareCommand) (*dto.TripShareDTO, error) {
	if err := s.requireTripShares(); err != nil {
		return nil, err
	}
	share, err := s.tripShares.FindByToken(ctx, cmd.Token)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	if share.IsExpired(now) {
		return nil, entity.ErrTripShareExpired
	}
	share.RecordAccess(now)
	if err := s.tripShares.Save(ctx, share); err != nil {
		return nil, err
	}
	s.log.Info("trip share accessed", zap.String("share_token", cmd.Token), zap.String("use_case", "AccessTripShare"))
	return dto.FromTripShare(share), nil
}

// RevokeTripShare 处理 P11 撤销行程分享。
func (s *Service) RevokeTripShare(ctx context.Context, cmd dto.RevokeTripShareCommand) error {
	if err := s.requireTripShares(); err != nil {
		return err
	}
	share, err := s.tripShares.FindByToken(ctx, cmd.Token)
	if err != nil {
		return err
	}
	if err := share.Revoke(time.Now()); err != nil {
		return err
	}
	if err := s.tripShares.Save(ctx, share); err != nil {
		return err
	}
	s.log.Info("trip share revoked", zap.String("share_token", cmd.Token), zap.String("use_case", "RevokeTripShare"))
	return nil
}

// QuoteCancelFee 处理 P12 生成取消报价：按司机接单时间判定免费窗口并绑定订单版本。
func (s *Service) QuoteCancelFee(ctx context.Context, cmd dto.QuoteCancelCommand) (*dto.CancelQuoteDTO, error) {
	if err := s.requireCancelQuotes(); err != nil {
		return nil, err
	}
	quote, err := entity.NewCancelQuote(cmd.OrderID, cmd.QuoteNo, cmd.OrderVersion, cmd.AcceptedAt, time.Now(), cmd.ReasonCode)
	if err != nil {
		return nil, err
	}
	quote.ID = uuid.NewString()
	if err := s.cancelQuotes.Save(ctx, quote); err != nil {
		return nil, err
	}
	s.log.Info("cancel quote created", zap.String("order_id", cmd.OrderID), zap.String("use_case", "QuoteCancelFee"))
	return dto.FromCancelQuote(quote), nil
}

// GetLatestCancelQuote 处理 P12 查询订单最新报价。
func (s *Service) GetLatestCancelQuote(ctx context.Context, cmd dto.GetLatestCancelQuoteCommand) (*dto.CancelQuoteDTO, error) {
	if err := s.requireCancelQuotes(); err != nil {
		return nil, err
	}
	quote, err := s.cancelQuotes.FindByOrderLatest(ctx, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	return dto.FromCancelQuote(quote), nil
}

// ConsumeCancelQuote 处理 P12 消费取消报价：校验未使用、未过期、绑定版本一致。
func (s *Service) ConsumeCancelQuote(ctx context.Context, cmd dto.ConsumeCancelQuoteCommand) (*dto.CancelQuoteDTO, error) {
	if err := s.requireCancelQuotes(); err != nil {
		return nil, err
	}
	quote, err := s.cancelQuotes.FindByQuoteNo(ctx, cmd.QuoteNo)
	if err != nil {
		return nil, err
	}
	if err := quote.Consume(cmd.OrderVersion, time.Now()); err != nil {
		return nil, err
	}
	if err := s.cancelQuotes.Save(ctx, quote); err != nil {
		return nil, err
	}
	s.log.Info("cancel quote consumed", zap.String("quote_no", cmd.QuoteNo), zap.String("use_case", "ConsumeCancelQuote"))
	return dto.FromCancelQuote(quote), nil
}
