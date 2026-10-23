package service

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// PricingProvider 由计价域提供估价能力。
type PricingProvider interface {
	// Quote 按起终点与车型试算价格。返回的金额单位均为分。
	Quote(ctx context.Context, carType int, startLng, startLat, endLng, endLat float64, couponID string) (*PricingQuote, error)
}

// PricingQuote 是一次计价试算结果。
type PricingQuote struct {
	BaseFee     int64
	ServiceFee  int64
	DiscountFee int64
	PayFee      int64
	DistanceM   int64
	DurationS   int64
	// Version 为命中的计价规则版本，下单时冻结进订单快照。
	Version string
}

// 坐标合法性与间距限制。
const (
	// MinOrderDistanceM 为起终点直线距离下限，低于此值提示重选。
	MinOrderDistanceM = 100
	// MaxRemarkLength 为乘客备注字数上限。
	MaxRemarkLength = 50
)

// Estimate 生成估价单。
//
// 估价单必须归属账号：它同时是价格承诺凭证与防重复下单凭证，
// 凭证不能属于游客。游客看价走 Quote 试算，拿不到估价单。
func (s *Service) Estimate(ctx context.Context, cmd dto.EstimateCommand) (*dto.EstimateDTO, error) {
	if cmd.AccountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if err := validateCoordinate(cmd.StartLng, cmd.StartLat); err != nil {
		return nil, err
	}
	if err := validateCoordinate(cmd.EndLng, cmd.EndLat); err != nil {
		return nil, err
	}
	if err := validateDistance(cmd.StartLng, cmd.StartLat, cmd.EndLng, cmd.EndLat); err != nil {
		return nil, err
	}

	quote, err := s.pricing().Quote(ctx, cmd.CarType, cmd.StartLng, cmd.StartLat, cmd.EndLng, cmd.EndLat, cmd.CouponID)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	estimate, err := entity.NewEstimate(s.ids.NewID(), s.ids.NewEstimateNo(now), cmd.AccountID, cmd.CarType, now)
	if err != nil {
		return nil, err
	}
	estimate.BaseFee = quote.BaseFee
	estimate.ServiceFee = quote.ServiceFee
	estimate.DiscountFee = quote.DiscountFee
	estimate.PayFee = quote.PayFee
	estimate.DistanceM = quote.DistanceM
	estimate.DurationS = quote.DurationS
	estimate.PricingVersion = quote.Version
	estimate.CouponID = cmd.CouponID
	if err := s.estimates.Save(ctx, estimate); err != nil {
		return nil, err
	}
	return &dto.EstimateDTO{
		EstimateNo: estimate.No,
		CarType:    estimate.CarType,
		BaseFee:    estimate.BaseFee,
		ServiceFee: estimate.ServiceFee,
		DiscountFee: estimate.DiscountFee,
		PayFee:     estimate.PayFee,
		DistanceM:  estimate.DistanceM,
		DurationS:  estimate.DurationS,
		ExpiresAt:  estimate.ExpiresAt,
	}, nil
}

// CreateOrder 创建订单。
//
// 三道防线保证不产生重复订单：
// 一是幂等键，相同键重试返回同一订单；
// 二是估价单号，一张估价单只能创建一笔订单；
// 三是乐观锁，估价单消费与订单写入用版本号兜住并发。
func (s *Service) CreateOrder(ctx context.Context, cmd dto.CreateOrderCommand) (*dto.OrderDTO, error) {
	if cmd.AccountID == "" {
		return nil, entity.ErrUnauthorized
	}
	if cmd.IdempotencyKey == "" {
		return nil, entity.ErrInvalidStateTransition
	}
	now := s.now().UTC()

	// 幂等：相同键已存在则直接返回原订单。
	if existing, err := s.orders.FindByIdempotencyKey(ctx, cmd.AccountID, cmd.IdempotencyKey); err == nil && existing != nil {
		return toOrderDTO(existing), nil
	} else if err != nil && !errors.Is(err, entity.ErrOrderNotFound) {
		return nil, err
	}

	// 下单前置：实名通过、账号可用。
	account, err := s.accounts.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if !account.Enabled() {
		return nil, entity.ErrAccountDisabled
	}
	if !account.RealNameVerified {
		return nil, entity.ErrForbidden
	}

	// 估价单校验顺序：存在 → 归属当前账号 → 车型一致 → 未过期 → 未被使用。
	// 这一组排在活动行程检查之前：用户刚拿到的估价单被复用时，
	// 提示估价单问题比提示有进行中订单更贴近真实原因。
	estimate, err := s.estimates.FindByNo(ctx, cmd.EstimateNo)
	if err != nil {
		return nil, entity.ErrEstimateNotFound
	}
	if estimate.AccountID != cmd.AccountID {
		return nil, entity.ErrForbidden
	}
	if estimate.CarType != cmd.CarType {
		return nil, entity.ErrInvalidStateTransition
	}
	if estimate.Used {
		return nil, entity.ErrEstimateUsed
	}
	if estimate.Expired(now) {
		return nil, entity.ErrEstimateExpired
	}

	// 无活动行程才允许下单。
	if active, err := s.orders.FindActive(ctx, cmd.AccountID); err == nil && active != nil {
		return nil, entity.ErrOrderStateConflict
	}

	if err := validateCoordinate(cmd.StartLng, cmd.StartLat); err != nil {
		return nil, err
	}
	if err := validateCoordinate(cmd.EndLng, cmd.EndLat); err != nil {
		return nil, err
	}
	if err := validateDistance(cmd.StartLng, cmd.StartLat, cmd.EndLng, cmd.EndLat); err != nil {
		return nil, err
	}
	if len([]rune(cmd.Remark)) > MaxRemarkLength {
		return nil, entity.ErrInvalidStateTransition
	}

	// 消费估价单。乐观锁失败说明并发已消费，直接返回冲突。
	if err := s.estimates.Consume(ctx, estimate); err != nil {
		return nil, entity.ErrEstimateUsed
	}

	order, err := entity.NewOrder(s.ids.NewID(), s.ids.NewOrderNo(now), cmd.AccountID, now)
	if err != nil {
		return nil, err
	}
	order.CarType = cmd.CarType
	order.StartName = cmd.StartName
	order.StartAddr = cmd.StartAddr
	order.StartLng = cmd.StartLng
	order.StartLat = cmd.StartLat
	order.EndName = cmd.EndName
	order.EndAddr = cmd.EndAddr
	order.EndLng = cmd.EndLng
	order.EndLat = cmd.EndLat
	order.EstimateNo = estimate.No
	order.TotalFee = estimate.PayFee
	order.CouponFee = estimate.DiscountFee
	order.Remark = cmd.Remark
	order.IdempotencyKey = cmd.IdempotencyKey
	// 冻结计价规则快照，规则改价只影响新订单，历史订单金额永不变。
	order.PricingSnapshot = estimate.PricingVersion

	if err := s.orders.Save(ctx, order); err != nil {
		return nil, err
	}
	// 状态留痕，保证任意一单可完整回放。
	log := entity.NewOrderStatusLog(s.ids.NewID(), order.ID, "", entity.OrderStatePending, cmd.AccountID, "创建订单", now)
	if err := s.orders.AppendStatusLog(ctx, log); err != nil {
		return nil, err
	}
	return toOrderDTO(order), nil
}

// GetOrder 返回订单详情。
func (s *Service) GetOrder(ctx context.Context, accountID, orderID string) (*dto.OrderDTO, error) {
	order, err := s.orders.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.AccountID != accountID {
		return nil, entity.ErrForbidden
	}
	return toOrderDTO(order), nil
}

// CurrentOrder 返回进行中的订单，没有时返回空。
func (s *Service) CurrentOrder(ctx context.Context, accountID string) (*dto.OrderDTO, error) {
	order, err := s.orders.FindActive(ctx, accountID)
	if err != nil {
		if errors.Is(err, entity.ErrOrderNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return toOrderDTO(order), nil
}

// QuotePrice 供游客看价。游客不生成估价单，只返回即时价格。
func (s *Service) QuotePrice(ctx context.Context, cmd dto.EstimateCommand) (*dto.EstimateDTO, error) {
	if err := validateCoordinate(cmd.StartLng, cmd.StartLat); err != nil {
		return nil, err
	}
	if err := validateCoordinate(cmd.EndLng, cmd.EndLat); err != nil {
		return nil, err
	}
	quote, err := s.pricing().Quote(ctx, cmd.CarType, cmd.StartLng, cmd.StartLat, cmd.EndLng, cmd.EndLat, cmd.CouponID)
	if err != nil {
		return nil, err
	}
	return &dto.EstimateDTO{
		CarType:    cmd.CarType,
		BaseFee:    quote.BaseFee,
		ServiceFee: quote.ServiceFee,
		DiscountFee: quote.DiscountFee,
		PayFee:     quote.PayFee,
		DistanceM:  quote.DistanceM,
		DurationS:  quote.DurationS,
	}, nil
}

// pricing 返回计价提供方，未配置时返回内置试算。
func (s *Service) pricing() PricingProvider {
	if s.pricingProvider != nil {
		return s.pricingProvider
	}
	return builtinPricing{}
}

// builtinPricing 是没有接计价域时的兜底试算。
// ADAPTER: mock —— 真实实现接入计价域后本实现下线。
type builtinPricing struct{}

// Quote 按直线距离与固定单价试算，金额单位为分。
func (builtinPricing) Quote(ctx context.Context, carType int, startLng, startLat, endLng, endLat float64, couponID string) (*PricingQuote, error) {
	distanceM := int64(haversineMeters(startLng, startLat, endLng, endLat))
	durationS := distanceM / 8
	base := int64(1300)
	perKM := int64(260)
	switch carType {
	case CarTypeCarpool:
		base, perKM = 800, 160
	case CarTypePremium:
		base, perKM = 2500, 480
	}
	extraKm := distanceM - 3000
	if extraKm < 0 {
		extraKm = 0
	}
	mileageFee := extraKm / 1000 * perKM
	minuteFee := durationS / 60 * 50
	total := base + mileageFee + minuteFee
	serviceFee := total * 10 / 100
	pay := total + serviceFee
	return &PricingQuote{
		BaseFee:    total,
		ServiceFee: serviceFee,
		PayFee:     pay,
		DistanceM:  distanceM,
		DurationS:  durationS,
		Version:    "builtin-v1",
	}, nil
}

// validateCoordinate 校验坐标合法性。
func validateCoordinate(lng, lat float64) error {
	if math.IsNaN(lng) || math.IsNaN(lat) {
		return entity.ErrInvalidCoordinate
	}
	if lng < -180 || lng > 180 || lat < -90 || lat > 90 {
		return entity.ErrInvalidCoordinate
	}
	return nil
}

// validateDistance 校验起终点间距。距离过近的订单没有营运价值，
// 且多半是误选，直接拒绝。
func validateDistance(startLng, startLat, endLng, endLat float64) error {
	if haversineMeters(startLng, startLat, endLng, endLat) < MinOrderDistanceM {
		return entity.ErrTooClose
	}
	return nil
}

// haversineMeters 计算两点球面距离，单位米。
func haversineMeters(lng1, lat1, lng2, lat2 float64) int64 {
	const earthRadiusM = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return int64(2 * earthRadiusM * math.Atan2(math.Sqrt(a), math.Sqrt(1-a)))
}

// toOrderDTO 把订单实体转成出参。
func toOrderDTO(order *entity.Order) *dto.OrderDTO {
	if order == nil {
		return nil
	}
	return &dto.OrderDTO{
		OrderID:   order.ID,
		OrderNo:   order.OrderNo,
		State:     string(order.State),
		CarType:   order.CarType,
		StartName: order.StartName,
		StartAddr: order.StartAddr,
		EndName:   order.EndName,
		EndAddr:   order.EndAddr,
		TotalFee:  order.TotalFee,
		CancelFee: order.CancelFee,
		Remark:    order.Remark,
		CreatedAt: order.CreatedAt,
	}
}

// ensure the remark helper stays referenced.
var _ = strings.TrimSpace
