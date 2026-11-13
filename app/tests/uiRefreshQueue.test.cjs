const assert = require('node:assert/strict');
const { test } = require('node:test');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const refresh = require('../src/uiRefreshQueue');
const utility = require('../src/utility');
const label = require('../src/define');
const renderer = require('../src/timeTableRenderer');

const settle = async () => { for (let i = 0; i < 30; i++) await Promise.resolve(); };
function clock() {
  let time = 0, next = 0;
  const timers = new Map();
  return {
    now: () => time,
    setTimer(fn, delay) { timers.set(++next, { at: time + delay, fn }); return next; },
    clearTimer(id) { timers.delete(id); },
    async advance(ms) {
      time += ms;
      for (const [id, timer] of [...timers]) {
        if (timer.at <= time) { timers.delete(id); timer.fn(); await settle(); }
      }
      await settle();
    },
  };
}

test('notification bursts have a leading refresh, one active request and bounded trailing refresh', async () => {
  const time = clock(), runs = [], waiting = [];
  const queue = refresh.create({ ...time, run: flags => {
    runs.push([...flags]); return new Promise(resolve => waiting.push(resolve));
  } });
  queue.request('guide');
  await settle();
  assert.equal(runs.length, 1);
  for (let i = 0; i < 1000; i++) queue.request('schedule', 'guide');
  assert.equal(runs.length, 1);
  waiting.shift()(); await settle();
  await time.advance(900);
  queue.request('guide'); // ここで次の実行時刻を後ろへずらしてはいけない。
  await time.advance(100);
  assert.deepEqual(runs[1].sort(), ['guide', 'schedule']);
  queue.request('guide');
  await time.advance(5000);
  assert.equal(runs.length, 2, 'an active HTTP request is never overlapped by another notification');
  waiting.shift()(); await settle();
  assert.equal(runs.length, 3, 'slow responses still permit the accumulated refresh to run');
  queue.dispose(); waiting.shift()(); await settle();
  queue.request('guide'); await time.advance(10000);
  assert.equal(runs.length, 3);
});

test('refresh failures do not leave the queue stuck', async () => {
  const time = clock(), errors = [];
  let runs = 0;
  const queue = refresh.create({ ...time, onError: error => errors.push(error), run: () => {
    if (++runs === 1) throw new Error('temporary failure');
  } });
  queue.request('guide'); await settle();
  queue.request('guide'); await time.advance(1000);
  assert.equal(errors.length, 1); assert.equal(runs, 2);
  queue.dispose();
});

function fixture() {
  const start = Math.floor(Date.now() / 3600000) * 3600;
  return { Start: start, End: start + 14400, Week: false, ServiceList: [10], services: ['10'],
    Sta: { 10: { Name: 'test' } }, CurrentMilliseconds: Date.now(),
    Tvp: { 10: [{ Sid: 10, Eid: 1, Start: start, End: start + 1800, Title: 'program',
      ID: 0, IsRec: false, IsWatch: false, DeviceID: '', SearchID: 0 }] } };
}

function appContext(axios, time) {
  const dependencies = { label, utility, axios, renderer, genreColors: require('../src/timeTableGenreColors'), refresh: {
    ...refresh, create: options => refresh.create({ ...options, ...time }),
  } };
  let script = fs.readFileSync(path.join(__dirname, '../src/App.vue'), 'utf8').match(/<script>([\s\S]*?)<\/script>/)[1];
  script = script.replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g, (_, binding, source) => {
    const name = source.includes('uiRefreshQueue') ? 'refresh' : source.includes('timeTableRenderer') ? 'renderer' :
      source.includes('utility') ? 'utility' : source.includes('define') ? 'label' :
        source.includes('timeTableGenreColors') ? 'genreColors' : source === 'axios' ? 'axios' : null;
    return `const ${binding} = ${name ? 'dependencies.' + name : '{}'};`;
  }).replace('export default', 'module.exports =');
  const sandbox = { module: { exports: {} }, dependencies, require: () => '',
    window: { location: { origin: 'http://test' } }, document: { cookie: '' },
    setTimeout, clearTimeout, setInterval, clearInterval };
  vm.runInNewContext(script, sandbox);
  const component = sandbox.module.exports;
  const state = component.data();
  Object.assign(state, { timetable: fixture(), schedule: [],
    api: { timetable: '/guide', schedule: '/schedule', feature: '/feature' },
    station: { ts: { 10: { Sid: 10, View: 1 } }, all: { 10: { Name: 'test' } }, viewTS: 1 }, befBC: 1,
    options: { current: new Date(), broadcast: 1, dispservice: 1, hours: 4, page: [0, 0] } });
  for (const [name, method] of Object.entries(component.methods)) state[name] = method.bind(state);
  return state;
}

test('scroll extension starts immediately during an automatic fetch and old responses cannot overwrite it', async () => {
  const time = clock(), requests = [];
  const state = appContext({ get: async () => ({ data: [], status: 200 }),
    put: (_, params) => new Promise(resolve => requests.push({ params, resolve })) }, time);
  state.handleSocketMessage({ data: 'utt:' }); await settle();
  assert.equal(requests.length, 1);
  let done;
  const scrolling = state.extendTimeTable({ direction: 1, done: applied => { done = applied; } });
  assert.equal(requests.length, 2, 'scrolling bypasses the notification queue');
  for (let i = 0; i < 1000; i++) state.handleSocketMessage({ data: 'utt:' });
  assert.equal(requests.length, 2);
  const extra = fixture(); extra.Start = requests[1].params.start; extra.End = extra.Start + 14400;
  requests[1].resolve({ data: extra, status: 200 }); await scrolling;
  assert.equal(done, true);
  const extended = state.timetable;
  requests[0].resolve({ data: fixture(), status: 200 }); await settle();
  assert.equal(state.timetable, extended, 'automatic response cannot collapse the extended range');
  await time.advance(1000);
  assert.equal(requests.length, 3);
  assert.equal(requests[2].params.start, extended.Start);
  assert.equal(requests[2].params.hours, (extended.End - extended.Start) / 3600);
  state.handleSocketMessage({ data: 'utt:' });
  const latest = { ...extended, CurrentMilliseconds: Date.now() };
  requests[2].resolve({ data: latest, status: 200 }); await settle();
  assert.equal(state.timetable, latest, 'further notifications do not invalidate a useful response');
  state._uiRefreshQueue.dispose();
});

test('reservation notifications patch cached marks without fetching guide or features', async () => {
  const time = clock(), gets = [];
  const reservation = { id: 7, sid: 10, eid: 1, validate: true, deviceid: 'REC01', searchid: 0,
    starttime: 1, endtime: 100, complete: false, watchonly: false };
  const state = appContext({ get: async url => {
    gets.push(url); assert.equal(url, '/schedule');
    return { data: [{ ...reservation }], status: 200 };
  }, put: () => { throw new Error('unexpected guide request'); } }, time);
  const original = state.timetable, originalOptions = state.options;
  state.handleSocketMessage({ data: 'usc:' }); await settle();
  assert.equal(gets.length, 1);
  assert.equal(state.timetable.Tvp[10][0].ID, 7);
  assert.equal(state.timetable.Tvp[10][0].DeviceID, 'REC01');
  assert.equal(state.options, originalOptions);
  assert.equal(state.timetable.Start, original.Start);
  const unchanged = state.timetable;
  for (let i = 0; i < 100; i++) state.handleSocketMessage({ data: 'usc:' });
  await time.advance(1000);
  assert.equal(gets.length, 2);
  assert.equal(state.timetable, unchanged, 'identical reservations do not trigger another redraw');
  state._uiRefreshQueue.dispose();
});

test('background guide fetch defers to a foreground fetch and not the other way around', async () => {
  const time = clock(), requests = [];
  const state = appContext({ put: (_, params) => new Promise(resolve => requests.push({ params, resolve })) }, time);
  const foreground = state.updateTvProgram(true);
  state.handleSocketMessage({ data: 'utt:' }); await settle();
  assert.equal(requests.length, 1);
  requests[0].resolve({ data: fixture(), status: 200 }); await foreground; await settle();
  await time.advance(1000);
  assert.equal(requests.length, 2);
  state._uiRefreshQueue.dispose();
  requests[1].resolve({ data: fixture(), status: 200 }); await settle();
});
