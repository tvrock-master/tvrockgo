const { _electron } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const fs = require('node:fs');
const assert = require('node:assert/strict');

(async () => {
  const env = { ...process.env };
  delete env.ELECTRON_RUN_AS_NODE;
  const app = await _electron.launch({ executablePath: require('electron'), args: [path.join(__dirname, 'fixtures/program-main.cjs')], env });
  try {
    const page = await app.firstWindow();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.waitForFunction(() => typeof window.testUpdate === 'function' && document.querySelector('#current-program .program-scroll-content'));
    await page.evaluate(() => {
      window.device = { ID: 'REC01', State: 2, CurrentService: 1,
        CurrentProgram: { Sid: 1, Eid: 1, Start: 1000, End: 4000, Title: '表示検証中の番組タイトル', Subtitle: '表示検証中のサブタイトル', Content: '表示検証用の詳細' },
        NextProgram: { Sid: 1, Eid: 2, Start: 4000, End: 6000, Title: '次の番組タイトル' },
      };
      testUpdate({ type: 'status', value: { connected: true } });
      testUpdate({ type: 'server-time', value: 2000000 });
      testUpdate({ type: 'stations', value: [{ Sid: 1, Name: '表示検証用の局名', Tsp: 1, View: 1 }] });
      testUpdate({ type: 'devices', value: [device] });
    });
    // Inspect actual text bounds in Electron, including non-default display scaling.
    for (const zoom of [1, 1.5]) {
      await app.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.setZoomFactor(value), zoom);
      await page.evaluate(() => {
        for (let index = 0; index < 15; index++) {
          device.CurrentProgram.Subtitle = `更新された副題 ${index}`;
          device.CurrentProgram.End++;
          testUpdate({ type: 'devices', value: [device] });
        }
      });
      assert.equal(await page.locator('.program-scroll-outgoing').count(), 0, 'supplemental metadata does not animate');
      await page.locator('#timekeeper-toggle').click({ force: true });
      await page.locator('#timekeeper-toggle').click({ force: true });
      await page.waitForTimeout(500);
      for (const id of ['service', 'title', 'subtitle', 'next']) {
        const bounds = await page.locator(`#${id}`).evaluate(element => {
          const range = document.createRange();
          range.selectNodeContents(element);
          const text = range.getBoundingClientRect();
          const box = element.closest('.program-scroll').getBoundingClientRect();
          return { visible: element.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }),
            overlap: text.width > 0 && text.height > 0 && text.bottom > box.top && text.top < box.bottom && text.right > box.left && text.left < box.right };
        });
        assert.ok(bounds.visible && bounds.overlap, `${id} is painted inside its viewport at zoom ${zoom}`);
      }
      await page.locator('#title').click({ force: true });
      assert.equal(await page.locator('#program-content-text').textContent(), '表示検証用の詳細');
      await page.keyboard.press('Escape');
    }
    const output = path.resolve(__dirname, '../test-output');
    fs.mkdirSync(output, { recursive: true });
    await page.screenshot({ path: path.join(output, 'electron-program-visible.png') });
    assert.deepEqual(errors, []);
    console.log('PASS Electron: title, service, next program and details remain visible after metadata/mode updates at 100% and 150% zoom');
  } finally { await app.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
