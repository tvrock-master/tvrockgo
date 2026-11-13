(() => {
  // The desktop renderer uses the IPC API supplied by preload instead.
  if (window.rockbar) return;
  const prefix = location.pathname.split('/static/rockbar')[0];
  const base = location.origin + (location.pathname.includes('/static/rockbar') ? prefix : '');
  const listeners = new Set();
  const client = new rockbarClient.Client(
    (type, value) => listeners.forEach(callback => callback({ type, value })),
    (url, options) => rockbarIdentity.request(fetch, url, { ...options, credentials: 'same-origin' }),
    url => new WebSocket(rockbarIdentity.socketURL(url)),
  );
  let initialized = false;
  let loginRequested = false;
  const result = callback => {
    try { callback(); return { ok: true }; }
    catch (error) { return { error: error.message }; }
  };
  function connect(value) {
    return result(() => {
      if (rockbarProtocol.serverURL(value) !== base) throw new Error('ブラウザ版は表示中のtvrockgoに接続します');
      initialized = true;
      void client.connect(base);
    });
  }
  window.rockbar = {
    platform: 'browser',
    guide: async () => result(() => { window.open(base + '/static/index.html', '_blank', 'noopener'); }),
    settings: async () => ({ server: base }),
    connect: async value => connect(value),
    action: async (id, action) => result(() => client.send(id, action)),
    playback: async (id, action, seconds) => result(() => client.playback(id,action,seconds)),
    playbackRecordings: () => client.playbackRecordings(),
    tune: async (id, sid) => result(() => client.tune(id, sid)),
    watchDevice: async id => client.watchDevice(id),
    setMode: async mode => client.setMode(mode),
    sleepTimerPause: async paused => {
      try { return await client.setSleepTimerPause(paused); }
      catch (error) { return { error: error.message }; }
    },
    login: async () => {
      loginRequested = true;
      window.open(base + '/login', '_blank', 'noopener');
      return { ok: true };
    },
    onUpdate: callback => {
      listeners.add(callback);
      return () => listeners.delete(callback);
    },
  };
  window.addEventListener('focus', () => {
    if (initialized && (loginRequested || !client.connected)) {
      loginRequested = false;
      connect(base);
    }
  });
  window.addEventListener('pagehide', () => client.close());
  window.addEventListener('pageshow', event => { if (event.persisted) connect(base); });
})();
