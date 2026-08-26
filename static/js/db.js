'use strict';

// Trakka's IndexedDB layer. This file is loaded two ways: as a plain
// <script> on the page, and via importScripts() inside the service worker.
// It must therefore never touch window/document — only `indexedDB` and
// `self`, both of which exist in a Window (where `self === window`) and in
// a ServiceWorkerGlobalScope alike. Attaching to `self.TrakkaDB` makes the
// same implementation reachable from both places.
(function () {
  const DB_NAME = 'trakka';
  const DB_VERSION = 2;

  const STORE_HOUSES = 'houses';
  const STORE_LISTS = 'lists';
  const STORE_ITEMS = 'items';
  const STORE_QUEUE = 'queue';

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
      };

      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error);
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
    getItem,
    getItemsByList,
    putItems,
    putItem,
    deleteItem,
    enqueueRequest,
    getQueue,
    updateQueueEntry,
    dequeue,
  };
})();
