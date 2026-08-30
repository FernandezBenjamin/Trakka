package validate

import (
	"strings"
	"unicode/utf8"
)

// Length ceilings for the free-text fields the API accepts. None of these
// were bounded before: decodeJSON's 1 MiB body cap was the only limit, so a
// single request could persist a ~1 MiB "name" and every later response
// carrying that row would echo it back. They are deliberately generous —
// large enough that no realistic user input hits them, small enough that
// the database can't be inflated one field at a time.
const (
	MaxNameLen        = 200  // house / list / category names
	MaxTitleLen       = 500  // item titles
	MaxIconLen        = 32   // freeform emoji icon (a few code points, never a sentence)
	MaxDisplayNameLen = 100  // user display name
	MaxEmailLen       = 254  // RFC 5321 maximum forward-path length
	MaxURLLen         = 2048 // conventional practical URL ceiling
)

// Text normalizes a free-text field coming from a request body: it strips
// Unicode control characters (C0/C1, except nothing — none of them are
// meaningful in any of Trakka's single-line text fields) and trims
// surrounding whitespace.
//
// Control characters are removed rather than merely trimmed because they
// serve no purpose in a name/title and are the raw material for log
// injection (a newline in a value that later lands in a log line), terminal
// escape-sequence tricks when the same value is read with `sqlite3` or
// `jq` in a terminal, and bidirectional-override rendering games in the UI.
// The stored value stays plain text; nothing here is an XSS defense in its
// own right — that remains textContent/html-template escaping.
func Text(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == utf8.RuneError:
			return -1
		case r < 0x20, r == 0x7f: // C0 controls and DEL
			return -1
		case r >= 0x80 && r <= 0x9f: // C1 controls
			return -1
		case r >= 0x202a && r <= 0x202e: // bidi embedding/override
			return -1
		case r >= 0x2066 && r <= 0x2069: // bidi isolates
			return -1
		}
		return r
	}, raw)
	return strings.TrimSpace(cleaned)
}

// MaxLen reports whether s is within max characters (runes, not bytes, so a
// limit means the same thing regardless of the script the user types in).
func MaxLen(s string, max int) bool {
	return utf8.RuneCountInString(s) <= max
}
