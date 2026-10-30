package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
	"github.com/BwCloudWeGo/bw-cli/pkg/crypto"
)

// realNameIDNoPattern 校验 15/18 位身份证号格式。
var realNameIDNoPattern = regexp.MustCompile(`^\d{15}$|^\d{17}[0-9Xx]$`)

// SubmitRealName 提交实名认证（P20）。
//
// 本地演示通道：格式合法即写回账号实名状态，让「实名 → 叫车」的链路
// 可以在联调环境闭环；正式环境应替换为二要素校验服务并按状态机
// （未认证→认证中→已认证/失败）推进，写回时机为认证通过。
// 提交天然幂等：已认证的账号重复提交直接返回当前状态，不产生副作用。
// 真值不落库，只保存脱敏后的姓名与证件号。
func (s *Service) SubmitRealName(ctx context.Context, cmd dto.SubmitRealNameCommand) (*dto.RealNameStatusDTO, error) {
	if cmd.AccountID == "" {
		return nil, entity.ErrUnauthorized
	}
	name := strings.TrimSpace(cmd.Name)
	idNo := strings.TrimSpace(cmd.IDNo)
	if name == "" || !realNameIDNoPattern.MatchString(idNo) {
		return nil, entity.ErrInvalidRealName
	}
	account, err := s.accounts.FindByID(ctx, cmd.AccountID)
	if err != nil {
		return nil, err
	}
	if !account.RealNameVerified {
		account.RealNameVerified = true
		account.RealNameName = crypto.MaskName(name)
		account.RealNameIDNo = crypto.MaskIDNo(idNo)
		account.UpdatedAt = s.now().UTC()
		if err := s.accounts.Save(ctx, account); err != nil {
			return nil, err
		}
	}
	return realNameStatus(account), nil
}

// GetRealNameStatus 返回实名状态，只含状态与脱敏值。
func (s *Service) GetRealNameStatus(ctx context.Context, accountID string) (*dto.RealNameStatusDTO, error) {
	if accountID == "" {
		return nil, entity.ErrUnauthorized
	}
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return realNameStatus(account), nil
}

// realNameStatus 把账号实体映射为实名状态出参。
func realNameStatus(account *entity.Account) *dto.RealNameStatusDTO {
	state := dto.RealNameStateUnverified
	if account.RealNameVerified {
		state = dto.RealNameStateVerified
	}
	return &dto.RealNameStatusDTO{
		State:    state,
		NameMask: account.RealNameName,
		IDNoMask: account.RealNameIDNo,
	}
}
