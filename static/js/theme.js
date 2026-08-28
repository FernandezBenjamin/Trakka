'use strict';

// Light/Dark/Auto theme switcher. The synchronous, dependency-free
// anti-FOUC bootstrap that actually decides what paints first lives inline
// in static/index.html's <head> (it has to run before tokens.css and the
// dark: Tailwind utilities are applied, which is earlier than this file —
// loaded as a classic <script> at the bottom of <body> — could ever run).
// This file owns everything after that first paint: persistence, the
// header dropdown (mirrors static/js/i18n.js's lang-button/lang-menu
// pattern), keeping <meta name="theme-color"> in sync, and — the one
// thing the inline bootstrap can't do — staying live-updated against the
// OS setting for as long as "Auto" stays selected.
(function () {
  const THEME_STORAGE_KEY = 'trakka:theme';
  const THEME_OPTIONS = ['light', 'dark', 'auto'];
  const DEFAULT_THEME = 'auto';
  const THEME_COLOR = { light: '#ffffff', dark: '#0f172a' };
  const THEME_ICON = { light: '☀️', dark: '🌙' };

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
    const icon = document.getElementById('theme-button-icon');
    if (icon) icon.textContent = THEME_ICON[resolved];
    document.querySelectorAll('#theme-menu [data-theme-option]').forEach((btn) => {
      btn.setAttribute('aria-current', btn.getAttribute('data-theme-option') === pref ? 'true' : 'false');
    });
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

  function wireThemeSwitcher() {
    const button = document.getElementById('theme-button');
    const menu = document.getElementById('theme-menu');
    if (!button || !menu) return;

    function closeMenu() {
      menu.hidden = true;
      button.setAttribute('aria-expanded', 'false');
    }
    function toggleMenu() {
      const willOpen = menu.hidden;
      menu.hidden = !willOpen;
      button.setAttribute('aria-expanded', String(willOpen));
    }

    button.addEventListener('click', (event) => {
      event.stopPropagation();
      toggleMenu();
    });
    menu.querySelectorAll('[data-theme-option]').forEach((option) => {
      option.addEventListener('click', () => {
        setTheme(option.getAttribute('data-theme-option'));
        closeMenu();
      });
    });
    document.addEventListener('click', (event) => {
      if (!menu.hidden && !menu.contains(event.target) && event.target !== button) closeMenu();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape' && !menu.hidden) closeMenu();
    });
  }

  window.TrakkaTheme = { get: getStoredPref, set: setTheme };

  document.addEventListener('DOMContentLoaded', () => {
    applyTheme(getStoredPref());
    wireThemeSwitcher();
  });
})();
