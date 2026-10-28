package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/internal/user/dto"
	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
)

// serviceConfigKeys 是客服页读取的参数字典 key（A18；faq key 待补，附录 D 登记）。
var serviceConfigKeys = []string{"hotline", "service_time", "service_spec", "faq"}

// ConfigService 编排客服配置读取（P22，契约 3.7.3/3.9.6）。
type ConfigService struct {
	repo entity.ServiceConfigRepository
}

// NewConfigService 创建客服配置服务。
func NewConfigService(repo entity.ServiceConfigRepository) *ConfigService {
	return &ConfigService{repo: repo}
}

// GetServiceConfig 返回 hotline/service_time/service_spec 现值与 FAQ 数组文本。
// key 无记录返回空串（不 500）；FAQ 无 faq key 时按 service_spec 现值兜底，
// 均非 JSON 数组时缺省空数组，等 A18 补 faq key 后自然生效（附录 D 登记）。
func (s *ConfigService) GetServiceConfig(ctx context.Context) (*dto.ServiceConfig, error) {
	values, err := s.repo.GetPublishedValues(ctx, serviceConfigKeys)
	if err != nil {
		return nil, err
	}
	cfg := &dto.ServiceConfig{
		Hotline:     values["hotline"],
		ServiceTime: values["service_time"],
		ServiceSpec: values["service_spec"],
		FAQJSON:     "[]",
	}
	for _, candidate := range []string{values["faq"], values["service_spec"]} {
		if isJSONArray(candidate) {
			cfg.FAQJSON = candidate
			break
		}
	}
	return cfg, nil
}

// isJSONArray 判断现值是否为 JSON 数组文本（FAQ 形态）。
func isJSONArray(value string) bool {
	trimmed := strings.TrimSpace(value)
	return strings.HasPrefix(trimmed, "[") && json.Valid([]byte(trimmed))
}
