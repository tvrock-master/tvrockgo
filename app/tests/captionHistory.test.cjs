const assert = require('assert');
const fs = require('fs');
const path = require('path');

async function main() {
  const source = fs.readFileSync(path.join(__dirname, '../src/components/CaptionHistoryWindow.vue'), 'utf8');
  globalThis.historyUtility = require('../src/utility');
  const script = source.match(/<script>([\s\S]*?)<\/script>/)[1]
    .replace("import axios from 'axios';", 'const axios = globalThis.historyAxios;')
    .replace("import utility from '../utility.js';", 'const utility = globalThis.historyUtility;');
  let resolve;
  globalThis.historyAxios = { get: (url, request) => {
    assert.equal(url, '/caption');
    assert.deepStrictEqual(request.params, { sid: 101, eid: 2, start: 100, after: 0 });
    return new Promise(done => { resolve = done; });
  } };
  const { default: component } = await import('data:text/javascript;base64,' + Buffer.from(script).toString('base64'));
  const makeState = () => {
    const state = { ...component.data(), program: { Sid: 101, Eid: 2, Start: 100 }, api: '/caption', $refs: {}, $nextTick: callback => callback() };
    for (const [name, method] of Object.entries(component.methods)) state[name] = method.bind(state);
    return state;
  };
  const state = makeState();
  const pending = component.methods.load.call(state);
  assert.equal(state.loading, true);
  resolve({ data: { items: [{ ID: 3, Text: '字幕' }], hasMore: true, snapshot: 5 } });
  await pending;
  assert.equal(state.items[0].Text, '字幕');
  globalThis.historyAxios.get = async (url, request) => {
    assert.equal(request.params.after, 3);
    assert.equal(request.params.snapshot, 5);
    return { data: { items: [{ ID: 5, Text: '続き' }], hasMore: false, snapshot: 5 } };
  };
  await component.methods.load.call(state);
  assert.deepStrictEqual(state.items.map(item => item.ID), [3, 5]);
  const closed = makeState();
  globalThis.historyAxios.get = () => new Promise(done => { resolve = done; });
  const stale = component.methods.load.call(closed);
  closed.disposed = true;
  resolve({ data: { items: [{ ID: 9, Text: '古い応答' }], hasMore: false } });
  await stale;
  assert.equal(closed.items.length, 0);
  let emptyClosed = false;
  const empty = makeState();
  empty.$emit = event => { emptyClosed = event === 'close'; };
  globalThis.historyAxios.get = async () => ({ data: { items: [], hasMore: false, snapshot: 0 } });
  await component.methods.load.call(empty);
  assert.equal(emptyClosed, true);
  assert(source.includes('v-show="displayItems.length > 0 || error"'));
  emptyClosed = false;
  empty.program = { ...empty.program, End: Date.now() / 1000 + 60 };
  empty.hasMore = true;
  await empty.load();
  assert.equal(emptyClosed, false, 'current program waits invisibly for its first caption');

  const ruby = makeState();
  ruby.snapshot = 2;
  ruby.hasMore = false;
  ruby.items = [{ ID: 2, Pts: 9000, Lang: 1, Text: '漢字' }];
  ruby.pending.push(
    { HistoryID: 3, Pts: 9000, Lang: 1, Size: 'small', Text: 'かんじ' },
    { HistoryID: 4, Pts: 9000, Lang: 1, Size: 'medium', Text: 'の本文' },
    { HistoryID: 5, Pts: 9000, Lang: 1, Size: 'micro', Text: 'ほんぶん' },
  );
  ruby.flushPending();
  assert.equal(component.computed.displayItems.call(ruby)[0].Text, '漢字の本文');
  assert.equal(ruby.lastHistoryID, 5);
  ruby.pending.push(
    { HistoryID: 5, Pts: 9000, Lang: 1, Text: '重複' },
    { HistoryID: 6, Pts: 9000, Lang: 1, Size: 'small', Clear: true, Text: 'つぎ' },
    { HistoryID: 7, Pts: 9000, Lang: 1, Text: '次' },
  );
  ruby.flushPending();
  assert.deepStrictEqual(component.computed.displayItems.call(ruby).map(item => item.Text), ['漢字の本文', '次']);

  const realtime = makeState();
  const packet = (id, text, extra = {}) => ({ Sid: 101, Eid: 2, Start: 100, HistoryID: id, Text: text, Pts: 9000, Lang: 1, ...extra });
  realtime.enqueue(packet(1, '別番組', { Start: 200 }));
  assert.equal(realtime.pending.length, 0);
  const scrolling = makeState();
  scrolling.snapshot = 0;
  scrolling.hasMore = false;
  scrolling.$refs.body = { scrollHeight: 1000, scrollTop: 100, clientHeight: 400 };
  scrolling.pending.push(packet(0, '保存無効でも表示'));
  scrolling.flushPending();
  assert.equal(scrolling.liveItems[0].Text, '保存無効でも表示');
  assert.equal(scrolling.$refs.body.scrollTop, 100, 'reading older text must not jump');
  scrolling.$refs.body.scrollTop = 600;
  scrolling.pending.push(packet(0, '続き'));
  scrolling.flushPending();
  assert.equal(scrolling.$refs.body.scrollTop, 1000, 'follow new text when already at bottom');
  realtime.enqueue(packet(4, 'DBと重複'));
  realtime.enqueue(packet(6, 'さて'));
  realtime.enqueue(packet(7, '風向きは'));
  clearTimeout(realtime.flushTimer);
  realtime.flushPending();
  assert.equal(realtime.liveItems.length, 0, 'wait for DB snapshot');
  realtime.snapshot = 5;
  realtime.hasMore = false;
  realtime.items = [{ ID: 5, Pts: 9000, Lang: 1, Text: '「' }];
  realtime.flushPending();
  assert.equal(realtime.liveItems[0].Text, 'さて風向きは');
  assert.equal(component.computed.displayItems.call(realtime)[0].Text, '「さて風向きは');
  realtime.pending.push(packet(7, '重複'), packet(8, '北向き」'));
  realtime.flushPending();
  assert.equal(component.computed.displayItems.call(realtime)[0].Text, '「さて風向きは北向き」');
  realtime.pending.push(packet(9, '', { Clear: true }), packet(10, '次の発話'));
  realtime.flushPending();
  assert.equal(realtime.liveItems.length, 2, 'clear separates statements without erasing history');
  realtime.hasMore = true;
  assert(component.computed.displayItems.call(realtime).some(item => item.Gap));
  realtime.disposed = true;
  realtime.enqueue(packet(11, '閉じた後'));
  assert.equal(realtime.pending.length, 0);
  globalThis.window = { innerWidth: 1200, innerHeight: 800 };
  const placement = { anchor: { isConnected: true, getBoundingClientRect: () => ({ left: 900, right: 1100, top: 650 }) } };
  component.methods.place.call(placement);
  assert.equal(placement.position.left, '532px');
  assert.equal(placement.position.top, '272px');
  console.log('Caption history pagination, stale response and placement: OK');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
