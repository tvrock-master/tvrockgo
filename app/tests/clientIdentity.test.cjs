const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

for (const version of [undefined, '2.3.4']) {
  test(`App API and WebSocket identify version ${version || 'dev'}`, () => {
    const context = { module: { exports: {} }, URL, process: { env: { VUE_APP_VERSION: version } } };
    vm.runInNewContext(fs.readFileSync(path.join(__dirname, '../src/clientIdentity.js'), 'utf8')
      .replace('export default', 'module.exports ='), context);
    const identity = context.module.exports;
    const url = new URL(identity.socketURL('wss://example.test/con?a=1'));
    assert.equal(url.searchParams.get('X-TvRock-Client'), `TvRockApp/${version || 'dev'}`);
    assert.equal(url.searchParams.get('a'), '1');
    const axios = { defaults: { headers: { common: {} } } };
    const main = fs.readFileSync(path.join(__dirname, '../src/main.js'), 'utf8')
      .replace(/^import .*$/gm, '').split('Vue.config')[0];
    vm.runInNewContext(main, { axios, clientIdentity: identity });
    assert.equal(axios.defaults.headers.common['X-TvRock-Client'], identity.client);
  });
}
