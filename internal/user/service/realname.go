package service

import (
	"context"
	"strings"
	"time"

	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/clock"
	"github.com/BwCloudWeGo/bw-cli/pkg/crypto"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// realNameDailyLimit 每账号每日有效提交上限（契约 3.7.1）。
const realNameDailyLimit = 3

// RealNameService 编排 P20 实名认证用例（契约 3.7.1）。
// 每账号每日 3 次有效提交；同证件仅一个有效主账号；已认证不可改。
type RealNameService struct {
	repo     entity.RealNameAccountRepository
	verifier entity.RealNameVerifier
	key      []byte // security.data_key，32 字节，三件套加密用
	clk      clock.Clock
}

// NewRealNameService 创建实名认证用例服务。
func NewRealNameService(repo entity.RealNameAccountRepository, verifier entity.RealNameVerifier, key []byte, clk clock.Clock) *RealNameService {
	return &RealNameService{repo: repo, verifier: verifier, key: key, clk: clk}
}

// SimulatedRealNameVerifier 课程模拟二要素适配器（契约 1.6），默认返回「认证中」。
type SimulatedRealNameVerifier struct{}

// Verify 模拟二要素校验：恒为认证中，等待外部回调/查询结果。
func (SimulatedRealNameVerifier) Verify(context.Context, string, string) (entity.RealNameVerdict, error) {
	return entity.RealNameVerdictPending, nil
}

// GetRealNameStatus 查询实名状态（GET /api/pax/realname），只回脱敏值。
func (s *RealNameService) GetRealNameStatus(ctx context.Context, accountID string) (*dto.RealNameStatus, error) {
	auth, err := s.repo.GetRealNameAuth(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if auth == nil {
		return &dto.RealNameStatus{State: entity.RealNameStateNone}, nil
	}
	return toRealNameStatus(auth), nil
}

// SubmitRealName 提交实名认证（POST /api/pax/realname）。
// 同幂等键同内容重试不计次；适配器超时/错误保「认证中」。
func (s *RealNameService) SubmitRealName(ctx context.Context, cmd dto.SubmitRealNameCommand) (*dto.RealNameStatus, error) {
	if strings.TrimSpace(cmd.Name) == "" || strings.TrimSpace(cmd.IDNo) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" {
		return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "name, id_no and idempotency_key are required")
	}
	nameHash := crypto.SHA256Hex(cmd.Name)
	idNoHash := crypto.SHA256Hex(cmd.IDNo)
	keyHash := crypto.SHA256Hex(cmd.IdempotencyKey)

	auth, err := s.repo.GetRealNameAuth(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	sameContent := auth != nil && auth.NameHash == nameHash && auth.IDNoHash == idNoHash

	// 已认证不可改：异参拒绝（40901），同参幂等返回当前状态。
	if auth != nil && auth.State == entity.RealNameStatePassed {
		if !sameContent {
			return nil, apperrors.New(apperrors.KindConflict, "40901", "real name already verified and immutable")
		}
		return toRealNameStatus(auth), nil
	}

	// 每账号每日 3 次有效提交，跨日清零。
	today := s.clk.Now().Format("2006-01-02")
	count := 0
	idemKeys := []string{}
	if auth != nil && auth.SubmitDay == today {
		count = auth.SubmitCount
		idemKeys = auth.IdemKeyHashes
	}
	// 同幂等键同内容重试：不计次、不重复校验。
	if sameContent && containsKey(idemKeys, keyHash) {
		return toRealNameStatus(auth), nil
	}
	if count >= realNameDailyLimit {
		return nil, apperrors.New(apperrors.KindConflict, "42901", "real name submit daily limit exceeded")
	}

	// 同证件仅一个有效主账号（D5）；本人重提排除自身。
	holder, err := s.repo.FindValidHolderByIDNo(ctx, idNoHash, cmd.IDNo, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if holder != "" {
		return nil, apperrors.New(apperrors.KindConflict, "42201", "id_no already bound to a valid account")
	}

	// 三件套写入（D6）：Enc/Hash/Mask，不落真值。
	nameEnc, err := crypto.EncryptAESGCM(s.key, cmd.Name)
	if err != nil {
		return nil, err
	}
	idNoEnc, err := crypto.EncryptAESGCM(s.key, cmd.IDNo)
	if err != nil {
		return nil, err
	}

	state := entity.RealNameStatePending
	var authAt *time.Time
	// 超时/错误保「认证中」，再查询结果（契约 3.7.1）。
	verdict, err := s.verifier.Verify(ctx, cmd.Name, cmd.IDNo)
	if err != nil {
		verdict = entity.RealNameVerdictPending
	}
	switch verdict {
	case entity.RealNameVerdictPassed:
		state = entity.RealNameStatePassed
		now := s.clk.Now().UTC()
		authAt = &now
	case entity.RealNameVerdictFailed:
		state = entity.RealNameStateFailed
	}

	saved := &entity.RealNameAuth{
		NameEnc:       nameEnc,
		NameHash:      nameHash,
		NameMask:      crypto.MaskName(cmd.Name),
		IDNoEnc:       idNoEnc,
		IDNoHash:      idNoHash,
		IDNoMask:      crypto.MaskIDNo(cmd.IDNo),
		State:         state,
		SubmitCount:   count + 1,
		SubmitDay:     today,
		IdemKeyHashes: append(append([]string{}, idemKeys...), keyHash),
		AuthAt:        authAt,
	}
	if err := s.repo.SaveRealNameAuth(ctx, cmd.AccountID, saved); err != nil {
		return nil, err
	}
	return toRealNameStatus(saved), nil
}

// toRealNameStatus 只输出 state 与脱敏值（响应不回真值）。
func toRealNameStatus(auth *entity.RealNameAuth) *dto.RealNameStatus {
	return &dto.RealNameStatus{State: auth.State, NameMask: auth.NameMask, IDNoMask: auth.IDNoMask}
}

// containsKey 判断幂等键哈希是否已在当日集合中。
func containsKey(keys []string, keyHash string) bool {
	for _, k := range keys {
		if k == keyHash {
			return true
		}
	}
	return false
}
