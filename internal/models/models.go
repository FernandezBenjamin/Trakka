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
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
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
	CreatedAt   string `json:"created_at"`
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
