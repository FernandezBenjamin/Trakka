'use strict';

// Admin "Paramètres du Système" panel: a header button (hidden for
// non-admins) opening a modal that reads/writes GET+PATCH
// /api/v1/admin/settings — instance name, whether local registration is
// open, and the OIDC/SSO configuration (see internal/handlers/admin.go).
// Shares `state`, `apiRequest`, `showError`/`hideError`, `t` with
// app.js/list_view.js/planning.js/notifications.js — same classic-
// <script>-tags shared-scope pattern as those files.
//
// Like the notifications bell, this is a header-level control rather than a
// dashboard tab, so it has its own open/close wiring here instead of going
// through refreshVisibleView.

const adminEls = {
  button: document.getElementById('admin-settings-button'),
  modal: document.getElementById('admin-settings-modal'),
  closeButton: document.getElementById('close-admin-settings-modal-button'),
  form: document.getElementById('admin-settings-form'),
  instanceName: document.getElementById('admin-instance-name'),
  registrationOpen: document.getElementById('admin-registration-open'),
  oidcEnabled: document.getElementById('admin-oidc-enabled'),
  oidcIssuer: document.getElementById('admin-oidc-issuer'),
  oidcClientId: document.getElementById('admin-oidc-client-id'),
  oidcClientSecret: document.getElementById('admin-oidc-client-secret'),
  status: document.getElementById('admin-settings-status'),
};

// Called once state.currentUser is known (see app.js's init) and again
// after a language switch's re-render — cheap, and keeps the button's
// visibility correct even if currentUser was still null the first time
// (e.g. the /me call failed while offline and only resolved later).
function refreshAdminButtonVisibility() {
  adminEls.button.hidden = !state.currentUser || !state.currentUser.is_admin;
}

function applySecretPlaceholder(secretSet) {
  adminEls.oidcClientSecret.value = '';
  adminEls.oidcClientSecret.placeholder = secretSet
    ? t('modals.adminSettings.oidcClientSecretPlaceholderSet')
    : t('modals.adminSettings.oidcClientSecretPlaceholderUnset');
}

async function loadAdminSettings() {
  let settings;
  try {
    settings = await apiRequest('/admin/settings');
  } catch (err) {
    showError(err.message);
    return;
  }
  adminEls.instanceName.value = settings.instance_name;
  adminEls.registrationOpen.checked = settings.registration_open;
  adminEls.oidcEnabled.checked = settings.oidc_enabled;
  adminEls.oidcIssuer.value = settings.oidc_issuer;
  adminEls.oidcClientId.value = settings.oidc_client_id;
  applySecretPlaceholder(settings.oidc_client_secret_set);
}

function openAdminSettingsModal() {
  adminEls.status.hidden = true;
  adminEls.modal.hidden = false;
  document.body.classList.add('overflow-hidden');
  loadAdminSettings();
}

function closeAdminSettingsModal() {
  adminEls.modal.hidden = true;
  document.body.classList.remove('overflow-hidden');
}

adminEls.button.addEventListener('click', openAdminSettingsModal);
adminEls.closeButton.addEventListener('click', closeAdminSettingsModal);
adminEls.modal.addEventListener('click', (event) => {
  if (event.target === adminEls.modal) closeAdminSettingsModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !adminEls.modal.hidden) closeAdminSettingsModal();
});

adminEls.form.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();
  adminEls.status.hidden = true;

  const body = {
    instance_name: adminEls.instanceName.value.trim(),
    registration_open: adminEls.registrationOpen.checked,
    oidc_enabled: adminEls.oidcEnabled.checked,
    oidc_issuer: adminEls.oidcIssuer.value.trim(),
    oidc_client_id: adminEls.oidcClientId.value.trim(),
  };
  // The secret field only ever carries a *new* value: it's never
  // pre-filled with the stored one (see adminSettingsView server-side), so
  // an empty field always means "leave it as-is", matching the backend's
  // own "empty = unchanged" convention for this one field.
  if (adminEls.oidcClientSecret.value) {
    body.oidc_client_secret = adminEls.oidcClientSecret.value;
  }

  let settings;
  try {
    settings = await apiRequest('/admin/settings', { method: 'PATCH', body: JSON.stringify(body) });
  } catch (err) {
    showError(err.message);
    return;
  }

  adminEls.oidcIssuer.value = settings.oidc_issuer;
  adminEls.oidcClientId.value = settings.oidc_client_id;
  applySecretPlaceholder(settings.oidc_client_secret_set);
  adminEls.status.textContent = t('modals.adminSettings.saved');
  adminEls.status.hidden = false;
});
