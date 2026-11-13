// Offline documentation capture. No production server, configuration or database is used.
// Build first: npm run build --prefix app -- --dest ../.tmp/manual-preview
// Run: node scripts/generate-manual-images.cjs
// Install Playwright separately, or set PLAYWRIGHT_PATH to its module directory.
const fs = require('node:fs');
const path = require('node:path');
const http = require('node:http');
const { pathToFileURL } = require('node:url');
const { chromium } = require(process.env.PLAYWRIGHT_PATH || 'playwright');
const { WebSocketServer } = require('../rockbar/node_modules/ws');
const root = path.resolve(__dirname, '..');
const base = path.join(root, '.tmp/manual-preview');
const out = path.join(root, 'docs/images/manual');
const epoch = new Date('2026-11-06T20:15:00+09:00').getTime();
const hour = epoch / 1000 - 900;
const names = ['星見野テレビ', '風待ち放送', '雲庭チャンネル', '灯台テレビ'];
const titles = ['星あかり工房の一日', '雲の郵便局', '小さな庭の観察帳', '夜空の実験室'];
const stations = Object.fromEntries([...names.map((Name, i) => ({ Sid: i + 1, Name, Tsp: 1, Parent: 0, View: 1 })),
  { Sid: 5, Name: '星見野テレビ・空', Tsp: 1, Parent: 1, View: 1 },
  { Sid: 6, Name: '風待ち放送・庭', Tsp: 1, Parent: 2, View: 1 },
  { Sid: 101, Name: 'BS星の港', Tsp: 2, Parent: 0, View: 1 },
  { Sid: 102, Name: 'BS星の港・風', Tsp: 2, Parent: 101, View: 1 },
  { Sid: 201, Name: 'CS雲の広場', Tsp: 3, Parent: 0, View: 1 },
  { Sid: 301, Name: '星見野ワンセグ', Tsp: 4, Parent: 0, View: 1 }].map(s => [s.Sid, s]));
const program = (sid, start, n = 0) => ({ Sid: sid, Eid: 100 + Math.round((start - hour) / 1800), Start: start, End: start + 1800,
  Title: titles[((Math.round((start - hour) / 1800) + sid - 1) % 4 + 4) % 4], Subtitle: '第1話　風の地図をつくろう',
  Content: '架空の町・星見野で、工房の仲間が空を観察します。\n手作りの道具で、新しい発見を探しましょう。', Cnib_a: (sid + n) % 8, Cnib_b: 0 });
const schedules = [0, 1, 2].map((n) => ({ id: n + 1, sid: n + 1, eid: 100, validate: true, complete: false,
  deviceid: `REC0${n + 1}`, title: titles[n], starttime: hour + n * 3600, endtime: hour + n * 3600 + 1800,
  searchid: n === 1 ? 1 : 0, idletime: 60, readytime: 30, tailtime: 0, recordonly: true, watchonly: false, shutdown: true, epgfollow: true }));
const devices = [1, 2, 3, 4].map((n) => ({ ID: `REC0${n}`, RegistryCode: `manual-${n}`, State: n === 1 ? 3 : n === 2 ? 2 : 0,
  TunerType: 0, Hybrid: true, CurrentService: n, CurrentProgram: program(n, hour), NextProgram: program(n, hour + 1800, 1),
  DeviceInfo: { bitrate: 16000, signal: 30, error: 0, scramble: 0 } }));
const power = { powermanager: true, sleeptimerpause: false, sleeptimer: 5, preventsleep: false,
  preventdisplayoff: false, preventifdevicerunning: true, wakebeforeschedule: 5, wakeupsuppressduration: 60 };
const keyword = [{ id: 1, validate: true, title: '雲の郵便局', content: '', sid: [], category: 65535,
  andsearch: false, fuzzysearch: true, days: 240, week: 0, starthours: 0, hours: 24, deviceid: '',
  idletime: 60, readytime: 30, tailtime: 0, recordonly: true, shutdown: true, epgfollow: true }];
const recordings = [1, 2, 3].map((n) => ({ ...schedules[n - 1], id: n + 20, complete: true, slot: 'current',
  filename: `C:\\Recordings\\sample-${n}.ts`, starttime: hour - n * 86400, endtime: hour - n * 86400 + 1800 }));
const json = (res, value) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(value)); };
const server = http.createServer(async (req, res) => {
  try {
    const u = new URL(req.url, 'http://localhost');
    let text = ''; for await (const part of req) text += part;
    const p = text ? JSON.parse(text) : {};
    if (u.pathname === '/cnf') return json(res, { PowerManagerSetting: power, RecorderSetting: { keywordreservethreshold: 30 } });
    if (u.pathname === '/pms') return json(res, Object.assign(power, p));
    if (u.pathname === '/stl') return json(res, stations);
    if (u.pathname === '/sch') return json(res, schedules);
    if (u.pathname === '/dvs') return json(res, devices);
    if (u.pathname === '/kwr' || u.pathname === '/kwr/0') return json(res, keyword);
    if (u.pathname === '/rst') return json(res, { CurrentRecording: [1] });
    if (u.pathname === '/caption') return json(res, { snapshot: 3, hasMore: false, items: [
      { ID: 1, Text: '今日は、風の地図を作ってみましょう。', Pts: 1, Lang: 0 },
      { ID: 2, Text: 'この羽根が向いた方角を記録します。', Pts: 2, Lang: 0 },
      { ID: 3, Text: '小さな発見が、明日の工夫につながります。', Pts: 3, Lang: 0 }] });
    if (u.pathname === '/tvf') return json(res, Array.from({ length: 4 }, (_, i) => ({
      new: i === 0 ? [1, 2, 3].map((s, n) => ({ ...program(s, hour + (n + 1) * 3600), Title: '[新]' + titles[n] })) : [], end: [], first: [], free: [] })));
    if (u.pathname === '/tvp') {
      const ids = p.services?.length ? p.services : [1, 2, 3, 4];
      const start = p.start || hour, hours = p.hours || 4;
      const Sta = Object.fromEntries(ids.map(s => [s, stations[s]]));
      const Tvp = {};
      for (let i = 0; i < (p.week ? 7 : ids.length); i++) {
        const sid = p.week ? ids[0] : ids[i];
        Tvp[p.week ? i : sid] = Array.from({ length: hours * 2 }, (_, n) => program(sid, start + (p.week ? i * 86400 : 0) + n * 1800, n));
      }
      return json(res, { Start: start, End: start + hours * 3600, Week: !!p.week, ServiceList: ids, CurrentMilliseconds: epoch, Sta, Tvp });
    }
    if (!path.extname(u.pathname) && u.pathname !== '/') return json(res, {});
    const file = path.resolve(base, '.' + (u.pathname === '/' ? '/index.html' : u.pathname));
    if (!file.startsWith(base + path.sep) || !fs.existsSync(file)) { res.writeHead(404); return res.end(); }
    const types = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.png': 'image/png', '.woff2': 'font/woff2' };
    res.setHeader('Content-Type', types[path.extname(file)] || 'application/octet-stream'); res.end(fs.readFileSync(file));
  } catch (e) { res.writeHead(500); res.end(String(e)); }
});
const wss = new WebSocketServer({ server });
const sleep = ms => new Promise(r => setTimeout(r, ms));
async function capture(page, name, title, target) {
  const highlights = {
    'tv-overview': '.toolbar-broadcast-types, .toolbar-dates', 'tv-calendar': '.v-picker',
    'tv-multi': '.toolbar-broadcast-types', 'tv-reserve': '.tt-program-actions', 'tv-captions': '.caption-history',
    'tv-schedule': '.recordingschedule-table tbody', 'tv-keyword': '.keywordreserve-row',
    'tv-search': '.search-result-record-button', 'tv-power': '.power-manager-setting .row:first-child',
    'rb-controls': '#devices, .controls', 'rb-recording': '#state, #stop', 'rb-login': '#server, #login',
    'rb-viewer': '#viewer-device, #viewer-enabled', 'rb-playback-setting': '#viewer-device, #viewer-transport',
    'rb-playback': '#playback-device, #playback-file-list, #playback-panel', 'rb-timekeeper': '#timekeeper-toggle, #devices'
  };
  await page.evaluate(selector => {
    document.querySelectorAll('[data-manual-mark]').forEach(el => { el.style.outline = ''; el.removeAttribute('data-manual-mark'); });
    if (selector) document.querySelectorAll(selector).forEach(el => { el.dataset.manualMark = 'true'; el.style.outline = '2px solid #ed983b'; });
  }, highlights[name]);
  await page.evaluate(title => {
    document.querySelector('#manual-caption')?.remove();
    const el = document.createElement('div'); el.id = 'manual-caption';
    el.textContent = title + ' ｜ 放送局・番組・字幕はすべて説明用の架空データ';
    Object.assign(el.style, { position: 'fixed', bottom: '0', left: '0', width: '100%', padding: '8px 16px',
      background: '#102c43', color: '#fff', font: '14px "Yu Gothic",sans-serif', zIndex: '10000', pointerEvents: 'none' });
    document.body.append(el);
  }, title);
  await page.mouse.move(0, 0); await sleep(250);
  await (target ? page.locator(target) : page).screenshot({ path: path.join(out, name + '.png'), animations: 'disabled' });
  console.log(name);
}
async function webImages(browser) {
  const page = await browser.newPage({ viewport: { width: 1440, height: 960 }, colorScheme: 'light', timezoneId: 'Asia/Tokyo' });
  await page.clock.install({ time: new Date(epoch) });
  await page.route('https://**', route => route.abort());
  await page.goto(`http://127.0.0.1:${server.address().port}`);
  // Use locally installed MDI assets when supplied; no font CDN is required.
  if (process.env.MDI_CSS) {
    let css = fs.readFileSync(process.env.MDI_CSS, 'utf8');
    const font = fs.readFileSync(path.join(path.dirname(process.env.MDI_CSS), 'materialdesignicons-webfont.woff2')).toString('base64');
    css = css.replace(/url\([^)]*\)/g, `url(data:font/woff2;base64,${font})`);
    await page.addStyleTag({ content: css });
  }
  await page.waitForSelector('.tt-program-grid [data-caption-key]');
  await page.evaluate(() => {
    let vm = document.querySelector('.v-application').__vue__; while (vm.$options.name !== 'App') vm = vm.$parent;
    window.manualApp = vm;
    window.findManual = name => { const walk = v => v.$options.name === name ? v : v.$children.map(walk).find(Boolean); return walk(vm); };
    vm.options.displogo = false; vm.options.dispservice = 4; vm.options.twentyEight = false;
    vm.options.subtitlemin = 0; vm.options.contentmin = 30; vm.options.guidetype = 2;
    vm.sidenavi = false; vm.$refs.timetable.changeHours(0);
  });
  await sleep(500);
  await capture(page, 'tv-overview', '1　TIMETABLE：放送波・日時を選び、番組を予約');
  await page.evaluate(() => { manualApp.$refs.timetable.dpmodal = true; });
  await capture(page, 'tv-calendar', '2　カレンダーから未来・過去の日付を指定');
  await page.evaluate(() => { manualApp.$refs.timetable.dpmodal = false; manualApp.options.dispservice = 6; manualApp.$refs.timetable.setBroadcast(6); });
  await sleep(400); await capture(page, 'tv-multi', '3　地上波・BSの再クリックでマルチ表示');
  await page.evaluate(() => { manualApp.options.dispservice = 4; manualApp.$refs.timetable.tvpWeekly(1); });
  await sleep(400); await capture(page, 'tv-weekly', '4　週間表示：同じ局の番組を日ごとに確認');
  await page.evaluate(() => { manualApp.$refs.timetable.tvpWeeklyReverse(1); });
  await sleep(400);
  const cell = page.locator('#L1_102');
  await cell.scrollIntoViewIfNeeded();
  await cell.evaluate(el => el.click());
  await page.waitForSelector('.tt-program-expanded');
  await capture(page, 'tv-reserve', '5　番組枠の「録画予約」「検索」を使う');
  await page.evaluate(() => manualApp.$refs.timetable.closeProgramWindows());
  const currentCell = page.locator('#L1_100');
  await currentCell.scrollIntoViewIfNeeded();
  await currentCell.evaluate(el => el.click());
  await currentCell.evaluate(el => el.click()); await page.waitForSelector('.caption-history:visible');
  await capture(page, 'tv-captions', '6　現在・過去の番組で保存済み字幕を読む');
  await page.evaluate(() => { manualApp.$refs.timetable.closeProgramWindows(); manualApp.maintab = 1; });
  await capture(page, 'tv-schedule', '7　SCHEDULE：予約の日時・局・デバイスを確認');
  await page.evaluate(() => { manualApp.maintab = 4; });
  await capture(page, 'tv-device', '8　DEVICE：録画中・起動中・OFFの状態を確認');
  await page.setViewportSize({ width: 1440, height: 620 });
  await page.evaluate(() => { manualApp.maintab = 2; }); await sleep(300);
  await page.evaluate(() => {
    findManual('ProgramSearch').searchTitle = '雲の郵便局';
    manualApp.searchResults = [1, 2, 3].map((n) => ({ Sid: 2, Eid: 200 + n, Start: 1793962800 + n * 86400, End: 1793964600 + n * 86400,
      Title: '雲の郵便局', Subtitle: `第${n}話　空の便り`, ServiceName: '風待ち放送', DateW: `11/${6 + n} 21:00～21:30`, Content: '雲の上の町に届いた手紙をめぐる物語。' }));
  });
  await capture(page, 'tv-search', '9　SEARCH：結果を確かめ「検索予約登録」');
  await page.evaluate(() => { manualApp.maintab = 3; });
  await sleep(300); await capture(page, 'tv-keyword', '10　KEYWORD：検索条件と予約件数を確認');
  await page.setViewportSize({ width: 1440, height: 960 });
  await page.evaluate(() => { manualApp.maintab = 0; manualApp.sidenavi = true; }); await sleep(350);
  await page.getByRole('button', { name: '新番組', exact: true }).click();
  const newItem = page.locator('.program-item').filter({ hasText: '[新]' }).first();
  await newItem.hover(); await sleep(650);
  // Preserve the real hover action while taking this image.
  await page.addStyleTag({ content: '.program-item:hover .program-actions { visibility:visible; max-height:24px; }' });
  await newItem.evaluate(el => el.classList.add('manual-hover'));
  await page.addStyleTag({ content: '.manual-hover .program-actions { visibility:visible; max-height:24px; margin-top:2px; }' });
  await capture(page, 'tv-sidebar', '11　サイドナビゲーション：「新番組」から予約');
  await page.setViewportSize({ width: 1440, height: 620 });
  await page.evaluate(() => { manualApp.sidenavi = false; manualApp.maintab = 2; }); await sleep(250);
  await page.evaluate(() => {
    findManual('ProgramSearch').searchTitle = '星あかり工房';
    findManual('SearchResults').archiveSearch = true;
    manualApp.searchResults = [{ Sid: 1, Eid: 501, Start: 1162810800, End: 1162812600,
      Title: '星あかり工房の一日', Subtitle: '保存されていた昔の番組情報', ServiceName: '星見野テレビ', DateW: '2006/11/06 20:00～20:30' }];
  });
  await capture(page, 'tv-archive', '12　過去番組検索：保存・移行した情報を探す');
  await page.setViewportSize({ width: 1440, height: 960 });
  await page.evaluate(() => { manualApp.maintab = 5; }); await sleep(250);
  for (const [tab, name, title] of [[1, 'tv-settings-guide', '13　SETTING → 番組表：表示を好みに合わせる'],
    [2, 'tv-settings-device', '14　SETTING → デバイス：字幕と折りたたみの初期値'],
    [7, 'tv-power', '15　SETTING → 電源管理：「一時停止」OFFでタイマー有効']]) {
    await page.evaluate(tab => { findManual('SystemSetting').tab = tab; }, tab);
    await capture(page, name, title);
    if (tab === 1) {
      await page.locator('.system-settings .v-window-item--active').evaluate(el => { el.scrollTop = el.scrollHeight; });
      await page.getByText('視聴予約ボタンを表示', { exact: true }).scrollIntoViewIfNeeded();
      await capture(page, 'tv-settings-guide-more', '14　番組表設定の続き：表示方法・字幕・視聴予約ボタン');
    }
  }
  await page.close();
}
async function rockbarImages(browser) {
  const page = await browser.newPage({ viewport: { width: 1120, height: 650 }, colorScheme: 'light', timezoneId: 'Asia/Tokyo' });
  await page.clock.install({ time: new Date(epoch) });
  await page.route('https://**', route => route.abort());
  await page.addInitScript(({ stations, devices, schedules, recordings, power, epoch }) => {
    window.manualData = { stations, devices, schedules };
    const setting = { enabled: true, executable: 'C:\\TVTest\\TVTest.exe', options: '/d BonDriver_SRT.dll /nd',
      transport: 'srt', port: 3230, srtBasePort: 3230, srtLatency: 120 };
    window.rockbar = { platform: 'electron', onUpdate: cb => { window.manualUpdate = cb; },
      settings: async () => ({ server: 'http://tvrockgo.example.test', nativeViewer: true }), connect: async () => ({}),
      watchDevice: async () => {}, setMode: async () => {}, programs: async () => ({}),
      playbackRecordings: async () => recordings, windowOptions: async v => v,
      viewerSettings: async () => ({ devices: [{ id: 'REC01', registry: 'manual-1', kind: 'normal', setting },
        { id: 'PBK01', registry: 'manual-pbk1', kind: 'playback', setting: { ...setting, port: 3234 } }] }),
      saveViewerSettings: async (a, b, value) => ({ setting: value }), closeViewerSettings: async () => {} };
  }, { stations, devices, schedules, recordings, power, epoch });
  await page.goto(pathToFileURL(path.join(root, 'rockbar/src/index.html')).href);
  await page.evaluate(() => { document.body.style.height = 'calc(100vh - 38px)'; });
  await page.waitForFunction(() => typeof manualUpdate === 'function');
  await page.evaluate(({ epoch, power }) => {
    manualUpdate({ type: 'status', value: { connected: true, message: '接続済み（説明用）' } });
    manualUpdate({ type: 'stations', value: manualData.stations });
    manualUpdate({ type: 'server-time', value: epoch });
    manualUpdate({ type: 'schedules', value: manualData.schedules });
    manualUpdate({ type: 'devices', value: manualData.devices });
    manualUpdate({ type: 'programs', value: Object.fromEntries(manualData.devices.map(d => [d.CurrentService, [d.CurrentProgram]])) });
    manualUpdate({ type: 'power-manager', value: power });
  }, { epoch, power });
  await page.locator('[data-device="REC02"]').click();
  await capture(page, 'rb-controls', '1　デバイスを選択 → 電源 → 局をダブルクリック');
  await page.locator('[data-device="REC01"]').click();
  await capture(page, 'rb-recording', '2　REC：録画実行中／停止ボタンで録画停止');
  await page.locator('#settings').click();
  await capture(page, 'rb-login', '3　歯車 → 接続・ログイン → 保存して接続');
  await page.locator('#config-viewer-tab').click();
  await page.setViewportSize({ width: 1120, height: 1000 });
  await capture(page, 'rb-viewer', '4　リモート視聴設定：このPCのTVTestと受信ポート');
  await page.locator('#viewer-save').scrollIntoViewIfNeeded();
  await capture(page, 'rb-viewer-save', '5　設定の続き：送信内容を確認し、デバイスごとに保存');
  await page.locator('#viewer-device').selectOption('manual-pbk1');
  await page.locator('#viewer-device').scrollIntoViewIfNeeded();
  await capture(page, 'rb-playback-setting', '5　PBKの視聴設定：ファイル再生はSRT専用');
  await page.locator('#viewer-cancel').click();
  await page.setViewportSize({ width: 1120, height: 650 });
  await page.evaluate(() => {
    manualUpdate({ type: 'devices', value: [...manualData.devices, { ID: 'PBK01', Kind: 'playback', State: 2,
      Playback: { session: 'manual-session', scheduleId: 21, slot: 'current', title: '星あかり工房の一日', phase: 'paused',
        generation: 1, positionMs: 450000, durationMs: 1800000, cmStatus: 'not-signaled', manualBookmarks: [], programBoundaries: [] } }] });
  });
  await page.waitForSelector('[data-file]');
  await capture(page, 'rb-playback', '6　PBK：ファイル一覧をダブルクリックして再生');
  await page.locator('#timekeeper-toggle').click(); await sleep(600);
  await capture(page, 'rb-timekeeper', '7　TimeKeeper：録画・残り時間・次の予約を一覧');
  await page.close();
}
(async () => {
  if (!fs.existsSync(path.join(base, 'index.html'))) throw new Error('Build app to .tmp/manual-preview first.');
  fs.mkdirSync(out, { recursive: true });
  await new Promise(r => server.listen(0, '127.0.0.1', r));
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    if (process.env.MANUAL_TARGET !== 'rockbar') await webImages(browser);
    if (process.env.MANUAL_TARGET !== 'web') await rockbarImages(browser);
  }
  finally { await browser.close(); wss.close(); server.close(); }
})().catch(e => { console.error(e); wss.close(); server.close(); process.exitCode = 1; });
