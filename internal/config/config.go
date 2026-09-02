// Package config loads Trakka's runtime configuration from environment
// variables. There is no config file; every setting is an env var so the
// container image stays fully generic across deployments.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"trakka/internal/validate"
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

	// TargetPriceScrapeInterval is how often the target-price background
	// worker (internal/handlers.RunTargetPriceScan) re-scrapes every item
	// with an active "notify me when the price drops" threshold (see
	// models.Item.TargetPrice/AlertOnPriceDrop) and applies whatever current
	// price it finds. This is distinct from PriceCheckInterval above: that
	// scan compares a scraped price against the item's own current price and
	// only ever proposes a price_alerts row for the user to accept/reject,
	// while this one compares against the user's own explicit threshold and,
	// once crossed, writes items.price directly and notifies immediately
	// with no accept/reject step. A value <= 0 disables the periodic scan
	// entirely.
	TargetPriceScrapeInterval time.Duration

	// InstanceName and RegistrationOpen are the env-var defaults for two of
	// the settings manageable at runtime via the admin-only
	// PATCH /api/v1/admin/settings endpoint (see internal/settings.Resolve).
	// A row in the system_settings table always takes priority over these
	// once one exists; these are only what a fresh instance starts with.
	InstanceName     string
	RegistrationOpen bool

	// VAPIDPublicKey/VAPIDPrivateKey are this instance's Web Push
	// application-server identity (see internal/webpush) — a P-256 key pair,
	// base64url-encoded exactly as internal/webpush.GenerateVAPIDKeys (and
	// `trakka -generate-vapid-keys`) produce them. VAPIDSubject is the
	// contact URI (mailto: or https:) sent in every VAPID JWT's "sub" claim,
	// as RFC 8292 requires. All three are all-or-nothing, like OIDC below —
	// see Validate.
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string

	// NotifRecurringLeadTime is how long before a recurring item's due date
	// the background scan (internal/handlers.RunRecurringDueScan) sends a
	// reminder push, unless a specific item overrides it via its own
	// recurrence_lead_minutes. Accepts a plain Go duration ("2h", "30m") or a
	// whole number of days with a "d" suffix ("1d", "3d") — see
	// parseDurationWithDays, since time.ParseDuration alone has no day unit
	// and this setting's own examples include one.
	NotifRecurringLeadTime time.Duration

	// NotifRecurringScanInterval is how often that same scan re-checks every
	// eligible item — independent of, and normally much finer-grained than,
	// NotifRecurringLeadTime itself, so a lead time of e.g. 2h is actually
	// caught reasonably close to on time rather than only once a day. A
	// value <= 0 disables the periodic scan entirely.
	NotifRecurringScanInterval time.Duration

	// DefaultAppLanguage is the UI language (see static/locales/{fr,en}.json)
	// shown to any account that has never set its own preference — a brand
	// new registration (users.language starts out empty) and any
	// pre-existing account that never touched the Language section of the
	// "Paramètres" modal both resolve to this at read time (see
	// internal/handlers.resolveUserLanguage and models.User.Language),
	// rather than the instance's admin having to change it per account.
	// Defaults to "en"; an unrecognized value falls back to "en" too, the
	// same "invalid env var falls back to the fallback" convention as
	// envBool/envInt below.
	DefaultAppLanguage string
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

		TargetPriceScrapeInterval: envDuration("SCRAPE_INTERVAL", 12*time.Hour),

		InstanceName:     envOr("INSTANCE_NAME", "Trakka"),
		RegistrationOpen: envBool("REGISTRATION_OPEN", true),

		VAPIDPublicKey:  envOr("VAPID_PUBLIC_KEY", ""),
		VAPIDPrivateKey: envOr("VAPID_PRIVATE_KEY", ""),
		VAPIDSubject:    envOr("VAPID_SUBJECT", ""),

		NotifRecurringLeadTime:     envDuration("NOTIF_RECURRING_TASK_LEAD_TIME", 24*time.Hour),
		NotifRecurringScanInterval: time.Duration(envInt("NOTIF_RECURRING_SCAN_INTERVAL_MINUTES", 30)) * time.Minute,

		DefaultAppLanguage: envLanguage("DEFAULT_APP_LANGUAGE", "en"),
	}
}

// OIDCEnabled reports whether all three OIDC env vars are configured.
func (c Config) OIDCEnabled() bool {
	return c.OIDCIssuer != "" && c.OIDCClientID != "" && c.OIDCClientSecret != ""
}

// PushEnabled reports whether all three VAPID env vars are configured —
// checked before wiring up the push subscribe/vapid-public-key routes and
// the recurring-due-date scan (see cmd/server/main.go), mirroring how
// OIDCEnabled gates the OIDC client.
func (c Config) PushEnabled() bool {
	return c.VAPIDPublicKey != "" && c.VAPIDPrivateKey != "" && c.VAPIDSubject != ""
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

	vapidSet := 0
	for _, v := range []string{c.VAPIDPublicKey, c.VAPIDPrivateKey, c.VAPIDSubject} {
		if v != "" {
			vapidSet++
		}
	}
	if vapidSet != 0 && vapidSet != 3 {
		return errors.New("VAPID_PUBLIC_KEY, VAPID_PRIVATE_KEY and VAPID_SUBJECT must all be set together, or none of them")
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

// envLanguage reads key and validates it against validate.SupportedLanguages
// (case-insensitively, trimmed), falling back — same as every other envXxx
// helper here — when the variable is unset or isn't one of them.
func envLanguage(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		if lang, ok := validate.Language(v); ok {
			return lang
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := parseDurationWithDays(v)
	if err != nil {
		return fallback
	}
	return d
}

// parseDurationWithDays extends time.ParseDuration with a whole-number "Nd"
// day suffix (e.g. "1d", "3d") — Go's own parser has no day unit, and
// NOTIF_RECURRING_TASK_LEAD_TIME is meant to be set in days as often as in
// hours (see its own doc comment above), so a bare "1d" needs to work
// without operators having to spell out "24h" themselves.
func parseDurationWithDays(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid day duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
