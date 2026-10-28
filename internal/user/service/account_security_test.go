package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
)

// fakeAccountSecurityRepo 以内存 map 模拟 accounts 集合的绑定/协议读写。
type fakeAccountSecurityRepo struct {
	security map[string]*entity.AccountSecurity
	writes   int
}

var _ entity.AccountSecurityRepository = (*fakeAccountSecurityRepo)(nil)

func newFakeAccountSecurityRepo() *fakeAccountSecurityRepo {
	return &fakeAccountSecurityRepo{security: map[string]*entity.AccountSecurity{}}
}

func (f *fakeAccountSecurityRepo) GetAccountSecurity(_ context.Context, accountID string) (*entity.AccountSecurity, error) {
	sec, ok := f.security[accountID]
	if !ok {
		return nil, nil
	}
	cp := *sec
	return &cp, nil
}

func (f *fakeAccountSecurityRepo) SaveProtocolVer(_ context.Context, accountID, protocolVer string) error {
	f.writes++
	sec, ok := f.security[accountID]
	if !ok {
		sec = &entity.AccountSecurity{}
		f.security[accountID] = sec
	}
	sec.ProtocolVer = protocolVer
	return nil
}

// TestGetAccountSecurity_ListBindings：绑定列表（含未绑定渠道）+ 协议版本 + 实名状态。
func TestGetAccountSecurity_ListBindings(t *testing.T) {
	ctx := context.Background()
	repo := newFakeAccountSecurityRepo()
	t1 := time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	repo.security["acc-1"] = &entity.AccountSecurity{
		Bindings: []entity.ChannelBinding{
			{Channel: "wechat", ChannelUID: "wx-1", BoundAt: t1},
			{Channel: "sms", ChannelUID: "138****0000", BoundAt: t2},
		},
		ProtocolVer:   "v2.0",
		RealNameState: entity.RealNameStatePassed,
	}
	svc := NewAccountSecurityService(repo)

	got, err := svc.GetAccountSecurity(ctx, "acc-1")
	require.NoError(t, err)
	require.Len(t, got.Bindings, 3)

	byChannel := map[string]dto.ChannelBindingStatus{}
	for _, b := range got.Bindings {
		byChannel[b.Channel] = b
	}
	require.True(t, byChannel["wechat"].Bound)
	require.Equal(t, t1.Unix(), byChannel["wechat"].BoundAt)
	require.False(t, byChannel["qq"].Bound)
	require.EqualValues(t, 0, byChannel["qq"].BoundAt)
	require.True(t, byChannel["sms"].Bound)
	require.Equal(t, t2.Unix(), byChannel["sms"].BoundAt)

	require.Equal(t, "v2.0", got.ProtocolVer)
	require.Equal(t, entity.RealNameStatePassed, got.RealNameState)

	// 无记录账号：全未绑定、协议版本空、未认证（不 500）
	empty, err := svc.GetAccountSecurity(ctx, "acc-9")
	require.NoError(t, err)
	require.Len(t, empty.Bindings, 3)
	for _, b := range empty.Bindings {
		require.False(t, b.Bound)
	}
	require.Empty(t, empty.ProtocolVer)
	require.Equal(t, entity.RealNameStateNone, empty.RealNameState)
}

// TestRecordAgreement_Versioned：协议同意记录版本号；同版本幂等不重复记。
func TestRecordAgreement_Versioned(t *testing.T) {
	ctx := context.Background()
	repo := newFakeAccountSecurityRepo()
	svc := NewAccountSecurityService(repo)

	got, err := svc.RecordAgreement(ctx, "acc-1", "v1.0")
	require.NoError(t, err)
	require.Equal(t, "v1.0", got.ProtocolVer)
	require.Equal(t, 1, repo.writes)
	require.Equal(t, "v1.0", repo.security["acc-1"].ProtocolVer)

	// 同版本重试：幂等不重复记
	got, err = svc.RecordAgreement(ctx, "acc-1", "v1.0")
	require.NoError(t, err)
	require.Equal(t, "v1.0", got.ProtocolVer)
	require.Equal(t, 1, repo.writes)

	// 版本升级：正常记录
	got, err = svc.RecordAgreement(ctx, "acc-1", "v2.0")
	require.NoError(t, err)
	require.Equal(t, "v2.0", got.ProtocolVer)
	require.Equal(t, 2, repo.writes)

	// 版本号必填
	_, err = svc.RecordAgreement(ctx, "acc-1", "")
	require.Error(t, err)
	requireAppCode(t, err, "40001")
}
