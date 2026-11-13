const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1000, height: 600 } });
    await page.clock.install();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('https://fonts.googleapis.com/**', route => route.abort());
    await page.addInitScript(() => {
      window.rockbar = {
        onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {}, setMode: async () => {},
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      window.testDevices = [1, 2].map(index => ({ ID: `REC0${index}`, State: 3, CurrentService: index,
        CurrentProgram: { Sid: index, Eid: index, Start: 1000, End: 6000, Title: `番組${index}`, Subtitle: `副題${index}`, Content: `詳細${index}` },
        NextProgram: { Sid: index, Eid: index + 10, Start: 6000, End: 8000, Title: `次番組${index}` },
      }));
      updateRockbar({ type: 'status', value: { connected: true } });
      updateRockbar({ type: 'server-time', value: 2000000 });
      updateRockbar({ type: 'stations', value: testDevices.map(device => ({ Sid: device.CurrentService, Name: `局${device.CurrentService}`, Tsp: 1, View: 1 })) });
      updateRockbar({ type: 'devices', value: testDevices });
      updateRockbar({ type: 'programs', value: Object.fromEntries(testDevices.map(device => [device.CurrentService, [device.CurrentProgram]])) });
    });
    await page.clock.runFor(500);
    // Focus while the incoming title is below the clip must not scroll its viewport.
    const during = await page.evaluate(() => {
      testDevices[0].CurrentProgram.Title = '更新された番組1';
      testDevices[0].CurrentProgram.Start = 1500;
      updateRockbar({ type: 'devices', value: testDevices });
      const viewport = document.querySelector('#current-program');
      viewport.firstElementChild.getAnimations().forEach(animation => { animation.pause(); animation.currentTime = 0; });
      document.querySelector('#title').focus();
      return { scrollTop: viewport.scrollTop, scrollLeft: viewport.scrollLeft, layers: viewport.children.length };
    });
    assert.deepEqual(during, { scrollTop: 0, scrollLeft: 0, layers: 2 });
    await page.evaluate(() => currentProgramScroller.finish());
    await page.locator('#title').click();
    assert.equal(await page.locator('#program-content-text').textContent(), '詳細1');
    await page.locator('#devices button[data-device="REC02"]').click();
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    assert.equal(await page.locator('#title').textContent(), '番組2');
    assert.equal(await page.locator('#subtitle').textContent(), '副題2');
    assert.equal(await page.locator('#current-program>.program-scroll-outgoing').count(), 0);
    await page.locator('#title').hover();
    await page.clock.runFor(60000);
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), true);
    assert.equal(await page.locator('#program-content-text').textContent(), '詳細2');
    // Updates and device selection while hidden must render once, without an old outgoing title, on return.
    await page.locator('#timekeeper-toggle').click();
    const hiddenTitle = await page.locator('#title').textContent();
    await page.locator('#devices button[data-device="REC01"]').click();
    await page.evaluate(() => {
      testDevices[0].CurrentProgram = { ...testDevices[0].CurrentProgram, Title: '復帰後の最新番組', Content: '復帰後の詳細' };
      testDevices[0].NextProgram = { ...testDevices[0].NextProgram, Title: '復帰後の次番組' };
      updateRockbar({ type: 'devices', value: testDevices });
    });
    assert.equal(await page.locator('#title').textContent(), hiddenTitle);
    await page.locator('#timekeeper-toggle').click();
    assert.equal(await page.locator('#title').textContent(), '復帰後の最新番組');
    assert.match(await page.locator('#next').textContent(), /復帰後の次番組/);
    assert.equal(await page.locator('.program-scroll-outgoing').count(), 0);
    await page.locator('#title').click();
    assert.equal(await page.locator('#program-content-text').textContent(), '復帰後の詳細');
    // Repeated mode/device switches never detach or duplicate the original title fields.
    await page.evaluate(() => {
      for (let index = 0; index < 12; index++) {
        document.querySelector('#timekeeper-toggle').click();
        document.querySelector(`#devices button[data-device="REC0${index % 2 + 1}"]`).click();
        document.querySelector('#timekeeper-toggle').click();
      }
    });
    await page.clock.runFor(500);
    assert.equal(await page.locator('#title').count(), 1);
    assert.equal(await page.locator('#title').textContent(), '番組2');
    assert.equal(await page.locator('.program-scroll-outgoing').count(), 0);
    assert.equal(await page.locator('#current-program').evaluate(el => el.scrollTop), 0);
    assert.deepEqual(errors, []);
    console.log('PASS program lifecycle: focused animations, device changes, popup context and timekeeper return');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
