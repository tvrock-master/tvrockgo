const { test } = require('node:test');
const assert = require('node:assert/strict');
const { select, display, duration } = require('../src/schedules.js');
const { Client } = require('../src/client.js');
const reservation = { id: 1, deviceid: 'T1', validate: true, complete: false, starttime: 1000, endtime: 2800, tailtime: 30, idletime: 60, title: '番組' };

test('reservation display changes at each boundary including tail time', () => {
  const date = new Date(1000000);
  const endDate = new Date(reservation.endtime * 1000);
  const expectedDate = `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}～${String(endDate.getHours()).padStart(2, '0')}:${String(endDate.getMinutes()).padStart(2, '0')}`;
  for (const [now, label, time] of [
    [939, '次の予約', expectedDate],
    [940, '予約開始まで', '00:01:00'], [999, '予約開始まで', '00:00:01'],
    [1000, '予約実行中', '+00:00:00 / 00:30:30'], [2770, '予約実行中', '+00:29:30 / 00:30:30'],
    [2799, '予約実行中', '+00:29:59 / 00:30:30'],
    [2800, '予約終了まで', '00:00:30'], [2829, '予約終了まで', '00:00:01'],
  ]) assert.deepEqual(display(select([reservation], 'T1', now), now), { label, time, title: '番組' });
  assert.equal(select([reservation], 'T1', 2830), null);
  assert.equal(display(null, 2830).label, 'このデバイスでの予約はありません。');
  assert.equal(duration(90061), '25:01:01');
});

test('later overlapping reservation takes precedence only once its idle period starts', () => {
  const later = { ...reservation, id: 2, starttime: 2000, endtime: 4000 };
  const list = [later, reservation];
  assert.equal(select(list, 'T1', 900).id, 1);
  assert.equal(select(list, 'T1', 1939).id, 1);
  assert.equal(select(list, 'T1', 1940).id, 2);
  assert.equal(select(list, 'T1', 2000).id, 2);
  assert.equal(select(list, 'T2', 2000), null);
  assert.equal(select(list, 'T1', null), null);
  assert.equal(select([{ ...reservation, validate: false }, { ...later, complete: true }], 'T1', 2000), null);
});

test('ready time shifts idle boundary, elapsed time and overlap ordering but not displayed start', () => {
  const item = { ...reservation, readytime: 30 };
  assert.equal(display(item, 909).label, '次の予約');
  assert.equal(display(item, 909).time, display(reservation, 909).time);
  assert.equal(display(item, 910).time, '00:01:00');
  assert.equal(display(item, 969).time, '00:00:01');
  assert.deepEqual(display(item, 970), { label: '予約実行中', time: '+00:00:00 / 00:31:00', title: '番組' });
  assert.equal(display(item, 1000).time, '+00:00:30 / 00:31:00');
  assert.equal(display(item, 2800).time, '00:00:30');
  const earlier = { ...reservation, id: 2, starttime: 950, readytime: 0 };
  assert.equal(select([earlier, item], 'T1', 909).id, 2);
  assert.equal(select([earlier, item], 'T1', 910).id, 1);
  const shiftedEarlier = { ...item, readytime: 100 };
  assert.equal(select([earlier, shiftedEarlier], 'T1', 800).id, 1);
  assert.equal(select([earlier, shiftedEarlier], 'T1', 900).id, 2);
});

test('next reservation shows original start and adds a date only on a different server day', () => {
  const starttime = new Date(2026, 6, 3, 12, 34, 56).getTime() / 1000;
  const item = { ...reservation, starttime, endtime: starttime + 1800, readytime: 120 };
  const sameDay = new Date(2026, 6, 3, 10).getTime() / 1000;
  const previousDay = new Date(2026, 6, 2, 23, 59, 59).getTime() / 1000;
  assert.equal(display(item, sameDay).time, '12:34～13:04');
  assert.equal(display(item, previousDay).time, '7/3 12:34～13:04');
  assert.equal(display(item, new Date(2026, 6, 3).getTime() / 1000).time, '12:34～13:04');
  assert.equal(display(item, new Date(2025, 6, 3, 10).getTime() / 1000).time, '7/3 12:34～13:04');
  const overnight = { ...item, starttime: new Date(2026, 11, 31, 23, 30).getTime() / 1000,
    endtime: new Date(2027, 0, 1, 0, 30).getTime() / 1000 };
  assert.equal(display(overnight, new Date(2026, 11, 31, 20).getTime() / 1000).time, '23:30～1/1 00:30');
});

test('schedule fetch ignores stale responses and reports failures', async () => {
  const events = [], pending = [];
  const client = new Client((type, value) => events.push({ type, value }), url => url.endsWith('/rst')
    ? Promise.resolve(Response.json({ recording: [] })) : new Promise(resolve => pending.push(resolve)));
  client.base = 'http://test';
  const first = client.schedules(0), second = client.schedules(0);
  pending[1](Response.json([reservation])); await second;
  pending[0](Response.json([])); await first;
  assert.deepEqual(events.filter(event => event.type === 'schedules'), [{ type: 'schedules', value: [reservation] }]);
  client.request = async () => new Response('', { status: 500 });
  await client.schedules(0);
  assert.ok(events.some(event => event.type === 'schedule-error'));
  client.close();
});

test('program requests obtain server time instead of falling back to local time', async () => {
  const starts = [];
  const client = new Client(() => {}, async (_url, options) => {
    starts.push(JSON.parse(options.body).start);
    return Response.json({ CurrentMilliseconds: 1700000000000, Tvp: {} });
  });
  client.base = 'http://test';
  client.stationList = [{ Sid: 1, Tsp: 1, View: 1 }];
  await client.programs(0);
  assert.deepEqual(starts, [0, 1700000000]);
  client.close();
});
