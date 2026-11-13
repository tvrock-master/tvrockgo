const { test } = require('node:test');
const assert = require('node:assert/strict');
const { followSystemTheme } = require('../src/systemTheme');

for (const legacy of [false, true]) {
  test(`system theme applies at startup, follows changes and releases its listener (${legacy ? 'legacy' : 'modern'})`, () => {
    let listener;
    let removed;
    const query = { matches: true };
    if (legacy) {
      query.addListener = fn => { listener = fn; };
      query.removeListener = fn => { removed = fn; };
    } else {
      query.addEventListener = (event, fn) => { assert.equal(event, 'change'); listener = fn; };
      query.removeEventListener = (event, fn) => { assert.equal(event, 'change'); removed = fn; };
    }
    const theme = { dark: false };
    const root = { dataset: {}, style: {} };
    const controller = followSystemTheme(theme, () => 0, { matchMedia: media => {
      assert.equal(media, '(prefers-color-scheme: dark)'); return query;
    } }, root);
    assert.equal(theme.dark, true);
    assert.equal(root.dataset.colorScheme, 'dark');
    assert.equal(root.style.colorScheme, 'dark');
    query.matches = false;
    listener();
    assert.equal(theme.dark, false);
    assert.equal(root.dataset.colorScheme, 'light');
    assert.equal(root.style.colorScheme, 'light');
    query.matches = true;
    listener();
    assert.equal(theme.dark, true);
    controller.dispose();
    assert.equal(removed, listener);
  });
}

test('forced themes override OS changes, refresh immediately and resume the latest OS theme', () => {
  let listener;
  let preference = 1;
  const query = { matches: true, addEventListener: (_, fn) => { listener = fn; }, removeEventListener() {} };
  const theme = {};
  const root = { dataset: {}, style: {} };
  const controller = followSystemTheme(theme, () => preference, { matchMedia: () => query }, root);
  const check = dark => {
    assert.equal(theme.dark, dark);
    assert.equal(root.dataset.colorScheme, dark ? 'dark' : 'light');
    assert.equal(root.style.colorScheme, dark ? 'dark' : 'light');
  };
  check(false);
  listener();
  check(false);
  preference = 2;
  controller.refresh();
  check(true);
  query.matches = false;
  listener();
  check(true);
  preference = 0;
  controller.refresh();
  check(false);
  query.matches = true;
  listener();
  check(true);
  controller.dispose();
});
