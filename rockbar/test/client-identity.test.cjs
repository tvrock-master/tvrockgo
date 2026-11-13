const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const vm = require('node:vm');
const identity = require('../src/client-identity.js');
const { clientVersion } = require('../scripts/client-version.cjs');
const { buildWeb } = require('../scripts/build-web.cjs');

test('Browser identity preserves request options and URL parameters', async () => {
  await identity.request(async (url, options) => {
    assert.equal(url, '/pms');
    assert.equal(options.method, 'PUT');
    assert.equal(options.body, '{}');
    assert.equal(options.headers.get('Content-Type'), 'application/json');
    assert.equal(options.headers.get('X-TvRock-Client'), 'TvRockRockbar/dev');
  }, '/pms', { method: 'PUT', body: '{}', headers: { 'Content-Type': 'application/json' } });
  const url = new URL(identity.socketURL('ws://localhost/con?a=1'));
  assert.equal(url.searchParams.get('a'), '1');
  assert.equal(url.searchParams.get('X-TvRock-Client'), 'TvRockRockbar/dev');
});

test('Version uses tag metadata and defaults to dev, regardless of package version', () => {
  assert.equal(clientVersion({}), 'dev');
  assert.equal(clientVersion({ APP_VERSION: 'dev', ROCKBAR_VERSION: '1.0.0' }), 'dev');
  assert.equal(clientVersion({ CI_COMMIT_TAG: 'v2.3.4' }), '2.3.4');
  assert.equal(clientVersion({ GITHUB_REF_TYPE: 'tag', GITHUB_REF_NAME: 'v3.4.5' }), '3.4.5');
});

test('Electron API and WebSocket use packaged release version and preserve cookies', async () => {
  class SharedClient {
    constructor(emit, request, createSocket) { this.request = request; this.createSocket = createSocket; }
  }
  class Socket {
    constructor(url, options) { this.url = url; this.options = options; }
  }
  const context = { module: { exports: {} }, Headers, require(name) {
    if (name === 'ws') return Socket;
    if (name === '../src/client.js') return { Client: SharedClient };
    if (name === '../package.json') return { version: '1.0.0', tvrockVersion: '2.3.4' };
    throw new Error(name);
  } };
  vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../electron/client.cjs'), 'utf8'), context);
  const client = new context.module.exports.Client(() => {}, async (url, options) => {
    assert.equal(options.headers.get('User-Agent'), 'TvRockRockbar/2.3.4');
    assert.equal(options.headers.get('Content-Type'), 'application/json');
    assert.equal(options.body, '{}');
  }, async () => [{ name: 'session', value: 'test' }]);
  await client.request('http://localhost/pms', { body: '{}', headers: { 'Content-Type': 'application/json' } });
  const socket = await client.createSocket('ws://localhost/con');
  assert.equal(socket.options.headers['User-Agent'], 'TvRockRockbar/2.3.4');
  assert.equal(socket.options.headers.Cookie, 'session=test');
});

test('Browser build embeds release identity and loads it before browser runtime', () => {
  const output = fs.mkdtempSync(path.join(os.tmpdir(), 'rockbar-identity-'));
  const previous = process.env.APP_VERSION;
  try {
    process.env.APP_VERSION = 'v2.3.4';
    buildWeb(output);
    const context = { module: { exports: {} } };
    vm.runInNewContext(fs.readFileSync(path.join(output, 'client-identity.js'), 'utf8'), context);
    assert.equal(context.module.exports.userAgent, 'TvRockRockbar/2.3.4');
    const html = fs.readFileSync(path.join(output, 'index.html'), 'utf8');
    assert.ok(html.indexOf('src="client-identity.js"') < html.indexOf('src="browser.js"'));
  } finally {
    if (previous === undefined) delete process.env.APP_VERSION;
    else process.env.APP_VERSION = previous;
    fs.rmSync(output, { recursive: true, force: true });
  }
});
