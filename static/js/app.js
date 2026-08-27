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
  logoLink: document.getElementById('logo-link'),
  listsSection: document.getElementById('lists-section'),
  houseSelect: document.getElementById('house-select'),
  shoppingLists: document.getElementById('shopping-lists'),
  todoLists: document.getElementById('todo-lists'),
  newListButton: document.getElementById('new-list-button'),
  newListModal: document.getElementById('new-list-modal'),
  closeModalButton: document.getElementById('close-modal-button'),
  createListForm: document.getElementById('create-list-form'),
  listNameInput: document.getElementById('list-name'),
  typeOptions: document.querySelectorAll('#create-list-form [data-type-option]'),
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
  let response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      headers: { 'Content-Type': 'application/json' },
      ...options,
    });
  } catch {
    throw new Error('Impossible de contacter le serveur. Vérifiez votre connexion.');
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
    // CLAUDE.md/docs/PWA.md. Only surface the error banner if the cache
    // has nothing either, since the header's network badge already
    // communicates "offline" on its own in the common case.
    houses = await cachedHouses();
    if (houses.length === 0) showError(err.message);
  }

  state.houses = houses;
  populateHouseSelect(houses);

  const stored = Number(localStorage.getItem(HOUSE_STORAGE_KEY));
  const storedIsValid = houses.some((house) => house.id === stored);
  state.currentHouseId = storedIsValid ? stored : (houses[0]?.id ?? null);

  els.houseSelect.value = state.currentHouseId !== null ? String(state.currentHouseId) : CREATE_HOUSE_OPTION_VALUE;
  updateManageMembersButton();
}

function selectHouse(houseId) {
  state.currentHouseId = houseId;
  localStorage.setItem(HOUSE_STORAGE_KEY, String(houseId));
  els.houseSelect.value = String(houseId);
  updateManageMembersButton();
}

function updateManageMembersButton() {
  els.manageMembersButton.hidden = state.currentHouseId === null;
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

function buildMemberRow(member, isOwnerView) {
  const li = document.createElement('li');
  li.className = 'flex items-center justify-between gap-2 rounded-xl border border-slate-700 bg-slate-900/60 px-3 py-2';

  const info = document.createElement('div');
  const name = document.createElement('p');
  name.className = 'text-sm font-medium text-slate-100';
  name.textContent = member.display_name || member.email;
  const email = document.createElement('p');
  email.className = 'text-xs text-slate-400';
  email.textContent = member.role === 'owner' ? `${member.email} · propriétaire` : member.email;
  info.append(name, email);
  li.appendChild(info);

  if (isOwnerView && member.role !== 'owner') {
    const removeBtn = document.createElement('button');
    removeBtn.type = 'button';
    removeBtn.setAttribute('aria-label', t('common.removeMember', { email: member.email }));
    removeBtn.className = 'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-400';
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
    showError(err.message);
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
    sky: 'bg-sky-500/10 text-sky-300',
    slate: 'bg-slate-700/50 text-slate-300',
    emerald: 'bg-emerald-500/10 text-emerald-300',
  };
  const span = document.createElement('span');
  span.className = `inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${colors[palette] || colors.slate}`;
  span.textContent = text;
  return span;
}

// Shopping cards show which sites their items link to (up to 3 domains,
// deduplicated, with a "+N" overflow badge) — a quick sourcing overview.
function urlBadges(items) {
  const domains = [];
  for (const item of items) {
    if (!item.url) continue;
    try {
      domains.push(new URL(item.url).hostname.replace(/^www\./, ''));
    } catch {
      // malformed URL slipped through some other path; skip it silently
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
  return frag;
}

function progressBadge(items) {
  const frag = document.createDocumentFragment();
  if (items.length === 0) return frag;
  const done = items.filter((item) => item.done).length;
  frag.appendChild(badge(`${done}/${items.length} terminées`, done === items.length ? 'emerald' : 'slate'));
  return frag;
}

function emptyState(message) {
  const li = document.createElement('li');
  li.className = 'col-span-full rounded-xl border border-dashed border-slate-800 p-6 text-center text-sm text-slate-500';
  li.textContent = message;
  return li;
}

function buildListCard(list, badgesFragment) {
  const li = document.createElement('li');
  li.className = 'rounded-2xl border border-slate-800 bg-slate-800/40 shadow-sm transition hover:border-slate-700 hover:bg-slate-800/70';

  const row = document.createElement('div');
  row.className = 'flex items-start justify-between gap-2 p-4';

  const openBtn = document.createElement('button');
  openBtn.type = 'button';
  openBtn.className = 'min-h-[44px] flex-1 text-left';
  openBtn.addEventListener('click', () => selectList(list.id));

  const title = document.createElement('h3');
  title.className = 'text-base font-semibold text-slate-100';
  title.textContent = list.name;

  const count = document.createElement('p');
  count.className = 'mt-1 text-sm text-slate-400';
  const n = (list.items || []).length;
  count.textContent = `${n} élément${n === 1 ? '' : 's'}`;

  const badgesRow = document.createElement('div');
  badgesRow.className = 'mt-3 flex flex-wrap gap-1.5 empty:hidden';
  badgesRow.appendChild(badgesFragment);

  openBtn.append(title, count, badgesRow);

  const deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.setAttribute('aria-label', t('common.deleteList', { name: list.name }));
  deleteBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-400';
  deleteBtn.innerHTML = TRASH_ICON_SVG;
  deleteBtn.addEventListener('click', () => removeList(list));

  row.append(openBtn, deleteBtn);
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

// Renders the dashboard purely from the local IndexedDB mirror, with no
// network request involved — this is what keeps lists/items on screen while
// offline instead of the grid going blank. Lists mid-undo-grace-period (see
// removeList) are filtered out here too, same as the network path below.
async function renderDashboardFromCache() {
  if (state.currentHouseId === null) {
    renderGrid(els.shoppingLists, [], urlBadges, 'Créez une Maison pour commencer.');
    renderGrid(els.todoLists, [], progressBadge, 'Créez une Maison pour commencer.');
    return;
  }

  const detailed = (await cachedDashboardLists(state.currentHouseId)).filter(
    (list) => !pendingDeletedListIds.has(list.id)
  );

  renderGrid(els.shoppingLists, detailed.filter((l) => l.type === 'shopping'), urlBadges, 'Aucune liste de courses pour le moment.');
  renderGrid(els.todoLists, detailed.filter((l) => l.type === 'todo'), progressBadge, 'Aucun espace tâches pour le moment.');
}

async function loadDashboard() {
  if (state.currentHouseId === null) {
    renderGrid(els.shoppingLists, [], urlBadges, 'Créez une Maison pour commencer.');
    renderGrid(els.todoLists, [], progressBadge, 'Créez une Maison pour commencer.');
    return;
  }

  let lists;
  try {
    lists = await apiRequest(`/lists?house_id=${state.currentHouseId}`);
  } catch (err) {
    // Offline, or a transient server error: keep the dashboard populated
    // from the local mirror rather than leaving it blank — see the
    // Offline-First requirement in CLAUDE.md/docs/PWA.md. The banner still
    // surfaces the underlying error since it doesn't hide the grid, only
    // supplements the header's discreet network badge.
    await renderDashboardFromCache();
    showError(err.message);
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

  renderGrid(els.shoppingLists, detailed.filter((l) => l.type === 'shopping'), urlBadges, 'Aucune liste de courses pour le moment.');
  renderGrid(els.todoLists, detailed.filter((l) => l.type === 'todo'), progressBadge, 'Aucun espace tâches pour le moment.');
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

// ---------------------------------------------------------------------------
// "New list" modal
// ---------------------------------------------------------------------------

function setListTypeSelection(value) {
  for (const label of els.typeOptions) {
    const input = label.querySelector('input');
    const active = input.value === value;
    input.checked = active;
    label.classList.toggle('border-sky-500', active);
    label.classList.toggle('bg-sky-500/10', active);
    label.classList.toggle('text-sky-300', active);
    label.classList.toggle('border-slate-700', !active);
    label.classList.toggle('bg-slate-900', !active);
    label.classList.toggle('text-slate-300', !active);
  }
}

function openNewListModal() {
  els.createListForm.reset();
  setListTypeSelection('shopping');
  els.newListModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  els.listNameInput.focus();
}

function closeNewListModal() {
  els.newListModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
  els.newListButton.focus();
}

for (const label of els.typeOptions) {
  label.querySelector('input').addEventListener('change', (event) => setListTypeSelection(event.target.value));
}

els.newListButton.addEventListener('click', openNewListModal);
els.closeModalButton.addEventListener('click', closeNewListModal);
els.newListModal.addEventListener('click', (event) => {
  if (event.target === els.newListModal) closeNewListModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !els.newListModal.hidden) closeNewListModal();
});

els.createListForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const name = els.listNameInput.value.trim();
  const type = els.newListModal.querySelector('input[name="list-type"]:checked')?.value || 'shopping';
  if (!name || state.currentHouseId === null) return;

  try {
    await apiRequest('/lists', { method: 'POST', body: JSON.stringify({ name, type, house_id: state.currentHouseId }) });
    closeNewListModal();
    await loadDashboard();
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

  navigator.serviceWorker.register('/sw.js').catch((err) => {
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

  await renderDashboardFromCache();
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
  } catch (err) {
    console.warn('Session non vérifiée (probablement hors ligne) :', err);
  }

  // Stale-while-revalidate: now refresh from the network. Both calls fall
  // back to the cache again on their own if this fails, so it's safe to
  // run unconditionally regardless of whether the /me check above worked.
  await loadHouses();
  await loadDashboard();
  // loadNotifications is defined in notifications.js, resolved lazily the
  // same way every other cross-file call in this function already is.
  await loadNotifications();
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
