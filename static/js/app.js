'use strict';

const API_BASE = '/api/v1';

// `state.currentList` is the full list detail (with items) for whichever
// list is currently open — list_view.js owns rendering it and applies
// optimistic edits directly to its `items` array before the matching
// request resolves. Kept here (rather than in list_view.js) only because
// dashboard navigation (selectList's caller) also needs `currentListId`.
const state = {
  currentListId: null,
  currentList: null,
  currentHouseId: null,
  currentUser: null,
  houses: [],
};

// Remembers the last house selected across reloads (per-browser only,
// nothing shared with the server) so switching houses sticks between visits.
const HOUSE_STORAGE_KEY = 'trakka:currentHouseId';

// Whichever dashboard tab or list was last visited, saved as JSON — either
// { type: 'tab', tab: 'planning' } or { type: 'list', id: 42 } — so
// relaunching the app can reopen there instead of always landing on the
// dashboard (the "keep last page on launch" preference below). Written by
// saveLastView, called from list_view.js's selectList/showDashboard and
// planning.js's setActiveTab — the only three places that change what's
// visible.
const LAST_VIEW_STORAGE_KEY = 'trakka:lastView';

// Local mirror of state.currentUser.keep_last_page (the server-side
// preference, PATCHed via /api/v1/me — see static/js/settings.js), read
// before the /me call resolves so the very first paint can already decide
// whether a restore should even be attempted, without blocking on a network
// round-trip that might never come back (e.g. while offline). The server
// value is authoritative and re-synced into this mirror every time /me
// succeeds (see init() below); this key exists purely to avoid the "restore
// or not" decision depending on a round-trip that might not have resolved
// yet at startup.
const KEEP_LAST_PAGE_STORAGE_KEY = 'trakka:keepLastPage';

function isKeepLastPageEnabled() {
  if (state.currentUser) return state.currentUser.keep_last_page;
  const stored = localStorage.getItem(KEEP_LAST_PAGE_STORAGE_KEY);
  // Defaults to enabled, matching the server column's own DEFAULT 1 (see
  // internal/db/migrations/0010_keep_last_page_preference.sql) for a user
  // whose preference hasn't been fetched yet.
  return stored === null ? true : stored === 'true';
}

// Keeps the localStorage mirror in step with the server value — called once
// state.currentUser is known (init()) and again right after settings.js
// PATCHes a change, so neither has to wait on the other to be correct.
function setKeepLastPagePreference(enabled) {
  localStorage.setItem(KEEP_LAST_PAGE_STORAGE_KEY, String(enabled));
}

// Persists whichever view is now visible. A no-op while the preference is
// off, so turning it off needs no separate cleanup — loadLastView is simply
// never consulted again until it's turned back on, at which point whatever
// was last saved (possibly stale) is used, same as any other "remember my
// last choice" setting in this app (house, theme, language).
function saveLastView(view) {
  if (!isKeepLastPageEnabled()) return;
  try {
    localStorage.setItem(LAST_VIEW_STORAGE_KEY, JSON.stringify(view));
  } catch {
    // Storage unavailable (private browsing, quota) — losing this
    // convenience is harmless, so it's silently ignored.
  }
}

function loadLastView() {
  try {
    const raw = localStorage.getItem(LAST_VIEW_STORAGE_KEY);
    return raw ? JSON.parse(raw) : null;
  } catch {
    return null;
  }
}

// Tabs restorable via setActiveTab (planning.js) — 'dashboard' is excluded
// since it's already the default view, needing no restore step at all.
const RESTORABLE_TABS = new Set(['planning', 'urgent', 'spaces', 'shared']);

// Reopens whichever tab or list was last visited. Called once, at the very
// end of init(), after state.currentUser/state.currentHouseId and the
// dashboard are all already resolved. Both branches fail silently (leaving
// whatever the dashboard already painted) rather than surfacing an error —
// a stale/deleted/no-longer-accessible list or an invalid tab name is not
// worth interrupting startup over, since the default "just show the
// dashboard" outcome is already correct in that case.
async function restoreLastView() {
  if (!isKeepLastPageEnabled()) return;
  const view = loadLastView();
  if (!view) return;

  if (view.type === 'list' && Number.isInteger(view.id)) {
    // selectList is defined in list_view.js, resolved lazily the same way
    // every other cross-file call in this function already is — safe here
    // since restoreLastView only ever runs from the tail of init(), well
    // after every script tag has finished loading and defined its
    // top-level functions.
    await selectList(view.id, { silent: true });
    return;
  }
  if (view.type === 'tab' && RESTORABLE_TABS.has(view.tab)) {
    // setActiveTab is defined in planning.js — same lazy-resolution
    // reasoning as selectList above.
    setActiveTab(view.tab);
  }
}

// Ids of lists currently sitting in their undo grace period (see
// removeList below) — a list in here is hidden from the dashboard even
// though it hasn't actually been deleted server-side yet, so a re-render
// triggered by something else during the 5s window (switching houses,
// creating another list, ...) can't make it flash back into view.
const pendingDeletedListIds = new Set();

// Shell/dashboard elements only — the list-detail view's own elements are
// cached separately in list_view.js's `listEls`.
const els = {
  networkDot: document.getElementById('network-dot'),
  networkLabel: document.getElementById('network-label'),
  pendingBadge: document.getElementById('pending-badge'),
  errorBanner: document.getElementById('error-banner'),
  updateBanner: document.getElementById('update-banner'),
  updateReloadButton: document.getElementById('update-reload-button'),
  logoLink: document.getElementById('logo-link'),
  listsSection: document.getElementById('lists-section'),
  houseToolbar: document.getElementById('house-toolbar'),
  houseSelect: document.getElementById('house-select'),
  renameHouseInlineButton: document.getElementById('rename-house-inline-button'),
  renameHouseInlineForm: document.getElementById('rename-house-inline-form'),
  renameHouseInlineInput: document.getElementById('rename-house-inline-input'),
  cancelRenameHouseInlineButton: document.getElementById('cancel-rename-house-inline-button'),
  shoppingLists: document.getElementById('shopping-lists'),
  todoLists: document.getElementById('todo-lists'),
  customLists: document.getElementById('custom-lists'),
  newListButton: document.getElementById('new-list-button'),
  newListModal: document.getElementById('new-list-modal'),
  newListModalTitle: document.getElementById('new-list-modal-title'),
  closeModalButton: document.getElementById('close-modal-button'),
  createListForm: document.getElementById('create-list-form'),
  listNameInput: document.getElementById('list-name'),
  listIconInput: document.getElementById('list-icon'),
  listIconPresetButtons: document.querySelectorAll('#create-list-form [data-list-icon-preset]'),
  typeOptions: document.querySelectorAll('#create-list-form [data-type-option]'),
  listCategorySelect: document.getElementById('list-category'),
  listSubmitButton: document.getElementById('list-submit-button'),
  newHouseModal: document.getElementById('new-house-modal'),
  closeHouseModalButton: document.getElementById('close-house-modal-button'),
  createHouseForm: document.getElementById('create-house-form'),
  houseNameInput: document.getElementById('house-name'),
  manageMembersButton: document.getElementById('manage-members-button'),
  membersModal: document.getElementById('members-modal'),
  closeMembersModalButton: document.getElementById('close-members-modal-button'),
  membersList: document.getElementById('members-list'),
  inviteMemberForm: document.getElementById('invite-member-form'),
  inviteEmailInput: document.getElementById('invite-email'),
};

const CREATE_HOUSE_OPTION_VALUE = '__create__';

// Thin wrapper around TrakkaI18n.t (i18n.js, loaded before this file) that
// degrades to the raw key if the module somehow failed to load — mirrors
// the `!window.TrakkaDB` guard already used below for the optional offline
// module.
function t(key, vars) {
  return window.TrakkaI18n ? window.TrakkaI18n.t(key, vars) : key;
}

// Static, hard-coded icon markup (never interpolates user data) — safe to
// insert via innerHTML. Never reuse this pattern for anything containing a
// list/item title, URL, or other user-supplied value; use textContent or
// createElement for that instead (see buildListCard below, and
// buildItemRow in list_view.js).
const TRASH_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5" aria-hidden="true">' +
  '<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2m3 0-1 14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2L4 6h16Z"/></svg>';

// Same static-markup-only safety rule as TRASH_ICON_SVG above. Used for the
// 👥 "Partager" button on a list card/space section (see buildListCard
// below and spaces.js's buildCategorySection) and for the 👥 "shared with
// you" indicator on a card in the "Partagé avec moi" tab (shares.js).
const SHARE_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5" aria-hidden="true">' +
  '<path d="M16 11a4 4 0 1 0-4-4"/><path d="M8 21v-2a4 4 0 0 1 4-4h1"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/></svg>';

// Same static-markup-only safety rule as TRASH_ICON_SVG above. Used for the
// 📌 pin/unpin button buildListCard shows on a card reached via a direct
// List share (list.access_source === 'list_share') — see toggleListPin and
// the "Pinning shared lists" feature in CLAUDE.md.
const PIN_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5" aria-hidden="true">' +
  '<path d="M12 17v5"/><path d="M9 10.76a2 2 0 0 1-1.11 1.79l-1.78.9A2 2 0 0 0 5 15.24V16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-.76a2 2 0 0 0-1.11-1.79l-1.78-.9A2 2 0 0 1 15 10.76V7a1 1 0 0 1 1-1 2 2 0 0 0 0-4H8a2 2 0 0 0 0 4 1 1 0 0 1 1 1z"/></svg>';

function showError(message) {
  els.errorBanner.textContent = message;
  els.errorBanner.hidden = false;
}

function hideError() {
  els.errorBanner.hidden = true;
  els.errorBanner.textContent = '';
}

// isSafeHttpUrl re-checks, client-side, that a URL is absolute http(s).
// The backend already enforces this before persisting anything, but
// re-validating before ever setting an <a href> is cheap defense in depth.
function isSafeHttpUrl(value) {
  try {
    const parsed = new URL(value, window.location.origin);
    return parsed.protocol === 'http:' || parsed.protocol === 'https:';
  } catch {
    return false;
  }
}

// apiRequest wraps fetch for the JSON API: it always sets the JSON content
// type, parses JSON responses, and turns network failures or non-2xx
// responses into a single Error with a user-facing message. A request the
// service worker queued while offline still comes back as a 2xx (202), so
// no special-casing is needed here for that path.
async function apiRequest(path, options = {}) {
  const method = (options.method || 'GET').toUpperCase();

  // navigator.onLine is a synchronous, immediate signal — when it's already
  // false, skip attempting fetch() at all for a read rather than waiting on
  // one that's guaranteed to fail (which, without a controlling service
  // worker, can take several seconds to time out rather than reject
  // instantly). This is what makes a reload while offline paint every
  // GET-driven view (loadHouses/loadDashboard here, selectList/
  // refreshCurrentList in list_view.js, and planning.js/urgent.js/
  // spaces.js's tab loaders) from IndexedDB instantly instead of stalling
  // first. Never short-circuit a write (POST/PUT/PATCH/DELETE) this way: it
  // still has to reach the service worker's fetch handler even while
  // offline, since that's what queues it for later replay (see sw.js's
  // handleApiWrite/queueOfflineWrite) — skipping fetch() here would silently
  // drop the write instead of queuing it. navigator.onLine can still say
  // `true` on a connection that can't actually reach the server (a captive
  // portal, a down server, ...); that case is unaffected and still runs the
  // normal fetch()-then-catch path below.
  if (method === 'GET' && !navigator.onLine) {
    const err = new Error('Impossible de contacter le serveur. Vérifiez votre connexion.');
    err.isNetworkError = true;
    throw err;
  }

  let response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      headers: { 'Content-Type': 'application/json' },
      // Explicit even though 'same-origin' has been the fetch() spec
      // default since 2017: some embedded/WebKit mobile browsers (notably
      // iOS Safari's standalone "Add to Home Screen" PWA mode) have been
      // unreliable about implicit defaults for cookie handling, so this is
      // spelled out rather than relied on implicitly.
      credentials: 'same-origin',
      ...options,
    });
  } catch {
    // fetch() itself failed (no connectivity, DNS failure, ...) rather than
    // the server answering with a non-2xx status — tagged so read-path
    // callers (loadDashboard, planning.js/urgent.js/spaces.js's loaders, ...)
    // can tell "we're offline" apart from a genuine server-side error and
    // skip the blocking error banner for the former, since the header's
    // discreet network badge already communicates that on its own. See
    // isNetworkError below.
    const err = new Error('Impossible de contacter le serveur. Vérifiez votre connexion.');
    err.isNetworkError = true;
    throw err;
  }

  // The session cookie is missing or expired: every call in app.js and
  // list_view.js funnels through here, so this one check auth-gates the
  // whole SPA, including mid-session expiry. Never resolves so the caller
  // simply stops running rather than acting on a response that never came.
  if (response.status === 401) {
    window.location.href = '/auth/login';
    return new Promise(() => {});
  }

  const text = await response.text();
  let body = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = null;
    }
  }

  if (!response.ok) {
    const message = body && typeof body.error === 'string' ? body.error : `Erreur ${response.status}`;
    throw new Error(message);
  }

  return body;
}

// True for the Error apiRequest throws when fetch() itself failed, as
// opposed to the server answering with a non-2xx status. A read-path caller
// (any GET-driven loader: loadDashboard here, and planning.js/urgent.js/
// spaces.js's loaders) should never surface the blocking error banner for
// this case while navigating — the header's network dot + "Hors-ligne"
// label already say so on their own, and popping a banner on top of an
// already-rendered cached view reads as an intrusive false alarm rather
// than useful information. A genuine server-side error (5xx, a bug) has no
// such flag and should still be surfaced.
function isNetworkError(err) {
  return Boolean(err && err.isNetworkError);
}

// ---------------------------------------------------------------------------
// Network status + pending sync count
// ---------------------------------------------------------------------------

async function updateNetworkStatus() {
  let reachable = navigator.onLine;
  if (reachable) {
    try {
      const res = await fetch('/healthz', { cache: 'no-store' });
      reachable = res.ok;
    } catch {
      reachable = false;
    }
  }

  els.networkDot.classList.toggle('bg-emerald-400', reachable);
  els.networkDot.classList.toggle('bg-amber-400', !reachable);
  els.networkLabel.textContent = reachable ? t('header.online') : t('header.offline');

  await refreshPendingBadge();
}

// Reads the offline sync queue directly from IndexedDB (via db.js) so the
// user can see how many changes are waiting to reach the server — the
// service worker owns writing to that queue, this only ever reads it.
async function refreshPendingBadge() {
  if (!window.TrakkaDB) {
    els.pendingBadge.hidden = true;
    return;
  }
  try {
    const queue = await window.TrakkaDB.getQueue();
    if (queue.length > 0) {
      els.pendingBadge.hidden = false;
      els.pendingBadge.textContent = t('header.pending', { count: queue.length });
    } else {
      els.pendingBadge.hidden = true;
    }
  } catch {
    els.pendingBadge.hidden = true;
  }
}

// ---------------------------------------------------------------------------
// Houses (top-of-dashboard selector that scopes which lists are shown)
// ---------------------------------------------------------------------------

function populateHouseSelect(houses) {
  els.houseSelect.replaceChildren();
  for (const house of houses) {
    const option = document.createElement('option');
    option.value = String(house.id);
    option.textContent = house.name;
    els.houseSelect.appendChild(option);
  }
  const createOption = document.createElement('option');
  createOption.value = CREATE_HOUSE_OPTION_VALUE;
  createOption.textContent = t('dashboard.createHouseOption');
  els.houseSelect.appendChild(createOption);
}

// Reads the IndexedDB houses mirror, defaulting to [] whenever the module
// isn't available or the read itself fails (e.g. private-browsing profiles
// without IndexedDB) — the shared fallback used whenever a network call
// can't reach the server, so the dashboard degrades to "last known" data
// instead of going blank.
async function cachedHouses() {
  if (!window.TrakkaDB) return [];
  try {
    return await window.TrakkaDB.getHouses();
  } catch {
    return [];
  }
}

// Loads the house list, restores the last-selected house from localStorage
// when it still exists, and otherwise falls back to the first house. Does
// NOT load the dashboard itself — callers do that afterward once
// state.currentHouseId is settled.
async function loadHouses() {
  let houses;
  try {
    houses = await apiRequest('/houses');
  } catch (err) {
    // Offline, or a transient server error (non-2xx): fall back to
    // whatever the IndexedDB mirror last saw rather than wiping the
    // dashboard to empty — see the Offline-First requirement in
    // CLAUDE.md/docs/PWA.md. Never surface the banner for a plain
    // connectivity failure (the header's network badge already covers
    // that); only a genuine server-side error with nothing cached to show
    // for it still gets one.
    houses = await cachedHouses();
    if (houses.length === 0 && !isNetworkError(err)) showError(err.message);
  }

  state.houses = houses;
  populateHouseSelect(houses);

  const stored = Number(localStorage.getItem(HOUSE_STORAGE_KEY));
  const storedIsValid = houses.some((house) => house.id === stored);
  state.currentHouseId = storedIsValid ? stored : (houses[0]?.id ?? null);

  els.houseSelect.value = state.currentHouseId !== null ? String(state.currentHouseId) : CREATE_HOUSE_OPTION_VALUE;
  updateManageMembersButton();
  updateRenameHouseButton();
}

// Tracks whether loadHouses() has resolved at least once for this page
// load, and the in-flight promise while it hasn't. state.currentHouseId is
// only ever authoritative once loadHouses() has validated it against a
// live GET /api/v1/houses (see loadHouses above) — before that it's either
// null or whatever hydrateFromCache() derived from the IndexedDB mirror, a
// value that can be stale (e.g. left over from a different account that
// was previously signed in on this same browser) and isn't guaranteed to
// still belong to the current session. Several independent triggers can
// fire a house-scoped fetch — the initial load itself, a language switch,
// the tab regaining visibility, the 'online' event, and a
// trakka-sync-complete message from the service worker — and any of them
// firing before the first loadHouses() has resolved would hit the backend
// with a house_id the caller doesn't actually have access to yet, which
// correctly 403s but surfaces as a spurious "not a member of this house"
// error banner. ensureHousesLoaded() is the single choke point every
// house-scoped loader (loadDashboard below, notifications.js's
// loadNotifications) awaits first, so none of them can ever run ahead of
// the one authoritative resolution regardless of which trigger fires it.
let housesLoadedOnce = false;
let housesLoadingPromise = null;

async function ensureHousesLoaded() {
  if (housesLoadedOnce) return;
  if (!housesLoadingPromise) housesLoadingPromise = loadHouses();
  await housesLoadingPromise;
  housesLoadedOnce = true;
}

function selectHouse(houseId) {
  state.currentHouseId = houseId;
  localStorage.setItem(HOUSE_STORAGE_KEY, String(houseId));
  els.houseSelect.value = String(houseId);
  updateManageMembersButton();
  updateRenameHouseButton();
  // Switching houses mid-edit is only reachable programmatically (the
  // select is hidden while #rename-house-inline-form is open), but reset
  // defensively so a stale edit never lingers pointed at the wrong house.
  closeRenameHouseInline();
}

function updateManageMembersButton() {
  els.manageMembersButton.hidden = state.currentHouseId === null;
}

// Owner-only, same gate as the "remove member"/invite-form visibility in
// the Members modal — a plain member can view the house name but not
// rename it (see internal/handlers.authorizeHouseOwner).
function updateRenameHouseButton() {
  els.renameHouseInlineButton.hidden = currentHouseRole() !== 'owner';
}

els.houseSelect.addEventListener('change', async (event) => {
  const { value } = event.target;
  if (value === CREATE_HOUSE_OPTION_VALUE) {
    els.houseSelect.value = state.currentHouseId !== null ? String(state.currentHouseId) : CREATE_HOUSE_OPTION_VALUE;
    openNewHouseModal();
    return;
  }
  selectHouse(Number(value));
  await refreshVisibleView();
  // refreshNotifications is defined in notifications.js, resolved lazily
  // the same way refreshVisibleView's own cross-file calls already are.
  refreshNotifications();
});

function openNewHouseModal() {
  els.createHouseForm.reset();
  els.newHouseModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  els.houseNameInput.focus();
}

function closeNewHouseModal() {
  els.newHouseModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

els.closeHouseModalButton.addEventListener('click', closeNewHouseModal);
els.newHouseModal.addEventListener('click', (event) => {
  if (event.target === els.newHouseModal) closeNewHouseModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !els.newHouseModal.hidden) closeNewHouseModal();
});

els.createHouseForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const name = els.houseNameInput.value.trim();
  if (!name) return;

  try {
    const house = await apiRequest('/houses', { method: 'POST', body: JSON.stringify({ name }) });
    await loadHouses();
    selectHouse(house.id);
    closeNewHouseModal();
    await refreshVisibleView();
  } catch (err) {
    showError(err.message);
  }
});

// ---------------------------------------------------------------------------
// House members (roster + invite-by-email; remove is owner-only)
// ---------------------------------------------------------------------------

function currentHouseRole() {
  return state.houses.find((house) => house.id === state.currentHouseId)?.role ?? null;
}

function currentHouseName() {
  return state.houses.find((house) => house.id === state.currentHouseId)?.name ?? '';
}

// Inline rename of the current house, directly on the dashboard header —
// deliberately not tucked inside the Members modal (that was last
// session's first pass, moved out here for discoverability: renaming is a
// one-click action from the same row as the house selector, no modal to
// open first). #house-toolbar (the label + <select> + pencil + "Membres"
// row) and #rename-house-inline-form are mutually exclusive; toggling one
// hidden and the other visible swaps between browse and edit mode in
// place, the same "one real row, two states" pattern list_view.js's
// quick-add bar uses for its collapsed/expanded advanced panel.
function openRenameHouseInline() {
  if (state.currentHouseId === null) return;
  els.renameHouseInlineInput.value = currentHouseName();
  els.houseToolbar.hidden = true;
  els.renameHouseInlineForm.hidden = false;
  els.renameHouseInlineInput.focus();
  els.renameHouseInlineInput.select();
}

function closeRenameHouseInline() {
  els.renameHouseInlineForm.hidden = true;
  els.houseToolbar.hidden = false;
}

function isRenameHouseInlineOpen() {
  return !els.renameHouseInlineForm.hidden;
}

els.renameHouseInlineButton.addEventListener('click', openRenameHouseInline);
els.cancelRenameHouseInlineButton.addEventListener('click', closeRenameHouseInline);
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && isRenameHouseInlineOpen()) closeRenameHouseInline();
});

els.renameHouseInlineForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const name = els.renameHouseInlineInput.value.trim();
  if (!name || state.currentHouseId === null) return;

  let house;
  try {
    house = await apiRequest(`/houses/${state.currentHouseId}`, { method: 'PUT', body: JSON.stringify({ name }) });
  } catch (err) {
    showError(err.message);
    return;
  }

  // Patches state.houses in place (rather than a full loadHouses() round
  // trip) so the header selector's option label updates immediately.
  const stored = state.houses.find((h) => h.id === house.id);
  if (stored) stored.name = house.name;
  populateHouseSelect(state.houses);
  els.houseSelect.value = String(state.currentHouseId);

  closeRenameHouseInline();
  window.TrakkaToast?.success(t('dashboard.renameSuccess', { name: house.name }));
});

function buildMemberRow(member, isOwnerView) {
  const li = document.createElement('li');
  li.className = 'flex items-center justify-between gap-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/60 px-3 py-2';

  const info = document.createElement('div');
  const name = document.createElement('p');
  name.className = 'text-sm font-medium text-slate-900 dark:text-slate-100';
  name.textContent = member.display_name || member.email;
  const email = document.createElement('p');
  email.className = 'text-xs text-slate-500 dark:text-slate-400';
  email.textContent = member.role === 'owner' ? `${member.email} · propriétaire` : member.email;
  info.append(name, email);
  li.appendChild(info);

  if (isOwnerView && member.role !== 'owner') {
    const removeBtn = document.createElement('button');
    removeBtn.type = 'button';
    removeBtn.setAttribute('aria-label', t('common.removeMember', { email: member.email }));
    removeBtn.className = 'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400';
    removeBtn.innerHTML = TRASH_ICON_SVG;
    removeBtn.addEventListener('click', () => removeMember(member.user_id));
    li.appendChild(removeBtn);
  }

  return li;
}

async function loadMembers() {
  if (state.currentHouseId === null) return;
  let members;
  try {
    members = await apiRequest(`/houses/${state.currentHouseId}/members`);
  } catch (err) {
    // No offline mirror for members — a plain connectivity failure just
    // leaves the modal empty without a blocking banner (the header's
    // network badge already covers it); a genuine server-side error still
    // gets one.
    if (!isNetworkError(err)) showError(err.message);
    return;
  }

  const isOwnerView = currentHouseRole() === 'owner';
  els.membersList.replaceChildren();
  for (const member of members) {
    els.membersList.appendChild(buildMemberRow(member, isOwnerView));
  }
  els.inviteMemberForm.hidden = !isOwnerView;
}

function openMembersModal() {
  els.inviteMemberForm.reset();
  els.membersModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  loadMembers();
}

function closeMembersModal() {
  els.membersModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

els.manageMembersButton.addEventListener('click', openMembersModal);
els.closeMembersModalButton.addEventListener('click', closeMembersModal);
els.membersModal.addEventListener('click', (event) => {
  if (event.target === els.membersModal) closeMembersModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !els.membersModal.hidden) closeMembersModal();
});

els.inviteMemberForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const email = els.inviteEmailInput.value.trim();
  if (!email || state.currentHouseId === null) return;

  try {
    await apiRequest(`/houses/${state.currentHouseId}/members`, { method: 'POST', body: JSON.stringify({ email }) });
    els.inviteMemberForm.reset();
    await loadMembers();
  } catch (err) {
    showError(err.message);
  }
});

async function removeMember(userId) {
  hideError();
  try {
    await apiRequest(`/houses/${state.currentHouseId}/members/${userId}`, { method: 'DELETE' });
    await loadMembers();
  } catch (err) {
    showError(err.message);
  }
}

// ---------------------------------------------------------------------------
// Dashboard (lists grouped by type, with per-card badges)
// ---------------------------------------------------------------------------

function typeLabel(type) {
  return type === 'todo' ? 'tâches' : 'courses';
}

function badge(text, palette) {
  const colors = {
    sky: 'bg-sky-500/10 text-sky-600 dark:text-sky-300',
    slate: 'bg-slate-200/60 dark:bg-slate-700/50 text-slate-600 dark:text-slate-300',
    emerald: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-300',
    violet: 'bg-violet-500/10 text-violet-600 dark:text-violet-300',
    orange: 'bg-orange-500/10 text-orange-600 dark:text-orange-300',
  };
  const span = document.createElement('span');
  span.className = `inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${colors[palette] || colors.slate}`;
  span.textContent = text;
  return span;
}

// One badge per list `type`, always shown on a card regardless of which
// dashboard section it lands in — the "Achats & Sourcing" grid already
// mixes 'groceries'/'shopping'/'recurring_shopping' together under one
// heading (see isPurchaseList below), and the Espaces tab (spaces.js) mixes
// every type in a single space (e.g. a "Homelab" space holding a shopping
// list, a todo list and a notes list side by side), so this is the one
// place that tells them apart at a glance without opening the card. Labels
// are dedicated, short `dashboard.listType*` i18n keys rather than the
// "new list" modal's own (longer, more descriptive) type-picker strings —
// the modal's radio grid has room for "Abonnements & Récurrences", a small
// pill on a card doesn't. Icons/colors are still kept in sync with that
// modal's own type-picker icons (static/index.html's #create-list-form) so
// the same type never shows two different icons in two places.
const LIST_TYPE_BADGE_META = {
  shopping: { icon: '🛒', key: 'dashboard.listTypeShopping', palette: 'sky' },
  recurring_shopping: { icon: '🔄', key: 'dashboard.listTypeRecurring', palette: 'violet' },
  groceries: { icon: '🛍️', key: 'dashboard.listTypeGroceries', palette: 'emerald' },
  todo: { icon: '✅', key: 'dashboard.listTypeTodo', palette: 'orange' },
  custom: { icon: '📝', key: 'dashboard.listTypeCustom', palette: 'slate' },
};

function typeBadge(type) {
  const meta = LIST_TYPE_BADGE_META[type];
  return meta ? badge(`${meta.icon} ${t(meta.key)}`, meta.palette) : null;
}

// A list's own `icon` (set via the create/edit list modal) takes priority
// over its type's fixed default — see LIST_TYPE_BADGE_META above — so a
// list only falls back to a generic type icon until the user picks its own.
function listIcon(list) {
  return list.icon || LIST_TYPE_BADGE_META[list.type]?.icon || '📋';
}

// Shopping cards show which sites their items link to (up to 3 domains,
// deduplicated, with a "+N" overflow badge) plus a running estimated total
// (the sum of every priced item's price * quantity — see lineTotal in
// list_view.js, the same per-unit-price convention this mirrors) — a quick
// sourcing + budget overview without opening the list.
function urlBadges(items) {
  const domains = [];
  let total = 0;
  let hasPrice = false;
  for (const item of items) {
    if (item.url) {
      try {
        domains.push(new URL(item.url).hostname.replace(/^www\./, ''));
      } catch {
        // malformed URL slipped through some other path; skip it silently
      }
    }
    if (typeof item.price === 'number') {
      hasPrice = true;
      total += item.price * (item.quantity > 0 ? item.quantity : 1);
    }
  }
  const unique = [...new Set(domains)];
  const frag = document.createDocumentFragment();
  for (const domain of unique.slice(0, 3)) {
    frag.appendChild(badge(domain, 'sky'));
  }
  if (unique.length > 3) {
    frag.appendChild(badge(`+${unique.length - 3}`, 'slate'));
  }
  // formatEuro is defined in list_view.js, loaded after this file — safe to
  // call here since this only ever runs at render time, well after every
  // script has finished loading (same deferred-call pattern buildItemRow
  // already uses for monthLabel/buildRecurrenceBadge from planning.js).
  if (hasPrice) {
    frag.appendChild(badge(formatEuro(total), 'emerald'));
  }
  return frag;
}

function progressBadge(items) {
  const frag = document.createDocumentFragment();
  if (items.length === 0) return frag;
  const done = items.filter((item) => item.done).length;
  frag.appendChild(badge(`${done}/${items.length} terminées`, done === items.length ? 'emerald' : 'slate'));
  return frag;
}

// custom (freeform notes) lists have no url or completion concept — see
// FIELD_VISIBILITY_BY_TYPE in list_view.js — so unlike urlBadges/
// progressBadge there is nothing meaningful to summarize beyond the item
// count buildListCard already shows; this only exists so renderGrid can be
// called uniformly with a badge function for every dashboard section.
function noBadges() {
  return document.createDocumentFragment();
}

function emptyState(message) {
  const li = document.createElement('li');
  li.className = 'col-span-full rounded-xl border border-dashed border-slate-200 dark:border-slate-800 p-6 text-center text-sm text-slate-500';
  li.textContent = message;
  return li;
}

function buildListCard(list, badgesFragment) {
  const li = document.createElement('li');
  li.className = 'rounded-2xl border border-slate-200 dark:border-slate-800 bg-slate-100/70 dark:bg-slate-800/40 shadow-sm transition hover:border-slate-300 dark:hover:border-slate-700 hover:bg-slate-200 dark:hover:bg-slate-800/70';

  const row = document.createElement('div');
  row.className = 'flex items-start justify-between gap-2 p-4';

  const openBtn = document.createElement('button');
  openBtn.type = 'button';
  openBtn.className = 'min-h-[44px] flex-1 text-left';
  openBtn.addEventListener('click', () => selectList(list.id));

  // Shown above the name on every card, in every dashboard section, since
  // even the "Achats & Sourcing" grid mixes several types together (see
  // isPurchaseList) and the Espaces tab mixes all of them — see
  // LIST_TYPE_BADGE_META's comment above for why this can't just live
  // inside badgesRow below.
  const typeRow = document.createElement('div');
  typeRow.className = 'mb-2 flex flex-wrap items-center gap-1.5 empty:hidden';
  const typeBadgeEl = typeBadge(list.type);
  if (typeBadgeEl) typeRow.appendChild(typeBadgeEl);
  // list.access_source is only ever set by db.ListSharedListsForUser (see
  // shares.js's "Partagé avec moi" tab) — the 👥 indicator CLAUDE.md's
  // sharing feature asks for, so a list someone else shared with you is
  // recognizable at a glance among your own.
  if (list.access_source) {
    typeRow.appendChild(badge(`👥 ${t('shares.sharedBadge')}`, 'violet'));
  }

  const titleRow = document.createElement('div');
  titleRow.className = 'flex min-w-0 items-center gap-2';

  const iconSpan = document.createElement('span');
  iconSpan.setAttribute('aria-hidden', 'true');
  iconSpan.className = 'shrink-0 text-lg leading-none';
  iconSpan.textContent = listIcon(list);

  const title = document.createElement('h3');
  title.className = 'truncate text-base font-semibold text-slate-900 dark:text-slate-100';
  title.textContent = list.name;

  titleRow.append(iconSpan, title);

  const count = document.createElement('p');
  count.className = 'mt-1 text-sm text-slate-500 dark:text-slate-400';
  const n = (list.items || []).length;
  count.textContent = `${n} élément${n === 1 ? '' : 's'}`;

  const badgesRow = document.createElement('div');
  badgesRow.className = 'mt-3 flex flex-wrap gap-1.5 empty:hidden';
  badgesRow.appendChild(badgesFragment);

  openBtn.append(typeRow, titleRow, count, badgesRow);

  const actions = document.createElement('div');
  actions.className = 'flex shrink-0 items-center gap-1';

  // A card reached via db.ListSharedListsForUser (list.access_source set —
  // see shares.js's "Partagé avec moi" tab) shows no edit/share/delete
  // controls at all: managing, editing or deleting a list requires actual
  // House membership (see internal/handlers/shares.go's
  // handleListShareCreate and handleListsDelete), which by definition this
  // list showing up there means the viewer doesn't have. openShareModal is
  // defined in shares.js, resolved lazily the same way openListModal above
  // already is.
  if (!list.access_source) {
    const editBtn = document.createElement('button');
    editBtn.type = 'button';
    editBtn.setAttribute('aria-label', t('common.editList', { name: list.name }));
    editBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-700 hover:text-slate-900 dark:hover:text-slate-200';
    editBtn.innerHTML = PENCIL_ICON_SVG;
    editBtn.addEventListener('click', () => openListModal(list));
    actions.appendChild(editBtn);

    const shareBtn = document.createElement('button');
    shareBtn.type = 'button';
    shareBtn.setAttribute('aria-label', t('common.shareList', { name: list.name }));
    shareBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-violet-500/10 hover:text-violet-600 dark:hover:text-violet-400';
    shareBtn.innerHTML = SHARE_ICON_SVG;
    shareBtn.addEventListener('click', () => openShareModal({ kind: 'list', id: list.id, name: list.name }));
    actions.appendChild(shareBtn);

    const deleteBtn = document.createElement('button');
    deleteBtn.type = 'button';
    deleteBtn.setAttribute('aria-label', t('common.deleteList', { name: list.name }));
    deleteBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400';
    deleteBtn.innerHTML = TRASH_ICON_SVG;
    deleteBtn.addEventListener('click', () => removeList(list));
    actions.appendChild(deleteBtn);
  } else if (list.access_source === 'list_share') {
    // Pinning is only offered for a list shared *directly* (a list_shares
    // row the recipient themselves holds) — one reached only via a shared
    // Space has no such row for PATCH /api/v1/lists/{id}/share/pin to flip,
    // see handleListSharePin/SetListSharePinned. This is the recipient's
    // own action (CLAUDE.md's "Pinning shared lists" feature): pinning
    // makes the card also show up on their own dashboard grids, alongside
    // their House's own lists, without needing House membership.
    const pinBtn = document.createElement('button');
    pinBtn.type = 'button';
    const pinned = !!list.is_pinned_to_dashboard;
    pinBtn.setAttribute('aria-label', t(pinned ? 'common.unpinList' : 'common.pinList', { name: list.name }));
    pinBtn.className =
      'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg hover:bg-amber-500/10 hover:text-amber-600 dark:hover:text-amber-400 ' +
      (pinned ? 'text-amber-500 dark:text-amber-400' : 'text-slate-500');
    pinBtn.innerHTML = PIN_ICON_SVG;
    pinBtn.addEventListener('click', () => toggleListPin(list));
    actions.appendChild(pinBtn);
  }

  row.append(openBtn, actions);
  li.appendChild(row);
  return li;
}

function renderGrid(container, lists, badgeFn, emptyMessage) {
  container.replaceChildren();
  if (lists.length === 0) {
    container.appendChild(emptyState(emptyMessage));
    return;
  }
  for (const list of lists) {
    container.appendChild(buildListCard(list, badgeFn(list.items || [])));
  }
}

// Reads every list belonging to `houseId` from the IndexedDB mirror, each
// merged with its own cached items (db.js's getListWithItems) so the result
// is shaped exactly like a batch of successful `GET /api/v1/lists/{id}`
// responses. Used both for the instant paint on load and as loadDashboard's
// fallback when the network is unreachable.
async function cachedDashboardLists(houseId) {
  if (!window.TrakkaDB) return [];
  try {
    const lists = await window.TrakkaDB.getListsByHouse(houseId);
    const detailed = await Promise.all(lists.map((list) => window.TrakkaDB.getListWithItems(list.id)));
    return detailed.filter(Boolean);
  } catch {
    return [];
  }
}

// A list is purchase-oriented (lands in the "Achats & Sourcing" grid) when
// it's neither a todo list nor a custom/freeform one — 'shopping',
// 'groceries' and 'recurring_shopping' all qualify (see
// models.ValidListTypes). 'custom' gets its own dedicated "Notes & Listes
// Libres" grid instead (see renderDashboardGrids below) — a freeform note/
// idea list has nothing to do with purchasing or tasks, so it must never be
// folded into either existing section, totals, or filter.
function isPurchaseList(type) {
  return type !== 'todo' && type !== 'custom';
}

// Splits `detailed` (a house's lists, each with its items already attached)
// into the dashboard's three mutually exclusive grids and renders all of
// them — shared by the cache-only offline path and the normal network path
// below so the three-way split can't drift between them.
function renderDashboardGrids(detailed) {
  renderGrid(els.shoppingLists, detailed.filter((l) => isPurchaseList(l.type)), urlBadges, 'Aucune liste de courses pour le moment.');
  renderGrid(els.todoLists, detailed.filter((l) => l.type === 'todo'), progressBadge, 'Aucun espace tâches pour le moment.');
  renderGrid(els.customLists, detailed.filter((l) => l.type === 'custom'), noBadges, 'Aucune liste libre pour le moment.');
}

// Renders the dashboard purely from the local IndexedDB mirror, with no
// network request involved — this is what keeps lists/items on screen while
// offline instead of the grid going blank. Lists mid-undo-grace-period (see
// removeList) are filtered out here too, same as the network path below.
async function renderDashboardFromCache() {
  if (state.currentHouseId === null) {
    renderDashboardGrids([]);
    return;
  }

  const detailed = (await cachedDashboardLists(state.currentHouseId)).filter(
    (list) => !pendingDeletedListIds.has(list.id)
  );
  renderDashboardGrids(detailed);
}

async function loadDashboard() {
  // See ensureHousesLoaded's own comment above: this guarantees
  // state.currentHouseId is never used for a network request until it's
  // been validated against a live GET /api/v1/houses, no matter which of
  // the several independent triggers called loadDashboard first.
  await ensureHousesLoaded();
  if (state.currentHouseId === null) {
    renderDashboardGrids([]);
    return;
  }

  let lists;
  try {
    lists = await apiRequest(`/lists?house_id=${state.currentHouseId}`);
  } catch (err) {
    // Offline, or a transient server error: keep the dashboard populated
    // from the local mirror rather than leaving it blank — see the
    // Offline-First requirement in CLAUDE.md/docs/PWA.md. A plain
    // connectivity failure stays silent (the header's discreet network
    // badge already says "Hors-ligne"); only a genuine server-side error
    // still raises the banner.
    await renderDashboardFromCache();
    if (!isNetworkError(err)) showError(err.message);
    return;
  }

  // Lists mid-undo-grace-period (see removeList) haven't actually been
  // deleted server-side yet, so a plain refetch would still include them —
  // filter them back out here rather than only hiding their card once.
  lists = lists.filter((list) => !pendingDeletedListIds.has(list.id));

  // Fetch each list's detail (items included) in parallel to compute
  // badges. Best effort per list: one failing shouldn't hide the rest —
  // and falls back to that single list's cached detail before giving up
  // and showing it with no items, so a partial network failure doesn't
  // erase items that are still sitting in the local mirror.
  const detailed = await Promise.all(
    lists.map((list) =>
      apiRequest(`/lists/${list.id}`).catch(async () => {
        const cached = window.TrakkaDB ? await window.TrakkaDB.getListWithItems(list.id).catch(() => null) : null;
        return cached || { ...list, items: [] };
      })
    )
  );

  // Alongside the current House's own lists, the dashboard also shows any
  // list shared directly with the caller that they've chosen to pin (see
  // buildListCard's 📌 button, toggleListPin, and loadPinnedSharedLists in
  // shares.js) — CLAUDE.md's "Pinning shared lists" feature. No offline
  // mirror for these (same "requires connectivity" scoping as the rest of
  // the sharing feature), so they simply don't appear while offline; a
  // best-effort failure here must never block the rest of the dashboard
  // from rendering.
  const pinnedShared = await loadPinnedSharedLists().catch(() => []);

  renderDashboardGrids([...detailed, ...pinnedShared]);
}

// ---------------------------------------------------------------------------
// View switching (list detail rendering itself lives in list_view.js)
// ---------------------------------------------------------------------------

function refreshVisibleView() {
  if (state.currentListId !== null) {
    refreshCurrentList();
  } else if (isPlanningTabActive()) {
    // isPlanningTabActive/refreshPlanningIfActive are defined in
    // planning.js, resolved lazily here the same way showDashboard is
    // below — see that comment for why cross-file calls like this are safe.
    refreshPlanningIfActive();
  } else if (isUrgentTabActive()) {
    // isUrgentTabActive/refreshUrgentIfActive are defined in urgent.js,
    // resolved lazily the same way.
    refreshUrgentIfActive();
  } else if (isSpacesTabActive()) {
    // isSpacesTabActive/refreshSpacesIfActive are defined in spaces.js,
    // resolved lazily the same way.
    refreshSpacesIfActive();
  } else if (isSharedTabActive()) {
    // isSharedTabActive/refreshSharedIfActive are defined in shares.js,
    // resolved lazily the same way.
    refreshSharedIfActive();
  } else {
    loadDashboard();
  }
}

// The logo keeps its plain `href="/"` (full reload, works with JS disabled
// or a middle-click/new-tab), but a same-tab left-click intercepts it to
// hop back to the dashboard in place — showDashboard is defined in
// list_view.js, resolved lazily here the same way selectList is above.
els.logoLink.addEventListener('click', (event) => {
  event.preventDefault();
  hideError();
  showDashboard();
});

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

// Deletion is deferred behind a 5s undo grace period rather than sent
// immediately: the list disappears from the dashboard right away (via
// pendingDeletedListIds, since there's no per-list DOM node kept around to
// just re-show — loadDashboard always rebuilds the grid from a fresh
// fetch), and the actual DELETE only fires from TrakkaUndo's onCommit if
// the countdown runs out without the user clicking "Annuler". Because nothing
// hits the network until then, this needs no special handling for the
// offline queue in sw.js — the eventual apiRequest call is indistinguishable
// from one made right away, whether it goes out online or offline.
function removeList(list) {
  hideError();
  pendingDeletedListIds.add(list.id);
  loadDashboard();

  TrakkaUndo.schedule({
    message: t('undo.listDeleted', { name: list.name }),
    undoLabel: t('undo.cancel'),
    onUndo: () => {
      pendingDeletedListIds.delete(list.id);
      loadDashboard();
    },
    onCommit: async () => {
      try {
        await apiRequest(`/lists/${list.id}`, { method: 'DELETE' });
      } catch (err) {
        showError(err.message);
      }
      pendingDeletedListIds.delete(list.id);
      await loadDashboard();
      await refreshPendingBadge();
    },
  });
}

// Pins or unpins a directly-shared list on the caller's own dashboard (see
// buildListCard's pin button above and PATCH /api/v1/lists/{id}/share/pin).
// Unlike removeList this isn't optimistic/undo-able — it's a quick,
// infrequent toggle, so it follows shares.js's simpler
// await-then-refresh pattern (see revokeShare) rather than the
// coalesced-optimistic pattern item quantity/urgent toggles use for
// rapid-fire clicks. refreshVisibleView() (defined above) re-renders
// whichever of the dashboard grids/"Partagé avec moi" tab is currently on
// screen, since pinning can change what either one shows.
async function toggleListPin(list) {
  hideError();
  try {
    await apiRequest(`/lists/${list.id}/share/pin`, {
      method: 'PATCH',
      body: JSON.stringify({ pinned: !list.is_pinned_to_dashboard }),
    });
    await refreshVisibleView();
  } catch (err) {
    showError(err.message);
  }
}

// ---------------------------------------------------------------------------
// "New/edit list" modal — one shared modal for both, mirroring
// spaces.js's openCategoryModal (editingList === null means "create").
// ---------------------------------------------------------------------------

// The list currently open in the modal for editing, or null when the modal
// is in "create" mode — set by openListModal, read by createListForm's
// submit handler.
let editingList = null;

function setListTypeSelection(value) {
  for (const label of els.typeOptions) {
    const input = label.querySelector('input');
    const active = input.value === value;
    input.checked = active;
    label.classList.toggle('border-sky-500', active);
    label.classList.toggle('bg-sky-500/10', active);
    label.classList.toggle('text-sky-600', active);
    label.classList.toggle('dark:text-sky-300', active);
    label.classList.toggle('border-slate-200', !active);
    label.classList.toggle('dark:border-slate-700', !active);
    label.classList.toggle('bg-white', !active);
    label.classList.toggle('dark:bg-slate-900', !active);
    label.classList.toggle('text-slate-600', !active);
    label.classList.toggle('dark:text-slate-300', !active);
  }
}

// Opens the modal in create mode (list === null) or edit mode (prefilled
// from an existing list, name/icon/type/custom_category_id all editable —
// house_id stays fixed, matching PUT /api/v1/lists/{id} not accepting it).
function openListModal(list) {
  editingList = list || null;

  els.createListForm.reset();
  els.listNameInput.value = list?.name || '';
  els.listIconInput.value = list?.icon || '';
  setListTypeSelection(list?.type || 'shopping');
  els.newListModalTitle.textContent = t(editingList ? 'modals.newList.titleEdit' : 'modals.newList.titleCreate');
  els.listSubmitButton.textContent = t(editingList ? 'modals.newList.submitEdit' : 'modals.newList.submitCreate');
  // populateCategorySelect/loadCustomCategories are defined in spaces.js,
  // resolved lazily the same way every other cross-file call in this file
  // already is — refetching here (rather than trusting whatever spaces.js
  // last cached) keeps the picker correct even if a category was created/
  // deleted in another tab since the last time the Spaces tab was opened.
  loadCustomCategories().then(() => populateCategorySelect(els.listCategorySelect, list?.custom_category_id ?? null));
  els.newListModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  els.listNameInput.focus();
}

function closeNewListModal() {
  els.newListModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
  editingList = null;
  els.newListButton.focus();
}

for (const label of els.typeOptions) {
  label.querySelector('input').addEventListener('change', (event) => setListTypeSelection(event.target.value));
}

els.newListButton.addEventListener('click', () => openListModal(null));
els.closeModalButton.addEventListener('click', closeNewListModal);
els.newListModal.addEventListener('click', (event) => {
  if (event.target === els.newListModal) closeNewListModal();
});
document.addEventListener('keydown', (event) => {
  // The category modal can open on top of this one (see the change listener
  // below) — when it's the one currently visible, let its own Escape
  // handler in spaces.js close just that one instead of both at once.
  if (event.key === 'Escape' && !els.newListModal.hidden && spacesEls.categoryModal.hidden) closeNewListModal();
});

for (const button of els.listIconPresetButtons) {
  button.addEventListener('click', () => {
    els.listIconInput.value = button.dataset.listIconPreset;
  });
}

// CREATE_CATEGORY_OPTION_VALUE is defined in spaces.js (the module that owns
// custom categories) — selecting it here is a shortcut into the "new space"
// modal without leaving the list-creation flow, the same sentinel-option
// pattern els.houseSelect already uses for CREATE_HOUSE_OPTION_VALUE.
els.listCategorySelect.addEventListener('change', (event) => {
  if (event.target.value !== CREATE_CATEGORY_OPTION_VALUE) return;
  event.target.value = '';
  openCategoryModal(null, { onCreated: (category) => populateCategorySelect(els.listCategorySelect, category.id) });
});

els.createListForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const name = els.listNameInput.value.trim();
  const icon = els.listIconInput.value.trim();
  const type = els.newListModal.querySelector('input[name="list-type"]:checked')?.value || 'shopping';
  const categoryValue = els.listCategorySelect.value;
  const customCategoryId = categoryValue && categoryValue !== CREATE_CATEGORY_OPTION_VALUE ? Number(categoryValue) : null;
  if (!name) return;

  try {
    const isEdit = editingList !== null;
    if (isEdit) {
      await apiRequest(`/lists/${editingList.id}`, {
        method: 'PUT',
        body: JSON.stringify({ name, type, icon, custom_category_id: customCategoryId }),
      });
    } else {
      if (state.currentHouseId === null) return;
      await apiRequest('/lists', {
        method: 'POST',
        body: JSON.stringify({ name, type, icon, house_id: state.currentHouseId, custom_category_id: customCategoryId }),
      });
    }
    closeNewListModal();
    await loadDashboard();
    await refreshSpacesIfActive();
    // No-op if the currently open list detail view isn't the one just
    // edited (refreshCurrentList is a no-op when state.currentListId is
    // null) — see list_view.js.
    await refreshCurrentList();
  } catch (err) {
    showError(err.message);
  }
  await refreshPendingBadge();
});

// ---------------------------------------------------------------------------
// Service worker registration
// ---------------------------------------------------------------------------

// Registers sw.js (app-shell caching + offline write queue) and wires the
// two ways a flush can be triggered on browsers without Background Sync
// (i.e. all of iOS/iPadOS Safari): the page's own 'online' event, and a
// notification once the service worker has actually flushed the queue.
function registerServiceWorker() {
  if (!('serviceWorker' in navigator)) return;

  navigator.serviceWorker.register('/sw.js')
    .then((registration) => watchForServiceWorkerUpdate(registration))
    .catch((err) => {
      console.error('Échec de l’enregistrement du service worker :', err);
    });

  navigator.serviceWorker.addEventListener('message', (event) => {
    if (event.data && event.data.type === 'trakka-sync-complete') {
      refreshVisibleView();
      updateNetworkStatus();
      refreshNotifications();
    }
  });
}

// Detects a deployed frontend update and surfaces it as a discreet,
// dismissible-by-inaction banner rather than reloading on the user's
// behalf: sw.js's install handler already calls self.skipWaiting() and its
// activate handler calls self.clients.claim(), so the new service worker
// (and its bumped cache versions) takes over almost immediately regardless
// — but the *page's own already-loaded JS* stays the old version until an
// actual reload happens, and forcing that reload automatically could wipe
// out whatever the user is mid-typing in a form. Reaching 'installed'
// while navigator.serviceWorker.controller is already set is the standard
// signal that this is a genuine update (a new worker replacing one already
// controlling the page), not the very first install on a fresh visit,
// which has no controller yet and needs no banner.
function watchForServiceWorkerUpdate(registration) {
  if (!registration) return;

  registration.addEventListener('updatefound', () => {
    const installingWorker = registration.installing;
    if (!installingWorker) return;

    installingWorker.addEventListener('statechange', () => {
      if (installingWorker.state === 'installed' && navigator.serviceWorker.controller) {
        showUpdateBanner();
      }
    });
  });
}

function showUpdateBanner() {
  els.updateBanner.hidden = false;
}

els.updateReloadButton?.addEventListener('click', () => {
  window.location.reload();
});

window.addEventListener('online', () => {
  updateNetworkStatus();
  navigator.serviceWorker.controller?.postMessage({ type: 'flush-queue' });
});
window.addEventListener('offline', updateNetworkStatus);
document.addEventListener('visibilitychange', () => {
  if (!document.hidden) {
    updateNetworkStatus();
    // Picks up whatever the periodic backend price-drop scan found while
    // this tab was in the background, without requiring a full reload.
    refreshNotifications();
  }
});

// Paints the dashboard from whatever's already in IndexedDB before any
// network request is made — an empty mirror (brand new browser profile)
// just renders the normal empty state, so this is always safe to call.
// This is the "instant paint" half of the stale-while-revalidate load: the
// network refresh in init() below repaints over this once it resolves.
async function hydrateFromCache() {
  const houses = await cachedHouses();
  state.houses = houses;
  populateHouseSelect(houses);

  const stored = Number(localStorage.getItem(HOUSE_STORAGE_KEY));
  const storedIsValid = houses.some((house) => house.id === stored);
  state.currentHouseId = storedIsValid ? stored : (houses[0]?.id ?? null);
  els.houseSelect.value = state.currentHouseId !== null ? String(state.currentHouseId) : CREATE_HOUSE_OPTION_VALUE;
  updateManageMembersButton();
  updateRenameHouseButton();

  await renderDashboardFromCache();

  // Also warm the custom-categories ("Espaces") mirror here, not just lists/
  // items, so the "Espaces" tab's highlight dot and the "new list" modal's
  // category picker are already correct the instant a reload finishes,
  // rather than waiting on loadCustomCategories()'s own network-first call
  // further down in init() to fail over to the cache. customCategories/
  // updateSpacesTabBadge are defined in spaces.js, loaded after this file —
  // safe to reference here despite that <script> load order because this
  // only runs after hydrateFromCache has already crossed a real IndexedDB
  // await (cachedHouses() above), by which point every script tag on the
  // page has long finished executing and defined its top-level functions —
  // the same timing guarantee init()'s own later call to
  // loadCustomCategories() already relies on.
  if (window.TrakkaDB) {
    try {
      customCategories = await window.TrakkaDB.getCustomCategories();
    } catch {
      customCategories = [];
    }
    updateSpacesTabBadge();
  }
}

async function init() {
  // Offline-first hydration: paint immediately from the local mirror so a
  // reload while offline (or on a slow connection) never shows a blank
  // dashboard while the requests below are still in flight.
  await hydrateFromCache();

  try {
    // A real 401 here redirects to /auth/login via apiRequest's 401
    // handling and never resolves, so nothing below runs for an
    // unauthenticated visitor. This only catches "couldn't reach the
    // server at all" (offline, or the service worker's offline fallback,
    // which doesn't special-case /me and answers 503) — expected while
    // offline, so it just keeps whatever hydrateFromCache already painted
    // instead of aborting startup.
    state.currentUser = await apiRequest('/me');
    setKeepLastPagePreference(state.currentUser.keep_last_page);
  } catch (err) {
    console.warn('Session non vérifiée (probablement hors ligne) :', err);
  }
  // refreshAdminButtonVisibility is defined in admin.js, resolved lazily
  // the same way every other cross-file call in this function already is.
  refreshAdminButtonVisibility();

  // Stale-while-revalidate: now refresh from the network. Both calls fall
  // back to the cache again on their own if this fails, so it's safe to
  // run unconditionally regardless of whether the /me check above worked.
  await ensureHousesLoaded();
  await loadDashboard();
  // loadNotifications is defined in notifications.js, resolved lazily the
  // same way every other cross-file call in this function already is.
  await loadNotifications();
  // loadCustomCategories is defined in spaces.js — fetches the user's custom
  // categories once at startup (and updates the "Espaces" tab's highlight
  // dot as a side effect) so the tab reflects reality immediately, without
  // waiting for it to be opened.
  await loadCustomCategories();
  // Reopens the last-visited tab/list, if the preference is on and one was
  // saved — must run last, once everything it might need (the dashboard,
  // the current house, custom categories for the "Espaces" tab) is ready.
  await restoreLastView();
}

// Re-render everything that embeds translated strings inside JS-generated
// markup (data-i18n only covers static HTML, which i18n.js already
// re-applies on its own after a language switch).
document.addEventListener('trakka:lang-changed', () => {
  populateHouseSelect(state.houses);
  els.houseSelect.value = state.currentHouseId !== null ? String(state.currentHouseId) : CREATE_HOUSE_OPTION_VALUE;
  refreshVisibleView();
  updateNetworkStatus();
  refreshNotifications();
});

init();
updateNetworkStatus();
registerServiceWorker();
