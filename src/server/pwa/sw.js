const CACHE_VERSION = '{asset_stamp}';
const CACHE_NAME = '{app_name}-cache-' + CACHE_VERSION;

const PRECACHE_ASSETS = [
  '/static/css/common.css?v={asset_stamp}',
  '/static/css/components.css?v={asset_stamp}',
  '/static/css/public.css?v={asset_stamp}',
  '/static/js/app.js?v={asset_stamp}',
  '/static/icons/icon-192.png',
  '/static/icons/icon-512.png',
  '/offline.html'
];

self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(PRECACHE_ASSETS))
      .then(() => self.skipWaiting())
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
      fetch(request)
        .then(response => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
          return response;
        })
        .catch(async () =>
          (await caches.match(request))
            || (await caches.match('/offline.html'))
            || offlineFallbackResponse()
        )
    );
    return;
  }

  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request)
        .then(cached => cached || fetch(request).then(response => {
          const clone = response.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(request, clone));
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

function offlineFallbackResponse() {
  return new Response(
    '<!doctype html><html lang="en"><head><meta charset="utf-8">'
      + '<meta name="viewport" content="width=device-width,initial-scale=1">'
      + '<title>Offline</title></head><body><main>'
      + '<h1>You are offline</h1><p>This page could not be loaded and no '
      + 'cached copy is available. Check your connection and try again.</p>'
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
