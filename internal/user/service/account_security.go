package service

import (
	"context"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// knownBindingChannels 是登录模块支持的渠道全集（契约 3.1.2：wechat/qq/sms）。
var knownBindingChannels = []string{"wechat", "qq", "sms"}

// AccountSecurityService 编排 P21 账号与安全用例（契约 3.7.2）。
// 只读绑定列表 + 记录协议版本；换绑/解绑本体复用登录模块 3.1.6。
type AccountSecurityService struct {
	repo entity.AccountSecurityRepository
}

// NewAccountSecurityService 创建账号安全用例服务。
func NewAccountSecurityService(repo entity.AccountSecurityRepository) *AccountSecurityService {
	return &AccountSecurityService{repo: repo}
}

// GetAccountSecurity 返回绑定列表+协议版本+实名状态（GET /api/pax/account-security）。
func (s *AccountSecurityService) GetAccountSecurity(ctx context.Context, accountID string) (*dto.AccountSecurityStatus, error) {
	sec, err := s.repo.GetAccountSecurity(ctx, accountID)
	if err != nil {
		return nil, err
	}
	status := &dto.AccountSecurityStatus{
		Bindings:      enumerateBindings(sec),
		RealNameState: entity.RealNameStateNone,
	}
	if sec != nil {
		status.ProtocolVer = sec.ProtocolVer
		status.RealNameState = sec.RealNameState
	}
	return status, nil
}

// RecordAgreement 记录协议同意版本（POST /api/pax/account-security）。
// 幂等：同版本不重复记，返回当前版本。
func (s *AccountSecurityService) RecordAgreement(ctx context.Context, accountID, protocolVer string) (*dto.AgreementRecord, error) {
	if strings.TrimSpace(protocolVer) == "" {
		return nil, apperrors.New(apperrors.KindInvalidArgument, "40001", "protocol_ver is required")
	}
	sec, err := s.repo.GetAccountSecurity(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if sec != nil && sec.ProtocolVer == protocolVer {
		return &dto.AgreementRecord{ProtocolVer: protocolVer}, nil
	}
	if err := s.repo.SaveProtocolVer(ctx, accountID, protocolVer); err != nil {
		return nil, err
	}
	return &dto.AgreementRecord{ProtocolVer: protocolVer}, nil
}

// enumerateBindings 以渠道全集枚举绑定状态（未出现渠道=未绑定），并兼容未知渠道追加。
func enumerateBindings(sec *entity.AccountSecurity) []dto.ChannelBindingStatus {
	byChannel := map[string]entity.ChannelBinding{}
	if sec != nil {
		for _, b := range sec.Bindings {
			byChannel[b.Channel] = b
		}
	}
	out := make([]dto.ChannelBindingStatus, 0, len(knownBindingChannels))
	seen := map[string]bool{}
	for _, channel := range knownBindingChannels {
		seen[channel] = true
		b, ok := byChannel[channel]
		out = append(out, toBindingStatus(channel, ok, b))
	}
	for channel, b := range byChannel {
		if !seen[channel] {
			out = append(out, toBindingStatus(channel, true, b))
		}
	}
	return out
}

// toBindingStatus 将绑定条目转为展示状态；列表中出现即绑定。
func toBindingStatus(channel string, bound bool, b entity.ChannelBinding) dto.ChannelBindingStatus {
	st := dto.ChannelBindingStatus{Channel: channel, Bound: bound}
	if bound {
		st.BoundAt = b.BoundAt.Unix()
	}
	return st
}
