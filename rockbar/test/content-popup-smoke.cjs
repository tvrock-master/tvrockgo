const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const assert = require('node:assert/strict');
const fs = require('node:fs');

(async () => {
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 680, height: 320 }, colorScheme: 'light' });
    await page.clock.install();
    const output = path.resolve(__dirname, '../test-output');
    fs.mkdirSync(output, { recursive: true });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.addInitScript(() => {
      window.rockbar = {
        platform: 'electron', onUpdate: callback => { window.updateRockbar = callback; },
        settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
        watchDevice: async () => {},
        tune: async (...args) => { (window.tuneRequests ||= []).push(args); return {}; },
      };
    });
    await page.goto(pathToFileURL(path.resolve(__dirname, '../src/index.html')).href);
    await page.evaluate(() => {
      window.popupProgram = { Sid: 65537, Eid: 1, Start: 1000, End: 4600,
        Title: '現在の番組', Subtitle: 'サブタイトル', Content: '番組詳細\n2行目 <script>文字列</script>\n公式 https://example.com/info?q=1&lang=ja#details\n関連 http://example.com/next\njavascript:alert(1)' };
      window.popupDevice = { ID: 'T1', State: 2, Hybrid: true, CurrentService: 65537, CurrentProgram: popupProgram };
      updateRockbar({ type: 'status', value: { connected: true, message: '接続済み' } });
      updateRockbar({ type: 'server-time', value: 2000000 });
      updateRockbar({ type: 'stations', value: [{ Sid: 65537, Tsp: 1, View: 1, Name: 'テスト放送' }, { Sid: 65538, Tsp: 1, View: 1, Name: '別局' }, { Sid: 65539, Tsp: 1, View: 1, Name: '第三局' }] });
      updateRockbar({ type: 'devices', value: [popupDevice] });
      updateRockbar({ type: 'programs', value: { 65537: [{ ...popupProgram, Content: '' }],
        65538: [{ ...popupProgram, Sid: 65538, Title: '別局の番組', Subtitle: '別局の副題 <script>文字列</script>', Content: '別局の詳細' }] } });
    });
    assert.equal(await page.locator('#wave').textContent(), '地上波 放送中の番組 ⇄');
    await page.locator('#wave').click();
    assert.equal(await page.locator('#wave').textContent(), 'ワンセグ 放送中の番組 ⇄');
    await page.locator('#wave').click();
    await page.locator('#title').hover();
    await page.clock.fastForward(59000);
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    await page.clock.fastForward(1000);
    await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
    assert.equal(await page.locator('#program-content-text').textContent(), await page.evaluate(() => popupProgram.Content));
    assert.equal(await page.locator('#program-content-subtitle').textContent(), 'サブタイトル');
    assert.equal(await page.locator('#program-content script').count(), 0);
    const search = page.locator('#program-content-search');
    const ai = page.locator('#program-content-ai');
    assert.equal(await search.textContent(), '現在の番組');
    await search.focus();
    await page.evaluate(() => updateRockbar({ type: 'devices', value: [popupDevice] }));
    assert.equal(await search.evaluate(el => el === document.activeElement), true);
    await page.context().route('https://google.com/**', route => route.fulfill({ body: 'Search results' }));
    async function checkSearchLinks(title) {
      for (const [link, mode] of [[search, 14], [ai, 50]]) {
        const url = `https://google.com/search?q=${encodeURIComponent(title)}&udm=${mode}`;
        assert.equal(await link.getAttribute('href'), url);
        assert.equal(await link.getAttribute('rel'), 'noopener noreferrer');
        const opened = page.waitForEvent('popup');
        await link.click();
        const external = await opened;
        await external.waitForLoadState();
        assert.equal(external.url(), url);
        await external.close();
      }
    }
    await checkSearchLinks('現在の番組');
    const links = page.locator('#program-content-text a');
    assert.equal(await links.count(), 2);
    assert.equal(await links.first().getAttribute('href'), 'https://example.com/info?q=1&lang=ja#details');
    assert.equal(await links.first().getAttribute('rel'), 'noopener noreferrer');
    await links.first().focus();
    await page.evaluate(() => updateRockbar({ type: 'devices', value: [popupDevice] }));
    assert.equal(await links.first().evaluate(el => el === document.activeElement), true);
    // Intercept the destination so the smoke test never visits an external server.
    await page.context().route('https://example.com/**', route => route.fulfill({ body: 'Program website' }));
    const opened = page.waitForEvent('popup');
    await links.first().click();
    const externalPage = await opened;
    await externalPage.waitForLoadState();
    assert.equal(externalPage.url(), 'https://example.com/info?q=1&lang=ja#details');
    await externalPage.close();
    await page.keyboard.press('Escape');
    const row = page.locator('.service-program[data-sid="65538"]');
    assert.equal(await row.getAttribute('title'), null);
    for (const colorScheme of ['light', 'dark']) {
      await page.mouse.move(670, 310);
      await page.emulateMedia({ colorScheme });
      await page.locator('#title').click();
      await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
      const currentBackground = await page.locator('#program-content').evaluate(el => getComputedStyle(el).backgroundImage);
      await page.keyboard.press('Escape');
      await row.hover();
      await page.clock.fastForward(59000);
      assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
      await page.clock.fastForward(1000);
      await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
      assert.equal(await search.textContent(), '別局の番組');
      await checkSearchLinks('別局の番組');
      assert.equal(await page.locator('#program-content-subtitle').textContent(), '別局の副題 <script>文字列</script>');
      assert.equal(await page.locator('#program-content script').count(), 0);
      assert.equal(await page.locator('#program-content-text').textContent(), '別局の詳細');
      assert.equal(await page.locator('#program-content').evaluate(el => getComputedStyle(el).backgroundImage), currentBackground);
      const rowBounds = await row.boundingBox();
      const popupBounds = await page.locator('#program-content').boundingBox();
      assert.ok(popupBounds.y + popupBounds.height <= rowBounds.y - 23 || popupBounds.y >= rowBounds.y + rowBounds.height + 23, 'popup leaves space above or below the station row for moving between stations');
      await page.evaluate(() => updateRockbar({ type: 'devices', value: [popupDevice] }));
      assert.equal(await search.textContent(), '別局の番組');
      await page.screenshot({ path: path.join(output, `station-popup-${colorScheme}.png`) });
      await row.click();
      assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false, 'clicking the hovered station dismisses its popup');
      await row.focus();
      await page.keyboard.press('Escape');
    }
    await page.evaluate(() => updateRockbar({ type: 'programs', value: {
      65537: [popupProgram], 65538: [{ ...popupProgram, Sid: 65538, Title: '固定して読む番組', Content: Array.from({ length: 100 }, (_, n) => `詳細の${n + 1}行目`).join('\n') }],
    } }));
    await row.click();
    assert.equal(await page.locator('#program-content-hint').isVisible(), true);
    assert.deepEqual(await page.evaluate(() => window.tuneRequests || []), []);
    await page.locator('.service-program[data-sid="65539"]').hover();
    await page.waitForTimeout(550);
    assert.equal(await search.textContent(), '固定して読む番組');
    await page.locator('#program-content').hover();
    await page.mouse.wheel(0, 500);
    await page.waitForFunction(() => document.querySelector('#program-content').scrollTop > 0);
    await row.click();
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false, 'clicking the pinned station dismisses its popup');
    await row.click();
    assert.equal(await page.locator('#program-content-hint').isVisible(), true, 'click opens immediately when closed');
    await page.locator('.service-program[data-sid="65539"]').click();
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false, 'another station click dismisses without reopening');
    await page.waitForTimeout(3200);
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false, 'outside click also cancels delayed opening');
    await row.click();
    await page.locator('.list-heading').click();
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    await row.click();
    await page.keyboard.press('Escape');
    assert.equal(await page.locator('#program-content-hint').isVisible(), false);
    await row.click();
    await row.dblclick();
    assert.deepEqual(await page.evaluate(() => window.tuneRequests), [['T1', 65538]]);
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    await page.waitForFunction(() => !document.querySelector('.service-program[data-sid="65538"]').disabled);
    await row.click();
    await page.keyboard.press('Enter');
    assert.deepEqual(await page.evaluate(() => window.tuneRequests), [['T1', 65538], ['T1', 65538]]);
    await page.waitForFunction(() => !document.querySelector('.service-program[data-sid="65538"]').disabled);
    await page.mouse.move(670, 310);
    await page.evaluate(() => document.activeElement.blur());
    await row.click();
    await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
    await page.evaluate(() => updateRockbar({ type: 'stations', value: [{ Sid: 65537, Tsp: 1, View: 1, Name: 'テスト放送' }] }));
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    await page.mouse.move(670, 310);
    await page.locator('#subtitle').click();
    await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
    await page.locator('#program-content-text').hover();
    await page.waitForTimeout(300);
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), true);
    await page.mouse.move(670, 310);
    await page.locator('.list-heading').click();
    await page.waitForFunction(() => !document.querySelector('#program-content').matches(':popover-open'));
    await page.evaluate(() => {
      popupDevice.CurrentProgram = { ...popupProgram, Eid: 2, Title: '次の番組', Subtitle: '', Content: '' };
      updateRockbar({ type: 'devices', value: [popupDevice] });
    });
    await page.locator('#title').click();
    await page.waitForFunction(() => document.querySelector('#program-content').matches(':popover-open'));
    assert.equal(await page.locator('#program-content-text').textContent(), '番組の詳細情報はありません。');
    assert.equal(await page.locator('#program-content-subtitle').isVisible(), false);
    await page.evaluate(() => {
      popupDevice.State = 0;
      updateRockbar({ type: 'devices', value: [popupDevice] });
    });
    assert.equal(await page.locator('#program-content').evaluate(el => el.matches(':popover-open')), false);
    await page.mouse.move(670, 310);
    await page.screenshot({ path: path.join(output, 'sections-open.png') });
    for (const id of ['next', 'schedule', 'captions']) {
      const section = page.locator(`#${id}-section`);
      assert.equal(await section.evaluate(el => el.open), true);
      await section.locator('summary').click();
      await page.waitForFunction(id => localStorage.getItem(`rockbar:${id}-section:open`) === 'false', id);
      assert.equal(await section.locator('div').first().isVisible(), false);
      assert.equal(await section.locator('summary').isVisible(), true);
    }
    await page.setViewportSize({ width: 680, height: 240 });
    await page.screenshot({ path: path.join(output, 'sections-closed.png') });
    const guideBounds = await page.locator('.guide-heading').boundingBox();
    assert.ok(guideBounds.y + guideBounds.height < 200);
    await page.setViewportSize({ width: 370, height: 150 });
    await page.screenshot({ path: path.join(output, 'sections-small.png') });
    assert.ok(await page.locator('.program').evaluate(el => el.scrollWidth <= el.clientWidth));
    await page.setViewportSize({ width: 680, height: 320 });
    await page.reload();
    for (const id of ['next', 'schedule', 'captions']) {
      assert.equal(await page.locator(`#${id}-section`).evaluate(el => el.open), false);
      await page.locator(`#${id}-section summary`).focus();
      await page.keyboard.press('Enter');
      await page.waitForFunction(id => document.getElementById(`${id}-section`).open, id);
    }
    await page.locator('#captions').hover();
    await page.clock.fastForward(59000);
    assert.equal(await page.locator('#caption-history').isVisible(), false);
    await page.clock.fastForward(1000);
    await page.waitForFunction(() => document.querySelector('#caption-history').matches(':popover-open'));
    await page.locator('#captions').click();
    assert.equal(await page.locator('#caption-history').isVisible(), false);
    await page.locator('#captions').click();
    assert.equal(await page.locator('#caption-history').isVisible(), true);
    await page.locator('.list-heading').hover();
    await page.waitForTimeout(300);
    assert.equal(await page.locator('#caption-history').isVisible(), true, 'click-opened captions stay open when the pointer leaves');
    await page.locator('.list-heading').click();
    assert.equal(await page.locator('#caption-history').isVisible(), false);
    await page.locator('#captions').click();
    await page.locator('#captions-section summary').click();
    await page.waitForFunction(() => !document.querySelector('#caption-history').matches(':popover-open'));
    assert.deepEqual(errors, []);
    console.log('Program popup links, text escaping, focus retention, folding, keyboard controls, persistence and caption popup cleanup: OK');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
