package main

import (
	"errors"
	"os"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/pkg/config"
)

// configureArrivalGateway permits isolated deployment without embedding secrets in YAML.
func configureArrivalGateway(cfg *config.Config) error {
	if secret, exists := os.LookupEnv("ARRIVAL_JWT_SECRET"); exists {
		cfg.Middleware.JWT.Secret = secret
	}
	if issuer, exists := os.LookupEnv("ARRIVAL_JWT_ISSUER"); exists {
		cfg.Middleware.JWT.Issuer = issuer
	}
	if strings.EqualFold(os.Getenv("ARRIVAL_ENABLED"), "true") || strings.EqualFold(os.Getenv("ARRIVAL_READ_ENABLED"), "true") {
		if strings.TrimSpace(cfg.Middleware.JWT.Secret) == "" || strings.TrimSpace(cfg.Middleware.JWT.Issuer) == "" {
			return errors.New("arrival requires JWT secret and issuer")
		}
	}
	return nil
}
