const { test } = require('node:test');
const assert = require('node:assert/strict');
const http = require('node:http');
const { WebSocketServer } = require('ws');
const { Client } = require('../electron/client.cjs');

test('Authenticated HTTP bootstrap, WebSocket cookie forwarding, commands and disconnect handling', { timeout: 10000 }, async () => {
  const device = { ID: 'T1', RegistryCode: 'R1', State: 2, CurrentService: 1, CurrentProgram: { Title: '番組', Eid: 1 } };
  let schedules = [{ id: 1, deviceid: 'T1', title: '予約' }];
  const agents = [];
  const server = http.createServer((req, res) => {
    agents.push(req.headers['user-agent']);
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(req.url === '/sch' ? schedules : req.url === '/dvs' ? [device] : { 1: { Sid: 1, Name: 'テスト放送' } }));
  });
  const wss = new WebSocketServer({ server, path: '/con',
    verifyClient: ({ req }) => req.headers.cookie === 'session=test-login',
  });
  let socket;
  let connections = 0;
  let snapshotState = 3;
  const received = [];
  wss.on('connection', (ws, req) => { agents.push(req.headers['user-agent']); connections++; socket = ws; ws.on('message', message => {
    received.push(message.toString());
    if (message.toString() === 'REL:') ws.send('dev:' + JSON.stringify({ ...device, State: snapshotState }));
  }); });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const events = [];
  const cookieURLs = [];
  const client = new Client((type, value) => events.push({ type, value }), fetch, async url => {
    cookieURLs.push(url);
    return [{ name: 'session', value: 'test-login', httpOnly: true }];
  });
  async function until(condition) {
    for (let n = 0; n < 200; n++) {
      if (condition()) return;
      await new Promise(resolve => setTimeout(resolve, 25));
    }
    assert.fail('Expected event was not received');
  }
  try {
    await client.connect(`http://127.0.0.1:${server.address().port}`);
    await until(() => client.devices.get('T1')?.State === 3 && events.some(event => event.type === 'stations'));
    assert.equal(client.connected, true);
    assert.ok(agents.length >= 2);
    assert.ok(agents.every(agent => agent === 'TvRockRockbar/dev'));
    assert.deepEqual(cookieURLs, [`http://127.0.0.1:${server.address().port}/con`]);
    await until(() => events.some(event => event.type === 'schedules' && event.value?.[0]?.id === 1));
    schedules = [];
    socket.send('usc:');
    await until(() => events.some(event => event.type === 'schedules' && event.value?.length === 0));
    assert.ok(events.some(event=>event.type==='schedule-update'),'ScheduleUpdate is forwarded independently of schedule polling');
    client.send('T1', 'stop');
    client.send('T1', 'capture');
    await until(() => received.filter(message => !message.startsWith('PLY:')).length === 3);
    assert.ok(received.includes('COM:{"id":"R1","command":"RecordStop","message":"T1"}'));
    assert.ok(received.includes('REQ:{"id":"R1","command":"Capture","message":"T1"}'));
    socket.send('dev:' + JSON.stringify({ ...device, State: 0 }));
    await until(() => client.devices.get('T1')?.State === 0);
    client.send('T1', 'power');
    await until(() => received.includes('BDV:{"registryid":"R1","boottype":1}'));
    socket.close();
    await until(() => !client.connected);
    assert.throws(() => client.send('T1', 'stop'));
    assert.ok(events.some(event => event.type === 'status' && event.value.message.includes('再接続')));
    snapshotState = 0;
    await until(() => connections === 2 && client.connected && client.devices.get('T1')?.State === 0);
    assert.equal(received.filter(message => message === 'REL:').length, 2,
      'reconnection requests the latest device state');
    const powerCount = received.filter(message => message.startsWith('BDV:')).length;
    client.send('T1', 'power');
    await until(() => received.filter(message => message.startsWith('BDV:')).length === powerCount + 1);
    assert.equal(received.at(-1), 'BDV:{"registryid":"R1","boottype":1}',
      'power commands work on the replacement connection');
  } finally {
    client.close();
    for (const ws of wss.clients) ws.terminate();
    await new Promise(resolve => wss.close(resolve));
    await new Promise(resolve => server.close(resolve));
  }
});

test('Device list can load while unauthenticated controller WebSocket is rejected', { timeout: 10000 }, async () => {
  const server = http.createServer((req, res) => {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify(req.url === '/dvs' ? [{ ID: 'T1' }] : {}));
  });
  const wss = new WebSocketServer({ server, path: '/con', verifyClient: () => false });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  let disconnected;
  const closed = new Promise(resolve => { disconnected = resolve; });
  const client = new Client((type, value) => {
    if (type === 'status' && value.message.includes('接続が切れました')) disconnected();
  });
  try {
    await client.connect(`http://127.0.0.1:${server.address().port}`);
    await closed;
    assert.equal(client.devices.has('T1'), true);
    assert.equal(client.connected, false);
    assert.throws(() => client.send('T1', 'power'), /接続していません/);
  } finally {
    client.close();
    await new Promise(resolve => wss.close(resolve));
    await new Promise(resolve => server.close(resolve));
  }
});

test('Electron HTML login response without a response URL prompts login without retries', async () => {
 const events=[];
 const client=new Client((type,value)=>events.push({type,value}),async()=>new Response('<form>login</form>',{headers:{'Content-Type':'text/html; charset=utf-8'}}));
 try {
  await client.connect('http://localhost');
  assert.equal(client.connected,false);
  assert.equal(client.retry,undefined);
  assert.ok(events.some(e=>e.type==='status'&&e.value.message.includes('ログインが必要')));
 } finally {client.close();}
});
