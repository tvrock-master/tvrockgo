(function (root, factory) {
  if (typeof module !== 'undefined') module.exports = factory();
  else root.rockbarIdentity = factory();
})(globalThis, function () {
  // Replaced only in the browser build; Electron uses packaged metadata.
  const userAgent = 'TvRockRockbar/' + 'dev';
  function request(fetchRequest, url, options = {}) {
    const headers = new Headers(options.headers);
    headers.set('X-TvRock-Client', userAgent);
    return fetchRequest(url, { ...options, headers });
  }
  function socketURL(url) {
    const target = new URL(url);
    target.searchParams.set('X-TvRock-Client', userAgent);
    return target.href;
  }
  return { userAgent, request, socketURL };
});
