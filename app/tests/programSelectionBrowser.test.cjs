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
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
    await page.setContent('<div id="app"></div>');
    await page.addScriptTag({ path: require.resolve('vue/dist/vue.js') });
    await page.addScriptTag({ path: require.resolve('vuetify/dist/vuetify.js') });
    await page.addStyleTag({ path: require.resolve('vuetify/dist/vuetify.min.css') });
    await page.addStyleTag({ content: read('assets/tvrock.css') + sfc.styles.map(style => style.content).join('\n') });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    await page.evaluate(async ({ script, template, appScript, utilitySource, rendererSource, refreshSource, label, timeTableGenres }) => {
      const utility = new Function('require', 'const module = { exports: {} };' + utilitySource +
        ';return module.exports;')(() => label);
      const renderer = new Function('require', 'const module = { exports: {} };' + rendererSource +
        ';return module.exports;')(source => source.includes('define') ? label : utility);
      const refresh = new Function('const module = { exports: {} };' + refreshSource + ';return module.exports;')();
      const tableScript = script.replace(/import\s+[^;]+?\s+from\s+['"][^'"]+['"];?/g, '')
        .replace('export default', 'return');
      const table = new Function('label', 'utility', 'axios', 'CaptionHistoryWindow', 'timeTableGenres',
        ...Object.keys(renderer), tableScript)(label, utility, {}, {}, timeTableGenres, ...Object.values(renderer));
      table.template = template;
      const parentScript = appScript.replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g,
        (_, binding, source) => 'const ' + binding + ' = ' +
          (source.includes('utility') ? 'dependencies.utility' : source.includes('define') ? 'dependencies.label' :
            source.includes('timeTableRenderer') ? 'dependencies.renderer' :
              source.includes('uiRefreshQueue') ? 'dependencies.refresh' : '{}') + ';')
        .replace('export default', 'return');
      const parent = new Function('dependencies', 'require', parentScript)(
        { utility, label, renderer, refresh }, () => '');
      const now = new Date();
      now.setMinutes(0, 0, 0);
      const start = now.getTime() / 1000;
      const programs = Array.from({ length: 24 }, (_, index) => {
        const time = start + index * 1800;
        return { Sid: 10, Eid: 100 + index, Start: time, End: time + 1800,
          StartM: time / 60, EndM: time / 60 + 30, StartD: new Date(time * 1000),
          EndD: new Date((time + 1800) * 1000), DurationM: 30,
          Title: '検証番組' + index, Subtitle: '', Content: '', Cnib_a: 0, Cnib_b: 0 };
      });
      const station = { Sid: 10, Tsp: 1, Parent: 0, View: 1, Name: '検証局' };
      const app = new Vue({ vuetify: new Vuetify(), components: { TimeTable: table },
        data: () => ({ maintab: 0, redraws: 0,
          timetable: { Start: start, End: start + 43200, Week: false, services: [10],
            ServiceList: [10], Sta: { 10: station }, Tvp: { 10: programs } },
          station: { all: { 10: station }, ts: { 10: station }, bs: {}, cs: {}, os: {},
            tsall: { 10: station }, bsall: {}, viewAll: 1, viewTS: 1, viewBS: 0,
            viewCS: 0, viewOS: 0, viewTSAll: 1, viewBSAll: 0 },
          options: { current: new Date((start + 10800) * 1000), broadcast: 1, weekly: false,
            page: Array(8).fill(0), hours: 4, dispservice: 1, divider: 3,
            guidetype: 2, subtitlemin: 0, contentmin: 0 } }),
        methods: { selectProgram: parent.methods.selectProgram },
        template: '<v-app><v-tabs-items v-model="maintab"><v-tab-item :transition="false">' +
          '<TimeTable ref="timetable" :timetable="timetable" :options="options" :station="station" ' +
          ':schedule="[]" :api="{}" @redrawtt="redraws++" /></v-tab-item>' +
          '<v-tab-item :transition="false"><div>番組検索</div></v-tab-item></v-tabs-items></v-app>',
      }).$mount('#app');
      const frames = async () => {
        await Vue.nextTick();
        for (let i = 0; i < 5; i++) await new Promise(requestAnimationFrame);
      };
      await frames();
      const component = app.$refs.timetable;
      const body = component.programScrollElement();
      // 確認対象のDOMを維持し、先読み通信を伴わない同じページ内の移動を検証する。
      component.extensionRetryAfter = Date.now() + 60000;
      const baseline = app.redraws;
      for (const hideTab of [true, false]) {
        // 先に別番組を選び、その上にある番組の先頭を見切れさせる。
        await app.selectProgram({ sid: 10, eid: 108, starttime: programs[8].Start });
        await new Promise(resolve => setTimeout(resolve, 700));
        const target = document.getElementById('L10_106');
        body.scrollTop = component.programGridTop(body) + 6 * 120 + 25;
        await frames();
        if (target.getBoundingClientRect().top >= body.getBoundingClientRect().top) {
          throw new Error('fixture must start with the target clipped above the viewport');
        }
        if (hideTab) { app.maintab = 1; await frames(); }
        await app.selectProgram({ sid: 10, eid: 106, starttime: programs[6].Start });
        await new Promise(resolve => setTimeout(resolve, 700));
        await frames();
        const offset = target.getBoundingClientRect().top - body.getBoundingClientRect().top - body.clientTop;
        if (Math.abs(offset) > 2 || !body.clientHeight || app.maintab !== 0) {
          throw new Error('selected program must align with the visible top: ' + JSON.stringify({ hideTab, offset }));
        }
        if (app.redraws !== baseline) throw new Error('same-page selection must not reload the timetable');
      }
      app.$destroy();
    }, { script: sfc.script.content, template: sfc.template.content,
      appScript: compiler.parseComponent(read('App.vue')).script.content,
      utilitySource: read('utility.js'), rendererSource: read('timeTableRenderer.js'),
      refreshSource: read('uiRefreshQueue.js'), label, timeTableGenres });
    assert.deepStrictEqual(errors, []);
    console.log('Program selection: clipped targets scroll into view in the same page and after switching tabs: OK');
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
