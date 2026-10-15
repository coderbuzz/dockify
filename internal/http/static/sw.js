// Light PWA service worker: caches /static/* only (cache-first, versioned by ?v=AssetVersion in the URL).
// HTML, API and WebSocket traffic always goes to the network.
var CACHE = 'dockify-static-' + (new URL(location.href).searchParams.get('v') || '0'); // registered as sw.js?v=AssetVersion
self.addEventListener('install', function () { self.skipWaiting(); });
self.addEventListener('activate', function (e) {
  e.waitUntil(caches.keys().then(function (keys) {
    return Promise.all(keys.filter(function (k) { return k !== CACHE && k.indexOf('dockify-static-') === 0; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});
self.addEventListener('fetch', function (e) {
  var req = e.request, url = new URL(req.url);
  if (req.method !== 'GET' || url.origin !== location.origin) return;
  var dir = location.pathname.replace(/sw\.js$/, ''); // .../static/
  if (url.pathname.indexOf(dir) !== 0 || url.pathname === location.pathname) return;
  e.respondWith(caches.open(CACHE).then(function (c) {
    return c.match(req).then(function (hit) {
      return hit || fetch(req).then(function (res) {
        if (res.ok) c.put(req, res.clone());
        return res;
      });
    });
  }));
});
