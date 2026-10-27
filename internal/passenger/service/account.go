package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// ListAddresses 列出账号的常用地址与历史地址。
//
// 常用地址最多二十条，历史地址按选择次数与最近使用时间取前十条。
func (s *Service) ListAddresses(ctx context.Context, accountID string, addrType entity.AddressType) ([]*dto.AddressDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	limit := 10
	if addrType != entity.AddressTypeHistory {
		limit = entity.AddressLimit
	}
	list, err := s.addresses.ListByType(ctx, accountID, addrType, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.AddressDTO, 0, len(list))
	for _, address := range list {
		item := toAddressDTO(address)
		out = append(out, &item)
	}
	return out, nil
}

// SaveAddress 新增或更新常用地址。
//
// 常用地址有二十条上限，超出时直接拒绝并提示先删除。
// 更新时只允许改自己的地址，地址簿按账号隔离做对象级授权。
func (s *Service) SaveAddress(ctx context.Context, accountID string, cmd dto.SaveAddressCommand) (*dto.AddressDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if entity.AddressType(cmd.AddrType) == entity.AddressTypeHistory {
		return nil, entity.ErrInvalidStateTransition
	}
	now := s.now().UTC()

	if cmd.ID != "" {
		existing, err := s.addresses.FindByID(ctx, cmd.ID)
		if err != nil {
			return nil, err
		}
		if existing.AccountID != accountID {
			return nil, entity.ErrForbidden
		}
		existing.Tag = cmd.Tag
		existing.Name = cmd.Name
		existing.Addr = cmd.Addr
		existing.POIID = cmd.POIID
		existing.Lng = cmd.Lng
		existing.Lat = cmd.Lat
		existing.UpdatedAt = now
		if err := s.addresses.Save(ctx, existing); err != nil {
			return nil, err
		}
		item := toAddressDTO(existing)
		return &item, nil
	}

	count, err := s.addresses.CountByType(ctx, accountID, entity.AddressType(cmd.AddrType))
	if err != nil {
		return nil, err
	}
	if count >= entity.AddressLimit {
		return nil, entity.ErrOrderStateConflict
	}
	address, err := entity.NewAddress(s.ids.NewID(), accountID, entity.AddressType(cmd.AddrType), cmd.Tag, cmd.Name, cmd.Addr, cmd.POIID, cmd.Lng, cmd.Lat, now)
	if err != nil {
		return nil, err
	}
	if err := s.addresses.Save(ctx, address); err != nil {
		return nil, err
	}
	item := toAddressDTO(address)
	return &item, nil
}

// DeleteAddress 删除常用地址。
//
// 删除常用地址不影响历史订单的地址快照，老订单仍显示下单时的地址。
func (s *Service) DeleteAddress(ctx context.Context, accountID, addressID string) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	address, err := s.addresses.FindByID(ctx, addressID)
	if err != nil {
		return err
	}
	if address.AccountID != accountID {
		return entity.ErrForbidden
	}
	return s.addresses.Delete(ctx, addressID)
}

// RecordHistory 记录一次选中的地点到历史地址。
//
// 选择次数累加并刷新最近使用时间，列表按这两个维度排序。
func (s *Service) RecordHistory(ctx context.Context, accountID string, cmd dto.RecordHistoryCommand) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	now := s.now().UTC()
	existing, err := s.addresses.ListByType(ctx, accountID, entity.AddressTypeHistory, 0)
	if err != nil {
		return err
	}
	for _, address := range existing {
		if address.POIID == cmd.POIID && cmd.POIID != "" {
			address.UseCount++
			address.LastUsedAt = now
			address.UpdatedAt = now
			return s.addresses.Save(ctx, address)
		}
	}
	address, err := entity.NewAddress(s.ids.NewID(), accountID, entity.AddressTypeHistory, "", cmd.Name, cmd.Addr, cmd.POIID, cmd.Lng, cmd.Lat, now)
	if err != nil {
		return err
	}
	address.UseCount = 1
	address.LastUsedAt = now
	return s.addresses.Save(ctx, address)
}

// ClearHistory 清空历史地址。
func (s *Service) ClearHistory(ctx context.Context, accountID string) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	return s.addresses.ClearHistory(ctx, accountID)
}

// RecommendPickup 返回当前位置附近的上车点。
func (s *Service) RecommendPickup(ctx context.Context, accountID string, lng, lat float64, limit int) ([]*dto.POIDTO, error) {
	if err := validateCoordinate(lng, lat); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 10 {
		limit = 10
	}
	list, err := s.pois.Recommend(ctx, lng, lat, limit)
	if err != nil {
		return nil, err
	}
	out := make([]*dto.POIDTO, 0, len(list))
	for _, poi := range list {
		out = append(out, &dto.POIDTO{
			ID:          poi.ID,
			Name:        poi.Name,
			Addr:        poi.Addr,
			City:        poi.City,
			Lng:         poi.Lng,
			Lat:         poi.Lat,
			HasDistance: true,
			DistanceM:   poi.DistanceM,
		})
	}
	return out, nil
}

// ListCoupons 列出账号可用的优惠券。
func (s *Service) ListCoupons(ctx context.Context, accountID string) ([]*dto.CouponDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	list, err := s.coupons.ListUsable(ctx, accountID, s.now().UTC())
	if err != nil {
		return nil, err
	}
	out := make([]*dto.CouponDTO, 0, len(list))
	for _, coupon := range list {
		out = append(out, &dto.CouponDTO{
			CouponID:  coupon.ID,
			Name:      coupon.Name,
			FaceValue: coupon.FaceValue,
			Threshold: coupon.Threshold,
			ExpiresAt: coupon.ExpiresAt,
		})
	}
	return out, nil
}

// BestCoupon 选出抵扣金额最大的一张券。
//
// 在门槛达标、未过期、未使用的券中挑抵扣最大的一张。
// 指定券试算时，若该券不满足门槛或已过期，直接返回不可用，不做静默降级。
func (s *Service) BestCoupon(ctx context.Context, accountID, couponID string, carType int, amount int64) (*entity.Coupon, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	now := s.now().UTC()
	list, err := s.coupons.ListUsable(ctx, accountID, now)
	if err != nil {
		return nil, err
	}
	if couponID != "" {
		for _, coupon := range list {
			if coupon.ID == couponID {
				if !coupon.Usable(now, carType, amount) {
					return nil, entity.ErrCouponUnavailable
				}
				return coupon, nil
			}
		}
		return nil, entity.ErrCouponUnavailable
	}
	var best *entity.Coupon
	for _, coupon := range list {
		if !coupon.Usable(now, carType, amount) {
			continue
		}
		if best == nil || coupon.FaceValue > best.FaceValue {
			best = coupon
		}
	}
	return best, nil
}

// ListOrders 分页列出该账号的订单。
func (s *Service) ListOrders(ctx context.Context, accountID string, state entity.OrderState, pageNo, pageSize int) (*dto.OrderListDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if pageNo <= 0 {
		pageNo = 1
	}
	// 列表查询单页默认二十条，上限一百条。
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	total, err := s.orders.CountByAccount(ctx, accountID, state)
	if err != nil {
		return nil, err
	}
	list, err := s.orders.ListByAccount(ctx, accountID, state, (pageNo-1)*pageSize, pageSize)
	if err != nil {
		return nil, err
	}
	out := &dto.OrderListDTO{Total: total, List: make([]*dto.OrderDTO, 0, len(list))}
	for _, order := range list {
		out.List = append(out.List, toOrderDTO(order))
	}
	return out, nil
}

// CancelQuote 返回取消订单的报价。
//
// 两段式取消的第一步：只报价不执行取消，乘客确认后带确认标记再次调用。
func (s *Service) CancelQuote(ctx context.Context, accountID, orderID string) (*dto.CancelQuoteDTO, error) {
	order, err := s.ownedOrder(ctx, accountID, orderID)
	if err != nil {
		return nil, err
	}
	if order.State.Terminal() {
		return nil, entity.ErrCancelNotAllowed
	}
	now := s.now().UTC()
	fee := entity.CancelFeeFor(order.State, s.cancelFeeAccepted, s.cancelFeeArrived)
	return &dto.CancelQuoteDTO{
		NeedConfirm: fee > 0,
		CancelFee:   fee,
		State:       string(order.State),
		ExpiresAt:   now.Add(30 * time.Second),
	}, nil
}

// CancelOrder 取消订单。
//
// 取消费为零时直接取消；取消费大于零时必须带确认标记，
// 否则只返回报价让乘客确认，避免误扣。
func (s *Service) CancelOrder(ctx context.Context, cmd dto.CancelOrderCommand) (*dto.CancelQuoteDTO, error) {
	order, err := s.ownedOrder(ctx, cmd.AccountID, cmd.OrderID)
	if err != nil {
		return nil, err
	}
	if order.State.Terminal() {
		return nil, entity.ErrCancelNotAllowed
	}
	now := s.now().UTC()
	fee := entity.CancelFeeFor(order.State, s.cancelFeeAccepted, s.cancelFeeArrived)
	if fee > 0 && !cmd.ConfirmFee {
		// 第一次调用只报价，不执行取消。
		return &dto.CancelQuoteDTO{
			NeedConfirm: true,
			CancelFee:   fee,
			State:       string(order.State),
			ExpiresAt:   now.Add(30 * time.Second),
		}, nil
	}
	from := order.State
	if err := order.ApplyTransition(entity.OrderStateCancelled, now); err != nil {
		return nil, err
	}
	order.CancelFee = fee
	if err := s.orders.UpdateState(ctx, order); err != nil {
		return nil, err
	}
	log := entity.NewOrderStatusLog(s.ids.NewID(), order.ID, from, entity.OrderStateCancelled, cmd.AccountID, cmd.Reason, now)
	if err := s.orders.AppendStatusLog(ctx, log); err != nil {
		return nil, err
	}
	// 券锁定随订单取消释放，避免券被永久占用。
	if order.EstimateNo != "" {
		if estimate, err := s.estimates.FindByNo(ctx, order.EstimateNo); err == nil && estimate.CouponID != "" {
			_ = s.coupons.Unlock(ctx, estimate.CouponID, now)
		}
	}
	return &dto.CancelQuoteDTO{
		NeedConfirm: false,
		CancelFee:   fee,
		State:       string(entity.OrderStateCancelled),
	}, nil
}

// DriverLocation 返回订单当前司机的实时位置。
//
// 超过三十秒没有新位置就标记延迟，乘客端据此提示而不是盯着不动的点。
func (s *Service) DriverLocation(ctx context.Context, accountID, orderID string) (*dto.DriverLocationDTO, error) {
	order, err := s.ownedOrder(ctx, accountID, orderID)
	if err != nil {
		return nil, err
	}
	switch order.State {
	case entity.OrderStateAccepted, entity.OrderStateArrived, entity.OrderStateInService:
	default:
		return nil, entity.ErrDriverNotAssigned
	}
	driver, err := s.drivers.FindByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	delayed := driver.PositionDelayed(now)
	etaMin := 0
	if !delayed {
		etaMin = estimateETAMin(driver.LastLng, driver.LastLat, order.EndLng, order.EndLat)
	}
	return &dto.DriverLocationDTO{
		DriverID:   driver.ID,
		DriverName: driver.Name,
		PlateNo:    driver.PlateNo,
		CarModel:   driver.CarModel,
		Rating:     driver.Rating,
		Lng:        driver.LastLng,
		Lat:        driver.LastLat,
		ETAMin:     etaMin,
		Delayed:    delayed,
		ReportedAt: driver.LastReportAt,
	}, nil
}

// BindPhone 绑定手机号。
//
// 手机号已存在时先验证原账号再绑定渠道，不允许直接把渠道挂到别人账号上。
func (s *Service) BindPhone(ctx context.Context, accountID, phone string) (*dto.BindPhoneDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if err := entity.ValidatePhone(phone); err != nil {
		return nil, err
	}
	now := s.now().UTC()

	existing, err := s.accounts.FindByPhone(ctx, phone)
	if err == nil && existing.ID != accountID {
		// 手机号被别的账号占用，必须先验证原账号。
		return nil, entity.ErrPhoneAlreadyUsed
	}
	if err != nil && !errors.Is(err, entity.ErrAccountNotFound) {
		return nil, err
	}
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	account.Phone = phone
	// 手机号变了，哈希必须跟着重算，否则按新号码查不到这个账号。
	account.PhoneHash = ""
	account.UpdatedAt = now
	if err := s.accounts.UpdatePhone(ctx, account); err != nil {
		return nil, err
	}
	return &dto.BindPhoneDTO{Phone: entity.MaskPhone(phone)}, nil
}

// UnbindChannel 解绑一个第三方渠道。
//
// 账号至少要保留一种可用登录方式，解绑到没有渠道时直接拒绝。
func (s *Service) UnbindChannel(ctx context.Context, accountID string, channel entity.Channel) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	if !channel.Valid() {
		return entity.ErrInvalidChannel
	}
	bindings, err := s.channels.ListByAccount(ctx, accountID)
	if err != nil {
		return err
	}
	target := ""
	remaining := 0
	for _, binding := range bindings {
		if binding.Channel == channel {
			target = binding.ID
			continue
		}
		remaining++
	}
	if target == "" {
		return entity.ErrChannelNotBound
	}
	if remaining == 0 {
		// 全部解绑后账号将无法登录。
		return entity.ErrOrderStateConflict
	}
	return s.channels.Delete(ctx, target)
}

// CancelAccount 申请注销账号。
//
// 注销前必须结清未付账单、无进行中订单、无在途提现，
// 否则会出现乘客被扔在半路的事故。
func (s *Service) CancelAccount(ctx context.Context, accountID string) error {
	if accountID == "" {
		return entity.ErrUnauthorized
	}
	if active, err := s.orders.FindActive(ctx, accountID); err == nil && active != nil {
		return entity.ErrOrderStateConflict
	}
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	now := s.now().UTC()
	account.Status = entity.AccountStatusCanceled
	account.UpdatedAt = now
	if err := s.accounts.Save(ctx, account); err != nil {
		return err
	}
	// 注销后所有刷新令牌立即失效。
	return s.sessions.RevokeByAccount(ctx, accountID, now)
}

// WithdrawCancelAccount 撤回注销申请。
func (s *Service) WithdrawCancelAccount(ctx context.Context, accountID string) error {
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return err
	}
	if account.Status != entity.AccountStatusCanceled {
		return entity.ErrInvalidStateTransition
	}
	account.Status = entity.AccountStatusEnabled
	account.UpdatedAt = s.now().UTC()
	return s.accounts.Save(ctx, account)
}

// ownedOrder 查订单并校验归属，防止越权访问他人订单。
func (s *Service) ownedOrder(ctx context.Context, accountID, orderID string) (*entity.Order, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.AccountID != accountID {
		return nil, entity.ErrForbidden
	}
	return order, nil
}

// estimateETAMin 估算司机到终点的分钟数，按直线距离与平均车速折算。
func estimateETAMin(lng, lat, endLng, endLat float64) int {
	meters := haversineMeters(lng, lat, endLng, endLat)
	// 平均车速按 25 米每秒折算，最少一分钟。
	minutes := int(meters/25/60) + 1
	if minutes < 1 {
		return 1
	}
	return minutes
}

// sortCoupons 按抵扣金额倒序排券。
func sortCoupons(coupons []*entity.Coupon) {
	sort.SliceStable(coupons, func(i, j int) bool {
		return coupons[i].FaceValue > coupons[j].FaceValue
	})
}
