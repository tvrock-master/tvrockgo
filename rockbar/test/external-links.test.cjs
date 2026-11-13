const { test } = require('node:test');
const assert = require('node:assert/strict');
const { openProgramLink } = require('../electron/external-links.cjs');

test('program links open only HTTP and HTTPS pages in the default browser', async () => {
  const opened = [];
  const shell = { openExternal: async url => opened.push(url) };
  for (const url of ['https://example.com/info?q=1&lang=ja#details', 'http://example.com/']) {
    assert.equal(await openProgramLink(shell, url), true);
  }
  assert.equal(opened.length, 2);
  for (const url of ['file:///C:/test.exe', 'javascript:alert(1)', 'mailto:test@example.com', 'custom:launch', 'invalid', 'https://user:pass@example.com/']) {
    assert.equal(await openProgramLink(shell, url), false);
  }
  assert.equal(opened.length, 2);
  assert.equal(await openProgramLink({ openExternal: async () => { throw new Error('unavailable'); } }, 'https://example.com/'), false);
});
