// Package config loads Trakka's runtime configuration from environment
// variables. There is no config file; every setting is an env var so the
// container image stays fully generic across deployments.
package config

import (
	"errors"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port      string
	DBPath    string
	StaticDir string

	TemplatesDir string // dir containing login.html, mirrors StaticDir's pattern

	// BaseURL is the externally-visible origin (e.g. "https://trakka.example.com"),
	// used to construct the OIDC redirect_uri deterministically. Required
	// only when OIDC is configured.
	BaseURL string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string

	SessionCookieSecure bool
	SessionTTL          time.Duration

	// PriceCheckInterval is how often the background price-drop scan
	// (internal/handlers.RunPriceAlertScan) re-checks every eligible item.
	// A value <= 0 disables the periodic scan entirely (on-demand checks via
	// POST /api/v1/items/{id}/price-check still work regardless).
	PriceCheckInterval time.Duration

	// InstanceName and RegistrationOpen are the env-var defaults for two of
	// the settings manageable at runtime via the admin-only
	// PATCH /api/v1/admin/settings endpoint (see internal/settings.Resolve).
	// A row in the system_settings table always takes priority over these
	// once one exists; these are only what a fresh instance starts with.
	InstanceName     string
	RegistrationOpen bool
}

func Load() Config {
	return Config{
		Port:      envOr("PORT", "8080"),
		DBPath:    envOr("DB_PATH", "/data/trakka.db"),
		StaticDir: envOr("STATIC_DIR", "/app/static"),

		TemplatesDir: envOr("TEMPLATES_DIR", "/app/templates"),
		BaseURL:      envOr("BASE_URL", ""),

		OIDCIssuer:       envOr("OIDC_ISSUER", ""),
		OIDCClientID:     envOr("OIDC_CLIENT_ID", ""),
		OIDCClientSecret: envOr("OIDC_CLIENT_SECRET", ""),

		SessionCookieSecure: envBool("SESSION_COOKIE_SECURE", true),
		SessionTTL:          time.Duration(envInt("SESSION_TTL_HOURS", 720)) * time.Hour,

		PriceCheckInterval: time.Duration(envInt("PRICE_CHECK_INTERVAL_HOURS", 24)) * time.Hour,

		InstanceName:     envOr("INSTANCE_NAME", "Trakka"),
		RegistrationOpen: envBool("REGISTRATION_OPEN", true),
	}
}

// OIDCEnabled reports whether all three OIDC env vars are configured.
func (c Config) OIDCEnabled() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

// Validate checks cross-field constraints Load() alone can't enforce.
func (c Config) Validate() error {
	set := 0
	for _, v := range []string{c.OIDCIssuer, c.OIDCClientID, c.OIDCClientSecret} {
		if v != "" {
			set++
		}
	}
	if set != 0 && set != 3 {
		return errors.New("OIDC_ISSUER, OIDC_CLIENT_ID and OIDC_CLIENT_SECRET must all be set together, or none of them")
	}
	if c.OIDCEnabled() && c.BaseURL == "" {
		return errors.New("BASE_URL is required when OIDC is configured")
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func envInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
