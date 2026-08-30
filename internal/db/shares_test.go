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

// TestSetListSharePinned exercises the "pin a shared list to my dashboard"
// action: it defaults to unpinned, can be toggled on and back off by the
// recipient, and returns ErrNotFound for a user with no list_shares row on
// that list at all (never created, or already revoked).
func TestSetListSharePinned(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, nil, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	share, err := d.CreateOrUpdateListShare(ctx, list.ID, recipient, "read")
	if err != nil {
		t.Fatalf("CreateOrUpdateListShare: %v", err)
	}
	if share.IsPinnedToDashboard {
		t.Fatalf("expected a freshly created share to default to unpinned")
	}

	pinned, err := d.SetListSharePinned(ctx, list.ID, recipient, true)
	if err != nil {
		t.Fatalf("SetListSharePinned(true): %v", err)
	}
	if !pinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard true after pinning")
	}
	// GetListShare must agree with what SetListSharePinned returned.
	got, err := d.GetListShare(ctx, list.ID, recipient)
	if err != nil {
		t.Fatalf("GetListShare: %v", err)
	}
	if !got.IsPinnedToDashboard {
		t.Fatalf("expected GetListShare to reflect the pinned state")
	}

	unpinned, err := d.SetListSharePinned(ctx, list.ID, recipient, false)
	if err != nil {
		t.Fatalf("SetListSharePinned(false): %v", err)
	}
	if unpinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard false after unpinning")
	}

	if _, err := d.SetListSharePinned(ctx, list.ID, stranger, true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound pinning a share that doesn't exist, got %v", err)
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
	if _, err := d.SetListSharePinned(ctx, directList.ID, recipient, true); err != nil {
		t.Fatalf("SetListSharePinned: %v", err)
	}

	shared, err := d.ListSharedListsForUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSharedListsForUser: %v", err)
	}

	byID := map[int64]*struct {
		source     string
		permission string
		pinned     bool
	}{}
	for _, l := range shared {
		byID[l.ID] = &struct {
			source     string
			permission string
			pinned     bool
		}{l.AccessSource, l.AccessPermission, l.IsPinnedToDashboard}
	}

	if _, ok := byID[memberHouseList.ID]; ok {
		t.Fatalf("expected the recipient's own house list to be excluded, got %+v", shared)
	}
	direct, ok := byID[directList.ID]
	if !ok || direct.source != "list_share" || direct.permission != "read" {
		t.Fatalf("expected directList tagged list_share/read, got %+v", byID[directList.ID])
	}
	if !direct.pinned {
		t.Fatalf("expected directList to come back pinned, got %+v", byID[directList.ID])
	}
	viaSpace, ok := byID[spaceList.ID]
	if !ok || viaSpace.source != "space_share" || viaSpace.permission != "write" {
		t.Fatalf("expected spaceList tagged space_share/write, got %+v", byID[spaceList.ID])
	}
	if viaSpace.pinned {
		t.Fatalf("expected spaceList (reached only via a Space, no list_shares row) to never come back pinned, got %+v", byID[spaceList.ID])
	}
	if len(shared) != 2 {
		t.Fatalf("expected exactly 2 shared lists, got %d: %+v", len(shared), shared)
	}
}

// TestSetSpaceSharePinned is the Space-level equivalent of
// TestSetListSharePinned: defaults to unpinned, can be toggled on and back
// off by the recipient, and returns ErrNotFound for a user with no
// space_shares row on that category at all.
func TestSetSpaceSharePinned(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")

	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "🏖️", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	share, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, recipient, "read")
	if err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}
	if share.IsPinnedToDashboard {
		t.Fatalf("expected a freshly created space share to default to unpinned")
	}

	pinned, err := d.SetSpaceSharePinned(ctx, category.ID, recipient, true)
	if err != nil {
		t.Fatalf("SetSpaceSharePinned(true): %v", err)
	}
	if !pinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard true after pinning")
	}
	got, err := d.GetSpaceShare(ctx, category.ID, recipient)
	if err != nil {
		t.Fatalf("GetSpaceShare: %v", err)
	}
	if !got.IsPinnedToDashboard {
		t.Fatalf("expected GetSpaceShare to reflect the pinned state")
	}

	unpinned, err := d.SetSpaceSharePinned(ctx, category.ID, recipient, false)
	if err != nil {
		t.Fatalf("SetSpaceSharePinned(false): %v", err)
	}
	if unpinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard false after unpinning")
	}

	if _, err := d.SetSpaceSharePinned(ctx, category.ID, stranger, true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound pinning a space share that doesn't exist, got %v", err)
	}
}

// TestSetListSharePinnedViaSpaceAccess covers the bug this whole change was
// written to fix: a user reaching a list *only* through a shared Space (no
// list_shares row of their own) must still be able to pin it individually.
// SetListSharePinned must auto-create the list_shares row that carries the
// flag, scoped to exactly the permission the Space already grants, so the
// recipient's actual combined access level (db.AccessLevelForList) is
// unaffected by the row's existence.
func TestSetListSharePinnedViaSpaceAccess(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")
	house, err := d.CreateHouseWithOwner(ctx, "Maison Test", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses", "shopping", house.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, recipient, "write"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}

	// Sanity check: the recipient has no direct list_shares row at all —
	// only space-based access — otherwise this test would prove nothing
	// about the fallback path it's meant to exercise.
	if _, err := d.GetListShare(ctx, list.ID, recipient); err != ErrNotFound {
		t.Fatalf("test setup bug: expected no pre-existing list_shares row, got %v", err)
	}

	pinned, err := d.SetListSharePinned(ctx, list.ID, recipient, true)
	if err != nil {
		t.Fatalf("SetListSharePinned via space access: %v", err)
	}
	if !pinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard true after pinning")
	}
	if pinned.Permission != "write" {
		t.Fatalf("expected the auto-created list_shares row to mirror the Space's own permission (write), got %q", pinned.Permission)
	}

	level, err := d.AccessLevelForList(ctx, recipient, list)
	if err != nil {
		t.Fatalf("AccessLevelForList: %v", err)
	}
	if level != "write" {
		t.Fatalf("expected access level to remain write after the auto-created pin row, got %q", level)
	}

	// Once the row exists, a further toggle takes the ordinary update path
	// (no second insert, no error).
	unpinned, err := d.SetListSharePinned(ctx, list.ID, recipient, false)
	if err != nil {
		t.Fatalf("SetListSharePinned(false): %v", err)
	}
	if unpinned.IsPinnedToDashboard {
		t.Fatalf("expected IsPinnedToDashboard false after unpinning")
	}

	// A user with neither a list_shares row nor any space-based access at
	// all still gets ErrNotFound, exactly as before this fallback existed.
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")
	if _, err := d.SetListSharePinned(ctx, list.ID, stranger, true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for a user with no access at all, got %v", err)
	}
}

// TestListSpacesVisibleToUser exercises the Space-level "shared with me"
// query backing GET /api/v1/custom-categories?shared_with_me=true: it must
// include a shared category tagged with the recipient's own AccessSource/
// AccessPermission/IsPinnedToDashboard, reflect a pin toggle, and return
// nothing at all — not even the owner's other, unshared categories — for a
// user with no space_shares row and no House-based access either.
func TestListSpacesVisibleToUser(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")

	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "🏖️", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	if _, err := d.CreateCustomCategory(ctx, owner, "Perso", "", "", 1); err != nil {
		t.Fatalf("CreateCustomCategory (unshared): %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, recipient, "read"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}

	shared, err := d.ListSpacesVisibleToUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser: %v", err)
	}
	if len(shared) != 1 || shared[0].ID != category.ID {
		t.Fatalf("expected exactly the shared category, got %+v", shared)
	}
	if shared[0].AccessSource != "space_share" {
		t.Fatalf("expected AccessSource space_share, got %q", shared[0].AccessSource)
	}
	if shared[0].AccessPermission != "read" {
		t.Fatalf("expected AccessPermission read, got %q", shared[0].AccessPermission)
	}
	if shared[0].IsPinnedToDashboard {
		t.Fatalf("expected a freshly shared space to default to unpinned")
	}

	if _, err := d.SetSpaceSharePinned(ctx, category.ID, recipient, true); err != nil {
		t.Fatalf("SetSpaceSharePinned: %v", err)
	}
	shared, err = d.ListSpacesVisibleToUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser (after pin): %v", err)
	}
	if len(shared) != 1 || !shared[0].IsPinnedToDashboard {
		t.Fatalf("expected the shared category to come back pinned, got %+v", shared)
	}

	strangerShared, err := d.ListSpacesVisibleToUser(ctx, stranger)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser (stranger): %v", err)
	}
	if len(strangerShared) != 0 {
		t.Fatalf("expected no visible spaces at all for a stranger, got %+v", strangerShared)
	}
}

// TestSpaceAccessibleViaHouseMemberPin covers the bug report this session
// fixes: a House member who neither owns a Space nor holds a space_shares
// grant on it, but who can see it because a fellow House member tagged one
// of the House's own lists with it, must still be able to pin it — and a
// genuine stranger to the House must not.
func TestSpaceAccessibleViaHouseMemberPin(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	fellowMember := mustCreateUserWithEmail(t, ctx, d, "fellow@example.com")
	stranger := mustCreateUserWithEmail(t, ctx, d, "stranger@example.com")

	house, err := d.CreateHouseWithOwner(ctx, "Maison Principale", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := d.AddHouseMember(ctx, house.ID, fellowMember, "member"); err != nil {
		t.Fatalf("AddHouseMember: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "🏖️", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	if _, err := d.CreateList(ctx, "Courses vacances", "shopping", house.ID, &category.ID, ""); err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	// Sanity check: the fellow member has no space_shares row at all — only
	// House-based access — otherwise this test would prove nothing about
	// the fallback path it's meant to exercise.
	if _, err := d.GetSpaceShare(ctx, category.ID, fellowMember); err != ErrNotFound {
		t.Fatalf("test setup bug: expected no pre-existing space_shares row, got %v", err)
	}

	accessible, err := d.spaceAccessibleViaHouse(ctx, category.ID, fellowMember)
	if err != nil {
		t.Fatalf("spaceAccessibleViaHouse: %v", err)
	}
	if !accessible {
		t.Fatalf("expected the fellow House member to have House-based access to the category")
	}
	strangerAccessible, err := d.spaceAccessibleViaHouse(ctx, category.ID, stranger)
	if err != nil {
		t.Fatalf("spaceAccessibleViaHouse (stranger): %v", err)
	}
	if strangerAccessible {
		t.Fatalf("expected a stranger to the House to have no House-based access")
	}

	// The category's own owner is also, in the ordinary case, a House
	// member of wherever their own Space is used (they created the list
	// tagged with it, in their own House) — spaceAccessibleViaHouse must
	// still report false for them specifically, mirroring
	// ListSpacesVisibleToUser's own owner exclusion, so SetSpaceHousePinned
	// can never let an owner "pin" their own already-visible Space via this
	// fallback.
	ownerAccessible, err := d.spaceAccessibleViaHouse(ctx, category.ID, owner)
	if err != nil {
		t.Fatalf("spaceAccessibleViaHouse (owner): %v", err)
	}
	if ownerAccessible {
		t.Fatalf("expected the category's own owner to have no House-based access to their own Space")
	}
	if _, err := d.SetSpaceHousePinned(ctx, category.ID, owner, true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound pinning via house access for the category's own owner, got %v", err)
	}

	pinned, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, true)
	if err != nil {
		t.Fatalf("SetSpaceHousePinned(true): %v", err)
	}
	if !pinned {
		t.Fatalf("expected SetSpaceHousePinned(true) to report pinned")
	}
	// Pinning twice must not error (ON CONFLICT DO NOTHING).
	if _, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, true); err != nil {
		t.Fatalf("SetSpaceHousePinned(true) again: %v", err)
	}

	unpinned, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, false)
	if err != nil {
		t.Fatalf("SetSpaceHousePinned(false): %v", err)
	}
	if unpinned {
		t.Fatalf("expected SetSpaceHousePinned(false) to report unpinned")
	}

	if _, err := d.SetSpaceHousePinned(ctx, category.ID, stranger, true); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound pinning via house access for a non-member, got %v", err)
	}
}

// TestListSpacesVisibleToUserHouseMember covers ListSpacesVisibleToUser's
// AccessSource "house_member" branch: a category owned by one House member,
// used to tag a list in that House, must show up for a fellow member with
// AccessSource "house_member"/AccessPermission "write" and no
// space_shares row involved at all, reflecting a pin made via
// SetSpaceHousePinned.
func TestListSpacesVisibleToUserHouseMember(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	fellowMember := mustCreateUserWithEmail(t, ctx, d, "fellow@example.com")

	house, err := d.CreateHouseWithOwner(ctx, "Maison Principale", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := d.AddHouseMember(ctx, house.ID, fellowMember, "member"); err != nil {
		t.Fatalf("AddHouseMember: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	if _, err := d.CreateList(ctx, "Courses vacances", "shopping", house.ID, &category.ID, ""); err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	visible, err := d.ListSpacesVisibleToUser(ctx, fellowMember)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != category.ID {
		t.Fatalf("expected exactly the House-visible category, got %+v", visible)
	}
	if visible[0].AccessSource != "house_member" {
		t.Fatalf("expected AccessSource house_member, got %q", visible[0].AccessSource)
	}
	if visible[0].AccessPermission != "write" {
		t.Fatalf("expected AccessPermission write, got %q", visible[0].AccessPermission)
	}
	if visible[0].IsPinnedToDashboard {
		t.Fatalf("expected a freshly visible House space to default to unpinned")
	}

	if _, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, true); err != nil {
		t.Fatalf("SetSpaceHousePinned: %v", err)
	}
	visible, err = d.ListSpacesVisibleToUser(ctx, fellowMember)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser (after pin): %v", err)
	}
	if len(visible) != 1 || !visible[0].IsPinnedToDashboard {
		t.Fatalf("expected the House space to come back pinned, got %+v", visible)
	}

	// The category's owner never shows up in their own "visible to me" list.
	ownerVisible, err := d.ListSpacesVisibleToUser(ctx, owner)
	if err != nil {
		t.Fatalf("ListSpacesVisibleToUser (owner): %v", err)
	}
	if len(ownerVisible) != 0 {
		t.Fatalf("expected the owner to see none of their own categories here, got %+v", ownerVisible)
	}
}

// TestListPinnedHouseSpaceLists covers the other half of pinning a House
// Space: once pinned via SetSpaceHousePinned, its tagged lists must come
// back from ListPinnedHouseSpaceLists (the query backing
// GET /api/v1/lists?pinned_house_spaces=true, which is what actually
// surfaces them on the caller's dashboard) — and must disappear again once
// unpinned, and must never include a list from a House the caller doesn't
// belong to.
func TestListPinnedHouseSpaceLists(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	fellowMember := mustCreateUserWithEmail(t, ctx, d, "fellow@example.com")

	house, err := d.CreateHouseWithOwner(ctx, "Maison Principale", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	if _, err := d.AddHouseMember(ctx, house.ID, fellowMember, "member"); err != nil {
		t.Fatalf("AddHouseMember: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	list, err := d.CreateList(ctx, "Courses vacances", "shopping", house.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList: %v", err)
	}

	before, err := d.ListPinnedHouseSpaceLists(ctx, fellowMember)
	if err != nil {
		t.Fatalf("ListPinnedHouseSpaceLists (before pin): %v", err)
	}
	if len(before) != 0 {
		t.Fatalf("expected no pinned House-space lists before pinning, got %+v", before)
	}

	if _, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, true); err != nil {
		t.Fatalf("SetSpaceHousePinned: %v", err)
	}
	after, err := d.ListPinnedHouseSpaceLists(ctx, fellowMember)
	if err != nil {
		t.Fatalf("ListPinnedHouseSpaceLists (after pin): %v", err)
	}
	if len(after) != 1 || after[0].ID != list.ID {
		t.Fatalf("expected exactly the pinned House-space list, got %+v", after)
	}
	if after[0].AccessSource != "house_member" || after[0].AccessPermission != "write" || !after[0].IsPinnedToDashboard {
		t.Fatalf("expected AccessSource/AccessPermission/IsPinnedToDashboard to be populated, got %+v", after[0])
	}

	if _, err := d.SetSpaceHousePinned(ctx, category.ID, fellowMember, false); err != nil {
		t.Fatalf("SetSpaceHousePinned (unpin): %v", err)
	}
	afterUnpin, err := d.ListPinnedHouseSpaceLists(ctx, fellowMember)
	if err != nil {
		t.Fatalf("ListPinnedHouseSpaceLists (after unpin): %v", err)
	}
	if len(afterUnpin) != 0 {
		t.Fatalf("expected no pinned House-space lists after unpinning, got %+v", afterUnpin)
	}
}

// TestListSharedListsForUserSpacePinPropagates covers the other half of
// pinning a whole Space (see TestListSpacesSharedWithUser for the Espaces
// tab's own side of it): once a Space is pinned, every list reachable
// through it must come back pinned from ListSharedListsForUser too — the
// query the dashboard merge (static/js/shares.js's loadPinnedSharedLists)
// relies on — with no list_shares row created for any of them, and
// unpinning the Space must revert all of them at once.
func TestListSharedListsForUserSpacePinPropagates(t *testing.T) {
	ctx := context.Background()
	d := openTestDB(t)

	owner := mustCreateUser(t, ctx, d)
	recipient := mustCreateUserWithEmail(t, ctx, d, "recipient@example.com")
	house, err := d.CreateHouseWithOwner(ctx, "Maison d'un tiers", owner)
	if err != nil {
		t.Fatalf("creating house: %v", err)
	}
	category, err := d.CreateCustomCategory(ctx, owner, "Vacances", "", "", 0)
	if err != nil {
		t.Fatalf("CreateCustomCategory: %v", err)
	}
	listA, err := d.CreateList(ctx, "Liste A", "shopping", house.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList (A): %v", err)
	}
	listB, err := d.CreateList(ctx, "Liste B", "todo", house.ID, &category.ID, "")
	if err != nil {
		t.Fatalf("CreateList (B): %v", err)
	}
	if _, err := d.CreateOrUpdateSpaceShare(ctx, category.ID, recipient, "read"); err != nil {
		t.Fatalf("CreateOrUpdateSpaceShare: %v", err)
	}

	shared, err := d.ListSharedListsForUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSharedListsForUser: %v", err)
	}
	for _, l := range shared {
		if l.IsPinnedToDashboard {
			t.Fatalf("expected no list pinned before the Space itself is pinned, got %+v", l)
		}
	}

	if _, err := d.SetSpaceSharePinned(ctx, category.ID, recipient, true); err != nil {
		t.Fatalf("SetSpaceSharePinned: %v", err)
	}

	shared, err = d.ListSharedListsForUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSharedListsForUser (after space pin): %v", err)
	}
	byID := map[int64]bool{}
	for _, l := range shared {
		byID[l.ID] = l.IsPinnedToDashboard
	}
	if !byID[listA.ID] || !byID[listB.ID] {
		t.Fatalf("expected both lists to come back pinned once the Space is pinned, got %+v", byID)
	}
	if _, err := d.GetListShare(ctx, listA.ID, recipient); err != ErrNotFound {
		t.Fatalf("expected pinning the Space to create no list_shares row, got %v", err)
	}

	if _, err := d.SetSpaceSharePinned(ctx, category.ID, recipient, false); err != nil {
		t.Fatalf("SetSpaceSharePinned (unpin): %v", err)
	}
	shared, err = d.ListSharedListsForUser(ctx, recipient)
	if err != nil {
		t.Fatalf("ListSharedListsForUser (after space unpin): %v", err)
	}
	for _, l := range shared {
		if l.IsPinnedToDashboard {
			t.Fatalf("expected no list pinned after unpinning the Space, got %+v", l)
		}
	}
}
