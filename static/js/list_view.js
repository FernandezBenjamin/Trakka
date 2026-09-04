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
  financeTotalCollapsed: document.getElementById('finance-total-collapsed'),
  financeSpentCollapsed: document.getElementById('finance-spent-collapsed'),
  financeRemainingCollapsed: document.getElementById('finance-remaining-collapsed'),
  createItemFormAnchor: document.getElementById('create-item-form-anchor'),
  createItemForm: document.getElementById('create-item-form'),
  itemTitle: document.getElementById('item-title'),
  quickAddToggle: document.getElementById('quick-add-toggle'),
  quickAddAdvanced: document.getElementById('quick-add-advanced'),
  itemUrl: document.getElementById('item-url'),
  itemQuantity: document.getElementById('item-quantity'),
  itemPrice: document.getElementById('item-price'),
  itemTargetPrice: document.getElementById('item-target-price'),
  itemTargetMonth: document.getElementById('item-target-month'),
  itemRecurrence: document.getElementById('item-recurrence'),
  itemUrgent: document.getElementById('item-urgent'),
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
  editItemTargetPrice: document.getElementById('edit-item-target-price'),
  editItemTargetMonth: document.getElementById('edit-item-target-month'),
  editItemRecurrence: document.getElementById('edit-item-recurrence'),
  editItemUrgent: document.getElementById('edit-item-urgent'),
  itemActionsSheet: document.getElementById('item-actions-sheet'),
  itemActionsSheetTitle: document.getElementById('item-actions-sheet-title'),
  itemActionsSheetMeta: document.getElementById('item-actions-sheet-meta'),
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

// A small non-interactive icon-only indicator for a price that isn't known
// yet — either because the item is still offline-queued (waiting on
// connectivity) or because the server's bounded synchronous lookup (see
// scrapePrice in internal/handlers/scrape.go) didn't finish in time and is
// still running in the background (see scheduleAutoPriceRefresh below). Kept
// to a single glyph with a title/aria-label tooltip, not a text pill, so a
// pending price never grows the item's single-line row.
function buildPendingPriceIcon(glyph, label, extraClass) {
  const span = document.createElement('span');
  span.className = `flex h-5 w-5 shrink-0 items-center justify-center text-sm ${extraClass}`;
  span.textContent = glyph;
  span.title = label;
  span.setAttribute('aria-label', label);
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
// expanding it is what the [⚙️] toggle does. The bar itself (#create-item-
// form-anchor) is pinned to the bottom of the viewport on mobile and sits
// in its original in-flow spot above the item list at `md:` and up — see
// its responsive classes in index.html and the safe-area/#items-section
// padding in base.css — so there is exactly one form/one set of listeners
// regardless of viewport width, nothing to relocate between two DOM slots.
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
  const remaining = total - spent;
  listEls.financeTotal.textContent = formatEuro(total);
  listEls.financeTotalCompact.textContent = formatEuro(total);
  listEls.financeSpent.textContent = formatEuro(spent);
  listEls.financeRemaining.textContent = formatEuro(remaining);
  // Collapsed-header compact figures (see the #finance-summary markup in
  // index.html) mirror the same three numbers so the accordion stays
  // informative without needing to be expanded. Every figure's color is a
  // static --tk-money-* token (tokens.css) — no JS ever sets a color here,
  // regardless of remaining's sign.
  listEls.financeTotalCollapsed.textContent = formatEuro(total);
  listEls.financeSpentCollapsed.textContent = formatEuro(spent);
  listEls.financeRemainingCollapsed.textContent = formatEuro(remaining);
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

// Mirrors internal/handlers.priceAlertCondition exactly: whether item's own
// user-set "notify me when the price drops" threshold currently holds —
// alerting opted in, a threshold set, a price present, and that price at
// or below the threshold. Used both to decide whether to render
// buildPriceAlertIcon below and, in app.js/reorder.js's dashboard/Espaces
// card badges, whether to show the same highlighted state there.
function priceAlertCondition(item) {
  return Boolean(item.alert_on_price_drop) && item.target_price != null && item.price != null && item.price <= item.target_price;
}

// A single eye-catching 🔥 glyph shown once an item's price has reached the
// threshold set via its "Déclencher une alerte si le prix descend en
// dessous de" field — an icon rather than the old full-text "Bonne affaire"
// pill, so it reads at a glance without competing for width on the item's
// single-line row; the full detail (both prices) is still available as a
// title/aria-label tooltip on hover/focus, and was already announced once,
// in full, via the in-app toast/push notification checkPriceDropAlert fires
// server-side the moment this condition first becomes true (see items.js's
// price_alert_triggered handling on create/update/patch).
function buildPriceAlertIcon(item) {
  const label = t('items.priceAlertBadge', { price: formatEuro(item.price), target: formatEuro(item.target_price) });
  const icon = document.createElement('span');
  icon.className = 'flex h-5 w-5 shrink-0 items-center justify-center text-sm';
  icon.textContent = '🔥';
  icon.title = label;
  icon.setAttribute('aria-label', label);
  return icon;
}

// Shows the in-app toast half of the "notification (toast + push)"
// requirement — the push half is sent server-side by
// internal/handlers.checkPriceDropAlert regardless of whether this tab is
// even open. `price_alert_triggered` (models.Item.PriceAlertTriggered) is a
// transient, response-only field set exactly once per genuine crossing —
// see its own doc comment — so this never re-fires on an unrelated edit or
// a plain GET. Called from the create-item form and edit-item form submit
// handlers, the only two places an item's price-mutating fields reach the
// server synchronously enough to read this back from the response.
function notifyPriceAlertIfTriggered(item) {
  if (!item || !item.price_alert_triggered) return;
  window.TrakkaToast?.success(t('items.priceAlertToast', { title: item.title, price: formatEuro(item.price), target: formatEuro(item.target_price) }));
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

// A discreet ⏳ dot (not a text pill — see the header comment on buildItemRow
// for why the single-line card only ever shows single-glyph indicators) for
// an item that either was itself created while offline and hasn't reached
// the server yet (isOfflineQueuedItem) or has some other write (an edit, a
// toggle, a delete) still sitting in the offline sync queue
// (hasPendingItemChanges, defined in app.js — see the "Network status +
// offline sync indicators" section there). animate-pulse (Tailwind's
// built-in utility, no custom CSS needed) gives it a gentle "still waiting"
// motion, same as buildPendingPriceIcon's own pending-price indicator above.
function buildUnsyncedDot(item) {
  const dot = document.createElement('span');
  dot.className = 'h-2 w-2 shrink-0 animate-pulse rounded-full bg-amber-500';
  dot.title = t('sync.itemBadgeAriaLabel', { title: item.title });
  dot.setAttribute('aria-label', dot.title);
  return dot;
}

// A same-size, empty stand-in for buildUnsyncedDot/buildInlineLinkIcon on a
// row where that particular glyph doesn't currently apply — appended instead
// of simply omitting the element, so every row reserves the same width for
// it either way. See PRICE_STATUS_SLOT_CLASS above for why an inconsistent
// per-row width here would misalign the stepper/price of every row after it.
function buildEmptySlot(sizeClasses) {
  const span = document.createElement('span');
  span.className = `${sizeClasses} shrink-0`;
  span.setAttribute('aria-hidden', 'true');
  return span;
}

function emptyItemsRow(message) {
  const li = document.createElement('li');
  li.className = 'rounded-xl border border-dashed border-slate-200 dark:border-slate-800 p-6 text-center text-sm text-slate-500';
  li.textContent = message;
  return li;
}

// buildItemThumbnail renders a small (36px — kept just large enough to
// recognize a product at a glance without pushing the item's single-line
// row past its ~56px height budget) clickable product thumbnail for an item
// whose image_url passed isSafeHttpUrl. It's a plain <img> (never innerHTML
// with the URL interpolated) set via the `src` property, which is the same
// safe pattern app.js already uses for <a href> — the browser treats `src`
// as a URL attribute, not markup, so this can't inject HTML; isSafeHttpUrl
// having already rejected non-http(s) schemes is what stops a stray
// "javascript:" URL from being handed to the image loader at all. Hovering
// scales it up in place for a quick glance (CSS only, via Tailwind utility
// classes); clicking (or focusing + Enter/Space, since it's a real
// <button>) opens the full-size lightbox instead of navigating anywhere.
function buildItemThumbnail(item) {
  const button = document.createElement('button');
  button.type = 'button';
  button.title = t('items.enlargeImageAriaLabel', { title: item.title });
  button.setAttribute('aria-label', t('items.enlargeImageAriaLabel', { title: item.title }));
  button.className =
    'block h-9 w-9 shrink-0 overflow-hidden rounded-md border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900 transition duration-150 hover:z-10 hover:scale-150 hover:shadow-lg focus-visible:z-10 focus-visible:scale-150';

  const img = document.createElement('img');
  img.src = item.image_url;
  img.alt = '';
  img.loading = 'lazy';
  img.decoding = 'async';
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

// A small inline 🔗 icon-link, sitting among the item's single-line row's
// trailing action icons (see buildItemRow) rather than the old standalone
// "lien ↗" pill badge — a real, if compact, 32px tap box rather than a bare
// glyph, so it stays comfortably tappable even packed in next to the price
// and kebab. event.stopPropagation() keeps a tap here from also triggering
// the title's own "open the actions sheet" handler.
function buildInlineLinkIcon(item) {
  const link = document.createElement('a');
  link.href = item.url;
  link.target = '_blank';
  link.rel = 'noopener noreferrer';
  link.className =
    'long-press-target flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-sky-500 hover:bg-sky-500/10 hover:text-sky-400';
  link.setAttribute('aria-label', t('items.openLinkAriaLabel', { title: item.title }));
  link.textContent = '🔗';
  link.addEventListener('click', (event) => event.stopPropagation());
  return link;
}

// Builds the small clickable ⚡ sparkle badge marking a price internal/
// scraper filled in automatically rather than the user having typed it in.
// Clicking it opens the same edit modal as the actions sheet's "Modifier"
// entry, so "modifier en un clic" just reuses the existing edit flow rather
// than needing a dedicated inline editor.
function buildAutoPriceIcon(item) {
  const autoBadge = document.createElement('button');
  autoBadge.type = 'button';
  autoBadge.title = t('items.autoPriceAriaLabel', { title: item.title });
  autoBadge.setAttribute('aria-label', t('items.autoPriceAriaLabel', { title: item.title }));
  autoBadge.className =
    'flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-300 hover:bg-emerald-500/20';
  autoBadge.innerHTML = AUTO_PRICE_ICON_SVG;
  autoBadge.addEventListener('click', (event) => {
    event.stopPropagation();
    openEditItemModal(item);
  });
  return autoBadge;
}

// A fixed-width (44px — room for up to two 20px glyphs side by side, right-
// justified) slot for the ⚡/🔥/⏳/🔄 status glyphs, always rendered — empty
// when none apply — alongside every row of a price-showing list, rather than
// only when a status glyph actually applies to *this* item. Without this, an
// item carrying zero, one, or two of these icons contributed a different
// width to its row than a sibling item carrying a different combination —
// and since every element after the title shares that same title as its
// flex-1 "give way" sibling (see buildItemRow), that width difference shifted
// the *whole* trailing group (stepper, price, link, kebab) left or right
// from one card to the next, even though the shifted elements themselves
// never changed. Reserving this slot at a constant width regardless of its
// content is what keeps every row's trailing group aligned.
const PRICE_STATUS_SLOT_CLASS = 'flex h-5 w-11 shrink-0 items-center justify-end gap-0.5';

// The trailing price cell on an item's single-line row: a right-aligned,
// fixed-min-width "Prix Total Article = Prix Unitaire × Quantité" line
// subtotal (see lineTotal) plus the PRICE_STATUS_SLOT_CLASS status slot
// above. min-w-[68px] on the price figure is the other half of the same
// alignment fix — comfortably fits everything up to "999,99 €" at this
// font size, so two rows showing e.g. "5,00 €" and "129,90 €" still line up
// their status glyphs identically instead of drifting by digit count (an
// unusually large total can still grow past it rather than ever being
// clipped, at the cost of that one row's own alignment). The per-unit price
// a multi-quantity line was computed from is no longer shown here — it's
// still visible (and editable) in the edit-item modal, and keeping it off
// the row is what makes a single 56px-tall line wide enough for the rest of
// a packed row (checkbox, thumbnail, title, stepper, price, link, actions).
// Called for every row of a list whose type shows a price at all
// (`showPrice`, mirroring buildItemRow's own showQuantity/showLink) — even
// an item with no price (and nothing pending) still renders its two empty,
// fixed-width cells, so it lines up with priced siblings in the same list
// rather than collapsing to zero width the way an entirely absent block
// would. Returns null only when the list type doesn't show price at all
// (plain to-do/custom-list items), so buildItemRow can skip appending it
// entirely — every row in *that* list is equally price-less, so there's
// nothing to misalign.
function buildPriceBlock(item, { showPrice }) {
  if (!showPrice) return null;

  const block = document.createElement('div');
  block.className = 'item-card__price flex shrink-0 items-center gap-1';

  const price = document.createElement('span');
  // Before it's aggregated into the finance summary, a single item's own
  // price is just its contribution to the list's TOTAL — never yet
  // "spent" or "remaining" on its own — so it uses the same
  // --tk-money-total token those figures use once summed (see
  // updateFinanceSummary above), not a color of its own.
  price.className = 'min-w-[68px] text-right text-sm font-semibold tabular-nums text-[color:var(--tk-money-total)]';

  const statusSlot = document.createElement('span');
  statusSlot.className = PRICE_STATUS_SLOT_CLASS;

  if (item.price != null) {
    price.textContent = formatEuro(lineTotal(item));
    if (item.price_auto) statusSlot.appendChild(buildAutoPriceIcon(item));
    if (priceAlertCondition(item)) statusSlot.appendChild(buildPriceAlertIcon(item));
  } else if (item.url && isOfflineQueuedItem(item)) {
    statusSlot.appendChild(buildPendingPriceIcon('⏳', t('items.priceSyncPending'), 'text-amber-600 dark:text-amber-300'));
  } else if (item.url && item.priceScrapePending) {
    statusSlot.appendChild(buildPendingPriceIcon('🔄', t('items.priceDetecting'), 'animate-pulse text-sky-600 dark:text-sky-300'));
  }
  // else: no price yet and nothing pending either — both cells stay empty,
  // still reserving their usual width so this row lines up with its priced
  // siblings instead of collapsing.

  block.append(price, statusSlot);
  return block;
}

// One text row (icon + label) for buildItemActionsMeta below.
function buildMetaRow(icon, label) {
  const row = document.createElement('div');
  row.className = 'flex items-center gap-2 px-3 py-1 text-sm text-slate-600 dark:text-slate-300';
  const iconEl = document.createElement('span');
  iconEl.setAttribute('aria-hidden', 'true');
  iconEl.textContent = icon;
  const text = document.createElement('span');
  text.textContent = label;
  row.append(iconEl, text);
  return row;
}

// The secondary detail — target month, recurrence, the price-alert
// threshold, offline-sync status — that used to sit in a second line under
// an item's title (.item-card__secondary) before the card became a single
// 56px-tall line. There's no room left on the row itself for any of it, so
// it's shown instead inside #item-actions-sheet (see openItemActionsSheet
// below), the one place every "act on this item" entry point already
// funnels into — exactly the "menu ⋮" this data was asked to move into.
// Returns a DocumentFragment with one row per applicable field, empty when
// the item carries none of them (a plain to-do/custom-list item, most
// commonly).
function buildItemActionsMeta(item) {
  const fragment = document.createDocumentFragment();

  // monthLabel is defined in planning.js, safe to call here since all
  // script tags finish loading and defining their top-level functions
  // before any rendering actually runs.
  if (item.target_month) {
    fragment.appendChild(buildMetaRow('📅', monthLabel(item.target_month, 'long')));
  }

  const recurrenceLabel = recurrenceBadgeLabel(item.recurrence_rule);
  if (recurrenceLabel) {
    fragment.appendChild(buildMetaRow('🔄', recurrenceLabel));
  }

  if (item.alert_on_price_drop && item.target_price != null) {
    fragment.appendChild(buildMetaRow('🔥', t('items.targetPriceInfo', { target: formatEuro(item.target_price) })));
  }

  // hasPendingItemChanges is defined in app.js; isOfflineQueuedItem (above)
  // already covers an item still carrying its temp-item-* id — this also
  // catches an already-synced item with some other write (an edit, a
  // toggle, a delete) still sitting in the offline sync queue.
  if (hasPendingItemChanges(item.id) || isOfflineQueuedItem(item)) {
    fragment.appendChild(buildMetaRow('⏳', t('sync.listBadgeLabel')));
  }

  return fragment;
}

// buildItemRow lays out one card as a single ~56px-tall line on a wide
// enough screen (Tailwind's `sm` breakpoint, 640px, up) — checkbox/marker,
// an optional thumbnail, the title (truncated, flex-1 so it's the one thing
// that actually gives way on a narrow screen), the quantity stepper, the
// price cell (with its own compact ⚡/🔥/⏳ glyphs — see buildPriceBlock), a
// small unsynced-status dot, the 🔗 link icon, then edit/delete/kebab at the
// trailing edge. Below `sm`, that single line simply doesn't fit: adding up
// every fixed-width slot (checkbox 44px + quantity stepper ~90px + price
// cell ~116px + unsynced dot 8px + link icon 32px + kebab 44px, plus the
// gaps between them) comfortably exceeds a 375px phone's own width before
// the title — the one flexible element — gets anything at all, which is
// exactly the "title squeezed to nothing, price/link/kebab clipped off the
// right edge" overflow this two-row split fixes.
//
// Every child buildItemRow appends is grouped into two always-present
// wrapper `<div>`s, `rowTop` (checkbox/marker + thumbnail + title +
// edit/delete + kebab) and `rowBottom` (quantity stepper + unsynced dot +
// price cell + link icon), each its own `flex items-center gap-2` row
// internally — not just below `sm`, but at every width, so there's exactly
// one place the child-append order is decided rather than two diverging
// code paths to keep in sync. What changes per breakpoint is purely how `li`
// itself lays those two wrappers out: `flex-col` (stacked, one row above the
// other) below `sm`, `sm:flex-row sm:items-center` (side by side, i.e. one
// visual line again — rowTop's `sm:flex-1` is what lets it claim the
// leftover width for its own title to truncate into, with rowBottom staying
// at its natural content width on the right) from `sm` up. This is why
// rowTop needs `min-w-0` unconditionally: it's title's actual flex ancestor
// now (previously `li` filled that role directly), and `flex-1` without a
// `min-w-0` companion can't shrink past its content's own auto min-width —
// the same reason `title` itself already carries `min-w-0`.
//
// Everything that doesn't fit even on two lines (target month, recurrence,
// the price-alert threshold) still lives in #item-actions-sheet instead —
// see buildItemActionsMeta above and openItemActionsSheet below — the
// single sheet every "act on this item" entry point (a tap on the title,
// the [⋮] kebab, or a long press) already funnels into. `.item-card__actions`
// (✏️/🗑️) and `.item-card__kebab` (⋮) are both always built; Tailwind's
// `hidden md:flex`/`flex md:hidden` on the two decide which one is actually
// visible per breakpoint — see the comment above that CSS block for why.
//
// showQuantity/showPrice/showLink (all list-type-level, from
// fieldVisibilityFor — see renderItems' two call sites) each gate whether
// that field's slot is reserved *at all* for every row of this list, as
// opposed to hasLink/item.price below, which are per-item and only decide
// what's actually drawn *inside* an already-reserved slot. That split is
// deliberate: within one list every row shares the same showQuantity/
// showPrice/showLink (the list's type doesn't vary row to row), so there's
// no alignment risk in adding or omitting a whole slot consistently: but
// whether *this* item happens to have a link, a price, or a price-status
// glyph absolutely does vary row to row, and reserving that slot's width
// unconditionally (leaving it empty rather than absent) is what keeps the
// stepper/price/link/kebab group aligned from one card to the next within
// rowBottom — see buildPriceBlock's own PRICE_STATUS_SLOT_CLASS comment for
// the mechanism.
function buildItemRow(item, { showCheckbox = true, index, showQuantity = true, showPrice = true, showLink = true } = {}) {
  const li = document.createElement('li');
  // An unfinished urgent item gets a distinctive rose border so it stands
  // out at a glance in the (already sorted-to-top, see renderItems) active
  // list — a completed urgent item just reverts to the normal styling since
  // it no longer needs attention. py-1.5 (6px) plus the 44px checkbox/kebab
  // touch targets below is what keeps a single-line row (`sm` and up) within
  // the ~56px height budget this redesign targets; below `sm`, `flex-col`
  // stacks rowTop/rowBottom instead — see the header comment above.
  li.className =
    item.is_urgent && !item.done
      ? 'item-card flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2 rounded-xl border-2 border-rose-500/60 bg-rose-500/5 px-3 py-1.5'
      : 'item-card flex flex-col gap-1.5 sm:flex-row sm:items-center sm:gap-2 rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30 px-3 py-1.5';

  // rowTop: checkbox/marker + thumbnail + title + edit/delete + kebab — see
  // the header comment above for why this wrapper exists and why it needs
  // `min-w-0` alongside `sm:flex-1`.
  const rowTop = document.createElement('div');
  rowTop.className = 'item-card__row-top flex min-w-0 items-center gap-2 sm:flex-1';

  // rowBottom: quantity stepper + unsynced dot + price cell + link icon —
  // stays at its own natural content width rather than growing, both
  // stacked under rowTop (mobile) and sitting to its right (`sm` and up).
  const rowBottom = document.createElement('div');
  rowBottom.className = 'item-card__row-bottom flex shrink-0 items-center gap-2';

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
    rowTop.appendChild(checkboxLabel);
  } else {
    rowTop.appendChild(buildLineMarker(index));
  }

  // A thumbnail only ever appears when a product image was actually found
  // (see internal/scraper's og:image/JSON-LD/twitter:image lookup) — no
  // placeholder box is reserved for items without one, so plain to-do items
  // and shopping items with no matched image just skip straight to the
  // title, unchanged from before this feature existed.
  if (item.image_url && isSafeHttpUrl(item.image_url)) {
    rowTop.appendChild(buildItemThumbnail(item));
  }

  const title = document.createElement('span');
  // `truncate` (Tailwind's overflow-hidden/text-overflow-ellipsis/
  // whitespace-nowrap shorthand) needs a `min-w-0` flex ancestor to actually
  // clip instead of forcing the row wider — `rowTop` (itself `min-w-0`,
  // and `flex-1` from `sm` up) is that ancestor, so a long title on a
  // narrow screen never pushes the edit/kebab group out of the card instead
  // of just eliding. A tap on the title itself opens the same item-actions
  // sheet as the [⋮] kebab button, the mobile-first "tap the item" entry
  // point the pencil/trash buttons already cover on wider screens.
  title.className = item.done
    ? 'min-w-0 flex-1 cursor-pointer truncate text-sm font-semibold text-slate-500 line-through opacity-60'
    : 'min-w-0 flex-1 cursor-pointer truncate text-sm font-semibold text-slate-900 dark:text-slate-100';
  // When the stepper below is shown, it's the canonical place quantity is
  // displayed/edited, so the title stays plain; otherwise (custom lists,
  // where quantity is hidden entirely) fall back to the old "title × N"
  // form, still used as a compact read-only summary in urgent.js/planning.js.
  title.textContent = !showQuantity && item.quantity > 1 ? `${item.title} × ${item.quantity}` : item.title;
  title.addEventListener('click', () => openItemActionsSheet(item));
  rowTop.appendChild(title);

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

  rowTop.appendChild(actions);

  const kebabBtn = document.createElement('button');
  kebabBtn.type = 'button';
  kebabBtn.setAttribute('aria-label', t('items.moreActionsAriaLabel', { title: item.title }));
  kebabBtn.className = 'item-card__kebab flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-700 md:hidden';
  kebabBtn.innerHTML = KEBAB_ICON_SVG;
  kebabBtn.addEventListener('click', () => openItemActionsSheet(item));
  rowTop.appendChild(kebabBtn);

  if (showQuantity) {
    rowBottom.appendChild(buildQuantityStepper(item));
  }

  // hasPendingItemChanges is defined in app.js; isOfflineQueuedItem (above)
  // already covers an item still carrying its temp-item-* id — this also
  // catches an already-synced item with some other write (an edit, a
  // toggle, a delete) still sitting in the offline sync queue. Always
  // reserved (a bare dot, or its same-size empty stand-in) rather than only
  // appended when pending — see buildItemRow's own header comment on why an
  // absent-vs-present slot here would misalign every row after it.
  rowBottom.appendChild(
    hasPendingItemChanges(item.id) || isOfflineQueuedItem(item) ? buildUnsyncedDot(item) : buildEmptySlot('h-2 w-2')
  );

  const priceBlock = buildPriceBlock(item, { showPrice });
  if (priceBlock) rowBottom.appendChild(priceBlock);

  const hasLink = Boolean(item.url && isSafeHttpUrl(item.url));
  if (showLink) {
    if (hasLink) {
      const linkIcon = buildInlineLinkIcon(item);
      rowBottom.appendChild(linkIcon);
      // A long press (~500ms touch hold, desktop mouse unaffected — see
      // attachLongPress) on either the title or the link icon itself opens
      // the very same #item-actions-sheet a short tap does, just pre-focused
      // on its Ouvrir/Copier/Partager link group — the "appui long sur
      // l'item ou le lien" gesture, without a second sheet to keep in sync.
      attachLongPress(title, () => openItemActionsSheet(item, { focusLink: true }));
      attachLongPress(linkIcon, () => openItemActionsSheet(item, { focusLink: true }));
    } else {
      // Same h-8 w-8 footprint as buildInlineLinkIcon's own <a> so a list
      // where only some items carry a url (the common case) still keeps
      // every row's trailing group aligned — see buildItemRow's header
      // comment.
      rowBottom.appendChild(buildEmptySlot('h-8 w-8'));
    }
  }

  li.appendChild(rowTop);
  li.appendChild(rowBottom);

  // attachItemSwipeGestures is defined in gestures.js — swipe right to
  // toggle done (only when this list type actually shows the checkbox at
  // all), swipe left to delete, both reusing toggleDone/removeItem exactly
  // as the checkbox/trash button above already do. Touch-only and a no-op
  // on a device with no touchscreen — see that file's own header comment.
  // It moves every child of `li` (i.e. rowTop and rowBottom themselves) into
  // its own foreground wrapper, so it needs no changes for this split.
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
      listEls.itemsActive.appendChild(
        buildItemRow(item, {
          showCheckbox: visibility.done,
          index,
          showQuantity: visibility.quantity,
          showPrice: visibility.price,
          showLink: visibility.url,
        })
      );
    });
  }

  listEls.itemsDone.replaceChildren();
  // Always a checkbox here, regardless of visibility.done: a custom list's
  // create/edit form never lets an item become done in the first place (the
  // checkbox is hidden), so any item that does show up in this "done"
  // bucket only got there some other way (e.g. the list's type was changed
  // after the item was already completed) — it still needs a way back to
  // active, which only the checkbox provides.
  for (const item of done) {
    listEls.itemsDone.appendChild(
      buildItemRow(item, {
        showCheckbox: true,
        showQuantity: visibility.quantity,
        showPrice: visibility.price,
        showLink: visibility.url,
      })
    );
  }
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
// Shown by selectList, synchronously, only when a list has never been
// opened on this device before (cachedListDetail below found nothing to
// paint immediately instead) — a shimmering placeholder so a first-ever
// open on a slow connection shows visible structure right away instead of
// sitting on the still-visible dashboard with no feedback. Real content
// (from the cache or the network, whichever resolves first) replaces this
// via renderItems' own listEls.itemsActive.replaceChildren() the moment
// selectList calls it — nothing here needs to be explicitly torn down.
function renderItemsSkeleton() {
  listEls.itemsHeading.textContent = t('items.loadingHeading');
  listEls.listSyncIndicator.hidden = true;
  listEls.doneSection.hidden = true;
  listEls.itemsActive.replaceChildren();
  for (let i = 0; i < 4; i++) {
    const li = document.createElement('li');
    li.className = 'tk-skeleton tk-skeleton-row';
    li.setAttribute('aria-hidden', 'true');
    listEls.itemsActive.appendChild(li);
  }
  listEls.itemsDone.replaceChildren();
}

async function selectList(id, opts = {}) {
  const { silent = false } = opts;
  hideError();
  // exitReorderModeIfActive is defined in reorder.js — opening another list
  // (or reopening this one) mid-drag must not leave the new view stuck in
  // reorder mode.
  exitReorderModeIfActive();

  // Stale-while-revalidate: paint immediately from whatever's already in
  // the local mirror, before the network fetch below even starts, so
  // opening a previously-seen list feels instant instead of waiting on a
  // round trip — falls back to a shimmering skeleton (renderItemsSkeleton
  // above) when there's nothing cached yet for this particular list.
  const cachedForPaint = await cachedListDetail(id);
  els.listsSection.hidden = true;
  listEls.itemsSection.hidden = false;
  if (cachedForPaint) {
    state.currentListId = id;
    state.currentList = cachedForPaint;
    renderItems();
  } else {
    renderItemsSkeleton();
  }

  let list;
  try {
    list = await apiRequest(`/lists/${id}`);
  } catch (err) {
    // Offline, or a transient server error: keep showing whatever the
    // pre-paint above already rendered (or fail to open the list at all if
    // there was nothing to paint in the first place).
    list = cachedForPaint;
    if (!list) {
      listEls.itemsSection.hidden = true;
      els.listsSection.hidden = false;
      state.currentListId = null;
      state.currentList = null;
      if (!silent && !isNetworkError(err)) showError(err.message);
      return;
    }
  }
  state.currentListId = id;
  state.currentList = list;
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
      if (!isNetworkError(err)) showError(err.message);
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
      if (!isNetworkError(err)) showError(err.message);
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

  // max-w-[90px] (mobile) / sm:max-w-none is a hard cap on top of the
  // already-compact 28px buttons + narrower input below — see buildItemRow's
  // header comment on the two-row mobile layout this stepper now sits
  // inside as part of rowBottom, where it no longer has to share a line
  // with the title at all.
  const wrap = document.createElement('div');
  wrap.className =
    'flex max-w-[90px] shrink-0 items-center gap-0.5 rounded-full border border-slate-300 dark:border-slate-600 bg-white dark:bg-slate-900 px-0.5 sm:max-w-none';

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
    'w-8 shrink-0 rounded border-0 bg-transparent text-center text-sm font-medium text-slate-900 dark:text-slate-100 focus:outline-none focus:ring-2 focus:ring-sky-500/40 sm:w-10';
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

  // Captured now, at call time, rather than re-read as state.currentList
  // inside onUndo/onCommit below — toggleDone is only ever invoked from a
  // row rendered for the currently open list (same reasoning removeItem's
  // own doc comment already gives), whereas by the time onUndo fires (the
  // toast persists across navigation) or onCommit fires (5s later), the
  // user may have moved to a different list or away from any list entirely.
  const list = state.currentList;
  const listId = state.currentListId;

  const pending = pendingToggles.get(item);
  const committedDone = pending ? pending.committedDone : item.done;
  if (pending) pending.dismiss();

  item.done = !item.done;
  renderItems();
  // notifyItemsChanged (app.js) is the "event bus" side of live dashboard
  // reactivity: it snapshots this list's items (including this optimistic
  // toggle, well before the real PATCH below ever goes out) so that if the
  // user navigates back to the dashboard within the 5s undo window, the
  // card there already reflects the new state — recalculating its
  // remaining-items count and swapping in the "all done" badge immediately
  // — instead of showing the stale pre-toggle state until the deferred
  // commit finally lands and a real refetch corrects it.
  notifyItemsChanged(listId, list.items);
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
      notifyItemsChanged(listId, list.items);
    },
    onCommit: async () => {
      pendingToggles.delete(item);
      try {
        const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({ done: newDone }) });
        Object.assign(item, updated);
      } catch (err) {
        item.done = committedDone;
        if (!isNetworkError(err)) showError(err.message);
      } finally {
        // The real PATCH has now either landed or failed, so the server (or
        // the reverted local state, on failure) is authoritative again —
        // drop the optimistic snapshot above rather than let it keep
        // shadowing whatever a fresh fetch/re-render is about to show.
        clearItemsOverride(listId);
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
  // See toggleDone's own notifyItemsChanged call above — same immediate
  // dashboard-reactivity snapshot, minus whatever's mid-undo-grace-period
  // (a deleted-but-not-yet-committed item shouldn't count toward the
  // dashboard card's remaining-items total either).
  notifyItemsChanged(list.id, list.items.filter((i) => !i.pendingDelete));

  TrakkaUndo.schedule({
    message: t('undo.itemDeleted', { title: item.title }),
    undoLabel: t('undo.cancel'),
    onUndo: () => {
      delete item.pendingDelete;
      renderItems();
      notifyItemsChanged(list.id, list.items.filter((i) => !i.pendingDelete));
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
        if (!isNetworkError(err)) showError(err.message);
      }
      // The real DELETE has now either landed or failed — see toggleDone's
      // own onCommit for why the optimistic snapshot is dropped here rather
      // than left to shadow the fresh state a re-render/refetch is about to
      // show.
      clearItemsOverride(list.id);
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
  let targetPrice = null;
  let targetMonth = '';
  if (visibility.price) {
    const raw = listEls.itemPrice.value.trim();
    if (raw !== '') {
      const parsed = Number.parseFloat(raw);
      if (Number.isFinite(parsed) && parsed >= 0) price = parsed;
    }
    const rawTarget = listEls.itemTargetPrice.value.trim();
    if (rawTarget !== '') {
      const parsedTarget = Number.parseFloat(rawTarget);
      if (Number.isFinite(parsedTarget) && parsedTarget >= 0) targetPrice = parsedTarget;
    }
  }
  if (visibility.targetMonth) targetMonth = listEls.itemTargetMonth.value;

  const recurrenceRule = visibility.recurrence ? listEls.itemRecurrence.value : '';
  const isUrgent = visibility.urgent ? listEls.itemUrgent.checked : false;

  const payload = { list_id: state.currentListId, title, quantity };
  if (url) payload.url = url;
  if (price !== null) payload.price = price;
  // A target price with no separate opt-in checkbox in the UI: typing one
  // in implicitly opts the item into the alert, mirroring
  // internal/handlers.checkPriceDropAlert's two-field model server-side
  // (target_price/alert_on_price_drop) while keeping the form itself down
  // to the single field the feature's own request described.
  if (targetPrice !== null) {
    payload.target_price = targetPrice;
    payload.alert_on_price_drop = true;
  }
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
    target_price: targetPrice,
    alert_on_price_drop: targetPrice !== null,
    target_month: targetMonth || null,
    recurrence_rule: recurrenceRule || null,
    is_urgent: isUrgent,
    done: false,
    position: 0,
  };
  state.currentList.items = [...(state.currentList.items || []), optimisticItem];
  renderItems();
  // Resets the text field and collapses the advanced panel back to its
  // default "épuré" state, but keeps focus in the (now-empty) title input —
  // so tapping [+] or pressing Enter never dismisses the mobile keyboard,
  // letting several items be added back-to-back without re-tapping the field.
  listEls.createItemForm.reset();
  listEls.itemQuantity.value = '1';
  setQuickAddAdvancedExpanded(false);
  listEls.itemTitle.focus();

  try {
    const created = await apiRequest('/items', { method: 'POST', body: JSON.stringify(payload) });
    Object.assign(optimisticItem, created);
    optimisticItem.priceScrapePending = created.price_status === 'pending';
    if (optimisticItem.priceScrapePending) scheduleAutoPriceRefresh(optimisticItem);
    notifyPriceAlertIfTriggered(created);
  } catch (err) {
    state.currentList.items = state.currentList.items.filter((item) => item !== optimisticItem);
    if (!isNetworkError(err)) showError(err.message);
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
  listEls.editItemTargetPrice.value = item.target_price != null ? item.target_price : '';
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
  listEls.itemActionsSheetMeta.replaceChildren(buildItemActionsMeta(item));
  listEls.itemActionsSheetMeta.hidden = listEls.itemActionsSheetMeta.children.length === 0;
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
    // Same implicit-opt-in convention as the create-item form above: a
    // filled-in target price opts the item into the alert, an emptied one
    // clears both fields together — no separate checkbox in the UI.
    const rawTarget = listEls.editItemTargetPrice.value.trim();
    if (rawTarget === '') {
      payload.target_price = null;
      payload.alert_on_price_drop = false;
    } else {
      const parsedTarget = Number.parseFloat(rawTarget);
      if (!Number.isFinite(parsedTarget) || parsedTarget < 0) {
        showError(t('items.targetPriceValidationError'));
        return;
      }
      payload.target_price = parsedTarget;
      payload.alert_on_price_drop = true;
    }
  }
  if (visibility.targetMonth) payload.target_month = listEls.editItemTargetMonth.value;
  if (visibility.recurrence) payload.recurrence_rule = listEls.editItemRecurrence.value;

  const item = editingItem;
  const previous = {
    title: item.title,
    url: item.url,
    price: item.price,
    target_price: item.target_price,
    alert_on_price_drop: item.alert_on_price_drop,
    target_month: item.target_month,
    recurrence_rule: item.recurrence_rule,
    is_urgent: item.is_urgent,
  };
  item.title = title;
  item.url = url || null;
  if ('price' in payload) item.price = payload.price;
  if ('target_price' in payload) item.target_price = payload.target_price;
  if ('alert_on_price_drop' in payload) item.alert_on_price_drop = payload.alert_on_price_drop;
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
    notifyPriceAlertIfTriggered(updated);
  } catch (err) {
    Object.assign(item, previous);
    if (!isNetworkError(err)) showError(err.message);
  } finally {
    renderItems();
  }
  await refreshPendingBadge();
});
