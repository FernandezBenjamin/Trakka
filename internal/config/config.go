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
