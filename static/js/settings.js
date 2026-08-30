'use strict';

// User "Paramètres" panel: a header button, visible to every signed-in user
// (unlike the admin-only "Paramètres du Système" panel in admin.js), opening
// a modal that reads/writes GET+PATCH /api/v1/me. Currently just
// keep_last_page — the "reopen on the last tab/list visited" preference,
// whose actual save/restore machinery (LAST_VIEW_STORAGE_KEY,
// isKeepLastPageEnabled, saveLastView/loadLastView/restoreLastView) lives in
// app.js, hooked from list_view.js's selectList/showDashboard and
// planning.js's setActiveTab — this file only owns the toggle's UI and
// persisting it to the user's profile. Shares `state`, `apiRequest`,
// `showError`/`hideError`, `t`, `isKeepLastPageEnabled`,
// `setKeepLastPagePreference` with app.js/list_view.js/planning.js/admin.js
// — same classic-<script>-tags shared-scope pattern as those files.
//
// Like the notifications bell and the admin panel, this is a header-level
// control rather than a dashboard tab, so it has its own open/close wiring
// here instead of going through refreshVisibleView.

const userSettingsEls = {
  button: document.getElementById('user-settings-button'),
  modal: document.getElementById('user-settings-modal'),
  closeButton: document.getElementById('close-user-settings-modal-button'),
  form: document.getElementById('user-settings-form'),
  keepLastPage: document.getElementById('user-settings-keep-last-page'),
  status: document.getElementById('user-settings-status'),
};

function openUserSettingsModal() {
  userSettingsEls.status.hidden = true;
  // isKeepLastPageEnabled is defined in app.js — prefers state.currentUser's
  // server value, falling back to the localStorage mirror if /me hasn't
  // resolved yet (e.g. opened while offline).
  userSettingsEls.keepLastPage.checked = isKeepLastPageEnabled();
  userSettingsEls.modal.hidden = false;
  document.body.classList.add('overflow-hidden');
}

function closeUserSettingsModal() {
  userSettingsEls.modal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

userSettingsEls.button.addEventListener('click', openUserSettingsModal);
userSettingsEls.closeButton.addEventListener('click', closeUserSettingsModal);
userSettingsEls.modal.addEventListener('click', (event) => {
  if (event.target === userSettingsEls.modal) closeUserSettingsModal();
});
document.addEventListener('keydown', (event) => {
  // install-help.js's modal can open on top of this one (from the button
  // added to the form below) — when it's the one currently visible, let its
  // own Escape handler close just that one instead of both at once (same
  // pattern as app.js/spaces.js's new-list/category modal pair).
  if (event.key === 'Escape' && !userSettingsEls.modal.hidden && installHelpEls.modal.hidden) closeUserSettingsModal();
});

userSettingsEls.form.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  userSettingsEls.status.hidden = true;

  const keepLastPage = userSettingsEls.keepLastPage.checked;
  let user;
  try {
    user = await apiRequest('/me', { method: 'PATCH', body: JSON.stringify({ keep_last_page: keepLastPage }) });
  } catch (err) {
    showError(err.message);
    return;
  }

  state.currentUser = user;
  // setKeepLastPagePreference is defined in app.js — keeps the localStorage
  // mirror in step immediately, rather than waiting for the next reload's
  // /me call to do it.
  setKeepLastPagePreference(user.keep_last_page);
  userSettingsEls.status.textContent = t('modals.userSettings.saved');
  userSettingsEls.status.hidden = false;
});
