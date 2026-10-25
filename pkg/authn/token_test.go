package authn_test

import (
	"testing"

	"github.com/BwCloudWeGo/bw-cli/pkg/authn"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyToken(t *testing.T) {
	tests := []struct {
		name    string
		change  func(jwt.MapClaims)
		method  jwt.SigningMethod
		secret  string
		config  authn.Config
		wantErr bool
	}{
		{name: "valid driver"},
		{name: "legacy role", change: func(c jwt.MapClaims) { c["role"] = "user" }},
		{name: "expired", change: func(c jwt.MapClaims) { c["exp"] = 946684800 }, wantErr: true},
		{name: "missing expiry", change: func(c jwt.MapClaims) { delete(c, "exp") }, wantErr: true},
		{name: "wrong issuer", change: func(c jwt.MapClaims) { c["iss"] = "other" }, wantErr: true},
		{name: "missing issuer", change: func(c jwt.MapClaims) { delete(c, "iss") }, wantErr: true},
		{name: "blank user", change: func(c jwt.MapClaims) { c["user_id"] = " " }, wantErr: true},
		{name: "missing user", change: func(c jwt.MapClaims) { delete(c, "user_id") }, wantErr: true},
		{name: "wrong user type", change: func(c jwt.MapClaims) { c["user_id"] = 42 }, wantErr: true},
		{name: "legacy empty role", change: func(c jwt.MapClaims) { c["role"] = "" }},
		{name: "wrong role type", change: func(c jwt.MapClaims) { c["role"] = 42 }, wantErr: true},
		{name: "not yet valid", change: func(c jwt.MapClaims) { c["nbf"] = 4102444800 }, wantErr: true},
		{name: "wrong signature", secret: "other-secret", wantErr: true},
		{name: "HS384 rejected", method: jwt.SigningMethodHS384, wantErr: true},
		{name: "HS512 rejected", method: jwt.SigningMethodHS512, wantErr: true},
		{name: "missing secret", config: authn.Config{Issuer: "arrival"}, wantErr: true},
		{name: "blank secret", config: authn.Config{Secret: " ", Issuer: "arrival"}, wantErr: true},
		{name: "missing configured issuer", config: authn.Config{Secret: "test-secret"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := jwt.MapClaims{"user_id": "driver-1", "role": "driver", "iss": "arrival", "exp": 4102444800}
			if tt.change != nil {
				tt.change(claims)
			}
			method := tt.method
			if method == nil {
				method = jwt.SigningMethodHS256
			}
			secret := tt.secret
			if secret == "" {
				secret = "test-secret"
			}
			cfg := tt.config
			if cfg == (authn.Config{}) {
				cfg = authn.Config{Secret: "test-secret", Issuer: "arrival"}
			}
			token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
			if err != nil {
				t.Fatal(err)
			}
			got, err := authn.VerifyToken(token, cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyToken() error=%v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr {
				if got != (authn.Principal{}) {
					t.Fatalf("failed authentication returned principal: %+v", got)
				}
				return
			}
			if got.UserID != "driver-1" || got.Role != claims["role"] {
				t.Fatalf("unexpected principal: %+v", got)
			}
		})
	}
	for _, token := range []string{"", "garbage", "a.b.c"} {
		if _, err := authn.VerifyToken(token, authn.Config{Secret: "test-secret", Issuer: "arrival"}); err == nil {
			t.Errorf("accepted malformed token %q", token)
		}
	}
}
