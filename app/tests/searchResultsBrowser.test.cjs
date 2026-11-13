const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { chromium } = require('playwright');
const compiler = require('vue-template-compiler');
const { compileStyle } = require('@vue/component-compiler-utils');

async function main() {
  const read = name => fs.readFileSync(path.join(__dirname, '../src', name), 'utf8');
  const resultComponent = compiler.parseComponent(read('components/SearchResults.vue'));
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
    await page.route('https://search.test/**', route => route.fulfill({ contentType: 'text/html', body: '<div id="app"></div>' }));
    await page.goto('https://search.test/');
    await page.addScriptTag({ path: require.resolve('vue/dist/vue.js') });
    await page.addScriptTag({ path: require.resolve('vuetify/dist/vuetify.js') });
    await page.addStyleTag({ path: require.resolve('vuetify/dist/vuetify.min.css') });
    await page.addStyleTag({ content: resultComponent.styles.map(s => compileStyle({
      source: s.content, filename: 'SearchResults.vue', id: 'data-v-search-test', scoped: s.scoped,
    }).code).join('\n') });
    const errors = [];
    page.on('pageerror', error => errors.push(String(error)));
    const result = await page.evaluate(async sources => {
      const { Vue, Vuetify } = window;
      const load = (source, require) => new Function('require', 'const module={exports:{}};' + source + ';return module.exports;')(require);
      const utility = load(sources.utility, () => sources.label);
      const renderer = load(sources.renderer, name => name.includes('define') ? sources.label : utility);
      const searchResults = load(sources.stream);
      const axios = { post: async () => ({ status: 200 }) };
      const deps = { label: sources.label, utility, renderer, searchResults, axios };
      const component = source => new Function('deps', 'require', source.replace(
        /import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g, (_, binding, name) => {
          const key = name.includes('define') ? 'label' : name.includes('utility') ? 'utility' :
            name.includes('timeTableRenderer') ? 'renderer' : name.includes('searchResults') ? 'searchResults' : name === 'axios' ? 'axios' : null;
          return `const ${binding} = ${key ? 'deps.' + key : '{}'};`;
        }).replace('export default', 'return'))(deps, () => '');
      const parent = component(sources.app.match(/<script>([\s\S]*?)<\/script>/)[1]);
      const results = component(sources.script);
      results._scopeId = 'data-v-search-test';
      results.template = sources.template;
      const view = new Vue({ vuetify: new Vuetify(), components: { SearchResults: results },
        data: () => ({ searchResults: Object.freeze([]), searchProgress: {}, clicks: 0,
          schedule: Array.from({ length: 10000 }, (_, i) => ({ sid: 202, eid: i, validate: true })),
          station: { all: { 101: { Sid: 101, Name: 'test station', View: 1 } } } }),
        methods: Object.fromEntries(['beginSearchResults', 'handleSocketMessage'].map(name => [name, parent.methods[name]])),
        template: '<v-app><button id="heartbeat" @click="clicks++">操作</button><SearchResults ref="results" :search-results="searchResults" :search-progress="searchProgress" :search-condition="{}" :schedule="schedule" :station="station" :options="{watchbutton:true}" :configure="{CustomCommand:[]}" :api="{schedule: \'/schedule\'}" /></v-app>',
      }).$mount('#app');
      let frames = 0, maxRows = 0, active = true;
      const frame = () => { if (!active) return; frames++; maxRows = Math.max(maxRows, document.querySelectorAll('.search-result-row').length); requestAnimationFrame(frame); };
      requestAnimationFrame(frame);
      const program = i => ({ Sid: 101, Eid: i, Start: 1800000000 + i * 1800, End: 1800001800 + i * 1800, Title: 'program ' + i });
      const message = data => view.handleSocketMessage({ data: 'res:' + JSON.stringify({ requestId: 'large', data }) });
      view.beginSearchResults('large');
      const started = performance.now();
      let clickedDuringSearch = false;
      for (let i = 0; i < 10000; i += 200) {
        message(Array.from({ length: 200 }, (_, j) => program(i + j)));
        if (i === 5000) {
          document.getElementById('heartbeat').click();
          await Vue.nextTick();
          clickedDuringSearch = view.clicks === 1 && view.searchProgress.running;
        }
        await new Promise(resolve => setTimeout(resolve, 0));
      }
      message({ hits: 10000 });
      const waitingSince = performance.now();
      while (view.searchProgress.running) {
        if (performance.now() - waitingSince > 15000) throw new Error('search did not finish: ' + view.searchResults.length);
        await new Promise(resolve => setTimeout(resolve, 20));
      }
      await Vue.nextTick();
      const child = view.$refs.results;
      const watchButton = document.querySelector('.search-result-watch-button');
      const watchBackground = getComputedStyle(watchButton).backgroundColor;
      document.documentElement.style.setProperty('--tt-watch-button', '#71602e');
      const darkWatchBackground = getComputedStyle(watchButton).backgroundColor;
      document.documentElement.style.removeProperty('--tt-watch-button');
      const firstRows = document.querySelectorAll('.search-result-row').length;
      child.resultPage = 100;
      await Vue.nextTick(); await Vue.nextTick();
      const lastRow = [...document.querySelectorAll('.search-result-row')].pop().textContent;
      child.resultPage = 1;
      await Vue.nextTick();
      child.addProgramReserve(child.displaySearchResults[0], 0);
      await new Promise(resolve => setTimeout(resolve, 10));
      await Vue.nextTick();
      const reserved = document.querySelector('.search-result-row').classList.contains('search-result-recording');
      const outcome = { hits: view.searchResults.length, frames, maxRows, firstRows, lastRow, reserved,
        watchBackground, darkWatchBackground,
        clickedDuringSearch, elapsedMs: Math.round(performance.now() - started), receivingMs: Math.round(waitingSince - started) };
      active = false; view._searchResultStream.dispose(); view.$destroy();
      return outcome;
    }, { label: require('../src/define'), utility: read('utility.js'), renderer: read('timeTableRenderer.js'),
      stream: read('searchResults.js'), app: read('App.vue'), script: resultComponent.script.content, template: resultComponent.template.content });
    assert.equal(result.hits, 10000);
    assert.equal(result.firstRows, 100);
    assert(result.maxRows <= 100);
    assert(result.frames >= 5);
    assert(result.clickedDuringSearch);
    assert(result.elapsedMs < 15000, 'large searches must not repeatedly traverse every reservation per row');
    assert(result.lastRow.includes('program 9999'));
    assert(result.reserved, 'reservation mark must update even on shallow result arrays');
    assert.equal(result.watchBackground, 'rgba(245, 215, 122, 0.94)');
    assert.equal(result.darkWatchBackground, 'rgb(113, 96, 46)');
    assert.deepEqual(errors, []);
    console.log('Search results browser test: OK', result);
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
