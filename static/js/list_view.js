'use strict';

// List detail view: the "Google Tasks"-style checklist shown when a list is
// opened from the dashboard. Shares `state`, `els`, `apiRequest`,
// `showError`/`hideError`, `isSafeHttpUrl`, `TRASH_ICON_SVG`,
// `refreshPendingBadge` and `loadDashboard` with app.js — classic <script>
// tags on one page execute in the same top-level scope, so these are
// visible here as plain identifiers without any import.

const listEls = {
  itemsSection: document.getElementById('items-section'),
  itemsHeading: document.getElementById('items-heading'),
  backButton: document.getElementById('back-button'),
  financeTotal: document.getElementById('finance-total'),
  financeSpent: document.getElementById('finance-spent'),
  financeRemaining: document.getElementById('finance-remaining'),
  createItemForm: document.getElementById('create-item-form'),
  itemTitle: document.getElementById('item-title'),
  itemUrl: document.getElementById('item-url'),
  itemQuantity: document.getElementById('item-quantity'),
  itemPrice: document.getElementById('item-price'),
  itemTargetMonth: document.getElementById('item-target-month'),
  itemRecurrence: document.getElementById('item-recurrence'),
  itemsActive: document.getElementById('items-active'),
  itemsDone: document.getElementById('items-done'),
  doneSection: document.getElementById('done-section'),
  doneSummaryLabel: document.getElementById('done-summary-label'),
  editItemModal: document.getElementById('edit-item-modal'),
  closeEditItemModalButton: document.getElementById('close-edit-item-modal-button'),
  editItemForm: document.getElementById('edit-item-form'),
  editItemTitle: document.getElementById('edit-item-title'),
  editItemUrl: document.getElementById('edit-item-url'),
  editItemPrice: document.getElementById('edit-item-price'),
  editItemPriceAutoBadge: document.getElementById('edit-item-price-auto-badge'),
  editItemTargetMonth: document.getElementById('edit-item-target-month'),
  editItemRecurrence: document.getElementById('edit-item-recurrence'),
  imagePreviewModal: document.getElementById('image-preview-modal'),
  imagePreviewImg: document.getElementById('image-preview-img'),
  closeImagePreviewButton: document.getElementById('close-image-preview-button'),
};

// The item currently open in the edit modal, or null when the modal is
// closed — set by openEditItemModal, read by editItemForm's submit handler.
let editingItem = null;

const PENCIL_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5" aria-hidden="true">' +
  '<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4Z"/></svg>';

// Static, hard-coded icon markup (never interpolates user data, same rule
// as TRASH_ICON_SVG/PENCIL_ICON_SVG in app.js) — a small "sparkle" used to
// flag a price that internal/scraper filled in automatically rather than
// the user having typed it in.
const AUTO_PRICE_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="currentColor" class="h-3 w-3" aria-hidden="true"><path d="M12 2l1.8 5.2L19 9l-5.2 1.8L12 16l-1.8-5.2L5 9l5.2-1.8L12 2z"/></svg>';

// True for an item created while offline whose "create" request is still
// sitting in the service worker's sync queue — see the tempId generation in
// sw.js's queueOfflineWrite. Such an item's price (if any comes from a url)
// can't have been looked up yet since the request never reached the server.
function isOfflineQueuedItem(item) {
  return typeof item.id === 'string' && item.id.startsWith('temp-item');
}

// A small non-interactive badge for a price that isn't known yet — either
// because the item is still offline-queued (waiting on connectivity) or
// because the server's bounded synchronous lookup (see scrapePrice in
// internal/handlers/scrape.go) didn't finish in time and is still running
// in the background (see scheduleAutoPriceRefresh below).
function buildPendingPriceBadge(label, palette) {
  const span = document.createElement('span');
  span.className = `shrink-0 rounded-full px-2 py-0.5 text-xs font-medium ${palette}`;
  span.textContent = label;
  return span;
}

const euroFormatter = new Intl.NumberFormat('fr-FR', { style: 'currency', currency: 'EUR' });
function formatEuro(amount) {
  return euroFormatter.format(amount);
}

function typeLabel(type) {
  return type === 'todo' ? 'tâches' : 'courses';
}

// Fills listEls.itemTargetMonth with a fixed set of rolling-month options,
// once: "Non planifié" (the default), then 12 months starting with the
// current one, worded as "Ce mois-ci (<mois>)" / "Mois prochain (<mois>)"
// for the first two and a plain "<mois> <année>" label after that.
// monthsFromNow/monthLabel are defined in planning.js, loaded after this
// file — calling this eagerly at parse time would hit them before they
// exist, so it's called lazily instead, from applyListTypeVisibility below
// (first invoked from renderItems, well after every script has loaded),
// the same deferred-call pattern buildItemRow already uses for monthLabel.
function ensureTargetMonthOptions() {
  const select = listEls.itemTargetMonth;
  if (select.options.length > 0) return;

  const unscheduled = document.createElement('option');
  unscheduled.value = '';
  unscheduled.textContent = t('items.targetMonthUnscheduled');
  select.appendChild(unscheduled);

  monthsFromNow(12).forEach((month, index) => {
    const option = document.createElement('option');
    option.value = month;
    const label = monthLabel(month, 'short');
    if (index === 0) option.textContent = t('items.targetMonthCurrent', { month: label });
    else if (index === 1) option.textContent = t('items.targetMonthNext', { month: label });
    else option.textContent = label;
    select.appendChild(option);
  });
}

// Shows/hides every element flagged data-shopping-only (the price input on
// the create-item form, the price field in the edit modal, and the
// financial summary bar) — price only makes sense on `shopping` lists.
function applyListTypeVisibility(type) {
  const isShopping = type === 'shopping';
  ensureTargetMonthOptions();
  for (const el of document.querySelectorAll('[data-shopping-only]')) {
    el.hidden = !isShopping;
  }
}

// Recomputes the financial summary bar directly from state.currentList.items
// — the same array toggleDone/removeItem/create/edit mutate optimistically —
// so it stays in sync whether the underlying change came from the network,
// an optimistic local edit, or the offline sync queue.
function updateFinanceSummary(items) {
  let total = 0;
  let spent = 0;
  for (const item of items) {
    const price = typeof item.price === 'number' ? item.price : 0;
    total += price;
    if (item.done) spent += price;
  }
  listEls.financeTotal.textContent = formatEuro(total);
  listEls.financeSpent.textContent = formatEuro(spent);
  listEls.financeRemaining.textContent = formatEuro(total - spent);
}

// recurrenceBadgeLabel turns an item.recurrence_rule value (one of the
// fixed cadences, or the custom "EVERY_X_DAYS:<n>" form — see
// internal/validate.Recurrence) into the short, translated phrase shown
// next to the 🔄 pictogram on a recurring item's row, e.g. "Chaque semaine".
function recurrenceBadgeLabel(rule) {
  switch (rule) {
    case 'DAILY':
      return t('items.recurrenceBadgeDaily');
    case 'WEEKLY':
      return t('items.recurrenceBadgeWeekly');
    case 'MONTHLY':
      return t('items.recurrenceBadgeMonthly');
    case 'YEARLY':
      return t('items.recurrenceBadgeYearly');
    default: {
      const match = /^EVERY_X_DAYS:([1-9][0-9]*)$/.exec(rule || '');
      return match ? t('items.recurrenceBadgeEveryXDays', { n: match[1] }) : null;
    }
  }
}

// A discreet, non-interactive badge (🔄 + frequency) shown on a recurring
// item's row — see recurrenceBadgeLabel. Completing a recurring item never
// removes the badge: the server (or, offline, sw.js's own mirror of the
// same logic) advances due_date and un-checks the item instead of clearing
// recurrence_rule, so the row simply reappears among the active items with
// the badge still in place.
function buildRecurrenceBadge(item) {
  const label = recurrenceBadgeLabel(item.recurrence_rule);
  if (!label) return null;
  const badge = document.createElement('span');
  badge.className = 'flex shrink-0 items-center gap-1 rounded-full bg-slate-700/40 px-2 py-0.5 text-xs font-medium text-slate-300';
  badge.setAttribute('aria-label', t('items.recurrenceBadgeAriaLabel', { frequency: label }));
  const icon = document.createElement('span');
  icon.setAttribute('aria-hidden', 'true');
  icon.textContent = '🔄';
  const text = document.createElement('span');
  text.textContent = label;
  badge.appendChild(icon);
  badge.appendChild(text);
  return badge;
}

function emptyItemsRow(message) {
  const li = document.createElement('li');
  li.className = 'rounded-xl border border-dashed border-slate-800 p-6 text-center text-sm text-slate-500';
  li.textContent = message;
  return li;
}

// buildItemThumbnail renders a small (48px) clickable product thumbnail for
// an item whose image_url passed isSafeHttpUrl. It's a plain <img> (never
// innerHTML with the URL interpolated) set via the `src` property, which is
// the same safe pattern app.js already uses for <a href> — the browser
// treats `src` as a URL attribute, not markup, so this can't inject HTML;
// isSafeHttpUrl having already rejected non-http(s) schemes is what stops a
// stray "javascript:" URL from being handed to the image loader at all.
// Hovering scales it up in place for a quick glance (CSS only, via Tailwind
// utility classes); clicking (or focusing + Enter/Space, since it's a real
// <button>) opens the full-size lightbox instead of navigating anywhere.
function buildItemThumbnail(item) {
  const button = document.createElement('button');
  button.type = 'button';
  button.title = 'Agrandir l’image';
  button.setAttribute('aria-label', `Agrandir l'image de ${item.title}`);
  button.className =
    'block h-12 w-12 shrink-0 overflow-hidden rounded-lg border border-slate-700 bg-slate-900 transition duration-150 hover:z-10 hover:scale-150 hover:shadow-lg focus-visible:z-10 focus-visible:scale-150';

  const img = document.createElement('img');
  img.src = item.image_url;
  img.alt = '';
  img.loading = 'lazy';
  img.className = 'h-full w-full object-cover';
  // A broken/expired image URL (site redesign, hotlink protection, ...)
  // should just quietly disappear rather than showing a browser's broken
  // image icon — there is no server-side way to know in advance that a
  // once-valid scraped URL has gone stale.
  img.addEventListener('error', () => button.remove());
  button.appendChild(img);

  button.addEventListener('click', () => openImagePreview(item));
  return button;
}

function openImagePreview(item) {
  if (!item.image_url || !isSafeHttpUrl(item.image_url)) return;
  listEls.imagePreviewImg.src = item.image_url;
  listEls.imagePreviewImg.alt = item.title;
  listEls.imagePreviewModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  listEls.closeImagePreviewButton.focus();
}

function closeImagePreview() {
  listEls.imagePreviewModal.hidden = true;
  listEls.imagePreviewImg.src = '';
  document.body.classList.remove('overflow-hidden');
}

listEls.closeImagePreviewButton.addEventListener('click', closeImagePreview);
listEls.imagePreviewModal.addEventListener('click', (event) => {
  if (event.target === listEls.imagePreviewModal) closeImagePreview();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !listEls.imagePreviewModal.hidden) closeImagePreview();
});

function buildItemRow(item) {
  const li = document.createElement('li');
  li.className = 'flex items-center gap-3 rounded-xl border border-slate-800 bg-slate-800/30 p-3';

  const checkboxLabel = document.createElement('label');
  checkboxLabel.className = 'flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center';
  const checkbox = document.createElement('input');
  checkbox.type = 'checkbox';
  checkbox.checked = item.done;
  checkbox.className = 'h-5 w-5 rounded-full border-slate-600 bg-slate-900 text-sky-500 focus:ring-sky-500/40';
  checkbox.setAttribute('aria-label', `Marquer « ${item.title} » comme terminé`);
  checkbox.addEventListener('change', () => toggleDone(item));
  checkboxLabel.appendChild(checkbox);

  li.appendChild(checkboxLabel);

  // A thumbnail only ever appears when a product image was actually found
  // (see internal/scraper's og:image/JSON-LD/twitter:image lookup) — no
  // placeholder box is reserved for items without one, so plain to-do items
  // and shopping items with no matched image just skip straight to the
  // title, unchanged from before this feature existed.
  if (item.image_url && isSafeHttpUrl(item.image_url)) {
    li.appendChild(buildItemThumbnail(item));
  }

  const title = document.createElement('span');
  title.className = item.done ? 'flex-1 text-base text-slate-500 line-through opacity-60' : 'flex-1 text-base text-slate-100';
  title.textContent = item.quantity > 1 ? `${item.title} × ${item.quantity}` : item.title;

  li.appendChild(title);

  if (item.price != null) {
    const priceWrap = document.createElement('span');
    priceWrap.className = 'flex shrink-0 items-center gap-1';

    const price = document.createElement('span');
    price.className = 'text-sm font-medium text-slate-300';
    price.textContent = formatEuro(item.price);
    priceWrap.appendChild(price);

    // Auto-detected prices get a small clickable badge instead of a plain
    // label — clicking it opens the same edit modal as the pencil icon,
    // so "modifier en un clic" just reuses the existing edit flow rather
    // than needing a dedicated inline editor.
    if (item.price_auto) {
      const autoBadge = document.createElement('button');
      autoBadge.type = 'button';
      autoBadge.title = 'Prix détecté automatiquement — cliquer pour modifier';
      autoBadge.setAttribute('aria-label', `Prix détecté automatiquement pour ${item.title}, cliquer pour modifier`);
      autoBadge.className = 'flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-300 hover:bg-emerald-500/20';
      autoBadge.innerHTML = AUTO_PRICE_ICON_SVG;
      autoBadge.addEventListener('click', () => openEditItemModal(item));
      priceWrap.appendChild(autoBadge);
    }

    li.appendChild(priceWrap);
  } else if (item.url) {
    if (isOfflineQueuedItem(item)) {
      li.appendChild(buildPendingPriceBadge('Prix en attente de synchro', 'bg-amber-500/10 text-amber-300'));
    } else if (item.priceScrapePending) {
      li.appendChild(buildPendingPriceBadge('Détection du prix…', 'animate-pulse bg-sky-500/10 text-sky-300'));
    }
  }

  // Only shown once an item has actually been scheduled via the edit
  // modal's "Mois prévu" field — monthLabel is defined in planning.js,
  // safe to call here since all script tags finish loading and defining
  // their top-level functions before any rendering actually runs.
  if (item.target_month) {
    const monthBadge = document.createElement('span');
    monthBadge.className = 'shrink-0 rounded-full bg-violet-500/10 px-2 py-0.5 text-xs font-medium text-violet-300';
    monthBadge.textContent = monthLabel(item.target_month, 'short');
    li.appendChild(monthBadge);
  }

  if (item.recurrence_rule) {
    const recurrenceBadge = buildRecurrenceBadge(item);
    if (recurrenceBadge) li.appendChild(recurrenceBadge);
  }

  if (item.url && isSafeHttpUrl(item.url)) {
    const link = document.createElement('a');
    link.href = item.url;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.className = 'rounded-full bg-sky-500/10 px-2.5 py-1 text-xs font-medium text-sky-300 hover:bg-sky-500/20';
    link.textContent = 'lien ↗';
    li.appendChild(link);
  }

  const editBtn = document.createElement('button');
  editBtn.type = 'button';
  editBtn.setAttribute('aria-label', `Modifier ${item.title}`);
  editBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-sky-500/10 hover:text-sky-400';
  editBtn.innerHTML = PENCIL_ICON_SVG;
  editBtn.addEventListener('click', () => openEditItemModal(item));
  li.appendChild(editBtn);

  const deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.setAttribute('aria-label', `Supprimer ${item.title}`);
  deleteBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-400';
  deleteBtn.innerHTML = TRASH_ICON_SVG;
  deleteBtn.addEventListener('click', () => removeItem(item));
  li.appendChild(deleteBtn);

  return li;
}

// Renders directly from state.currentList — the single source of truth
// that toggleDone/removeItem/the create-item handler mutate optimistically
// before their request resolves. Splitting into active/done here (rather
// than trusting a `done` flag baked into row position) is what makes
// checking/unchecking an item move it between sections on every render.
function renderItems() {
  const list = state.currentList;
  if (!list) return;

  listEls.itemsHeading.textContent = `${list.name} (${typeLabel(list.type)})`;
  applyListTypeVisibility(list.type);

  // Items mid-undo-grace-period (see removeItem below) stay in
  // state.currentList.items — so undo can just clear the flag and re-render
  // with nothing else to reconstruct — but are filtered out of every view
  // (including the finance summary) as if already gone.
  const items = (list.items || []).filter((item) => !item.pendingDelete);
  const active = items.filter((item) => !item.done);
  const done = items.filter((item) => item.done);

  if (list.type === 'shopping') updateFinanceSummary(items);

  listEls.itemsActive.replaceChildren();
  if (items.length === 0) {
    listEls.itemsActive.appendChild(emptyItemsRow('Aucun élément pour le moment.'));
  } else if (active.length === 0) {
    listEls.itemsActive.appendChild(emptyItemsRow('Tout est terminé pour le moment.'));
  } else {
    for (const item of active) listEls.itemsActive.appendChild(buildItemRow(item));
  }

  listEls.itemsDone.replaceChildren();
  for (const item of done) listEls.itemsDone.appendChild(buildItemRow(item));
  listEls.doneSummaryLabel.textContent = `Terminés (${done.length})`;
  listEls.doneSection.hidden = done.length === 0;
}

// Reads a single list (with its items) from the IndexedDB mirror, shaped
// like a successful `GET /api/v1/lists/{id}` — db.js's getListWithItems
// already does the composition. Resolves to null (never throws) whenever
// the module is unavailable, the read fails, or the list isn't mirrored.
async function cachedListDetail(id) {
  if (!window.TrakkaDB) return null;
  try {
    return await window.TrakkaDB.getListWithItems(id);
  } catch {
    return null;
  }
}

// Re-fetches the current list from the API and re-renders — used only to
// reconcile with the server after the offline sync queue flushes (see
// app.js's 'trakka-sync-complete' handler). Regular mutations below apply
// optimistically instead of round-tripping through this.
async function refreshCurrentList() {
  if (state.currentListId === null) return;
  try {
    state.currentList = await apiRequest(`/lists/${state.currentListId}`);
    renderItems();
  } catch (err) {
    // Offline, or a transient server error: fall back to the local mirror
    // rather than leaving the item list stuck on stale data with no
    // explanation — see the Offline-First requirement in
    // CLAUDE.md/docs/PWA.md.
    const cached = await cachedListDetail(state.currentListId);
    if (cached) {
      state.currentList = cached;
      renderItems();
    } else {
      showError(err.message);
    }
  }
}

async function selectList(id) {
  hideError();
  let list;
  try {
    list = await apiRequest(`/lists/${id}`);
  } catch (err) {
    // Offline, or a transient server error: open the list from the local
    // mirror instead of failing to open it at all.
    list = await cachedListDetail(id);
    if (!list) {
      showError(err.message);
      return;
    }
  }
  state.currentListId = id;
  state.currentList = list;
  els.listsSection.hidden = true;
  listEls.itemsSection.hidden = false;
  renderItems();
}

function showDashboard() {
  state.currentListId = null;
  state.currentList = null;
  listEls.itemsSection.hidden = true;
  els.listsSection.hidden = false;
  loadDashboard();
}

// ---------------------------------------------------------------------------
// Optimistic mutations — the view updates immediately from local state;
// the API call runs in the background and rolls the change back (with an
// error banner) if it fails outright. A request the service worker queues
// while offline still resolves as a 2xx, so the optimistic state simply
// becomes final in that case — no separate offline branch needed here.
// ---------------------------------------------------------------------------

// Tracks, per item, the undo toast/timer of its most recent still-pending
// toggle plus the last server-confirmed `done` value — so re-clicking the
// same checkbox again before the previous toggle has committed replaces the
// pending action instead of racing two conflicting PATCH requests. Keyed by
// the item object itself (not its id), since an offline-queued item's id
// can change (temp-item-* -> real id) out from under a still-pending toggle.
const pendingToggles = new Map();

function toggleDone(item) {
  hideError();

  const pending = pendingToggles.get(item);
  const committedDone = pending ? pending.committedDone : item.done;
  if (pending) pending.dismiss();

  item.done = !item.done;
  renderItems();
  const newDone = item.done;

  if (newDone === committedDone) {
    // Back to the last server-confirmed state within the grace window
    // (checked, then unchecked again) — nothing to sync, nothing to undo.
    pendingToggles.delete(item);
    return;
  }

  const controller = TrakkaUndo.schedule({
    message: t(newDone ? 'undo.itemMarkedDone' : 'undo.itemMarkedUndone', { title: item.title }),
    undoLabel: t('undo.cancel'),
    onUndo: () => {
      pendingToggles.delete(item);
      item.done = committedDone;
      renderItems();
    },
    onCommit: async () => {
      pendingToggles.delete(item);
      try {
        const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({ done: newDone }) });
        Object.assign(item, updated);
      } catch (err) {
        item.done = committedDone;
        showError(err.message);
      } finally {
        renderItems();
      }
      await refreshPendingBadge();
    },
  });

  pendingToggles.set(item, { dismiss: controller.dismiss, committedDone });
}

// Deletion is deferred behind a 5s undo grace period: the item disappears
// from view immediately (renderItems filters out `pendingDelete`) but stays
// in state.currentList.items until the countdown actually runs out, so undo
// only needs to clear the flag rather than reconstruct the item's position
// in the array. The real DELETE only fires from TrakkaUndo's onCommit, so
// this needs no special handling for the offline queue in sw.js — whether
// that request ends up going out online or offline is invisible from here.
function removeItem(item) {
  hideError();

  const pendingToggle = pendingToggles.get(item);
  if (pendingToggle) {
    pendingToggle.dismiss();
    pendingToggles.delete(item);
  }

  item.pendingDelete = true;
  renderItems();

  TrakkaUndo.schedule({
    message: t('undo.itemDeleted', { title: item.title }),
    undoLabel: t('undo.cancel'),
    onUndo: () => {
      delete item.pendingDelete;
      renderItems();
    },
    onCommit: async () => {
      const items = state.currentList.items;
      const index = items.indexOf(item);
      if (index !== -1) items.splice(index, 1);

      try {
        await apiRequest(`/items/${item.id}`, { method: 'DELETE' });
      } catch (err) {
        if (index !== -1) items.splice(index, 0, item);
        delete item.pendingDelete;
        showError(err.message);
      }
      renderItems();
      await refreshPendingBadge();
    },
  });
}

// The create/update/patch handlers already wait briefly for the price
// lookup and return it directly when it lands in time (price_status
// "found") — see scrapePrice in internal/handlers/scrape.go. This is only
// for the remaining case: price_status "pending" means the lookup was
// still running server-side when the response was sent. There's no push
// channel to tell the client when that finishes, so this just re-fetches
// the item once, a few seconds later, comfortably after the scraper's own
// bounded timeout, and merges the result in if the item (and its id) are
// still around. A single delayed check, not polling.
function scheduleAutoPriceRefresh(item) {
  setTimeout(async () => {
    if (typeof item.id !== 'number') return; // still optimistic/offline-queued
    if (!state.currentList || !state.currentList.items.includes(item)) return;
    try {
      const fresh = await apiRequest(`/items/${item.id}`);
      if (state.currentList.items.includes(item)) {
        Object.assign(item, fresh);
        item.priceScrapePending = false;
        renderItems();
      }
    } catch {
      // best effort only — item may have been deleted, or we're offline
    }
  }, 4000);
}

listEls.createItemForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  if (state.currentListId === null || !state.currentList) return;

  const title = listEls.itemTitle.value.trim();
  const url = listEls.itemUrl.value.trim();
  const quantity = Number.parseInt(listEls.itemQuantity.value, 10) || 1;
  if (!title) return;

  // Price and target month only apply to shopping lists — gated on the
  // list's actual type rather than the inputs' `hidden` state, so a stale
  // value left over from a previously-open shopping list can never leak
  // into a todo item.
  let price = null;
  let targetMonth = '';
  if (state.currentList.type === 'shopping') {
    const raw = listEls.itemPrice.value.trim();
    if (raw !== '') {
      const parsed = Number.parseFloat(raw);
      if (Number.isFinite(parsed) && parsed >= 0) price = parsed;
    }
    targetMonth = listEls.itemTargetMonth.value;
  }

  // Recurrence applies regardless of list type (a todo chore repeats just
  // as naturally as a shopping item), unlike price/target_month above.
  const recurrenceRule = listEls.itemRecurrence.value;

  const payload = { list_id: state.currentListId, title, quantity };
  if (url) payload.url = url;
  if (price !== null) payload.price = price;
  if (targetMonth) payload.target_month = targetMonth;
  if (recurrenceRule) payload.recurrence_rule = recurrenceRule;

  // Locally-scoped id, distinct from the server's own `temp-item-*` ids
  // (assigned by sw.js when a request is queued offline) — this one only
  // ever needs to identify the row within this array until Object.assign
  // below replaces it with whatever the request actually resolves to.
  const optimisticItem = {
    id: `local-${Date.now()}-${Math.random().toString(36).slice(2)}`,
    list_id: state.currentListId,
    title,
    url: url || null,
    quantity,
    price,
    target_month: targetMonth || null,
    recurrence_rule: recurrenceRule || null,
    done: false,
    position: 0,
  };
  state.currentList.items = [...(state.currentList.items || []), optimisticItem];
  renderItems();
  listEls.createItemForm.reset();
  listEls.itemQuantity.value = '1';
  listEls.itemTitle.focus();

  try {
    const created = await apiRequest('/items', { method: 'POST', body: JSON.stringify(payload) });
    Object.assign(optimisticItem, created);
    optimisticItem.priceScrapePending = created.price_status === 'pending';
    if (optimisticItem.priceScrapePending) scheduleAutoPriceRefresh(optimisticItem);
  } catch (err) {
    state.currentList.items = state.currentList.items.filter((item) => item !== optimisticItem);
    showError(err.message);
  } finally {
    renderItems();
  }
  await refreshPendingBadge();
});

listEls.backButton.addEventListener('click', () => {
  hideError();
  showDashboard();
});

// ---------------------------------------------------------------------------
// Edit-item modal — lets the user change an existing item's name, URL and
// (on shopping lists) price via PATCH, without touching its done/position.
// ---------------------------------------------------------------------------

function openEditItemModal(item) {
  editingItem = item;
  listEls.editItemTitle.value = item.title;
  listEls.editItemUrl.value = item.url || '';
  listEls.editItemPrice.value = item.price != null ? item.price : '';
  listEls.editItemPriceAutoBadge.hidden = !item.price_auto;
  listEls.editItemTargetMonth.value = item.target_month || '';
  listEls.editItemRecurrence.value = item.recurrence_rule || '';
  listEls.editItemModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  listEls.editItemTitle.focus();
}

// Editing the price field by hand is what "modifier en un clic" means in
// practice — as soon as the user touches it, the auto-detected badge no
// longer applies to whatever they're about to save (the server resets
// price_auto to false the moment a manual price is submitted; hiding the
// badge immediately here just keeps the modal from lying in the meantime).
listEls.editItemPrice.addEventListener('input', () => {
  listEls.editItemPriceAutoBadge.hidden = true;
});

function closeEditItemModal() {
  editingItem = null;
  listEls.editItemModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

listEls.closeEditItemModalButton.addEventListener('click', closeEditItemModal);
listEls.editItemModal.addEventListener('click', (event) => {
  if (event.target === listEls.editItemModal) closeEditItemModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !listEls.editItemModal.hidden) closeEditItemModal();
});

listEls.editItemForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  if (!editingItem) return;

  const title = listEls.editItemTitle.value.trim();
  const url = listEls.editItemUrl.value.trim();
  if (!title) return;

  // `price` and `target_month` are only ever included for shopping lists.
  // When included, an empty field sends an explicit "clear" value (`null`
  // for price, `""` for target_month) rather than omitting the key (leave
  // unchanged) — see the PATCH handler's field-presence handling in
  // internal/handlers/items.go.
  // Recurrence, unlike price/target_month, applies to both list types, so
  // it's always included in the payload rather than gated on
  // state.currentList.type — an empty value clears it back to "not
  // recurring", same "absent = untouched, empty = clear" PATCH convention.
  const payload = { title, url, recurrence_rule: listEls.editItemRecurrence.value };
  if (state.currentList && state.currentList.type === 'shopping') {
    const raw = listEls.editItemPrice.value.trim();
    if (raw === '') {
      payload.price = null;
    } else {
      const parsed = Number.parseFloat(raw);
      if (!Number.isFinite(parsed) || parsed < 0) {
        showError('Le prix doit être un nombre positif.');
        return;
      }
      payload.price = parsed;
    }
    payload.target_month = listEls.editItemTargetMonth.value;
  }

  const item = editingItem;
  const previous = {
    title: item.title,
    url: item.url,
    price: item.price,
    target_month: item.target_month,
    recurrence_rule: item.recurrence_rule,
  };
  item.title = title;
  item.url = url || null;
  if ('price' in payload) item.price = payload.price;
  if ('target_month' in payload) item.target_month = payload.target_month || null;
  item.recurrence_rule = payload.recurrence_rule || null;
  renderItems();
  closeEditItemModal();

  try {
    const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify(payload) });
    Object.assign(item, updated);
    item.priceScrapePending = updated.price_status === 'pending';
    if (item.priceScrapePending) scheduleAutoPriceRefresh(item);
  } catch (err) {
    Object.assign(item, previous);
    showError(err.message);
  } finally {
    renderItems();
  }
  await refreshPendingBadge();
});
