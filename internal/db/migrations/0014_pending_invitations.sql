-- Migration 14 ("pending invitations"): removes the account-enumeration
-- oracle from the invite/share endpoints, and — as a direct consequence —
-- finally makes it possible to invite somebody who has not registered yet.
--
-- Before this table, POST /houses/{id}/members and POST
-- /{lists,custom-categories}/{id}/share resolved the submitted email to a
-- user row and answered 404 "no account exists for this email yet" when
-- there was none. That answer is an oracle: any authenticated user could ask
-- the instance whether an arbitrary email address has an account here, one
-- request at a time. The reply is now identical whether or not the address
-- is registered, which is only honest because the invitation is genuinely
-- recorded either way — that is what this table holds.
--
-- A pending invitation is keyed by email rather than by user id, precisely
-- because the point is not to know whether a user exists at the moment it is
-- created. It is turned into a real house_members / list_shares /
-- space_shares row by internal/db.MaterializePendingInvitations, called when
-- the invited person next authenticates. Nothing here is ever granted to
-- somebody who has not signed in themselves, so an invitation cannot be used
-- to attach data to an address its owner does not actually control.
--
-- target_id is polymorphic (a house id, a list id, or a custom_categories
-- id, per `kind`) and therefore cannot carry a foreign key. Materialization
-- re-checks that the target still exists and drops the row when it does not,
-- so a deleted house or list leaves no invitation that could later resurrect
-- access to something that no longer exists.
CREATE TABLE pending_invitations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    kind        TEXT NOT NULL CHECK (kind IN ('house', 'list', 'space')),
    target_id   INTEGER NOT NULL,
    email       TEXT NOT NULL COLLATE NOCASE,
    -- 'read'/'write' for a list/space share; '' for a house invitation,
    -- where membership carries its own role rather than a permission.
    permission  TEXT NOT NULL DEFAULT '' CHECK (permission IN ('', 'read', 'write')),
    invited_by  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (kind, target_id, email)
);

CREATE INDEX idx_pending_invitations_email ON pending_invitations(email);
