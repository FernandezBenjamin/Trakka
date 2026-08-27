'use strict';

// Notification bell in the header: surfaces price_alerts (see
// internal/handlers/price_alerts.go) — a lower price internal/scraper found
// for one of the current house's items versus its current price — as a
// badge count plus a drawer to accept ("Appliquer le nouveau prix", which
// PATCHes the item's price) or reject ("Ignorer") each one. Shares `state`,
// `els`, `apiRequest`, `showError`/`hideError`, `t`, `formatEuro`,
// `isSafeHttpUrl`, `refreshVisibleView` with app.js/list_view.js/planning.js
// — same classic-<script>-tags shared-scope pattern as those files.
//
// Unlike the planning/urgent tabs this isn't a dashboard view: it's a
// header-level drawer, independent of which tab is currently active, so it
// is refreshed from its own hook points (see refreshNotifications' call
// sites in app.js) rather than through refreshVisibleView's tab switch.

const notifEls = {
  button: document.getElementById('notifications-button'),
  badge: document.getElementById('notifications-badge'),
  modal: document.getElementById('notifications-modal'),
  closeButton: document.getElementById('close-notifications-modal-button'),
  list: document.getElementById('notifications-list'),
};

// Every pending price alert known for state.currentHouseId — re-fetched by
// loadNotifications, re-rendered into the drawer whenever it's open.
let notificationAlerts = [];

function updateNotificationsBadge() {
  const count = notificationAlerts.length;
  notifEls.badge.hidden = count === 0;
  notifEls.badge.textContent = count > 9 ? '9+' : String(count);
}

function renderNotificationsList() {
  notifEls.list.replaceChildren();
  if (notificationAlerts.length === 0) {
    const li = document.createElement('li');
    li.className = 'rounded-xl border border-dashed border-slate-700 p-6 text-center text-sm text-slate-500';
    li.textContent = t('notifications.empty');
    notifEls.list.appendChild(li);
    return;
  }
  for (const alert of notificationAlerts) notifEls.list.appendChild(buildAlertRow(alert));
}

function buildAlertRow(alert) {
  const li = document.createElement('li');
  li.className = 'rounded-xl border border-slate-700 bg-slate-900/60 p-3';

  const title = document.createElement('p');
  title.className = 'mb-2 truncate text-sm font-medium text-slate-100';
  title.textContent = alert.item_title;
  li.appendChild(title);

  const compare = document.createElement('div');
  compare.className = 'mb-3 flex items-center gap-3';

  const oldPrice = document.createElement('div');
  oldPrice.className = 'flex flex-col';
  const oldLabel = document.createElement('span');
  oldLabel.className = 'text-xs text-slate-500';
  oldLabel.textContent = t('notifications.oldPrice');
  const oldValue = document.createElement('span');
  oldValue.className = 'text-sm font-medium text-slate-500 line-through';
  oldValue.textContent = formatEuro(alert.original_price);
  oldPrice.append(oldLabel, oldValue);

  const arrow = document.createElement('span');
  arrow.className = 'text-slate-500';
  arrow.setAttribute('aria-hidden', 'true');
  arrow.textContent = '→';

  const newPrice = document.createElement('div');
  newPrice.className = 'flex flex-col';
  const newLabel = document.createElement('span');
  newLabel.className = 'text-xs text-slate-500';
  newLabel.textContent = t('notifications.newPrice');
  const newValue = document.createElement('span');
  newValue.className = 'text-base font-semibold text-emerald-400';
  newValue.textContent = formatEuro(alert.found_price);
  newPrice.append(newLabel, newValue);

  compare.append(oldPrice, arrow, newPrice);
  li.appendChild(compare);

  // isSafeHttpUrl (defined in app.js) re-checks the scheme client-side even
  // though the backend only ever persists a validated http(s) source_url —
  // the same defense-in-depth pattern used for every other rendered <a>.
  if (alert.source_url && isSafeHttpUrl(alert.source_url)) {
    const link = document.createElement('a');
    link.href = alert.source_url;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.className = 'mb-3 block truncate text-xs text-sky-400 hover:underline';
    link.textContent = alert.source_url;
    li.appendChild(link);
  }

  const actions = document.createElement('div');
  actions.className = 'flex gap-2';

  const applyBtn = document.createElement('button');
  applyBtn.type = 'button';
  applyBtn.className = 'flex-1 rounded-xl bg-emerald-500 px-3 py-2 text-sm font-semibold text-white transition hover:bg-emerald-400 active:scale-95';
  applyBtn.textContent = t('notifications.apply');
  applyBtn.addEventListener('click', () => resolveAlert(alert, 'accepted'));

  const ignoreBtn = document.createElement('button');
  ignoreBtn.type = 'button';
  ignoreBtn.className = 'flex-1 rounded-xl border border-slate-700 bg-slate-900 px-3 py-2 text-sm font-medium text-slate-300 transition hover:bg-slate-800 active:scale-95';
  ignoreBtn.textContent = t('notifications.ignore');
  ignoreBtn.addEventListener('click', () => resolveAlert(alert, 'rejected'));

  actions.append(applyBtn, ignoreBtn);
  li.appendChild(actions);

  return li;
}

// Fetches the current house's pending price alerts and refreshes the badge
// count; also re-renders the drawer's contents if it's currently open. This
// is a background refresh (house switch, offline sync completing, a tab
// becoming visible again, ...), so a network failure just keeps the last
// known count rather than surfacing an error banner.
async function loadNotifications() {
  if (state.currentHouseId === null) {
    notificationAlerts = [];
    updateNotificationsBadge();
    return;
  }
  try {
    notificationAlerts = await apiRequest(`/price-alerts?house_id=${state.currentHouseId}&status=pending`);
  } catch {
    return;
  }
  updateNotificationsBadge();
  if (!notifEls.modal.hidden) renderNotificationsList();
}

// Called from app.js's hook points (house switch, offline sync completing,
// a language switch, the tab regaining visibility) — resolved lazily the
// same way planning.js/urgent.js's refresh functions are called from there.
function refreshNotifications() {
  loadNotifications();
}

function openNotificationsModal() {
  notifEls.modal.hidden = false;
  document.body.classList.add('overflow-hidden');
  renderNotificationsList();
}

function closeNotificationsModal() {
  notifEls.modal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

notifEls.button.addEventListener('click', () => {
  openNotificationsModal();
  loadNotifications();
});
notifEls.closeButton.addEventListener('click', closeNotificationsModal);
notifEls.modal.addEventListener('click', (event) => {
  if (event.target === notifEls.modal) closeNotificationsModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !notifEls.modal.hidden) closeNotificationsModal();
});

// Resolving is optimistic (the row disappears immediately) with a rollback
// on failure, the same pattern urgent.js's markUrgentItemDone uses.
// Accepting can change an item's price while it's on screen elsewhere (its
// own list view, the planning view, ...), hence the refreshVisibleView()
// call — defined in app.js, resolved lazily the same way.
async function resolveAlert(alert, status) {
  hideError();
  notificationAlerts = notificationAlerts.filter((a) => a.id !== alert.id);
  renderNotificationsList();
  updateNotificationsBadge();

  try {
    await apiRequest(`/price-alerts/${alert.id}`, { method: 'PATCH', body: JSON.stringify({ status }) });
  } catch (err) {
    notificationAlerts = [...notificationAlerts, alert];
    renderNotificationsList();
    updateNotificationsBadge();
    showError(err.message);
    return;
  }
  refreshVisibleView();
}
