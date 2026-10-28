package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// runRoleChain 用真实路由链执行：桩中间件写入 claims → RequireRole → 终态 handler 置位 flag。
func runRoleChain(t *testing.T, claimsRole, requireRole string, withClaims bool) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/x",
		func(c *gin.Context) {
			if withClaims {
				c.Set(claimsContextKey, JWTClaims{UserID: "u1", Role: claimsRole})
			}
			c.Next()
		},
		RequireRole(requireRole),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ran": true}) },
	)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	var body struct {
		Ran bool `json:"ran"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w, body.Ran
}

func TestRequireRoleRejectsMismatch(t *testing.T) {
	w, ran := runRoleChain(t, "drv", "pax", true)

	if ran {
		t.Fatal("角色不符时后续 handler 不应执行")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("HTTP 状态 = %d, want 403", w.Code)
	}

	var envelope struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("响应非 JSON: %v", err)
	}
	if envelope.Code != 40101 {
		t.Fatalf("业务码 = %d, want 40101", envelope.Code)
	}
	if envelope.Data != nil {
		t.Fatalf("data 应为 null, got %v", envelope.Data)
	}
}

func TestRequireRoleAllowsMatch(t *testing.T) {
	w, ran := runRoleChain(t, "pax", "pax", true)

	if !ran {
		t.Fatal("角色匹配时后续 handler 应执行")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("HTTP 状态 = %d, want 200", w.Code)
	}
}

func TestRequireRoleMissingClaimsReturns401(t *testing.T) {
	w, ran := runRoleChain(t, "", "pax", false)

	if ran {
		t.Fatal("claims 缺失时后续 handler 不应执行")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("HTTP 状态 = %d, want 401", w.Code)
	}
}
