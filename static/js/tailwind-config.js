'use strict';

// Must be a separate external file loaded after the Tailwind CDN <script>
// (same reasoning as theme-init.js: this app's CSP has no 'unsafe-inline'
// for script-src, so an inline config assignment would be silently
// blocked rather than executed). Tailwind's dark: variant is gated on
// this exact attribute selector so it flips in lockstep with
// tokens.css's [data-theme="dark"] rules — both are driven by the single
// data-theme attribute theme-init.js (and later static/js/theme.js) set
// on <html>.
tailwind.config = { darkMode: ['selector', '[data-theme="dark"]'] };
