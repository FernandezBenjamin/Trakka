'use strict';

// "Espaces" (custom categories) view: a fourth tab alongside the dashboard,
// the planning view and the urgent view (shares `state`, `els`, `apiRequest`,
// `showError`/`hideError`, `t`, `TRASH_ICON_SVG`, `refreshPendingBadge`,
// `buildListCard`, `urlBadges`, `progressBadge`, `noBadges`, `isPurchaseList`
// with app.js, and `PENCIL_ICON_SVG` with list_view.js — same classic-
// <script>-tags shared-scope pattern as planning.js/urgent.js). A custom
// category ("space") is a personal, freeform way to group lists across
// their fixed `type` (e.g. a "Homelab" space mixing a shopping list, a todo
// list and a notes list) — see the "Custom categories" bullet in CLAUDE.md
// for the backend design this talks to. This file also owns the create/
// edit category modal, reused both from this tab and from the "new list"
// modal's category picker in app.js (via the CREATE_CATEGORY_OPTION_VALUE
// sentinel and the onCreated callback openCategoryModal accepts).

const spacesEls = {
  tabBadge: document.getElementById('tab-spaces-badge'),
  newCategoryButton: document.getElementById('new-category-button'),
  emptyNewCategoryButton: document.getElementById('empty-new-category-button'),
  spacesEmpty: document.getElementById('spaces-empty'),
  spacesList: document.getElementById('spaces-list'),
  categoryModal: document.getElementById('category-modal'),
  categoryModalTitle: document.getElementById('category-modal-title'),
  closeCategoryModalButton: document.getElementById('close-category-modal-button'),
  categoryForm: document.getElementById('category-form'),
  categoryName: document.getElementById('category-name'),
  categoryIcon: document.getElementById('category-icon'),
  categoryColor: document.getElementById('category-color'),
  iconPresetButtons: document.querySelectorAll('#category-form [data-icon-preset]'),
  colorPresetButtons: document.querySelectorAll('#category-form [data-color-preset]'),
  deleteCategoryButton: document.getElementById('delete-category-button'),
  categorySubmitButton: document.getElementById('category-submit-button'),
};

// Sentinel option value for the "new list" modal's category <select>,
// mirroring CREATE_HOUSE_OPTION_VALUE in app.js — picking it opens the
// create-category modal instead of actually selecting a category.
const CREATE_CATEGORY_OPTION_VALUE = '__create__';

// The caller's custom categories, refreshed on every loadCustomCategories()
// call (startup, opening the Spaces tab, opening the "new list" modal, and
// after any category create/edit/delete) rather than kept incrementally in
// sync — the dataset is small and per-user, so a full refetch is cheap.
let customCategories = [];

// Every list in the current house that has a custom_category_id, each with
// its items already attached (a per-list `GET /lists/{id}` fan-out, the same
// pattern urgent.js/planning.js use) so buildListCard's item count and
// per-type badge are accurate — re-fetched on every loadSpacesView() call.
let spacesLists = [];

// The category currently open in the modal for editing, or null when the
// modal is in "create" mode — set by openCategoryModal, read by
// categoryForm's submit handler and the delete button.
let editingCategory = null;

// Set by openCategoryModal's caller (currently only app.js's "new list"
// modal) to learn about a category created through this modal without a
// tighter coupling between the two files — cleared on close.
let categoryCreatedCallback = null;

const CHEVRON_ICON_SVG =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-4 w-4 shrink-0 transition-transform group-open:rotate-90" aria-hidden="true"><path d="M9 6l6 6-6 6"/></svg>';

// Fetches the caller's custom categories and updates the "Espaces" tab's
// highlight dot as a side effect — called at startup (app.js's init), every
// time the Spaces tab is opened, and before the "new list" modal's category
// picker is populated, so all three stay in sync with the latest set.
// Errors are swallowed (falling back to []) rather than shown via the error
// banner: this is a background/supporting fetch, not the user's primary
// action — loadSpacesView below surfaces its own error for the tab's main
// fetch (the house's lists) instead.
async function loadCustomCategories() {
  try {
    customCategories = await apiRequest('/custom-categories');
  } catch {
    customCategories = [];
  }
  updateSpacesTabBadge();
  return customCategories;
}

function updateSpacesTabBadge() {
  spacesEls.tabBadge.hidden = customCategories.length === 0;
}

// Fills a <select> (either #list-category in the "new list" modal, or a
// future caller) with "Aucun", one option per known category, and the
// "+ Créer un nouvel espace" sentinel — always rebuilt from scratch rather
// than diffed, since the list is short and this only runs when a modal
// opens.
function populateCategorySelect(selectEl, selectedId) {
  selectEl.replaceChildren();

  const noneOption = document.createElement('option');
  noneOption.value = '';
  noneOption.textContent = t('modals.newList.categoryNone');
  selectEl.appendChild(noneOption);

  for (const category of customCategories) {
    const option = document.createElement('option');
    option.value = String(category.id);
    option.textContent = category.icon ? `${category.icon} ${category.name}` : category.name;
    selectEl.appendChild(option);
  }

  const createOption = document.createElement('option');
  createOption.value = CREATE_CATEGORY_OPTION_VALUE;
  createOption.textContent = t('modals.newList.categoryCreateOption');
  selectEl.appendChild(createOption);

  selectEl.value = selectedId !== null && selectedId !== undefined ? String(selectedId) : '';
}

// ---------------------------------------------------------------------------
// "Espaces" tab: categories rendered as collapsible sections, each holding
// every list (any type) attached to it.
// ---------------------------------------------------------------------------

function isSpacesTabActive() {
  // activeTab/setActiveTab live in planning.js, loaded before this file.
  return activeTab === 'spaces';
}

async function loadSpacesView() {
  await loadCustomCategories();

  if (state.currentHouseId === null) {
    spacesLists = [];
    renderSpaces();
    return;
  }

  try {
    const lists = await apiRequest(`/lists?house_id=${state.currentHouseId}`);
    const categorized = lists.filter((list) => list.custom_category_id);
    spacesLists = await Promise.all(
      categorized.map((list) => apiRequest(`/lists/${list.id}`).catch(() => ({ ...list, items: [] })))
    );
  } catch (err) {
    showError(err.message);
    spacesLists = [];
  }
  renderSpaces();
}

// Called after anything that might have changed the current house's
// categorized lists (offline sync completing, a language switch, a house
// switch, a list/category create-edit-delete) — a no-op unless this tab is
// the one currently visible.
function refreshSpacesIfActive() {
  if (isSpacesTabActive()) loadSpacesView();
}

// A purchase-oriented list shows its sourcing-domain badges, a todo list
// shows its completion progress, and a custom (freeform notes) list shows
// nothing extra — mirrors renderDashboardGrids' per-section badge choice in
// app.js, just resolved per-list here since a single space can mix types.
function badgeFnForType(type) {
  if (type === 'todo') return progressBadge;
  if (type === 'custom') return noBadges;
  return urlBadges;
}

function renderSpaces() {
  const sorted = [...customCategories].sort((a, b) => a.position - b.position || a.id - b.id);
  spacesEls.spacesEmpty.hidden = sorted.length > 0;
  spacesEls.spacesList.replaceChildren();
  for (const category of sorted) {
    spacesEls.spacesList.appendChild(buildCategorySection(category));
  }
}

function buildCategorySection(category) {
  const lists = spacesLists.filter((list) => list.custom_category_id === category.id);

  const details = document.createElement('details');
  details.open = true;
  details.className = 'group rounded-2xl border border-slate-200 dark:border-slate-800 bg-slate-50 dark:bg-slate-800/30 p-4';

  const summary = document.createElement('summary');
  summary.className =
    'flex cursor-pointer list-none items-center justify-between gap-2 marker:hidden [&::-webkit-details-marker]:hidden';

  const heading = document.createElement('div');
  heading.className = 'flex min-w-0 items-center gap-2';

  const chevron = document.createElement('span');
  chevron.className = 'flex shrink-0 items-center';
  chevron.innerHTML = CHEVRON_ICON_SVG; // static markup, no interpolated data — safe, see TRASH_ICON_SVG's comment in app.js
  heading.appendChild(chevron);

  const iconSpan = document.createElement('span');
  iconSpan.setAttribute('aria-hidden', 'true');
  iconSpan.className = 'text-xl leading-none';
  iconSpan.textContent = category.icon || '📁';
  heading.appendChild(iconSpan);

  const name = document.createElement('h2');
  name.className = 'truncate text-base font-semibold text-slate-900 dark:text-slate-100';
  name.textContent = category.name;
  heading.appendChild(name);

  const count = document.createElement('span');
  count.className =
    'shrink-0 rounded-full bg-slate-200/60 dark:bg-slate-700/50 px-2 py-0.5 text-xs font-medium text-slate-600 dark:text-slate-300';
  count.textContent = t('spaces.listCount', { count: lists.length });
  heading.appendChild(count);

  summary.appendChild(heading);

  const actions = document.createElement('div');
  actions.className = 'flex shrink-0 items-center gap-1';

  const editBtn = document.createElement('button');
  editBtn.type = 'button';
  editBtn.setAttribute('aria-label', t('spaces.editAriaLabel', { name: category.name }));
  editBtn.className =
    'flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-200 dark:hover:bg-slate-700 hover:text-slate-900 dark:hover:text-slate-200';
  editBtn.innerHTML = PENCIL_ICON_SVG;
  editBtn.addEventListener('click', (event) => {
    event.preventDefault(); // inside a <summary> — don't also toggle the <details>
    openCategoryModal(category);
  });
  actions.appendChild(editBtn);

  const deleteBtn = document.createElement('button');
  deleteBtn.type = 'button';
  deleteBtn.setAttribute('aria-label', t('spaces.deleteAriaLabel', { name: category.name }));
  deleteBtn.className =
    'flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 hover:bg-rose-500/10 hover:text-rose-600 dark:hover:text-rose-400';
  deleteBtn.innerHTML = TRASH_ICON_SVG;
  deleteBtn.addEventListener('click', (event) => {
    event.preventDefault();
    deleteCategory(category);
  });
  actions.appendChild(deleteBtn);

  summary.appendChild(actions);
  details.appendChild(summary);

  const body = document.createElement('div');
  body.className = 'mt-4';
  if (lists.length === 0) {
    const empty = document.createElement('p');
    empty.className = 'text-sm text-slate-500';
    empty.textContent = t('spaces.emptyCategory');
    body.appendChild(empty);
  } else {
    const grid = document.createElement('ul');
    grid.className = 'grid grid-cols-1 gap-3 md:grid-cols-2 lg:grid-cols-3';
    for (const list of lists) grid.appendChild(buildListCard(list, badgeFnForType(list.type)(list.items || [])));
    body.appendChild(grid);
  }
  details.appendChild(body);

  return details;
}

// Deletion is deferred behind a 5s undo grace period, mirroring removeList
// in app.js — the category disappears from the tab right away, and the
// actual DELETE only fires from TrakkaUndo's onCommit if the countdown runs
// out without the user clicking "Annuler". A deleted category's lists are
// never themselves deleted (the backend detaches them via ON DELETE SET
// NULL — see CLAUDE.md's "Custom categories" bullet), so they simply drop
// out of every space and become reachable from the ordinary dashboard tabs
// again once the delete actually lands.
function deleteCategory(category) {
  hideError();
  customCategories = customCategories.filter((c) => c.id !== category.id);
  updateSpacesTabBadge();
  refreshSpacesIfActive();

  TrakkaUndo.schedule({
    message: t('undo.categoryDeleted', { name: category.name }),
    undoLabel: t('undo.cancel'),
    onUndo: async () => {
      await loadCustomCategories();
      await refreshSpacesIfActive();
    },
    onCommit: async () => {
      try {
        await apiRequest(`/custom-categories/${category.id}`, { method: 'DELETE' });
      } catch (err) {
        showError(err.message);
      }
      await loadCustomCategories();
      await refreshSpacesIfActive();
    },
  });
}

// ---------------------------------------------------------------------------
// Create/edit category modal
// ---------------------------------------------------------------------------

// Opens the modal in create mode (category === null) or edit mode
// (prefilled from an existing category). `onCreated(category)`, when given,
// fires once after a successful *create* only (not an edit) — used by
// app.js's "new list" modal to select the just-created category in its
// picker without this file needing to know anything about that form.
function openCategoryModal(category, { onCreated } = {}) {
  editingCategory = category || null;
  categoryCreatedCallback = onCreated || null;

  spacesEls.categoryForm.reset();
  spacesEls.categoryName.value = category?.name || '';
  spacesEls.categoryIcon.value = category?.icon || '';
  spacesEls.categoryColor.value = category?.color || '#0ea5e9';
  spacesEls.categoryModalTitle.textContent = t(editingCategory ? 'modals.newCategory.titleEdit' : 'modals.newCategory.titleCreate');
  spacesEls.categorySubmitButton.textContent = t(editingCategory ? 'modals.newCategory.submitEdit' : 'modals.newCategory.submitCreate');
  spacesEls.deleteCategoryButton.hidden = !editingCategory;

  spacesEls.categoryModal.hidden = false;
  document.body.classList.add('overflow-hidden');
  spacesEls.categoryName.focus();
}

function closeCategoryModal() {
  spacesEls.categoryModal.hidden = true;
  // This modal can be opened on top of the "new list" modal (via its
  // category picker's "+ Créer un nouvel espace" option), so only release
  // the shared body scroll-lock if that other modal isn't still open —
  // otherwise closing this one would let the page scroll behind it.
  if (els.newListModal.hidden) {
    document.body.classList.remove('overflow-hidden');
  }
  editingCategory = null;
  categoryCreatedCallback = null;
}

spacesEls.newCategoryButton.addEventListener('click', () => openCategoryModal(null));
spacesEls.emptyNewCategoryButton.addEventListener('click', () => openCategoryModal(null));
spacesEls.closeCategoryModalButton.addEventListener('click', closeCategoryModal);
spacesEls.categoryModal.addEventListener('click', (event) => {
  if (event.target === spacesEls.categoryModal) closeCategoryModal();
});
document.addEventListener('keydown', (event) => {
  if (event.key === 'Escape' && !spacesEls.categoryModal.hidden) closeCategoryModal();
});

for (const button of spacesEls.iconPresetButtons) {
  button.addEventListener('click', () => {
    spacesEls.categoryIcon.value = button.dataset.iconPreset;
  });
}
for (const button of spacesEls.colorPresetButtons) {
  button.addEventListener('click', () => {
    spacesEls.categoryColor.value = button.dataset.colorPreset;
  });
}

spacesEls.deleteCategoryButton.addEventListener('click', () => {
  if (!editingCategory) return;
  const category = editingCategory;
  closeCategoryModal();
  deleteCategory(category);
});

spacesEls.categoryForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  hideError();

  const name = spacesEls.categoryName.value.trim();
  if (!name) return;
  const payload = {
    name,
    icon: spacesEls.categoryIcon.value.trim(),
    color: spacesEls.categoryColor.value,
    position: editingCategory?.position ?? 0,
  };

  try {
    const isEdit = editingCategory !== null;
    const category = isEdit
      ? await apiRequest(`/custom-categories/${editingCategory.id}`, { method: 'PUT', body: JSON.stringify(payload) })
      : await apiRequest('/custom-categories', { method: 'POST', body: JSON.stringify(payload) });
    await loadCustomCategories();
    const callback = !isEdit ? categoryCreatedCallback : null;
    closeCategoryModal();
    if (callback) callback(category);
    await refreshSpacesIfActive();
  } catch (err) {
    showError(err.message);
  }
});
