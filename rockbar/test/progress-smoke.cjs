const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const fs = require('node:fs');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 680, height: 300 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(() => {
      window.rockbar = {
        platform: 'electron', onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {},
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      window.progressDevice = { ID: 'T1', State: 2, CurrentService: 65537,
        CurrentProgram: { Start: 1000, End: 4600, Title: '60分の番組', Sid: 65537, Eid: 1 } };
      updateRockbar({ type: 'status', value: { connected: true, message: '接続済み' } });
      updateRockbar({ type: 'stations', value: [{ Sid: 65537, Tsp: 1, View: 1, Name: '星見野テレビ' }] });
      updateRockbar({ type: 'devices', value: [progressDevice] });
      updateRockbar({ type: 'server-time', value: 2199500 });
    });
    assert.equal(await page.locator('#program-progress-text').textContent(), '19 / 60min(33%)');
    await page.waitForFunction(() => document.querySelector('#program-progress-text').textContent === '20 / 60min(33%)');
    assert.equal(await page.locator('#program-progress-bar .filled').count(), 6);
    assert.equal(await page.locator('#program-progress-bar').evaluate(el => el.getBoundingClientRect().height), 12);
    await page.evaluate(() => updateRockbar({ type: 'server-time', value: 2170000 }));
    const partial = page.locator('#program-progress-bar span').nth(6);
    const fill = await partial.evaluate(el => Number(el.style.getPropertyValue('--fill')));
    assert.ok(fill >= 0.5 && fill < 0.51);
    await page.waitForTimeout(1100);
    const opacity = await partial.evaluate(el => Number(getComputedStyle(el, '::before').opacity));
    assert.ok(opacity > 0 && opacity < 1, 'partial segment has intermediate brightness');
    await page.evaluate(() => updateRockbar({ type: 'server-time', value: 2179000 }));
    assert.ok(await partial.evaluate(el => Number(el.style.getPropertyValue('--fill'))) > fill);
    await page.evaluate(() => updateRockbar({ type: 'server-time', value: 2200000 }));
    await page.waitForTimeout(1100);
    const output = path.resolve(__dirname, '../test-output');
    fs.mkdirSync(output, { recursive: true });
    for (const width of [680, 370]) {
      await page.setViewportSize({ width, height: 300 });
      assert.ok(await page.locator('main').evaluate(el => {
        const available = el.getBoundingClientRect().width;
        const right = el.querySelector('aside').getBoundingClientRect().width;
        return Math.abs(right - Math.max(71.25, available * 0.18)) < 1;
      }), `device panel is three quarters of its previous width at ${width}px`);
      assert.ok(await page.locator('.program-top').evaluate(el => {
        const children = [...el.children].map(child => child.getBoundingClientRect());
        return children.every(rect => rect.right <= el.getBoundingClientRect().right + 1) &&
          children[0].right <= children[1].left && children[1].right <= children[2].left;
      }), `header fits at ${width}px`);
      assert.ok(await page.locator('.program-top').evaluate(el => {
        const progress = document.querySelector('#program-progress');
        const state = document.querySelector('#state').getBoundingClientRect();
        const gap = state.left - progress.getBoundingClientRect().right;
        return Math.abs(state.right - el.getBoundingClientRect().right) < 1 &&
          gap >= 4 && gap <= 9 && getComputedStyle(progress).borderRightWidth === '1px';
      }), `progress sits beside the state with a divider at ${width}px`);
      await page.screenshot({ path: path.join(output, `progress-${width}.png`) });
    }
    await page.evaluate(() => updateRockbar({ type: 'server-time', value: 4600000 }));
    assert.equal(await page.locator('#program-progress-text').textContent(), '60 / 60min(100%)');
    assert.equal(await page.locator('#program-progress-bar .filled').count(), 20);
    assert.ok(await page.locator('#program-progress-bar').evaluate(el =>
      [...el.children].every(segment => segment.style.getPropertyValue('--fill') === '1')));
    await page.evaluate(() => { progressDevice.CurrentProgram = null; updateRockbar({ type: 'devices', value: [progressDevice] }); });
    assert.equal(await page.locator('#program-progress').isVisible(), false);
    await page.evaluate(() => { progressDevice.CurrentProgram = { Start: 1000, End: 4600 }; progressDevice.State = 0; updateRockbar({ type: 'devices', value: [progressDevice] }); });
    assert.equal(await page.locator('#program-progress').isVisible(), false);
    await page.evaluate(() => { progressDevice.State = 3; updateRockbar({ type: 'devices', value: [progressDevice] }); });
    assert.equal(await page.locator('#program-progress').isVisible(), true);
    assert.equal(await page.locator('#state').textContent(), 'REC');
    await page.evaluate(() => {
      progressDevice.State = 2;
      updateRockbar({ type: 'devices', value: [progressDevice] });
      window.watchSchedule = { id: 1, deviceid: 'T1', validate: true, watchonly: true, starttime: 4000, endtime: 5000 };
      updateRockbar({ type: 'schedules', value: [watchSchedule] });
    });
    assert.equal(await page.locator('#state').textContent(), 'LIVE');
    assert.equal(await page.locator('#state').evaluate(el => getComputedStyle(el).color), 'rgb(255, 220, 98)');
    assert.equal(await page.locator('#state').evaluate(el => getComputedStyle(el).animationName), 'recording-blink');
    assert.equal(await page.locator('#devices .led').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(255, 220, 98)');
    assert.equal(await page.locator('#devices small').textContent(), 'LIVE');
    await page.screenshot({ path: path.join(output, 'watch-only.png') });
    for (const change of [{ watchonly: false }, { starttime: 6000 }, { complete: true }, { endtime: 4500 }]) {
      await page.evaluate(change => updateRockbar({ type: 'schedules', value: [{ ...watchSchedule, ...change }] }), change);
      assert.equal(await page.locator('#state').textContent(), 'ON AIR');
      assert.equal(await page.locator('#state').evaluate(el => getComputedStyle(el).animationName), 'none');
      assert.equal(await page.locator('#devices .led').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(112, 232, 184)');
    }
    await page.evaluate(() => {
      updateRockbar({ type: 'schedules', value: [watchSchedule] });
      progressDevice.State = 3;
      updateRockbar({ type: 'devices', value: [progressDevice] });
    });
    assert.equal(await page.locator('#state').textContent(), 'REC');
    assert.equal(await page.locator('#state').evaluate(el => getComputedStyle(el).color), 'rgb(255, 91, 112)');
    assert.equal(await page.locator('#devices .led').evaluate(el => getComputedStyle(el).backgroundColor), 'rgb(255, 112, 127)');
    await page.evaluate(() => {
      progressDevice.State = 2;
      updateRockbar({ type: 'devices', value: [progressDevice] });
      updateRockbar({ type: 'schedules', value: [] });
    });
    assert.equal(await page.locator('#state').textContent(), 'ON AIR');
    assert.equal(await page.locator('#state').evaluate(el => getComputedStyle(el).animationName), 'none');
    await page.evaluate(() => updateRockbar({ type: 'status', value: { connected: false, message: '切断' } }));
    assert.equal(await page.locator('#program-progress').isVisible(), false);
    assert.deepEqual(errors, []);
    console.log('PASS progress: server time, ticking, segments, narrow layout, completion, no program, OFF, REC, disconnect');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
