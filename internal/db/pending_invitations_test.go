package db

import (
	"context"
	"testing"
)

// TestPendingInvitationMaterializesForNewAccount covers the case the old
// invite path could not serve at all: inviting an address that has no account
// here yet. Nothing is granted at invite time; the membership appears only
// once that address actually signs in.
func TestPendingInvitationMaterializesForNewAccount(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	hash := "x"
	owner, err := d.CreateUser(ctx, "owner@example.com", &hash, nil, nil, "Owner")
	if err != nil {
		t.Fatalf("creating owner: %v", err)
	}
	house, err := d.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}

	if _, err := d.CreatePendingInvitation(ctx, InvitationKindHouse, house.ID, "newcomer@example.com", "", owner.ID); err != nil {
		t.Fatalf("creating invitation: %v", err)
	}

	// The invitation must not grant anything before the invitee exists.
	invitations, err := d.ListPendingInvitations(ctx, InvitationKindHouse, house.ID)
	if err != nil {
		t.Fatalf("listing invitations: %v", err)
	}
	if len(invitations) != 1 || invitations[0].Email != "newcomer@example.com" {
		t.Fatalf("expected exactly one pending invitation, got %+v", invitations)
	}

	newcomer, err := d.CreateUser(ctx, "newcomer@example.com", &hash, nil, nil, "Newcomer")
	if err != nil {
		t.Fatalf("creating newcomer: %v", err)
	}
	applied, err := d.MaterializePendingInvitations(ctx, newcomer.ID, newcomer.Email)
	if err != nil {
		t.Fatalf("materializing: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 invitation applied, got %d", applied)
	}

	canAccess, err := d.UserCanAccessHouse(ctx, newcomer.ID, house.ID)
	if err != nil {
		t.Fatalf("UserCanAccessHouse: %v", err)
	}
	if !canAccess {
		t.Fatal("the invited user should be a member of the house after signing in")
	}

	// The invitation is consumed, so a second sign-in is a no-op.
	remaining, err := d.ListPendingInvitations(ctx, InvitationKindHouse, house.ID)
	if err != nil {
		t.Fatalf("listing invitations after materialization: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected the invitation to be consumed, got %+v", remaining)
	}
	if applied, err := d.MaterializePendingInvitations(ctx, newcomer.ID, newcomer.Email); err != nil || applied != 0 {
		t.Fatalf("re-materializing should be a no-op, got applied=%d err=%v", applied, err)
	}
}

// TestPendingInvitationIsCaseInsensitiveAndUpsertable guards two properties
// the enumeration fix depends on: an invitation must find its recipient
// regardless of the case the address was typed in (the users table is
// COLLATE NOCASE, so the invitation must be too), and re-inviting the same
// address must update the permission rather than erroring — otherwise a
// second invite would fail in a way that reveals the first one exists.
func TestPendingInvitationIsCaseInsensitiveAndUpsertable(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	hash := "x"
	owner, err := d.CreateUser(ctx, "owner@example.com", &hash, nil, nil, "Owner")
	if err != nil {
		t.Fatalf("creating owner: %v", err)
	}
	house, err := d.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("creating list: %v", err)
	}

	if _, err := d.CreatePendingInvitation(ctx, InvitationKindList, list.ID, "Mixed.Case@Example.COM", "read", owner.ID); err != nil {
		t.Fatalf("creating invitation: %v", err)
	}
	// Re-inviting the same address upgrades the permission in place.
	if _, err := d.CreatePendingInvitation(ctx, InvitationKindList, list.ID, "mixed.case@example.com", "write", owner.ID); err != nil {
		t.Fatalf("re-inviting: %v", err)
	}
	invitations, err := d.ListPendingInvitations(ctx, InvitationKindList, list.ID)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(invitations) != 1 || invitations[0].Permission != "write" {
		t.Fatalf("expected one invitation upgraded to write, got %+v", invitations)
	}

	recipient, err := d.CreateUser(ctx, "MIXED.CASE@example.com", &hash, nil, nil, "Recipient")
	if err != nil {
		t.Fatalf("creating recipient: %v", err)
	}
	if _, err := d.MaterializePendingInvitations(ctx, recipient.ID, recipient.Email); err != nil {
		t.Fatalf("materializing: %v", err)
	}

	level, err := d.AccessLevelForList(ctx, recipient.ID, list)
	if err != nil {
		t.Fatalf("AccessLevelForList: %v", err)
	}
	if level != "write" {
		t.Fatalf("expected write access after materialization, got %q", level)
	}
}

// TestPendingInvitationForDeletedTargetIsDropped confirms an invitation
// cannot resurrect access to something that no longer exists — the reason
// applyInvitation re-checks the target rather than trusting the stored id
// (which cannot carry a foreign key, being polymorphic).
func TestPendingInvitationForDeletedTargetIsDropped(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	hash := "x"
	owner, err := d.CreateUser(ctx, "owner@example.com", &hash, nil, nil, "Owner")
	if err != nil {
		t.Fatalf("creating owner: %v", err)
	}
	house, err := d.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}

	if _, err := d.CreatePendingInvitation(ctx, InvitationKindHouse, house.ID, "later@example.com", "", owner.ID); err != nil {
		t.Fatalf("creating invitation: %v", err)
	}
	if err := d.DeleteHouse(ctx, house.ID); err != nil {
		t.Fatalf("deleting house: %v", err)
	}

	invitee, err := d.CreateUser(ctx, "later@example.com", &hash, nil, nil, "Later")
	if err != nil {
		t.Fatalf("creating invitee: %v", err)
	}
	applied, err := d.MaterializePendingInvitations(ctx, invitee.ID, invitee.Email)
	if err != nil {
		t.Fatalf("materializing: %v", err)
	}
	if applied != 0 {
		t.Fatalf("an invitation to a deleted house must grant nothing, got applied=%d", applied)
	}

	houses, err := d.ListHousesForUser(ctx, invitee.ID)
	if err != nil {
		t.Fatalf("listing houses: %v", err)
	}
	if len(houses) != 0 {
		t.Fatalf("expected no houses for the invitee, got %+v", houses)
	}
}

// TestDeletePendingInvitation covers withdrawing an invitation, which the
// mistyped-address case needs now that a bad address no longer fails loudly.
func TestDeletePendingInvitation(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	hash := "x"
	owner, err := d.CreateUser(ctx, "owner@example.com", &hash, nil, nil, "Owner")
	if err != nil {
		t.Fatalf("creating owner: %v", err)
	}
	house, err := d.CreateHouseWithOwner(ctx, "Maison", owner.ID)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := d.CreatePendingInvitation(ctx, InvitationKindHouse, house.ID, "typo@example.com", "", owner.ID); err != nil {
		t.Fatalf("creating invitation: %v", err)
	}

	if err := d.DeletePendingInvitation(ctx, InvitationKindHouse, house.ID, "TYPO@example.com"); err != nil {
		t.Fatalf("deleting invitation (case-insensitively): %v", err)
	}
	if err := d.DeletePendingInvitation(ctx, InvitationKindHouse, house.ID, "typo@example.com"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting twice, got %v", err)
	}
}
