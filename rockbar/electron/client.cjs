const WebSocket = require('ws');
const { Client: SharedClient } = require('../src/client.js');
const userAgent = 'TvRockRockbar/' + (require('../package.json').tvrockVersion || 'dev');
class Client extends SharedClient {
  constructor(emit, request = fetch, getCookies = async () => []) {
    super(emit, (url, options = {}) => {
      const headers = new Headers(options.headers);
      headers.set('User-Agent', userAgent);
      return request(url, { ...options, headers });
    }, async url => {
      // Node's ws does not share Electron's HTTP/login cookie store.
      const cookies = await getCookies(url.replace(/^ws/, 'http'));
      const cookie = cookies.map(({ name, value }) => `${name}=${value}`).join('; ');
      return new WebSocket(url, {
        handshakeTimeout: 8000,
        headers: { 'User-Agent': userAgent, ...(cookie ? { Cookie: cookie } : {}) },
        followRedirects: false,
      });
    });
    this.nativePlayback=true;
  }
}
module.exports = { Client };
