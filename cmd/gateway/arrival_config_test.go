package main

import (
	"testing"

	"github.com/BwCloudWeGo/bw-cli/pkg/config"
	"github.com/BwCloudWeGo/bw-cli/pkg/middleware"
)

func TestArrivalGatewayConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name, enabled, read, secret string
		wantErr                     bool
	}{
		{"disabled", "", "", "", false},
		{"missing secret", "true", "", "", true},
		{"read requires secret", "", "true", "", true},
		{"enabled", "true", "", "isolated-test-secret", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ARRIVAL_ENABLED", tt.enabled)
			t.Setenv("ARRIVAL_READ_ENABLED", tt.read)
			t.Setenv("ARRIVAL_JWT_SECRET", tt.secret)
			t.Setenv("ARRIVAL_JWT_ISSUER", "arrival-test")
			cfg := &config.Config{Middleware: config.MiddlewareConfig{JWT: middleware.JWTConfig{Issuer: "legacy"}}}
			err := configureArrivalGateway(cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error %v", err, tt.wantErr)
			}
			if tt.secret != "" && (cfg.Middleware.JWT.Secret != tt.secret || cfg.Middleware.JWT.Issuer != "arrival-test") {
				t.Fatal("isolated JWT override not applied")
			}
		})
	}
}
