'use strict';

// Trakka service worker: offline-first app shell + an IndexedDB-backed
// sync queue for API mutations made while offline. Uses static/js/db.js
// for all persistence (see that file for why it's importScripts()-safe).
importScripts('/js/db.js');

// Bump both on any change to APP_SHELL's contents so activate()
// evicts the old cache instead of serving stale assets forever.
const SHELL_CACHE = 'trakka-shell-v48';
const RUNTIME_CACHE = 'trakka-runtime-v48';
const KNOWN_CACHES = [SHELL_CACHE, RUNTIME_CACHE];

const APP_SHELL = [
  '/',
  '/index.html',
  '/js/theme-init.js',
  '/js/theme.js',
  '/js/tailwind.js',
  '/js/tailwind-config.js',
  '/js/i18n.js',
  '/js/app.js',
  '/js/db.js',
  '/js/undo.js',
  '/js/list_view.js',
  '/js/gestures.js',
  '/js/reorder.js',
  '/js/planning.js',
  '/js/urgent.js',
  '/js/spaces.js',
  '/js/shares.js',
  '/js/notifications.js',
  '/js/admin.js',
  '/js/settings.js',
  '/js/push.js',
  '/js/install-help.js',
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
  '/icons/trakka-lockup-light-bg.svg',
];

// index.html's styling depends entirely on this cross-origin script (the
// Tailwind Play CDN) actually loading — without it cached, the app shell
// would come back from cache while offline but render completely
// unstyled. Cached separately from APP_SHELL (best-effort, not part of
// the atomic install) so a CDN hiccup at install time can't break the
// installation of Trakka's own app shell.
const API_PREFIX = '/api/v1';
const SYNC_TAG = 'trakka-sync-queue';

// ---------------------------------------------------------------------------
// Lifecycle
// ---------------------------------------------------------------------------

// skipWaiting() here is what makes app.js's update banner (see
// watchForServiceWorkerUpdate in static/js/app.js) meaningful: without it, a
// newly-installed worker would sit in the 'waiting' state until every tab
// running the old one closes, so the banner's "Mettre à jour" button
// wouldn't actually have anything new to switch to yet on click. With it,
// this worker proceeds straight to activating (below) as soon as it's
// installed, so a reload after the banner appears really does pick up the
// new assets.
self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(SHELL_CACHE)
      .then((cache) => cache.addAll(APP_SHELL).then(() => cache))
      .then(() => self.skipWaiting())
  );
});

// Only ever deletes entries from the Cache Storage API (KNOWN_CACHES,
// declared above) — never touches IndexedDB, which is a completely
// separate storage system the browser doesn't tie to cache versioning at
// all. This is what lets a deployed update evict every stale cached asset
// on activation without losing the offline mirror/sync queue db.js
// maintains; see db.js's own onupgradeneeded/onversionchange handling for
// how *that* store evolves safely across versions instead.
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
// Web Push: a message from internal/handlers.sendToUsers
// (internal/handlers/push.go), encrypted per RFC 8291 — decryption itself is
// handled entirely by the browser/OS push stack before this event ever
// fires; by the time 'push' runs, event.data is already the plaintext JSON
// pushPayload {title, body, url} the Go backend sent. Every push this app
// sends is a real, user-visible notification (never a data-only "silent
// push" with no UI, which browsers restrict/penalize) — `silent: true` on
// the Notification itself is what satisfies the "sans son, discrète" intent
// instead: no sound/vibration, but still shown.
self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = {};
  }
  const title = data.title || 'Trakka';
  const url = data.url || '/';
  event.waitUntil(
    self.registration.showNotification(title, {
      body: data.body || '',
      icon: '/icons/trakka-icon-192.png',
      badge: '/icons/trakka-maskable-192.png',
      silent: true,
      tag: 'trakka-' + url,
      data: { url },
    })
  );
});

// Clicking the notification focuses an already-open tab (and tells it,
// via postMessage, which list to switch to client-side — see app.js's
// handleNotificationClickMessage — rather than relying on the unevenly
// supported WindowClient.navigate(), which an SPA has no real need for
// anyway) or, with no tab open, opens a fresh one straight at the deep
// link; static/js/app.js's own boot-time handleDeepLinkOrRestore reads that
// URL's ?list= query param the same way either path ends up at the right
// list.
self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';

  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clientList) => {
      for (const client of clientList) {
        if ('focus' in client) {
          client.postMessage({ type: 'trakka-notification-click', url });
          return client.focus();
        }
      }
      if (self.clients.openWindow) {
        return self.clients.openWindow(url);
      }
      return undefined;
    })
  );
});

// ---------------------------------------------------------------------------
// Fetch routing
// ---------------------------------------------------------------------------

self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

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

// ---------------------------------------------------------------------------
// API: network-first for reads (mirrored into IndexedDB), queued-write for
// mutations made while offline. This stays network-first rather than
// stale-while-revalidate at the SW layer on purpose — a mutation's response
// (a fresh price/quantity/RBAC check, etc.) must never be silently served
// from a stale cache. The "instant paint, then refresh" effect the app's
// own performance story wants is instead implemented one layer up, at each
// view's own call site (loadDashboard in app.js; loadUrgentView/
// loadPlanningView/loadSpacesView; selectList in list_view.js), which
// already reads the exact same IndexedDB mirror this file writes on every
// successful read (mirrorReadResponse below) before ever calling this
// endpoint — see CLAUDE.md's "Loading skeletons & stale-while-revalidate
// view loaders" entry for the full design.
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
  // ?shared_with_me=true returns lists from Houses the caller isn't
  // necessarily a member of (see db.ListSharedListsForUser), tagged with
  // access_source/access_permission — mirroring those into the same
  // IndexedDB store as the caller's own House-scoped lists would pollute it
  // with rows that don't belong to any of their own Houses, for a view that
  // has no offline support in the first place (see shares.js's
  // loadSharedView). Skip it entirely rather than either.
  const isSharedWithMe = url.pathname === '/api/v1/lists' && url.searchParams.get('shared_with_me') === 'true';
  // Same reasoning as isSharedWithMe, applied to CLAUDE.md's "pinned house
  // spaces" feature: ?pinned_house_spaces=true also returns lists from
  // Houses the caller belongs to but that aren't necessarily the currently
  // selected one — mirroring them into the plain lists store under their
  // *other* House's house_id would be correct on its own, but this query has
  // no offline support in the first place (see shares.js's
  // loadPinnedHouseSpaceLists), so it's skipped the same way.
  const isPinnedHouseSpaces = url.pathname === '/api/v1/lists' && url.searchParams.get('pinned_house_spaces') === 'true';
  // Same reasoning as isSharedWithMe above, applied to Spaces: a category
  // shared with the caller (rather than owned by them) belongs to whoever
  // actually owns it, not to any of the caller's own custom_category_id
  // associations — mirroring it into the plain custom_categories mirror
  // would pollute it with a row the caller can't rename/delete and that
  // has no offline support in the first place (see spaces.js's
  // loadSharedCustomCategories). Skip it entirely rather than either.
  const isSharedCategoriesQuery =
    url.pathname === '/api/v1/custom-categories' && url.searchParams.get('shared_with_me') === 'true';

  if (url.pathname === '/api/v1/houses' && Array.isArray(data)) {
    await self.TrakkaDB.putHouses(data);
  } else if (url.pathname === '/api/v1/lists' && !isSharedWithMe && !isPinnedHouseSpaces && Array.isArray(data)) {
    await self.TrakkaDB.putLists(data.map(withoutItems));
  } else if (listMatch && data && typeof data === 'object') {
    const { items, ...list } = data;
    await self.TrakkaDB.putList(list);
    if (Array.isArray(items)) await self.TrakkaDB.putItems(items);
  } else if (url.pathname === '/api/v1/items' && Array.isArray(data)) {
    await self.TrakkaDB.putItems(data);
  } else if (url.pathname === '/api/v1/custom-categories' && !isSharedCategoriesQuery && Array.isArray(data)) {
    await self.TrakkaDB.putCustomCategories(data);
  }
}

async function offlineReadFallback(url) {
  const headers = { 'Content-Type': 'application/json; charset=utf-8', 'X-Trakka-Offline': 'true' };

  if (url.pathname === '/api/v1/houses') {
    const houses = await self.TrakkaDB.getHouses();
    return new Response(JSON.stringify(houses), { status: 200, headers });
  }

  if (url.pathname === '/api/v1/lists' && url.searchParams.get('shared_with_me') === 'true') {
    // No offline mirror for cross-house shared lists (see mirrorReadResponse
    // above) — answer with an empty list rather than falling through to the
    // general lists mirror below, which would incorrectly surface the
    // caller's own House-scoped lists in the "Partagé avec moi" tab.
    return new Response(JSON.stringify([]), { status: 200, headers });
  }

  if (url.pathname === '/api/v1/lists' && url.searchParams.get('pinned_house_spaces') === 'true') {
    // Same reasoning as the shared_with_me case just above, applied to
    // CLAUDE.md's "pinned house spaces" feature — no offline mirror, so an
    // empty list rather than falling through to the general lists mirror
    // below (which has no notion of "only the ones reached via a pinned
    // House Space" and would return every one of the caller's own
    // House-scoped lists instead).
    return new Response(JSON.stringify([]), { status: 200, headers });
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

  if (url.pathname === '/api/v1/custom-categories' && url.searchParams.get('shared_with_me') === 'true') {
    // No offline mirror for Spaces shared with the caller (see
    // mirrorReadResponse above) — answer with an empty list rather than
    // falling through to the general categories mirror below, which would
    // incorrectly surface the caller's own categories as "shared with me".
    return new Response(JSON.stringify([]), { status: 200, headers });
  }

  if (url.pathname === '/api/v1/custom-categories') {
    const categories = await self.TrakkaDB.getCustomCategories();
    return new Response(JSON.stringify(categories), { status: 200, headers });
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
  const reorderMatch = url.pathname.match(/^\/api\/v1\/lists\/(\d+)\/reorder$/);
  const itemMatch = url.pathname.match(/^\/api\/v1\/items\/(\d+)$/);
  const categoryMatch = url.pathname.match(/^\/api\/v1\/custom-categories\/(\d+)$/);

  if (reorderMatch && method === 'PUT' && Array.isArray(data)) {
    // The response is every item in the list, freshly re-ordered — mirror
    // all of them in one go rather than one at a time, the same bulk helper
    // GET /api/v1/lists/{id}'s own item-array mirroring already uses.
    await self.TrakkaDB.putItems(data);
  } else if (url.pathname === '/api/v1/houses' && method === 'POST' && data) {
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
  } else if (url.pathname === '/api/v1/custom-categories' && method === 'POST' && data) {
    await self.TrakkaDB.putCustomCategory(data);
  } else if (categoryMatch && method === 'PUT' && data) {
    await self.TrakkaDB.putCustomCategory(data);
  } else if (categoryMatch && method === 'DELETE') {
    await self.TrakkaDB.deleteCustomCategory(Number(categoryMatch[1]));
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

  // Admin settings (OIDC config, registration policy, instance name) are a
  // rare, system-wide mutation with real side effects (re-running OIDC
  // discovery, flipping registration open/closed) — the same reasoning that
  // keeps houses off the offline queue applies here, even more so given
  // what's at stake if a stale queued change silently reapplied later.
  if (pathname === '/api/v1/admin/settings') {
    return jsonError(
      { 'Content-Type': 'application/json; charset=utf-8' },
      503,
      'Cette action nécessite une connexion réseau.'
    );
  }

  // Granting/revoking a List or Space share (or withdrawing an invitation
  // that has not been accepted yet) needs an immediate round-trip and
  // reports success or failure back to the modal right away — queuing it
  // silently, like houses/admin settings above, would surface a confusing
  // failure only once the queue eventually flushes, long after the modal
  // already looked like it worked.
  if (
    /^\/api\/v1\/(custom-categories|lists)\/[^/]+\/share(\/[^/]+)?$/.test(pathname) ||
    /^\/api\/v1\/(custom-categories|lists|houses)\/[^/]+\/invitations$/.test(pathname)
  ) {
    return jsonError(
      { 'Content-Type': 'application/json; charset=utf-8' },
      503,
      'Cette action nécessite une connexion réseau.'
    );
  }

  // A manual drag-and-drop reorder (PUT /lists/{id}/reorder) sends the
  // complete new ordering as item_ids. Unlike houses/admin settings/shares
  // above, this is a plain per-list item mutation exactly like a PATCH on a
  // single item's `position` would be — there is no reason it needs to be
  // exempted from the offline queue, and previously blocking it with a hard
  // 503 here is what surfaced as "Cette action nécessite une connexion
  // réseau." mid-drag (see the Offline-First requirement in
  // CLAUDE.md/docs/PWA.md). queueOfflineReorder below applies the new
  // ordering straight to the local IndexedDB mirror (so it survives a reload
  // even before the request ever reaches the server) and queues the request
  // for replay once back online, synthesizing the same shape a real 200
  // response carries — every item in the list, in its new order — so
  // reorder.js's commitReorder can assign it straight into
  // state.currentList.items exactly as it would for a live response.
  // reorder.js's own reorderAvailable() keeps the "⇅ Réordonner" button
  // hidden whenever any item still has an offline-queued temp-item-* id, so
  // item_ids here is always a set of real, numeric ids already present in
  // the mirror — no temp-id resolution is needed the way pending item/list
  // creates require below.
  const reorderMatch = pathname.match(/^\/api\/v1\/lists\/([^/]+)\/reorder$/);
  if (reorderMatch) {
    return queueOfflineReorder(pathname, decodeId(reorderMatch[1]), body, headers, now);
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
    // listId: tempId — this queue entry *is* the list's own still-pending
    // create, so it's attributed to itself (see the listId/itemId doc
    // comment above resolveQueueTargetIds below).
    await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId, dependsOnListTempId: null, listId: tempId, itemId: null, createdAt: now });
    scheduleSync();
    await broadcastQueueState(false);
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
    await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId, dependsOnListTempId, listId, itemId: tempId, createdAt: now });
    scheduleSync();
    await broadcastQueueState(false);
    return new Response(JSON.stringify(item), { status: 202, headers });
  }

  // Editing/deleting an already-synced list or item. Resolved BEFORE
  // enqueueing/applyOptimisticEdit runs, since a DELETE's optimistic apply
  // removes the row from the mirror — after that point there would be
  // nothing left to look list_id up from (see resolveQueueTargetIds).
  const { listId: targetListId, itemId: targetItemId } = await resolveQueueTargetIds(pathname, body);
  await self.TrakkaDB.enqueueRequest({ method, path: pathname, body, tempId: null, dependsOnListTempId: null, listId: targetListId, itemId: targetItemId, createdAt: now });
  scheduleSync();
  const optimistic = await applyOptimisticEdit(pathname, method, body);
  await broadcastQueueState(false);
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

// Resolves which list (and, for an item write, which item) a queued
// list-or-item write should be attributed to. Every queue entry carries a
// listId (and itemId, for an item-scoped write) precisely so app.js's
// "unsynced changes" indicator (refreshPendingChangeIndicators, driving the
// small dot on a list's card/detail header and an item's row) can be
// derived with one read of the queue, without re-parsing every entry's HTTP
// path itself. Must run BEFORE applyOptimisticEdit, since a queued DELETE's
// optimistic apply removes the row from the IndexedDB mirror — after that
// point there would be nothing left to look list_id up from.
async function resolveQueueTargetIds(pathname, body) {
  const listMatch = pathname.match(/^\/api\/v1\/lists\/([^/]+)$/);
  if (listMatch) {
    return { listId: decodeId(listMatch[1]), itemId: null };
  }
  const itemMatch = pathname.match(/^\/api\/v1\/items\/([^/]+)$/);
  if (itemMatch) {
    const id = decodeId(itemMatch[1]);
    const existing = await self.TrakkaDB.getItem(id);
    const listId = existing ? existing.list_id : (body && body.list_id !== undefined ? body.list_id : null);
    return { listId, itemId: id };
  }
  return { listId: null, itemId: null };
}

// Applies a drag-and-drop reorder locally (position = index in item_ids,
// matching db.ReorderItems's own server-side assignment exactly) and queues
// the PUT for replay once back online. Returns every item currently mirrored
// under this list, freshly sorted, the same shape as a live 200 response —
// commitReorder (reorder.js) reads this straight into
// state.currentList.items. Any id in item_ids that isn't in the local mirror
// (shouldn't happen — see the comment at this function's one call site) is
// silently skipped rather than failing the whole request.
async function queueOfflineReorder(pathname, listId, body, headers, now) {
  const itemIds = Array.isArray(body?.item_ids) ? body.item_ids : [];
  const items = await self.TrakkaDB.getItemsByList(listId);
  const byId = new Map(items.map((item) => [String(item.id), item]));

  const reordered = [];
  itemIds.forEach((id, index) => {
    const item = byId.get(String(id));
    if (item) reordered.push({ ...item, position: index, updated_at: now });
  });
  if (reordered.length) await self.TrakkaDB.putItems(reordered);

  await self.TrakkaDB.enqueueRequest({ method: 'PUT', path: pathname, body, tempId: null, dependsOnListTempId: null, listId, itemId: null, createdAt: now });
  scheduleSync();
  await broadcastQueueState(false);

  const all = await self.TrakkaDB.getItemsByList(listId);
  all.sort((a, b) => a.position - b.position || a.id - b.id);
  return new Response(JSON.stringify(all), { status: 202, headers });
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
    await broadcastQueueState(false);
    // headers carries X-Trakka-Queued — apiRequest (app.js) checks that on
    // every response to know it needs to refresh its pending-list/item id
    // caches, so even this body-less cancellation response has to carry it,
    // same as every other response this function/queueOfflineWrite return.
    return new Response(null, { status: 204, headers });
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
  // sw.js's own fetch handler opportunistically calls flushQueue() on
  // *every* same-origin request while online (see the top-level 'fetch'
  // listener above) — hadEntries is what keeps that from broadcasting a
  // 'trakka-sync-status' message (and triggering a client-side repaint) on
  // every single ordinary API call once the queue is already empty; it only
  // ever fires when this attempt actually found something to do.
  let hadEntries = false;

  try {
    const queue = await self.TrakkaDB.getQueue();
    hadEntries = queue.length > 0;
    // "syncing" only ever means "flushQueue is actively replaying entries
    // right now" — skipped entirely when there's nothing to replay, so
    // reconnecting with an empty queue never flashes a spinner for no
    // reason.
    if (hadEntries) await broadcastQueueState(true, queue.length);
    for (const entry of queue) {
      try {
        const response = await fetch(entry.path, {
          method: entry.method,
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
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

  // Broadcast the final state whenever this attempt had anything to do,
  // whether or not the queue fully drained (still offline, or a 401 left
  // the rest queued): the header indicator and the per-list/per-item dots
  // need an up-to-date pending count either way, not just on a full
  // success. Skipped when hadEntries is false — see its own comment above.
  if (hadEntries) await broadcastQueueState(false);
  if (processedAny) await notifyClients({ type: 'trakka-sync-complete' });
}

// Tells every open tab the offline sync queue's current size, and whether
// flushQueue is actively replaying it right now — drives the header's
// pending/syncing/synced indicator and the per-list/per-item "unsynced
// changes" dots (see app.js's handleSyncStatusMessage/
// refreshPendingChangeIndicators), and the trakka:sync-pending/
// trakka:sync-complete window events the rest of the UI listens for.
// Called after every point that adds to or removes from the queue
// (queueOfflineWrite/queueOfflineReorder/resolveAgainstPendingCreate above)
// and at the start/end of flushQueue itself.
async function broadcastQueueState(syncing, pendingOverride) {
  const pending = pendingOverride !== undefined ? pendingOverride : (await self.TrakkaDB.getQueue()).length;
  await notifyClients({ type: 'trakka-sync-status', pending, syncing });
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

// payload defaults to the original 'trakka-sync-complete' shape this
// function always sent before it grew a parameter — broadcastQueueState
// above is what actually sends the newer 'trakka-sync-status' payload.
async function notifyClients(payload = { type: 'trakka-sync-complete' }) {
  const clients = await self.clients.matchAll({ includeUncontrolled: true, type: 'window' });
  for (const client of clients) {
    client.postMessage(payload);
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
