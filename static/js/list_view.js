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
  listSyncIndicator: document.getElementById('list-sync-indicator'),
  itemsRemainingCount: document.getElementById('items-remaining-count'),
  backButton: document.getElementById('back-button'),
  editListButton: document.getElementById('edit-list-button'),
  shareListButton: document.getElementById('share-list-button'),
  financeSummary: document.getElementById('finance-summary'),
  financeTotal: document.getElementById('finance-total'),
  financeTotalCompact: document.getElementById('finance-total-compact'),
  financeSpent: document.getElementById('finance-spent'),
  financeRemaining: document.getElementById('finance-remaining'),
  createItemFormAnchor: document.getElementById('create-item-form-anchor'),
  createItemForm: document.getElementById('create-item-form'),
  itemTitle: document.getElementById('item-title'),
  quickAddToggle: document.getElementById('quick-add-toggle'),
  quickAddAdvanced: document.getElementById('quick-add-advanced'),
  itemUrl: document.getElementById('item-url'),
  itemQuantity: document.getElementById('item-quantity'),
  itemPrice: document.getElementById('item-price'),
  itemTargetMonth: document.getElementById('item-target-month'),
  itemRecurrence: document.getElementById('item-recurrence'),
  itemUrgent: document.getElementById('item-urgent'),
  addItemFab: document.getElementById('add-item-fab'),
  addItemSheet: document.getElementById('add-item-sheet'),
  addItemSheetBody: document.getElementById('add-item-sheet-body'),
  closeAddItemSheetButton: document.getElementById('close-add-item-sheet-button'),
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
  editItemUrgent: document.getElementById('edit-item-urgent'),
  itemActionsSheet: document.getElementById('item-actions-sheet'),
  itemActionsSheetTitle: document.getElementById('item-actions-sheet-title'),
  closeItemActionsSheetButton: document.getElementById('close-item-actions-sheet-button'),
  itemActionsEditButton: document.getElementById('item-actions-edit-button'),
  itemActionsLinkGroup: document.getElementById('item-actions-link-group'),
  itemActionsOpenLinkButton: document.getElementById('item-actions-open-link-button'),
  itemActionsCopyLinkButton: document.getElementById('item-actions-copy-link-button'),
  itemActionsShareLinkButton: document.getElementById('item-actions-share-link-button'),
  itemActionsUrgentButton: document.getElementById('item-actions-urgent-button'),
  itemActionsUrgentLabel: document.getElementById('item-actions-urgent-label'),
  itemActionsDeleteButton: document.getElementById('item-actions-delete-button'),
  imagePreviewModal: document.getElementById('image-preview-modal'),
  imagePreviewImg: document.getElementById('image-preview-img'),
  closeImagePreviewButton: document.getElementById('close-image-preview-button'),
};

// The item currently open in the item-actions bottom sheet (#item-actions-
// sheet), or null when it's closed — set by openItemActionsSheet, read by
// that sheet's own button handlers below. Every entry point that acts on an
// item (title tap, kebab tap, long press) opens this same sheet — there is
// no separate link-only sheet to keep in sync with it.
let itemActionsSheetItem = null;

// The item currently open in the edit modal, or null when the modal is
// closed — set by openEditItemModal, read by editItemForm's submit handler.
let editingItem = null;

const PENCIL_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5" aria-hidden="true">' +
  '<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/><path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4Z"/></svg>';

// Static, hard-coded icon markup (never interpolates user data, same rule as
// PENCIL_ICON_SVG/TRASH_ICON_SVG) for the quantity stepper's [-]/[+] buttons.
const MINUS_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5" aria-hidden="true"><path d="M5 12h14"/></svg>';
const PLUS_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-3.5 w-3.5" aria-hidden="true"><path d="M5 12h14"/><path d="M12 5v14"/></svg>';

// Static, hard-coded icon markup (never interpolates user data, same rule
// as TRASH_ICON_SVG/PENCIL_ICON_SVG in app.js) — a small "sparkle" used to
// flag a price that internal/scraper filled in automatically rather than
// the user having typed it in.
const AUTO_PRICE_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="currentColor" class="h-3 w-3" aria-hidden="true"><path d="M12 2l1.8 5.2L19 9l-5.2 1.8L12 16l-1.8-5.2L5 9l5.2-1.8L12 2z"/></svg>';

// Static, hard-coded icon markup (never interpolates user data) for the [⋮]
// kebab button — the mobile replacement for the ✏️/🗑️ buttons, opening
// #item-actions-sheet instead (see buildItemRow and openItemActionsSheet).
const KEBAB_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="currentColor" class="h-5 w-5" aria-hidden="true"><circle cx="12" cy="5" r="1.75"/><circle cx="12" cy="12" r="1.75"/><circle cx="12" cy="19" r="1.75"/></svg>';

// ---------------------------------------------------------------------------
// Link actions — Ouvrir/Copier/Partager, shared by #item-actions-sheet's
// link group (its only caller — see buildItemRow/openItemActionsSheet below)
// so the clipboard/share-API handling isn't duplicated anywhere else.
// ---------------------------------------------------------------------------

function openLink(url) {
  if (!url || !isSafeHttpUrl(url)) return;
  window.open(url, '_blank', 'noopener,noreferrer');
}

// document.execCommand('copy') is deprecated but remains the only dependency
// -free fallback for a browser/context without navigator.clipboard.writeText
// (older WebKit, or any non-secure-context edge case) — there is no modern
// replacement that doesn't pull in a library, which CLAUDE.md's "no
// dependencies" frontend convention rules out.
function legacyCopyToClipboard(text) {
  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  textarea.style.left = '-9999px';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    document.execCommand('copy');
  } finally {
    textarea.remove();
  }
}

async function copyLink(url) {
  if (!url) return;
  try {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      await navigator.clipboard.writeText(url);
    } else {
      legacyCopyToClipboard(url);
    }
    TrakkaToast.success(t('items.linkCopied'));
  } catch {
    showError(t('items.linkCopyFailed'));
  }
}

// navigator.share triggers the OS-native share sheet — supported on most
// mobile browsers and a handful of desktop ones (Safari, Edge), but not
// Chrome/Firefox desktop — so this falls back to copyLink wherever it's
// unavailable, per the feature's own "bascule sur la copie" fallback rule,
// rather than "Partager" silently doing nothing on unsupported browsers.
async function shareLink(item) {
  const url = item && item.url;
  if (!url || !isSafeHttpUrl(url)) return;
  if (navigator.share) {
    try {
      await navigator.share({ title: item.title, url });
    } catch (err) {
      // AbortError: the user closed/canceled the native share sheet — not a
      // real failure, nothing to report.
      if (err && err.name !== 'AbortError') showError(t('items.linkShareFailed'));
    }
    return;
  }
  await copyLink(url);
}

// ---------------------------------------------------------------------------
// Long-press gesture (~500ms touch hold) — the mobile affordance that opens
// #item-actions-sheet (focused on its link group) from an item's title or
// its inline 🔗 link icon (see buildItemRow) — the same sheet a short tap
// already opens, just pre-focused on the link actions rather than a separate
// sheet of its own. Touch-only by design: a desktop mouse never fires
// touchstart, so attachLongPress is a complete no-op there and the element's
// ordinary click handler (openItemActionsSheet on the title, ordinary
// navigation on the link icon) is entirely unaffected.
// ---------------------------------------------------------------------------

const LONG_PRESS_MS = 500;
const LONG_PRESS_MOVE_TOLERANCE_PX = 10;

// Attaches the gesture to `el`, calling `onLongPress(event)` once the touch
// has been held in place for LONG_PRESS_MS. A move past the tolerance (the
// user is scrolling, not pressing) or an early lift cancels it. `touchend`'s
// own default is prevented when a long press actually fired, so the
// synthetic `click` a touch normally triggers afterward can't also run the
// element's ordinary tap handler on top of the long-press action.
function attachLongPress(el, onLongPress) {
  let timer = null;
  let startX = 0;
  let startY = 0;
  let firedLongPress = false;

  function clearTimer() {
    if (timer) clearTimeout(timer);
    timer = null;
  }

  el.addEventListener(
    'touchstart',
    (event) => {
      if (event.touches.length !== 1) {
        clearTimer();
        return;
      }
      const touch = event.touches[0];
      startX = touch.clientX;
      startY = touch.clientY;
      firedLongPress = false;
      clearTimer();
      timer = setTimeout(() => {
        firedLongPress = true;
        timer = null;
        if (navigator.vibrate) {
          try {
            navigator.vibrate(50);
          } catch {
            // Vibration blocked/unsupported — a purely cosmetic touch, skip it.
          }
        }
        onLongPress(event);
      }, LONG_PRESS_MS);
    },
    { passive: true },
  );

  el.addEventListener(
    'touchmove',
    (event) => {
      if (!timer) return;
      const touch = event.touches[0];
      if (
        Math.abs(touch.clientX - startX) > LONG_PRESS_MOVE_TOLERANCE_PX ||
        Math.abs(touch.clientY - startY) > LONG_PRESS_MOVE_TOLERANCE_PX
      ) {
        clearTimer();
      }
    },
    { passive: true },
  );

  el.addEventListener(
    'touchend',
    (event) => {
      clearTimer();
      if (firedLongPress) event.preventDefault();
    },
    { passive: false },
  );

  el.addEventListener('touchcancel', () => {
    clearTimer();
    firedLongPress = false;
  });

  // Suppresses the native long-press context menu (Android Chrome's
  // copy-link/open-in-new-tab popup, and — via `long-press-target` in
  // base.css — iOS Safari's link callout) only while our own gesture is
  // mid-flight or just fired, so a genuine desktop right-click (which never
  // sets either flag) is completely unaffected.
  el.addEventListener('contextmenu', (event) => {
    if (firedLongPress || timer) event.preventDefault();
  });
}

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
  switch (type) {
    case 'todo':
      return t('items.typeLabelTasks');
    case 'recurring_shopping':
      return t('items.typeLabelSubscriptions');
    case 'custom':
      return t('items.typeLabelNotes');
    default:
      return t('items.typeLabelShopping');
  }
}

// FIELD_VISIBILITY_BY_TYPE maps a list type to which of the create/edit-item
// form's optional fields make sense for it: `url`/`price`/`target-month`/
// `recurrence`/`quantity`/`urgent` each correspond to one or more elements
// flagged data-item-field="..." in index.html (see applyListTypeVisibility
// below); `done` isn't a form field but tells renderItems/buildItemRow below
// whether to render the completion checkbox at all. `groceries` shows only
// the always-present name/quantity fields; `shopping` adds URL/price/target
// month (purchase planning) but not recurrence; `recurring_shopping` adds
// URL/price/recurrence instead of a target month, since a subscription/
// recurring purchase isn't scheduled for one specific month. `custom` (a
// freeform list — names, ideas, notes) is the odd one out: it has no notion
// of a "done" task, a price, a schedule, or urgency, so it hides every
// optional field and even the completion checkbox itself, leaving only the
// name — see buildItemRow's line-number marker for what replaces the
// checkbox. Any other/unrecognized type falls back to
// DEFAULT_FIELD_VISIBILITY, the same shape every list had before groceries/
// recurring_shopping/custom existed: URL and recurrence shown, price/target
// month hidden.
const FIELD_VISIBILITY_BY_TYPE = {
  groceries: { url: false, price: false, targetMonth: false, recurrence: false, quantity: true, urgent: true, done: true },
  shopping: { url: true, price: true, targetMonth: true, recurrence: false, quantity: true, urgent: true, done: true },
  recurring_shopping: { url: true, price: true, targetMonth: false, recurrence: true, quantity: true, urgent: true, done: true },
  custom: { url: false, price: false, targetMonth: false, recurrence: false, quantity: false, urgent: false, done: false },
};
const DEFAULT_FIELD_VISIBILITY = { url: true, price: false, targetMonth: false, recurrence: true, quantity: true, urgent: true, done: true };

function fieldVisibilityFor(type) {
  return FIELD_VISIBILITY_BY_TYPE[type] || DEFAULT_FIELD_VISIBILITY;
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

// Shows/hides every element flagged data-item-field="url|price|target-month|
// recurrence|quantity|urgent" (the create-item form's inputs, the edit
// modal's fields, and the financial summary bar) according to
// fieldVisibilityFor(type) — see its comment above for which fields each
// list type shows. Also swaps the create form's title placeholder to a
// custom-list-flavored hint ("Prénom, idée, note...") so the single
// remaining input reads as "add a note" rather than "add an item".
function applyListTypeVisibility(type) {
  const visibility = fieldVisibilityFor(type);
  ensureTargetMonthOptions();
  for (const el of document.querySelectorAll('[data-item-field="url"]')) el.hidden = !visibility.url;
  for (const el of document.querySelectorAll('[data-item-field="price"]')) el.hidden = !visibility.price;
  for (const el of document.querySelectorAll('[data-item-field="target-month"]')) el.hidden = !visibility.targetMonth;
  for (const el of document.querySelectorAll('[data-item-field="recurrence"]')) el.hidden = !visibility.recurrence;
  for (const el of document.querySelectorAll('[data-item-field="quantity"]')) el.hidden = !visibility.quantity;
  for (const el of document.querySelectorAll('[data-item-field="urgent"]')) el.hidden = !visibility.urgent;
  listEls.itemTitle.placeholder = t(type === 'custom' ? 'items.titlePlaceholderCustom' : 'items.titlePlaceholder');

  // A `custom` list (freeform names/ideas/notes) shows none of url/price/
  // target-month/recurrence/quantity/urgent — there is nothing left for the
  // quick-add bar's [⚙️] toggle to reveal, so it's hidden outright rather
  // than opening onto an empty panel.
  const advanced = hasAdvancedFields(visibility);
  listEls.quickAddToggle.hidden = !advanced;
  if (!advanced) setQuickAddAdvancedExpanded(false);
}

function hasAdvancedFields(visibility) {
  return visibility.url || visibility.price || visibility.targetMonth || visibility.recurrence || visibility.quantity || visibility.urgent;
}

// Expands/collapses the quick-add bar's advanced panel (URL/quantity/price/
// target month/recurrence/urgent) — collapsed is the default "épuré" state;
// expanding it is what the [⚙️] toggle and the FAB's add-item sheet do.
function setQuickAddAdvancedExpanded(expanded) {
  listEls.quickAddAdvanced.hidden = !expanded;
  listEls.quickAddToggle.setAttribute('aria-expanded', String(expanded));
  listEls.quickAddToggle.classList.toggle('bg-sky-500/10', expanded);
  listEls.quickAddToggle.classList.toggle('text-sky-600', expanded);
  listEls.quickAddToggle.classList.toggle('dark:text-sky-400', expanded);
}

listEls.quickAddToggle.addEventListener('click', () => {
  setQuickAddAdvancedExpanded(listEls.quickAddAdvanced.hidden);
});

// The FAB (#add-item-fab, mobile only — see its md:hidden class in
// index.html) opens #create-item-form as a bottom sheet instead of a second,
// duplicated form: openAddItemSheet/closeAddItemSheet simply relocate the
// one real <form> (and its already-attached listeners) between its normal
// inline slot (#create-item-form-anchor) and the sheet's body — plain
// `appendChild` on an already-attached node moves it rather than cloning it,
// so nothing about the submit handler below needs to know or care where the
// form currently lives.
function openAddItemSheet() {
  const visibility = fieldVisibilityFor(state.currentList && state.currentList.type);
  setQuickAddAdvancedExpanded(hasAdvancedFields(visibility));
  listEls.addItemSheetBody.appendChild(listEls.createItemForm);
  listEls.addItemSheet.hidden = false;
  document.body.classList.add('overflow-hidden');
  listEls.itemTitle.focus();
}

function closeAddItemSheet() {
  listEls.createItemFormAnchor.appendChild(listEls.createItemForm);
  listEls.addItemSheet.hidden = true;
  document.body.classList.remove('overflow-hidden');
  setQuickAddAdvancedExpanded(false);
}

listEls.addItemFab.addEventListener('click', openAddItemSheet);
listEls.closeAddItemSheetButton.addEventListener('click', closeAddItemSheet);
listEls.addItemSheet.addEventListener('click', (event) => {
  if (event.target === listEls.addItemSheet) closeAddItemSheet();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !listEls.addItemSheet.hidden) closeAddItemSheet();
});

// Recomputes the financial summary bar directly from state.currentList.items
// — the same array toggleDone/removeItem/create/edit mutate optimistically —
// so it stays in sync whether the underlying change came from the network,
// an optimistic local edit, or the offline sync queue. `item.price` is a
// per-unit price, so every line contributes `price * quantity` — see
// lineTotal below, also used by buildItemRow's per-item price display so
// the row-level subtotals and this bar's total always agree.
function updateFinanceSummary(items) {
  let total = 0;
  let spent = 0;
  for (const item of items) {
    const line = lineTotal(item);
    total += line;
    if (item.done) spent += line;
  }
  listEls.financeTotal.textContent = formatEuro(total);
  listEls.financeTotalCompact.textContent = formatEuro(total);
  listEls.financeSpent.textContent = formatEuro(spent);
  listEls.financeRemaining.textContent = formatEuro(total - spent);
}

// #finance-summary is a plain <details> (same collapsible idiom as
// #done-section) so a tap on its header can free up the whole screen on a
// small phone without needing any bespoke JS toggle logic — only the user's
// open/closed preference needs persisting, since the element itself already
// remembers its own state across re-renders (renderItems never rebuilds it).
const FINANCE_SUMMARY_COLLAPSED_KEY = 'trakka:financeSummaryCollapsed';
listEls.financeSummary.open = localStorage.getItem(FINANCE_SUMMARY_COLLAPSED_KEY) !== 'true';
listEls.financeSummary.addEventListener('toggle', () => {
  localStorage.setItem(FINANCE_SUMMARY_COLLAPSED_KEY, String(!listEls.financeSummary.open));
});

// Prix Total Article = Prix Unitaire * Quantité. `item.quantity` is always
// >= 1 server-side (see internal/handlers/items.go), but an optimistic
// locally-built item or a stale cached one could momentarily lack it, so
// this falls back to 1 the same way the create-item form already does.
function lineTotal(item) {
  if (typeof item.price !== 'number') return 0;
  const quantity = item.quantity > 0 ? item.quantity : 1;
  return item.price * quantity;
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
  badge.className = 'flex shrink-0 items-center gap-1 rounded-full bg-slate-200/50 dark:bg-slate-700/40 px-2 py-0.5 text-xs font-medium text-slate-600 dark:text-slate-300';
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

// A discreet, non-interactive badge (🚨 + "Urgent") shown on an item flagged
// is_urgent, regardless of done state — mirrors buildRecurrenceBadge's shape
// but in the rose palette used everywhere else for "needs attention".
function buildUrgentBadge() {
  const badge = document.createElement('span');
  badge.className = 'flex shrink-0 items-center gap-1 rounded-full bg-rose-500/15 px-2 py-0.5 text-xs font-semibold text-rose-600 dark:text-rose-300';
  badge.setAttribute('aria-label', t('items.urgentBadgeAriaLabel'));
  const icon = document.createElement('span');
  icon.setAttribute('aria-hidden', 'true');
  icon.textContent = '🚨';
  const text = document.createElement('span');
  text.textContent = t('items.urgentBadgeLabel');
  badge.appendChild(icon);
  badge.appendChild(text);
  return badge;
}

// A discreet, non-interactive badge (⏳ + "Non synchronisé") shown on an
// item that either was itself created while offline and hasn't reached the
// server yet (isOfflineQueuedItem) or has some other write (an edit, a
// toggle, a delete) still sitting in the offline sync queue
// (hasPendingItemChanges, defined in app.js — see the "Network status +
// offline sync indicators" section there). animate-pulse (Tailwind's
// built-in utility, no custom CSS needed) gives it a gentle "still waiting"
// motion, same as buildPendingPriceBadge's own pending-price badge above.
function buildUnsyncedItemBadge(item) {
  const badge = document.createElement('span');
  badge.className =
    'flex shrink-0 animate-pulse items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-700 dark:text-amber-300';
  badge.setAttribute('aria-label', t('sync.itemBadgeAriaLabel', { title: item.title }));
  const icon = document.createElement('span');
  icon.setAttribute('aria-hidden', 'true');
  icon.textContent = '⏳';
  const text = document.createElement('span');
  text.textContent = t('sync.listBadgeLabel');
  badge.appendChild(icon);
  badge.appendChild(text);
  return badge;
}

function emptyItemsRow(message) {
  const li = document.createElement('li');
  li.className = 'rounded-xl border border-dashed border-slate-200 dark:border-slate-800 p-6 text-center text-sm text-slate-500';
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
  button.title = t('items.enlargeImageAriaLabel', { title: item.title });
  button.setAttribute('aria-label', t('items.enlargeImageAriaLabel', { title: item.title }));
  button.className =
    'block h-12 w-12 shrink-0 overflow-hidden rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 transition duration-150 hover:z-10 hover:scale-150 hover:shadow-lg focus-visible:z-10 focus-visible:scale-150';

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

// A small non-interactive line-number marker shown instead of the
// completion checkbox for list types with no "done" concept (currently only
// `custom` — see FIELD_VISIBILITY_BY_TYPE's `done` flag) — a freeform note
// like a name/idea isn't a task that gets checked off, so it gets a plain
// "1.", "2.", ... in the same 44px slot the checkbox would otherwise occupy,
// keeping row alignment identical between list types. Falls back to a bare
// bullet when no index is available (shouldn't normally happen, since every
// caller in renderItems passes one).
function buildLineMarker(index) {
  const span = document.createElement('span');
  span.className = 'flex h-11 w-11 shrink-0 items-center justify-center text-sm font-semibold text-slate-400 dark:text-slate-500';
  span.textContent = typeof index === 'number' ? `${index + 1}.` : '•';
  span.setAttribute('aria-hidden', 'true');
  return span;
}

// A small inline 🔗 icon-link, appended right after an item's title instead
// of the old standalone "lien ↗" pill badge — folding it into the title line
// keeps the row's one-glance hierarchy to "checkbox + thumbnail + title (+
// price)", with everything else (quantity, badges) pushed down to
// .item-card__secondary. event.stopPropagation() keeps a tap here from also
// triggering the title's own "open the actions sheet" handler below.
function buildInlineLinkIcon(item) {
  const link = document.createElement('a');
  link.href = item.url;
  link.target = '_blank';
  link.rel = 'noopener noreferrer';
  link.className = 'long-press-target shrink-0 text-sky-500 hover:text-sky-400';
  link.setAttribute('aria-label', t('items.openLinkAriaLabel', { title: item.title }));
  link.textContent = '🔗';
  link.addEventListener('click', (event) => event.stopPropagation());
  return link;
}

// The trailing, right-aligned price figure on an item's top row — the "Prix
// Total Article = Prix Unitaire × Quantité" line subtotal (see lineTotal),
// the auto-detected-price sparkle badge, and (only once quantity > 1) the
// per-unit price it was computed from. Falls back to a pending-price badge
// when there's a url but no price yet (offline-queued, or still being
// scraped server-side — see isOfflineQueuedItem/priceScrapePending). Returns
// null when there's nothing price-related to show at all (plain to-do/
// custom-list items), so buildItemRow can skip appending it entirely.
function buildPriceBlock(item) {
  const block = document.createElement('div');
  block.className = 'item-card__price flex shrink-0 flex-col items-end gap-0.5';

  if (item.price != null) {
    const priceRow = document.createElement('div');
    priceRow.className = 'flex items-center gap-1';

    const price = document.createElement('span');
    price.className = 'text-base font-semibold text-slate-900 dark:text-slate-100';
    price.textContent = formatEuro(lineTotal(item));
    priceRow.appendChild(price);

    // Auto-detected prices get a small clickable badge instead of a plain
    // label — clicking it opens the same edit modal as the actions sheet's
    // "Modifier" entry, so "modifier en un clic" just reuses the existing
    // edit flow rather than needing a dedicated inline editor.
    if (item.price_auto) {
      const autoBadge = document.createElement('button');
      autoBadge.type = 'button';
      autoBadge.title = t('items.autoPriceAriaLabel', { title: item.title });
      autoBadge.setAttribute('aria-label', t('items.autoPriceAriaLabel', { title: item.title }));
      autoBadge.className = 'flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-300 hover:bg-emerald-500/20';
      autoBadge.innerHTML = AUTO_PRICE_ICON_SVG;
      autoBadge.addEventListener('click', (event) => {
        event.stopPropagation();
        openEditItemModal(item);
      });
      priceRow.appendChild(autoBadge);
    }

    block.appendChild(priceRow);

    if (item.quantity > 1) {
      const unitPrice = document.createElement('span');
      unitPrice.className = 'text-xs text-slate-400 dark:text-slate-500';
      unitPrice.textContent = `(${formatEuro(item.price)}/u)`;
      block.appendChild(unitPrice);
    }
  } else if (item.url) {
    if (isOfflineQueuedItem(item)) {
      block.appendChild(buildPendingPriceBadge(t('items.priceSyncPending'), 'bg-amber-500/10 text-amber-700 dark:text-amber-300'));
    } else if (item.priceScrapePending) {
      block.appendChild(buildPendingPriceBadge(t('items.priceDetecting'), 'animate-pulse bg-sky-500/10 text-sky-600 dark:text-sky-300'));
    }
  }

  return block.children.length > 0 ? block : null;
}

// The secondary line shown under an item's title: the compact [-]/N/[+]
// quantity stepper (when the list type shows quantity at all — see
// fieldVisibilityFor) plus the month/recurrence/urgent badges. Indented via
// Tailwind's pl-[3.5rem] to align under the title rather than the checkbox —
// approximate (it doesn't account for a thumbnail's extra width) but close
// enough to read as "belongs to the title above it" either way. Returns null
// when there's nothing to show (a `custom`-list item has none of these),
// so buildItemRow can skip reserving an empty row for it.
function buildSecondaryRow(item, { showQuantity }) {
  const secondary = document.createElement('div');
  secondary.className = 'item-card__secondary flex flex-wrap items-center gap-2 pl-[3.5rem] text-sm';

  if (showQuantity) {
    secondary.appendChild(buildQuantityStepper(item));
  }

  // Only shown once an item has actually been scheduled via the edit
  // modal's "Mois prévu" field — monthLabel is defined in planning.js,
  // safe to call here since all script tags finish loading and defining
  // their top-level functions before any rendering actually runs.
  if (item.target_month) {
    const monthBadge = document.createElement('span');
    monthBadge.className = 'shrink-0 rounded-full bg-violet-500/10 px-2 py-0.5 text-xs font-medium text-violet-700 dark:text-violet-300';
    monthBadge.textContent = monthLabel(item.target_month, 'short');
    secondary.appendChild(monthBadge);
  }

  if (item.recurrence_rule) {
    const recurrenceBadge = buildRecurrenceBadge(item);
    if (recurrenceBadge) secondary.appendChild(recurrenceBadge);
  }

  if (item.is_urgent) {
    secondary.appendChild(buildUrgentBadge());
  }

  // hasPendingItemChanges is defined in app.js; isOfflineQueuedItem (above)
  // already covers an item still carrying its temp-item-* id — this also
  // catches an already-synced item with some other write (an edit, a
  // toggle, a delete) still sitting in the offline sync queue.
  if (hasPendingItemChanges(item.id) || isOfflineQueuedItem(item)) {
    secondary.appendChild(buildUnsyncedItemBadge(item));
  }

  return secondary.children.length > 0 ? secondary : null;
}

// buildItemRow lays out one card as a top .item-card__row (checkbox/marker +
// optional thumbnail + title + inline 🔗 icon on the left, the price block,
// then edit/delete/kebab on the right) plus, only when there's something to
// show, a .item-card__secondary line underneath it (quantity stepper +
// badges) — the same shape at every breakpoint, Todoist/Notion-style, rather
// than the single dense desktop row this used to reflow via CSS `order` on a
// narrow screen. `.item-card__actions` (✏️/🗑️) and `.item-card__kebab` (⋮,
// opening #item-actions-sheet) are both always built; base.css's `hidden
// md:flex`/`flex md:hidden` decide which one is actually visible per
// breakpoint — see the comment above that block for why.
function buildItemRow(item, { showCheckbox = true, index, showQuantity = true } = {}) {
  const li = document.createElement('li');
  // An unfinished urgent item gets a distinctive rose border so it stands
  // out at a glance in the (already sorted-to-top, see renderItems) active
  // list — a completed urgent item just reverts to the normal styling since
  // it no longer needs attention.
  li.className =
    item.is_urgent && !item.done
      ? 'item-card flex flex-col gap-1.5 rounded-2xl border-2 border-rose-500/60 bg-rose-500/5 p-3 shadow-sm'
      : 'item-card flex flex-col gap-1.5 rounded-2xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30 p-3 shadow-sm';

  const row = document.createElement('div');
  row.className = 'item-card__row flex items-center gap-2';

  const lead = document.createElement('div');
  lead.className = 'item-card__lead flex min-w-0 flex-1 items-center gap-3';

  if (showCheckbox) {
    const checkboxLabel = document.createElement('label');
    checkboxLabel.className = 'flex h-11 w-11 shrink-0 cursor-pointer items-center justify-center';
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.checked = item.done;
    checkbox.className = 'h-5 w-5 rounded-full border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 text-sky-500 focus:ring-sky-500/40';
    // Reuses urgent.js's own key: identical text/purpose ("mark X as done"),
    // no need for a second near-duplicate.
    checkbox.setAttribute('aria-label', t('urgent.markDoneAriaLabel', { title: item.title }));
    checkbox.addEventListener('change', () => toggleDone(item));
    checkboxLabel.appendChild(checkbox);
    lead.appendChild(checkboxLabel);
  } else {
    lead.appendChild(buildLineMarker(index));
  }

  // A thumbnail only ever appears when a product image was actually found
  // (see internal/scraper's og:image/JSON-LD/twitter:image lookup) — no
  // placeholder box is reserved for items without one, so plain to-do items
  // and shopping items with no matched image just skip straight to the
  // title, unchanged from before this feature existed.
  if (item.image_url && isSafeHttpUrl(item.image_url)) {
    lead.appendChild(buildItemThumbnail(item));
  }

  const title = document.createElement('span');
  // `truncate` (Tailwind's overflow-hidden/text-overflow-ellipsis/
  // whitespace-nowrap shorthand) needs a `min-w-0` flex ancestor to actually
  // clip instead of forcing the row wider — `lead` above provides that — so
  // a long title on a narrow screen never pushes the price/actions groups
  // out of the card instead of just eliding. A tap on the title itself opens
  // the same item-actions sheet as the [⋮] kebab button, the mobile-first
  // "tap the item" entry point the pencil/trash buttons already cover on
  // wider screens.
  title.className = item.done
    ? 'min-w-0 flex-1 cursor-pointer truncate text-base font-semibold text-slate-500 line-through opacity-60'
    : 'min-w-0 flex-1 cursor-pointer truncate text-base font-semibold text-slate-900 dark:text-slate-100';
  // When the stepper below is shown, it's the canonical place quantity is
  // displayed/edited, so the title stays plain; otherwise (custom lists,
  // where quantity is hidden entirely) fall back to the old "title × N"
  // form, still used as a compact read-only summary in urgent.js/planning.js.
  title.textContent = !showQuantity && item.quantity > 1 ? `${item.title} × ${item.quantity}` : item.title;
  title.addEventListener('click', () => openItemActionsSheet(item));
  lead.appendChild(title);

  const hasLink = Boolean(item.url && isSafeHttpUrl(item.url));
  if (hasLink) {
    const linkIcon = buildInlineLinkIcon(item);
    lead.appendChild(linkIcon);
    // A long press (~500ms touch hold, desktop mouse unaffected — see
    // attachLongPress) on either the title or the link icon itself opens
    // the very same #item-actions-sheet a short tap does, just pre-focused
    // on its Ouvrir/Copier/Partager link group — the "appui long sur
    // l'item ou le lien" gesture, without a second sheet to keep in sync.
    attachLongPress(title, () => openItemActionsSheet(item, { focusLink: true }));
    attachLongPress(linkIcon, () => openItemActionsSheet(item, { focusLink: true }));
  }

  row.appendChild(lead);

  const priceBlock = buildPriceBlock(item);
  if (priceBlock) row.appendChild(priceBlock);

  const actions = document.createElement('div');
  actions.className = 'item-card__actions hidden shrink-0 items-center gap-1 md:flex';

  const editBtn = document.createElement('button');
  editBtn.type = 'button';
  editBtn.setAttribute('aria-label', t('items.editItemAriaLabel', { title: item.title }));
  editBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-sky-500/10 hover:text-sky-600 dark:hover:text-sky-400';
  editBtn.innerHTML = PENCIL_ICON_SVG;
  editBtn.addEventListener('click', () => openEditItemModal(item));
  actions.appendChild(editBtn);

  const deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.setAttribute('aria-label', t('items.deleteItemAriaLabel', { title: item.title }));
  deleteBtn.className = 'flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400';
  deleteBtn.innerHTML = TRASH_ICON_SVG;
  deleteBtn.addEventListener('click', () => removeItem(item));
  actions.appendChild(deleteBtn);

  row.appendChild(actions);

  const kebabBtn = document.createElement('button');
  kebabBtn.type = 'button';
  kebabBtn.setAttribute('aria-label', t('items.moreActionsAriaLabel', { title: item.title }));
  kebabBtn.className = 'item-card__kebab flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-700 md:hidden';
  kebabBtn.innerHTML = KEBAB_ICON_SVG;
  kebabBtn.addEventListener('click', () => openItemActionsSheet(item));
  row.appendChild(kebabBtn);

  li.appendChild(row);

  const secondary = buildSecondaryRow(item, { showQuantity });
  if (secondary) li.appendChild(secondary);

  // attachItemSwipeGestures is defined in gestures.js — swipe right to
  // toggle done (only when this list type actually shows the checkbox at
  // all), swipe left to delete, both reusing toggleDone/removeItem exactly
  // as the checkbox/trash button above already do. Touch-only and a no-op
  // on a device with no touchscreen — see that file's own header comment.
  attachItemSwipeGestures(li, item, { canToggleDone: showCheckbox });

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

  // isReorderModeActive/renderReorderList are defined in reorder.js, loaded
  // after this file — while the "⇅ Réordonner" mode is active, it fully
  // owns rendering #items-active instead of the active/done split below.
  if (isReorderModeActive()) {
    renderReorderList();
    return;
  }

  // listIcon is defined in app.js — a list's own icon, falling back to a
  // fixed icon for its type, same as every list card on the dashboard.
  listEls.itemsHeading.textContent = `${listIcon(list)} ${list.name} (${typeLabel(list.type)})`;
  applyListTypeVisibility(list.type);

  // hasPendingListChanges is defined in app.js — see the "Network status +
  // offline sync indicators" section there. Re-checked on every render
  // (this function already re-runs on every mutation, and on every
  // trakka-sync-status broadcast via repaintPendingChangeIndicators), so the
  // dot appears/clears in step with the offline queue without a dedicated
  // refresh path of its own.
  const listHasPendingChanges = hasPendingListChanges(list.id);
  listEls.listSyncIndicator.hidden = !listHasPendingChanges;
  listEls.listSyncIndicator.title = listHasPendingChanges ? t('sync.listBadgeAriaLabel', { name: list.name }) : '';

  // Items mid-undo-grace-period (see removeItem below) stay in
  // state.currentList.items — so undo can just clear the flag and re-render
  // with nothing else to reconstruct — but are filtered out of every view
  // (including the finance summary) as if already gone.
  const items = (list.items || []).filter((item) => !item.pendingDelete);
  // remainingItemsLabel is defined in app.js — the same "unchecked items,
  // not total items" count shown on this list's own dashboard/Espaces card,
  // kept in sync here since every mutation (toggleDone, add, delete, the
  // swipe gestures in gestures.js) re-runs this whole function.
  listEls.itemsRemainingCount.textContent = remainingItemsLabel(items);
  // Array.prototype.sort is stable, so this only ever moves unfinished
  // urgent items ahead of everything else — items within each group keep
  // their existing relative order (position/id) instead of being reshuffled.
  const active = items.filter((item) => !item.done).sort((a, b) => (b.is_urgent ? 1 : 0) - (a.is_urgent ? 1 : 0));
  const done = items.filter((item) => item.done);

  const visibility = fieldVisibilityFor(list.type);
  if (visibility.price) updateFinanceSummary(items);

  listEls.itemsActive.replaceChildren();
  if (items.length === 0) {
    listEls.itemsActive.appendChild(emptyItemsRow(t('items.emptyList')));
  } else if (active.length === 0) {
    listEls.itemsActive.appendChild(emptyItemsRow(t('items.allDone')));
  } else {
    active.forEach((item, index) => {
      listEls.itemsActive.appendChild(buildItemRow(item, { showCheckbox: visibility.done, index, showQuantity: visibility.quantity }));
    });
  }

  listEls.itemsDone.replaceChildren();
  // Always a checkbox here, regardless of visibility.done: a custom list's
  // create/edit form never lets an item become done in the first place (the
  // checkbox is hidden), so any item that does show up in this "done"
  // bucket only got there some other way (e.g. the list's type was changed
  // after the item was already completed) — it still needs a way back to
  // active, which only the checkbox provides.
  for (const item of done) listEls.itemsDone.appendChild(buildItemRow(item, { showCheckbox: true, showQuantity: visibility.quantity }));
  listEls.doneSummaryLabel.textContent = t('items.doneCount', { count: done.length });
  listEls.doneSection.hidden = done.length === 0;

  // updateReorderButtonVisibility is defined in reorder.js — re-evaluated on
  // every normal render so the "⇅ Réordonner" button reflects the current
  // item count/sync state (e.g. it appears once a temp-item-* id turns into
  // a real one after the offline queue flushes).
  updateReorderButtonVisibility();
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
  // exitReorderModeIfActive is defined in reorder.js — state.currentList is
  // about to be fully replaced below, which would leave an in-progress
  // reorder draft pointing at stale item objects.
  exitReorderModeIfActive();
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
    } else if (!isNetworkError(err)) {
      showError(err.message);
    }
  }
}

// opts.silent suppresses the error banner on failure (a 404, or offline
// with nothing cached) — used only by restoreLastView (app.js) when
// reopening a list from a previous session, since a list that's since been
// deleted or made inaccessible shouldn't greet the user with an error the
// moment the app launches; every ordinary caller (clicking a list card)
// leaves it false and keeps today's behavior exactly as before.
async function selectList(id, opts = {}) {
  const { silent = false } = opts;
  hideError();
  // exitReorderModeIfActive is defined in reorder.js — opening another list
  // (or reopening this one) mid-drag must not leave the new view stuck in
  // reorder mode.
  exitReorderModeIfActive();
  let list;
  try {
    list = await apiRequest(`/lists/${id}`);
  } catch (err) {
    // Offline, or a transient server error: open the list from the local
    // mirror instead of failing to open it at all.
    list = await cachedListDetail(id);
    if (!list) {
      if (!silent && !isNetworkError(err)) showError(err.message);
      return;
    }
  }
  state.currentListId = id;
  state.currentList = list;
  els.listsSection.hidden = true;
  listEls.itemsSection.hidden = false;
  renderItems();
  // saveLastView is defined in app.js — see the "keep last page on launch"
  // preference there.
  saveLastView({ type: 'list', id });
}

function showDashboard() {
  // exitReorderModeIfActive is defined in reorder.js — leaving the list
  // detail view mid-drag must not leave reorder mode's chrome (the bottom
  // action bar, the hidden quick-add bar/FAB) stuck on for whatever's opened
  // next.
  exitReorderModeIfActive();
  state.currentListId = null;
  state.currentList = null;
  listEls.itemsSection.hidden = true;
  els.listsSection.hidden = false;
  // activeTab is defined in planning.js; saveLastView in app.js — see the
  // "keep last page on launch" preference there.
  saveLastView({ type: 'tab', tab: activeTab });
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

// Same coalescing idea as pendingToggles above, minus the undo toast — a
// quantity bump from the [-]/[+] stepper is a lightweight, instantly-
// committed action, not something a user expects to "undo" the way a
// done-toggle or delete is. Tracks the last server-confirmed quantity plus
// an in-flight request id per item, so a slower response from an earlier
// click can never clobber a value the user has since moved past by
// clicking again.
const pendingQuantityUpdates = new Map();

function changeQuantity(item, newQuantity) {
  if (!Number.isFinite(newQuantity) || newQuantity < 1) return;
  hideError();

  const pending = pendingQuantityUpdates.get(item);
  const committedQuantity = pending ? pending.committedQuantity : item.quantity;
  const requestId = Symbol('quantity-update');

  item.quantity = newQuantity;
  pendingQuantityUpdates.set(item, { committedQuantity, requestId });
  renderItems();

  (async () => {
    try {
      const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({ quantity: newQuantity }) });
      if (pendingQuantityUpdates.get(item)?.requestId !== requestId) return; // superseded by a newer click
      Object.assign(item, updated);
      pendingQuantityUpdates.delete(item);
    } catch (err) {
      if (pendingQuantityUpdates.get(item)?.requestId !== requestId) return;
      item.quantity = committedQuantity;
      pendingQuantityUpdates.delete(item);
      showError(err.message);
    } finally {
      renderItems();
    }
    await refreshPendingBadge();
  })();
}

// Same coalescing pattern as pendingQuantityUpdates/changeQuantity above —
// toggling urgency from the item-actions sheet is a lightweight, instantly-
// committed action like the quantity stepper, not something that needs an
// undo toast the way a done-toggle or delete does.
const pendingUrgentUpdates = new Map();

function toggleUrgent(item) {
  hideError();

  const pending = pendingUrgentUpdates.get(item);
  const committedUrgent = pending ? pending.committedUrgent : item.is_urgent;
  const newUrgent = !item.is_urgent;
  const requestId = Symbol('urgent-update');

  item.is_urgent = newUrgent;
  pendingUrgentUpdates.set(item, { committedUrgent, requestId });
  renderItems();

  (async () => {
    try {
      const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({ is_urgent: newUrgent }) });
      if (pendingUrgentUpdates.get(item)?.requestId !== requestId) return; // superseded by a newer toggle
      Object.assign(item, updated);
      pendingUrgentUpdates.delete(item);
    } catch (err) {
      if (pendingUrgentUpdates.get(item)?.requestId !== requestId) return;
      item.is_urgent = committedUrgent;
      pendingUrgentUpdates.delete(item);
      showError(err.message);
    } finally {
      renderItems();
    }
    await refreshPendingBadge();
  })();
}

// [-]/[+] buttons plus an inline-editable number field, so an item's
// quantity can be changed directly from the list view without opening the
// edit modal — wired to changeQuantity above for the optimistic-PATCH-with-
// rollback behavior. Only rendered when the list's type shows quantity at
// all (see FIELD_VISIBILITY_BY_TYPE's `quantity` flag) — buildItemRow gates
// this the same way it already gates the checkbox/line-marker choice.
function buildQuantityStepper(item) {
  const quantity = item.quantity > 0 ? item.quantity : 1;

  const wrap = document.createElement('div');
  wrap.className =
    'flex shrink-0 items-center gap-0.5 rounded-full border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-0.5';

  const decrementBtn = document.createElement('button');
  decrementBtn.type = 'button';
  decrementBtn.innerHTML = MINUS_ICON_SVG;
  decrementBtn.disabled = quantity <= 1;
  decrementBtn.setAttribute('aria-label', t('items.decreaseQuantityAriaLabel', { title: item.title }));
  decrementBtn.className =
    'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-slate-500 dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-700 disabled:opacity-30 disabled:hover:bg-transparent';
  decrementBtn.addEventListener('click', () => changeQuantity(item, quantity - 1));

  const input = document.createElement('input');
  input.type = 'number';
  input.min = '1';
  input.inputMode = 'numeric';
  input.value = String(quantity);
  input.setAttribute('aria-label', t('items.quantityForItemAriaLabel', { title: item.title }));
  input.className =
    'w-10 shrink-0 rounded border-0 bg-transparent text-center text-sm font-medium text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-sky-500/40';
  input.addEventListener('change', () => {
    const parsed = Number.parseInt(input.value, 10);
    if (Number.isFinite(parsed) && parsed >= 1) {
      changeQuantity(item, parsed);
    } else {
      input.value = String(quantity); // reject an invalid/empty value, restore what was there
    }
  });
  input.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') input.blur(); // commits via the 'change' handler above
  });
  input.addEventListener('click', (event) => event.stopPropagation());

  const incrementBtn = document.createElement('button');
  incrementBtn.type = 'button';
  incrementBtn.innerHTML = PLUS_ICON_SVG;
  incrementBtn.setAttribute('aria-label', t('items.increaseQuantityAriaLabel', { title: item.title }));
  incrementBtn.className =
    'flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-slate-500 dark:text-slate-400 hover:bg-slate-200 dark:hover:bg-slate-700';
  incrementBtn.addEventListener('click', () => changeQuantity(item, quantity + 1));

  wrap.append(decrementBtn, input, incrementBtn);
  return wrap;
}

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
        // Still on this list (or some other one): a plain local re-render is
        // enough, and — critically — must not be replaced by a network
        // refetch here, since that could clobber a *different* item's still-
        // pending optimistic toggle (its own commit hasn't landed yet
        // either) with the server's not-yet-updated view of it. Only once
        // the user has actually left the list entirely (state.currentListId
        // === null, e.g. back to the dashboard) is there anything else on
        // screen that this commit could have made stale — a dashboard card's
        // count, an Urgent/Planning/Spaces/Shared tab — and refreshVisibleView
        // (app.js) is what refreshes whichever of those is actually visible.
        // Without this, checking an item off and navigating away inside the
        // 5s undo window left the previous view's counters stuck until a
        // full page reload, since nothing else ever re-ran its fetch once
        // this deferred PATCH finally went out.
        if (state.currentListId !== null) {
          renderItems();
        } else {
          refreshVisibleView();
        }
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

  // Captured now, at call time, rather than re-read as state.currentList
  // inside onCommit below: removeItem is only ever invoked from a row
  // rendered for the currently open list, so this is guaranteed to be that
  // same list's object — whereas by the time the deferred commit actually
  // fires (5s later), the user may have navigated to a different list or
  // away from any list entirely, at which point state.currentList would no
  // longer be this item's list (or would be null), and splicing into
  // whatever it happens to hold then would either corrupt an unrelated
  // list's items or throw outright.
  const list = state.currentList;

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
      const items = list.items;
      const index = items.indexOf(item);
      if (index !== -1) items.splice(index, 1);

      try {
        await apiRequest(`/items/${item.id}`, { method: 'DELETE' });
      } catch (err) {
        if (index !== -1) items.splice(index, 0, item);
        delete item.pendingDelete;
        showError(err.message);
      }
      // Same reasoning as toggleDone's onCommit above: only fall back to
      // refreshVisibleView() (a real refetch) when the list this item
      // belonged to isn't the one currently on screen any more — otherwise
      // a plain local re-render already reflects the deletion without
      // risking clobbering some other item's still-pending optimistic
      // change with stale server data.
      if (state.currentList === list) {
        renderItems();
      } else {
        refreshVisibleView();
      }
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
  if (!title) return;

  // URL, price, target month, recurrence, quantity and urgency only apply to
  // list types that show them (see fieldVisibilityFor) — gated on the list's
  // actual type rather than the inputs' `hidden` state, so a stale value
  // left over from a previously-open list of a different type (the form
  // isn't reset on selectList) can never leak into this one — e.g. a
  // quantity typed while a shopping list's form was open must not survive
  // into a `custom` note submitted after switching lists without resetting.
  const visibility = fieldVisibilityFor(state.currentList.type);
  const quantity = visibility.quantity ? Number.parseInt(listEls.itemQuantity.value, 10) || 1 : 1;
  const url = visibility.url ? listEls.itemUrl.value.trim() : '';
  let price = null;
  let targetMonth = '';
  if (visibility.price) {
    const raw = listEls.itemPrice.value.trim();
    if (raw !== '') {
      const parsed = Number.parseFloat(raw);
      if (Number.isFinite(parsed) && parsed >= 0) price = parsed;
    }
  }
  if (visibility.targetMonth) targetMonth = listEls.itemTargetMonth.value;

  const recurrenceRule = visibility.recurrence ? listEls.itemRecurrence.value : '';
  const isUrgent = visibility.urgent ? listEls.itemUrgent.checked : false;

  const payload = { list_id: state.currentListId, title, quantity };
  if (url) payload.url = url;
  if (price !== null) payload.price = price;
  if (targetMonth) payload.target_month = targetMonth;
  if (recurrenceRule) payload.recurrence_rule = recurrenceRule;
  if (isUrgent) payload.is_urgent = true;

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
    is_urgent: isUrgent,
    done: false,
    position: 0,
  };
  // Captured before the form/sheet state changes below: an add made from the
  // FAB's bottom sheet closes it afterward (the "add one item" mobile flow
  // this sheet exists for — see openAddItemSheet), while an add made from
  // the always-visible inline quick-add bar just collapses its advanced
  // panel back to the default "épuré" state for the next entry.
  const sheetWasOpen = !listEls.addItemSheet.hidden;

  state.currentList.items = [...(state.currentList.items || []), optimisticItem];
  renderItems();
  listEls.createItemForm.reset();
  listEls.itemQuantity.value = '1';
  if (sheetWasOpen) {
    closeAddItemSheet();
  } else {
    setQuickAddAdvancedExpanded(false);
    listEls.itemTitle.focus();
  }

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

// openListModal is defined in app.js, resolved lazily the same way every
// other cross-file call in this file already is.
listEls.editListButton.addEventListener('click', () => {
  if (state.currentList) openListModal(state.currentList);
});

// openShareModal is defined in shares.js, resolved lazily the same way.
listEls.shareListButton.addEventListener('click', () => {
  if (state.currentList) openShareModal({ kind: 'list', id: state.currentList.id, name: state.currentList.name });
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
  listEls.editItemUrgent.checked = Boolean(item.is_urgent);
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

// ---------------------------------------------------------------------------
// Item actions bottom sheet — the single, mobile-first replacement for the
// ✏️/🗑️ buttons shown directly on a card on wider screens (see buildItemRow):
// opened by a tap on an item's title, its [⋮] kebab button, or a long press
// on the title/link icon, it offers Modifier/[Ouvrir·Copier·Partager le
// lien]/Basculer en urgent/Supprimer for whichever item was acted on,
// tracked in the module-level `itemActionsSheetItem` declared above. Every
// entry point funnels into this one sheet — there is no separate link-only
// sheet to keep in sync with it.
// ---------------------------------------------------------------------------

// `focusLink` moves keyboard focus straight to "Ouvrir le lien" once the
// sheet is shown — used by the long-press gesture (see attachLongPress in
// buildItemRow) so that gesture lands the user directly on the link actions
// rather than at the top of the full action list. Only meaningful when the
// item actually has a link, which is the only case attachLongPress ever
// requests it for.
function openItemActionsSheet(item, { focusLink = false } = {}) {
  itemActionsSheetItem = item;
  listEls.itemActionsSheetTitle.textContent = item.title;
  const hasLink = Boolean(item.url && isSafeHttpUrl(item.url));
  listEls.itemActionsLinkGroup.hidden = !hasLink;
  listEls.itemActionsUrgentLabel.textContent = t(item.is_urgent ? 'modals.itemActions.unmarkUrgent' : 'modals.itemActions.markUrgent');
  listEls.itemActionsSheet.hidden = false;
  document.body.classList.add('overflow-hidden');
  if (focusLink && hasLink) {
    listEls.itemActionsOpenLinkButton.focus();
  }
}

function closeItemActionsSheet() {
  itemActionsSheetItem = null;
  listEls.itemActionsSheet.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

listEls.closeItemActionsSheetButton.addEventListener('click', closeItemActionsSheet);
listEls.itemActionsSheet.addEventListener('click', (event) => {
  if (event.target === listEls.itemActionsSheet) closeItemActionsSheet();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !listEls.itemActionsSheet.hidden) closeItemActionsSheet();
});

listEls.itemActionsEditButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) openEditItemModal(item);
});

listEls.itemActionsOpenLinkButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) openLink(item.url);
});

listEls.itemActionsCopyLinkButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) copyLink(item.url);
});

listEls.itemActionsShareLinkButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) shareLink(item);
});

listEls.itemActionsUrgentButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) toggleUrgent(item);
});

listEls.itemActionsDeleteButton.addEventListener('click', () => {
  const item = itemActionsSheetItem;
  closeItemActionsSheet();
  if (item) removeItem(item);
});

listEls.editItemForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  if (!editingItem) return;

  const title = listEls.editItemTitle.value.trim();
  const url = listEls.editItemUrl.value.trim();
  if (!title) return;

  // `price`/`target_month`/`recurrence_rule` are only ever included for list
  // types that show them (see fieldVisibilityFor) — when included, an empty
  // field sends an explicit "clear" value (`null` for price, `""` for
  // target_month/recurrence_rule) rather than omitting the key (leave
  // unchanged), see the PATCH handler's field-presence handling in
  // internal/handlers/items.go. is_urgent, unlike those three, applies to
  // every list type, so it's always included in the payload.
  const visibility = fieldVisibilityFor(state.currentList && state.currentList.type);
  const payload = {
    title,
    url,
    is_urgent: listEls.editItemUrgent.checked,
  };
  if (visibility.price) {
    const raw = listEls.editItemPrice.value.trim();
    if (raw === '') {
      payload.price = null;
    } else {
      const parsed = Number.parseFloat(raw);
      if (!Number.isFinite(parsed) || parsed < 0) {
        showError(t('items.priceValidationError'));
        return;
      }
      payload.price = parsed;
    }
  }
  if (visibility.targetMonth) payload.target_month = listEls.editItemTargetMonth.value;
  if (visibility.recurrence) payload.recurrence_rule = listEls.editItemRecurrence.value;

  const item = editingItem;
  const previous = {
    title: item.title,
    url: item.url,
    price: item.price,
    target_month: item.target_month,
    recurrence_rule: item.recurrence_rule,
    is_urgent: item.is_urgent,
  };
  item.title = title;
  item.url = url || null;
  if ('price' in payload) item.price = payload.price;
  if ('target_month' in payload) item.target_month = payload.target_month || null;
  if ('recurrence_rule' in payload) item.recurrence_rule = payload.recurrence_rule || null;
  item.is_urgent = payload.is_urgent;
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
