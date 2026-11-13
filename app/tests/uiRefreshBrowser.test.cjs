const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');
const compiler = require('vue-template-compiler');
const label = require('../src/define');

async function main() {
  const read = name => fs.readFileSync(path.join(__dirname, '../src', name), 'utf8');
  const sfc = compiler.parseComponent(read('components/TimeTable.vue'));
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await page.route('https://timetable.test/**', route => route.fulfill({
      contentType: 'text/html', body: '<div id="app"></div>',
    }));
    await page.goto('https://timetable.test/');
    await page.addScriptTag({ path: require.resolve('vue/dist/vue.js') });
    await page.addScriptTag({ path: require.resolve('vuetify/dist/vuetify.js') });
    await page.addStyleTag({ path: require.resolve('vuetify/dist/vuetify.min.css') });
    await page.addStyleTag({ content: read('assets/tvrock.css') + sfc.styles.map(s => s.content).join('\n') });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    const result = await page.evaluate(async sources => {
      const { Vue, Vuetify } = window;
      const load = (source, require) => new Function('require',
        'const module={exports:{}};' + source + ';return module.exports;')(require);
      const utility = load(sources.utility, () => sources.label);
      const renderer = load(sources.renderer, name => name.includes('define') ? sources.label : utility);
      const refresh = load(sources.refresh);
      const genreColors = load(sources.genreColors);
      const component = source => new Function('deps', 'require', source.replace(
        /import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g,
        (_, binding, name) => {
          const key = name.includes('define') ? 'label' : name.includes('utility') ? 'utility' :
            name.includes('timeTableRenderer') ? 'renderer' : name.includes('uiRefreshQueue') ? 'refresh' :
            name.includes('timeTableGenreColors') ? 'genreColors' : name === 'axios' ? 'axios' : null;
          return `const ${binding} = ${key ? 'deps.' + key : '{}'};`;
        }).replace('export default', 'return'));
      const requests = [];
      let delay = 0;
      const axios = { put: async (_, params) => {
        requests.push({ ...params });
        await new Promise(resolve => setTimeout(resolve, delay));
        return { status: 200, data: { Start: params.start, End: params.start + params.hours * 3600,
          Week: false, ServiceList: [10], services: ['10'], Sta: { 10: { Name: 'test' } },
          CurrentMilliseconds: Date.now(), Tvp: { 10: Array.from({ length: params.hours * 2 }, (_, i) => ({
            Sid: 10, Eid: Math.floor(params.start / 1800) + i, Start: params.start + i * 1800,
            End: params.start + (i + 1) * 1800, Title: 'test', Content: '', Cnib_a: 0, Cnib_b: 0,
          })) } } };
      } };
      const deps = { label: sources.label, utility, renderer, refresh, genreColors, axios };
      const parent = component(sources.app.match(/<script>([\s\S]*?)<\/script>/)[1])(deps, () => '');
      const table = component(sources.table)(deps, () => '');
      table.template = sources.template;
      const start = Math.floor(Date.now() / 3600000) * 3600;
      const initial = (await axios.put('/guide', { start: start - 21600, hours: 12 })).data;
      const station = { ts: { 10: { Sid: 10, View: 1 } }, all: { 10: { Sid: 10, View: 1 } }, viewTS: 1 };
      const live = new Vue({ vuetify: new Vuetify(), components: { TimeTable: table },
        data: () => ({ timetable: initial, schedule: [], befBC: 1, timetableRequestId: 0,
          station, api: { timetable: '/guide' }, options: { current: new Date(start * 1000),
            broadcast: 1, dispservice: 1, hours: 4, page: [0, 0], guidetype: 2 } }),
        methods: Object.fromEntries(['updateTvProgram', 'extendTimeTable', 'requestUiRefresh',
          'handleSocketMessage'].map(name => [name, parent.methods[name]])),
        template: '<v-app><TimeTable ref="timetable" :timetable="timetable" :options="options" :station="station" :schedule="schedule" :api="api" @extendtt="extendTimeTable" /></v-app>',
      }).$mount('#app');
      await new Promise(resolve => setTimeout(resolve, 800));
      requests.length = 0;
      delay = 500;
      live.handleSocketMessage({ data: 'utt:' });
      await Promise.resolve(); await Promise.resolve();
      const leading = requests.length;
      for (let i = 0; i < 1000; i++) live.handleSocketMessage({ data: 'utt:' });
      const burst = requests.length;
      const tableView = live.$refs.timetable;
      tableView.extensionRetryAfter = 0;
      const body = tableView.$refs.programScrollBody;
      body.scrollTop = tableView.programGridTop(body) + (live.timetable.End - live.timetable.Start) / 15 - body.clientHeight - 100;
      tableView.checkProgramScroll();
      const immediate = requests.length > burst;
      const oldEnd = live.timetable.End;
      await new Promise(resolve => setTimeout(resolve, 1800));
      const result = { leading, burst, immediate, total: requests.length, oldEnd, finalEnd: live.timetable.End };
      live._uiRefreshQueue.dispose();
      live.$destroy();
      return result;
    }, { label, utility: read('utility.js'), renderer: read('timeTableRenderer.js'),
      refresh: read('uiRefreshQueue.js'), genreColors: read('timeTableGenreColors.js'), app: read('App.vue'),
      table: sfc.script.content, template: sfc.template.content });
    assert.equal(result.leading, 1);
    assert.equal(result.burst, 1);
    assert.equal(result.immediate, true, JSON.stringify(result));
    assert(result.finalEnd > result.oldEnd, JSON.stringify(result));
    assert(result.total <= 5, JSON.stringify(result));
    assert.deepEqual(errors, []);
    console.log('Browser notification burst and immediate timetable scroll: OK', result);
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
