package db

import "errors"

// ErrNotFound is returned by lookup methods when no row matches the given
// id. Handlers translate this into an HTTP 404 without needing to know
// anything about the underlying storage (e.g. database/sql.ErrNoRows).
var ErrNotFound = errors.New("not found")

// ErrDuplicateEmail is returned by CreateUser when the email is already
// registered (detected from the users.email UNIQUE constraint).
var ErrDuplicateEmail = errors.New("email already registered")

// ErrAlreadyMember is returned by AddHouseMember when the user is already a
// member of the house (detected from the house_members composite primary
// key constraint).
var ErrAlreadyMember = errors.New("already a member")

// ErrInvalidReorder is returned by ReorderItems when the given item ids
// aren't exactly a permutation of the target list's current items (missing
// an item, repeating one, or naming one that belongs to a different list) —
// a partial or foreign set would leave positions ambiguous, so the whole
// request is rejected rather than applying whatever prefix does match.
var ErrInvalidReorder = errors.New("invalid reorder: item_ids must exactly match the list's current items")
