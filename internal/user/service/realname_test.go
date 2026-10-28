package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/crypto"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// fixedClock 返回固定时刻，用于日限跨日等时间敏感用例。
type fixedClock struct{ t time.Time }

func (f *fixedClock) Now() time.Time { return f.t }

// fakeRealNameRepo 以内存 map 模拟 accounts 集合的实名子文档读写。
type fakeRealNameRepo struct {
	auths map[string]*entity.RealNameAuth
}

var _ entity.RealNameAccountRepository = (*fakeRealNameRepo)(nil)

func newFakeRealNameRepo() *fakeRealNameRepo {
	return &fakeRealNameRepo{auths: map[string]*entity.RealNameAuth{}}
}

func (f *fakeRealNameRepo) GetRealNameAuth(_ context.Context, accountID string) (*entity.RealNameAuth, error) {
	auth, ok := f.auths[accountID]
	if !ok {
		return nil, nil
	}
	cp := *auth
	return &cp, nil
}

func (f *fakeRealNameRepo) SaveRealNameAuth(_ context.Context, accountID string, auth *entity.RealNameAuth) error {
	cp := *auth
	f.auths[accountID] = &cp
	return nil
}

func (f *fakeRealNameRepo) FindValidHolderByIDNo(_ context.Context, idNoHash, idNoPlain, excludeAccountID string) (string, error) {
	for accountID, auth := range f.auths {
		if accountID == excludeAccountID || auth.State == entity.RealNameStateFailed {
			continue
		}
		// 新写入比哈希；存量明文兼容路径由 idNoPlain 兜底（内存比对）。
		if auth.IDNoHash == idNoHash && idNoHash != "" {
			return accountID, nil
		}
		if auth.IDNoHash == "" && idNoPlain != "" && crypto.SHA256Hex(idNoPlain) == idNoHash {
			return accountID, nil
		}
	}
	return "", nil
}

// fakeVerifier 可配置结论/错误的二要素适配器替身。
type fakeVerifier struct {
	verdict entity.RealNameVerdict
	err     error
}

var _ entity.RealNameVerifier = (*fakeVerifier)(nil)

func (f *fakeVerifier) Verify(_ context.Context, _, _ string) (entity.RealNameVerdict, error) {
	return f.verdict, f.err
}

// testKey 返回 32 字节样例密钥（security.data_key 形态）。
func testKey() []byte {
	return []byte("0123456789abcdef0123456789abcdef")
}

func requireAppCode(t *testing.T, err error, code string) {
	t.Helper()
	appErr, ok := apperrors.As(err)
	require.True(t, ok, "期望应用错误，得到 %v", err)
	require.Equal(t, code, appErr.Code)
}

func realNameCmd(accountID, name, idNo, key string) dto.SubmitRealNameCommand {
	return dto.SubmitRealNameCommand{
		AccountID:      accountID,
		Name:           name,
		IDNo:           idNo,
		IdempotencyKey: key,
	}
}

// TestSubmitRealName_DailyLimit：每账号每日 3 次有效提交；幂等重试不计次；跨日清零。
func TestSubmitRealName_DailyLimit(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	repo := newFakeRealNameRepo()
	verifier := &fakeVerifier{verdict: entity.RealNameVerdictPending}
	clk := fixedClock{t: base}
	svc := NewRealNameService(repo, verifier, testKey(), &clk)

	for i, key := range []string{"k1", "k2", "k3"} {
		got, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", key))
		require.NoError(t, err, "第 %d 次提交应成功", i+1)
		require.Equal(t, entity.RealNameStatePending, got.State)
		require.Equal(t, i+1, repo.auths["acc-1"].SubmitCount)
	}

	// 同幂等键重试不计次
	_, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k2"))
	require.NoError(t, err)
	require.Equal(t, 3, repo.auths["acc-1"].SubmitCount)

	// 第 4 次有效提交 → 42901
	_, err = svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k4"))
	require.Error(t, err)
	requireAppCode(t, err, "42901")

	// 跨日清零：次日重新计数
	clk.t = base.AddDate(0, 0, 1)
	got, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k5"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, got.State)
	require.Equal(t, 1, repo.auths["acc-1"].SubmitCount)
}

// TestSubmitRealName_IdNoUnique：同证件仅一个有效主账号，重复 → 42201。
func TestSubmitRealName_IdNoUnique(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRealNameRepo()
	verifier := &fakeVerifier{verdict: entity.RealNameVerdictPending}
	svc := NewRealNameService(repo, verifier, testKey(), &fixedClock{t: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)})

	_, err := svc.SubmitRealName(ctx, realNameCmd("acc-a", "张三丰", "110101199001011234", "a1"))
	require.NoError(t, err)

	// 另一账号提交同一证件 → 42201
	_, err = svc.SubmitRealName(ctx, realNameCmd("acc-b", "张三丰", "110101199001011234", "b1"))
	require.Error(t, err)
	requireAppCode(t, err, "42201")

	// 换证件即可提交
	got, err := svc.SubmitRealName(ctx, realNameCmd("acc-b", "李四", "110101199002022345", "b2"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, got.State)

	// 本人换幂等键重提同一证件不冲突（排除自身）
	_, err = svc.SubmitRealName(ctx, realNameCmd("acc-a", "张三丰", "110101199001011234", "a2"))
	require.NoError(t, err)
}

// TestSubmitRealName_MaskedOutput：响应只出 state + 脱敏姓名/证件号，不回真值。
func TestSubmitRealName_MaskedOutput(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRealNameRepo()
	verifier := &fakeVerifier{verdict: entity.RealNameVerdictPending}
	svc := NewRealNameService(repo, verifier, testKey(), &fixedClock{t: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)})

	got, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k1"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, got.State)
	require.Equal(t, crypto.MaskName("张三丰"), got.NameMask)
	require.Equal(t, crypto.MaskIDNo("110101199001011234"), got.IDNoMask)
	require.NotContains(t, got.NameMask, "张三丰")
	require.NotContains(t, got.IDNoMask, "1101011990")

	// 查询同样只出脱敏值
	status, err := svc.GetRealNameStatus(ctx, "acc-1")
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, status.State)
	require.Equal(t, got.NameMask, status.NameMask)
	require.Equal(t, got.IDNoMask, status.IDNoMask)

	// 落库只存三件套（Enc/Hash/Mask），不落真值
	stored := repo.auths["acc-1"]
	require.NotEmpty(t, stored.NameEnc)
	require.NotEmpty(t, stored.IDNoEnc)
	require.Equal(t, crypto.SHA256Hex("张三丰"), stored.NameHash)
	require.Equal(t, crypto.SHA256Hex("110101199001011234"), stored.IDNoHash)
}

// TestSubmitRealName_PassedImmutable：已认证不可改（异参拒绝 40901）；同参幂等重试不计次。
func TestSubmitRealName_PassedImmutable(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRealNameRepo()
	verifier := &fakeVerifier{verdict: entity.RealNameVerdictPassed}
	svc := NewRealNameService(repo, verifier, testKey(), &fixedClock{t: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)})

	got, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k1"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePassed, got.State)
	require.NotNil(t, repo.auths["acc-1"].AuthAt)

	// 改姓名 / 改证件均不可改 → 40901
	_, err = svc.SubmitRealName(ctx, realNameCmd("acc-1", "张四丰", "110101199001011234", "k2"))
	require.Error(t, err)
	requireAppCode(t, err, "40901")
	_, err = svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199003033456", "k3"))
	require.Error(t, err)
	requireAppCode(t, err, "40901")

	// 同参幂等重试：返回已认证且不计次
	got2, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k4"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePassed, got2.State)
	require.Equal(t, 1, repo.auths["acc-1"].SubmitCount)
}

// TestSubmitRealName_StateMachine：未认证→认证中→已认证/认证失败；超时保认证中。
func TestSubmitRealName_StateMachine(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRealNameRepo()
	verifier := &fakeVerifier{verdict: entity.RealNameVerdictPending}
	svc := NewRealNameService(repo, verifier, testKey(), &fixedClock{t: time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)})

	// 初始未认证
	status, err := svc.GetRealNameStatus(ctx, "acc-1")
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStateNone, status.State)

	// 模拟适配器默认「认证中」
	got, err := svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k1"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, got.State)

	// 适配器超时：保持认证中
	verifier.err = context.DeadlineExceeded
	got, err = svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k2"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePending, got.State)

	// 适配器通过：已认证
	verifier.err = nil
	verifier.verdict = entity.RealNameVerdictPassed
	got, err = svc.SubmitRealName(ctx, realNameCmd("acc-1", "张三丰", "110101199001011234", "k3"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePassed, got.State)

	// 适配器拒绝：认证失败（另一账号走失败分支）
	verifier.verdict = entity.RealNameVerdictFailed
	got, err = svc.SubmitRealName(ctx, realNameCmd("acc-2", "李四", "110101199002022345", "b1"))
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStateFailed, got.State)

	// 已认证态保持可查
	status, err = svc.GetRealNameStatus(ctx, "acc-1")
	require.NoError(t, err)
	require.Equal(t, entity.RealNameStatePassed, status.State)
}
