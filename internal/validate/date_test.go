package validate

import "testing"

func TestDate(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{"empty is valid", "", "", false},
		{"whitespace-only is valid", "   ", "", false},
		{"valid date", "2026-08-26", "2026-08-26", false},
		{"trims surrounding whitespace", "  2026-08-26  ", "2026-08-26", false},
		{"rejects bad format", "26-08-2026", "", true},
		{"rejects non-calendar date", "2026-02-30", "", true},
		{"rejects garbage", "not-a-date", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Date(tc.raw)
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
				t.Fatalf("Date(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}
