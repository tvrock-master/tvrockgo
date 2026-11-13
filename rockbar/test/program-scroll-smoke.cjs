const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 680, height: 320 } });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(() => {
      window.rockbar = {
        platform: 'electron', onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {}, tune: async () => ({}),
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      window.first = { Sid: 65537, Eid: 1, Start: 1000, End: 3000, Title: '旧番組', Subtitle: '旧サブタイトル' };
      window.second = { ...first, Eid: 2, Start: 3000, End: 5000, Title: '新番組', Subtitle: '新サブタイトル' };
      window.third = { ...first, Eid: 3, Start: 5000, End: 7000, Title: '次番組', Subtitle: '次サブタイトル' };
      window.device = { ID: 'T1', State: 2, CurrentService: 65537, CurrentProgram: first, NextProgram: second };
      updateRockbar({ type: 'status', value: { connected: true } });
      updateRockbar({ type: 'server-time', value: 2000000 });
      updateRockbar({ type: 'stations', value: [
        { Sid: 65537, Tsp: 1, View: 1, Name: '局A' }, { Sid: 65538, Tsp: 1, View: 1, Name: '局B' },
      ] });
      updateRockbar({ type: 'devices', value: [device] });
      updateRockbar({ type: 'programs', value: { 65537: [first, second, third], 65538: [{ ...first, Sid: 65538, Title: '局B番組' }] } });
    });
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    const height = await page.locator('#current-program').evaluate(el => el.clientHeight);
    await page.evaluate(() => {
      window.retainedRow = document.querySelector('.service-program');
      retainedRow.focus();
      device.CurrentProgram = second;
      device.NextProgram = third;
      updateRockbar({ type: 'devices', value: [device] });
      updateRockbar({ type: 'server-time', value: 3000000 });
    });
    // Sample a deterministic intermediate frame even on a busy build machine.
    await page.evaluate(() => {
      for (const animation of document.getAnimations()) {
        animation.pause();
        animation.currentTime = 120;
      }
    });
    const frames = await page.evaluate(() => {
      const viewports = ['#current-program', '#next', '.service-program>span:nth-child(2)', '.service-program>span:nth-child(3)'];
      return viewports.map(selector => {
        const viewport = document.querySelector(selector);
        const incoming = viewport.firstElementChild;
        const outgoing = viewport.lastElementChild;
        const dy = element => parseFloat(getComputedStyle(element).top) || 0;
        return { height: viewport.clientHeight, incoming: dy(incoming), outgoing: dy(outgoing),
          count: viewport.children.length, clipped: getComputedStyle(viewport).overflow === 'clip', inert: outgoing.inert };
      });
    });
    for (const frame of frames) {
      assert.equal(frame.count, 2);
      assert.ok(frame.incoming > 0 && frame.incoming < frame.height, JSON.stringify(frame));
      assert.ok(frame.outgoing < 0 && frame.outgoing > -frame.height, JSON.stringify(frame));
      assert.ok(Math.abs(frame.incoming - frame.outgoing - frame.height) < 1);
      assert.ok(frame.clipped && frame.inert);
    }
    assert.equal(await page.locator('#current-program').evaluate(el => el.clientHeight), height);
    assert.equal(await page.locator('#title').count(), 1);
    assert.equal(await page.locator('#subtitle').count(), 1);
    assert.ok(await page.evaluate(() => document.querySelector('.service-program') === retainedRow && document.activeElement === retainedRow));
    assert.ok(await page.evaluate(() => {
      const animation = document.querySelector('#current-program').firstElementChild.getAnimations()[0];
      updateRockbar({ type: 'schedules', value: [] });
      return document.querySelector('#current-program').firstElementChild.getAnimations()[0] === animation;
    }));
    await page.evaluate(() => document.getAnimations().forEach(animation => animation.play()));
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    assert.equal(await page.locator('#title').textContent(), '新番組');
    assert.equal(await page.locator('#subtitle').textContent(), '新サブタイトル');
    assert.equal(await page.locator('.service-program').first().locator('span').nth(2).textContent(), '新番組');
    // Selection metadata arrives ahead of the program; identical content must stay still.
    const metadataLayers = await page.evaluate(() => {
      device.CurrentService = 65538;
      for (let n = 0; n < 4; n++) {
        device.CurrentProgram = { ...second, Sid: 65538, Eid: 20 + n };
        device.NextProgram = { ...third, Sid: 65538, Eid: 30 + n };
        updateRockbar({ type: 'devices', value: [device] });
      }
      return ['current-program', 'next'].map(id => document.querySelectorAll(`#${id}>.program-scroll-outgoing`).length);
    });
    assert.deepEqual(metadataLayers, [0, 0]);
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    // Late title/subtitle details for the same broadcast render in place.
    const selectionLayers = await page.evaluate(() => {
      device.CurrentProgram = { ...second, Sid: 65538, Title: '局B番組' };
      device.NextProgram = { ...third, Sid: 65538, Title: '局B次番組' };
      updateRockbar({ type: 'devices', value: [device] });
      return ['current-program', 'service', 'next'].map(id => document.querySelectorAll(`#${id}>.program-scroll-outgoing`).length);
    });
    assert.deepEqual(selectionLayers, [0, 0, 0]);
    assert.ok(await page.evaluate(() => {
      const content = document.querySelector('#current-program').firstElementChild;
      const next = document.querySelector('#next').firstElementChild;
      const animations = [content.getAnimations()[0], next.getAnimations()[0]];
      for (let n = 0; n < 4; n++) {
        device.CurrentProgram.Eid++;
        device.NextProgram.Eid++;
        updateRockbar({ type: 'devices', value: [device] });
      }
      return content.getAnimations()[0] === animations[0] && next.getAnimations()[0] === animations[1];
    }));
    // Rapid switches never leave stale layers or text after completion.
    await page.evaluate(() => {
      for (let n = 0; n < 8; n++) {
        device.CurrentProgram = { ...second, Eid: n + 10, Title: '連続' + n };
        updateRockbar({ type: 'devices', value: [device] });
      }
    });
    assert.equal(await page.locator('#current-program>.program-scroll-outgoing').count(), 0);
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    assert.equal(await page.locator('#title').textContent(), '連続7');
    // A corrected end time updates the display without another scroll.
    assert.equal(await page.evaluate(() => {
      device.CurrentProgram.End++;
      updateRockbar({ type: 'devices', value: [device] });
      return document.querySelectorAll('#current-program>.program-scroll-outgoing').length;
    }), 0);
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    await page.emulateMedia({ reducedMotion: 'reduce' });
    await page.evaluate(() => {
      device.CurrentProgram = first;
      updateRockbar({ type: 'devices', value: [device] });
    });
    assert.equal(await page.locator('.program-scroll-outgoing').count(), 0);
    assert.equal(await page.locator('#title').textContent(), '旧番組');
    await page.setViewportSize({ width: 390, height: 240 });
    assert.ok(await page.locator('#subtitle').isVisible());
    assert.ok(await page.locator('#next').isVisible());
    await page.emulateMedia({ reducedMotion: 'no-preference' });
    await page.evaluate(() => {
      device.CurrentProgram = { ...second, Title: '長いタイトル'.repeat(50), Subtitle: '長いサブタイトル'.repeat(50) };
      updateRockbar({ type: 'devices', value: [device] });
    });
    await page.setViewportSize({ width: 410, height: 250 });
    await page.waitForFunction(() => !document.querySelector('.program-scroll-outgoing'));
    assert.ok(await page.locator('#current-program').evaluate(el => el.scrollHeight === el.clientHeight));
    assert.deepEqual(errors, []);
    console.log('PASS program scroll: rollover, staged selection updates, metadata deduplication, time changes, clipped simultaneous motion, stable rows/focus, rapid updates, reduced motion and resize');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
