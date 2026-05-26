// Conduit service worker.
//
// Strategy:
//   /api/*, /ws   → network only (live data, never cached)
//   GET other     → cache-first, fall back to network and update cache
//   navigation    → network-first, fall back to cached index.html
//
// Bumping CACHE_VERSION invalidates the cache on next visit.

const CACHE_VERSION = 'conduit-v1';

self.addEventListener('install', (event) => {
  event.waitUntil(
    (async () => {
      const cache = await caches.open(CACHE_VERSION);
      await cache.addAll([
        '/',
        '/manifest.webmanifest',
        '/icon.svg',
        '/icon-maskable.svg',
      ]);
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(
        keys.filter((k) => k !== CACHE_VERSION).map((k) => caches.delete(k)),
      );
      await self.clients.claim();
    })(),
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);

  // Never cache or interpose on API / WebSocket / token-bearing URLs.
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/ws')) return;

  // Navigation requests: network-first so users see updates, fall back
  // to cached index.html (the SPA shell) when offline.
  if (req.mode === 'navigate') {
    event.respondWith(networkFirstNav(req));
    return;
  }

  // Same-origin assets: cache-first.
  if (url.origin === self.location.origin) {
    event.respondWith(cacheFirst(req));
  }
});

async function networkFirstNav(req) {
  try {
    const res = await fetch(req);
    const cache = await caches.open(CACHE_VERSION);
    cache.put('/', res.clone());
    return res;
  } catch {
    const cache = await caches.open(CACHE_VERSION);
    const cached = await cache.match('/');
    if (cached) return cached;
    return new Response('Offline and no cached app shell.', { status: 503 });
  }
}

async function cacheFirst(req) {
  const cache = await caches.open(CACHE_VERSION);
  const cached = await cache.match(req);
  if (cached) {
    // Refresh in the background.
    fetch(req)
      .then((r) => {
        if (r.ok) cache.put(req, r.clone());
      })
      .catch(() => {});
    return cached;
  }
  try {
    const res = await fetch(req);
    if (res.ok) cache.put(req, res.clone());
    return res;
  } catch {
    return new Response('Offline', { status: 503 });
  }
}
