'use strict';

// Anti-FOUC theme bootstrap. Loaded as the very first thing in <head> (in
// both static/index.html and templates/login.html) — before the
// tokens.css/base.css <link> tags — so [data-theme] already exists on
// <html> by the time tokens.css's [data-theme="dark"] rules and every
// dark: Tailwind utility are first applied. It has to be an external file
// rather than an inline <script>: this app's CSP (internal/handlers's
// SecurityHeaders) sets script-src to 'self' plus the Tailwind CDN host
// only, with no 'unsafe-inline' — an inline script here would simply be
// silently blocked by the browser rather than executed.
//
// Kept intentionally tiny and dependency-free. static/js/theme.js (loaded
// later, once the DOM exists) owns the full theme system: persistence,
// the header menu, and staying in sync with the OS while "Auto" is
// selected — this file only handles the one-time synchronous resolution
// that script can't do fast enough to prevent a flash.
(function () {
  try {
    var pref = localStorage.getItem('trakka:theme') || 'auto';
    var resolved = pref === 'auto'
      ? (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')
      : pref;
    document.documentElement.setAttribute('data-theme', resolved === 'dark' ? 'dark' : 'light');
  } catch (e) {
    // localStorage/matchMedia unavailable (private mode, very old browser, ...) — the
    // @media (prefers-color-scheme: dark) fallback in tokens.css still applies.
  }
})();
