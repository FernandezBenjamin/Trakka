'use strict';

// "Achats & Tâches Urgentes" dashboard widget: a third tab alongside the
// dashboard and the planning view (shares `state`, `els`, `apiRequest`,
// `showError`/`hideError`, `t`, `refreshPendingBadge`, `selectList` with
// app.js/list_view.js/planning.js — same classic-<script>-tags shared-scope
// pattern as those files). Unlike planning.js it fans out over *every* list
// in the house (shopping and todo alike, since a task can be just as
// urgent as a purchase), keeping only items flagged `is_urgent` that
// aren't `done` yet — the same "still pressing" definition list_view.js
// uses to float an item to the top of its own list.

const urgentEls = {
  items: document.getElementById('urgent-items'),
};

// Every currently-urgent, not-yet-done item known for state.currentHouseId,
// as { item, listId, listName } — re-fetched on every tab activation rather
// than kept in sync incrementally, mirroring planningEntries in planning.js.
let urgentEntries = [];

// Keeps only the still-pressing items (is_urgent, not done) out of a house's
// lists, each with its items already attached — shared between the
// live-network path and the IndexedDB cache fallback below so the filter
// can't drift between them.
function extractUrgentEntries(detailedLists) {
  const entries = [];
  for (const list of detailedLists) {
    for (const item of list.items || []) {
      if (item.is_urgent && !item.done) entries.push({ item, listId: list.id, listName: list.name });
    }
  }
  return entries;
}

// Fetches every list (shopping and todo) in the current house plus each
// one's items, then hands them to extractUrgentEntries above. Re-run
// whenever the urgent tab is opened (see setActiveTab in planning.js) or
// refreshUrgentIfActive is called after something that might have changed
// urgency/done state elsewhere (offline sync completing, a language switch,
// a house switch).
async function loadUrgentView() {
  if (state.currentHouseId === null) {
    urgentEntries = [];
    renderUrgent();
    return;
  }

  // Stale-while-revalidate: paint immediately from the IndexedDB mirror
  // before the network fetch below even starts, so a first tab open in a
  // session (urgentEls.items starts out with only the static skeleton
  // markup in index.html — see base.css's "Loading skeletons" comment)
  // shows real, if possibly slightly stale, content right away instead of
  // sitting on the shimmer for a full house_id + per-list fan-out round
  // trip. Re-rendered for real below once the network settles either way.
  const cachedFirst = await cachedDashboardLists(state.currentHouseId);
  if (cachedFirst.length > 0) {
    urgentEntries = extractUrgentEntries(cachedFirst);
    renderUrgent();
  }

  try {
    const lists = await apiRequest(`/lists?house_id=${state.currentHouseId}`);
    const detailed = await Promise.all(lists.map((list) => apiRequest(`/lists/${list.id}`).catch(() => ({ ...list, items: [] }))));
    urgentEntries = extractUrgentEntries(detailed);
  } catch (err) {
    // Offline, or a transient server error: fall back to the IndexedDB
    // mirror (cachedDashboardLists, defined in app.js) instead of emptying
    // the view — see the Offline-First requirement in CLAUDE.md/docs/PWA.md.
    // A plain connectivity failure stays silent (the header's network badge
    // already says "Hors-ligne"); only a genuine server-side error still
    // raises the banner.
    const cached = await cachedDashboardLists(state.currentHouseId);
    urgentEntries = extractUrgentEntries(cached);
    if (!isNetworkError(err)) showError(err.message);
  }
  renderUrgent();
}

// Called after anything that might have changed the current house's urgent
// items (offline sync completing, a language switch, a house switch) — a
// no-op unless this tab is the one currently visible.
function refreshUrgentIfActive() {
  if (isUrgentTabActive()) loadUrgentView();
}

function renderUrgent() {
  urgentEls.items.replaceChildren();
  if (urgentEntries.length === 0) {
    const li = document.createElement('li');
    li.className = 'rounded-xl border border-dashed border-slate-200 dark:border-slate-800 p-6 text-center text-sm text-slate-500';
    li.textContent = t('urgent.empty');
    urgentEls.items.appendChild(li);
    return;
  }
  for (const entry of urgentEntries) urgentEls.items.appendChild(buildUrgentItemRow(entry));
}

function buildUrgentItemRow(entry) {
  const { item, listId, listName } = entry;
  const li = document.createElement('li');
  li.className = 'flex items-center gap-3 rounded-xl border-2 border-rose-500/60 bg-rose-500/5 p-3';

  const checkboxLabel = document.createElement('label');
  checkboxLabel.className = 'flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center';
  const checkbox = document.createElement('input');
  checkbox.type = 'checkbox';
  checkbox.checked = false;
  checkbox.className = 'h-5 w-5 rounded-full border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 text-sky-500 focus:ring-sky-500/40';
  checkbox.setAttribute('aria-label', t('urgent.markDoneAriaLabel', { title: item.title }));
  checkbox.addEventListener('change', () => markUrgentItemDone(entry));
  checkboxLabel.appendChild(checkbox);
  li.appendChild(checkboxLabel);

  const info = document.createElement('button');
  info.type = 'button';
  info.className = 'min-w-0 flex-1 text-left';
  info.setAttribute('aria-label', t('urgent.openListLabel', { list: listName }));
  const title = document.createElement('p');
  title.className = 'truncate text-base text-slate-900 dark:text-slate-100';
  title.textContent = item.quantity > 1 ? `${item.title} × ${item.quantity}` : item.title;
  const listLabel = document.createElement('p');
  listLabel.className = 'truncate text-xs text-slate-500 dark:text-slate-400';
  listLabel.textContent = listName;
  info.append(title, listLabel);
  // selectList is defined in list_view.js, resolved lazily the same way
  // moveItemToMonth's neighbors already do for cross-file calls here.
  info.addEventListener('click', () => selectList(listId));
  li.appendChild(info);

  li.appendChild(buildUrgentBadge());

  return li;
}

// Marking an item done straight from this widget is a direct, optimistic
// PATCH (no undo toast) rather than reusing list_view.js's toggleDone — the
// item here isn't part of state.currentList, and the only outcome that
// matters from this view is the item dropping out of the urgent list.
async function markUrgentItemDone(entry) {
  hideError();
  urgentEntries = urgentEntries.filter((e) => e !== entry);
  renderUrgent();

  try {
    await apiRequest(`/items/${entry.item.id}`, { method: 'PATCH', body: JSON.stringify({ done: true }) });
  } catch (err) {
    urgentEntries = [...urgentEntries, entry];
    renderUrgent();
    showError(err.message);
  }
  await refreshPendingBadge();
}
