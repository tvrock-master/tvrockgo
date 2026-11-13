const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 400 } });
    await page.clock.install();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('https://fonts.googleapis.com/**', route => route.abort());
    await page.addInitScript(() => {
      window.rockbar = { onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {}, setMode: async () => {},
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      window.testDevices = [1, 2].map(index => ({ ID: `REC0${index}`, State: 3, CurrentService: index,
        CurrentProgram: { Sid: index, Eid: index * 10, Title: `録画番組${index}`, Start: 1000, End: 2800 },
      }));
      window.testReservations = testDevices.map((device, index) => ({ id: index + 1,
        deviceid: testDevices[1 - index].ID, sid: device.CurrentService, eid: device.CurrentProgram.Eid,
        title: device.CurrentProgram.Title, validate: true, starttime: 1000, endtime: 2800, readytime: 30,
      }));
      updateRockbar({ type: 'status', value: { connected: true } });
      updateRockbar({ type: 'server-time', value: 1710000 });
      updateRockbar({ type: 'devices', value: testDevices });
      updateRockbar({ type: 'schedules', value: testReservations });
    });
    await page.locator('#timekeeper-toggle').click();
    const elapsed = index => page.locator(`#devices button[data-device="REC0${index}"] .tk-cell`).nth(3);
    assert.equal(await elapsed(1).textContent(), '--:--:-- / --:--:--', 'reproduces the missing SQL assignment before running IDs arrive');
    await page.evaluate(() => updateRockbar({ type: 'recording-status', value: { recording: [1, 2] } }));
    for (const index of [1, 2]) assert.equal(await elapsed(index).textContent(), '00:12:20 / 00:30:30');
    await page.clock.runFor(2100);
    const beforeError = [];
    for (const index of [1, 2]) {
      const text = await elapsed(index).textContent();
      assert.notEqual(text, '00:12:20 / 00:30:30');
      assert.match(text, /^00:12:2\d \/ 00:30:30$/);
      beforeError.push(text);
    }
    await page.evaluate(() => {
      updateRockbar({ type: 'recording-status', value: { recording: [1, 2], current: {
        REC01: testReservations[0], REC02: testReservations[1],
      } } });
      updateRockbar({ type: 'schedules', value: [] });
      updateRockbar({ type: 'schedule-error', value: '予約一覧を取得できません' });
    });
    await page.clock.runFor(2100);
    for (const index of [1, 2]) {
      const text = await elapsed(index).textContent();
      assert.notEqual(text, beforeError[index - 1]);
      assert.match(text, /^00:12:2\d \/ 00:30:30$/);
    }
    // Simulate extended operation/suspension while the device still reports recording.
    await page.clock.fastForward(3 * 3600000);
    assert.match(await elapsed(1).textContent(), /^03:12:2\d \/ 00:30:30$/);
    assert.equal(await page.locator('#devices button').first().locator('.tk-cell').nth(4).textContent(), '00:00:00');
    await page.evaluate(() => { testDevices[0].State = 0; updateRockbar({ type: 'devices', value: testDevices }); });
    assert.equal(await elapsed(1).textContent(), '--:--:-- / --:--:--');
    await page.evaluate(() => {
      updateRockbar({ type: 'recording-status', value: { recording: [], current: {} } });
      updateRockbar({ type: 'schedules', value: [
        { id: 10, deviceid: 'REC01', validate: true, title: '待機する番組', sid: 1,
          starttime: 20000, endtime: 21800, readytime: 30, idletime: 60 },
        { id: 11, deviceid: 'REC01', validate: true, title: 'その次の番組', sid: 1,
          starttime: 23000, endtime: 24800, readytime: 30, idletime: 60 },
      ] });
      updateRockbar({ type: 'server-time', value: 19939000 });
    });
    assert.equal(await page.locator('#devices button').first().locator('.tk-cell').nth(5).textContent(), '待機する番組');
    await page.evaluate(() => updateRockbar({ type: 'server-time', value: 19940000 }));
    assert.equal(await elapsed(1).textContent(), '-00:00:30 / 00:30:30');
    assert.equal(await page.locator('.tk-transfer').textContent(), '待機する番組');
    await page.clock.runFor(1100);
    await page.locator('.tk-transfer').waitFor({ state: 'detached', timeout: 5000 });
    assert.equal(await page.locator('.tk-transfer').count(), 0);
    assert.equal(await page.locator('#devices button').first().locator('.tk-cell').nth(5).textContent(), 'その次の番組');
    await page.evaluate(() => {
      testDevices[0].State = 2;
      updateRockbar({ type: 'devices', value: testDevices });
      updateRockbar({ type: 'schedules', value: [] });
      updateRockbar({ type: 'recording-status', value: { recording: [12], current: {
        REC01: { id: 12, deviceid: 'REC01', watchonly: true, starttime: 19000, endtime: 22000 },
      } } });
    });
    assert.equal(await page.locator('#stop').isEnabled(), true);
    assert.equal(await page.locator('#stop').getAttribute('aria-label'), '視聴予約を停止');
    await page.evaluate(() => updateRockbar({ type: 'recording-status', value: { recording: [], current: {} } }));
    assert.equal(await page.locator('#stop').isDisabled(), true);
    assert.equal(await page.locator('#devices button').first().locator('.tk-cell').nth(0).evaluate(el => el.style.opacity), '');
    await page.locator('#timekeeper-toggle').click();
    assert.equal(await page.locator('.tk-transfer').count(), 0);
    assert.deepEqual(errors, []);
    console.log('PASS timekeeper clock: swapped assignments, legacy running IDs, schedule loss/error, 3-hour operation and actual stop');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
