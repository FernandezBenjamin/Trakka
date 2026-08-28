'use strict';

// Trakka's IndexedDB layer. This file is loaded two ways: as a plain
// <script> on the page, and via importScripts() inside the service worker.
// It must therefore never touch window/document — only `indexedDB` and
// `self`, both of which exist in a Window (where `self === window`) and in
// a ServiceWorkerGlobalScope alike. Attaching to `self.TrakkaDB` makes the
// same implementation reachable from both places.
(function () {
  const DB_NAME = 'trakka';
  const DB_VERSION = 3;

  const STORE_HOUSES = 'houses';
  const STORE_LISTS = 'lists';
  const STORE_ITEMS = 'items';
  const STORE_QUEUE = 'queue';
  const STORE_CUSTOM_CATEGORIES = 'custom_categories';

  let dbPromise = null;

  function open() {
    if (dbPromise) return dbPromise;

    dbPromise = new Promise((resolve, reject) => {
      const request = indexedDB.open(DB_NAME, DB_VERSION);

      request.onupgradeneeded = () => {
        const db = request.result;

        if (!db.objectStoreNames.contains(STORE_HOUSES)) {
          db.createObjectStore(STORE_HOUSES, { keyPath: 'id' });
        }
        if (!db.objectStoreNames.contains(STORE_LISTS)) {
          db.createObjectStore(STORE_LISTS, { keyPath: 'id' });
        }
        if (!db.objectStoreNames.contains(STORE_ITEMS)) {
          const items = db.createObjectStore(STORE_ITEMS, { keyPath: 'id' });
          items.createIndex('list_id', 'list_id');
        }
        if (!db.objectStoreNames.contains(STORE_QUEUE)) {
          db.createObjectStore(STORE_QUEUE, { keyPath: 'id', autoIncrement: true });
        }
        // v3: mirror of GET /api/v1/custom-categories, added so the
        // "Espaces" tab (static/js/spaces.js) has something to show while
        // offline — same read-through mirror pattern as houses/lists/items.
        if (!db.objectStoreNames.contains(STORE_CUSTOM_CATEGORIES)) {
          db.createObjectStore(STORE_CUSTOM_CATEGORIES, { keyPath: 'id' });
        }
      };

      request.onsuccess = () => resolve(request.result);
      request.onerror = () => {
        // Don't memoize a permanently-rejected promise: every read/write
        // helper below shares this same cached dbPromise, so a transient
        // failure here (a browser hiccup, a version-upgrade conflict with
        // another open tab, ...) would otherwise disable the entire
        // IndexedDB mirror — and with it every offline fallback in
        // app.js/list_view.js/planning.js/urgent.js/spaces.js — for the rest
        // of this page's lifetime. Clearing it lets the next open() call
        // retry with a fresh request instead of replaying the same failure
        // forever.
        dbPromise = null;
        reject(request.error);
      };
    });

    return dbPromise;
  }

  function requestToPromise(request) {
    return new Promise((resolve, reject) => {
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
    });
  }

  function store(name, mode) {
    return open().then((db) => db.transaction(name, mode).objectStore(name));
  }

  function getAll(name) {
    return store(name, 'readonly').then((s) => requestToPromise(s.getAll()));
  }

  function getOne(name, key) {
    return store(name, 'readonly').then((s) => requestToPromise(s.get(key)));
  }

  function putOne(name, value) {
    return store(name, 'readwrite').then((s) => requestToPromise(s.put(value)));
  }

  function putMany(name, values) {
    return store(name, 'readwrite').then((s) => Promise.all(values.map((v) => requestToPromise(s.put(v)))));
  }

  function deleteOne(name, key) {
    return store(name, 'readwrite').then((s) => requestToPromise(s.delete(key)));
  }

  function getAllByIndex(name, indexName, value) {
    return store(name, 'readonly').then((s) => requestToPromise(s.index(indexName).getAll(value)));
  }

  // ---- Houses mirror ------------------------------------------------------
  //
  // Same read-through mirror pattern as lists/items, kept small (houses are
  // rarely created, and only online — see sw.js) purely so the "Maison"
  // selector and the dashboard still have something to show while offline.

  function getHouses() {
    return getAll(STORE_HOUSES);
  }

  function putHouses(houses) {
    return putMany(STORE_HOUSES, houses);
  }

  function getHouse(id) {
    return getOne(STORE_HOUSES, id);
  }

  // Offline-fallback convenience for app.js/list_view.js: the mirror has no
  // index on lists.house_id (see the comment on deleteHouse above for why a
  // full scan is fine here), so this filters in memory rather than adding one.
  function getListsByHouse(houseId) {
    return getLists().then((lists) => lists.filter((list) => list.house_id === houseId));
  }

  function putHouse(house) {
    return putOne(STORE_HOUSES, house);
  }

  // Cascades to every list (and, via deleteList, every item) filed under
  // this house — mirrors the server's ON DELETE CASCADE chain. No dedicated
  // index on lists.house_id: the mirror is small enough that a full-store
  // scan is cheap, and adding one would only matter at a scale this app
  // never reaches.
  function deleteHouse(id) {
    return getAll(STORE_LISTS)
      .then((lists) => lists.filter((list) => list.house_id === id))
      .then((lists) => Promise.all(lists.map((list) => deleteList(list.id))))
      .then(() => deleteOne(STORE_HOUSES, id));
  }

  // ---- Lists & items mirror ----------------------------------------------
  //
  // A local mirror of whatever the API last returned, kept up to date by
  // the service worker on every successful request. It exists so the app
  // still has something to show when the network is unreachable, and so
  // the service worker can synthesize an API-shaped response for GET
  // requests made while offline.

  function getLists() {
    return getAll(STORE_LISTS);
  }

  function putLists(lists) {
    return putMany(STORE_LISTS, lists);
  }

  function getList(id) {
    return getOne(STORE_LISTS, id);
  }

  // Composite read used by app.js/list_view.js's offline fallbacks — mirrors
  // the shape a successful `GET /api/v1/lists/{id}` returns (list fields
  // plus an `items` array), so callers can treat a cache hit and a live
  // response identically. Resolves to null if the list itself isn't mirrored.
  function getListWithItems(id) {
    return getList(id).then((list) => {
      if (!list) return null;
      return getItemsByList(id).then((items) => ({ ...list, items }));
    });
  }

  function putList(list) {
    return putOne(STORE_LISTS, list);
  }

  function deleteList(id) {
    return getAllByIndex(STORE_ITEMS, 'list_id', id)
      .then((items) => Promise.all(items.map((item) => deleteOne(STORE_ITEMS, item.id))))
      .then(() => deleteOne(STORE_LISTS, id));
  }

  function getItem(id) {
    return getOne(STORE_ITEMS, id);
  }

  function getItemsByList(listId) {
    return getAllByIndex(STORE_ITEMS, 'list_id', listId);
  }

  function putItems(items) {
    return putMany(STORE_ITEMS, items);
  }

  function putItem(item) {
    return putOne(STORE_ITEMS, item);
  }

  function deleteItem(id) {
    return deleteOne(STORE_ITEMS, id);
  }

  // ---- Offline sync queue -------------------------------------------------
  //
  // One entry per mutating API call that could not reach the server. The
  // service worker is the only reader/writer of this store at replay time;
  // `tempId` / `dependsOnListTempId` let it patch a still-queued request's
  // body once an earlier queued "create" receives its real server id (e.g.
  // items added to a list that was itself created while offline).

  function enqueueRequest(entry) {
    return putOne(STORE_QUEUE, entry).then((id) => Object.assign(entry, { id }));
  }

  function getQueue() {
    return getAll(STORE_QUEUE);
  }

  function updateQueueEntry(entry) {
    return putOne(STORE_QUEUE, entry);
  }

  function dequeue(id) {
    return deleteOne(STORE_QUEUE, id);
  }

  // ---- Custom categories ("Spaces") mirror --------------------------------
  //
  // Same read-through mirror pattern as houses/lists/items: kept up to date
  // by the service worker on every successful GET/POST/PUT/DELETE against
  // /api/v1/custom-categories, so the "Espaces" tab still has something to
  // show while offline instead of going blank. There is no offline-write
  // support for categories (unlike lists/items) — see sw.js's
  // queueOfflineWrite — so this mirror is purely a read fallback.

  function getCustomCategories() {
    return getAll(STORE_CUSTOM_CATEGORIES);
  }

  function putCustomCategories(categories) {
    return putMany(STORE_CUSTOM_CATEGORIES, categories);
  }

  function getCustomCategory(id) {
    return getOne(STORE_CUSTOM_CATEGORIES, id);
  }

  function putCustomCategory(category) {
    return putOne(STORE_CUSTOM_CATEGORIES, category);
  }

  function deleteCustomCategory(id) {
    return deleteOne(STORE_CUSTOM_CATEGORIES, id);
  }

  self.TrakkaDB = {
    getHouses,
    putHouses,
    getHouse,
    putHouse,
    deleteHouse,
    getLists,
    putLists,
    getList,
    putList,
    deleteList,
    getListsByHouse,
    getListWithItems,
    getItem,
    getItemsByList,
    putItems,
    putItem,
    deleteItem,
    enqueueRequest,
    getQueue,
    updateQueueEntry,
    dequeue,
    getCustomCategories,
    putCustomCategories,
    getCustomCategory,
    putCustomCategory,
    deleteCustomCategory,
  };
})();
