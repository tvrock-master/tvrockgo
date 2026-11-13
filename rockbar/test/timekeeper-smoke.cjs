const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const fs = require('node:fs');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 300 }, colorScheme: 'dark' });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('https://fonts.googleapis.com/**', route => route.abort());
    await page.addInitScript(() => {
      window.modes = [];
      window.rockbar = {
        platform: 'electron', onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {}, setMode: async mode => window.modes.push(mode),
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      const now = new Date(2026, 9, 24, 22, 12, 4).getTime() / 1000;
      const devices = Array.from({ length: 4 }, (_, i) => ({ ID: `REC0${i + 1}`, State: i < 2 ? 3 : 0, CurrentService: i + 1,
        CurrentProgram: { Start: now - 754, End: now + 1076, Title: `録画タイトル${i + 1}`, Sid: i + 1, Eid: i + 1 } }));
      const schedules = devices.slice(0, 2).flatMap((device, i) => [
        { id: i + 1, deviceid: device.ID, validate: true, starttime: now - 754, endtime: now + 1076, title: device.CurrentProgram.Title, sid: i + 1 },
        { id: i + 5, deviceid: device.ID, validate: true, starttime: now + 6446, endtime: now + 8276, title: `次の予約タイトル${i + 3}`, sid: i + 3 },
      ]);
      updateRockbar({ type: 'status', value: { connected: true, message: '接続済み' } });
      updateRockbar({ type: 'stations', value: ['風待ち放送', '星見野テレビ', '雲庭チャンネル', '灯台テレビ'].map((Name, i) => ({ Sid: i + 1, Name, Tsp: 1, View: 1 })) });
      updateRockbar({ type: 'server-time', value: now * 1000 });
      updateRockbar({ type: 'schedules', value: schedules });
      updateRockbar({ type: 'devices', value: devices });
    });
    const panel = page.locator('.device-panel');
    const separator = () => page.locator('#devices button').first().evaluate(el => getComputedStyle(el, '::after').opacity);
    assert.equal(await separator(), '0', 'separators stay hidden in device mode');
    const before = await panel.boundingBox();
    await page.evaluate(() => {
      document.querySelector('#timekeeper-toggle').click();
      const animation = document.querySelector('.device-panel').getAnimations()[0];
      if (!animation) throw new Error('Device frame transition is missing');
      animation.pause();
      animation.currentTime = 180;
    });
    const during = await panel.boundingBox();
    assert.ok(during.x < before.x && during.x > 15, 'frame moves left during transition');
    await page.evaluate(() => document.querySelector('.device-panel').getAnimations().forEach(animation => animation.finish()));
    await page.waitForTimeout(450);
    assert.ok((await panel.boundingBox()).width > before.width * 3);
    assert.equal(await page.locator('.program').evaluate(el => el.inert), true);
    assert.equal(await page.locator('.tk-heading:visible').count(), 8);
    assert.equal(await separator(), '1', 'separators fade in for timekeeper mode');
    assert.equal(await page.locator('#stop').isEnabled(), true, 'shared recording controls remain available');
    assert.equal(await page.locator('#devices button').first().locator('.tk-cell').nth(0).textContent(), '録画タイトル1');
    const elapsed = page.locator('#devices button').first().locator('.tk-cell').nth(3);
    const initial = await elapsed.textContent();
    const title = await page.locator('#title').textContent();
    await page.waitForTimeout(1200);
    assert.notEqual(await elapsed.textContent(), initial);
    assert.equal(await page.locator('#title').textContent(), title, 'hidden program display remains untouched');
    const output = path.resolve(__dirname, '../test-output');
    fs.mkdirSync(output, { recursive: true });
    await page.screenshot({ path: path.join(output, 'timekeeper.png') });
    // Header and row columns remain aligned at the default desktop width.
    await page.setViewportSize({ width: 680, height: 240 });
    await page.waitForTimeout(100);
    const headingX = await page.locator('.tk-heading').first().evaluate(el => el.getBoundingClientRect().x);
    const cellX = await page.locator('#devices button').first().locator('.tk-cell').first().evaluate(el => el.getBoundingClientRect().x);
    assert.ok(Math.abs(headingX - cellX) < 1);
    assert.ok(await panel.evaluate(el => el.scrollWidth > el.clientWidth));
    await panel.evaluate(el => { el.scrollLeft = 400; });
    await panel.evaluate(el => { el.scrollLeft = 0; });
    await page.locator('#timekeeper-toggle').click();
    await page.waitForTimeout(500);
    assert.equal(await page.locator('.program').evaluate(el => el.inert), false);
    assert.equal(await page.locator('.tk-cell:visible').count(), 0);
    assert.equal(await separator(), '0', 'separators fade out on return to device mode');
    const stopped = await elapsed.textContent();
    await page.waitForTimeout(1100);
    assert.equal(await elapsed.textContent(), stopped, 'hidden timekeeper stops counting');
    assert.deepEqual(await page.evaluate(() => window.modes), ['timekeeper', 'device']);
    await page.setViewportSize({ width: 370, height: 160 });
    assert.ok(await page.locator('#timekeeper-toggle').isVisible());
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.locator('#timekeeper-toggle').click();
    assert.equal(await panel.evaluate(el => getComputedStyle(el).transitionDuration), '0s');
    assert.deepEqual(errors, []);
    console.log('Timekeeper transitions, counters, inactive displays and compact layout passed');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
