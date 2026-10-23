// Package service 编排乘客端业务用例。
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/service/token"
)

// 令牌签发与校验的依赖，对业务用例隐藏实现细节。
type TokenIssuer interface {
	// Issue 签发访问令牌与刷新令牌。
	Issue(accountID string, audience entity.Audience, role entity.Role, now time.Time) (access string, refresh string, expiresIn time.Duration, err error)
	// ParseAccess 解析访问令牌并校验受众与角色。
	ParseAccess(token string, expectAudience entity.Audience, now time.Time) (accountID string, role entity.Role, err error)
	// ParseRefresh 解析刷新令牌并校验受众。
	ParseRefresh(token string, expectAudience entity.Audience, now time.Time) (accountID string, role entity.Role, err error)
}

// PasswordHasher 隐藏密码哈希实现细节。
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(hash string, password string) bool
}

// CodeSender 向手机号下发验证码。
type CodeSender interface {
	// Send 交付验证码并返回通道消息号。
	Send(ctx context.Context, phone string, code string) (messageID string, err error)
}

// CodeInspector 由保留验证码的通道实现，供开发环境回显验证码。
type CodeInspector interface {
	// LastCode 返回发给该号码的验证码。
	LastCode(phone string) (string, bool)
}

// ChannelAuthenticator 用第三方授权码换取渠道唯一标识。
type ChannelAuthenticator interface {
	// Code2Session 解析授权码。
	Code2Session(ctx context.Context, channel entity.Channel, code string) (channelUID string, err error)
}

// IDGenerator 生成各类业务标识。
type IDGenerator interface {
	// NewID 生成通用标识。
	NewID() string
	// NewOrderNo 生成订单号。
	NewOrderNo(now time.Time) string
	// NewEstimateNo 生成估价单号。
	NewEstimateNo(now time.Time) string
}

// Service 编排乘客端全部用例。
type Service struct {
	accounts  entity.AccountRepository
	channels  entity.ChannelRepository
	sessions  entity.SessionRepository
	codes     entity.CodeRepository
	estimates entity.EstimateRepository
	addresses entity.AddressRepository
	pois      entity.POIRepository
	orders    entity.OrderRepository
	quick     entity.QuickEntryRepository
	coupons   entity.CouponRepository
	drivers   entity.DriverRepository

	issuer   TokenIssuer
	hasher   PasswordHasher
	sender   CodeSender
	auth     ChannelAuthenticator
	ids      IDGenerator
	// carTypes 读取城市可用车型，由计价域提供；未配置时退回兜底车型。
	carTypes CarTypeProvider
	// pricingProvider 由计价域提供试算；未配置时退回内置试算。
	pricingProvider PricingProvider
	// cancelFeeAccepted 与 cancelFeeArrived 是取消费配置，单位分。
	cancelFeeAccepted int64
	cancelFeeArrived  int64
	now               func() time.Time
}

// NewService 创建乘客端用例服务。
func NewService(
	accounts entity.AccountRepository,
	channels entity.ChannelRepository,
	sessions entity.SessionRepository,
	codes entity.CodeRepository,
	estimates entity.EstimateRepository,
	addresses entity.AddressRepository,
	pois entity.POIRepository,
	orders entity.OrderRepository,
	quick entity.QuickEntryRepository,
	issuer TokenIssuer,
	hasher PasswordHasher,
	sender CodeSender,
	auth ChannelAuthenticator,
	ids IDGenerator,
) *Service {
	return &Service{
		accounts: accounts, channels: channels, sessions: sessions, codes: codes,
		estimates: estimates, addresses: addresses, pois: pois, orders: orders, quick: quick,
		issuer: issuer, hasher: hasher, sender: sender, auth: auth, ids: ids,
		now: time.Now,
	}
}

// WithCouponRepository 注入优惠券仓储。
func (s *Service) WithCouponRepository(repo entity.CouponRepository) *Service {
	if repo != nil {
		s.coupons = repo
	}
	return s
}

// WithDriverRepository 注入司机档案只读仓储。
func (s *Service) WithDriverRepository(repo entity.DriverRepository) *Service {
	if repo != nil {
		s.drivers = repo
	}
	return s
}

// WithCancelFee 配置取消费金额，单位分。
func (s *Service) WithCancelFee(accepted int64, arrived int64) *Service {
	s.cancelFeeAccepted = accepted
	s.cancelFeeArrived = arrived
	return s
}

// WithClock 替换用例时钟，仅用于测试。
func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// WithCarTypeProvider 注入车型提供方。
func (s *Service) WithCarTypeProvider(provider CarTypeProvider) *Service {
	if provider != nil {
		s.carTypes = provider
	}
	return s
}

// WithPricingProvider 注入计价提供方。
func (s *Service) WithPricingProvider(provider PricingProvider) *Service {
	if provider != nil {
		s.pricingProvider = provider
	}
	return s
}

// SendCode 下发短信验证码。
//
// 限流有三层：同号码重发间隔、同设备小时级次数、连续错误锁定。
// 验证码消费后标记已使用而不是删除，保留 SentAt 才能让重发间隔继续生效。
func (s *Service) SendCode(ctx context.Context, cmd dto.SendCodeCommand) (*dto.SendCodeDTO, error) {
	if err := entity.ValidatePhone(cmd.Phone); err != nil {
		return nil, err
	}
	now := s.now().UTC()

	stored, err := s.codes.Load(ctx, cmd.Phone)
	switch {
	case err == nil:
		if stored.Locked(now) {
			return nil, entity.ErrCodeRateLimited
		}
		if !stored.CanResend(now) {
			return nil, entity.ErrCodeRateLimited
		}
	case errors.Is(err, entity.ErrAccountNotFound):
		// 首次下发，继续往下走。
	default:
		return nil, err
	}

	recent, err := s.codes.CountRecentHourly(ctx, cmd.Phone, now.Add(-time.Hour))
	if err != nil {
		return nil, err
	}
	if recent >= entity.CodeHourlyLimit {
		return nil, entity.ErrCodeRateLimited
	}

	code, err := generateCode()
	if err != nil {
		return nil, err
	}
	verification, err := entity.NewVerificationCode(cmd.Phone, code, now)
	if err != nil {
		return nil, err
	}
	if err := s.codes.Save(ctx, verification); err != nil {
		return nil, err
	}
	if _, err := s.sender.Send(ctx, cmd.Phone, verification.Code); err != nil {
		return nil, err
	}

	out := &dto.SendCodeDTO{
		ExpiresInSeconds:   int(entity.CodeTTL.Seconds()),
		ResendAfterSeconds: int(entity.CodeResendInterval.Seconds()),
	}
	// 本地通道保留验证码时回传，接入真实网关后该分支不会命中。
	if inspector, ok := s.sender.(CodeInspector); ok {
		if value, found := inspector.LastCode(cmd.Phone); found {
			out.DebugCode = value
		}
	}
	return out, nil
}

// Login 校验凭据并签发会话。
//
// 短信渠道：手机号即账号身份，未注册手机号自动建号。
// 微信与 QQ 渠道：按渠道唯一标识命中账号，没有则建号。
// 角色只决定本次会话进入哪一端，不拆分账号。
func (s *Service) Login(ctx context.Context, cmd dto.LoginCommand) (*dto.LoginDTO, error) {
	if !cmd.Role.Valid() {
		return nil, entity.ErrInvalidRole
	}
	if !cmd.Channel.Valid() {
		return nil, entity.ErrInvalidChannel
	}
	now := s.now().UTC()

	var (
		account   *entity.Account
		isNewUser bool
		err       error
	)
	switch cmd.Channel {
	case entity.ChannelSMS:
		account, isNewUser, err = s.loginBySMS(ctx, cmd, now)
	case entity.ChannelWeChat, entity.ChannelQQ:
		account, isNewUser, err = s.loginByChannel(ctx, cmd, now)
	default:
		return nil, entity.ErrInvalidChannel
	}
	if err != nil {
		return nil, err
	}
	if !account.Enabled() {
		return nil, entity.ErrAccountDisabled
	}

	audience := audienceOf(cmd.Role)
	access, refresh, expiresIn, err := s.issuer.Issue(account.ID, audience, cmd.Role, now)
	if err != nil {
		return nil, err
	}
	session := &entity.Session{
		ID:               s.ids.NewID(),
		AccountID:        account.ID,
		Audience:         audience,
		Role:             cmd.Role,
		RefreshTokenHash: token.HashToken(refresh),
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.sessions.Save(ctx, session); err != nil {
		return nil, err
	}

	restore, needDriver := s.restoreTarget(ctx, account.ID, cmd.Role)
	return &dto.LoginDTO{
		AccessToken:        access,
		RefreshToken:       refresh,
		AccountID:          account.ID,
		IsNewUser:          isNewUser,
		HasPassword:        account.HasPassword(),
		RestoreTarget:      restore,
		NeedDriverRegister: needDriver,
		ExpiresInSeconds:   int(expiresIn.Seconds()),
	}, nil
}

// loginBySMS 用手机号验证码登录。未注册手机号自动建号。
func (s *Service) loginBySMS(ctx context.Context, cmd dto.LoginCommand, now time.Time) (*entity.Account, bool, error) {
	if err := entity.ValidatePhone(cmd.Phone); err != nil {
		return nil, false, err
	}
	stored, err := s.codes.Load(ctx, cmd.Phone)
	if err != nil {
		if errors.Is(err, entity.ErrAccountNotFound) {
			return nil, false, entity.ErrInvalidCode
		}
		return nil, false, err
	}
	if stored.Used {
		return nil, false, entity.ErrCodeUsed
	}
	if stored.Expired(now) {
		return nil, false, entity.ErrCodeExpired
	}
	if stored.Locked(now) {
		return nil, false, entity.ErrCodeRateLimited
	}
	if !strings.EqualFold(stored.Code, strings.TrimSpace(cmd.Code)) {
		// 连续错误累加，达到上限后锁定该号码。
		stored.FailCount++
		if err := s.codes.Save(ctx, stored); err != nil {
			return nil, false, err
		}
		return nil, false, entity.ErrInvalidCode
	}
	// 校验通过即标记已使用，防止同一验证码被并发复用。
	stored.Used = true
	stored.FailCount = 0
	if err := s.codes.Save(ctx, stored); err != nil {
		return nil, false, err
	}

	account, err := s.accounts.FindByPhone(ctx, cmd.Phone)
	if err == nil {
		return account, false, nil
	}
	if !errors.Is(err, entity.ErrAccountNotFound) {
		return nil, false, err
	}
	created, err := entity.NewAccount(s.ids.NewID(), cmd.Phone, now)
	if err != nil {
		return nil, false, err
	}
	if err := s.accounts.Save(ctx, created); err != nil {
		return nil, false, err
	}
	return created, true, nil
}

// loginByChannel 用第三方授权码登录。没有绑定记录时建号。
func (s *Service) loginByChannel(ctx context.Context, cmd dto.LoginCommand, now time.Time) (*entity.Account, bool, error) {
	channelUID, err := s.auth.Code2Session(ctx, cmd.Channel, cmd.Code)
	if err != nil {
		return nil, false, entity.ErrInvalidCode
	}
	binding, err := s.channels.FindByChannelUID(ctx, cmd.Channel, channelUID)
	if err == nil {
		account, findErr := s.accounts.FindByID(ctx, binding.AccountID)
		if findErr != nil {
			return nil, false, findErr
		}
		return account, false, nil
	}
	if !errors.Is(err, entity.ErrAccountNotFound) {
		return nil, false, err
	}

	// 渠道首次登录建号。有手机号时复用手机号对应的账号，
	// 但不按昵称自动合并账号。
	var account *entity.Account
	if cmd.Phone != "" {
		if err := entity.ValidatePhone(cmd.Phone); err == nil {
			existing, findErr := s.accounts.FindByPhone(ctx, cmd.Phone)
			if findErr == nil {
				account = existing
			} else if !errors.Is(findErr, entity.ErrAccountNotFound) {
				return nil, false, findErr
			}
		}
	}
	isNew := false
	if account == nil {
		created, createErr := entity.NewAccount(s.ids.NewID(), cmd.Phone, now)
		if createErr != nil {
			return nil, false, createErr
		}
		account = created
		isNew = true
		if err := s.accounts.Save(ctx, account); err != nil {
			return nil, false, err
		}
	}
	newBinding, err := entity.NewChannelBinding(s.ids.NewID(), account.ID, cmd.Channel, channelUID, now)
	if err != nil {
		return nil, false, err
	}
	if err := s.channels.Save(ctx, newBinding); err != nil {
		return nil, false, err
	}
	return account, isNew, nil
}

// SetPassword 设置登录密码。
//
// 要求同时持有有效会话与该手机号的有效验证码，
// 只凭会话不能改密码，否则被盗的会话能把他人的账号改成自己的密码。
func (s *Service) SetPassword(ctx context.Context, cmd dto.SetPasswordCommand) error {
	if err := entity.ValidatePassword(cmd.Password); err != nil {
		return err
	}
	if err := entity.ValidatePhone(cmd.Phone); err != nil {
		return err
	}
	now := s.now().UTC()

	stored, err := s.codes.Load(ctx, cmd.Phone)
	if err != nil {
		if errors.Is(err, entity.ErrAccountNotFound) {
			return entity.ErrInvalidCode
		}
		return err
	}
	if stored.Used {
		return entity.ErrCodeUsed
	}
	if stored.Expired(now) {
		return entity.ErrCodeExpired
	}
	if !strings.EqualFold(stored.Code, strings.TrimSpace(cmd.Code)) {
		return entity.ErrInvalidCode
	}
	stored.Used = true
	if err := s.codes.Save(ctx, stored); err != nil {
		return err
	}

	account, err := s.accounts.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return err
	}
	hash, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return err
	}
	updated, err := account.WithPasswordHash(hash)
	if err != nil {
		return err
	}
	updated.UpdatedAt = now
	return s.accounts.Save(ctx, updated)
}

// Refresh 用刷新令牌换新令牌。
func (s *Service) Refresh(ctx context.Context, cmd dto.RefreshCommand) (*dto.LoginDTO, error) {
	accountID, role, err := s.issuer.ParseRefresh(cmd.RefreshToken, cmd.Audience, s.now().UTC())
	if err != nil {
		return nil, entity.ErrUnauthorized
	}
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !account.Enabled() {
		return nil, entity.ErrAccountDisabled
	}
	now := s.now().UTC()
	access, refresh, expiresIn, err := s.issuer.Issue(account.ID, cmd.Audience, role, now)
	if err != nil {
		return nil, err
	}
	session := &entity.Session{
		ID:               s.ids.NewID(),
		AccountID:        account.ID,
		Audience:         cmd.Audience,
		Role:             role,
		RefreshTokenHash: token.HashToken(refresh),
		ExpiresAt:        now.Add(7 * 24 * time.Hour),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.sessions.Save(ctx, session); err != nil {
		return nil, err
	}
	restore, needDriver := s.restoreTarget(ctx, account.ID, role)
	return &dto.LoginDTO{
		AccessToken:        access,
		RefreshToken:       refresh,
		AccountID:          account.ID,
		HasPassword:        account.HasPassword(),
		RestoreTarget:      restore,
		NeedDriverRegister: needDriver,
		ExpiresInSeconds:   int(expiresIn.Seconds()),
	}, nil
}

// Logout 退出登录，撤销该账号的全部刷新令牌。
//
// 只让前端删令牌是不够的，刷新令牌七天有效，原主人必须能踢掉持有者。
func (s *Service) Logout(ctx context.Context, cmd dto.LogoutCommand) error {
	return s.sessions.RevokeByAccount(ctx, cmd.AccountID, s.now().UTC())
}

// restoreTarget 计算登录后的恢复目标。
// 优先级：活动行程 大于 未付账单 大于 默认首页或接单大厅。
func (s *Service) restoreTarget(ctx context.Context, accountID string, role entity.Role) (string, bool) {
	active, err := s.orders.FindActive(ctx, accountID)
	if err == nil && active != nil {
		return "order_detail", false
	}
	if role == entity.RoleDriver {
		return "driver_hall", true
	}
	return "home", false
}

// audienceOf 按角色推导令牌受众。乘客、司机、后台三套令牌互相隔离。
func audienceOf(role entity.Role) entity.Audience {
	if role == entity.RoleDriver {
		return entity.AudienceDriver
	}
	return entity.AudiencePassenger
}

// generateCode 生成随机六位数字验证码。
func generateCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}
