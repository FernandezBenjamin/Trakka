package settings

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"trakka/internal/config"
	"trakka/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "trakka.db"), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestResolveFallsBackToEnvDefaults(t *testing.T) {
	d := openTestDB(t)
	cfg := config.Config{
		InstanceName:     "Env Instance",
		RegistrationOpen: false,
		OIDCIssuer:       "https://issuer.example.com",
		OIDCClientID:     "env-client",
		OIDCClientSecret: "env-secret",
	}

	v, err := Resolve(context.Background(), d, cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if v.InstanceName != "Env Instance" {
		t.Errorf("InstanceName = %q, want env fallback", v.InstanceName)
	}
	if v.RegistrationOpen {
		t.Error("RegistrationOpen = true, want env fallback (false)")
	}
	if !v.OIDCEnabled {
		t.Error("OIDCEnabled = false, want true (all three env OIDC vars set)")
	}
	if v.OIDCClientSecret != "env-secret" {
		t.Errorf("OIDCClientSecret = %q, want env fallback", v.OIDCClientSecret)
	}
}

func TestResolvePrioritizesStoredSettingsOverEnv(t *testing.T) {
	d := openTestDB(t)
	cfg := config.Config{
		InstanceName:     "Env Instance",
		RegistrationOpen: true,
		OIDCIssuer:       "https://env-issuer.example.com",
		OIDCClientID:     "env-client",
		OIDCClientSecret: "env-secret",
	}

	err := d.SetSettings(context.Background(), map[string]string{
		KeyInstanceName:     "DB Instance",
		KeyRegistrationOpen: "false",
		KeyOIDCEnabled:      "false",
		KeyOIDCIssuer:       "https://db-issuer.example.com",
	})
	if err != nil {
		t.Fatalf("SetSettings: %v", err)
	}

	v, err := Resolve(context.Background(), d, cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if v.InstanceName != "DB Instance" {
		t.Errorf("InstanceName = %q, want DB override", v.InstanceName)
	}
	if v.RegistrationOpen {
		t.Error("RegistrationOpen = true, want DB override (false)")
	}
	if v.OIDCEnabled {
		t.Error("OIDCEnabled = true, want DB override (false) despite all env OIDC vars being set")
	}
	if v.OIDCIssuer != "https://db-issuer.example.com" {
		t.Errorf("OIDCIssuer = %q, want DB override", v.OIDCIssuer)
	}
	// oidc_client_id/secret have no DB row, so they must still fall back to
	// env individually — a DB override on one OIDC field must not blank out
	// the others.
	if v.OIDCClientID != "env-client" {
		t.Errorf("OIDCClientID = %q, want env fallback since no DB row was set for it", v.OIDCClientID)
	}
	if v.OIDCClientSecret != "env-secret" {
		t.Errorf("OIDCClientSecret = %q, want env fallback since no DB row was set for it", v.OIDCClientSecret)
	}
}
