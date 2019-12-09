"use strict";

function isHtmlPage(event) {
    return event.request.method === 'GET' && event.request.headers.get('accept').includes('text/html');
}

self.addEventListener('install', function(event) {
  var offlinePage = new Request('/offline/');
  event.waitUntil(
    fetch(offlinePage).then(function(response) {
      return caches.open('offline').then(function(cache) {
        return cache.put(offlinePage, response);
      });
  }));
});
/*
self.addEventListener('fetch', function(event) {
  event.respondWith(
    fetch(event.request).catch(function(error) {
      return caches.open('offline').then(function(cache) {
        return cache.match('/offline/');
      });
    }
  ));
});*/
self.addEventListener('refreshOffline', function(response) {
  return caches.open('offline').then(function(cache) {
    return cache.put(offlinePage, response);
  });
});