'use strict';

// Ultra-light i18n engine: no build step, no dependencies. Loaded as a
// classic <script> before app.js/list_view.js so `window.TrakkaI18n` is
// available to them (mirrors how those two files already share globals).
//
// Static markup is translated declaratively via data-i18n* attributes:
//   data-i18n="path.to.key"            -> element.textContent
//   data-i18n-attr="aria-label"        -> combined with data-i18n above to
//                                          target that attribute instead of
//                                          textContent
//   data-i18n-placeholder="path.key"   -> element's placeholder attribute
//   data-i18n-aria-label="path.key"    -> element's aria-label attribute
//
// Dynamic strings built in app.js/list_view.js (item counts, badges, error
// messages, ...) are not covered by data-i18n and should call
// `TrakkaI18n.t('some.key', { count })` directly where translation matters.
(function () {
  const LANG_STORAGE_KEY = 'trakka:lang';
  const SUPPORTED_LANGS = ['fr', 'en'];
  const DEFAULT_LANG = 'fr';

  // Bootstrap copy of fr.json's content (kept in sync with
  // static/locales/fr.json) so t() never returns a raw, untranslated key
  // during the brief window between page load and the /locales/*.json
  // fetch resolving — it just mirrors the French text already hardcoded
  // in index.html (this app's default language) until then.
  let translations = {
    common: {
      deleteList: 'Supprimer {name}',
      removeMember: 'Retirer {email}',
    },
    header: {
      backToDashboard: 'Retour au tableau de bord',
      logout: 'Se déconnecter',
      langSwitcher: 'Changer de langue',
      langFr: 'Français',
      langEn: 'English',
      online: 'En ligne',
      offline: 'Hors-ligne',
      pending: '{count} en attente',
      themeSwitcher: 'Changer de thème',
      themeLight: 'Clair',
      themeDark: 'Sombre',
      themeAuto: 'Système',
    },
    dashboard: {
      house: 'Maison',
      members: 'Membres',
      title: 'Tableau de bord',
      newList: 'Nouvelle liste',
      shoppingHeading: 'Achats & Sourcing',
      todoHeading: 'Espaces Tâches',
      createHouseOption: '+ Créer une Maison',
      tabLists: 'Listes',
    },
    items: {
      back: 'Retour au tableau de bord',
      financeTotal: 'Total estimé',
      financeSpent: 'Déjà dépensé',
      financeRemaining: 'Reste à dépenser',
      titlePlaceholder: 'Article ou tâche',
      urlPlaceholder: 'URL (optionnel)',
      quantityAriaLabel: 'Quantité',
      pricePlaceholder: 'Prix € (optionnel)',
      priceAriaLabel: 'Prix en euros',
      targetMonthAriaLabel: 'Mois prévu (optionnel)',
      add: 'Ajouter',
      doneDefault: 'Terminés (0)',
    },
    planning: {
      tabTitle: 'Budget & Prévisions',
      title: 'Budget & Prévisions Achats',
      horizon3: '3 prochains mois',
      horizon6: '6 prochains mois',
      horizon12: '12 prochains mois',
      totalLabel: 'Total prévu sur la période',
      emptyMonth: 'Aucun achat prévu ce mois-ci.',
      unscheduled: '— Non planifié —',
      moveAriaLabel: 'Déplacer {title} vers un autre mois',
    },
    modals: {
      close: 'Fermer',
      newList: {
        title: 'Nouvelle liste',
        nameLabel: 'Nom',
        namePlaceholder: 'Ex. Courses de la semaine',
        typeLabel: 'Type',
        typeShopping: 'Courses',
        typeTodo: 'Tâches',
        submit: 'Créer la liste',
      },
      newHouse: {
        title: 'Nouvelle Maison',
        nameLabel: 'Nom',
        namePlaceholder: 'Ex. Maison des Parents',
        submit: 'Créer la Maison',
      },
      members: {
        title: 'Membres de la Maison',
        invitePlaceholder: 'Email de la personne à inviter',
        inviteSubmit: 'Inviter',
      },
      editItem: {
        title: "Modifier l'article",
        nameLabel: 'Nom',
        urlLabel: 'URL (optionnel)',
        priceLabel: 'Prix (€, optionnel)',
        autoBadge: 'Détecté automatiquement',
        targetMonthLabel: 'Mois prévu (optionnel)',
        submit: 'Enregistrer',
      },
      imagePreview: {
        ariaLabel: "Aperçu de l'image de l'article",
        close: "Fermer l'aperçu",
      },
    },
    undo: {
      cancel: 'Annuler',
      listDeleted: 'Liste « {name} » supprimée',
      itemDeleted: '« {title} » supprimé',
      itemMarkedDone: '« {title} » marqué comme terminé',
      itemMarkedUndone: '« {title} » marqué comme à faire',
    },
  };
  let currentLang = DEFAULT_LANG;

  function detectLang() {
    const stored = localStorage.getItem(LANG_STORAGE_KEY);
    if (stored && SUPPORTED_LANGS.includes(stored)) return stored;
    const nav = (navigator.language || '').slice(0, 2).toLowerCase();
    return SUPPORTED_LANGS.includes(nav) ? nav : DEFAULT_LANG;
  }

  function lookup(key) {
    return key.split('.').reduce((acc, part) => (acc && typeof acc === 'object' ? acc[part] : undefined), translations);
  }

  // t() never throws and always returns a string — falling back to the raw
  // key when a translation is missing keeps a broken key visible (rather
  // than blank) without breaking the caller.
  function t(key, vars) {
    const value = lookup(key);
    let text = typeof value === 'string' ? value : key;
    if (vars) {
      for (const [name, replacement] of Object.entries(vars)) {
        text = text.replace(new RegExp(`\\{${name}\\}`, 'g'), String(replacement));
      }
    }
    return text;
  }

  function applyTranslations(root) {
    const scope = root || document;
    scope.querySelectorAll('[data-i18n]').forEach((el) => {
      const key = el.getAttribute('data-i18n');
      const attr = el.getAttribute('data-i18n-attr');
      if (attr) {
        el.setAttribute(attr, t(key));
      } else {
        el.textContent = t(key);
      }
    });
    scope.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
      el.setAttribute('placeholder', t(el.getAttribute('data-i18n-placeholder')));
    });
    scope.querySelectorAll('[data-i18n-aria-label]').forEach((el) => {
      el.setAttribute('aria-label', t(el.getAttribute('data-i18n-aria-label')));
    });
    document.documentElement.lang = currentLang;
  }

  async function loadLang(lang) {
    const response = await fetch(`/locales/${lang}.json`, { cache: 'no-store' });
    translations = await response.json();
    currentLang = lang;
    applyTranslations();
    updateLangButton();
    document.dispatchEvent(new CustomEvent('trakka:lang-changed', { detail: { lang } }));
  }

  async function setLang(lang) {
    if (!SUPPORTED_LANGS.includes(lang) || lang === currentLang) return;
    localStorage.setItem(LANG_STORAGE_KEY, lang);
    await loadLang(lang);
  }

  // ---------------------------------------------------------------------
  // Discreet FR/EN dropdown in the header. Wired here rather than app.js
  // so the whole feature (dictionaries + engine + selector) stays in this
  // one file, following the same close-on-Escape/click-outside pattern
  // already used by every modal in app.js/list_view.js.
  // ---------------------------------------------------------------------

  function updateLangButton() {
    const label = document.getElementById('lang-button-label');
    if (label) label.textContent = currentLang.toUpperCase();
    document.querySelectorAll('#lang-menu [data-lang]').forEach((btn) => {
      btn.setAttribute('aria-current', btn.getAttribute('data-lang') === currentLang ? 'true' : 'false');
    });
  }

  function wireLangSwitcher() {
    const button = document.getElementById('lang-button');
    const menu = document.getElementById('lang-menu');
    if (!button || !menu) return;

    function closeMenu() {
      menu.hidden = true;
      button.setAttribute('aria-expanded', 'false');
    }
    function toggleMenu() {
      const willOpen = menu.hidden;
      menu.hidden = !willOpen;
      button.setAttribute('aria-expanded', String(willOpen));
    }

    button.addEventListener('click', (event) => {
      event.stopPropagation();
      toggleMenu();
    });
    menu.querySelectorAll('[data-lang]').forEach((option) => {
      option.addEventListener('click', () => {
        setLang(option.getAttribute('data-lang'));
        closeMenu();
      });
    });
    document.addEventListener('click', (event) => {
      if (!menu.hidden && !menu.contains(event.target) && event.target !== button) closeMenu();
    });
    document.addEventListener('keydown', (event) => {
      if (event.key === 'Escape' && !menu.hidden) closeMenu();
    });
  }

  window.TrakkaI18n = { t, setLang, getLang: () => currentLang, applyTranslations };

  document.addEventListener('DOMContentLoaded', wireLangSwitcher);
  window.TrakkaI18n.ready = loadLang(detectLang());
})();
