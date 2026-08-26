# PWA (offline support)

Trakka is installable and works offline on iOS, iPadOS, and Android, via three static files plus a small amount of glue in `static/index.html` and `static/js/app.js`:

| File | Role |
|---|---|
| `static/manifest.json` | App identity/appearance for the browser's install prompt (mainly Android/desktop Chrome) |
| `static/sw.js` | Service worker: caches the app shell, serves it offline, and queues API mutations made while offline |
| `static/js/db.js` | IndexedDB layer shared by the service worker (reads and writes) and the page (`app.js`, read-only — see below) |

## Requirement: HTTPS

Service workers only register in a [secure context](https://developer.mozilla.org/en-US/docs/Web/Security/Secure_Contexts) — `https://` or exactly `http://localhost`. **None of this works over plain `http://` from another device** (e.g. `http://192.168.1.20:8080` from a phone on the same network) — `navigator.serviceWorker.register()` will simply be unavailable, silently disabling offline support and installability, with no error surfaced to the user. `compose.yml` exposes Trakka over plain HTTP by design (it expects a TLS-terminating reverse proxy in front for anything beyond local testing — see [docs/DEPLOYMENT.md](DEPLOYMENT.md)). Testing on `localhost` (including via `docker run -p 8080:8080` on the same machine as the browser) works without TLS.

## iOS/iPadOS Safari: what's different

iOS Safari implements service workers and IndexedDB, but **not** the Web App Manifest (for icons/display mode) or the Background Sync API. `static/index.html` therefore carries iOS-specific tags that duplicate what the manifest provides for other platforms:

- `<link rel="apple-touch-icon" href="/icons/apple-touch-icon-180.png">` — iOS never reads `manifest.json`'s `icons` array.
- `<meta name="apple-mobile-web-app-capable" content="yes">` — launches full-screen (no browser chrome) once added to the home screen.
- `<meta name="apple-mobile-web-app-status-bar-style">` and `apple-mobile-web-app-title` — status bar appearance and the name shown under the home-screen icon.

Because iOS has no Background Sync API (`'sync' in self.registration` is simply `false` there), the offline write queue can only be flushed while the page is open. `sw.js` covers this with two triggers instead of relying on `sync`:
1. The page listens for the browser's `online` event (`static/js/app.js`, near the bottom of the file) and posts `{type: 'flush-queue'}` to the active service worker.
2. `sw.js`'s `fetch` handler opportunistically calls `flushQueue()` (cheap no-op if the queue is empty) whenever `navigator.onLine` is true, on every same-origin request — so simply using the app again after reconnecting is enough to drain the queue.

On Android/Chrome, `self.registration.sync.register()` (in `scheduleSync()`) additionally lets the browser flush the queue even if Trakka isn't open at all when connectivity returns.

## Caching the Tailwind CDN script

`static/index.html`'s entire visual design depends on `https://cdn.tailwindcss.com` (the Tailwind Play CDN script) loading successfully — there is no fallback stylesheet. That script is cross-origin, and `sw.js`'s general rule is to let cross-origin requests pass straight through uncached (see the `fetch` handler). Without an exception, opening the installed PWA offline would still load Trakka's own cached app shell, but render it completely unstyled, because the browser's own HTTP cache for a third-party CDN script is not something a PWA can rely on staying warm.

`sw.js` therefore treats `CDN_ASSETS` (currently just the one URL) as a special case: cached with `mode: 'no-cors'` (so caching succeeds even though the CDN doesn't send CORS headers — the resulting "opaque" response can be replayed but never inspected, which is fine since it's only ever handed back to a `<script>` tag) and served cache-first with a background refresh, the same strategy as the rest of the app shell. This precache is deliberately **not** part of the atomic `cache.addAll(APP_SHELL)` call in `install` — it runs as a separate, best-effort step afterward, so a CDN hiccup at install time can't prevent Trakka's own app shell from installing.

If you ever add another cross-origin script/stylesheet dependency, add its URL to `CDN_ASSETS` rather than leaving it to fall through the generic cross-origin bypass — otherwise it will silently break offline styling/behavior exactly like this.

## How offline reads and writes work

For the actual list/item CRUD, `static/js/app.js` needs **no offline-specific code at all** — every `fetch()` it makes is transparently intercepted by `sw.js`'s `fetch` event handler, and a queued/offline response looks enough like a normal one (any 2xx-range status, JSON body) that `apiRequest()` handles it the same way as talking to the real server.

- **Reads** (`GET /api/v1/...`): network-first. A successful response is mirrored into IndexedDB (via `TrakkaDB.putLists`/`putList`/`putItems`) before being returned. If the network fetch fails, `sw.js` reconstructs an equivalent JSON response directly from that IndexedDB mirror and returns it with an `X-Trakka-Offline: true` header.
- **Writes** (`POST`/`PUT`/`PATCH`/`DELETE` on `/api/v1/...`): also network-first. On failure, the request is recorded in an IndexedDB `queue` store and an optimistic response is returned immediately (HTTP `202`) so the UI updates right away, without the frontend needing to know it was queued rather than actually sent.

The one place `app.js` does talk to IndexedDB directly is the "N en attente" badge next to the network-status indicator: `refreshPendingBadge()` calls `TrakkaDB.getQueue()` purely to read the current queue length. It never writes to any `TrakkaDB` store — the service worker remains the sole writer of both the mirror and the queue, which is what keeps this a single-writer system instead of two independent copies that could drift out of sync.

## The tricky part: creating things offline

Newly created lists/items don't have a server id yet while offline, so `sw.js` assigns a temporary string id (`temp-list-<uuid>` / `temp-item-<uuid>`) and returns that in the optimistic response. Two situations need special handling as a result, both in `static/sw.js`:

1. **Adding an item to a list that was itself created offline.** The queued item-create entry records `dependsOnListTempId`. When the list's own queued create later succeeds and gets a real id, `remapTempId()` rewrites that item entry's `body.list_id` **in place, in the in-memory queue array `flushQueue()` is iterating** (not just in IndexedDB) — this is what makes the item request, sent later in the same flush pass, carry the real list id instead of the temp one. `remapTempId()` also has to re-parent any locally-mirrored items to the new real list id *before* calling `TrakkaDB.deleteList(tempId)`, because `deleteList` cascades to every item still filed under that `list_id` — get the order wrong and the flush silently deletes the very item it's about to sync (this was caught by testing, not inspection; see the git history for `remapTempId` if you're touching this again).
2. **Editing or deleting something that never reached the server.** A `PATCH`/`PUT`/`DELETE` against a `temp-*` id can't be sent to the API — there's nothing there yet. `resolveAgainstPendingCreate()` instead folds the change into the still-queued create (for edits) or cancels it outright (for deletes). Deleting a pending **list** this way must also cancel any queued item-creates that depended on it (`dependsOnListTempId`), or they'd later be replayed against a list id that will never exist.

What's deliberately **not** handled: chains deeper than one level (there's no scenario in this data model where an item depends on another item), and retrying a request the server rejected outright (a 4xx while flushing is dequeued, not retried — see the comment in `flushQueue()`).

### Session expiry vs. the offline queue

Authentication (see [docs/API.md](API.md#authentication)) is cookie-based, and cookies are ambient/browser-managed rather than something `sw.js` has to snapshot per request — a queued write, replayed later by `flushQueue()`, automatically carries whatever session cookie is present in the browser *at replay time*, not whatever was present when it was queued. Nothing needed to change in `queueOfflineWrite()`/the queue's stored shape for this to work.

What *did* need a fix: `flushQueue()`'s original logic treated every non-2xx replay response as "the server rejected this, drop it" — reasonable for an ordinary `4xx`, but wrong for a `401`. If the session expired while a write sat in the queue (e.g. the user closed their laptop offline for a few weeks), the old behavior would silently and permanently discard that edit the moment connectivity came back, before the user ever got a chance to log back in. `flushQueue()` now special-cases `401`: it stops processing immediately and leaves that entry — and everything queued after it, to preserve ordering — in place, rather than dequeuing it. Once the user logs back in (a normal page load, which re-establishes the session cookie), the next `flushQueue()` trigger (the opportunistic per-fetch check, the `online` event, or a Background Sync retry) picks the queue back up automatically.

`sw.js`'s `fetch` handler also passes `/auth/...` requests straight through, unintercepted (checked before the shell/API branching) — a login/register/logout navigation is neither cache-first-served like a shell asset nor offline-queued like an API write. A queued login could never meaningfully "come back true" later the way a queued list/item edit can, and the server-rendered login page's error/mode state can't be meaningfully cached either.

### Houses are read-mirrored but not offline-writable

Houses (`/api/v1/houses`) get the same read-through mirror as lists/items — `TrakkaDB.putHouses`/`putHouse`/`getHouses`, a `houses` IndexedDB store — so the "Maison" selector and the dashboard (now scoped by `?house_id=`) still show something while offline. But creating, renaming, or deleting a house while offline is deliberately **not** queued: `queueOfflineWrite()` returns a `503` for any `/api/v1/houses` request that reaches it. The reason is the one-level-only limit described above — a list can now depend on a house the same way an item depends on a list, and since houses can only ever be created online, `list.house_id` in a queued list-create is always a real id, never a `temp-house-*` one. Making house creation queueable too would require a second level of temp-id chaining (list → house) on top of the existing one (item → list), which is exactly the kind of chain this design explicitly doesn't support; houses are also created rarely enough that requiring connectivity for it is a reasonable trade rather than a real limitation.

## Undo/rollback and the offline queue

`static/js/undo.js` (`window.TrakkaUndo.schedule()`) is a small, generic bottom-snackbar module used by `app.js` (deleting a list) and `list_view.js` (deleting an item, toggling an item's `done` state) to give every easily-mistaken mutation a 5-second "Annuler" grace period. The optimistic DOM/state change (hiding the list card, filtering the item out of `renderItems()` via a `pendingDelete` flag, moving the checkbox to the other section) happens immediately and unconditionally; the real `apiRequest()` call is deferred into the toast's `onCommit` callback, which only runs once the countdown actually reaches zero without the user clicking "Annuler" (`onUndo`, which just reverts the optimistic change with no network call at all).

This needs **no changes to `sw.js`** and no special-casing for offline: because the deferred `apiRequest()` call is the same call that used to happen immediately, `sw.js`'s `fetch` handler can't tell the difference between a mutation sent right away and one sent five seconds later. If the device is offline when the grace period expires, `queueOfflineWrite()` queues it exactly as it would have if the user had never seen an undo option at all — including the `temp-item-*`/`temp-list-*` special-casing in `resolveAgainstPendingCreate()` for something created and then deleted, both while offline, before the create ever reached the server. The one thing `TrakkaUndo` deliberately has no opinion on is *when* the deferred request goes out — only that it eventually does, via the exact same `apiRequest()` path as any other mutation.

## IndexedDB schema versioning

`db.js`'s `DB_VERSION` must be bumped whenever an object store is added or its shape changes (e.g. adding the `houses` store bumped it from `1` to `2`) — `indexedDB.open(DB_NAME, DB_VERSION)` only re-runs `onupgradeneeded` (where stores/indexes are created) when the requested version is higher than what's already on disk in that browser.

## Cache versioning

`SHELL_CACHE`/`RUNTIME_CACHE` in `sw.js` are version-suffixed (`trakka-shell-v1`, etc.). Bump the suffix whenever `APP_SHELL`'s contents change; `activate()` deletes any cache name it doesn't recognize, which is what evicts the old version. Every URL listed in `APP_SHELL` must resolve with a 2xx status — `cache.addAll()` fails the entire install atomically if even one doesn't.

## Testing changes to sw.js / db.js

There's no Node-based test suite committed to the repo (consistent with the rest of the project having no test tooling — see [docs/DEVELOPMENT.md](DEVELOPMENT.md)), but both files are written to be loadable outside a browser: `db.js` only touches `indexedDB` and `self`, so it runs unmodified against a Node IndexedDB polyfill (e.g. `fake-indexeddb`) inside a `vm` context; `sw.js`'s top-level `function` declarations become properties of that same `vm` context, so its queue logic (`queueOfflineWrite`, `flushQueue`, `remapTempId`, `resolveAgainstPendingCreate`) can be called directly with a mocked `fetch`/`caches`/`registration`/`clients`, without needing a real browser. This is how the temp-id remapping bug described above was actually caught — reasoning through the code did not catch it, a scripted scenario did.
