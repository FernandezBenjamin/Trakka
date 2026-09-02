package validate

import "strings"

// SupportedLanguages are the UI languages Trakka's frontend ships
// dictionaries for (static/locales/{fr,en}.json) — see static/js/i18n.js.
var SupportedLanguages = map[string]bool{"fr": true, "en": true}

// Language normalizes (lowercases, trims) a language code and reports
// whether it's one of SupportedLanguages. Shared by internal/config (the
// DEFAULT_APP_LANGUAGE env var) and internal/handlers (a user's own
// PATCH /api/v1/me {"language"} choice) so both agree on what's valid.
func Language(raw string) (string, bool) {
	lang := strings.ToLower(strings.TrimSpace(raw))
	return lang, SupportedLanguages[lang]
}
