'use strict';

// Granular sharing: a Space (custom category) or an individual List can be
// shared directly with one other user (by email), giving them read-only or
// write access to it without adding them to the whole parent House — see
// the "granular sharing" design in CLAUDE.md and internal/handlers/shares.go
// for the backend half of this. This file owns two things: the shared
// create/revoke modal (opened from a list card's/space's 👥 button in
// app.js/spaces.js/list_view.js, via openShareModal) and the fifth
// dashboard tab, "Partagé avec moi" (`#tab-shared`/`#shared-view`, wired
// into planning.js's activeTab switcher the same way spaces.js's tab is).
// Shares `state`, `els`, `apiRequest`, `showError`/`hideError`, `t`,
// `TRASH_ICON_SVG`, `isNetworkError`, `buildListCard` with app.js, and
// `isSharedTabActive`/`activeTab` with planning.js (the tab switcher),
// `badgeFnForType` with spaces.js — same classic-<script>-tags shared-scope
// pattern as every other tab file.

const sharesEls = {
  shareModal: document.getElementById('share-modal'),
  shareModalTitle: document.getElementById('share-modal-title'),
  shareModalSubtitle: document.getElementById('share-modal-subtitle'),
  closeShareModalButton: document.getElementById('close-share-modal-button'),
  shareRoster: document.getElementById('share-roster'),
  shareForm: document.getElementById('share-form'),
  shareEmail: document.getElementById('share-email'),
  sharePermission: document.getElementById('share-permission'),
  sharedEmpty: document.getElementById('shared-empty'),
  sharedList: document.getElementById('shared-list'),
};

// ---------------------------------------------------------------------------
// Share modal: works identically for a List or a Space, distinguished only
// by which endpoint it talks to.
// ---------------------------------------------------------------------------

// { kind: 'list' | 'space', id, name } for whatever is currently open in the
// modal, set by openShareModal and read by shareForm's submit handler and
// buildShareRosterRow's revoke button.
let sharingTarget = null;

function shareEndpoint() {
  const root = sharingTarget.kind === 'space' ? '/custom-categories' : '/lists';
  return `${root}/${sharingTarget.id}/share`;
}

// Opens the share modal for either a List or a Space — called from a list
// card's/space's 👥 button (app.js's buildListCard, spaces.js's
// buildCategorySection) and the list-detail header's share button
// (list_view.js).
function openShareModal({ kind, id, name }) {
  sharingTarget = { kind, id, name };
  hideError();

  sharesEls.shareModalTitle.textContent = t(kind === 'space' ? 'modals.share.titleSpace' : 'modals.share.titleList');
  sharesEls.shareModalSubtitle.textContent = name;
  sharesEls.shareForm.reset();
  sharesEls.shareRoster.replaceChildren();

  sharesEls.shareModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  sharesEls.shareEmail.focus();

  loadShareRoster();
}

function closeShareModal() {
  sharesEls.shareModal.hidden = true;
  document.body.classList.remove('overflow-hidden');
  sharingTarget = null;
}

async function loadShareRoster() {
  if (!sharingTarget) return;
  let shares;
  try {
    shares = await apiRequest(shareEndpoint());
  } catch (err) {
    // No offline mirror for shares (the same "requires connectivity"
    // scoping as the members modal in app.js) — a plain connectivity
    // failure just leaves the roster empty without a blocking banner; a
    // genuine server-side error still gets one.
    if (!isNetworkError(err)) showError(err.message);
    return;
  }
  if (!sharingTarget) return; // modal was closed while the request was in flight

  sharesEls.shareRoster.replaceChildren();
  for (const share of shares) {
    sharesEls.shareRoster.appendChild(buildShareRosterRow(share));
  }
}

function buildShareRosterRow(share) {
  const li = document.createElement('li');
  li.className =
    'flex items-center justify-between gap-2 rounded-xl border border-slate-200 dark:border-slate-700 bg-white/60 dark:bg-slate-900/60 px-3 py-2';

  const info = document.createElement('div');
  info.className = 'min-w-0';
  const name = document.createElement('p');
  name.className = 'truncate text-sm font-medium text-slate-900 dark:text-slate-100';
  name.textContent = share.display_name || share.email;
  const permission = document.createElement('p');
  permission.className = 'text-xs text-slate-500 dark:text-slate-400';
  permission.textContent = t(share.permission === 'write' ? 'modals.share.permissionWrite' : 'modals.share.permissionRead');
  info.append(name, permission);
  li.appendChild(info);

  const revokeBtn = document.createElement('button');
  revokeBtn.type = 'button';
  revokeBtn.setAttribute('aria-label', t('modals.share.revokeAriaLabel', { email: share.email }));
  revokeBtn.className =
    'flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400';
  revokeBtn.innerHTML = TRASH_ICON_SVG;
  revokeBtn.addEventListener('click', () => revokeShare(share.shared_with_user_id));
  li.appendChild(revokeBtn);

  return li;
}

async function revokeShare(userId) {
  if (!sharingTarget) return;
  hideError();
  try {
    await apiRequest(`${shareEndpoint()}/${userId}`, { method: 'DELETE' });
    await loadShareRoster();
  } catch (err) {
    showError(err.message);
  }
}

sharesEls.closeShareModalButton.addEventListener('click', closeShareModal);
sharesEls.shareModal.addEventListener('click', (event) => {
  if (event.target === sharesEls.shareModal) closeShareModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !sharesEls.shareModal.hidden) closeShareModal();
});

sharesEls.shareForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  if (!sharingTarget) return;

  const email = sharesEls.shareEmail.value.trim();
  const permission = sharesEls.sharePermission.value;
  if (!email) return;

  try {
    await apiRequest(shareEndpoint(), { method: 'POST', body: JSON.stringify({ email, permission }) });
    sharesEls.shareForm.reset();
    await loadShareRoster();
  } catch (err) {
    showError(err.message);
  }
});

// ---------------------------------------------------------------------------
// "Partagé avec moi" tab: every List reachable via a direct share or a
// shared Space, regardless of which House it actually belongs to.
// ---------------------------------------------------------------------------

// The caller's shared-with-them lists, refreshed on every loadSharedView()
// call — re-fetched in full each time rather than kept incrementally in
// sync, the same "small, per-user dataset" reasoning spaces.js's
// customCategories already uses.
let sharedLists = [];

function refreshSharedIfActive() {
  if (isSharedTabActive()) loadSharedView();
}

async function loadSharedView() {
  let stubs;
  try {
    stubs = await apiRequest('/lists?shared_with_me=true');
  } catch (err) {
    // No offline mirror for cross-house shared lists (same scoping as the
    // share modal's roster above) — a plain connectivity failure just
    // leaves the tab showing whatever it last had instead of a blocking
    // banner; a genuine server-side error still gets one.
    if (!isNetworkError(err)) showError(err.message);
    return;
  }

  // ?shared_with_me=true only returns each list's own row (no items), same
  // as the plain ?house_id= listing — fetch each one's detail in parallel
  // for accurate item counts/badges, the same per-list fan-out pattern
  // planning.js/urgent.js/spaces.js already use. GET /lists/{id} doesn't
  // itself know about access_source/access_permission (only
  // db.ListSharedListsForUser does), so those two fields are carried over
  // from the stub onto the detailed result rather than lost.
  sharedLists = await Promise.all(
    stubs.map((stub) =>
      apiRequest(`/lists/${stub.id}`)
        .then((detailed) => ({ ...detailed, access_source: stub.access_source, access_permission: stub.access_permission }))
        .catch(() => stub)
    )
  );
  renderSharedList();
}

// badgeFnForType is defined in spaces.js (loaded before this file) — a
// shared list can be any type, same as a space's mixed-type list grid, so
// it's reused as-is rather than duplicated here.
function renderSharedList() {
  sharesEls.sharedEmpty.hidden = sharedLists.length > 0;
  sharesEls.sharedList.replaceChildren();
  for (const list of sharedLists) {
    sharesEls.sharedList.appendChild(buildListCard(list, badgeFnForType(list.type)(list.items || [])));
  }
}
