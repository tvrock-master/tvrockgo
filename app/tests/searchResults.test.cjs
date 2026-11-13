const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const Vue = require('vue');
const stream = require('../src/searchResults');

function load(file, axios = {}) {
  const deps = { axios, utility: require('../src/utility'), label: require('../src/define'),
    timeTableRenderer: require('../src/timeTableRenderer'), timeTableGenreColors: require('../src/timeTableGenreColors'),
    searchResults: stream };
  let source = fs.readFileSync(path.join(__dirname, '../src', file), 'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
  source = source.replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g, (_, binding, name) => {
    const dependency = Object.keys(deps).find(key => name.includes(key)) || (name.includes('define') ? 'label' : null);
    return `const ${binding} = ${dependency ? 'deps.' + dependency : '{}'};`;
  }).replace('export default', 'module.exports =');
  const context = { module: { exports: {} }, deps, require: () => '', window: { location: { origin: 'http://test' } },
    document: { cookie: '' }, setTimeout, clearTimeout, setInterval, clearInterval };
  vm.runInNewContext(source, context);
  return context.module.exports;
}
const program = i => ({ Sid: 101, Eid: i, Start: 1800000000 + i * 1800, End: 1800001800 + i * 1800, Title: 'test' });

test('weekday masks survive search, reservation editing, re-search and reset', async () => {
  const utility = require('../src/utility');
  for (let mask = 0; mask < 128; mask++) {
    assert.equal(utility.weekListToMask(utility.weekMaskToList(mask)), mask);
  }
  assert.equal(utility.weekListToMask(null), 0, 'clearing the selection enables all weekdays');
  assert.equal(utility.weekOptions[0].text, '日曜日');
  assert.equal(utility.weekOptions[6].text, '土曜日');

  const requests = [];
  const axios = {
    post: async (url, body) => { requests.push([url, body && JSON.parse(body)]); return { status: 200, data: { id: 1 } }; },
    put: async (url, body) => { requests.push([url, JSON.parse(body)]); return { status: 200, data: { id: 1 } }; },
    get: async () => ({ data: [] }),
  };
  const searchComponent = load('components/ProgramSearch.vue', axios);
  const search = new Vue({ ...searchComponent, propsData: { api: { search: '/search' }, UID: 'test' } });
  const resultComponent = load('components/SearchResults.vue', axios);
  const result = new Vue({ ...resultComponent, propsData: {
    api: { keyword: '/keyword' }, searchCondition: { title: 'test', week: 65 },
    searchResults: [], configure: { CustomCommand: [] },
  } });
  assert.deepEqual(Array.from(result.selectedWeekdays), [0, 6]);
  result.selectedWeekdays = [1, 3, 5];
  const option = result.currentSearchOption();
  assert.equal(option.week, 42);
  search.changeSearchOption(option);
  await Vue.nextTick();
  await search._searchSubmission;
  assert.equal(requests.at(-1)[1].week, 42, 'search API receives the selected weekdays');

  result.addKeywordReserve();
  for (let i = 0; i < 20; i++) await Promise.resolve();
  assert.equal(requests.find(([url]) => url === '/keyword')[1].week, 42);
  result.searchID = 1;
  result.saveKeywordReserve();
  for (let i = 0; i < 20; i++) await Promise.resolve();
  assert.equal(requests.at(-1)[1].week, 42, 'saving from search preserves the mask');

  const keywordComponent = load('components/KeywordReserve.vue', axios);
  const keyword = new Vue({ ...keywordComponent, propsData: { api: { keyword: '/keyword' } } });
  const item = { id: 1, title: 'test', sid: [], category: 0xffff, week: 42 };
  keyword.keywordReserves = [item];
  keyword.toggleDetail(1);
  assert.deepEqual(Array.from(keyword.weekMaskToList(item.week)), [1, 3, 5]);
  keyword.$set(item, 'week', keyword.weekListToMask([0, 6]));
  keyword.saveReserve(item);
  for (let i = 0; i < 20; i++) await Promise.resolve();
  assert.equal(requests.at(-1)[1].week, 65, 'detail edits are saved as a mask');
  keyword.changeValidate(item);
  for (let i = 0; i < 20; i++) await Promise.resolve();
  assert.equal(requests.at(-1)[1].week, 65, 'toggling validity preserves weekdays');

  const app = load('App.vue');
  const state = {};
  app.methods.searchKeywordReserve.call(state, item);
  assert.equal(state.keywordSearchCondition.week, 65);
  search.applyKeywordSearchCondition(state.keywordSearchCondition);
  await Vue.nextTick();
  await search._searchSubmission;
  assert.equal(requests.at(-1)[1].week, 65, 're-search uses the saved weekdays');
  search.resetSearchOption();
  await Vue.nextTick();
  await search._searchSubmission;
  assert.equal(requests.at(-1)[1].week, 0);
  resultComponent.watch.searchCondition.handler.call(result, search.searchCondition);
  assert.equal(result.selectedWeekdays.length, 0);
  assert.equal(result.currentSearchOption().week, 0);
  result.$destroy();
  search.$destroy();
  keyword.$destroy();
});

test('10,000 results yield to other tasks, deduplicate, publish in batches and remain pageable', async () => {
  let ticks = 0, publications = 0, rows;
  const publishedAt = [];
  const heartbeat = setInterval(() => ticks++, 0);
  try {
    await new Promise(resolve => {
      const queue = stream.create({ publish: value => { rows = value; publications++; publishedAt.push(Date.now()); }, complete: count => {
        assert.equal(count, 10000); resolve();
      } });
      queue.receive(Array.from({ length: 10000 }, (_, i) => program(9999 - i)));
      queue.receive(Array.from({ length: 1000 }, (_, i) => program(i)));
      queue.finish();
      assert.equal(publications, 0, 'receiving does not synchronously process the entire payload');
    });
    assert(ticks >= 5, 'the event loop continues while processing results');
    for (let i = 1; i < publishedAt.length - 1; i++) {
      assert(publishedAt[i] - publishedAt[i - 1] >= 95, 'intermediate publications are throttled');
    }
    assert.equal(rows.length, 10000);
    assert.equal(rows[0].Eid, 0);
    const component = load('components/SearchResults.vue');
    const view = new Vue({ ...component, propsData: { searchResults: rows, schedule: [],
      options: {}, configure: { CustomCommand: [] }, station: { all: { 101: { Name: 'station' } } } } });
    assert.equal(view.displaySearchResults.length, 100);
    assert.equal(view.pageCount, 100);
    assert.equal(rows[100].DateW, undefined, 'off-page results are not formatted');
    assert.equal(rows[0].__ob__, undefined, 'results are not deeply observed');
    view.resultPage = 100;
    assert.equal(view.displaySearchResults[99].Eid, 9999);
    assert.equal(view.programKey(rows[0], 1), view.programKey(rows[0], 999));
    view.$destroy();
  } finally { clearInterval(heartbeat); }
});

test('replacing a search discards staged old rows and late responses', async () => {
  const component = load('App.vue');
  const state = component.data();
  for (const [key, method] of Object.entries(component.methods)) state[key] = method.bind(state);
  const message = (id, data, prefix = 'res:') => state.handleSocketMessage({ data: prefix + JSON.stringify({ requestId: id, data }) });
  state.beginSearchResults('old');
  message('old', Array.from({ length: 10000 }, (_, i) => program(i)));
  state.beginSearchResults('new');
  message('old', null, 'rss:');
  message('old', { hits: 10000 });
  message('new', [program(42), program(42)]);
  message('new', { hits: 2 });
  await new Promise(resolve => setTimeout(resolve, 30));
  assert.equal(state.searchResults.length, 1);
  assert.equal(state.searchResults[0].Eid, 42);
  assert.equal(state.searchProgress.running, false);
  assert.equal(state.searchProgress.hits, 1);
  state._searchResultStream.dispose();
});

test('search registrations stay ordered and unsent obsolete conditions are skipped', async () => {
  const posts = [], events = [];
  const component = load('components/ProgramSearch.vue', { post: (_, payload) => new Promise(resolve => posts.push({ payload: JSON.parse(payload), resolve })) });
  const state = { ...component.data(), UID: 'test', searchCondition: { title: 'first' }, api: { search: '/search' },
    $emit: (name, value) => events.push([name, value]) };
  component.methods.changeWord.call(state);
  await Promise.resolve();
  component.methods.changeWord.call(state);
  state.searchCondition.title = 'latest';
  component.methods.changeWord.call(state);
  assert.equal(posts.length, 1);
  posts[0].resolve();
  for (let i = 0; i < 20; i++) await Promise.resolve();
  assert.equal(posts.length, 2);
  assert.equal(posts[1].payload.title, 'latest');
  assert.equal(posts[1].payload.requestId, events[2][1]);
  posts[1].resolve();
  await state._searchSubmission;
});

test('reservation flags use an index and locally added marks survive page changes', async () => {
  const component = load('components/SearchResults.vue', { post: async () => ({ status: 200 }) });
  const rows = Object.freeze(Array.from({ length: 200 }, (_, i) => program(i)));
  const view = new Vue({ ...component, propsData: { searchResults: rows, api: { schedule: '/schedule' },
    schedule: [{ sid: 101, eid: 150, validate: true, deviceid: 'REC01' }], configure: { CustomCommand: [] }, options: {}, station: { all: {} } } });
  assert.equal(view.recordingInfo(rows[150]).deviceID, 'REC01');
  view.addProgramReserve(rows[0], 0, true);
  await new Promise(resolve => setTimeout(resolve, 0));
  view.resultPage = 2; void view.displaySearchResults;
  view.resultPage = 1;
  assert.equal(view.recordingInfo(view.displaySearchResults[0]).isWatch, true);
  view.$destroy();
});
