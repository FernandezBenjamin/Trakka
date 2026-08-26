package handlers

import (
	"testing"
	"time"

	"trakka/internal/models"
)

func strPtr(s string) *string { return &s }

func TestApplyRecurrenceCompletion(t *testing.T) {
	t.Run("non-recurring item is left untouched", func(t *testing.T) {
		item := &models.Item{Done: true}
		applyRecurrenceCompletion(item, false)
		if !item.Done {
			t.Fatal("expected a non-recurring item to stay done")
		}
		if item.DueDate != nil {
			t.Fatal("expected due date to remain nil")
		}
	})

	t.Run("already-done item is not re-advanced", func(t *testing.T) {
		due := "2026-01-01"
		item := &models.Item{Done: true, DueDate: &due, RecurrenceRule: strPtr("WEEKLY")}
		applyRecurrenceCompletion(item, true) // wasDone=true: no false->true transition
		if *item.DueDate != due {
			t.Fatalf("expected due date to stay %q, got %q", due, *item.DueDate)
		}
		if !item.Done {
			t.Fatal("expected item to remain done")
		}
	})

	t.Run("checking off a recurring weekly item advances and un-checks it", func(t *testing.T) {
		due := "2026-01-01"
		item := &models.Item{Done: true, DueDate: &due, RecurrenceRule: strPtr("WEEKLY")}
		applyRecurrenceCompletion(item, false)
		if item.Done {
			t.Fatal("expected the item to be un-checked after advancing")
		}
		if item.DueDate == nil || *item.DueDate != "2026-01-08" {
			t.Fatalf("expected due date 2026-01-08, got %v", item.DueDate)
		}
	})

	t.Run("with no prior due date, advances from today", func(t *testing.T) {
		item := &models.Item{Done: true, RecurrenceRule: strPtr("DAILY")}
		applyRecurrenceCompletion(item, false)
		wantTomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
		if item.DueDate == nil || *item.DueDate != wantTomorrow {
			t.Fatalf("expected due date %q, got %v", wantTomorrow, item.DueDate)
		}
		if item.Done {
			t.Fatal("expected the item to be un-checked")
		}
	})

	t.Run("custom every-x-days interval", func(t *testing.T) {
		due := "2026-01-01"
		item := &models.Item{Done: true, DueDate: &due, RecurrenceRule: strPtr("EVERY_X_DAYS:3")}
		applyRecurrenceCompletion(item, false)
		if item.DueDate == nil || *item.DueDate != "2026-01-04" {
			t.Fatalf("expected due date 2026-01-04, got %v", item.DueDate)
		}
	})

	t.Run("stops advancing once past recurrence_end_date", func(t *testing.T) {
		due := "2026-01-01"
		end := "2026-01-05"
		item := &models.Item{Done: true, DueDate: &due, RecurrenceRule: strPtr("WEEKLY"), RecurrenceEndDate: &end}
		applyRecurrenceCompletion(item, false)
		// 2026-01-01 + 7 days = 2026-01-08, which is past the 2026-01-05 end
		// date: the item should stay done, as a final completed occurrence.
		if !item.Done {
			t.Fatal("expected the item to stay done once recurrence has ended")
		}
		if *item.DueDate != due {
			t.Fatalf("expected due date to remain %q, got %q", due, *item.DueDate)
		}
	})

	t.Run("still advances when the next occurrence is exactly the end date", func(t *testing.T) {
		due := "2026-01-01"
		end := "2026-01-08"
		item := &models.Item{Done: true, DueDate: &due, RecurrenceRule: strPtr("WEEKLY"), RecurrenceEndDate: &end}
		applyRecurrenceCompletion(item, false)
		if item.Done {
			t.Fatal("expected the item to be un-checked for its final occurrence")
		}
		if *item.DueDate != end {
			t.Fatalf("expected due date %q, got %q", end, *item.DueDate)
		}
	})
}

func TestNextDueDate(t *testing.T) {
	cases := []struct {
		name    string
		current string
		rule    string
		want    string
	}{
		{"daily", "2026-01-31", "DAILY", "2026-02-01"},
		{"weekly", "2026-01-01", "WEEKLY", "2026-01-08"},
		{"monthly rolls over year", "2026-12-15", "MONTHLY", "2027-01-15"},
		{"yearly", "2026-02-28", "YEARLY", "2027-02-28"},
		{"every x days", "2026-01-01", "EVERY_X_DAYS:10", "2026-01-11"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := nextDueDate(tc.current, tc.rule)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("nextDueDate(%q, %q) = %q, want %q", tc.current, tc.rule, got, tc.want)
			}
		})
	}

	t.Run("rejects unrecognized rule", func(t *testing.T) {
		if _, err := nextDueDate("2026-01-01", "FORTNIGHTLY"); err == nil {
			t.Fatal("expected an error for an unrecognized rule")
		}
	})
}
