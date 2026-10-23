package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWTAuthUsesStrictVerificationAndPreservesLegacyRoles(t *testing.T) {
	cfg := middleware.JWTConfig{Secret: "test-secret", Issuer: "arrival"}
	generated, err := middleware.GenerateToken(cfg, middleware.JWTClaims{UserID: "user-1", Role: "user"})
	if err != nil {
		t.Fatal(err)
	}
	roleless, err := middleware.GenerateToken(cfg, middleware.JWTClaims{UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		method    jwt.SigningMethod
		claims    jwt.MapClaims
		generated bool
		status    int
		roleless  bool
	}{
		{name: "generated legacy user", generated: true, status: 200},
		{name: "generated legacy roleless", generated: true, roleless: true, status: 200},
		{name: "HS384", method: jwt.SigningMethodHS384, claims: jwt.MapClaims{"user_id": "user-1", "role": "user", "iss": "arrival", "exp": 4102444800}, status: 401},
		{name: "missing expiry", method: jwt.SigningMethodHS256, claims: jwt.MapClaims{"user_id": "user-1", "role": "user", "iss": "arrival"}, status: 401},
		{name: "missing identity", method: jwt.SigningMethodHS256, claims: jwt.MapClaims{"role": "user", "iss": "arrival", "exp": 4102444800}, status: 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := generated
			wantRole := "user"
			if tt.roleless {
				token, wantRole = roleless, ""
			}
			if !tt.generated {
				var err error
				token, err = jwt.NewWithClaims(tt.method, tt.claims).SignedString([]byte(cfg.Secret))
				if err != nil {
					t.Fatal(err)
				}
			}
			router := gin.New()
			router.GET("/", middleware.JWTAuth(cfg), func(c *gin.Context) {
				claims := middleware.ClaimsFromContext(c)
				if claims.UserID != "user-1" || claims.Role != wantRole {
					t.Errorf("claims=%+v", claims)
				}
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tt.status {
				t.Fatalf("status=%d, want %d", response.Code, tt.status)
			}
		})
	}
}
