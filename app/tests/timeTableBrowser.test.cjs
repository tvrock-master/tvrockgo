const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');
const compiler = require('vue-template-compiler');
const label = require('../src/define');
const { timeTableGenres } = require('../src/timeTableGenreColors');

async function main() {
  const read = file => fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
  const sfc = compiler.parseComponent(read('components/TimeTable.vue'));
  const browser = await chromium.launch({ channel: 'msedge', headless: true, ignoreDefaultArgs: ['--hide-scrollbars'] });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await page.route('https://timetable.test/**', route => {
      if (route.request().url().includes('/static/cdt/')) {
        return route.fulfill({ contentType: 'image/gif', body: Buffer.from(
          'R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7', 'base64') });
      }
      return route.fulfill({ contentType: 'text/html', body: '<div id="app"></div>' });
    });
    await page.goto('https://timetable.test/');
    await page.addScriptTag({ path: require.resolve('vue/dist/vue.js') });
    await page.addScriptTag({ path: require.resolve('vuetify/dist/vuetify.js') });
    await page.addStyleTag({ path: require.resolve('vuetify/dist/vuetify.min.css') });
    await page.addStyleTag({ content: read('assets/tvrock.css') + sfc.styles.map(style => style.content).join('\n') });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    const result = await page.evaluate(async ({ script, template, rendererSource, utilitySource, refreshSource, appSource, label, timeTableGenres }) => {
      const { Vue, Vuetify } = window;
      let logoLoads = 0;
      document.addEventListener('load', event => {
        if (event.target.matches && event.target.matches('.tt-station-logo')) logoLoads++;
      }, true);
      const utility = new Function('require', 'const module = { exports: {} };' + utilitySource +
        ';return module.exports;')(() => label);
      const renderer = new Function('require', 'const module = { exports: {} };' + rendererSource +
        ';return module.exports;')(source => source.includes('define') ? label : utility);
      const refresh = new Function('const module = { exports: {} };' + refreshSource + ';return module.exports;')();
      const captionHistory = { props: ['program', 'anchor', 'api'], template: '<div class="caption-test">字幕</div>' };
      // The named renderer import is supplied explicitly after stripping imports.
      const methodsScript = script.replace(/import\s+[^;]+?\s+from\s+['"][^'"]+['"];?/g, '')
        .replace('export default', 'return');
      const timetable = new Function('label', 'utility', 'axios', 'CaptionHistoryWindow', 'timeTableGenres',
        ...Object.keys(renderer), methodsScript)(label, utility, {}, captionHistory, timeTableGenres,
        ...Object.values(renderer));
      timetable.template = template;
      const now = new Date();
      now.setMinutes(0, 0, 0);
      const start = Math.floor(now.getTime() / 1000);
      const program = { Sid: 10, Eid: 100, Start: start, End: start + 1800,
        StartM: start / 60, EndM: start / 60 + 30, StartD: now, EndD: new Date((start + 1800) * 1000),
        DurationM: 30, Title: '検証番組', Subtitle: '副題', Content: '本文', Cnib_a: 0, Cnib_b: 0 };
      const tt = { Start: start, End: start + 3600, Week: false, services: ['10'], ServiceList: [10],
        Sta: { 10: { Name: '検証局' } }, Tvp: { 10: [program] } };
      const options = { current: now, hours: 1, weekly: false, broadcast: 1, dispservice: 8,
        page: [0, 0, 0, 0, 0, 0, 0], divider: 3, displogo: true, guidetype: 2,
        subtitlemin: 0, contentmin: 0, stationsplitscroll: false, captiontimetable: true };
      const station = { all: { 10: { Sid: 10, View: 1, Tsp: 1 } },
        ts: { 10: { Sid: 10, View: 1, Tsp: 1 } }, bs: {}, cs: {}, os: {},
        viewAll: 16, viewTS: 16, viewBS: 0, viewCS: 0, viewOS: 0, viewTSAll: 16, viewBSAll: 0 };
      const app = new Vue({ vuetify: new Vuetify(), components: { TimeTable: timetable },
        data: () => ({ tt, options, station, redraws: 0, extensions: [] }),
        template: '<v-app><TimeTable ref="table" :timetable="tt" :options="options" :station="station" :schedule="[]" :api="{}" @redrawtt="redraws++" @extendtt="extensions.push($event)" /></v-app>',
      }).$mount('#app');
      const tick = async () => { await Vue.nextTick(); await Vue.nextTick(); };
      await tick();
      const check = (condition, message) => { if (!condition) throw new Error(message); };
      let cell = document.getElementById('L10_100');
      check(cell && cell.getBoundingClientRect().height === 120, '30-minute cell has the expected height');
      const pagination = document.querySelector('.v-pagination');
      check(pagination, 'pagination is visible');
      cell.click();
      await tick();
      check(cell.classList.contains('tt-program-expanded'), 'click expands details');
      cell.click();
      await tick();
      check(document.querySelector('.caption-test'), 'second click opens captions');
      cell.click();
      await tick();
      check(!document.querySelector('.caption-test'), 'third click closes captions');
      app.tt = { ...tt };
      await tick();
      check(document.querySelector('.v-pagination') === pagination, 'redraw preserves the pagination element');
      const selectedCell = document.getElementById('L10_100');
      const selectionScroll = utility.scrollToTarget('L10_100');
      await tick();
      check(selectedCell.classList.contains('selected-program-highlight'), 'selecting a program starts its highlight');
      app.tt = { ...tt, Start: tt.Start - 3600 };
      await tick();
      check(document.getElementById('L10_100') === selectedCell &&
        selectedCell.classList.contains('selected-program-highlight'),
        'a timetable update during selection must not cancel the highlight on the reused cell');
      await selectionScroll;
      check(!selectedCell.classList.contains('selected-program-highlight'), 'the highlight clears when its animation ends');
      app.tt = { ...tt };
      await tick();
      const originalBody = app.$refs.table.$refs.programScrollBody;
      check(originalBody && app.$refs.table.$refs.stationHeaderWrap, 'station headers stay fixed with the saved option disabled');
      app.options.stationsplitscroll = true;
      await tick();
      app.options.stationsplitscroll = false;
      await tick();
      const body = app.$refs.table.$refs.programScrollBody;
      check(body === originalBody && app.$refs.table.observedScrollBody === body,
        'changing the compatibility option preserves the body and its observer');
      check(app.$refs.table.$refs.stationHeaderTable.style.width === body.clientWidth + 'px', 'header width matches the body');
      const table = app.$refs.table;
      const frame = async () => {
        await tick();
        await new Promise(requestAnimationFrame);
        await new Promise(requestAnimationFrame);
        await tick();
      };
      for (const request of app.extensions.splice(0)) request.done();
      table.extensionRetryAfter = 0;
      table.resetProgramScrollOnRender = true;
      app.options.current = now;
      app.tt = { ...tt, Start: start - 21600, End: start + 21600 };
      await frame();
      check(Math.abs(body.scrollTop - table.programGridTop(body) - 1440) < 1, 'initial view starts at the selected hour');
      body.scrollTop = table.programGridTop(body) + 300;
      await frame();
      check(app.extensions.length === 1 && app.extensions[0].direction === -1,
        'scrolling near the top prefetches earlier hours: ' + JSON.stringify({ count: app.extensions.length,
          top: body.scrollTop, extending: table.extendingPrograms, start: table.renderedStart, expected: app.tt.Start }));
      const beforePrepend = document.getElementById('L10_100').getBoundingClientRect().top;
      const stableCell = document.getElementById('L10_100');
      const stableLogo = document.querySelector('#gst .tt-station-logo');
      const stableLogoContent = stableLogo.closest('.tt-grid-header').innerHTML;
      let logoMutations = 0;
      const logoObserver = new MutationObserver(records => { logoMutations += records.length; });
      logoObserver.observe(stableLogo.closest('.tt-grid-header'), { childList: true, subtree: true });
      const scrollBefore = body.scrollTop;
      const heightBefore = body.scrollHeight;
      await Promise.all(Array.from(document.querySelectorAll('.tt-station-logo'), logo => logo.decode()));
      await frame();
      const logoLoadsBefore = logoLoads;
      app.tt = { ...app.tt, Start: app.tt.Start - 14400, End: app.tt.End - 14400 };
      app.extensions.shift().done();
      await frame();
      check(Math.abs(document.getElementById('L10_100').getBoundingClientRect().top - beforePrepend) < 1,
        'prepending and trimming preserves the visible time');
      check(document.getElementById('L10_100') === stableCell, 'existing program cells are reused');
      check(document.querySelector('#gst .tt-station-logo') === stableLogo &&
        stableLogo.closest('.tt-grid-header').innerHTML === stableLogoContent && logoMutations === 0 &&
        logoLoads === logoLoadsBefore,
        'extending preserves logo nodes without loading them again');
      logoObserver.disconnect();
      check(body.scrollTop === scrollBefore && body.scrollHeight === heightBefore,
        'prepending does not move or resize the scrollbar');
      table.extensionRetryAfter = 0;
      body.scrollTop = table.programGridTop(body) + (app.tt.End - app.tt.Start) / 15 - body.clientHeight - 200;
      await frame();
      check(app.extensions.length === 1 && app.extensions[0].direction === 1, 'scrolling near the bottom prefetches later hours');
      const beforeAppend = document.getElementById('L10_100').getBoundingClientRect().top;
      const appendScrollBefore = body.scrollTop;
      app.tt = { ...app.tt, Start: app.tt.Start + 14400, End: app.tt.End + 14400 };
      app.extensions.shift().done();
      await frame();
      check(Math.abs(document.getElementById('L10_100').getBoundingClientRect().top - beforeAppend) < 1,
        'appending and trimming preserves the visible time');
      check(body.scrollTop === appendScrollBefore && body.scrollHeight === heightBefore,
        'appending does not move or resize the scrollbar');
      for (let frameIndex = 0; frameIndex < 5; frameIndex++) {
        await new Promise(requestAnimationFrame);
        check(body.scrollTop === appendScrollBefore && body.scrollHeight === heightBefore &&
          Math.abs(stableCell.getBoundingClientRect().top - beforeAppend) < 1,
          'consecutive animation frames keep the scrollbar and visible programs stable');
      }
      check(app.tt.End - app.tt.Start === 43200, 'rendered range stays bounded');
      for (const side of [true, false]) {
        Vue.set(app.options, 'sideselection', side);
        app.options.stationsplitscroll = false;
        table.resetProgramScrollOnRender = true;
        app.options.current = now;
        app.tt = { ...tt, Start: start - 21600, End: start + 21600 };
        await frame();
        const scroller = table.$refs.programScrollBody;
        check(scroller === originalBody && table.$refs.stationHeaderWrap,
          'fixed station scrolling remains enabled with side selection=' + side);
        for (const request of app.extensions.splice(0)) request.done();
        table.extensionRetryAfter = 0;
        scroller.scrollTop = table.programGridTop(scroller) + 300;
        await frame();
        check(app.extensions.length === 1 && app.extensions[0].direction === -1,
          'fixed station scrolling loads earlier programs with the option disabled, side selection=' + side);
        const before = document.getElementById('L10_100').getBoundingClientRect().top;
        app.tt = { ...app.tt, Start: app.tt.Start - 14400, End: app.tt.End - 14400 };
        app.extensions.shift().done();
        await frame();
        check(Math.abs(document.getElementById('L10_100').getBoundingClientRect().top - before) < 1,
          'fixed station scrolling preserves position with the option disabled, side selection=' + side);
      }
      app.options.stationsplitscroll = true;
      table.resetProgramScrollOnRender = true;
      app.options.current = now;
      app.tt = { ...tt, Start: start - 21600, End: start + 21600 };
      await frame();
      const tallBody = table.$refs.programScrollBody;
      tallBody.style.maxHeight = '1400px';
      for (const request of app.extensions.splice(0)) request.done();
      table.extensionRetryAfter = 0;
      tallBody.scrollTop = table.programGridTop(tallBody) + 1000;
      await frame();
      check(app.extensions.length === 1 && app.extensions[0].direction === 1 &&
        app.extensions[0].maxHours >= 16, 'tall viewports retain enough rows to avoid trimming visible programs');
      app.extensions.shift().done();
      table.extensionRetryAfter = 0;
      const beforeJump = app.redraws;
      tallBody.scrollTop = table.programGridTop(tallBody) + (app.tt.End + 86400 - app.tt.Start) / 15;
      await frame();
      check(app.redraws === beforeJump + 1 && app.extensions.length === 0,
        'dragging far away loads the destination directly');
      app.tt = { ...tt, Week: true, Tvp: { 0: [program] } };
      app.options.broadcast = 5;
      app.options.weekly = true;
      await tick();
      const before = app.redraws;
      document.querySelector('[data-week-start]').click();
      await tick();
      check(!app.options.weekly && app.options.broadcast === 1, 'weekly date header returns to a station timetable');
      check(app.redraws === before + 1, 'weekly date navigation requests data exactly once');
      app.$destroy();
      check(!window.pgClick && !window.tvpWeekly, 'global handlers are released');
      // 実際の親の取得処理と子の先読みをつなぎ、停止中に要求が循環しないか確認する。
      const requests = [];
      const network = { delay: 0 };
      let weeklyBase = 0;
      const hoverProgram = { ...program, Start: start + 3600, End: start + 5400, Content: '検証本文'.repeat(200) };
      const fakeAxios = { put: async (_, params) => {
        requests.push({ ...params });
        if (!params.services.every(service => Number.isSafeInteger(service) && service >= 0)) {
          throw new Error('services must be numeric IDs, matching the Go API []uint field');
        }
        if (requests.length > 30) throw new Error('runaway timetable requests');
        if (network.delay) await new Promise(resolve => setTimeout(resolve, network.delay));
        const dayPrograms = {};
        for (let day = 0; day < 7; day++) {
          const dayStart = params.start + day * 86400;
          const boundaryProgram = { ...hoverProgram, Eid: 200 + day,
            Start: weeklyBase + 1800 + day * 86400, End: weeklyBase + 5400 + day * 86400 };
          dayPrograms[day] = boundaryProgram.Start < dayStart + params.hours * 3600 && boundaryProgram.End > dayStart ?
            [boundaryProgram] : [];
          for (let minute = 0; minute < params.hours * 60; minute += 5) {
            const start = dayStart + minute * 60;
            dayPrograms[day].push({ ...hoverProgram, Eid: 1000 + Math.floor(start / 300) % 50000,
              Start: start, End: start + 300, Title: '追加取得番組 ' + start });
          }
        }
        return { status: 200, data: { Start: params.start, End: params.start + params.hours * 3600,
          Week: !!params.week, ServiceList: params.week ? undefined : [10], Sta: { 10: { Name: '検証局' } }, Tvp: params.week ? dayPrograms : {
            10: [
              ...(hoverProgram.Start < params.start + params.hours * 3600 && hoverProgram.End > params.start ? [hoverProgram] : []),
              ...dayPrograms[0].filter(item => item.Eid >= 1000),
            ],
          },
          CurrentMilliseconds: Date.now() } };
      } };
      const parentScript = appSource.replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g,
        (_, binding, source) => 'const ' + binding + ' = ' +
          (source.includes('utility') ? 'dependencies.utility' : source.includes('define') ? 'dependencies.label' :
            source.includes('timeTableRenderer') ? 'dependencies.renderer' :
            source.includes('uiRefreshQueue') ? 'dependencies.refresh' : source === 'axios' ? 'dependencies.axios' : '{}') + ';')
        .replace('export default', 'return');
      const parent = new Function('dependencies', 'require', parentScript)(
        { utility, label, renderer, refresh, axios: fakeAxios }, () => '');
      const mountPoint = document.createElement('div');
      document.body.appendChild(mountPoint);
      const live = new Vue({ vuetify: new Vuetify(), components: { TimeTable: timetable },
        data: () => ({ timetable: null, timetableRequestId: 0, befBC: 1,
          api: { timetable: '/tvp' }, schedule: [], options: { ...options, current: now, dispservice: 1,
            weekly: false, broadcast: 1, sideselection: false, stationsplitscroll: false, guidetype: 3 },
          station: { ...station, viewTS: 1 } }),
        methods: { updateTvProgram: parent.methods.updateTvProgram, extendTimeTable: parent.methods.extendTimeTable,
          redrawTT(opt) { if (opt) this.options = opt; this.updateTvProgram(!!opt); } },
        template: '<v-app><TimeTable ref="table" :timetable="timetable" :options="options" :station="station" :schedule="schedule" :api="api" @redrawtt="redrawTT" @extendtt="extendTimeTable" @timechange="options.current = $event" /></v-app>',
      }).$mount(mountPoint);
      await frame();
      for (let index = 0; index < 30; index++) await new Promise(requestAnimationFrame);
      check(requests.length <= 3, 'initial prefetch settles without interaction: ' + JSON.stringify(requests));
      const settled = requests.length;
      for (let index = 0; index < 30; index++) await new Promise(requestAnimationFrame);
      check(requests.length === settled, 'idle timetable does not repeatedly request /tvp');
      for (const height of [800, 1000, 1400, 1800, 2400, 5000]) {
        requests.length = 0;
        live.$refs.table.$refs.programScrollBody.style.maxHeight = height + 'px';
        live.$refs.table.requestRedrawWithProgramScrollReset({ ...live.options, current: now });
        for (let index = 0; index < 45; index++) await new Promise(requestAnimationFrame);
        check(requests.length < 10, 'prefetch settles at height ' + height + ': ' + JSON.stringify(requests));
        const liveTable = live.$refs.table;
        const liveBody = liveTable.$refs.programScrollBody;
        if (height < 4000) {
          const top = liveBody.scrollTop - liveTable.programGridTop(liveBody);
          const margin = Math.min(720, Math.max(240, liveBody.clientHeight));
          check(top >= margin && top + liveBody.clientHeight <=
            (live.timetable.End - live.timetable.Start) / 15 - margin,
            'both prefetch margins remain covered at height ' + height);
        }
        const requestCount = requests.length;
        const hoverCell = liveTable.$el.querySelector('[data-caption-key]');
        if (hoverCell) {
          for (let index = 0; index < 20; index++) {
            hoverCell.dispatchEvent(new MouseEvent('mouseenter'));
            hoverCell.dispatchEvent(new MouseEvent('mouseleave'));
          }
        }
        liveTable.checkProgramScroll();
        liveTable.checkProgramScroll();
        for (let index = 0; index < 15; index++) await new Promise(requestAnimationFrame);
        check(requests.length === requestCount, 'rechecking the same viewport cannot restart fetching at height ' + height);
      }
      const weeklyDate = new Date(now);
      weeklyDate.setHours(23, 0, 0, 0);
      weeklyBase = weeklyDate.getTime() / 1000;
      requests.length = 0;
      live.befBC = 5;
      live.$refs.table.$refs.programScrollBody.style.maxHeight = '800px';
      live.$refs.table.requestRedrawWithProgramScrollReset({ ...live.options,
        weekly: true, broadcast: 5, current: weeklyDate, hours: 24 });
      for (let index = 0; index < 45; index++) await new Promise(requestAnimationFrame);
      check(live.timetable.Week && requests.length <= 4 && requests.every(request => request.week && request.hours === 4),
        'weekly view loads small chunks and settles without using the hours setting');
      const weeklyTable = live.$refs.table;
      const weeklyBody = weeklyTable.$refs.programScrollBody;
      for (let day = 0; day < 7; day++) {
        check(weeklyTable.$el.querySelector('#L10_' + (200 + day)), 'each weekly column displays its own date');
      }
      weeklyBody.scrollTop = weeklyTable.programGridTop(weeklyBody) +
        (weeklyBase + 4500 - live.timetable.Start) / 15;
      await frame();
      const midnightScroll = weeklyBody.scrollTop;
      const midnightHeight = weeklyBody.scrollHeight;
      const headerDate = new Date(Number(weeklyTable.$el.querySelector('[data-week-start]').dataset.weekStart) * 1000);
      check(headerDate.toDateString() === new Date((weeklyBase + 4500) * 1000).toDateString(),
        'weekly headers follow the visible day across midnight');
      await frame();
      check(weeklyBody.scrollTop === midnightScroll && weeklyBody.scrollHeight === midnightHeight,
        'updating weekly dates does not move or resize the scrollbar');
      for (const direction of [-1, 1]) {
        const previousStart = live.timetable.Start;
        const previousEnd = live.timetable.End;
        weeklyBody.scrollTop = weeklyTable.programGridTop(weeklyBody) + (direction < 0 ? 300 :
          (previousEnd - previousStart) / 15 - weeklyBody.clientHeight - 200);
        const position = weeklyBody.scrollTop;
        const visibleTime = previousStart + (position - weeklyTable.programGridTop(weeklyBody)) * 15;
        const count = requests.length;
        for (let index = 0; index < 45; index++) await new Promise(requestAnimationFrame);
        check(requests.length > count && requests.length < count + 4, 'weekly scrolling extends and settles in direction ' + direction);
        check(direction < 0 ? live.timetable.Start < previousStart : live.timetable.End > previousEnd,
          'weekly view extends its time range in direction ' + direction);
        const addedProgram = live.timetable.Tvp[0].find(program =>
          direction < 0 ? program.Start < previousStart : program.Start >= previousEnd);
        check(addedProgram && weeklyTable.$el.querySelector('[data-grid-key="program_0_' + addedProgram.Sid +
          '_' + addedProgram.Eid + '_' + addedProgram.Start + '"]'),
          'newly fetched weekly programs outside the old range are actually rendered in direction ' + direction);
        const afterTime = live.timetable.Start + (weeklyBody.scrollTop - weeklyTable.programGridTop(weeklyBody)) * 15;
        check(Math.abs(afterTime - visibleTime) < 15 && weeklyBody.scrollTop === position &&
          weeklyBody.scrollHeight === midnightHeight, 'weekly extensions preserve the time and scrollbar');
      }
      const weeklySettled = requests.length;
      for (let index = 0; index < 30; index++) await new Promise(requestAnimationFrame);
      check(requests.length === weeklySettled, 'weekly view does not fetch while idle');
      live.befBC = 1;
      weeklyTable.$el.querySelector('[data-week-start]').click();
      for (let index = 0; index < 30; index++) await new Promise(requestAnimationFrame);
      check(!live.options.weekly && !live.timetable.Week, 'weekly header returns to a normal timetable after scrolling');
      window.timetableInteractionTest = { live, requests, network };
      return true;
    }, { script: sfc.script.content, template: sfc.template.content,
      rendererSource: read('timeTableRenderer.js'), utilitySource: read('utility.js'), refreshSource: read('uiRefreshQueue.js'),
      appSource: compiler.parseComponent(read('App.vue')).script.content, label, timeTableGenres });
    assert.strictEqual(result, true);
    const scrollBody = page.locator('.tt-scroll-body').last();
    await scrollBody.scrollIntoViewIfNeeded();
    await page.evaluate(async () => {
      const { live, requests } = window.timetableInteractionTest;
      requests.length = 0;
      live.$refs.table.$refs.programScrollBody.style.maxHeight = '600px';
      const current = new Date();
      current.setMinutes(0, 0, 0);
      live.$refs.table.requestRedrawWithProgramScrollReset({ ...live.options, current });
      for (let index = 0; index < 20; index++) await new Promise(requestAnimationFrame);
    });
    const position = () => page.evaluate(() => {
      const { live } = window.timetableInteractionTest;
      const table = live.$refs.table;
      const body = table.$refs.programScrollBody;
      const rect = body.getBoundingClientRect();
      const thumbHeight = Math.max(64, body.clientHeight * body.clientHeight / body.scrollHeight);
      return { time: live.timetable.Start + (body.scrollTop - table.programGridTop(body)) * 15,
        origin: table.virtualStart, height: body.clientHeight, scrollHeight: body.scrollHeight,
        thumbHeight, x: rect.right - 8,
        y: rect.top + body.scrollTop / (body.scrollHeight - body.clientHeight) * (body.clientHeight - thumbHeight) + thumbHeight / 2 };
    });
    const beforeSmallDrag = await position();
    await page.mouse.move(beforeSmallDrag.x, beforeSmallDrag.y);
    await page.mouse.down();
    await page.mouse.move(beforeSmallDrag.x, beforeSmallDrag.y + 12, { steps: 4 });
    await page.waitForTimeout(250);
    await page.mouse.up();
    await page.waitForTimeout(200);
    const afterSmallDrag = await position();
    assert(afterSmallDrag.time - beforeSmallDrag.time > 1800 && afterSmallDrag.time - beforeSmallDrag.time < 7200,
      'a small thumb drag moves about an hour instead of jumping across days');
    await page.evaluate(() => { window.timetableInteractionTest.network.delay = 600; });
    await page.mouse.move(afterSmallDrag.x, afterSmallDrag.y);
    await page.mouse.down();
    await page.mouse.move(afterSmallDrag.x, afterSmallDrag.y + 210);
    await page.waitForTimeout(100);
    const duringLargeDrag = await position();
    assert.strictEqual(duringLargeDrag.origin, afterSmallDrag.origin, 'loading during a drag never recenters the thumb');
    const expectedTime = afterSmallDrag.time + 210 *
      (afterSmallDrag.scrollHeight - afterSmallDrag.height) / (afterSmallDrag.height - afterSmallDrag.thumbHeight) * 15;
    assert(Math.abs(duringLargeDrag.time - expectedTime) < 120, 'loading preserves the dragged time without rounding to an hour');
    await page.mouse.up();
    await page.waitForTimeout(1000);
    const afterLargeDrag = await position();
    assert(Math.abs(afterLargeDrag.time - duringLargeDrag.time) < 15, 'ending a drag keeps the same visible time');
    await page.evaluate(() => { window.timetableInteractionTest.network.delay = 0; });
    const rect = await scrollBody.boundingBox();
    await page.mouse.move(rect.x + 100, Math.max(20, rect.y + 100));
    const beforeWheel = await position();
    await page.mouse.wheel(0, 240);
    await page.waitForTimeout(350);
    const afterWheel = await position();
    assert(Math.abs(afterWheel.time - beforeWheel.time - 3600) < 60,
      'wheel movement stays at four pixels per minute: ' + JSON.stringify({ beforeWheel, afterWheel, rect }));
    for (const hours of [8, 8, 2, 2, 4]) {
      const boundary = await page.evaluate(async hours => {
        const { live, network, requests } = window.timetableInteractionTest;
        requests.length = 0;
        const table = live.$refs.table;
        const body = table.$refs.programScrollBody;
        const beforeTime = live.timetable.Start + (body.scrollTop - table.programGridTop(body)) *
          60 / table.renderedPixelsPerMinute;
        table.$el.style.display = 'none';
        live.options.hours = hours;
        for (let index = 0; index < 5; index++) await new Promise(requestAnimationFrame);
        table.$el.style.display = '';
        table.syncStationHeaderLayout();
        network.delay = 200;
        for (let index = 0; index < 15; index++) await new Promise(requestAnimationFrame);
        const afterTime = live.timetable.Start + (body.scrollTop - table.programGridTop(body)) * 60 / hours;
        if (Math.abs(afterTime - beforeTime) > 15) {
          throw new Error('changing height in a hidden settings tab preserves visible time: ' +
            JSON.stringify({ hours, beforeTime, afterTime }));
        }
        body.style.maxHeight = hours === 2 ? '1200px' : '600px';
        const before = requests.length;
        body.scrollTop = body.scrollHeight;
        await new Promise(resolve => setTimeout(resolve, 100));
        // 外側のページのスクロールが、読み込み後に端の座標を補充する予定を消さない。
        window.dispatchEvent(new Event('scroll'));
        await new Promise(resolve => setTimeout(resolve, 800));
        for (let index = 0; index < 30; index++) await new Promise(requestAnimationFrame);
        const visible = live.timetable.Start + (body.scrollTop - table.programGridTop(body)) * 60 / hours;
        network.delay = 0;
        return { hours, viewportHeight: body.clientHeight, requested: requests.length - before, visible,
          start: live.timetable.Start, end: live.timetable.End, reset: table.resetProgramScrollOnRender,
          scrollTop: body.scrollTop, maxScroll: body.scrollHeight - body.clientHeight };
      }, hours);
      assert(boundary.requested > 0 && boundary.visible >= boundary.start && boundary.visible < boundary.end &&
        boundary.scrollTop < boundary.maxScroll - 100 && !boundary.reset,
        'reaching the bottom loads programs and restores room to continue scrolling: ' + JSON.stringify(boundary));
    }
    await page.evaluate(() => window.timetableInteractionTest.live.$destroy());
    assert.deepStrictEqual(errors, []);
    console.log('Timetable layout, details/captions, fixed station scrolling with legacy options, drag/wheel and weekly navigation: OK');
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
