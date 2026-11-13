const assert = require('assert');
const fs = require('fs');
const path = require('path');
const vm = require('vm');
const Vue = require('vue');
const renderer = require('../src/timeTableRenderer');
const utility = require('../src/utility');
const label = require('../src/define');

function loadComponent(file, axios = {}, globals = {}) {
  let script = fs.readFileSync(path.join(__dirname, '../src', file), 'utf8')
    .match(/<script>([\s\S]*?)<\/script>/)[1];
  script = script.replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g, (_, binding, source) => {
    const value = source.includes('timeTableRenderer') ? 'renderer' :
      source.includes('utility') ? 'utility' : source.includes('define') ? 'label' :
      source.includes('searchResults') ? 'searchResults' : source.includes('uiRefreshQueue') ? 'uiRefreshQueue' :
      source.includes('timeTableGenreColors') ? 'genreColors' : source === 'axios' ? 'axios' : '{}';
    return `const ${binding} = ${value === '{}' ? value : 'dependencies.' + value};`;
  }).replace('export default', 'module.exports =');
  const context = { module: { exports: {} }, dependencies: { renderer, utility, label, axios,
    searchResults: require('../src/searchResults'), uiRefreshQueue: require('../src/uiRefreshQueue'),
    genreColors: require('../src/timeTableGenreColors') },
    require: () => '', window: { addEventListener() {}, removeEventListener() {} },
    document: { cookie: '' }, WebSocket: { OPEN: 1 },
    setTimeout, clearTimeout, setInterval, clearInterval, ...globals };
  vm.runInNewContext(script, context, { filename: file });
  return context.module.exports;
}

function program(sid, eid, start, end) {
  return { Sid: sid, Eid: eid, StartM: start, EndM: end, Start: start * 60, End: end * 60,
    StartD: new Date(start * 60000), EndD: new Date(end * 60000), DurationM: end - start,
    Title: `番組${eid}`, Subtitle: '', Content: '', Cnib_a: 0, Cnib_b: 0 };
}

async function main() {
  const options = { divider: 2, subtitlemin: 30, contentmin: 60, guidetype: 0, weekly: false };
  for (const [hours, duration, compact] of [[2, 15, true], [2, 16, false], [4, 7, true],
    [4, 8, false], [8, 3, true], [8, 4, false], [8, 5, false]]) {
    const shortProgram = program(10, 1, 0, duration);
    const grid = renderer.createMinuteGrid(duration, 1).map(() => [shortProgram]);
    const html = renderer.renderProgramGrid(grid, new Date(0), duration, 1, { ...options, hours }, {});
    assert(html.includes('data-short="' + compact + '"'),
      `${duration}-minute programs at ${hours}px/min choose labels by available height`);
  }
  for (const hours of [2, 4, 8]) {
    const grid = renderer.renderProgramGrid(renderer.createMinuteGrid(60, 1), new Date(0),
      60, 1, { ...options, hours }, {});
    assert(grid.includes('height:' + (60 * hours) + 'px'), 'one hour scales to the selected minute height');
  }
  for (const hours of [undefined, null, 0, 6, 24, '8']) {
    assert.strictEqual(renderer.getPixelsPerMinute({ hours }), 4, 'unsupported legacy values use normal height');
  }
  // API/WebSocket由来の文字列は、通常表・週間表・詳細表示でHTMLにしない。
  const attack = '<img src=x onerror="alert(1)"></pre><svg onload=alert(2)>日本語 & "引用" \'文字\'';
  const escapedAttack = '&lt;img src=x onerror=&quot;alert(1)&quot;&gt;&lt;/pre&gt;&lt;svg onload=alert(2)&gt;日本語 &amp; &quot;引用&quot; &#039;文字&#039;';
  for (const weekly of [false, true]) {
    const headers = renderer.renderStationHeaders([10], {
      Week: weekly, Start: 0, ServiceList: [10], Sta: { 10: { Name: attack } },
    }, { ...options, weekly }, -1);
    assert(headers.includes(escapedAttack), 'station names remain literal text in both views');
    assert(!headers.includes('<img src=x') && !headers.includes('<svg'));
    assert(headers.includes('onclick="tvpWeekly'), 'generated header controls remain HTML');
  }
  const unsafeProgram = { ...program(10, 100, 0, 60), Title: attack,
    Subtitle: attack, Content: attack, DeviceID: attack, IsRec: true, SearchID: 0 };
  const originalProgram = JSON.stringify(unsafeProgram);
  const safeGrid = renderer.renderProgramGrid([[unsafeProgram]], new Date(0), 1, 1,
    { ...options, subtitlemin: 1, contentmin: 1 }, {});
  assert.strictEqual(safeGrid.split(escapedAttack).length - 1, 8,
    'title, subtitle, content and device names are escaped in all inline/detail occurrences');
  assert(!safeGrid.includes('<img src=x') && !safeGrid.includes('<svg'));
  assert(safeGrid.includes('q=' + encodeURIComponent(attack) + '&udm=14'),
    'Google searches use the original title');
  assert.strictEqual(JSON.stringify(unsafeProgram), originalProgram, 'rendering never escapes stored program data');

  const url = 'https://example.com/search?a=1&b=two#part';
  const escapedURL = 'https://example.com/search?a=1&amp;b=two#part';
  const linkedURL = '<a href="' + escapedURL + '" target="_blank" rel="noopener noreferrer">' + escapedURL + '</a>';
  assert.strictEqual(utility.replaceTextWithLinks('日本語\n' + attack + '\n' + url + '\n末尾 &'),
    '日本語\n' + escapedAttack + '\n' + linkedURL + '\n末尾 &amp;',
    'linkification escapes surrounding markup while preserving newlines and the entire query');
  assert.strictEqual(utility.replaceTextWithLinks(url + ' ' + url), linkedURL + ' ' + linkedURL);
  assert.strictEqual(utility.replaceTextWithLinks(url), linkedURL, 'repeated calls preserve URL matching');
  assert.strictEqual(utility.replaceTextWithLinks('<a href="javascript:alert(1)">リンク</a>'),
    '&lt;a href=&quot;javascript:alert(1)&quot;&gt;リンク&lt;/a&gt;');
  assert.strictEqual(utility.replaceTextWithLinks('&lt;b&gt;'), '&amp;lt;b&amp;gt;',
    'literal entity text is preserved rather than decoded into markup');
  assert.strictEqual(utility.replaceTextWithLinks(''), '');

  const first = program(10, 1, 0, 30);
  const overlapping = program(10, 2, 20, 40);
  const tt = { Week: false, ServiceList: [10, 20], Tvp: { 10: [first, overlapping] } };
  const grid = renderer.fillMinuteGrid([10, 20], tt, renderer.createMinuteGrid(60, 2), 0, 60);
  assert.strictEqual(grid[29][0], first, 'overlap preserves the first program');
  assert.strictEqual(grid[30][0], overlapping);
  const programs = {};
  const html = renderer.renderProgramGrid(grid, new Date(0), 60, 2, options, programs);
  assert(html.includes('grid-column:2;grid-row:1 / span 30'));
  assert(html.includes('grid-column:2;grid-row:31 / span 10'));
  assert(html.includes('class="pblank" style="grid-column:2;grid-row:41 / span 20'));
  assert(html.includes('class="pblank" style="grid-column:3;grid-row:1 / span 60'));
  assert.strictEqual(Object.keys(programs).length, 2);
  const clipped = renderer.fillMinuteGrid([10], tt, renderer.createMinuteGrid(10, 2), 25, 35);
  assert.strictEqual(clipped[0][0], first);
  assert.strictEqual(clipped[9][0], overlapping);

  const weeklyProgram = program(10, 3, label.DAY + 5, label.DAY + 10);
  const week = { Week: true, Start: 0, services: [10], Sta: { 10: { Name: '局名' } },
    Tvp: { 1: [weeklyProgram] } };
  const weeklyGrid = renderer.fillMinuteGrid([10], week, renderer.createMinuteGrid(60, 7), 0, 60);
  assert.strictEqual(weeklyGrid[5][1], weeklyProgram, 'weekly day offsets');
  assert.strictEqual(weeklyGrid[5][0], 0);
  const headers = renderer.renderStationHeaders([10], week, { ...options, weekly: true }, -1);
  assert.strictEqual((headers.match(/data-week-start=/g) || []).length, 7);
  assert(!headers.includes('renderTLC'));
  const logoHeaders = renderer.renderStationHeaders([10], week, { ...options, weekly: true, displogo: true }, -1);
  assert(logoHeaders.includes('src="/static/cdt/10_1"'));
  const stationLogo = loadComponent('components/StationLogo.vue');
  assert.strictEqual(stationLogo.computed.src.call({ sid: 10 }), '/static/cdt/10_1');

  const originalNow = utility.now;
  utility.now = () => new Date(100000);
  try {
    const future = program(10, 4, 5, 10);
    const renderActions = (opt, item = future) => renderer.renderProgramGrid(
      [[item]], item.StartD, 1, 1, opt, {});
    assert(!renderActions(options).includes('視聴予約'), 'watch reservation is hidden by default');
    const watchOptions = { ...options, watchbutton: true };
    const watchHtml = renderActions(watchOptions);
    assert(watchHtml.includes('pgReserve(10,4,300,600,true)'));
    assert(watchHtml.indexOf('録画予約') < watchHtml.indexOf('視聴予約'));
    assert(watchHtml.indexOf('視聴予約') < watchHtml.indexOf('>検索</label>'));
    assert(!renderActions(watchOptions, { ...future, IsRec: true, SearchID: 0 }).includes('視聴予約'),
      'reserved programs keep their cancellation action');
    assert(renderActions(watchOptions, { ...future, IsRec: true, IsWatch: true }).includes('data-watch="true"'));
    assert(renderActions(watchOptions, { ...future, IsRec: true }).includes('data-watch="false"'));
    assert(!renderActions(watchOptions, { ...future, EndD: new Date(90000) }).includes('視聴予約'),
      'ended programs cannot be reserved');
    const late = { StartD: new Date(300000), EndD: new Date(400000) };
    const early = { StartD: new Date(110000), EndD: new Date(120000) };
    assert.strictEqual(renderer.getNextProgramTransitionDelay({ ...tt, Tvp: { 10: [late, early] } }), 10000);
    assert.strictEqual(renderer.getNextProgramTransitionDelay({ ...week, Tvp: { 1: [early] } }), 10000);
  } finally { utility.now = originalNow; }

  const component = loadComponent('components/TimeTable.vue');
  const autoOptions = { dispservice: 0, divider: 3, broadcast: 1, page: [0, 8] };
  for (let count = 1; count <= 12; count++) {
    const width = count * 200 + (Math.ceil(count / 3) + 1) * 24;
    assert.strictEqual(renderer.getServiceDisplayCount(autoOptions, width), count,
      'automatic display supports every station count from one through twelve');
    if (count > 1) assert.strictEqual(renderer.getServiceDisplayCount(autoOptions, width - 1), count - 1);
  }
  assert.strictEqual(renderer.getServiceDisplayCount(autoOptions, 0), 1);
  assert.strictEqual(renderer.getServiceDisplayCount(autoOptions, 10000), 12);
  assert.strictEqual(renderer.getServiceDisplayCount({ dispservice: 8 }, 320), 8);
  const layoutRedraws = [];
  const scrollBody = { clientWidth: 1280, style: { getPropertyValue() {}, setProperty() {} },
    getBoundingClientRect: () => ({ top: 100 }) };
  const layoutContext = { options: autoOptions, serviceDisplayWidth: 0,
    $refs: { programScrollBody: scrollBody, stationHeaderWrap: { style: {} },
      stationHeaderTable: { style: {} }, programTable: { style: {} } },
    displayServiceCount: component.methods.displayServiceCount,
    requestRedrawWithProgramScrollReset: (opt, preserve) => layoutRedraws.push(preserve) };
  component.methods.syncStationHeaderLayout.call(layoutContext);
  assert.strictEqual(layoutContext.displayServiceCount(), 6);
  scrollBody.clientWidth = 1020; // 右側の選択欄によって番組表の幅だけが狭くなる。
  component.methods.syncStationHeaderLayout.call(layoutContext);
  assert.strictEqual(layoutContext.displayServiceCount(), 4);
  assert.strictEqual(autoOptions.page[1], 4, 'resizing retains the page containing the first visible station');
  assert.deepStrictEqual(layoutRedraws, [true, true], 'both layouts preserve the displayed time range');
  scrollBody.clientWidth = 0;
  component.methods.syncStationHeaderLayout.call(layoutContext);
  assert.strictEqual(layoutContext.displayServiceCount(), 4, 'hidden tabs retain their last visible width');
  for (const hours of [2, 4, 8]) {
    let boundaryRenders = 0;
    const viewportHeight = 1200;
    const scroll = { clientHeight: viewportHeight, scrollTop: 48 * 60 * hours - viewportHeight };
    const boundaryContext = { timetable: {}, virtualStart: 0, renderedStart: 0,
      renderedPixelsPerMinute: hours, programScrollElement: () => scroll,
      programGridTop: () => 0, $el: { getClientRects: () => [{}] },
      queueProgramRender() { boundaryRenders++; } };
    const visibleTime = scroll.scrollTop * 60 / hours;
    component.methods.onTimeTableScrollEnd.call(boundaryContext, { target: scroll });
    assert.strictEqual(boundaryRenders, 1, 'the physical bottom recenters even when a short viewport spans over six hours');
    const before = visibleTime - boundaryContext.virtualStart;
    const after = boundaryContext.virtualStart + 48 * 3600 - visibleTime - viewportHeight * 60 / hours;
    assert(before > 0 && before === after, 'recentering leaves space above and below the entire visible time range');
    scroll.scrollTop = before * hours / 60;
    boundaryContext.programGridTop = () => -boundaryContext.virtualStart * hours / 60;
    component.methods.onTimeTableScrollEnd.call(boundaryContext, { target: scroll });
    assert.strictEqual(boundaryRenders, 1, 'the centered viewport does not cause repeated recentering');
  }
  const transitionTimers = [];
  const timerComponent = loadComponent('components/TimeTable.vue', {}, {
    document: { cookie: '', getElementById: id => ({ id, isConnected: true }) },
    setTimeout: (callback, delay) => { transitionTimers.push({ callback, delay }); return transitionTimers.length; },
    clearTimeout() {},
  });
  let localRefreshes = 0;
  let networkRefreshes = 0;
  const timerTable = new (Vue.extend(timerComponent))({ propsData: {
    options: { current: new Date(), broadcast: 1, weekly: false, dispservice: 1,
      page: [0, 0], divider: 1, subtitlemin: 30, contentmin: 60, stationsplitscroll: true },
    station: { viewTS: 1 }, timetable: null,
  }, methods: { queueProgramRender() { localRefreshes++; } } });
  timerTable.$on('redrawtt', () => networkRefreshes++);
  const futureMinute = Math.floor(Date.now() / 60000) + 2;
  const transition = program(10, 99, futureMinute, futureMinute + 30);
  const transitionTable = { Start: transition.Start, End: transition.End, Week: false,
    ServiceList: [10], services: [10], Sta: { 10: { Name: '局名' } }, Tvp: { 10: [transition] } };
  timerTable.renderProgram(transitionTable);
  const localBefore = localRefreshes;
  transitionTimers.at(-1).callback();
  assert.strictEqual(localRefreshes, localBefore + 1, 'program transitions render cached data locally');
  assert.strictEqual(networkRefreshes, 0, 'program transitions do not request /tvp');
  const distant = program(10, 100, futureMinute + 40 * 1440, futureMinute + 40 * 1440 + 30);
  timerTable.renderProgram({ ...transitionTable, Start: distant.Start, End: distant.End, Tvp: { 10: [distant] } });
  assert.strictEqual(transitionTimers.at(-1).delay, 2147483647, 'distant transitions cannot overflow into a rapid timer');
  const scaleBody = { scrollTop: 120 };
  timerTable.$el = {};
  timerTable.programScrollElement = () => scaleBody;
  timerTable.programGridTop = () => 0;
  timerTable.onTimeTableScroll = () => {};
  timerTable.renderedStart = transition.Start;
  timerTable.renderedPixelsPerMinute = 4;
  for (const hours of [8, 2, 4]) {
    timerTable.options.hours = hours;
    timerTable.renderProgram(transitionTable);
    await Vue.nextTick();
    assert.strictEqual(scaleBody.scrollTop, 30 * hours,
      'changing scale preserves the time at the top of the viewport');
    assert(timerTable.content.includes('height:' + (48 * 60 * hours) + 'px'),
      'the virtual scroll range scales along with the program grid');
  }
  timerTable.resetProgramScrollOnRender = true;
  timerTable.pendingRedrawTimetable = transitionTable;
  timerTable.pendingProgramTime = transition.Start + 1800;
  const beforePendingRender = transitionTimers.length;
  timerTable.renderProgram(transitionTable);
  assert.strictEqual(timerTable.resetProgramScrollOnRender, true,
    'rendering unchanged data while a destination request is pending cannot clear the wait state');
  assert.strictEqual(timerTable.pendingProgramTime, transition.Start + 1800);
  assert.strictEqual(transitionTimers.length, beforePendingRender, 'pending old data does not restart rendering');
  timerTable.renderProgram({ ...transitionTable });
  assert.strictEqual(timerTable.resetProgramScrollOnRender, false, 'new response data completes the pending render');
  assert.strictEqual(timerTable.pendingRedrawTimetable, null);
  timerTable.$destroy();
  // 更新通知が複数のtickに分かれて届いても、操作中の詳細表示を維持する。
  let committedRenders = 0;
  const popupTable = new (Vue.extend(timerComponent))({ propsData: {
    options: { current: new Date(), broadcast: 1, dispservice: 1, page: [0, 0],
      divider: 1, subtitlemin: 30, contentmin: 60, captiontimetable: true },
    station: { viewTS: 1 }, timetable: transitionTable,
  }, methods: { renderProgram(value) {
    const awaitingResponse = this.resetProgramScrollOnRender && this.renderedStart !== null &&
      value && value === this.pendingRedrawTimetable;
    timerComponent.methods.renderProgram.call(this, value);
    if (!this.programRenderDeferred && !awaitingResponse) committedRenders++;
  } } });
  const flush = async () => { await Vue.nextTick(); await Vue.nextTick(); };
  await flush();
  for (let update = 1; update <= 2; update++) {
    popupTable.timetable = { ...transitionTable,
      Tvp: { 10: [{ ...transition, Title: '待機なし' + update }] } };
    await flush();
    assert(popupTable.content.includes('待機なし' + update),
      'successive automatic updates render on the next tick without waiting for an interval');
  }
  const beforePopupRenders = committedRenders;
  const beforePopupContent = popupTable.content;
  popupTable.popupHtml = '<button>録画予約</button>';
  for (let update = 1; update <= 3; update++) {
    popupTable.timetable = { ...transitionTable,
      Tvp: { 10: [{ ...transition, Title: '更新' + update }] } };
    await flush();
    assert.strictEqual(popupTable.popupHtml, '<button>録画予約</button>');
    assert.strictEqual(popupTable.content, beforePopupContent, 'incoming updates leave the visible grid intact');
  }
  assert.strictEqual(committedRenders, beforePopupRenders);
  assert.strictEqual(popupTable.programRenderDeferred, true);
  popupTable.closeProgramPopup();
  popupTable.closeProgramPopup();
  await flush();
  assert.strictEqual(committedRenders, beforePopupRenders + 1, 'closing flushes multiple updates exactly once');
  assert(popupTable.content.includes('更新3'), 'the single render uses the latest program data');
  popupTable.closeProgramPopup();
  await flush();
  assert.strictEqual(committedRenders, beforePopupRenders + 1, 'closing without updates does not redraw');

  popupTable.popupHtml = '詳細';
  popupTable.queueProgramRender();
  await flush();
  popupTable.closeProgramPopup();
  popupTable.popupHtml = '別の詳細';
  await flush();
  assert.strictEqual(popupTable.popupHtml, '別の詳細', 'reopening before the queued render defers it again');
  assert.strictEqual(committedRenders, beforePopupRenders + 1);
  popupTable.requestRedrawWithProgramScrollReset(popupTable.options);
  assert.strictEqual(popupTable.popupHtml, '', 'explicit navigation closes details immediately');
  popupTable.timetable = { ...transitionTable };
  await flush();
  assert.strictEqual(popupTable.resetProgramScrollOnRender, false);
  assert.strictEqual(committedRenders, beforePopupRenders + 2, 'navigation renders the new response');

  popupTable.expandedProgramCell = { classList: { remove() {} }, style: { removeProperty() {} } };
  popupTable.queueProgramRender();
  await flush();
  assert.strictEqual(popupTable.programRenderDeferred, true, 'expanded details also defer automatic rendering');
  popupTable.closeProgramPopup();
  await flush();
  assert.strictEqual(committedRenders, beforePopupRenders + 3);

  const beforeCaptionRenders = committedRenders;
  const captionProgramKey = transition.Sid + '_' + transition.Eid + '_' + transition.Start;
  const captionCell = { dataset: { captionKey: captionProgramKey } };
  const captionClick = { type: 'click', target: {
    closest: selector => selector === '[data-caption-key]' ? captionCell : null,
  } };
  popupTable.popupHtml = '詳細';
  popupTable.clickedProgramKey = captionProgramKey;
  popupTable.programClickStage = 'details';
  popupTable.timetable = { ...transitionTable };
  await flush();
  assert.strictEqual(popupTable.programRenderDeferred, true);
  popupTable.onProgramClick(captionClick);
  await flush();
  assert.strictEqual(popupTable.captionProgram, transition,
    'switching from details to caption history survives an already queued redraw');
  assert.strictEqual(popupTable.programClickStage, 'captions');
  assert.strictEqual(committedRenders, beforeCaptionRenders);
  const captionContent = popupTable.content;
  for (let update = 1; update <= 3; update++) {
    popupTable.timetable = { ...transitionTable,
      Tvp: { 10: [{ ...transition, Title: '字幕表示中' + update }] } };
    await flush();
    assert.strictEqual(popupTable.captionProgram, transition);
    assert.strictEqual(popupTable.content, captionContent);
  }
  popupTable.closeProgramPopup();
  await flush();
  assert.strictEqual(popupTable.programRenderDeferred, true,
    'scroll and hover popup closures cannot release updates while caption history is open');
  assert.strictEqual(committedRenders, beforeCaptionRenders);
  const captionPackets = [];
  popupTable.$refs.captionHistory = { enqueue: packet => captionPackets.push(packet) };
  const liveCaption = { Sid: transition.Sid, Eid: transition.Eid, Text: '字幕' };
  popupTable.queueHistoryCaption(liveCaption);
  assert.strictEqual(captionPackets[0], liveCaption, 'live captions continue while timetable rendering is deferred');
  popupTable.closeProgramWindows();
  popupTable.closeProgramWindows();
  await flush();
  assert.strictEqual(popupTable.captionProgram, null);
  assert.strictEqual(committedRenders, beforeCaptionRenders + 1,
    'closing caption history flushes all deferred updates exactly once');
  assert(popupTable.content.includes('字幕表示中3'));

  popupTable.captionProgram = transition;
  popupTable.timetable = { ...transitionTable };
  await flush();
  popupTable.options.captiontimetable = false;
  await flush();
  assert.strictEqual(popupTable.captionProgram, null, 'disabling caption history releases the deferred redraw');
  assert.strictEqual(committedRenders, beforeCaptionRenders + 2);
  popupTable.$destroy();

  const reservations = [];
  const reservationComponent = loadComponent('components/TimeTable.vue', {
    post: (url, body) => { reservations.push([url, JSON.parse(body)]); return Promise.resolve({ status: 200 }); },
  });
  let reservationPopupCloses = 0;
  const reservationContext = { api: { schedule: '/schedule' },
    closeProgramPopup() { reservationPopupCloses++; } };
  reservationComponent.methods.pgReserve.call(reservationContext, 10, 4, 300, 600, true);
  reservationComponent.methods.pgReserve.call(reservationContext, 10, 4, 300, 600);
  assert.deepStrictEqual(reservations, [
    ['/schedule', { sid: 10, eid: 4, start: 300, end: 600, watch: true }],
    ['/schedule', { sid: 10, eid: 4, start: 300, end: 600 }],
  ]);
  await Promise.resolve();
  assert.strictEqual(reservationPopupCloses, 2, 'successful reservations close details to flush pending updates');
  const failedReservation = loadComponent('components/TimeTable.vue', {
    post: () => Promise.reject(new Error('schedule failed')),
  });
  const previousErrorLogger = utility.err;
  try {
    utility.err = () => {};
    failedReservation.methods.pgReserve.call(reservationContext, 10, 4, 300, 600);
    await flush();
    assert.strictEqual(reservationPopupCloses, 2, 'failed reservations leave details open for retry');
  } finally { utility.err = previousErrorLogger; }
  const side = loadComponent('components/SideNavigation.vue', {
    post: (url, body) => { reservations.push([url, JSON.parse(body)]); return Promise.resolve({ status: 200 }); },
  });
  const search = loadComponent('components/SearchResults.vue', {
    post: (url, body) => { reservations.push([url, JSON.parse(body)]); return Promise.resolve({ status: 200 }); },
  });
  const reservedProgram = program(10, 4, 5, 10);
  side.methods.pgReserve.call(reservationContext, reservedProgram, true);
  side.methods.pgReserve.call(reservationContext, reservedProgram);
  const searchContext = { ...reservationContext, canReserveProgram: () => true,
    recordingInfo: item => ({ isRec: !!item.IsRec }), programKey: () => '10_4',
    $set: (item, key, value) => { item[key] = value; } };
  search.methods.addProgramReserve.call(searchContext, reservedProgram, 0, true);
  await new Promise(resolve => setImmediate(resolve));
  assert.strictEqual(reservedProgram.IsWatch, true, 'watch state updates immediately after reservation');
  assert.strictEqual(reservedProgram.IsRec, true);
  const beforeDuplicate = reservations.length;
  search.methods.addProgramReserve.call(searchContext, reservedProgram, 0);
  assert.strictEqual(reservations.length, beforeDuplicate, 'reserved programs cannot be reserved again');
  assert.deepStrictEqual(reservations.slice(2), [
    ['/schedule', { sid: 10, eid: 4, start: 300, end: 600, watch: true }],
    ['/schedule', { sid: 10, eid: 4, start: 300, end: 600 }],
    ['/schedule', { sid: 10, eid: 4, start: 300, end: 600, watch: true }],
  ]);
  for (const [watchonly, recordonly, expected] of [[true, false, true], [false, true, false], [true, true, false]]) {
    const schedule = [{ validate: true, sid: 10, eid: 4, watchonly, recordonly }];
    assert.strictEqual(side.methods.withRecordingState.call({ schedule }, reservedProgram).IsWatch, expected);
    assert.strictEqual(search.methods.recordingInfo.call({ schedule, reservationIndex: require('../src/searchResults').reservationIndex(schedule) }, reservedProgram).isWatch, expected);
    const scheduleComponent = loadComponent('components/RecordingSchedule.vue');
    assert.strictEqual(scheduleComponent.methods.rowClass.call({ isRecording: () => true }, schedule[0]),
      expected ? 'recordingschedule-watch' : 'recordingschedule-recording');
  }
  const scheduleView = loadComponent('components/RecordingSchedule.vue');
  const future = utility.tmtoux(utility.now()) + 86400;
  const schedules = [
    { id: 1, deviceid: 'REC10', starttime: future + 100, endtime: future + 200, DateMD: '11月2日(月)' },
    { id: 2, deviceid: 'REC2', starttime: future + 300, endtime: future + 400, DateMD: '11月3日(火)' },
    { id: 3, deviceid: 'REC2', starttime: future, endtime: future + 100, DateMD: '11月2日(月)' },
    { id: 4, deviceid: '', starttime: future + 50, endtime: future + 150, DateMD: '11月2日(月)' },
    { id: 5, deviceid: 'REC2', starttime: future, endtime: future + 100, complete: true },
    { id: 6, deviceid: 'REC10', starttime: future - 172800, endtime: future - 86401 },
  ];
  const scheduleState = { schedule: schedules, groupByDevice: false };
  const sortedSchedules = () => scheduleView.computed.activeSchedule.call(scheduleState);
  assert.deepStrictEqual(Array.from(sortedSchedules(), item => item.id), [3, 4, 1, 2],
    'date mode sorts active schedules chronologically and excludes completed/expired entries');
  scheduleState.groupByDevice = true;
  assert.deepStrictEqual(Array.from(sortedSchedules(), item => item.id), [3, 2, 1, 4],
    'device mode groups IDs naturally, sorts each group by time, and places unassigned schedules last');
  scheduleState.activeSchedule = sortedSchedules();
  scheduleState.scheduleGroup = item => scheduleView.methods.scheduleGroup.call(scheduleState, item);
  scheduleState.scheduleGroups = scheduleView.computed.scheduleGroups.call(scheduleState);
  assert.deepStrictEqual(Array.from(scheduleState.scheduleGroups, group => group.key), ['REC2', 'REC10', ''],
    'jump targets include each visible device once, including unassigned schedules');
  assert.deepStrictEqual(Array.from(scheduleView.computed.scheduleGroupCounts.call(scheduleState), ([key, count]) => [key, count]),
    [['REC2', 2], ['REC10', 1], ['', 1]],
    'device counts include unassigned reservations and exclude completed/expired entries');
  scheduleState.groupByDevice = false;
  scheduleState.activeSchedule = sortedSchedules();
  assert.deepStrictEqual(Array.from(scheduleView.computed.scheduleGroups.call(scheduleState), group => group.key),
    ['11月2日(月)', '11月3日(火)'], 'date jump targets follow chronological order without duplicates');
  assert.deepStrictEqual(Array.from(scheduleView.computed.scheduleGroupCounts.call(scheduleState), ([key, count]) => [key, count]),
    [['11月2日(月)', 3], ['11月3日(火)', 1]], 'date counts regroup the same four visible reservations');
  assert.strictEqual(scheduleState.activeSchedule.length, 4, 'both grouping modes use the full visible list as the total');
  assert.strictEqual(scheduleView.computed.scheduleGroupCounts.call({ activeSchedule: [] }).size, 0);
  scheduleState.groupByDevice = true;
  assert.strictEqual(scheduleView.methods.scheduleGroup.call(scheduleState, schedules[0]), 'REC10');
  scheduleState.groupByDevice = false;
  assert.strictEqual(scheduleView.methods.scheduleGroup.call(scheduleState, schedules[0]), '11月2日(月)');
  assert.deepStrictEqual(schedules.map(item => item.id), [1, 2, 3, 4, 5, 6],
    'display sorting never reorders the shared schedule array');
  schedules[0].deviceid = 'REC2';
  scheduleState.groupByDevice = true;
  assert.deepStrictEqual(Array.from(sortedSchedules(), item => item.id), [3, 1, 2, 4],
    'assignment updates regroup the current schedules');

  const observedScheduleElements = [];
  const scheduleResizeListeners = new Map();
  let scheduleResizeCallback, scheduleObserverDisconnected = false;
  const measuredSchedule = loadComponent('components/RecordingSchedule.vue', {}, {
    ResizeObserver: class {
      constructor(callback) { scheduleResizeCallback = callback; }
      observe(element) { observedScheduleElements.push(element); }
      disconnect() { scheduleObserverDisconnected = true; }
    },
    window: {
      addEventListener(event, callback) { scheduleResizeListeners.set(event, callback); },
      removeEventListener(event, callback) {
        if (scheduleResizeListeners.get(event) === callback) scheduleResizeListeners.delete(event);
      },
    },
  });
  const scheduleTable = {};
  const scheduleContainer = { clientHeight: 400, scrollHeight: 400, querySelector: () => scheduleTable };
  const overflowState = { ...measuredSchedule.data(),
    $el: { querySelector: () => scheduleContainer }, updateRecordingStatus() {} };
  overflowState.updateScheduleOverflow = measuredSchedule.methods.updateScheduleOverflow.bind(overflowState);
  measuredSchedule.mounted.call(overflowState);
  assert.strictEqual(overflowState.scheduleOverflow, false, 'jump buttons stay hidden when the list fits exactly');
  assert.deepStrictEqual(observedScheduleElements, [scheduleContainer, scheduleTable],
    'both viewport size and table contents trigger overflow measurements');
  scheduleContainer.scrollHeight = 700;
  scheduleResizeCallback();
  assert.strictEqual(overflowState.scheduleOverflow, true, 'new rows or expanded details reveal jump buttons');
  scheduleContainer.clientHeight = 800;
  scheduleResizeListeners.get('resize')();
  assert.strictEqual(overflowState.scheduleOverflow, false, 'enlarging the window hides unnecessary jump buttons');
  scheduleContainer.clientHeight = 400;
  measuredSchedule.updated.call(overflowState);
  assert.strictEqual(overflowState.scheduleOverflow, true, 'rendered grouping and detail changes recheck overflow');
  scheduleContainer.scrollHeight = 300;
  measuredSchedule.updated.call(overflowState);
  assert.strictEqual(overflowState.scheduleOverflow, false, 'removing rows or collapsing details hides jump buttons');
  scheduleContainer.clientHeight = 0;
  scheduleResizeCallback();
  assert.strictEqual(overflowState.scheduleOverflow, false, 'an inactive hidden tab does not count as an overflowing list');
  measuredSchedule.beforeDestroy.call(overflowState);
  assert.strictEqual(scheduleObserverDisconnected, true);
  assert.strictEqual(scheduleResizeListeners.size, 0, 'resize listeners are removed with the schedule view');

  const callbacks = [];
  const renders = [];
  const queueContext = { renderQueued: false, timetable: tt,
    $nextTick: cb => callbacks.push(cb), renderProgram: value => renders.push(value) };
  component.methods.queueProgramRender.call(queueContext);
  component.methods.queueProgramRender.call(queueContext);
  queueContext.timetable = week;
  assert.strictEqual(callbacks.length, 1);
  callbacks.shift()();
  assert.strictEqual(renders[0], week, 'coalesced rendering uses the latest data');
  queueContext._isBeingDestroyed = true;
  component.methods.queueProgramRender.call(queueContext);
  callbacks.shift()();
  assert.strictEqual(renders.length, 1, 'queued work stops on destruction');

  let redraws = 0;
  const timetableOptions = { broadcast: 1, current: new Date(), hours: 6, page: [0, 0, 0, 0, 0, 0, 0],
    dispservice: 8, displogo: true, guidetype: 0, subtitlemin: 30, contentmin: 60,
    divider: 3, twentyEight: true, stationsplitscroll: true, watchbutton: false, devconpin: {} };
  const instance = new (Vue.extend(component))({ propsData: { options: timetableOptions,
    station: { all: {} }, timetable: null }, methods: { renderProgram() { redraws++; } } });
  await Vue.nextTick();
  const initialRenders = redraws;
  Vue.set(timetableOptions.devconpin, 'device', true);
  await Vue.nextTick();
  assert.strictEqual(redraws, initialRenders, 'pinning a device must not rebuild the timetable');
  timetableOptions.stationsplitscroll = false;
  await Vue.nextTick();
  await Vue.nextTick();
  assert.strictEqual(redraws, initialRenders, 'the compatibility scroll option must not rebuild the timetable');
  timetableOptions.displogo = false;
  await Vue.nextTick();
  await Vue.nextTick();
  assert.strictEqual(redraws, initialRenders + 1, 'visual settings still rebuild the timetable');
  timetableOptions.watchbutton = true;
  await Vue.nextTick();
  await Vue.nextTick();
  assert.strictEqual(redraws, initialRenders + 2, 'watch button setting immediately rebuilds the timetable');
  timetableOptions.hours = 2;
  await Vue.nextTick();
  await Vue.nextTick();
  assert.strictEqual(redraws, initialRenders + 3, 'height changes immediately rebuild the timetable');
  instance.$destroy();

  const navigation = { timetable: { Start: 100, End: 200, Week: false, ServiceList: [10] },
    options: { current: new Date(), hours: 6, broadcast: label.TSP_TS },
    station: { all: { 10: { Sid: 10, Tsp: label.TSP_TS, Parent: 0 } } },
    stationBroadcast: component.methods.stationBroadcast,
    dateorg: new Date(), tvszone: () => [4, 7, 12, 15, 18, 22],
    calcStationPage() {}, requestRedrawWithProgramScrollReset() { redraws++; } };
  const beforeNavigation = redraws;
  component.methods.changeProgramDate.call(navigation, 10, 150);
  assert.strictEqual(redraws, beforeNavigation);
  component.methods.changeProgramDate.call(navigation, 10, 250);
  assert.strictEqual(redraws, beforeNavigation + 1, 'a different time must navigate even on the same station');

  for (const [normal, multi, services, allServices, view, viewAll] of [
    [label.TSP_TS, label.TSP_TSALL, 'ts', 'tsall', 'viewTS', 'viewTSAll'],
    [label.TSP_BS, label.TSP_BSALL, 'bs', 'bsall', 'viewBS', 'viewBSAll'],
  ]) {
    const parent = { Sid: 10, Tsp: normal, Parent: 0, View: 1 };
    const child = { Sid: 11, Tsp: normal, Parent: 10, View: 1 };
    const station = { all: { 10: parent, 11: child }, ts: {}, bs: {}, cs: {}, os: {}, tsall: {}, bsall: {},
      viewTS: 0, viewBS: 0, viewCS: 0, viewOS: 0, viewTSAll: 0, viewBSAll: 0,
      [services]: { 10: parent }, [allServices]: { 10: parent, 11: child }, [view]: 1, [viewAll]: 2 };
    const target = { ...navigation, station,
      timetable: { Start: 100, End: 200, Week: false, ServiceList: ['10', '11'] },
      options: { ...navigation.options, broadcast: multi, page: Array(8).fill(0) },
      calcStationPage: component.methods.calcStationPage, displayServiceCount: () => 1 };
    const beforeSwitch = redraws;
    component.methods.changeProgramDate.call(target, '10', 150);
    assert.strictEqual(target.options.broadcast, normal, 'selecting a normal station leaves the multi view');
    assert.strictEqual(target.bcast, String(normal));
    assert.strictEqual(redraws, beforeSwitch + 1, 'a view switch redraws even within the loaded time range');
    component.methods.changeProgramDate.call(target, '11', 150);
    assert.strictEqual(target.options.broadcast, multi, 'selecting a child station opens the multi view');
    assert.strictEqual(target.options.page[multi], 1, 'the child page uses the multi station order');
    assert.strictEqual(redraws, beforeSwitch + 2);
    component.methods.changeProgramDate.call(target, '11', 150);
    assert.strictEqual(redraws, beforeSwitch + 2, 'the same view and loaded range only scroll');
  }

  const responses = [];
  const app = loadComponent('App.vue', { put: () => new Promise(resolve => responses.push(resolve)) });
  const service = { Sid: 10, View: 1 };
  const context = { timetableRequestId: 0, api: { timetable: '/timetable' }, schedule: null,
    station: { ts: { 10: service }, viewTS: 1 },
    options: { current: new Date(), hours: 6, broadcast: 1, dispservice: 1, page: [0, 0] },
    befBC: 1 };
  const oldRequest = app.methods.updateTvProgram.call(context);
  const newRequest = app.methods.updateTvProgram.call(context);
  assert.strictEqual(responses.length, 2);
  const latest = { Week: false, Sta: { 10: { Name: '局名' } }, Tvp: { 10: [program(10, 1, 0, 30)] },
    CurrentMilliseconds: Date.now() };
  responses[1]({ data: latest, status: 200 });
  await newRequest;
  responses[0]({ data: { ...latest }, status: 200 });
  await oldRequest;
  assert.strictEqual(context.timetable, latest, 'out-of-order responses cannot restore an old timetable');
  assert.strictEqual(latest.Tvp[10][0].IsRec, false, 'programs can be enriched before schedules load');

  const extensionRequests = [];
  let completeExtension;
  const scrollingApp = loadComponent('App.vue', { put: (_, params) => {
    extensionRequests.push(params);
    return new Promise(resolve => { completeExtension = resolve; });
  } });
  const hour = 3600;
  const boundary = program(10, 55, 11 * 60, 13 * 60);
  const scrolling = { ...context, timetableRequestId: 0,
    timetable: { Start: 0, End: 12 * hour, Week: false, ServiceList: [10],
      Sta: { 10: { Name: '局名' } }, Tvp: { 10: [program(10, 1, 0, 30), boundary] } } };
  let completions = 0;
  const extend = direction => scrollingApp.methods.extendTimeTable.call(scrolling,
    { direction, done: () => completions++ });
  let pendingExtension = extend(1);
  assert.strictEqual(extensionRequests[0].start, 12 * hour);
  assert.strictEqual(extensionRequests[0].hours, 4, 'only four hours are fetched');
  completeExtension({ data: { Start: 12 * hour, End: 16 * hour, Tvp: { 10: [boundary] } } });
  await pendingExtension;
  assert.strictEqual(scrolling.timetable.Start, 4 * hour, 'distant earlier rows are discarded');
  assert.strictEqual(scrolling.timetable.Tvp[10].length, 1, 'cross-boundary programs are deduplicated');
  pendingExtension = extend(-1);
  completeExtension({ data: { Start: 0, End: 4 * hour, Tvp: { 10: [] } } });
  await pendingExtension;
  assert.strictEqual(scrolling.timetable.Start, 0);
  assert.strictEqual(scrolling.timetable.End, 12 * hour, 'reverse scrolling also bounds retained rows');
  pendingExtension = extend(-1);
  const replacement = { ...scrolling.timetable };
  scrolling.timetable = replacement;
  scrolling.timetableRequestId++;
  completeExtension({ data: { Start: -4 * hour, End: 0, Tvp: {} } });
  await pendingExtension;
  assert.strictEqual(scrolling.timetable, replacement, 'navigation discards an in-flight extension');
  assert.strictEqual(completions, 3, 'every request releases its loading guard');

  const rangeRequests = [];
  const rangeApp = loadComponent('App.vue', { put: async (_, params) => {
    rangeRequests.push(params);
    return { status: 200, data: { ...replacement, Tvp: {} } };
  } });
  await rangeApp.methods.updateTvProgram.call(scrolling);
  assert.strictEqual(rangeRequests[0].start, replacement.Start, 'refresh preserves the scrolled range');
  assert.strictEqual(rangeRequests[0].hours, 12);
  await rangeApp.methods.updateTvProgram.call(scrolling, true);
  assert.strictEqual(rangeRequests[1].hours, 4, 'explicit navigation starts with a small window');
  assert.strictEqual(rangeRequests[1].start, utility.tmtoux(scrolling.options.current) - hour);

  const weeklyColumns = {};
  const weeklyExtra = {};
  for (let column = 0; column < 7; column++) {
    const offset = column * 1440;
    const crossing = program(10, 100 + column, offset + 11 * 60, offset + 13 * 60);
    weeklyColumns[column] = [crossing];
    weeklyExtra[column] = [crossing, program(10, 200 + column, offset + 14 * 60, offset + 15 * 60)];
  }
  const weeklyContext = { ...context, timetableRequestId: 0, timetable: {
    Start: 0, End: 12 * hour, Week: true, services: ['10'], Sta: { 10: { Name: '局名' } }, Tvp: weeklyColumns,
  } };
  const weeklyRequest = scrollingApp.methods.extendTimeTable.call(weeklyContext, { direction: 1, done() {} });
  assert.strictEqual(extensionRequests.at(-1).week, true);
  assert.deepStrictEqual(Array.from(extensionRequests.at(-1).services), [10], 'weekly requests send numeric station IDs accepted by the Go API');
  completeExtension({ data: { Start: 12 * hour, End: 16 * hour, Week: true, Tvp: weeklyExtra } });
  await weeklyRequest;
  for (let column = 0; column < 7; column++) {
    assert.strictEqual(weeklyContext.timetable.Tvp[column].length, 2, 'each day keeps its own programs after trimming');
    assert.strictEqual(weeklyContext.timetable.Tvp[column][1].Eid, 200 + column);
  }
  weeklyContext.options = { ...context.options, weekly: true, hours: 24, broadcast: 5 };
  weeklyContext.station = { all: { 10: service }, viewAll: 1 };
  weeklyContext.befBC = 5;
  await rangeApp.methods.updateTvProgram.call(weeklyContext, true);
  assert.strictEqual(rangeRequests.at(-1).hours, 4, 'weekly loading also ignores the old hours setting');
  assert.strictEqual(rangeRequests.at(-1).week, true);

  const broadcastRequests = [];
  const broadcastApp = loadComponent('App.vue', { put: async (_, params) => {
    broadcastRequests.push(params);
    return { status: 200, data: { Week: false, Sta: {}, Tvp: {}, CurrentMilliseconds: Date.now() } };
  } });
  const broadcastContext = { timetableRequestId: 0, topTimetableRequestId: 0,
    api: { timetable: '/timetable' }, schedule: null,
    station: { tsall: { 10: service }, viewTSAll: 1,
      bsall: { 20: { Sid: 20, View: 1 }, 21: { Sid: 21, View: 1 },
        22: { Sid: 22, View: 0 } }, viewBSAll: 2 },
    options: { current: new Date(), hours: 6, broadcast: 7, dispservice: 2,
      page: [0, 0, 0, 0, 0, 0, 0] }, befBC: 7 };
  await broadcastApp.methods.updateTvProgram.call(broadcastContext);
  await broadcastApp.methods.updateTopTvProgram.call(broadcastContext);
  assert.strictEqual(broadcastContext.options.broadcast, 7, 'BS multi remains selected');
  assert.deepStrictEqual(Array.from(broadcastRequests[0].services), [20, 21],
    'BS multi requests visible BS services using the BS count');
  assert.deepStrictEqual(Array.from(broadcastRequests[1].services), [20, 21],
    'sidebar also requests BS multi services');
  broadcastContext.options.broadcast = broadcastContext.befBC = 6;
  await broadcastApp.methods.updateTvProgram.call(broadcastContext);
  assert.deepStrictEqual(Array.from(broadcastRequests[2].services), [10, 10],
    'terrestrial multi uses the terrestrial count');
  broadcastContext.options.dispservice = 0;
  broadcastContext.$refs = { timetable: { serviceDisplayWidth: 1400 } };
  await broadcastApp.methods.updateTvProgram.call(broadcastContext);
  assert.deepStrictEqual(Array.from(broadcastRequests[3].services), Array(6).fill(10),
    'automatic display loops from the first service even when there is only one page');
  broadcastContext.station.tsall = Object.fromEntries(Array.from({ length: 10 }, (_, i) =>
    [100 + i, { Sid: 100 + i, View: 1 }]));
  broadcastContext.station.viewTSAll = 10;
  broadcastContext.options.divider = 3;
  broadcastContext.options.page[6] = 7;
  await broadcastApp.methods.updateTvProgram.call(broadcastContext);
  assert.deepStrictEqual(Array.from(broadcastRequests[4].services), [106, 107, 108, 109, 100, 101],
    'the last automatic page fills remaining columns by looping from the first visible service');

  const multiTable = new Vue({ ...component,
    propsData: { api: {}, options: { ...timetableOptions, broadcast: 7 },
      station: { viewBSAll: 2 }, timetable: null },
  });
  multiTable.renderProgram({ Start: 0, End: 3600, Week: false,
    ServiceList: [], services: [], Sta: {}, Tvp: {} });
  assert.strictEqual(multiTable.page, 1, 'old saved page arrays start BS multi on page one');
  assert.strictEqual(multiTable.maxPage, 1, 'BS multi pagination uses the BS count');
  multiTable.$destroy();

  const systemSetting = loadComponent('components/SystemSetting.vue');
  const optionEvents = [];
  const optionContext = { $emit: (event, opt) => optionEvents.push([event, opt]) };
  systemSetting.methods.optionUpdate.call(optionContext, timetableOptions);
  systemSetting.methods.optionUpdate.call(optionContext, timetableOptions, true);
  assert.strictEqual(optionEvents[0][0], 'optionchange', 'display settings do not fetch programs');
  assert.strictEqual(optionEvents[1][0], 'redrawtt', 'query settings still fetch programs');
  const settings = loadComponent('components/setting/TimeTableSetting.vue');
  const settingContext = { options: timetableOptions, $emit: (...args) => optionEvents.push(args) };
  settings.methods.changeDispService.call(settingContext, 0);
  assert.strictEqual(optionEvents[2][2], true);
  for (const hours of [2, 4, 8]) {
    settings.methods.changeHeight.call(settingContext, hours);
    assert.strictEqual(timetableOptions.hours, hours);
    assert.strictEqual(settings.computed.pixelsPerMinute.call(settingContext), hours);
    assert.strictEqual(optionEvents.at(-1)[0], 'optionchange');
    assert.strictEqual(optionEvents.at(-1).length, 2, 'height changes only update display settings');
  }

  const device = loadComponent('components/DeviceController.vue');
  for (const initialCaption of [true, false]) {
    const controller = new (Vue.extend(device))({ propsData: {
      device: [{ ID: 'D1', State: label.DSTAT_READY }],
      options: { captionon: initialCaption, autopin: false },
    } });
    assert.strictEqual(controller.captionsw.D1, initialCaption,
      'caption switches respect both initial defaults');
    const captionSettings = loadComponent('components/setting/DeviceControllerSetting.vue');
    captionSettings.methods.changeCaptionON.call({ options: controller.options, $emit() {} });
    await Vue.nextTick();
    assert.strictEqual(controller.captionsw.D1, !initialCaption,
      'changing the caption default updates an existing controller');
    controller.captionsw.D1 = initialCaption;
    controller.device = [{ ID: 'D1', State: label.DSTAT_RECORDING },
      { ID: 'D2', State: label.DSTAT_READY }];
    await Vue.nextTick();
    assert.strictEqual(controller.captionsw.D1, initialCaption,
      'device updates preserve an individual caption switch');
    assert.strictEqual(controller.captionsw.D2, !initialCaption,
      'new devices use the updated caption default');
    captionSettings.methods.changeCaptionON.call({ options: controller.options, $emit() {} });
    await Vue.nextTick();
    assert.strictEqual(controller.captionsw.D1, initialCaption);
    assert.strictEqual(controller.captionsw.D2, initialCaption,
      'changing the default back updates every caption switch');
    controller.$destroy();
  }
  const sent = [];
  const socket = { readyState: 1, send: message => sent.push(message) };
  device.methods.comSend.call({ socket }, 'device', 'command', '引用"と改行\n');
  assert.strictEqual(JSON.parse(sent[0].slice(4)).message, '引用"と改行\n');
  device.methods.reqSend.call({ socket }, 'device', 'StreamChange', 101);
  assert.strictEqual(JSON.parse(sent[1].slice(4)).message, '101', 'numeric service IDs remain strings on the wire');
  socket.readyState = 0;
  device.methods.comSend.call({ socket }, 'device', 'command', 'message');
  assert.strictEqual(sent.length, 2);

  for (const tsp of [1, 2, 3, 4, 16]) {
    const parent = (tsp << 16) | 101;
    const child = (tsp << 16) | 102;
    const commands = [];
    device.methods.changeService.call({ station: { all: { [child]: { Sid: child, Parent: parent } } },
      reqSend: (...args) => commands.push(args) }, 'device', child);
    assert.deepStrictEqual(commands, [
      ['device', 'StreamChange', (label.SUBCHANNEL_CODE << 16) | 102],
      ['device', 'StreamChange', parent],
    ], 'all broadcast types send the selected child before its configured parent');
  }

  const keywordReserve = loadComponent('components/KeywordReserve.vue');
  const keywordContext = {
    keywordReserves: [{ id: 1 }, { id: 2 }, { id: 3 }],
    schedule: [{ searchid: 2, endtime: Math.floor(utility.now() / 1000) + 3600 },
      { searchid: 1, endtime: 0 }],
  };
  keywordContext.scheduleCount = id => keywordReserve.methods.scheduleCount.call(keywordContext, id);
  keywordContext.displayedKeywordReserves = keywordReserve.computed.displayedKeywordReserves.call(keywordContext);
  assert.deepStrictEqual(Array.from(keywordContext.displayedKeywordReserves, item => item.id), [2, 1, 3],
    'keywords with future reservations precede zero-reservation keywords, preserving order within each group');
  assert.strictEqual(keywordReserve.computed.firstUnreservedKeywordIndex.call(keywordContext), 1);
  assert.deepStrictEqual(keywordContext.keywordReserves.map(item => item.id), [1, 2, 3],
    'display grouping preserves the source list');
  for (const [items, expected] of [[[], -1], [[{ id: 2 }], -1], [[{ id: 1 }, { id: 3 }], 0]]) {
    keywordContext.displayedKeywordReserves = items;
    assert.strictEqual(keywordReserve.computed.firstUnreservedKeywordIndex.call(keywordContext), expected,
      'the separator occurs only at the first zero-reservation keyword, including an entirely empty group');
  }
  assert.strictEqual(keywordReserve.computed.keywordReserveThresholdDays.call({ configure: null }), null);
  assert.strictEqual(keywordReserve.computed.keywordReserveThresholdDays.call({ configure: {} }), null);
  for (const days of [0, 1.5, 30]) {
    assert.strictEqual(keywordReserve.computed.keywordReserveThresholdDays.call({
      configure: { CurrentKeywordReserveThreshold: days * 24 * 60 * 60 },
    }), days, 'retention seconds convert to days without rounding');
  }

  const pendingRecordingStatus=[];
  const pushSchedule=loadComponent('components/RecordingSchedule.vue',{get:()=>new Promise(resolve=>pendingRecordingStatus.push(resolve))},{setInterval(){throw new Error('recording status must not poll');}});
  const pushContext={...pushSchedule.data(),api:{recordingStatus:'/rst'},$el:{querySelector:()=>null}};
  pushContext.updateRecordingStatus=pushSchedule.methods.updateRecordingStatus.bind(pushContext);
  pushContext.updateScheduleOverflow=pushSchedule.methods.updateScheduleOverflow.bind(pushContext);
  pushSchedule.mounted.call(pushContext);
  pushSchedule.watch.recordingStatusRevision.call(pushContext);
  pushSchedule.watch.recordingStatusRevision.call(pushContext);
  assert.strictEqual(pendingRecordingStatus.length,1,'notifications coalesce while a request is active');
  pendingRecordingStatus[0]({data:{recording:[1]}});
  await new Promise(resolve=>setImmediate(resolve));
  assert.strictEqual(pendingRecordingStatus.length,2,'a notification during a request is replayed after completion');
  pendingRecordingStatus[1]({data:{recording:[2]}});
  await new Promise(resolve=>setImmediate(resolve));
  assert.strictEqual(JSON.stringify(pushContext.recordingIDs),'[2]');

  const clock = loadComponent('components/DigitalClock.vue');
  const at = hour => ({ currentTime: new Date(2026, 9, 5, hour, 25, 30), twelveHour: true, twentyEight: true });
  assert.strictEqual(clock.computed.hours.call(at(0)), '12');
  assert.strictEqual(clock.computed.hours.call(at(12)), '12');
  assert.strictEqual(clock.computed.amPm.call(at(12)), 'pm');
  assert.strictEqual(clock.computed.hours.call({ ...at(1), twelveHour: false }), '25');
  assert.strictEqual(clock.computed.displayDate.call(at(1)).getDate(), 4);
  const recorderSetting = loadComponent('components/setting/RecorderSetting.vue');
  const recorderContext = { normalizeGaijiExclusion: recorderSetting.methods.normalizeGaijiExclusion };
  assert.strictEqual(recorderSetting.methods.normalize.call(recorderContext, {}).analyzetsafterrecording, false);
  assert.strictEqual(recorderSetting.methods.normalize.call(recorderContext, { analyzetsafterrecording: true }).analyzetsafterrecording, true);
  console.log('Frontend rendering, navigation, response races, device commands and clock tests: OK');
}

const watchdog = setTimeout(() => {
  console.error('Frontend review tests did not complete');
  process.exitCode = 1;
}, 10000);
main().catch(error => { console.error(error); process.exitCode = 1; }).finally(() => clearTimeout(watchdog));
