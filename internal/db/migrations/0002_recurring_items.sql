-- Migration 2 ("Add reccurence for tasks"): recurring items. due_date is the
-- current occurrence's due date; recurrence_rule (DAILY/WEEKLY/MONTHLY/
-- YEARLY or EVERY_X_DAYS:<n>) drives internal/handlers.applyRecurrenceCompletion,
-- which advances due_date and un-checks the item on completion instead of
-- cloning a new row per occurrence. is_recurring is derived (always written
-- as recurrence_rule IS NOT NULL), never set independently.
ALTER TABLE items ADD COLUMN due_date TEXT;
ALTER TABLE items ADD COLUMN is_recurring INTEGER NOT NULL DEFAULT 0;
ALTER TABLE items ADD COLUMN recurrence_rule TEXT;
ALTER TABLE items ADD COLUMN recurrence_end_date TEXT;
