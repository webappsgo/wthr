const CACHE_VERSION = '{asset_stamp}';
const CACHE_NAME = '{app_name}-cache-' + CACHE_VERSION;
const STRINGS_CACHE = '{app_name}-offline-strings';

const PRECACHE_ASSETS = [
  '/static/css/common.css?v={asset_stamp}',
  '/static/css/components.css?v={asset_stamp}',
  '/static/css/public.css?v={asset_stamp}',
  '/static/js/app.js?v={asset_stamp}',
  '/static/icons/icon-192.png',
  '/static/icons/icon-512.png'
];

// No skipWaiting() on install: an updated worker waits for the update prompt (SKIP_WAITING), a first install activates by itself
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(PRECACHE_ASSETS))
  );
});

self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(keys => Promise.all(
        keys
          .filter(key => key.startsWith('{app_name}-cache-') && key !== CACHE_NAME)
          .map(key => caches.delete(key))
      ))
      .then(() => self.clients.claim())
  );
});

// A navigation never waits on the network forever: it rejects after NAV_TIMEOUT_MS and the catch below serves the cached page or the offline page
const NAV_TIMEOUT_MS = 8000;
function withTimeout(promise, ms) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error('timeout')), ms);
    promise.then(
      value => { clearTimeout(timer); resolve(value); },
      error => { clearTimeout(timer); reject(error); }
    );
  });
}

self.addEventListener('message', event => {
  if (event.data && event.data.type === 'SKIP_WAITING') {
    self.skipWaiting();
  }
});

self.addEventListener('fetch', event => {
  const request = event.request;
  const url = new URL(request.url);

  if (request.method !== 'GET' || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/api/')) return;

  if (request.mode === 'navigate') {
    event.respondWith(
      withTimeout(fetch(request), NAV_TIMEOUT_MS)
        .then(async response => {
          const cache = await caches.open(CACHE_NAME);
          if (response.ok && !response.redirected && response.headers.get('X-Offline-Cache') === 'public') {
            try {
              await cache.put(request, response.clone());
            } catch (error) {
              console.warn('Could not cache public page for offline use', error);
            }
          } else {
            try {
              await cache.delete(request);
            } catch (error) {
              console.warn('Could not remove a private page from the offline cache', error);
            }
          }
          return response;
        })
        .catch(() => offlineNavigationFallback(request))
        .catch(() => offlineFallbackResponse())
    );
    return;
  }

  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request)
        .then(cached => cached || fetch(request).then(response => {
          if (response.ok && !response.redirected) {
            const clone = response.clone();
            caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          }
          return response;
        }))
        .catch(() => new Response('', { status: 504, statusText: 'Gateway Timeout' }))
    );
    return;
  }

  event.respondWith(
    fetch(request)
      .catch(async () =>
        (await caches.match(request))
          || new Response('', { status: 504, statusText: 'Gateway Timeout' })
      )
  );
});

async function offlineNavigationFallback(request) {
  try {
    const cached = await caches.match(request, { cacheName: CACHE_NAME });
    if (cached) return cached;
  } catch (error) {
    console.warn('Could not read the cached public page', error);
  }
  return offlineFallbackResponse();
}

const DEFAULT_STRINGS = {
  lang: 'en', dir: 'ltr', title: 'Offline', heading: 'You are offline',
  message: 'This page could not be loaded and no cached copy is available. Check your connection and try again.'
};

function escapeHtml(text) {
  return String(text).replace(/[&<>\"]/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[character]));
}

async function offlineFallbackResponse() {
  let strings = DEFAULT_STRINGS;
  try {
    const cache = await caches.open(STRINGS_CACHE);
    const stored = await cache.match('/offline-strings.json');
    if (stored) strings = Object.assign({}, DEFAULT_STRINGS, await stored.json());
  } catch (error) {
    console.warn('Could not read translated offline strings', error);
  }
  return new Response(
    '<!doctype html><html lang="' + escapeHtml(strings.lang) + '" dir="' + escapeHtml(strings.dir) + '"><head><meta charset="utf-8">'
      + '<meta name="viewport" content="width=device-width,initial-scale=1">'
      + '<title>' + escapeHtml(strings.title) + '</title></head><body><main>'
      + '<h1>' + escapeHtml(strings.heading) + '</h1><p>' + escapeHtml(strings.message) + '</p>'
      + '</main></body></html>',
    { status: 503, statusText: 'Service Unavailable',
      headers: { 'Content-Type': 'text/html; charset=utf-8' } }
  );
}

self.addEventListener('push', event => {
  if (!event.data) return;
  const data = event.data.json();
  event.waitUntil(
    self.registration.showNotification(data.title || '{app_name}', {
      body: data.body || '',
      icon: '/static/icons/icon-192.png',
      badge: '/static/icons/icon-192.png',
      data: data.url || '/'
    })
  );
});

self.addEventListener('notificationclick', event => {
  event.notification.close();
  event.waitUntil(clients.openWindow(event.notification.data || '/'));
});
