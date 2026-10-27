package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/service/token"
)

// fakeTokenIssuer 是令牌签发器的测试替身。
type fakeTokenIssuer struct {
	issued int
}

func (f *fakeTokenIssuer) Issue(accountID string, audience entity.Audience, role entity.Role, now time.Time) (string, string, time.Duration, error) {
	f.issued++
	return "access-" + accountID, "refresh-" + accountID, 30 * time.Minute, nil
}

func (f *fakeTokenIssuer) ParseAccess(t string, expect entity.Audience, now time.Time) (string, entity.Role, error) {
	return "account-1", entity.RolePassenger, nil
}

func (f *fakeTokenIssuer) ParseRefresh(t string, expect entity.Audience, now time.Time) (string, entity.Role, error) {
	return "account-1", entity.RolePassenger, nil
}

// fakePasswordHasher 是密码哈希器的测试替身。
type fakePasswordHasher struct{}

func (fakePasswordHasher) Hash(password string) (string, error) { return "hashed:" + password, nil }

func (fakePasswordHasher) Verify(hash string, password string) bool {
	return hash == "hashed:"+password
}

// fakeCodeSender 是验证码通道的测试替身。
type fakeCodeSender struct {
	last string
}

func (f *fakeCodeSender) Send(ctx context.Context, phone string, code string) (string, error) {
	f.last = phone + ":" + code
	return "msg-1", nil
}

func (f *fakeCodeSender) LastCode(phone string) (string, bool) { return "", false }

// fakeChannelAuthenticator 是第三方渠道的测试替身。
type fakeChannelAuthenticator struct{}

func (fakeChannelAuthenticator) Code2Session(ctx context.Context, channel entity.Channel, code string) (string, error) {
	if code == "" {
		return "", errors.New("empty code")
	}
	return string(channel) + "_uid_" + code, nil
}

// fakeIDGenerator 是标识生成器的测试替身。
type fakeIDGenerator struct {
	seq int
}

func (g *fakeIDGenerator) NewID() string {
	g.seq++
	return "id-" + strconv.Itoa(g.seq)
}

func (g *fakeIDGenerator) NewOrderNo(now time.Time) string {
	g.seq++
	return "HXZ" + strconv.FormatInt(now.UnixNano(), 10)
}

func (g *fakeIDGenerator) NewEstimateNo(now time.Time) string {
	g.seq++
	return "EST" + strconv.FormatInt(now.UnixNano(), 10)
}

// newTestService 构造带全部内存依赖的乘客端服务。
func newTestService(t *testing.T, now time.Time) (*Service, *fakeAccountRepository, *fakeCodeRepository, *fakeOrderRepository, *fakeEstimateRepository) {
	t.Helper()
	accounts := newFakeAccountRepository()
	codes := newFakeCodeRepository()
	orders := newFakeOrderRepository()
	estimates := newFakeEstimateRepository()
	svc := NewService(
		accounts,
		newFakeChannelRepository(),
		newFakeSessionRepository(),
		codes,
		estimates,
		newFakeAddressRepository(),
		&fakePOIRepository{},
		orders,
		newFakeQuickEntryRepository(),
		&fakeTokenIssuer{},
		fakePasswordHasher{},
		&fakeCodeSender{},
		fakeChannelAuthenticator{},
		&fakeIDGenerator{},
	)
	svc.WithClock(func() time.Time { return now })
	return svc, accounts, codes, orders, estimates
}

// seedVerifiedAccount 建一个已实名通过的账号。
func seedVerifiedAccount(t *testing.T, repo *fakeAccountRepository, phone string, now time.Time) *entity.Account {
	t.Helper()
	account, err := entity.NewAccount("account-1", phone, now)
	if err != nil {
		t.Fatalf("NewAccount() error = %v", err)
	}
	account.RealNameVerified = true
	if err := repo.Save(context.Background(), account); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return account
}

func TestLoginBySMSCreatesTheAccountForUnknownPhone(t *testing.T) {
	now := time.Now()
	svc, _, codes, _, _ := newTestService(t, now)

	verification, err := entity.NewVerificationCode("13800000000", "123456", now)
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelSMS,
		Phone:   "13800000000",
		Code:    "123456",
		Role:    entity.RolePassenger,
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if !out.IsNewUser {
		t.Fatal("IsNewUser = false, want true for an unknown phone")
	}
	if out.AccessToken == "" {
		t.Fatal("AccessToken is empty, want a session")
	}
}

func TestLoginBySMSRejectsAWrongCode(t *testing.T) {
	now := time.Now()
	svc, _, codes, _, _ := newTestService(t, now)

	verification, err := entity.NewVerificationCode("13800000000", "123456", now)
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelSMS, Phone: "13800000000", Code: "000000", Role: entity.RolePassenger,
	}); !errors.Is(err, entity.ErrInvalidCode) {
		t.Fatalf("Login() error = %v, want ErrInvalidCode", err)
	}
}

func TestLoginBySMSRejectsAReusedCode(t *testing.T) {
	now := time.Now()
	svc, _, codes, _, _ := newTestService(t, now)

	verification, err := entity.NewVerificationCode("13800000000", "123456", now)
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	cmd := dto.LoginCommand{Channel: entity.ChannelSMS, Phone: "13800000000", Code: "123456", Role: entity.RolePassenger}
	if _, err := svc.Login(context.Background(), cmd); err != nil {
		t.Fatalf("first Login() error = %v", err)
	}
	// 验证码一次性，第二次必须被拒。
	if _, err := svc.Login(context.Background(), cmd); !errors.Is(err, entity.ErrCodeUsed) {
		t.Fatalf("second Login() error = %v, want ErrCodeUsed", err)
	}
}

func TestLoginBySMSRejectsAnExpiredCode(t *testing.T) {
	now := time.Now()
	svc, _, codes, _, _ := newTestService(t, now)

	verification, err := entity.NewVerificationCode("13800000000", "123456", now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelSMS, Phone: "13800000000", Code: "123456", Role: entity.RolePassenger,
	}); !errors.Is(err, entity.ErrCodeExpired) {
		t.Fatalf("Login() error = %v, want ErrCodeExpired", err)
	}
}

func TestLoginRejectsAnInvalidRole(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)
	if _, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelSMS, Phone: "13800000000", Code: "123456", Role: entity.Role("admin"),
	}); !errors.Is(err, entity.ErrInvalidRole) {
		t.Fatalf("Login() error = %v, want ErrInvalidRole", err)
	}
}

func TestLoginByChannelCreatesAndReusesTheAccount(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)

	first, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelWeChat, Code: "wx-1", Role: entity.RolePassenger,
	})
	if err != nil {
		t.Fatalf("first Login() error = %v", err)
	}
	if !first.IsNewUser {
		t.Fatal("IsNewUser = false, want true for a first channel login")
	}
	second, err := svc.Login(context.Background(), dto.LoginCommand{
		Channel: entity.ChannelWeChat, Code: "wx-1", Role: entity.RolePassenger,
	})
	if err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	if second.IsNewUser {
		t.Fatal("IsNewUser = true, want false for a known channel")
	}
	if second.AccountID != first.AccountID {
		t.Fatalf("AccountID = %q, want %q", second.AccountID, first.AccountID)
	}
}

func TestSendCodeEnforcesTheResendInterval(t *testing.T) {
	now := time.Now()
	svc, _, codes, _, _ := newTestService(t, now)

	if _, err := svc.SendCode(context.Background(), dto.SendCodeCommand{Phone: "13800000000"}); err != nil {
		t.Fatalf("first SendCode() error = %v", err)
	}
	// 记录一次下发供小时级统计。
	codes.recordSend("13800000000", now)
	if _, err := svc.SendCode(context.Background(), dto.SendCodeCommand{Phone: "13800000000"}); !errors.Is(err, entity.ErrCodeRateLimited) {
		t.Fatalf("second SendCode() error = %v, want ErrCodeRateLimited", err)
	}
}

func TestSendCodeRejectsAMalformedPhone(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)
	if _, err := svc.SendCode(context.Background(), dto.SendCodeCommand{Phone: "12345"}); !errors.Is(err, entity.ErrInvalidPhone) {
		t.Fatalf("SendCode() error = %v, want ErrInvalidPhone", err)
	}
}

func TestCreateOrderIsIdempotent(t *testing.T) {
	now := time.Now()
	svc, accounts, _, orders, estimates := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, PricingVersion: "v1", ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cmd := dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartName: "张江", StartAddr: "科苑路", StartLng: 121.601, StartLat: 31.2054,
		EndName: "虹桥", EndAddr: "申贵路", EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	}
	first, err := svc.CreateOrder(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first CreateOrder() error = %v", err)
	}
	// 相同幂等键重试必须返回同一订单，不产生第二单。
	second, err := svc.CreateOrder(context.Background(), cmd)
	if err != nil {
		t.Fatalf("second CreateOrder() error = %v", err)
	}
	if first.OrderID != second.OrderID {
		t.Fatalf("OrderID = %q then %q, want the same order", first.OrderID, second.OrderID)
	}
	if len(orders.orders) != 1 {
		t.Fatalf("created %d orders, want 1", len(orders.orders))
	}
}

func TestCreateOrderRejectsAReusedEstimate(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, estimates := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, PricingVersion: "v1", ExpiresAt: now.Add(5 * time.Minute),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	cmd := dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	}
	if _, err := svc.CreateOrder(context.Background(), cmd); err != nil {
		t.Fatalf("first CreateOrder() error = %v", err)
	}
	// 换幂等键但复用同一估价单，仍必须被拒。
	cmd.IdempotencyKey = "key-2"
	if _, err := svc.CreateOrder(context.Background(), cmd); !errors.Is(err, entity.ErrEstimateUsed) {
		t.Fatalf("second CreateOrder() error = %v, want ErrEstimateUsed", err)
	}
}

func TestCreateOrderRejectsAnUnverifiedAccount(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, estimates := newTestService(t, now)
	account, err := entity.NewAccount("account-1", "13800000000", now)
	if err != nil {
		t.Fatalf("NewAccount() error = %v", err)
	}
	account.RealNameVerified = false
	if err := accounts.Save(context.Background(), account); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 14335, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
		IdempotencyKey: "key-1",
	}); !errors.Is(err, entity.ErrForbidden) {
		t.Fatalf("CreateOrder() error = %v, want ErrForbidden", err)
	}
}

func TestCreateOrderRejectsTooClosePoints(t *testing.T) {
	now := time.Now()
	svc, accounts, _, _, estimates := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)
	estimate := &entity.Estimate{
		ID: "est-1", No: "EST-1", AccountID: "account-1", CarType: CarTypeExpress,
		PayFee: 100, ExpiresAt: now.Add(5 * time.Minute), CreatedAt: now, UpdatedAt: now,
	}
	if err := estimates.Save(context.Background(), estimate); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := svc.CreateOrder(context.Background(), dto.CreateOrderCommand{
		AccountID: "account-1", EstimateNo: "EST-1", CarType: CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.6011, EndLat: 31.2055,
		IdempotencyKey: "key-1",
	}); !errors.Is(err, entity.ErrTooClose) {
		t.Fatalf("CreateOrder() error = %v, want ErrTooClose", err)
	}
}

func TestEstimateIsRejectedForAVisitor(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)
	// 游客没有账号，估价单必须归属账号，拿不到估价单。
	if _, err := svc.Estimate(context.Background(), dto.EstimateCommand{
		CarType:  CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	}); !errors.Is(err, entity.ErrUnauthorized) {
		t.Fatalf("Estimate() error = %v, want ErrUnauthorized", err)
	}
}

func TestQuotePriceWorksForAVisitor(t *testing.T) {
	now := time.Now()
	svc, _, _, _, _ := newTestService(t, now)
	out, err := svc.QuotePrice(context.Background(), dto.EstimateCommand{
		CarType:  CarTypeExpress,
		StartLng: 121.601, StartLat: 31.2054, EndLng: 121.3208, EndLat: 31.1979,
	})
	if err != nil {
		t.Fatalf("QuotePrice() error = %v", err)
	}
	if out.PayFee <= 0 {
		t.Fatalf("PayFee = %d, want a positive amount", out.PayFee)
	}
	// 游客拿不到估价单。
	if out.EstimateNo != "" {
		t.Fatalf("EstimateNo = %q, want empty for a visitor", out.EstimateNo)
	}
}

func TestSetPasswordRequiresTheVerificationCode(t *testing.T) {
	now := time.Now()
	svc, accounts, codes, _, _ := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)

	verification, err := entity.NewVerificationCode("13800000000", "123456", now)
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := svc.SetPassword(context.Background(), dto.SetPasswordCommand{
		AccountID: "account-1", Phone: "13800000000", Code: "123456", Password: "abc123",
	}); err != nil {
		t.Fatalf("SetPassword() error = %v", err)
	}
	account, err := accounts.FindByID(context.Background(), "account-1")
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !account.HasPassword() {
		t.Fatal("the account has no password, want the stored digest")
	}
}

func TestSetPasswordRejectsAWeakPassword(t *testing.T) {
	now := time.Now()
	svc, accounts, codes, _, _ := newTestService(t, now)
	seedVerifiedAccount(t, accounts, "13800000000", now)
	verification, err := entity.NewVerificationCode("13800000000", "123456", now)
	if err != nil {
		t.Fatalf("NewVerificationCode() error = %v", err)
	}
	if err := codes.Save(context.Background(), verification); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	for _, password := range []string{"123", "abcdef", "12345678"} {
		if err := svc.SetPassword(context.Background(), dto.SetPasswordCommand{
			AccountID: "account-1", Phone: "13800000000", Code: "123456", Password: password,
		}); !errors.Is(err, entity.ErrInvalidPassword) {
			t.Fatalf("SetPassword(%q) error = %v, want ErrInvalidPassword", password, err)
		}
	}
}

// 令牌受众隔离必须成立：乘客令牌不能当司机令牌用。
func TestTokenAudienceIsolatesSurfaces(t *testing.T) {
	issuer := token.NewIssuer("test-secret")
	now := time.Now()
	access, _, _, err := issuer.Issue("account-1", entity.AudiencePassenger, entity.RolePassenger, now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, _, err := issuer.ParseAccess(access, entity.AudienceDriver, now); !errors.Is(err, token.ErrAudienceDenied) {
		t.Fatalf("ParseAccess() error = %v, want ErrAudienceDenied", err)
	}
	if _, _, err := issuer.ParseAccess(access, entity.AudiencePassenger, now); err != nil {
		t.Fatalf("ParseAccess() error = %v, want the passenger audience accepted", err)
	}
}

// 刷新令牌不能当访问令牌用。
func TestRefreshTokenCannotBeUsedAsAccessToken(t *testing.T) {
	issuer := token.NewIssuer("test-secret")
	now := time.Now()
	_, refresh, _, err := issuer.Issue("account-1", entity.AudiencePassenger, entity.RolePassenger, now)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, _, err := issuer.ParseAccess(refresh, entity.AudiencePassenger, now); !errors.Is(err, token.ErrInvalidToken) {
		t.Fatalf("ParseAccess() error = %v, want ErrInvalidToken", err)
	}
}
