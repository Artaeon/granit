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
      const keys = await caches.keys();
      const keep = new Set([SHELL_CACHE, API_CACHE]);
      await Promise.all(keys.filter((k) => !keep.has(k)).map((k) => caches.delete(k)));
      await (self as unknown as ServiceWorkerGlobalScope).clients.claim();
    })()
  );
});

self.addEventListener('fetch', (event) => {
  const e = event as FetchEvent;
  const url = new URL(e.request.url);

  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/ws')) return; // WebSocket — never cache

  // API: only cache GETs, stale-while-revalidate.
  if (url.pathname.startsWith('/api/v1/') && e.request.method === 'GET') {
    e.respondWith(staleWhileRevalidate(e.request));
    return;
  }
  // API non-GET (POST/PUT/PATCH/DELETE): pass through. If offline, fetch will
  // throw and the page surfaces a save error / retry — handled in-app.
  if (url.pathname.startsWith('/api/')) return;

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
