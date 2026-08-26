'use strict';

// Trakka service worker: offline-first app shell + an IndexedDB-backed
// sync queue for API mutations made while offline. Uses static/js/db.js
// for all persistence (see that file for why it's importScripts()-safe).
importScripts('/js/db.js');

// Bump both on any change to APP_SHELL's/CDN_ASSETS' contents so activate()
// evicts the old cache instead of serving stale assets forever.
const SHELL_CACHE = 'trakka-shell-v10';
const RUNTIME_CACHE = 'trakka-runtime-v10';
const KNOWN_CACHES = [SHELL_CACHE, RUNTIME_CACHE];

const APP_SHELL = [
  '/',
  '/index.html',
  '/js/i18n.js',
  '/js/app.js',
  '/js/db.js',
  '/js/undo.js',
  '/js/list_view.js',
  '/js/planning.js',
  '/css/base.css',
  '/css/tokens.css',
  '/locales/fr.json',
  '/locales/en.json',
  '/manifest.json',
  '/icons/favicon.ico',
  '/icons/trakka-favicon.svg',
  '/icons/apple-touch-icon-180.png',
  '/icons/trakka-icon-192.png',
  '/icons/trakka-icon-512.png',
  '/icons/trakka-maskable-192.png',
  '/icons/trakka-maskable-512.png',
  '/icons/trakka-lockup-dark-bg.svg',
];

// index.html's styling depends entirely on this cross-origin script (the
// Tailwind Play CDN) actually loading — without it cached, the app shell
// would come back from cache while offline but render completely
// unstyled. Cached separately from APP_SHELL (best-effort, not part of
// the atomic install) so a CDN hiccup at install time can't break the
// installation of Trakka's own app shell.
const CDN_ASSETS = ['https://cdn.tailwindcss.com'];

const API_PREFIX = '/api/v1';
const SYNC_TAG = 'trakka-sync-queue';

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(SHELL_CACHE)
      .then((cache) => cache.addAll(APP_SHELL).then(() => cache))
      .then((cache) => Promise.all(CDN_ASSETS.map((url) => precacheCdnAsset(cache, url))))
      .then(() => self.skipWaiting())
  );
});

// Fetched with mode 'no-cors' so caching succeeds even without CORS
// headers from the CDN — the resulting "opaque" response can be replayed
// later but never inspected (status/body unreadable), which is fine here
// since we only ever need to serve it back to a <script> tag, never read
// it ourselves. Failures are swallowed: an unreachable CDN at install
// time must not fail the install of Trakka's own app shell.
function precacheCdnAsset(cache, url) {
  return fetch(url, { mode: 'no-cors' })
    .then((response) => cache.put(url, response))
    .catch(() => {});
}

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(
        keys.filter((key) => !KNOWN_CACHES.includes(key)).map((key) => caches.delete(key))
      ))
      .then(() => self.clients.claim())
      .then(() => flushQueue())
  );
});

// Real Background Sync (Chrome/Android): fires even if no tab is open.
self.addEventListener('sync', (event) => {
  if (event.tag === SYNC_TAG) {
    event.waitUntil(flushQueue());
  }
});

// Fallback for browsers without Background Sync (notably iOS/iPadOS
// Safari, which never fires 'sync' at all): the page posts this when its
// own 'online' event fires.
self.addEventListener('message', (event) => {
  if (event.data && event.data.type === 'flush-queue') {
    event.waitUntil(flushQueue());
  }
});

// ---------------------------------------------------------------------------
// Fetch routing
// ---------------------------------------------------------------------------

self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  if (CDN_ASSETS.includes(request.url)) {
    event.respondWith(handleCdnAsset(request));
    return;
  }

  if (url.origin !== self.location.origin || url.pathname === '/healthz') {
    return; // other cross-origin requests and healthz pass straight through
  }

  // Login/register/logout/OIDC navigations: never cache-first-served like
  // shell requests, never offline-queued like API writes — a queued login
  // can never meaningfully "come back true" later the way a queued list or
  // item write can, and the server-rendered error/mode state on this page
  // can't be meaningfully cached either.
  if (url.pathname.startsWith('/auth/')) {
    return;
  }

  // Cheap, fire-and-forget opportunistic retry: any same-origin request
  // while online is a good moment to notice connectivity is back, without
  // waiting on Background Sync or the page's 'online' event.
  if (self.navigator.onLine) {
    flushQueue().catch(() => {});
  }

  if (url.pathname.startsWith(API_PREFIX + '/')) {
    event.respondWith(handleApiRequest(request, url));
    return;
  }

  event.respondWith(handleShellRequest(request));
});

// ---------------------------------------------------------------------------
// App shell: cache-first, refreshed in the background, offline fallback to
// the cached shell so a deep link still opens the SPA while offline.
// ---------------------------------------------------------------------------

async function handleShellRequest(request) {
  if (request.method !== 'GET') {
    return fetch(request);
  }

  const cached = await caches.match(request);
  const networkFetch = fetch(request)
    .then((response) => {
      if (response && response.ok) {
        caches.open(SHELL_CACHE).then((cache) => cache.put(request, response.clone()));
      }
      return response;
    })
    .catch(() => undefined);

  if (cached) {
    networkFetch.catch(() => {});
    return cached;
  }

  const fresh = await networkFetch;
  if (fresh) {
    return fresh;
  }

  const fallback = await caches.match('/index.html');
  return fallback || new Response('Hors ligne', { status: 503, statusText: 'Offline' });
}

// Same cache-first-then-refresh strategy as handleShellRequest, kept
// separate because the cached entry here is an opaque (no-cors) response:
// matching and re-caching it works the same way, but it can never be
// inspected (response.ok/status are meaningless on an opaque response).
async function handleCdnAsset(request) {
  const cached = await caches.match(request.url);
  const networkFetch = fetch(request.url, { mode: 'no-cors' })
    .then((response) => {
      caches.open(SHELL_CACHE).then((cache) => cache.put(request.url, response.clone()));
      return response;
    })
    .catch(() => undefined);

  if (cached) {
    networkFetch.catch(() => {});
    return cached;
  }

  const fresh = await networkFetch;
  return fresh || new Response('', { status: 503, statusText: 'Offline' });
}

// ---------------------------------------------------------------------------
// API: network-first for reads (mirrored into IndexedDB), queued-write for
// mutations made while offline.
// ---------------------------------------------------------------------------

async function handleApiRequest(request, url) {
  if (request.method === 'GET') {
    return handleApiRead(request, url);
  }
  return handleApiWrite(request, url);
}

async function handleApiRead(request, url) {
  try {
    const response = await fetch(request);
    if (response.ok) {
      mirrorReadResponse(url, response.clone()).catch(() => {});
    }
    return response;
  } catch {
    return offlineReadFallback(url);
  }
}

async function mirrorReadResponse(url, response) {
  let data;
  try {
    data = await response.json();
  } catch {
    return;
  }

  const listMatch = url.pathname.match(/^\/api\/v1\/lists\/(\d+)$/);

  if (url.pathname === '/api/v1/houses' && Array.isArray(data)) {
    await self.TrakkaDB.putHouses(data);
  } else if (url.pathname === '/api/v1/lists' && Array.isArray(data)) {
    await self.TrakkaDB.putLists(data.map(withoutItems));
  } else if (listMatch && data && typeof data === 'object') {
    const { items, ...list } = data;
    await self.TrakkaDB.putList(list);
    if (Array.isArray(items)) await self.TrakkaDB.putItems(items);
  } else if (url.pathname === '/api/v1/items' && Array.isArray(data)) {
    await self.TrakkaDB.putItems(data);
  }
}

async function offlineReadFallback(url) {
  const headers = { 'Content-Type': 'application/json; charset=utf-8', 'X-Trakka-Offline': 'true' };

  if (url.pathname === '/api/v1/houses') {
    const houses = await self.TrakkaDB.getHouses();
    return new Response(JSON.stringify(houses), { status: 200, headers });
  }

  if (url.pathname === '/api/v1/lists') {
    const houseId = decodeId(url.searchParams.get('house_id'));
    let lists = await self.TrakkaDB.getLists();
    if (houseId != null) lists = lists.filter((list) => list.house_id === houseId);
    return new Response(JSON.stringify(lists), { status: 200, headers });
  }

  const listMatch = url.pathname.match(/^\/api\/v1\/lists\/([^/]+)$/);
  if (listMatch) {
    const list = await self.TrakkaDB.getList(decodeId(listMatch[1]));
    if (!list) return jsonError(headers, 404, 'liste introuvable (hors ligne)');
    const items = await self.TrakkaDB.getItemsByList(list.id);
    return new Response(JSON.stringify({ ...list, items }), { status: 200, headers });
  }

  if (url.pathname === '/api/v1/items') {
    const listId = decodeId(url.searchParams.get('list_id'));
    const items = listId != null ? await self.TrakkaDB.getItemsByList(listId) : [];
    return new Response(JSON.stringify(items), { status: 200, headers });
  }

  return jsonError(headers, 503, 'hors ligne');
}

async function handleApiWrite(request, url) {
  const bodyText = await request.clone().text();
  let body = null;
  if (bodyText) {
    try { body = JSON.parse(bodyText); } catch { body = null; }
  }

  try {
    const response = await fetch(request);
    if (response.ok) {
      mirrorWriteResult(request.method, url, response.clone()).catch(() => {});
    }
    return response;
  } catch {
    return queueOfflineWrite(request.method, url, body);
  }
}

async function mirrorWriteResult(method, url, response) {
  let data = null;
  try { data = await response.json(); } catch { /* e.g. 204 No Content */ }

  const houseMatch = url.pathname.match(/^\/api\/v1\/houses\/(\d+)$/);
  const listMatch = url.pathname.match(/^\/api\/v1\/lists\/(\d+)$/);
  const itemMatch = url.pathname.match(/^\/api\/v1\/items\/(\d+)$/);

  if (url.pathname === '/api/v1/houses' && method === 'POST' && data) {
    await self.TrakkaDB.putHouse(data);
  } else if (houseMatch && method === 'PUT' && data) {
    await self.TrakkaDB.putHouse(data);
  } else if (houseMatch && method === 'DELETE') {
    await self.TrakkaDB.deleteHouse(Number(houseMatch[1]));
  } else if (url.pathname === '/api/v1/lists' && method === 'POST' && data) {
    await self.TrakkaDB.putList(withoutItems(data));
  } else if (listMatch && method === 'PUT' && data) {
    await self.TrakkaDB.putList(withoutItems(data));
  } else if (listMatch && method === 'DELETE') {
    await self.TrakkaDB.deleteList(Number(listMatch[1]));
  } else if (url.pathname === '/api/v1/items' && method === 'POST' && data) {
    await self.TrakkaDB.putItem(data);
  } else if (itemMatch && (method === 'PUT' || method === 'PATCH') && data) {
    await self.TrakkaDB.putItem(data);
  } else if (itemMatch && method === 'DELETE') {
    await self.TrakkaDB.deleteItem(Number(itemMatch[1]));
  }
}

// ---------------------------------------------------------------------------
// Offline write queue
// ---------------------------------------------------------------------------

async function queueOfflineWrite(method, url, body) {
  const headers = { 'Content-Type': 'application/json; charset=utf-8', 'X-Trakka-Queued': 'true' };
  const pathname = url.pathname;
  const now = new Date().toISOString();

  // Houses are simple, rarely-created resources with no offline queueing
  // support: unlike lists/items (which chain one level deep — an item can
  // depend on a list created in the same offline session — houses would
  // need a *second* level, since a list can depend on a house), which
  // docs/PWA.md explicitly calls out as unsupported. Mutating a house
  // requires connectivity; queuing anything else stays exactly as before.
  if (pathname === '/api/v1/houses' || /^\/api\/v1\/houses\/[^/]+$/.test(pathname)) {
    return jsonError(
      { 'Content-Type': 'application/json; charset=utf-8' },
      503,
      'Cette action nécessite une connexion réseau.'
    );
  }

  // Editing/deleting something that was itself created offline and never
  // reached the server: resolve it against the pending "create" entry
  // directly instead of queuing a request against an id the API has never
  // heard of.
  const pendingId = extractTempId(pathname);
  if (pendingId) {
    return resolveAgainstPendingCreate(pendingId, method, body, headers);
  }

  if (pathname === '/api/v1/lists' && method === 'POST') {
    const tempId = generateTempId('temp-list');
    const list = { id: tempId, house_id: body?.house_id, name: body?.name ?? '', type: body?.type || 'shopping', created_at: now, updated_at: now };
    await self.TrakkaDB.putList(list);
    await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId, dependsOnListTempId: null, createdAt: now });
    scheduleSync();
    return new Response(JSON.stringify(list), { status: 202, headers });
  }

  if (pathname === '/api/v1/items' && method === 'POST') {
    const tempId = generateTempId('temp-item');
    const listId = body?.list_id;
    // Still-pending list created earlier in this same offline session.
    const dependsOnListTempId = typeof listId === 'string' && listId.startsWith('temp-list') ? listId : null;
    const item = {
      id: tempId,
      list_id: listId,
      title: body?.title ?? '',
      url: body?.url || null,
      quantity: body?.quantity > 0 ? body.quantity : 1,
      done: false,
      position: body?.position ?? 0,
      created_at: now,
      updated_at: now,
    };
    await self.TrakkaDB.putItem(item);
    await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId, dependsOnListTempId, createdAt: now });
    scheduleSync();
    return new Response(JSON.stringify(item), { status: 202, headers });
  }

  // Editing/deleting an already-synced list or item.
  await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId: null, dependsOnListTempId: null, createdAt: now });
  scheduleSync();
  const optimistic = await applyOptimisticEdit(pathname, method, body);
  return new Response(JSON.stringify(optimistic ?? { queued: true }), { status: 202, headers });
}

async function applyOptimisticEdit(pathname, method, body) {
  const listMatch = pathname.match(/^\/api\/v1\/lists\/(\d+)$/);
  const itemMatch = pathname.match(/^\/api\/v1\/items\/(\d+)$/);
  const now = new Date().toISOString();

  if (listMatch) {
    const id = Number(listMatch[1]);
    if (method === 'DELETE') {
      await self.TrakkaDB.deleteList(id);
      return null;
    }
    const existing = (await self.TrakkaDB.getList(id)) || { id };
    const updated = { ...existing, ...body, id, updated_at: now };
    await self.TrakkaDB.putList(updated);
    return updated;
  }

  if (itemMatch) {
    const id = Number(itemMatch[1]);
    if (method === 'DELETE') {
      await self.TrakkaDB.deleteItem(id);
      return null;
    }
    const existing = (await self.TrakkaDB.getItem(id)) || { id };
    const updated = { ...existing, ...body, id, updated_at: now };
    applyRecurrenceCompletionOffline(updated, existing.done);
    await self.TrakkaDB.putItem(updated);
    return updated;
  }

  return null;
}

// ---------------------------------------------------------------------------
// Recurring-item completion, mirrored offline. This is a hand-kept JS port
// of applyRecurrenceCompletion/nextDueDate in internal/handlers/recurrence.go
// — there's no way to share code between the two runtimes — so that
// checking off a recurring item while offline advances it to its next
// occurrence exactly the way the server would, rather than leaving it
// stuck "done" until the sync queue flushes and a refetch corrects it. Any
// change to the Go version's rule handling must be mirrored here too.
// ---------------------------------------------------------------------------

function applyRecurrenceCompletionOffline(updated, wasDone) {
  if (wasDone || !updated.done || !updated.recurrence_rule) return;

  const next = nextDueDateOffline(updated.due_date || '', updated.recurrence_rule);
  if (!next) return; // unrecognized rule — leave it done, same as the Go path

  if (updated.recurrence_end_date && next > updated.recurrence_end_date) return;

  updated.due_date = next;
  updated.done = false;
}

function nextDueDateOffline(currentDueDate, rule) {
  const base = currentDueDate ? new Date(`${currentDueDate}T00:00:00Z`) : new Date();
  if (Number.isNaN(base.getTime())) return null;

  if (rule === 'DAILY') {
    base.setUTCDate(base.getUTCDate() + 1);
  } else if (rule === 'WEEKLY') {
    base.setUTCDate(base.getUTCDate() + 7);
  } else if (rule === 'MONTHLY') {
    base.setUTCMonth(base.getUTCMonth() + 1);
  } else if (rule === 'YEARLY') {
    base.setUTCFullYear(base.getUTCFullYear() + 1);
  } else {
    const match = /^EVERY_X_DAYS:([1-9][0-9]*)$/.exec(rule);
    if (!match) return null;
    base.setUTCDate(base.getUTCDate() + Number(match[1]));
  }

  return base.toISOString().slice(0, 10);
}

// A PATCH/PUT/DELETE against a temp-* id targets something that only ever
// existed locally. There is no server-side counterpart to call yet, so the
// change is folded into the still-queued "create" entry instead.
async function resolveAgainstPendingCreate(tempId, method, body, headers) {
  const isList = tempId.startsWith('temp-list');
  const queue = await self.TrakkaDB.getQueue();
  const entry = queue.find((e) => e.tempId === tempId);

  if (method === 'DELETE') {
    if (entry) await self.TrakkaDB.dequeue(entry.id);
    if (isList) {
      // Cancel any queued item-creates that were waiting on this list —
      // their target is being cancelled entirely, so they must not be
      // replayed against a list id that will never exist.
      for (const queued of queue) {
        if (queued.dependsOnListTempId === tempId) {
          await self.TrakkaDB.dequeue(queued.id);
        }
      }
      await self.TrakkaDB.deleteList(tempId);
    } else {
      await self.TrakkaDB.deleteItem(tempId);
    }
    return new Response(null, { status: 204 });
  }

  if (entry) {
    entry.body = { ...entry.body, ...body };
    await self.TrakkaDB.updateQueueEntry(entry);
  }

  const now = new Date().toISOString();
  if (isList) {
    const existing = (await self.TrakkaDB.getList(tempId)) || { id: tempId };
    const updated = { ...existing, ...body, id: tempId, updated_at: now };
    await self.TrakkaDB.putList(updated);
    return new Response(JSON.stringify(updated), { status: 200, headers });
  }

  const existing = (await self.TrakkaDB.getItem(tempId)) || { id: tempId };
  const updated = { ...existing, ...body, id: tempId, updated_at: now };
  applyRecurrenceCompletionOffline(updated, existing.done);
  await self.TrakkaDB.putItem(updated);
  return new Response(JSON.stringify(updated), { status: 200, headers });
}

let isFlushing = false;

async function flushQueue() {
  if (isFlushing) return;
  isFlushing = true;
  let processedAny = false;

  try {
    const queue = await self.TrakkaDB.getQueue();
    for (const entry of queue) {
      try {
        const response = await fetch(entry.path, {
          method: entry.method,
          headers: { 'Content-Type': 'application/json' },
          body: entry.body != null ? JSON.stringify(entry.body) : undefined,
        });

        // A 401 means the session expired while this write sat queued —
        // unlike an ordinary 4xx rejection, retrying *will* fix this once
        // the user logs back in, so the entry (and everything queued after
        // it, to preserve ordering) must stay queued rather than being
        // discarded as if the server had permanently rejected it.
        if (response.status === 401) {
          break;
        }

        if (entry.tempId && response.ok) {
          const data = await response.json().catch(() => null);
          if (data && data.id !== undefined) {
            await remapTempId(queue, entry.tempId, data.id);
          }
        }

        // Any other response resolves the entry: a 2xx means it's synced,
        // and any other 4xx means the server rejected it for a reason
        // blindly retrying won't fix either.
        await self.TrakkaDB.dequeue(entry.id);
        processedAny = true;
      } catch {
        // Still offline: stop here, leave this and later entries queued
        // for the next attempt so ordering (and dependent temp ids) holds.
        break;
      }
    }
  } finally {
    isFlushing = false;
  }

  if (processedAny) await notifyClients();
}

async function remapTempId(queue, tempId, realId) {
  const isList = tempId.startsWith('temp-list');

  if (isList) {
    const list = await self.TrakkaDB.getList(tempId);
    if (list) {
      // Re-parent any locally-mirrored items BEFORE deleting the temp list
      // record: TrakkaDB.deleteList() cascades to every item still filed
      // under that list_id, and until this point that includes items
      // whose own "create" is later in this same queue.
      const orphanedItems = await self.TrakkaDB.getItemsByList(tempId);
      for (const item of orphanedItems) {
        await self.TrakkaDB.putItem({ ...item, list_id: realId });
      }
      await self.TrakkaDB.deleteList(tempId);
      await self.TrakkaDB.putList({ ...list, id: realId });
    }
  } else {
    const item = await self.TrakkaDB.getItem(tempId);
    if (item) {
      await self.TrakkaDB.deleteItem(tempId);
      await self.TrakkaDB.putItem({ ...item, id: realId });
    }
  }

  // Mutate the in-memory queue entries (not just IndexedDB) so this same
  // flushQueue() pass picks up the corrected list_id when it reaches them.
  for (const queued of queue) {
    if (queued.dependsOnListTempId === tempId) {
      queued.dependsOnListTempId = null;
      queued.body = { ...queued.body, list_id: realId };
      await self.TrakkaDB.updateQueueEntry(queued);
    }
  }
}

async function notifyClients() {
  const clients = await self.clients.matchAll({ includeUncontrolled: true, type: 'window' });
  for (const client of clients) {
    client.postMessage({ type: 'trakka-sync-complete' });
  }
}

async function scheduleSync() {
  if (!('sync' in self.registration)) return;
  try {
    await self.registration.sync.register(SYNC_TAG);
  } catch {
    // Background Sync unsupported or denied (all of iOS/iPadOS Safari, and
    // some browsers without the permission granted) — the page's 'online'
    // handler and the opportunistic per-fetch check above cover this.
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function withoutItems(list) {
  const { items, ...rest } = list;
  return rest;
}

function extractTempId(pathname) {
  const match = pathname.match(/^\/api\/v1\/(?:lists|items)\/(temp-[^/]+)$/);
  return match ? match[1] : null;
}

function generateTempId(prefix) {
  const random = (self.crypto && self.crypto.randomUUID)
    ? self.crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`;
  return `${prefix}-${random}`;
}

function decodeId(raw) {
  if (raw == null) return null;
  if (raw.startsWith('temp-')) return raw;
  const n = Number(raw);
  return Number.isFinite(n) ? n : raw;
}

function jsonError(headers, status, message) {
  return new Response(JSON.stringify({ error: message }), { status, headers });
}
