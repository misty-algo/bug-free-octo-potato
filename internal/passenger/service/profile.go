package service

import (
	"context"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/passenger/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/passenger/entity"
)

// 档案字段长度上限，防止超长输入污染库与前端排版。
const (
	maxNicknameLength      = 20
	maxEmergencyNameLength = 20
)

// GetProfile 查询注册引导所需的个人资料，紧急联系电话只回传掩码值。
func (s *Service) GetProfile(ctx context.Context, accountID string) (*dto.ProfileDTO, error) {
	account, err := s.accounts.FindByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &dto.ProfileDTO{
		AccountID:          account.ID,
		Nickname:           account.Nickname,
		Avatar:             account.Avatar,
		EmergencyName:      account.EmergencyName,
		EmergencyPhoneMask: entity.MaskPhone(account.EmergencyPhone),
		Completed:          account.ProfileCompleted(),
	}, nil
}

// UpdateProfile 补全个人资料：昵称、头像与紧急联系人。
// 昵称与紧急联系人齐全才算完成注册引导（头像可选）。
func (s *Service) UpdateProfile(ctx context.Context, cmd dto.UpdateProfileCommand) (*dto.ProfileDTO, error) {
	account, err := s.accounts.FindByID(ctx, accountIDOf(cmd.AccountID))
	if err != nil {
		return nil, err
	}
	if account.Status == entity.AccountStatusCanceled {
		return nil, entity.ErrAccountCanceled
	}
	nickname := strings.TrimSpace(cmd.Nickname)
	if nickname != "" {
		if len([]rune(nickname)) > maxNicknameLength {
			return nil, entity.ErrProfileFieldTooLong
		}
		account.Nickname = nickname
	}
	if avatar := strings.TrimSpace(cmd.Avatar); avatar != "" {
		account.Avatar = avatar
	}
	emergencyName := strings.TrimSpace(cmd.EmergencyName)
	emergencyPhone := strings.TrimSpace(cmd.EmergencyPhone)
	if emergencyName != "" || emergencyPhone != "" {
		if len([]rune(emergencyName)) > maxEmergencyNameLength {
			return nil, entity.ErrProfileFieldTooLong
		}
		// 紧急联系人要么不填，要么姓名与电话成对提交，避免存半条记录。
		if emergencyName == "" || emergencyPhone == "" {
			return nil, entity.ErrEmergencyContactIncomplete
		}
		if err := entity.ValidatePhone(emergencyPhone); err != nil {
			return nil, err
		}
		account.EmergencyName = emergencyName
		account.EmergencyPhone = emergencyPhone
	}
	account.Touch(s.now().UTC())
	if err := s.accounts.Save(ctx, account); err != nil {
		return nil, err
	}
	return &dto.ProfileDTO{
		AccountID:          account.ID,
		Nickname:           account.Nickname,
		Avatar:             account.Avatar,
		EmergencyName:      account.EmergencyName,
		EmergencyPhoneMask: entity.MaskPhone(account.EmergencyPhone),
		Completed:          account.ProfileCompleted(),
	}, nil
}

// accountIDOf 归一化账号标识，避免空白值穿透到仓储层。
func accountIDOf(id string) string { return strings.TrimSpace(id) }
