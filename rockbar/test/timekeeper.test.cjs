const { test } = require('node:test');
const assert = require('node:assert/strict');
const { timekeeper } = require('../src/schedules.js');
const { Client } = require('../src/client.js');
const reservation = { id: 1, deviceid: 'REC01', validate: true, starttime: 1000, endtime: 2800, readytime: 30, tailtime: 30, sid: 1, title: '録画番組' };
const device = { ID: 'REC01', State: 3, CurrentService: 1 };
const stations = [{ Sid: 1, Name: '風待ち放送' }, { Sid: 2, Name: '雲庭チャンネル' }];

test('waiting starts at starttime minus idletime and counts negatively until recording starts', () => {
  const item = { ...reservation, idletime: 60, tailtime: 0 };
  const next = { ...item, id: 2, starttime: 4000, endtime: 5800, title: '次の番組' };
  const off = { ...device, State: 0, CurrentService: 2 };
  const before = timekeeper(off, [item, next], stations, 939);
  assert.equal(before.currentID, null);
  assert.equal(before.nextID, 1);
  const waiting = timekeeper(off, [item, next], stations, 940);
  assert.equal(waiting.currentID, 1);
  assert.equal(waiting.nextID, 2);
  assert.equal(waiting.waiting, true);
  assert.equal(waiting.values[0], '録画番組');
  assert.equal(waiting.values[1], '風待ち放送');
  assert.equal(waiting.values[3], '-00:00:30 / 00:30:30');
  assert.equal(waiting.ratio, 0);
  assert.equal(timekeeper(off, [item, next], stations, 969).values[3], '-00:00:01 / 00:30:30');
  const started = timekeeper(device, [item, next], stations, 970);
  assert.equal(started.currentID, 1);
  assert.equal(started.waiting, false);
  assert.equal(started.values[3], '00:00:00 / 00:30:30');
  assert.equal(timekeeper(device, [item, next], stations, 971).values[3], '00:00:01 / 00:30:30');
  assert.equal(timekeeper(off, [{ ...item, watchonly: true }], stations, 940).currentID, null);
});

test('timekeeper separates recording and next reservation and counts ready/tail time', () => {
  const next = { ...reservation, id: 2, starttime: 4000, endtime: 5800, sid: 2, title: '次の番組' };
  const list = [next, reservation, { ...next, validate: false, starttime: 3000 }, { ...next, complete: true, starttime: 2900 }];
  const view = timekeeper(device, list, stations, 1710);
  assert.equal(view.values[1], '風待ち放送');
  assert.equal(view.values[0], '録画番組');
  assert.equal(view.values[3], '00:12:20 / 00:31:00');
  assert.equal(view.values[4], '00:18:40');
  assert.equal(view.values[5], '次の番組');
  assert.equal(view.values[6], '雲庭チャンネル');
  assert.equal(timekeeper(device, list, stations, 1711).values[3], '00:12:21 / 00:31:00');
  assert.equal(timekeeper(device, list, stations, 2830).values[0], '録画情報を待っています');
  const off = timekeeper({ ...device, State: 0 }, list, stations, 1710);
  assert.equal(off.values[0], '予約はありません');
  assert.equal(off.values[5], '次の番組');
  assert.equal(off.ratio, 0);
  assert.equal(timekeeper({ ...device, CurrentService: 2 }, list, stations, 1710).values[3], '--:--:-- / --:--:--');
  assert.equal(timekeeper(device, [{ ...reservation, watchonly: true }], stations, 1710).values[0], '録画情報を待っています');
});

test('overnight reservations show a date and hours beyond midnight', () => {
  const starttime = new Date(2026, 9, 24, 23, 59, 30).getTime() / 1000;
  const item = { ...reservation, starttime, endtime: starttime + 1830, readytime: 0, tailtime: 0 };
  assert.equal(timekeeper(device, [item], stations, starttime - 60).values[7], '10/24 23:59:30～24:30:00');
  assert.equal(timekeeper(device, [item], stations, starttime).values[2], '23:59:30～24:30:00');
});

test('running reservation stays timed when it is missing from the schedule list', () => {
  const next = { ...reservation, id: 2, starttime: 4000, endtime: 5800, title: '次の番組' };
  const current = { REC01: reservation };
  const view = timekeeper(device, [next], stations, 1710, current);
  assert.equal(view.values[3], '00:12:20 / 00:31:00');
  assert.equal(view.values[4], '00:18:40');
  assert.equal(view.values[5], '次の番組');
  assert.equal(timekeeper(device, [next], stations, 1711, current).values[3], '00:12:21 / 00:31:00');
});

test('actual recording continues counting after the planned end and stops when the device stops', () => {
  const current = { REC01: reservation };
  const view = timekeeper(device, [], stations, 2890, current);
  assert.equal(view.values[3], '00:32:00 / 00:31:00');
  assert.equal(view.values[4], '00:00:00');
  assert.equal(view.ratio, 1);
  assert.equal(timekeeper({ ...device, State: 0 }, [], stations, 2890, current).values[3], '--:--:-- / --:--:--');
  assert.equal(timekeeper({ ...device, CurrentService: 2 }, [], stations, 1710, current).values[3], '--:--:-- / --:--:--');
});

test('running device binding wins over schedule reassignment and fresh end-time corrections are used', () => {
  const changed = { ...reservation, deviceid: 'REC02', endtime: 3400, complete: true };
  const view = timekeeper(device, [changed], stations, 1710, { REC01: reservation });
  assert.equal(view.values[3], '00:12:20 / 00:41:00');
  assert.equal(view.values[4], '00:28:40');
});

test('legacy server running IDs resolve the two devices whose SQL assignments were swapped', () => {
  const first = { ...reservation, deviceid: 'REC02', eid: 10 };
  const second = { ...reservation, id: 2, deviceid: 'REC01', sid: 2, eid: 20 };
  for (const [ID, sid, Eid] of [['REC01', 1, 10], ['REC02', 2, 20]]) {
    const current = { ID, State: 3, CurrentService: sid, CurrentProgram: { Sid: sid, Eid } };
    const view = timekeeper(current, [first, second], stations, 1710, null, [1, 2]);
    assert.equal(view.values[3], '00:12:20 / 00:31:00');
    assert.equal(view.values[4], '00:18:40');
  }
});

test('running status and precise clock refresh even when schedule fetching fails in timekeeper mode', async () => {
  const events = [], urls = [];
  let serverTime = 1710000;
  const client = new Client((type, value) => events.push({ type, value }), async url => {
    urls.push(url);
    return url.endsWith('/sch') ? new Response('', { status: 500 }) :
      Response.json({ recording: [1], current: { REC01: reservation }, currentmilliseconds: serverTime });
  });
  client.base = 'http://test'; client.setMode('timekeeper');
  await client.schedules(0);
  assert.ok(events.some(event => event.type === 'schedule-error'));
  assert.deepEqual(events.find(event => event.type === 'recording-status').value.current.REC01, reservation);
  assert.equal(events.find(event => event.type === 'server-time').value, 1710000);
  serverTime += 3 * 3600000;
  await client.schedules(0);
  assert.equal(events.filter(event => event.type === 'server-time').at(-1).value, serverTime);
  assert.ok(urls.every(url => !url.endsWith('/tvp')));
  client.close();
});

test('older running status responses cannot rewind the clock or replace newer running IDs', async () => {
  const events = [], pending = [];
  const client = new Client((type, value) => events.push({ type, value }), url => url.endsWith('/sch')
    ? Promise.resolve(Response.json([])) : new Promise(resolve => pending.push(resolve)));
  client.base = 'http://test';
  const first = client.schedules(0), second = client.schedules(0);
  pending[1](Response.json({ recording: [2], currentmilliseconds: 2000000 })); await second;
  pending[0](Response.json({ recording: [1], currentmilliseconds: 1000000 })); await first;
  assert.deepEqual(events.filter(event => event.type === 'recording-status').map(event => event.value.recording), [[2]]);
  assert.deepEqual(events.filter(event => event.type === 'server-time').map(event => event.value), [2000000]);
  client.close();
});

test('switching modes aborts in-flight guide requests, hides captions and suppresses stale results', async () => {
  const events = [], requests = [];
  const client = new Client((type, value) => events.push({ type, value }), (url, options) => new Promise(resolve => requests.push({ url, options, resolve })));
  client.base = 'http://test'; client.connected = true;
  client.stationList = stations.map(item => ({ ...item, View: 1, Tsp: 1 }));
  client.clock.sync(1710000);
  client.devices.set(device.ID, { ...device, CurrentProgram: { Sid: 1, Eid: 1, Start: 1000 } });
  client.watchDevice(device.ID);
  const pending = client.programs(0);
  client.setMode('timekeeper');
  assert.ok(requests.every(request => request.options.signal.aborted));
  assert.equal(client.captionFeed.key, '');
  for (const request of requests) request.resolve(Response.json({ Tvp: {}, CurrentMilliseconds: 1710000, items: [], snapshot: 0 }));
  await pending;
  assert.equal(events.some(event => event.type === 'programs'), false);
  const count = requests.length;
  await client.programs(0);
  assert.equal(requests.length, count);
  client.setMode('device');
  assert.ok(requests.length > count);
  assert.notEqual(client.captionFeed.key, '');
  client.close();
  for (const request of requests.slice(count)) request.resolve(Response.json({ Tvp: {}, items: [], snapshot: 0 }));
});
