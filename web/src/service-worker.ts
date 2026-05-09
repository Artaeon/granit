/// <reference types="@sveltejs/kit" />
// SvelteKit auto-registers this when present at src/service-worker.ts.
//
// Strategy by request type:
//   - SPA shell + bundle assets: cache-first (precached on install).
//   - HTML navigations: network-first → cache fallback (so the app shell
//     loads even while offline, after one prior visit).
//   - GET /api/v1/* (NOT /ws):
//       stale-while-revalidate. Serve cache immediately if present,
//       fetch fresh in the background and update the cache. While offline
//       you see the last-known data. /ws and non-GET methods always pass
//       through unmodified — the server is the source of truth for writes.

import { build, files, version } from '$service-worker';

const SHELL_CACHE = `granit-shell-${version}`;
const API_CACHE = `granit-api-${version}`;
const SHELL = [...build, ...files];

self.addEventListener('install', (event) => {
  const e = event as ExtendableEvent;
  e.waitUntil(
    (async () => {
      const cache = await caches.open(SHELL_CACHE);
      await cache.addAll(SHELL);
      await (self as unknown as ServiceWorkerGlobalScope).skipWaiting();
    })()
  );
});

self.addEventListener('activate', (event) => {
  const e = event as ExtendableEvent;
  e.waitUntil(
    (async () => {
      // Drop every cache that isn't the current build's. Catches the
      // common stale-bundle bug: user upgrades the binary but the old
      // SW (with `granit-shell-OLDVERSION`) keeps serving last week's
      // SPA. New SW is at a new version, so its keep-set excludes the
      // old shell + old API cache, both get nuked here.
      const keys = await caches.keys();
      const keep = new Set([SHELL_CACHE, API_CACHE]);
      await Promise.all(keys.filter((k) => !keep.has(k)).map((k) => caches.delete(k)));
      await (self as unknown as ServiceWorkerGlobalScope).clients.claim();
      // Tell every open page to reload so they pick up the new bundle
      // immediately. Without this, an open tab stays on the old JS
      // until the user manually refreshes — which is how "tasks stuck
      // on loading after a deploy" happened.
      const sw = self as unknown as ServiceWorkerGlobalScope;
      const allClients = await sw.clients.matchAll({ type: 'window' });
      for (const client of allClients) {
        try {
          // postMessage is non-blocking — pages decide whether to
          // honor it. The companion handler in the SPA reloads.
          client.postMessage({ type: 'sw-updated', version });
        } catch {
          // Client unreachable; nothing useful to do.
        }
      }
    })()
  );
});

self.addEventListener('fetch', (event) => {
  const e = event as FetchEvent;
  const url = new URL(e.request.url);

  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/ws')) return; // WebSocket — never cache

  // API: only cache GETs, stale-while-revalidate.
  // Note bodies (`/api/v1/notes/<path>`) are exempted from the SW
  // cache entirely. Why: a PUT autosave succeeded, but the cached
  // body still reflected the pre-edit content. On relaunch, the
  // editor showed the stale body — the user's typed content was
  // visibly "gone" until something forced a fresh GET. The note
  // payload is small enough that always going to network is fine,
  // and the freshness guarantee matters far more than offline-first
  // for the surface the user is actively editing.
  if (url.pathname.startsWith('/api/v1/') && e.request.method === 'GET') {
    if (isNoCacheGet(url)) return; // pass through to network
    e.respondWith(staleWhileRevalidate(e.request));
    return;
  }
  // API non-GET (POST/PUT/PATCH/DELETE): pass through, but invalidate any
  // cached GET responses for the same resource and its parent listing
  // endpoints. Without this, a PUT updates the file on disk but the SW
  // keeps serving the pre-PUT body to the next GET — a stale-cache data-
  // loss footgun. Invalidation happens fire-and-forget; we never block
  // the request on cache cleanup.
  if (url.pathname.startsWith('/api/')) {
    e.waitUntil(invalidateOnMutation(url));
    return;
  }

  // Static hashed bundle assets.
  if (SHELL.includes(url.pathname)) {
    e.respondWith(cacheFirst(e.request, SHELL_CACHE));
    return;
  }

  // SPA navigations — network-first, cache fallback.
  if (e.request.mode === 'navigate' || e.request.headers.get('accept')?.includes('text/html')) {
    e.respondWith(networkFirstHTML(e.request));
  }
});

async function cacheFirst(req: Request, cacheName: string): Promise<Response> {
  const cache = await caches.open(cacheName);
  const cached = await cache.match(req);
  if (cached) return cached;
  const res = await fetch(req);
  if (res.ok) cache.put(req, res.clone());
  return res;
}

async function networkFirstHTML(req: Request): Promise<Response> {
  const cache = await caches.open(SHELL_CACHE);
  try {
    const res = await fetch(req);
    if (res.ok) cache.put('/', res.clone()); // keep / warm as SPA fallback
    return res;
  } catch {
    const cached = (await cache.match(req)) ?? (await cache.match('/'));
    if (cached) return cached;
    return new Response('offline', { status: 503, statusText: 'offline' });
  }
}

// Endpoints that must never be served from the SW cache. Note bodies
// are the obvious one — staleness directly causes "my edit is gone"
// data-loss UX. Anything else added here should pass the same test:
// "would a stale response cause user-visible data loss or confusion".
function isNoCacheGet(url: URL): boolean {
  // /api/v1/notes/<path...> — single-note GET. The list endpoint
  // (/api/v1/notes with no extra segments / with a query string only)
  // is fine to cache because the listing is mod-time-keyed and the
  // mutation invalidation below also clears it.
  if (/^\/api\/v1\/notes\/[^?]/.test(url.pathname)) return true;
  return false;
}

// Best-effort cache invalidation when the client mutates an API resource.
// Strategy: drop every cache entry whose URL pathname starts with the
// mutated pathname's parent prefix. PUT /api/v1/notes/foo.md → drop the
// entry for /api/v1/notes/foo.md AND any /api/v1/notes (listing) entries
// — both could be stale. We don't try to be precise; the cache is
// re-populated on the next read either way.
async function invalidateOnMutation(url: URL): Promise<void> {
  try {
    const cache = await caches.open(API_CACHE);
    const keys = await cache.keys();
    // Compute the listing prefix — strip the trailing path segment so
    // the listing endpoint and any sibling fetches under the same
    // resource also drop. e.g. /api/v1/notes/foo.md → /api/v1/notes.
    const segs = url.pathname.split('/').filter(Boolean);
    const parent = '/' + segs.slice(0, -1).join('/');
    await Promise.all(
      keys.map((k) => {
        try {
          const kp = new URL(k.url).pathname;
          if (kp === url.pathname || kp === parent || kp.startsWith(parent + '?')) {
            return cache.delete(k);
          }
        } catch {}
        return undefined;
      })
    );
  } catch {
    // Cache API unavailable / browser pressure — nothing useful to do.
  }
}

async function staleWhileRevalidate(req: Request): Promise<Response> {
  const cache = await caches.open(API_CACHE);
  const cached = await cache.match(req);
  const networkPromise = fetch(req)
    .then((res) => {
      // Only cache successful, non-error responses. Cap response body size
      // pragmatically — caches.put on huge JSON has memory cost we don't
      // want for big vault listings (browser will evict though).
      if (res.ok) cache.put(req, res.clone()).catch(() => undefined);
      return res;
    })
    .catch(() => {
      // Offline (or origin unreachable). If we have cache, return it; else
      // a synthetic 503 with an `offline` flag the UI can detect.
      if (cached) return cached;
      return new Response(
        JSON.stringify({ error: 'offline', offline: true }),
        { status: 503, statusText: 'offline', headers: { 'Content-Type': 'application/json' } }
      );
    });
  // Return the cached value immediately if available, otherwise wait for
  // the network. Either way, the cache gets updated in the background for
  // next time.
  return cached ?? networkPromise;
}

// ── Web Push ──────────────────────────────────────────────────────
// Server fires reminders via VAPID-signed pushes; the SW receives
// them here, shows a notification, and routes the click back to
// the relevant page. Subscriptions are managed from the SPA via
// $lib/notifications.ts (request permission, subscribe, POST to
// /api/v1/push/subscribe).
//
// Payload shape (matches internal/push.Payload):
//   { title, body?, url?, tag?, icon? }
self.addEventListener('push', (event) => {
  const e = event as PushEvent;
  if (!e.data) return;
  let payload: {
    title?: string;
    body?: string;
    url?: string;
    tag?: string;
    icon?: string;
    category?: string;
  } = {};
  try {
    payload = e.data.json();
  } catch {
    payload = { title: e.data.text() };
  }
  const sw = self as unknown as ServiceWorkerGlobalScope;
  // Category styling: a leading glyph in the title gives a one-
  // glance cue in the notification stack, and per-category
  // vibration patterns let the user tell from feel which kind
  // of reminder fired without reading the screen.
  const titlePrefix =
    payload.category === 'event'    ? '📅 ' :
    payload.category === 'task'     ? '✓ ' :
    payload.category === 'deadline' ? '⏰ ' :
    '';
  const vibrate =
    payload.category === 'event'    ? [200, 100, 200] :
    payload.category === 'task'     ? [80, 40, 80] :
    payload.category === 'deadline' ? [120, 60, 120, 60, 200] :
    [200];
  // Tag groups same-category pushes so a flurry of task reminders
  // collapses into one notification card on Android instead of a
  // wall of N. Per-event tags (event-<id>) are still unique so
  // distinct events don't collide.
  const tag = payload.tag || (payload.category ? `granit-${payload.category}` : 'granit');
  e.waitUntil(
    sw.registration.showNotification((titlePrefix + (payload.title || 'Granit')).trim(), {
      body: payload.body,
      tag,
      icon: payload.icon || '/icon-192.png',
      badge: '/favicon.svg',
      // Custom data so the click handler can route to the right
      // page. Persists until the user dismisses or interacts.
      data: { url: payload.url || '/', category: payload.category },
      // Vibration is opt-in by browser; setting it is harmless on
      // browsers that ignore the field.
      vibrate,
      // Calendar events stay on screen until the user dismisses
      // (don't miss a meeting); tasks and deadlines auto-dismiss
      // like a regular push so the lock-screen doesn't hoard them.
      requireInteraction: payload.category === 'event'
    } as NotificationOptions & { vibrate?: number[]; requireInteraction?: boolean })
  );
});

self.addEventListener('notificationclick', (event) => {
  const e = event as NotificationEvent;
  e.notification.close();
  const target = (e.notification.data && (e.notification.data as { url?: string }).url) || '/';
  const sw = self as unknown as ServiceWorkerGlobalScope;
  e.waitUntil(
    (async () => {
      // Prefer focusing an open client showing the same path; fall
      // back to opening a new tab. Matches the user expectation
      // "the calendar tab I already had should come to the front".
      const clients = await sw.clients.matchAll({ type: 'window', includeUncontrolled: true });
      for (const client of clients) {
        try {
          const url = new URL(client.url);
          if (url.pathname === target || target === '/') {
            await client.focus();
            return;
          }
        } catch {}
      }
      await sw.clients.openWindow(target);
    })()
  );
});
