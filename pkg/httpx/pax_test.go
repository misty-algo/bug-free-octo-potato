package httpx

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

func newPaxTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

func TestPaxOKShape(t *testing.T) {
	c, w := newPaxTestContext(t)

	PaxOK(c, gin.H{"total": 1})

	var body PaxBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应非 JSON: %v", err)
	}
	if body.Code != 0 || body.Message != "success" {
		t.Fatalf("信封错误: code=%d message=%q", body.Code, body.Message)
	}
	if body.Data == nil {
		t.Fatal("data 不应为 nil")
	}
}

func TestPaxErrorShape(t *testing.T) {
	c, w := newPaxTestContext(t)

	PaxError(c, 400, 40001, "参数错误")

	var body PaxBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应非 JSON: %v", err)
	}
	if body.Code != 40001 || body.Message != "参数错误" {
		t.Fatalf("信封错误: code=%d message=%q", body.Code, body.Message)
	}
	if w.Code != 400 {
		t.Fatalf("HTTP 状态 = %d, want 400", w.Code)
	}
}

func TestPaxAppErrorMapsKind(t *testing.T) {
	cases := []struct {
		err  *apperrors.AppError
		code int
	}{
		{apperrors.InvalidArgument("x", "bad"), 40001},
		{apperrors.Unauthorized("x", "auth"), 40101},
		{apperrors.NotFound("x", "missing"), 40401},
		{apperrors.Conflict("x", "conflict"), 40901},
		{apperrors.Internal("x", "boom"), 50001},
	}
	for _, tc := range cases {
		c, w := newPaxTestContext(t)
		PaxAppError(c, tc.err)
		var body PaxBody
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("响应非 JSON: %v", err)
		}
		if body.Code != tc.code {
			t.Fatalf("Kind=%s 映射 code=%d, want %d", tc.err.Kind, body.Code, tc.code)
		}
	}
}

func TestPaxAppErrorKeepsCustomCode(t *testing.T) {
	c, w := newPaxTestContext(t)
	PaxAppError(c, apperrors.New(apperrors.KindConflict, "41001", "评价超时"))

	var body PaxBody
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应非 JSON: %v", err)
	}
	if body.Code != 41001 {
		t.Fatalf("自定义业务码丢失: code=%d, want 41001", body.Code)
	}
}
