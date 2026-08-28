package validate

import "testing"

func TestRecurrence(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"empty is valid (not recurring)", "", "", false},
		{"whitespace-only is valid", "   ", "", false},
		{"daily", "DAILY", "DAILY", false},
		{"weekly", "WEEKLY", "WEEKLY", false},
		{"monthly", "MONTHLY", "MONTHLY", false},
		{"yearly", "YEARLY", "YEARLY", false},
		{"normalizes case", "weekly", "WEEKLY", false},
		{"trims whitespace", "  DAILY  ", "DAILY", false},
		{"every-x-days", "EVERY_X_DAYS:3", "EVERY_X_DAYS:3", false},
		{"rejects every-x-days with zero", "EVERY_X_DAYS:0", "", true},
		{"rejects every-x-days without a number", "EVERY_X_DAYS:", "", true},
		{"rejects unknown rule", "FORTNIGHTLY", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Recurrence(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got none", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("Recurrence(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestEveryXDaysInterval(t *testing.T) {
	if n, ok := EveryXDaysInterval("EVERY_X_DAYS:5"); !ok || n != 5 {
		t.Fatalf("EveryXDaysInterval(EVERY_X_DAYS:5) = (%d, %v), want (5, true)", n, ok)
	}
	if _, ok := EveryXDaysInterval("DAILY"); ok {
		t.Fatal("expected ok=false for a fixed cadence")
	}
	if _, ok := EveryXDaysInterval(""); ok {
		t.Fatal("expected ok=false for an empty rule")
	}
}
