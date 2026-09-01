-- Migration 15 ("push notifications"): Web Push subscriptions, plus the
-- two columns that back a recurring item's own due-date reminder.
--
-- push_subscriptions stores what a browser's PushManager.subscribe() hands
-- back (endpoint/p256dh/auth — see internal/webpush for how those are used
-- to encrypt and address a push) against the user who granted it.
-- UNIQUE (user_id, endpoint) is what lets
-- internal/db.CreatePushSubscription upsert rather than error when the same
-- user re-subscribes the same endpoint (a browser occasionally rotates a
-- subscription's keys for an unchanged endpoint, or a user simply re-enables
-- push after having granted it once already). ON DELETE CASCADE means a
-- deleted account silently stops receiving pushes rather than leaving an
-- orphaned row a future scan would otherwise still try (and fail) to
-- deliver to.
CREATE TABLE push_subscriptions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    endpoint   TEXT NOT NULL,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE (user_id, endpoint)
);

CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions(user_id);

-- recurrence_lead_minutes optionally overrides NOTIF_RECURRING_TASK_LEAD_TIME
-- (internal/config) for one specific recurring item — how long before
-- due_date internal/handlers.RunRecurringDueScan sends a reminder push. NULL
-- means "use the instance-wide default"; meaningless unless recurrence_rule
-- is also set, the same relationship due_date/recurrence_end_date already
-- have to it.
--
-- due_reminder_sent_for is deliberately NOT exposed anywhere in the API
-- (unlike recurrence_lead_minutes, it is not a models.Item field at all) —
-- it records the exact due_date value a reminder has already been sent for,
-- so RunRecurringDueScan's own query can exclude an item without a second
-- "clear the flag" step anywhere: the moment due_date changes for any reason
-- (the item's recurrence advancing on completion, or a manual edit), this
-- stored value simply stops matching due_date and the item re-arms itself.
ALTER TABLE items ADD COLUMN recurrence_lead_minutes INTEGER;
ALTER TABLE items ADD COLUMN due_reminder_sent_for TEXT;
