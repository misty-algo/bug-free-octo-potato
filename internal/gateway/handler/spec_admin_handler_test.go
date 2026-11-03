package handler

// spec_admin_handler_test.go 覆盖管理端券模板发布（A07）的 HTTP 适配行为。
//
// 该接口曾只写进程内 store，导致管理端发布后乘客领券中心看不到券；
// 现在必须经乘客服务落库，因此这里断言「确实调用了发布 RPC」以及关键失败路径。

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	passengerv1 "github.com/BwCloudWeGo/bw-cli/api/gen/passenger/v1"
	"github.com/BwCloudWeGo/bw-cli/pkg/httpx"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

// fakePublishCouponsClient 只实现发布券 RPC，其余方法由内嵌接口占位（调用即 panic）。
type fakePublishCouponsClient struct {
	passengerv1.PassengerServiceClient
	err   error
	calls int
	got   *passengerv1.PublishPlatformCouponsRequest
}

func (f *fakePublishCouponsClient) PublishPlatformCoupons(
	_ context.Context, in *passengerv1.PublishPlatformCouponsRequest, _ ...grpc.CallOption,
) (*passengerv1.PublishPlatformCouponsResponse, error) {
	f.calls++
	f.got = in
	if f.err != nil {
		return nil, f.err
	}
	return &passengerv1.PublishPlatformCouponsResponse{TemplateNo: in.GetTemplateNo(), Issued: in.GetStock()}, nil
}

// newAdminCouponTestContext 构造最小 gin 上下文与 handler。
func newAdminCouponTestContext(t *testing.T, client passengerv1.PassengerServiceClient, at time.Time) (*gin.Context, *httptest.ResponseRecorder, *SpecContractHandler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := middleware.JWTConfig{Secret: "spec-admin-secret", Issuer: "spec-admin-issuer", ExpireSeconds: 1800}
	h := NewSpecContractHandler(cfg, zap.NewNop(), client).WithClock(&specFakeClock{at: at})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w, h
}

// TestAdminCouponTemplate 覆盖券模板发布的成功路径与关键失败路径。
func TestAdminCouponTemplate(t *testing.T) {
	at := time.Unix(1700000000, 0).UTC()
	validBody := `{"name":"满20减5","face_value":500,"threshold":2000,"valid_from":1700000000,"valid_to":1700100000,"stock":10}`

	t.Run("成功发布时把参数透传给乘客服务", func(t *testing.T) {
		client := &fakePublishCouponsClient{}
		c, w, h := newAdminCouponTestContext(t, client, at)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/coupon/template", strings.NewReader(validBody))
		c.Request.Header.Set("Content-Type", "application/json")

		h.AdminCouponTemplate(c)

		if w.Code != http.StatusOK {
			t.Fatalf("HTTP 状态: got %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
		}
		if client.calls != 1 {
			t.Fatalf("发布 RPC 调用次数: got %d, want 1", client.calls)
		}
		if got, want := client.got.GetFaceValueFen(), int64(500); got != want {
			t.Errorf("face_value_fen: got %d, want %d", got, want)
		}
		if got, want := client.got.GetThresholdFen(), int64(2000); got != want {
			t.Errorf("threshold_fen: got %d, want %d", got, want)
		}
		if got, want := client.got.GetExpireAtUnix(), int64(1700100000); got != want {
			t.Errorf("expire_at_unix: got %d, want %d", got, want)
		}
		if got, want := client.got.GetStock(), int32(10); got != want {
			t.Errorf("stock: got %d, want %d", got, want)
		}
		if !strings.HasPrefix(client.got.GetTemplateNo(), "CT") {
			t.Errorf("模板号应以 CT 开头: got %q", client.got.GetTemplateNo())
		}

		var body struct {
			Code int `json:"code"`
			Data struct {
				TemplateID string `json:"template_id"`
				Issued     int64  `json:"issued"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
		}
		if got, want := body.Data.Issued, int64(10); got != want {
			t.Errorf("issued: got %d, want %d", got, want)
		}
		if got, want := body.Data.TemplateID, client.got.GetTemplateNo(); got != want {
			t.Errorf("template_id: got %q, want %q", got, want)
		}
	})

	t.Run("库存为 0 时拒绝且不调用发布 RPC", func(t *testing.T) {
		client := &fakePublishCouponsClient{}
		c, w, h := newAdminCouponTestContext(t, client, at)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/coupon/template",
			strings.NewReader(`{"face_value":500,"threshold":2000,"valid_from":1700000000,"valid_to":1700100000,"stock":0}`))
		c.Request.Header.Set("Content-Type", "application/json")

		h.AdminCouponTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("HTTP 状态: got %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
		if client.calls != 0 {
			t.Errorf("非法参数不应调用发布 RPC: got %d 次, want 0", client.calls)
		}
	})

	t.Run("司机券不走乘客领券中心", func(t *testing.T) {
		client := &fakePublishCouponsClient{}
		c, w, h := newAdminCouponTestContext(t, client, at)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/coupon/template",
			strings.NewReader(`{"bearer":"driver","face_value":500,"threshold":2000,"valid_from":1700000000,"valid_to":1700100000,"stock":1}`))
		c.Request.Header.Set("Content-Type", "application/json")

		h.AdminCouponTemplate(c)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("HTTP 状态: got %d, want %d, body=%s", w.Code, http.StatusBadRequest, w.Body.String())
		}
		if client.calls != 0 {
			t.Errorf("司机券不应调用平台券发布 RPC: got %d 次, want 0", client.calls)
		}
	})

	t.Run("乘客服务失败时返回网关错误", func(t *testing.T) {
		client := &fakePublishCouponsClient{err: errors.New("passenger unavailable")}
		c, w, h := newAdminCouponTestContext(t, client, at)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/coupon/template", strings.NewReader(validBody))
		c.Request.Header.Set("Content-Type", "application/json")

		h.AdminCouponTemplate(c)

		if w.Code != http.StatusBadGateway {
			t.Fatalf("HTTP 状态: got %d, want %d, body=%s", w.Code, http.StatusBadGateway, w.Body.String())
		}
		var body httpx.PaxBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}
		if body.Code == httpx.PaxCodeOK {
			t.Errorf("失败路径业务码不应为成功: got %d", body.Code)
		}
	})

	t.Run("未注入乘客服务时返回 503", func(t *testing.T) {
		c, w, h := newAdminCouponTestContext(t, nil, at)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/admin/coupon/template", strings.NewReader(validBody))
		c.Request.Header.Set("Content-Type", "application/json")

		h.AdminCouponTemplate(c)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("HTTP 状态: got %d, want %d, body=%s", w.Code, http.StatusServiceUnavailable, w.Body.String())
		}
	})
}
