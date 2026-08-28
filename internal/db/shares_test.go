package db

import (
	"context"
	"testing"
)

// TestSpaceShareCRUD exercises granting/upserting/listing/revoking a Space
// share, independent of any list or house access check.
func TestSpaceShareCRUD(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	other := mustCreateUserWithEmail(t, ctx, d, "other@example.com")
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "🏖️", "#3366ff", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}

	share, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, other, "read")
	if err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}
	if share.Permission != "read" {
		t.Fatalf("expected read permission, got %q", share.Permission)
	}

	// Re-sharing with a different permission upserts rather than erroring or
	// creating a second row.
	upserted, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, other, "write")
	if err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare (upsert): %v", err)
	}
	if upserted.Permission != "write" {
		t.Fatalf("expected write permission after upsert, got %q", upserted.Permission)
	}

	shares, err := d.ListSpaceShares(ctx, category.ID)
	if err != nil {
		t.Fatalf("ListSpaceShares: %v", err)
	}
	if len(shares) != 1 || shares[0].SharedWithUserID != other || shares[0].Permission != "write" {
		t.Fatalf("expected exactly one write share for %d, got %+v", other, shares)
	}

	if err := d.RevokeSpaceShare(ctx, category.ID, other); err != nil {
		t.Fatalf("RevokeSpaceShare: %v", err)
	}
	if err := d.RevokeSpaceShare(ctx, category.ID, other); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound revoking an already-revoked share, got %v", err)
	}
}

// TestListShareCRUD is the List equivalent of TestSpaceShareCRUD.
func TestListShareCRUD(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	other := mustCreateUserWithEmail(t, ctx, d, "other@example.com")
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	share, err := d.CreateOrUpdateListShare(ctx, list.ID, other, "read")
	if err != nil {
		t.Fatalf("CreateOrUpdateListShare: %v", err)
	}
	if share.Permission != "read" {
		t.Fatalf("expected read permission, got %q", share.Permission)
	}

	shares, err := d.ListListShares(ctx, list.ID)
	if err != nil {
		t.Fatalf("ListListShares: %v", err)
	}
	if len(shares) != 1 || shares[0].SharedWithUserID != other {
		t.Fatalf("expected exactly one share for %d, got %+v", other, shares)
	}

	if err := d.RevokeListShare(ctx, list.ID, other); err != nil {
		t.Fatalf("RevokeListShare: %v", err)
	}
	if _, err := d.GetListShare(ctx, list.ID, other); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after revoke, got %v", err)
	}
}

// TestAccessLevelForList exercises every source db.AccessLevelForList
// combines: House membership always wins as "write", a direct List share
// grants exactly its own permission, a Space share (via the list's
// custom_category_id) grants its own permission the same way, the higher of
// the two applies when both are present, and a user with none of the three
// gets "".
func TestAccessLevelForList(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	member := mustCreateUserWithEmail(t, ctx, d, "member@example.com")
	listShared := mustCreateUserWithEmail(t, ctx, d, "list-shared@example.com")
	spaceShared := mustCreateUserWithEmail(t, ctx, d, "space-shared@example.com")
	both := mustCreateUserWithEmail(t, ctx, d, "both@example.com")
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")

	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := d.AddHouseMember(ctx, house.ID, member, "member"); err != nil {
		t.Fatalf("AddHouseMember: %v", err)
	}

	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	if _, err := d.CreateOrUpdateListShare(ctx, list.ID, listShared, "read"); err != nil {
		t.Fatalf("CreateOrUpdateListShare: %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, spaceShared, "write"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}
	if _, err := d.CreateOrUpdateListShare(ctx, list.ID, both, "read"); err != nil {
		t.Fatalf("CreateOrUpdateListShare (both): %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, both, "write"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare (both): %v", err)
	}

	cases := []struct {
		name string
		user int64
		want string
	}{
		{"house owner", owner, "write"},
		{"house member", member, "write"},
		{"direct list share (read)", listShared, "read"},
		{"space share (write)", spaceShared, "write"},
		{"both list-read and space-write takes the higher", both, "write"},
		{"stranger has no access", stranger, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := d.AccessLevelForList(ctx, tc.user, list)
			if err != nil {
				t.Fatalf("AccessLevelForList: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

// TestListSharedListsForUser exercises the "Partagé avec moi" query: it
// must include a directly-shared list and a list reachable only via its
// Space, tag each with the right AccessSource/AccessPermission, and exclude
// a list whose house the user is already a plain member of (that one
// already shows up through the ordinary House-scoped ListListsForUser, so
// repeating it here would just be noise).
func TestListSharedListsForUser(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")

	// Two separate houses: the recipient is a plain member of houseOwn (so
	// its own list must be excluded from "shared with me" — it's already
	// visible the ordinary way) but not a member of houseOther at all,
	// which is where the actually-shared lists live.
	houseOther, err := d.CreateHouseWithOwner(ctx, "Maison d'un tiers", owner)
	if err != nil {
		t.Fatalf("creating houseOther: %v", err)
	}
	houseOwn, err := d.CreateHouseWithOwner(ctx, "Maison du destinataire", recipient)
	if err != nil {
		t.Fatalf("creating houseOwn: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}

	directList, err := d.CreateList(ctx, "Direct", "shopping", houseOther.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateList (direct): %v", err)
	}
	spaceList, err := d.CreateList(ctx, "Via space", "shopping", houseOther.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList (via space): %v", err)
	}
	memberHouseList, err := d.CreateList(ctx, "Own house", "shopping", houseOwn.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateList (own house): %v", err)
	}

	if _, err := d.CreateOrUpdateListShare(ctx, directList.ID, recipient, "read"); err != nil {
		t.Fatalf("CreateOrUpdateListShare: %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, recipient, "write"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}

	shared, err := d.ListSharedListsForUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSharedListsForUser: %v", err)
	}

	byID := map[int64]*struct {
		source     string
		permission string
	}{}
	for _, l := range shared {
		byID[l.ID] = &struct {
			source     string
			permission string
		}{l.AccessSource, l.AccessPermission}
	}

	if _, ok := byID[memberHouseList.ID]; ok {
		t.Fatalf("expected the recipient's own house list to be excluded, got %+v", shared)
	}
	direct, ok := byID[directList.ID]
	if !ok || direct.source != "list_share" || direct.permission != "read" {
		t.Fatalf("expected directList tagged list_share/read, got %+v", byID[directList.ID])
	}
	viaSpace, ok := byID[spaceList.ID]
	if !ok || viaSpace.source != "space_share" || viaSpace.permission != "write" {
		t.Fatalf("expected spaceList tagged space_share/write, got %+v", byID[spaceList.ID])
	}
	if len(shared) != 2 {
		t.Fatalf("expected exactly 2 shared lists, got %d: %+v", len(shared), shared)
	}
}
