package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/BwCloudWeGo/bw-cli/pkg/biztime"
	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// 司机登录令牌配置。
const (
	// DriverTokenTTL 为司机端令牌有效期。
	DriverTokenTTL = 12 * time.Hour
	// DriverTokenRole 司机端角色标识，供网关区分乘客/司机令牌。
	DriverTokenRole = "driver"
	// DriverTokenIssuerFallback 配置缺失时的签发者兜底值。
	DriverTokenIssuerFallback = "xiaolanshu"
)

// tokenIssuer 取令牌签发者：优先用与网关同一份配置的 issuer。
func (s *Service) tokenIssuer() string {
	if strings.TrimSpace(s.auth.TokenIssuer) != "" {
		return s.auth.TokenIssuer
	}
	return DriverTokenIssuerFallback
}

// AuthConfig 司机登录所需的令牌签发配置。
type AuthConfig struct {
	// TokenSecret 令牌签发密钥（与网关 PASSENGER_TOKEN_SECRET 同源）。
	TokenSecret string
	// TokenIssuer 令牌签发者，需与网关 middleware.jwt.issuer 一致。
	TokenIssuer string
}

// SendDriverCodeResult 验证码下发结果。
type SendDriverCodeResult struct {
	Phone     string
	DebugCode string
	ExpiresIn int64
	ResendIn  int64
}

// DriverLoginResult 登录结果。
type DriverLoginResult struct {
	Token     string
	ExpiresIn int64
	Driver    *dto.DriverProfileDTO
	IsNew     bool
}

// SendDriverCode 下发登录验证码：同号码受重发间隔限制。
func (s *Service) SendDriverCode(ctx context.Context, cmd dto.SendDriverCodeCommand) (*SendDriverCodeResult, error) {
	phone, err := entity.NormalizePhone(cmd.Phone)
	if err != nil {
		return nil, err
	}
	now := biztime.UTC()
	existing, err := s.codes.Load(ctx, phone)
	if err != nil {
		return nil, err
	}
	if existing != nil && !existing.CanResend(now) {
		wait := existing.ResendWait(now)
		return &SendDriverCodeResult{
			Phone:    phone,
			ResendIn: int64(wait.Seconds() + 0.5),
		}, entity.ErrCodeResendSoon
	}
	code, err := generateDriverCode()
	if err != nil {
		return nil, err
	}
	record, err := entity.NewVerificationCode(phone, code, now)
	if err != nil {
		return nil, err
	}
	if err := s.codes.Save(ctx, record); err != nil {
		return nil, err
	}
	s.log.Info("driver login code issued", zap.String("phone", maskPhone(phone)))
	return &SendDriverCodeResult{
		Phone:     phone,
		DebugCode: code,
		ExpiresIn: int64(entity.CodeTTL.Seconds()),
	}, nil
}

// DriverLogin 手机号 + 验证码登录：号码未建档时自动建档，返回可复用的司机档案。
// 这样司机换设备后重新登录即可找回此前的收入与提现记录。
func (s *Service) DriverLogin(ctx context.Context, cmd dto.DriverLoginCommand) (*DriverLoginResult, error) {
	phone, err := entity.NormalizePhone(cmd.Phone)
	if err != nil {
		return nil, err
	}
	now := biztime.UTC()
	record, err := s.codes.Load(ctx, phone)
	if err != nil {
		return nil, err
	}
	// 校验失败要回写错误次数，连续错误达到上限后锁定该号码。
	if err := record.Verify(strings.TrimSpace(cmd.Code), now); err != nil {
		if record != nil {
			_ = s.codes.Save(ctx, record)
		}
		return nil, err
	}

	driver, err := s.drivers.FindByPhone(ctx, phone)
	isNew := false
	if err == entity.ErrDriverNotFound {
		// 首次登录缺姓名：验证码保持有效，等用户补齐信息后继续，避免白扣一次验证码。
		if strings.TrimSpace(cmd.Name) == "" {
			return nil, entity.ErrNameRequired
		}
		// 验证码已验证手机号归属，直接建档。
		driver, err = s.registerWithPhone(ctx, strings.TrimSpace(cmd.Name), phone)
		if err != nil {
			return nil, err
		}
		isNew = true
	} else if err != nil {
		return nil, err
	}

	// 登录流程走完才消耗验证码，防止同一个码被重复使用。
	record.MarkUsed()
	if err := s.codes.Save(ctx, record); err != nil {
		return nil, err
	}

	token, err := authn.IssueToken(
		authn.Config{Secret: s.auth.TokenSecret, Issuer: s.tokenIssuer()},
		driver.ID, DriverTokenRole, DriverTokenTTL, now,
	)
	if err != nil {
		s.log.Error("issue driver token failed", zap.String("driver_id", driver.ID), zap.Error(err))
		return nil, err
	}
	return &DriverLoginResult{
		Token:     token,
		ExpiresIn: int64(DriverTokenTTL.Seconds()),
		Driver:    dto.FromDriver(driver),
		IsNew:     isNew,
	}, nil
}

// registerWithPhone 登录即建档：复用注册用例，保证档案结构与事件一致。
func (s *Service) registerWithPhone(ctx context.Context, name, phone string) (*entity.Driver, error) {
	if strings.TrimSpace(name) == "" {
		return nil, entity.ErrNameRequired
	}
	driver, err := entity.NewDriver(name, phone)
	if err != nil {
		return nil, err
	}
	if err := s.drivers.Create(ctx, driver); err != nil {
		return nil, err
	}
	s.emit(ctx, &contracts.DriverStatusChangedPayload{
		EventID:    "dsc-" + driver.ID + "-" + fmt.Sprint(driver.CreatedAt.UnixNano()),
		DriverID:   driver.ID,
		FromStatus: "",
		ToStatus:   driver.Status,
		Reason:     "login_register",
		OccurredAt: biztime.RFC3339(driver.CreatedAt),
	}, contracts.EventDriverStatusChanged)
	return driver, nil
}

// generateDriverCode 生成定长数字验证码（密码学随机）。
func generateDriverCode() (string, error) {
	const digits = "0123456789"
	buf := make([]byte, entity.CodeLength)
	for i := range buf {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		buf[i] = digits[n.Int64()]
	}
	return string(buf), nil
}

// maskPhone 日志脱敏：只保留前三位与后四位。
func maskPhone(phone string) string {
	if len(phone) != 11 {
		return "***"
	}
	return phone[:3] + "****" + phone[7:]
}
