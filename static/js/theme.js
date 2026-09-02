'use strict';

// Light/Dark/Auto theme switcher. The synchronous, dependency-free
// anti-FOUC bootstrap that actually decides what paints first lives inline
// in static/index.html's <head> (it has to run before tokens.css and the
// dark: Tailwind utilities are applied, which is earlier than this file —
// loaded as a classic <script> at the bottom of <body> — could ever run).
// This file owns everything after that first paint: persistence, keeping
// <meta name="theme-color"> in sync, and — the one thing the inline
// bootstrap can't do — staying live-updated against the OS setting for as
// long as "Auto" stays selected. The interactive picker itself is the
// #user-settings-theme <select> inside #user-settings-modal, owned and
// wired by static/js/settings.js (this file only exposes window.TrakkaTheme
// for it to call) — there used to be a header dropdown here instead; see
// CLAUDE.md's session-handoff log for the cleanup that moved it into the
// "Paramètres" modal alongside the language picker.
(function () {
  const THEME_STORAGE_KEY = 'trakka:theme';
  const THEME_OPTIONS = ['light', 'dark', 'auto'];
  const DEFAULT_THEME = 'auto';
  const THEME_COLOR = { light: '#ffffff', dark: '#0f172a' };

  const media = window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)') : null;

  function getStoredPref() {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    return THEME_OPTIONS.includes(stored) ? stored : DEFAULT_THEME;
  }

  function resolve(pref) {
    if (pref === 'auto') return media && media.matches ? 'dark' : 'light';
    return pref;
  }

  function applyTheme(pref) {
    const resolved = resolve(pref);
    document.documentElement.setAttribute('data-theme', resolved);
    const meta = document.getElementById('theme-color-meta');
    if (meta) meta.setAttribute('content', THEME_COLOR[resolved]);
  }

  function setTheme(pref) {
    if (!THEME_OPTIONS.includes(pref)) return;
    localStorage.setItem(THEME_STORAGE_KEY, pref);
    applyTheme(pref);
    document.dispatchEvent(new CustomEvent('trakka:theme-changed', { detail: { pref, resolved: resolve(pref) } }));
  }

  // While "Auto" is selected, follow the OS live (e.g. the system switches
  // to dark at sunset) without needing a page reload.
  if (media && media.addEventListener) {
    media.addEventListener('change', () => {
      if (getStoredPref() === 'auto') applyTheme('auto');
    });
  }

  window.TrakkaTheme = { get: getStoredPref, set: setTheme };

  document.addEventListener('DOMContentLoaded', () => {
    applyTheme(getStoredPref());
  });
})();
