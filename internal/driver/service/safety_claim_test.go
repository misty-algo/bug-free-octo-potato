package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/driver/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/driver/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
)

func staffPrincipal() authn.Principal {
	return authn.Principal{UserID: "S1", Role: "safety_handler"}
}

func staffPrincipal2() authn.Principal {
	return authn.Principal{UserID: "S2", Role: "safety_handler"}
}

// TestClaimStateMachine 校验 RECEIVED → CLAIMED → PROCESSING，且版本冲突拒绝。
func TestClaimStateMachine(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, err := svc.Report(ctx, reportCmd("REQ-C1"))
	require.NoError(t, err)
	require.Equal(t, entity.SafetyStateReceived, created.State)
	require.Equal(t, int64(1), created.Version)

	claimed, err := svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1, RequestID: "r1",
	})
	require.NoError(t, err)
	require.Equal(t, entity.SafetyStateClaimed, claimed.State)
	require.Equal(t, "S1", claimed.Handler)
	require.Equal(t, int64(2), claimed.Version)

	// 旧版本再领取：拒绝，仅一个处理员成功。
	_, err = svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal2(), EventID: created.EventID, ExpectedVersion: 1, RequestID: "r2",
	})
	require.Error(t, err)
	require.Equal(t, "VERSION_CONFLICT", errorCode(err))
	require.Equal(t, "S1", repo.byID[created.EventID].Handler, "领取人不变")

	// 开始处理：仅当前处理人可推进。
	processing, err := svc.Start(ctx, dto.SafetyStartCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 2, RequestID: "r3",
	})
	require.NoError(t, err)
	require.Equal(t, entity.SafetyStateProcessing, processing.State)

	// 非处理人 start：拒绝。
	_, err = svc.Start(ctx, dto.SafetyStartCommand{
		Principal: staffPrincipal2(), EventID: created.EventID, ExpectedVersion: 3, RequestID: "r4",
	})
	require.Error(t, err)
}

// TestResolveRequiresResultAndNote 校验办结必填结果与处置说明。
func TestResolveRequiresResultAndNote(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-R1"))
	claimed, _ := svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1,
	})

	// 缺结果：拒绝。
	_, err := svc.Resolve(ctx, dto.SafetyResolveCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version, Note: "已处理",
	})
	require.Equal(t, "INVALID_RESOLVE_RESULT", errorCode(err))

	// 缺说明：拒绝。
	_, err = svc.Resolve(ctx, dto.SafetyResolveCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version, Result: entity.ResultAssisted,
	})
	require.Equal(t, "RESOLVE_NOTE_REQUIRED", errorCode(err))

	// 完整：办结。
	resolved, err := svc.Resolve(ctx, dto.SafetyResolveCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version,
		Result: entity.ResultAssisted, Note: "已联系乘客，双方和解",
	})
	require.NoError(t, err)
	require.Equal(t, entity.SafetyStateResolved, resolved.State)

	// 办结后再追加处置：拒绝（终态）。
	_, err = svc.AddAction(ctx, dto.SafetyAddActionCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: resolved.Version, Kind: entity.SafetyActionNote, Note: "晚了",
	})
	require.Equal(t, "VERSION_CONFLICT", errorCode(err))
}

// TestTransferPreservesFacts 校验转派不改 state、不改 claimedAt。
func TestTransferPreservesFacts(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-T1"))
	claimed, _ := svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1,
	})
	claimedAt := claimed.ClaimedAt

	// 缺原因：拒绝。
	_, err := svc.Transfer(ctx, dto.SafetyTransferCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version, To: "S2",
	})
	require.Equal(t, "TRANSFER_REASON_REQUIRED", errorCode(err))

	transferred, err := svc.Transfer(ctx, dto.SafetyTransferCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version,
		To: "S2", Reason: "夜班交接", RequestID: "r-t",
	})
	require.NoError(t, err)
	require.Equal(t, "S2", transferred.Handler)
	require.Equal(t, entity.SafetyStateClaimed, transferred.State, "转派不改状态")
	require.Equal(t, claimedAt.Unix(), transferred.ClaimedAt.Unix(), "受理/领取事实保留")
}

// TestAddActionKeepsState 校验追加处置保持状态，只增不改。
func TestAddActionKeepsState(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-A1"))
	claimed, _ := svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1,
	})
	updated, err := svc.AddAction(ctx, dto.SafetyAddActionCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: claimed.Version,
		Kind: entity.SafetyActionNote, Note: "已电话联系司机", RequestID: "r-a",
	})
	require.NoError(t, err)
	require.Equal(t, entity.SafetyStateClaimed, updated.State, "追加处置后仍是已领取")
	require.Equal(t, 2, updated.Actions, "领取 + 追加各一条")
}

// TestEscalateAfterFiveMinutes 校验升级按 received_at 判定，与重分配/重启无关。
func TestEscalateAfterFiveMinutes(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-E1"))
	require.Equal(t, 0, len(repo.byID[created.EventID].Escalations))

	// 未满 5 分钟：不升级。
	early := created.ReceivedAt.Add(time.Duration(entity.EscalationAfter)*time.Second - time.Minute)
	res, err := svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: early})
	require.NoError(t, err)
	require.Equal(t, int32(0), res.Escalated)

	// 满 5 分钟：升级，状态仍 RECEIVED。
	late := created.ReceivedAt.Add(time.Duration(entity.EscalationAfter)*time.Second + time.Second)
	res, err = svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: late})
	require.NoError(t, err)
	require.Equal(t, int32(1), res.Escalated)
	event := repo.byID[created.EventID]
	require.Len(t, event.Escalations, 1)
	require.Equal(t, entity.EscalationLevelUnclaimed, event.Escalations[0].Level)
	require.Equal(t, entity.SafetyStateReceived, event.State, "升级不改状态")

	// 再次扫描：档位唯一，不重复升级。
	res, err = svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: late.Add(time.Minute)})
	require.NoError(t, err)
	require.Equal(t, int32(0), res.Escalated)
	require.Len(t, repo.byID[created.EventID].Escalations, 1)
}

// TestEscalateSkippedAfterClaim 校验已领取后不再产生未领取升级，但已有升级保留。
func TestEscalateSkippedAfterClaim(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-E2"))
	late := created.ReceivedAt.Add(time.Duration(entity.EscalationAfter)*time.Second + time.Second)
	// 先升级一次（事实保留）。
	res, _ := svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: late})
	require.Equal(t, int32(1), res.Escalated)

	// 领取后旧定时器再扫：不新增升级，但历史保留。
	_, _ = svc.Claim(ctx, dto.SafetyClaimCommand{
		Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1,
	})
	res, err := svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: late.Add(time.Minute)})
	require.NoError(t, err)
	require.Equal(t, int32(0), res.Escalated)
	require.Len(t, repo.byID[created.EventID].Escalations, 1, "已有升级保留历史")
}

// TestS03InterleavedClaimBeforeEscalate 强制交错：升级预检暂停 → 领取提交 → 恢复，
// 证明升级不会误触发。
func TestS03InterleavedClaimBeforeEscalate(t *testing.T) {
	repo := newFakeSafetyRepo()
	svc := newSafetyService(repo, &fakeTaskReader{}, &fakeLocation{})
	ctx := context.Background()

	created, _ := svc.Report(ctx, reportCmd("REQ-S03"))
	late := created.ReceivedAt.Add(time.Duration(entity.EscalationAfter)*time.Second + time.Second)

	// 在升级的条件更新执行前，插入领取提交。
	repo.beforeEscalate = func() {
		_, err := svc.Claim(ctx, dto.SafetyClaimCommand{
			Principal: staffPrincipal(), EventID: created.EventID, ExpectedVersion: 1,
		})
		require.NoError(t, err)
	}

	res, err := svc.EscalateTick(ctx, dto.SafetyEscalateTickCommand{Principal: staffPrincipal(), Now: late})
	require.NoError(t, err)
	require.Equal(t, int32(0), res.Escalated, "领取先提交，升级被跳过")
	require.Equal(t, int32(1), res.Skipped)
	event := repo.byID[created.EventID]
	require.Equal(t, entity.SafetyStateClaimed, event.State)
	require.Empty(t, event.Escalations, "不产生误升级")
}
