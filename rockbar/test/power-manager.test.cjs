const { test } = require('node:test');
const assert = require('node:assert/strict');
const { Client } = require('../src/client.js');

test('sleep timer pause updates only the requested setting and uses the server response', async () => {
  const events = [], requests = [];
  let setting = { powermanager: true, sleeptimerpause: false, preventsleep: true, sleeptimer: 30 };
  const client = new Client((type, value) => events.push({ type, value }), async (url, options) => {
    requests.push({ url, options });
    if (options.method === 'PUT') setting = { ...setting, ...JSON.parse(options.body) };
    return Response.json(setting);
  });
  client.base = 'http://test/proxy';
  client.connected = true;
  try {
    await client.loadPowerManager(0);
    await client.setSleepTimerPause(true);
    assert.equal(requests[1].url, 'http://test/proxy/pms');
    assert.deepEqual(JSON.parse(requests[1].options.body), { sleeptimerpause: true });
    assert.deepEqual(events.at(-1), { type: 'power-manager', value: setting });
    assert.equal(client.powerManagerSetting.preventsleep, true);
    await client.setSleepTimerPause(false);
    assert.equal(client.powerManagerSetting.sleeptimerpause, false);
    setting = { ...setting, sleeptimerpause: true };
    await client.loadPowerManager(0);
    assert.equal(client.powerManagerSetting.sleeptimerpause, true);
    client.request = async () => new Response('', { status: 500 });
    await assert.rejects(client.setSleepTimerPause(false), /HTTP 500/);
    assert.equal(client.powerManagerSetting.sleeptimerpause, true);
    assert.equal(client.powerManagerBusy, null);
  } finally { client.close(); }
});

test('sleep timer rejects disabled management and disconnects, and resets unavailable settings', async () => {
  const events = [];
  const client = new Client((type, value) => events.push({ type, value }), async () => Response.json({ powermanager: false }));
  client.base = 'http://test';
  client.connected = true;
  await client.loadPowerManager(0);
  await assert.rejects(client.setSleepTimerPause(true), /電源管理/);
  client.request = async () => new Response('', { status: 403 });
  await client.loadPowerManager(0);
  assert.equal(events.at(-1).value, null);
  client.close();
  await assert.rejects(client.setSleepTimerPause(true), /接続していません/);
});

test('sleep timer ignores responses from an old server and prevents concurrent writes', async () => {
  let resolve;
  const client = new Client(() => {}, () => new Promise(done => { resolve = done; }));
  client.base = 'http://test';
  client.connected = true;
  client.powerManagerSetting = { powermanager: true, sleeptimerpause: false };
  const write = client.setSleepTimerPause(true);
  await assert.rejects(client.setSleepTimerPause(true), /更新中/);
  client.close();
  resolve(Response.json({ powermanager: true, sleeptimerpause: true }));
  await assert.rejects(write, /接続先/);
  assert.equal(client.powerManagerSetting, null);
  const read = client.loadPowerManager(client.generation);
  client.close();
  resolve(Response.json({ powermanager: true, sleeptimerpause: true }));
  await read;
  assert.equal(client.powerManagerSetting, null);
});
