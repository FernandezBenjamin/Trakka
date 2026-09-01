package config

import (
	"testing"
	"time"
)

func TestParseDurationWithDays(t *testing.T) {
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"1d", 24 * time.Hour, false},
		{"3d", 72 * time.Hour, false},
		{"2h", 2 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"1d2h", 0, true}, // mixed day+hour suffix isn't supported, only a bare "Nd"
		{"d", 0, true},
		{"-1d", 0, true},
		{"", 0, true},
		{"not-a-duration", 0, true},
	}
	for _, tc := range cases {
		got, err := parseDurationWithDays(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseDurationWithDays(%q) = %v, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDurationWithDays(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDurationWithDays(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestConfigValidateVAPIDAllOrNothing(t *testing.T) {
	base := Config{}
	if err := base.Validate(); err != nil {
		t.Errorf("empty config should validate cleanly, got: %v", err)
	}

	partial := Config{VAPIDPublicKey: "pub", VAPIDPrivateKey: "priv"}
	if err := partial.Validate(); err == nil {
		t.Error("expected an error when only some VAPID_* vars are set")
	}

	full := Config{VAPIDPublicKey: "pub", VAPIDPrivateKey: "priv", VAPIDSubject: "mailto:ops@example.com"}
	if err := full.Validate(); err != nil {
		t.Errorf("fully-configured VAPID should validate cleanly, got: %v", err)
	}
	if !full.PushEnabled() {
		t.Error("PushEnabled() = false for a fully-configured VAPID setup")
	}
	if base.PushEnabled() {
		t.Error("PushEnabled() = true for an empty config")
	}
}
