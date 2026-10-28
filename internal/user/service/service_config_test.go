package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/BwCloudWeGo/bw-cli/internal/user/entity"
)

// fakeServiceConfigRepo 返回预置的已发布配置现值。
type fakeServiceConfigRepo struct {
	values map[string]string
}

var _ entity.ServiceConfigRepository = (*fakeServiceConfigRepo)(nil)

func (f *fakeServiceConfigRepo) GetPublishedValues(_ context.Context, keys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range keys {
		if v, ok := f.values[k]; ok {
			out[k] = v
		}
	}
	return out, nil
}

// TestGetServiceConfig_FallbackFaq：hotline/service_time 透传、无记录空串（不 500）、
// FAQ 缺 faq key 时按 service_spec 现值兜底，均非 JSON 数组时缺省空数组。
func TestGetServiceConfig_FallbackFaq(t *testing.T) {
	ctx := context.Background()

	// 无 faq/service_spec：FAQ 缺省空数组，hotline/service_time 透传，无记录 key 空串
	svc := NewConfigService(&fakeServiceConfigRepo{values: map[string]string{
		"hotline":      "400-123",
		"service_time": "07:00-23:00",
	}})
	got, err := svc.GetServiceConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "400-123", got.Hotline)
	require.Equal(t, "07:00-23:00", got.ServiceTime)
	require.Empty(t, got.ServiceSpec)
	require.JSONEq(t, "[]", got.FAQJSON)

	// faq key 现值优先
	svc = NewConfigService(&fakeServiceConfigRepo{values: map[string]string{
		"faq":          `[{"q":"如何开发票","a":"行程结束后在订单详情申请"}]`,
		"service_spec": `[{"q":"兜底不应出现"}]`,
	}})
	got, err = svc.GetServiceConfig(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `[{"q":"如何开发票","a":"行程结束后在订单详情申请"}]`, got.FAQJSON)

	// 无 faq key：按 service_spec 现值出（FAQ 兜底），等 A18 补 faq key 后自然生效
	svc = NewConfigService(&fakeServiceConfigRepo{values: map[string]string{
		"service_spec": `[{"q":"服务规范条目","a":"规范说明"}]`,
	}})
	got, err = svc.GetServiceConfig(ctx)
	require.NoError(t, err)
	require.JSONEq(t, `[{"q":"服务规范条目","a":"规范说明"}]`, got.FAQJSON)
	require.Equal(t, `[{"q":"服务规范条目","a":"规范说明"}]`, got.ServiceSpec)

	// faq 非法 JSON：回空数组，不 500
	svc = NewConfigService(&fakeServiceConfigRepo{values: map[string]string{
		"faq": "not-json",
	}})
	got, err = svc.GetServiceConfig(ctx)
	require.NoError(t, err)
	require.JSONEq(t, "[]", got.FAQJSON)
}
