const { test } = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');
const { GuideWindow } = require('../electron/guide.cjs');
const fs = require('node:fs');
const vm = require('node:vm');

class Window extends EventEmitter {
  constructor(options) { super(); this.options = options; this.webContents = new EventEmitter(); this.webContents.setWindowOpenHandler = fn => { this.openHandler = fn; }; }
  setMenu(menu) { this.menu = menu; }
  show() { this.visible = true; this.emit('show'); }
  hide() { this.visible = false; this.emit('hide'); }
  focus() {}
  restore() { this.minimized = false; }
  isVisible() { return this.visible; }
  isMinimized() { return this.minimized; }
  isDestroyed() { return this.destroyed; }
  destroy() { this.destroyed = true; this.emit('closed'); }
  async loadURL(url) { this.url = url; }
}
const nativeImage = { createFromPath: () => ({ resize: () => 'icon' }) };
test('guide reuses page on toggle, restores minimized window, and resets on server change', () => {
  const updates = [];
  const guide = new GuideWindow(Window, nativeImage, (...args) => updates.push(args));
  guide.toggle('https://example.test/tv');
  const first = guide.window;
  assert.equal(first.url, 'https://example.test/tv/static/index.html');
  assert.equal(first.menu, null);
  assert.equal(first.options.webPreferences.nodeIntegration, false);
  guide.toggle('https://example.test/tv');
  assert.equal(first.visible, false);
  guide.toggle('https://example.test/tv');
  assert.equal(guide.window, first);
  assert.equal(first.visible, true);
  first.minimized = true;
  guide.toggle('https://example.test/tv');
  assert.equal(first.minimized, false);
  assert.equal(first.visible, true);
  guide.changeServer('https://other.test');
  assert.equal(first.destroyed, true);
  assert.equal(guide.window, null);
  guide.toggle('https://other.test');
  assert.equal(guide.window.url, 'https://other.test/static/index.html');
  guide.close();
  assert.deepEqual(updates.at(-1), ['guide-visible', false]);
});
test('guide blocks external navigation and rejects non-web server URLs', () => {
  const guide = new GuideWindow(Window, nativeImage, () => {});
  assert.throws(() => guide.toggle('file:///tmp'));
  guide.toggle('http://localhost');
  let blocked = false;
  guide.window.webContents.emit('will-navigate', { preventDefault() { blocked = true; } }, 'https://other.test');
  assert.equal(blocked, true);
  assert.deepEqual(guide.window.openHandler({ url: 'https://other.test' }), { action: 'deny' });
});
test('guide opens detail and search links in the default browser while keeping its page', async () => {
  const { openProgramLink } = require('../electron/external-links.cjs');
  const opened = [];
  const shell = { openExternal: async url => opened.push(url) };
  const guide = new GuideWindow(Window, nativeImage, () => {}, url => openProgramLink(shell, url));
  guide.toggle('http://localhost');
  const page = guide.window.url;
  for (const url of ['https://example.com/info?q=1&lang=ja#details', 'https://www.google.com/search?q=program&udm=14', 'file:///C:/test.exe', 'javascript:alert(1)']) {
    assert.deepEqual(guide.window.openHandler({ url }), { action: 'deny' });
  }
  await new Promise(resolve => setImmediate(resolve));
  assert.deepEqual(opened, ['https://example.com/info?q=1&lang=ja#details', 'https://www.google.com/search?q=program&udm=14']);
  assert.equal(guide.window.url, page);
});
test('browser guide opens a new tab at the current server including proxy prefix', async () => {
  const opened = [];
  const window = { open: (...args) => opened.push(args), addEventListener() {} };
  vm.runInNewContext(fs.readFileSync(require.resolve('../src/browser.js'), 'utf8'), {
    window, location: { origin: 'https://example.test', pathname: '/tv/static/rockbar/index.html' },
    rockbarClient: { Client: class {} },
  });
  assert.deepEqual(await window.rockbar.guide().then(value => JSON.parse(JSON.stringify(value))), { ok: true });
  assert.deepEqual(opened, [['https://example.test/tv/static/index.html', '_blank', 'noopener']]);
});
