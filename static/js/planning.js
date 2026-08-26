'use strict';

// "Budget & Prévisions Achats" planning view: a second tab alongside the
// dashboard (shares `state`, `els`, `apiRequest`, `showError`/`hideError`,
// `t`, `formatEuro`, `refreshPendingBadge` with app.js/list_view.js — same
// classic-<script>-tags shared-scope pattern as those two files). It groups
// every scheduled shopping item (items with a `target_month` set via the
// edit-item modal in list_view.js) by month over a selectable horizon and
// lets the user move one to a different month right from here.

const planningEls = {
  tabDashboard: document.getElementById('tab-dashboard'),
  tabPlanning: document.getElementById('tab-planning'),
  dashboardView: document.getElementById('dashboard-view'),
  planningView: document.getElementById('planning-view'),
  horizonButtons: document.querySelectorAll('#planning-view [data-horizon]'),
  total: document.getElementById('planning-total'),
  months: document.getElementById('planning-months'),
};

let planningTabActive = false;
let planningHorizon = 3;
// Every scheduled item currently known for state.currentHouseId, as
// { item, listName } — re-grouped by month locally on every horizon change
// rather than re-fetched, since the horizon only narrows/widens which of
// the already-loaded scheduled items are shown.
let planningEntries = [];

function isPlanningTabActive() {
  return planningTabActive;
}

// monthsFromNow(3) with today in August 2026 returns
// ["2026-08", "2026-09", "2026-10"] — the current month counts as the first
// of "the next N months" (an item scheduled for this month is still
// upcoming spending), not skipped in favor of the following one.
function monthsFromNow(count) {
  const now = new Date();
  const months = [];
  for (let i = 0; i < count; i++) {
    const d = new Date(now.getFullYear(), now.getMonth() + i, 1);
    months.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`);
  }
  return months;
}

// monthLabel formats a "YYYY-MM" string using the currently active UI
// language (see i18n.js) — 'long' for section headings ("Novembre 2026"),
// 'short' for the compact badge list_view.js shows on a scheduled item's
// row ("nov. 2026"). Intl capitalizes "November 2026" on its own in en-US
// but not "novembre 2026" in fr-FR, hence the manual capitalization below.
function monthLabel(monthStr, style) {
  const [year, month] = monthStr.split('-').map(Number);
  const locale = window.TrakkaI18n && TrakkaI18n.getLang() === 'en' ? 'en-US' : 'fr-FR';
  const formatted = new Intl.DateTimeFormat(locale, { month: style === 'short' ? 'short' : 'long', year: 'numeric' }).format(
    new Date(year, month - 1, 1)
  );
  return formatted.charAt(0).toUpperCase() + formatted.slice(1);
}

function setTabButtonState(button, active) {
  button.setAttribute('aria-selected', String(active));
  button.classList.toggle('border-sky-500', active);
  button.classList.toggle('bg-sky-500/10', active);
  button.classList.toggle('text-sky-300', active);
  button.classList.toggle('border-slate-700', !active);
  button.classList.toggle('bg-slate-900', !active);
  button.classList.toggle('text-slate-300', !active);
}

function setActiveTab(tab) {
  planningTabActive = tab === 'planning';
  planningEls.dashboardView.hidden = planningTabActive;
  planningEls.planningView.hidden = !planningTabActive;
  setTabButtonState(planningEls.tabDashboard, !planningTabActive);
  setTabButtonState(planningEls.tabPlanning, planningTabActive);
  if (planningTabActive) loadPlanningView();
}

planningEls.tabDashboard.addEventListener('click', () => setActiveTab('dashboard'));
planningEls.tabPlanning.addEventListener('click', () => setActiveTab('planning'));

function setHorizonButtonState(button, active) {
  button.classList.toggle('border-sky-500', active);
  button.classList.toggle('bg-sky-500/10', active);
  button.classList.toggle('text-sky-300', active);
  button.classList.toggle('border-slate-700', !active);
  button.classList.toggle('bg-slate-900', !active);
  button.classList.toggle('text-slate-300', !active);
}

for (const button of planningEls.horizonButtons) {
  button.addEventListener('click', () => {
    planningHorizon = Number(button.dataset.horizon);
    for (const other of planningEls.horizonButtons) setHorizonButtonState(other, other === button);
    renderPlanning();
  });
}

// Fetches every shopping list in the current house plus each one's items
// (the same "list_id -> detail" fan-out loadDashboard already does for its
// badges), then keeps only the items that actually have a target_month —
// unscheduled items simply don't appear in this view. Re-run whenever the
// planning tab is opened, the house changes, or the offline queue flushes;
// horizon changes alone just re-filter the already-loaded result.
async function loadPlanningView() {
  if (state.currentHouseId === null) {
    planningEntries = [];
    renderPlanning();
    return;
  }

  try {
    const lists = await apiRequest(`/lists?house_id=${state.currentHouseId}`);
    const shoppingLists = lists.filter((list) => list.type === 'shopping');
    const detailed = await Promise.all(
      shoppingLists.map((list) => apiRequest(`/lists/${list.id}`).catch(() => ({ ...list, items: [] })))
    );
    const entries = [];
    for (const list of detailed) {
      for (const item of list.items || []) {
        if (item.target_month) entries.push({ item, listName: list.name });
      }
    }
    planningEntries = entries;
  } catch (err) {
    showError(err.message);
    planningEntries = [];
  }
  renderPlanning();
}

// Called after anything that might have changed the current house's
// scheduled items (offline sync completing, a language switch, a house
// switch) — a no-op unless this tab is the one currently visible.
function refreshPlanningIfActive() {
  if (planningTabActive) loadPlanningView();
}

function renderPlanning() {
  const range = monthsFromNow(planningHorizon);
  const rangeSet = new Set(range);
  const grouped = new Map(range.map((month) => [month, []]));
  let total = 0;

  for (const entry of planningEntries) {
    const month = entry.item.target_month;
    if (!rangeSet.has(month)) continue;
    grouped.get(month).push(entry);
    // Mirrors updateFinanceSummary in list_view.js: price is treated as a
    // line total, not multiplied by quantity, so the two totals agree.
    if (typeof entry.item.price === 'number') total += entry.item.price;
  }

  planningEls.total.textContent = formatEuro(total);
  planningEls.months.replaceChildren();
  for (const month of range) {
    planningEls.months.appendChild(buildMonthCard(month, grouped.get(month)));
  }
}

function buildMonthCard(month, entries) {
  const card = document.createElement('div');
  card.className = 'rounded-2xl border border-slate-800 bg-slate-800/30 p-4';

  const header = document.createElement('div');
  header.className = 'mb-3 flex items-center justify-between gap-3';
  const heading = document.createElement('h3');
  heading.className = 'text-base font-semibold text-slate-100';
  heading.textContent = monthLabel(month, 'long');
  header.appendChild(heading);

  if (entries.length > 0) {
    const subtotal = document.createElement('span');
    subtotal.className = 'text-sm font-semibold text-emerald-400';
    const monthTotal = entries.reduce((sum, entry) => sum + (typeof entry.item.price === 'number' ? entry.item.price : 0), 0);
    subtotal.textContent = formatEuro(monthTotal);
    header.appendChild(subtotal);
  }

  card.appendChild(header);

  if (entries.length === 0) {
    const empty = document.createElement('p');
    empty.className = 'text-sm text-slate-500';
    empty.textContent = t('planning.emptyMonth');
    card.appendChild(empty);
    return card;
  }

  const list = document.createElement('ul');
  list.className = 'space-y-2';
  for (const entry of entries) list.appendChild(buildPlanningItemRow(entry, month));
  card.appendChild(list);
  return card;
}

function buildPlanningItemRow(entry, currentMonth) {
  const { item, listName } = entry;
  const li = document.createElement('li');
  li.className = 'flex flex-wrap items-center gap-2 rounded-xl border border-slate-700 bg-slate-900/60 px-3 py-2';

  const info = document.createElement('div');
  info.className = 'min-w-0 flex-1';
  const title = document.createElement('p');
  title.className = 'truncate text-sm font-medium text-slate-100';
  title.textContent = item.quantity > 1 ? `${item.title} × ${item.quantity}` : item.title;
  const listLabel = document.createElement('p');
  listLabel.className = 'truncate text-xs text-slate-400';
  listLabel.textContent = listName;
  info.append(title, listLabel);
  li.appendChild(info);

  if (item.price != null) {
    const price = document.createElement('span');
    price.className = 'shrink-0 text-sm font-medium text-slate-300';
    price.textContent = formatEuro(item.price);
    li.appendChild(price);
  }

  // Lets the user "report" (postpone) or pull forward a planned purchase
  // right from this view — a plain <select> rather than a modal, since a
  // single field is all a reschedule needs. Options span a fixed 12-month
  // window regardless of the active horizon filter, so an item can be
  // pushed out past the currently visible range.
  const select = document.createElement('select');
  select.setAttribute('aria-label', t('planning.moveAriaLabel', { title: item.title }));
  select.className =
    'shrink-0 rounded-lg border border-slate-700 bg-slate-900 px-2 py-1.5 text-xs text-slate-200 focus:border-sky-500 focus:outline-none focus:ring-2 focus:ring-sky-500/30';

  const unscheduledOption = document.createElement('option');
  unscheduledOption.value = '';
  unscheduledOption.textContent = t('planning.unscheduled');
  select.appendChild(unscheduledOption);

  for (const month of monthsFromNow(12)) {
    const option = document.createElement('option');
    option.value = month;
    option.textContent = monthLabel(month, 'long');
    option.selected = month === currentMonth;
    select.appendChild(option);
  }

  select.addEventListener('change', () => moveItemToMonth(item, select.value));
  li.appendChild(select);

  return li;
}

async function moveItemToMonth(item, newMonth) {
  hideError();
  const previous = item.target_month;
  item.target_month = newMonth || null;
  renderPlanning();

  try {
    const updated = await apiRequest(`/items/${item.id}`, { method: 'PATCH', body: JSON.stringify({ target_month: newMonth }) });
    Object.assign(item, updated);
  } catch (err) {
    item.target_month = previous;
    showError(err.message);
  } finally {
    // Unscheduling drops the item out of this view entirely, same as any
    // other item that has never had a target_month set.
    if (!item.target_month) {
      planningEntries = planningEntries.filter((entry) => entry.item !== item);
    }
    renderPlanning();
  }
  await refreshPendingBadge();
}
