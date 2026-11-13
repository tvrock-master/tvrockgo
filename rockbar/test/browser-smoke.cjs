const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const assert = require('node:assert/strict');
const { WebSocketServer } = require('ws');
const { buildWeb } = require('../scripts/build-web.cjs');

(async () => {
  const output = path.resolve(__dirname, '../test-output/browser-site');
  buildWeb(output);
  const now = Math.floor((Date.now() + 7200000) / 1000);
  const stations = [{ Sid: 65537, Tsp: 1, View: 1, No: 1, Name: '星見野テレビ' },
    { Sid: 65538, Tsp: 1, View: 1, No: 2, Name: '星見野テレビ・空', Parent: 65537 }, { Sid: 131073, Tsp: 2, View: 1, No: 1, Name: 'BSテスト' },
    { Sid: 131074, Tsp: 2, View: 1, No: 2, Name: 'BSサブ', Parent: 131073 }, { Sid: 196941, Tsp: 3, View: 1, No: 1, Name: 'AT-X' }];
  const device = { ID: 'チューナー1', RegistryCode: 'r1', State: 2, CurrentService: 65537,
    CurrentProgram: { Sid: 65537, Title: '現在のニュース', Start: now - 300, End: now + 1200, Eid: 1 } };
  const satellite = { ...device, ID: '衛星デバイス', RegistryCode: 'r2', TunerType: 1, CurrentService: 131073, CurrentProgram: { ...device.CurrentProgram, Sid: 131073 } };
  const messages = [], requests = [], origins = [];
  let connections = 0, captionRequests = 0;
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url, 'http://localhost');
    res.setHeader('Date', new Date(Date.now() + 7200000).toUTCString());
    if (url.pathname === '/login') {
      if (req.method === 'POST') {
        res.writeHead(302, { 'Set-Cookie': 'mockSession=ok; Path=/; HttpOnly; SameSite=Lax', Location: '/static/index.html' });
        res.end();
      } else {
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        res.end('<form method="post"><button>テストログイン</button></form>');
      }
      return;
    }
    if (!req.headers.cookie?.includes('mockSession=ok')) { res.writeHead(302, { Location: '/login' }); res.end(); return; }
    if (url.pathname === '/static/index.html') { res.end('login complete'); return; }
    if (url.pathname.startsWith('/static/rockbar/')) {
      const relative = url.pathname.slice('/static/rockbar/'.length) || 'index.html';
      const filename = path.resolve(output, relative);
      if (!filename.startsWith(output + path.sep) || !fs.existsSync(filename)) { res.writeHead(404); res.end(); return; }
      const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.svg': 'image/svg+xml' };
      res.setHeader('Content-Type', (types[path.extname(filename)] || 'application/octet-stream') + '; charset=utf-8');
      res.end(fs.readFileSync(filename)); return;
    }
    res.setHeader('Content-Type', 'application/json');
    if (url.pathname === '/dvs') res.end(JSON.stringify([device, satellite]));
    else if (url.pathname === '/stl') res.end(JSON.stringify(stations));
    else if (url.pathname === '/caption') { captionRequests++; res.end(JSON.stringify({items:[1,2,3].map(ID=>({ID,Text:'履歴'+ID,Pts:ID,Lang:0})),snapshot:3,hasMore:false})); }
    else if (url.pathname === '/tvp') {
      let body = ''; for await (const chunk of req) body += chunk;
      requests.push(JSON.parse(body));
      res.end(JSON.stringify({ CurrentMilliseconds: Date.now() + 7200000, Tvp: Object.fromEntries(stations.map(s => [s.Sid, [{ ...device.CurrentProgram, Sid: s.Sid, Title: s.Name + 'の番組' }]])) }));
    } else { res.writeHead(404); res.end('{}'); }
  });
  const wss = new WebSocketServer({ server, path: '/con' });
  wss.on('connection', (socket, req) => {
    connections++;
    origins.push(req.headers.origin);
    socket.on('message', raw => {
      const message = raw.toString(); messages.push(message);
      if (message.includes('StreamChange') && JSON.parse(message.slice(4)).message === '196941') { satellite.CurrentService = 196941; satellite.CurrentProgram = { ...satellite.CurrentProgram, Sid: 196941 }; socket.send('dev:' + JSON.stringify(satellite)); }
      if (message.includes('RecordStart')) { device.State = 3; socket.send('dev:' + JSON.stringify(device)); }
    });
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  let browser;
  try {
    browser = await chromium.launch({ channel: 'msedge', headless: true });
    const context = await browser.newContext({ viewport: { width: 680, height: 240 } });
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    page.on('console', message => { if (message.type() === 'error' && message.text().includes('Content Security Policy')) errors.push(message.text()); });
    await page.goto(base + '/static/rockbar/');
    await page.getByText('テストログイン').click();
    await page.waitForURL('**/static/index.html');
    await page.goto(base + '/static/rockbar/');
    await page.locator('#connection.connected').waitFor();
    await page.getByText('星見野テレビの番組', { exact: true }).waitFor();
    assert.equal(await page.locator('.service-program').count(), 1);
    assert.equal(await page.locator('#multi').isVisible(), true);
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'false');
    const receive = (target, sid) => {
      target.CurrentService = sid;
      for (const socket of wss.clients) socket.send('dev:' + JSON.stringify(target));
    };
    receive(device, 65538);
    await page.waitForFunction(() => document.querySelector('#multi').getAttribute('aria-pressed') === 'true');
    assert.equal(await page.locator('.service-program').count(), 2);
    assert.equal(await page.locator('[data-sid="65538"]').getAttribute('aria-current'), 'true');
    await page.locator('#multi').click();
    receive(device, 65538);
    await page.waitForTimeout(150);
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'false');
    receive(device, 65537);
    await page.waitForFunction(() => document.querySelector('[data-sid="65537"]').getAttribute('aria-current') === 'true');
    await page.locator('#multi').click();
    await page.getByText('星見野テレビ・空の番組', { exact: true }).waitFor();
    assert.equal(await page.locator('.service-program').count(), 2);
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'true');
    await page.locator('#multi').click();
    assert.equal(await page.locator('.service-program').count(), 1);
    await page.locator('#multi').click();
    assert.equal(messages.filter(s=>s.includes('StreamChange')).length, 0);
    assert.equal(await page.locator('#pin').isVisible(), false);
    assert.equal(await page.locator('#close').isVisible(), false);
    assert.equal(await page.locator('#server').getAttribute('readonly'), '');
    assert.equal(origins[0], base);
    assert.deepEqual(requests[0].services, [65537, 65538, 131073, 131074, 196941]);
    assert.equal((await page.evaluate(() => window.rockbar.connect('https://example.com'))).ok, undefined);
    await page.waitForFunction(() => document.querySelector('#clock').textContent !== '--:--:--');
    const clockCorrect = await page.evaluate(() => {
      const [h,m,sec] = document.querySelector('#clock').textContent.split(':').map(Number);
      const expected = new Date(Date.now()+7200000);
      return Math.abs((h*3600+m*60+sec)-(expected.getHours()*3600+expected.getMinutes()*60+expected.getSeconds())) <= 2;
    });
    assert.ok(clockCorrect);
    assert.equal(captionRequests,0);
    assert.equal(await page.locator('#caption-lines').innerText(),'');
    const cap = (HistoryID,Text,extra={}) => 'cap:'+JSON.stringify({DeviceID:device.ID,Sid:65537,Eid:1,Start:device.CurrentProgram.Start,HistoryID,Text,Pts:99,Lang:0,...extra});
    for(const socket of wss.clients) {socket.send(cap(4,'最新',{Color:3,BgColor:1}));socket.send(cap(5,'の字幕',{Color:6,BgColor:2}));socket.send(cap(6,'',{Clear:true}));socket.send(cap(7,'次の字幕',{Color:7}));}
    await page.waitForFunction(()=>document.querySelector('#caption-lines').innerText==='最新の字幕\n次の字幕');
    assert.equal(await page.locator('#caption-lines').innerText(),'最新の字幕\n次の字幕');
    const inks=page.locator('#caption-lines>div').first().locator('span');
    assert.equal(await inks.count(),2);
    assert.notEqual(await inks.nth(0).evaluate(el=>getComputedStyle(el).color),await inks.nth(1).evaluate(el=>getComputedStyle(el).color));
    assert.equal(await inks.nth(0).evaluate(el=>getComputedStyle(el).backgroundColor),'rgba(0, 0, 0, 0)');
    assert.notEqual(await inks.nth(0).evaluate(el=>getComputedStyle(el).textShadow),'none');
    await page.locator('#clock').hover();
    await page.locator('#caption-history').waitFor({state:'hidden'});
    await page.locator('#captions').click();
    await page.locator('#caption-history').waitFor({state:'visible'});
    assert.equal(await page.locator('#caption-history-lines>div').count(),2);
    await page.locator('#caption-history-lines').hover();
    await page.waitForTimeout(200);
    assert.ok(await page.locator('#caption-history').isVisible());
    for(const socket of wss.clients) for(let n=8;n<=19;n++) socket.send(cap(n,'追加字幕'+n,{Pts:n,Color:n%8}));
    await page.waitForFunction(()=>document.querySelector('#caption-history-lines').lastElementChild?.textContent==='追加字幕19');
    // Two buffered statements plus twelve additions fit within the 50-item history.
    assert.equal(await page.locator('#caption-history-lines>div').count(),14);
    await page.waitForFunction(()=>document.querySelector('#caption-lines').children.length===2);
    assert.equal(await page.locator('#caption-lines').innerText(),'追加字幕18\n追加字幕19');
    await page.screenshot({path:path.resolve(__dirname,'../test-output/caption-history.png')});
    await page.locator('#clock').click();
    await page.locator('#caption-history').waitFor({state:'hidden'});
    await page.locator('#devices button').nth(1).click();
    assert.ok((await page.locator('#wave').textContent()).includes('BS'));
    assert.equal(await page.locator('#multi').isVisible(), true);
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'false');
    const tuneCount = messages.filter(s=>s.includes('StreamChange')).length;
    await page.locator('#wave').click();
    assert.ok((await page.locator('#wave').textContent()).includes('CS'));
    assert.equal(await page.locator('#multi').isVisible(), false);
    assert.equal(messages.filter(s=>s.includes('StreamChange')).length,tuneCount);
    await page.locator('.service-program').first().dblclick();
    await page.waitForFunction(()=>document.querySelector('#ticker').textContent.includes('AT-X (333)'));
    await page.waitForFunction(()=>!document.querySelector('#record').disabled);
    // A received BS child must override the manually selected CS list.
    receive(satellite, 131074);
    await page.getByText('BSサブの番組', { exact: true }).waitFor();
    assert.ok((await page.locator('#wave').textContent()).includes('BS'));
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'true');
    assert.equal(await page.locator('[data-sid="131074"]').getAttribute('aria-current'), 'true');
    receive(satellite, 196941);
    await page.locator('#wave').click();
    assert.equal(await page.locator('#multi').isVisible(), false);
    await page.locator('#devices button').first().click();
    assert.equal(await page.locator('#multi').getAttribute('aria-pressed'), 'true');
    const powerCount=messages.filter(s=>s.startsWith('BDV:')).length;
    await page.locator('#devices button').first().dblclick();
    await page.waitForFunction(()=>!document.querySelector('#power').disabled);
    assert.equal(messages.filter(s=>s.startsWith('BDV:')).length,powerCount+1);
    assert.equal(JSON.parse(messages.find(s=>s.startsWith('BDV:')).slice(4)).boottype,0);
    // Periodic REL polling may arrive while checking the click's effect.
    const commands = () => messages.filter(s => /^(BDV:|COM:|REQ:)/.test(s));
    const before = commands();
    await page.locator('.service-program').nth(1).click();
    await page.waitForTimeout(150);
    assert.deepEqual(commands(), before);
    await page.locator('.service-program').nth(1).dblclick();
    await page.waitForFunction(() => !document.querySelector('#record').disabled);
    assert.deepEqual(messages.filter(s => s.includes('StreamChange')).map(s => JSON.parse(s.slice(4)).message), ['196941', '524290', '65537']);
    await page.locator('#capture').click();
    await page.waitForFunction(() => !document.querySelector('#record').disabled);
    assert.ok(messages.some(s => s.startsWith('REQ:') && s.includes('Capture')));
    await page.locator('#record').click();
    await page.waitForFunction(() => document.querySelector('#state').textContent === 'REC');
    assert.equal(await page.locator('.service-program').first().isDisabled(), true);
    assert.equal(await page.locator('#state').evaluate(el=>getComputedStyle(el).color),'rgb(255, 91, 112)');
    const powerBefore=messages.filter(s=>s.startsWith('BDV:')).length;
    await page.locator('#devices button').first().dblclick();
    assert.equal(messages.filter(s=>s.startsWith('BDV:')).length,powerBefore);
    await page.waitForTimeout(4700);
    await page.screenshot({ path: path.resolve(__dirname, '../test-output/browser-normal.png') });
    await page.setViewportSize({ width: 370, height: 150 });
    assert.ok(await page.locator('#programs').evaluate(el => el.clientHeight) >= 26);
    await page.screenshot({ path: path.resolve(__dirname, '../test-output/browser-small.png') });
    for (const socket of wss.clients) socket.terminate();
    await page.waitForFunction(() => !document.querySelector('#connection').classList.contains('connected'));
    await page.locator('#connection.connected').waitFor();
    assert.ok(connections >= 2);
    await context.clearCookies();
    await page.evaluate(base => window.rockbar.connect(base), base);
    await page.locator('#programs').filter({ hasText: 'ログインが必要' }).waitFor();
    await page.setViewportSize({ width: 680, height: 300 });
    await page.locator('#settings').click();
    const popupPromise = context.waitForEvent('page');
    await page.locator('#login').click();
    const popup = await popupPromise;
    await popup.getByText('テストログイン').click();
    await popup.waitForURL('**/static/index.html');
    await popup.close();
    await page.bringToFront();
    await page.evaluate(() => window.dispatchEvent(new Event('focus')));
    await page.locator('#connection.connected').waitFor();
    assert.deepEqual(errors, []);
    console.log('PASS browser: login cookies, HTTP/WS, program list, tuning, recording guard, reconnect, re-login, desktop buttons hidden, small layout');
  } finally {
    if (browser) await browser.close();
    for (const socket of wss.clients) socket.terminate();
    await new Promise(resolve => wss.close(resolve));
    await new Promise(resolve => server.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });

