// Package models defines the data types shared between the db and handlers
// layers.
package models

// House groups related lists together (e.g. shared by the members of a
// household). Every List belongs to exactly one House.
type House struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// List represents a shopping or to-do list.
type List struct {
	ID        int64   `json:"id"`
	HouseID   int64   `json:"house_id"`
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Items     []*Item `json:"items,omitempty"`
}

// Item represents a single entry within a List.
type Item struct {
	ID       int64    `json:"id"`
	ListID   int64    `json:"list_id"`
	Title    string   `json:"title"`
	URL      *string  `json:"url,omitempty"`
	Quantity int      `json:"quantity"`
	Price    *float64 `json:"price,omitempty"`
	// PriceAuto is true when Price was filled in automatically by
	// internal/scraper's background lookup rather than typed in by a user.
	// It is always false when Price is nil, and is reset to false the
	// moment a user sets or clears Price manually (see
	// internal/handlers/items.go) — it exists purely so the frontend can
	// show a "detected automatically" badge next to the price.
	PriceAuto bool `json:"price_auto"`
	// ImageURL is the item's product image, either filled in by
	// internal/scraper's background lookup (the same one that fills in
	// Price) or left nil if none was found. Unlike Price/PriceAuto there is
	// no manual "set your own image" input anywhere in the API — it is
	// scraper-only, and is cleared back to nil whenever URL changes to
	// something new (see internal/handlers/items.go), since an image
	// scraped for the previous URL no longer describes the current one.
	ImageURL *string `json:"image_url,omitempty"`
	Done     bool    `json:"done"`
	Position int     `json:"position"`
	// TargetMonth is the month (YYYY-MM) an item's purchase is planned for,
	// used by the "Budget & Prévisions Achats" planning view to group and
	// total upcoming spending. Nil means the item isn't scheduled.
	TargetMonth *string `json:"target_month,omitempty"`
	// DueDate (YYYY-MM-DD) is the date this item (or, for a recurring item,
	// its current occurrence) is due. Nil means no due date has been set
	// yet. For a recurring item it is advanced automatically each time the
	// item is completed (see internal/handlers.applyRecurrenceCompletion)
	// rather than edited directly through the UI.
	DueDate *string `json:"due_date,omitempty"`
	// IsRecurring is true exactly when RecurrenceRule is set — it is never
	// set independently, purely a convenience so callers don't have to
	// check RecurrenceRule for non-nilness themselves.
	IsRecurring bool `json:"is_recurring"`
	// RecurrenceRule is one of the fixed cadences ("DAILY", "WEEKLY",
	// "MONTHLY", "YEARLY") or the custom "EVERY_X_DAYS:<n>" form (see
	// internal/validate.Recurrence), or nil if the item doesn't repeat.
	// Completing a recurring item doesn't delete or clone it — it advances
	// DueDate to the next occurrence and resets Done to false instead (see
	// internal/handlers.applyRecurrenceCompletion), so the same row is
	// reused indefinitely rather than accumulating one row per occurrence.
	RecurrenceRule *string `json:"recurrence_rule,omitempty"`
	// RecurrenceEndDate (YYYY-MM-DD), if set, is the last date this item
	// should recur on: once the next computed occurrence would fall after
	// it, the item stops advancing and simply stays done, like a
	// non-recurring item. Nil means the recurrence never ends on its own.
	RecurrenceEndDate *string `json:"recurrence_end_date,omitempty"`
	// IsUrgent flags an item that needs attention right away (e.g. "out of
	// toilet paper"). It's a plain user-set toggle, independent of
	// TargetMonth/RecurrenceRule — unlike IsRecurring it has no derived
	// relationship to another field. The frontend sorts an unfinished urgent
	// item to the top of its list and surfaces it in the cross-list
	// "Achats & Tâches Urgentes" dashboard widget (static/js/urgent.js).
	IsUrgent  bool   `json:"is_urgent"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// PriceStatus is a transient, response-only field set by
	// internal/handlers.scrapePrice after a create/update/patch — never
	// persisted, and never populated by a plain GET (it's the zero value
	// there, which omitempty drops from the JSON entirely). One of
	// "found" (a price is present, manual or freshly scraped), "pending"
	// (a background lookup is still running and will land later via
	// db.UpdateItemPriceIfMissing), or "none" (no url to scrape, or
	// nothing found within the request's bounded wait).
	PriceStatus string `json:"price_status,omitempty"`
}

// ValidListTypes enumerates the allowed values for List.Type.
var ValidListTypes = map[string]bool{
	"shopping": true,
	"todo":     true,
}

// User is the API-facing account shape. It never carries a password hash
// or OIDC identity — those live only in UserWithCredentials, which is
// returned solely to internal/auth and must never be passed to writeJSON.
type User struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	// IsAdmin grants access to the /api/v1/admin/... endpoints and the
	// "Paramètres du Système" panel (see internal/handlers/admin.go). It is
	// never settable through the registration/profile API — the only way to
	// become an admin is being the very first account created on a fresh
	// instance (internal/db.CreateUser).
	IsAdmin   bool   `json:"is_admin"`
	CreatedAt string `json:"created_at"`
}

// UserWithCredentials is returned by db lookups used for authentication
// (GetUserByEmail, GetUserByOIDCSubject). Never pass this to writeJSON.
type UserWithCredentials struct {
	User
	PasswordHash *string
	OIDCSubject  *string
	OIDCIssuer   *string
}

// Session is an opaque server-side session. ID holds the hex-encoded
// SHA-256 of the raw cookie token, never the raw token itself, and is
// never API-serialized.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt string
	CreatedAt string
}

// HouseMember links a User to a House with a role. Email/DisplayName are
// populated by a JOIN for the member-roster endpoint only.
type HouseMember struct {
	HouseID     int64  `json:"house_id"`
	UserID      int64  `json:"user_id"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// ValidHouseRoles enumerates the allowed values for HouseMember.Role.
var ValidHouseRoles = map[string]bool{
	"owner":  true,
	"member": true,
}

// PriceAlert records a lower price internal/scraper found for an item
// versus its current price, awaiting a user decision (see
// internal/handlers/price_alerts.go). It is only ever created by the
// periodic background scan or an on-demand check, never directly through
// the API. OriginalPrice is a snapshot of the item's price at the moment
// the alert was created, not re-read live from the item, so a notification
// always reflects what the comparison was actually made against even if
// the item's price changes in the meantime.
type PriceAlert struct {
	ID     int64 `json:"id"`
	ItemID int64 `json:"item_id"`
	// ItemTitle/ListID are populated by a JOIN for display and
	// authorization purposes only (mirroring HouseMember's
	// Email/DisplayName) — never written back to the database.
	ItemTitle     string  `json:"item_title"`
	ListID        int64   `json:"list_id"`
	OriginalPrice float64 `json:"original_price"`
	FoundPrice    float64 `json:"found_price"`
	SourceURL     string  `json:"source_url"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

// ValidPriceAlertStatuses enumerates the allowed values for
// PriceAlert.Status, and the "status" filter accepted by
// GET /api/v1/price-alerts.
var ValidPriceAlertStatuses = map[string]bool{
	"pending":  true,
	"accepted": true,
	"rejected": true,
}
