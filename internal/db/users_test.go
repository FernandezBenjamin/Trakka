package db

import (
	"context"
	"testing"
)

// TestFirstUserBecomesAdmin exercises CreateUser's "very first account on
// this instance is an admin" bootstrap (see the comment on CreateUser) — the
// only way Trakka ever gets an initial administrator, since there is no
// separate seeding mechanism.
func TestFirstUserBecomesAdmin(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	hash := "x"
	first, err := d.CreateUser(ctx, "first@example.com", &hash, nil, nil, "First")
	if err != nil {
		t.Fatalf("creating first user: %v", err)
	}
	if !first.IsAdmin {
		t.Fatal("expected the first user created to be an admin")
	}

	second, err := d.CreateUser(ctx, "second@example.com", &hash, nil, nil, "Second")
	if err != nil {
		t.Fatalf("creating second user: %v", err)
	}
	if second.IsAdmin {
		t.Fatal("expected the second user created to not be an admin")
	}

	// GetUser and GetUserByEmail must agree on the persisted admin flag.
	reloaded, err := d.GetUser(ctx, first.ID)
	if err != nil {
		t.Fatalf("reloading first user: %v", err)
	}
	if !reloaded.IsAdmin {
		t.Fatal("expected GetUser to report the first user as admin")
	}

	byEmail, err := d.GetUserByEmail(ctx, "second@example.com")
	if err != nil {
		t.Fatalf("reloading second user by email: %v", err)
	}
	if byEmail.IsAdmin {
		t.Fatal("expected GetUserByEmail to report the second user as non-admin")
	}
}
