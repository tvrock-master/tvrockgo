const { test } = require('node:test');
const assert = require('node:assert/strict');
const { serverURL, command } = require('../electron/protocol.cjs');
const device = { ID: 'チューナー1', RegistryCode: 'registry-1', State: 2, CurrentService: 100, CurrentProgram: { Eid: 1 } };
test('URL normalization preserves reverse proxy paths and rejects unsupported URLs', () => {
  assert.equal(serverURL('https://example.com/tv/'), 'https://example.com/tv');
  for (const url of ['file:///c:/test', 'http://user:pass@localhost', 'http://localhost/?x=1', 'invalid']) {
    assert.throws(() => serverURL(url));
  }
});
test('power and record commands match tvrockgo protocol', () => {
  assert.deepEqual(JSON.parse(command({ ...device, State: 0 }, 'power').slice(4)), { registryid: 'registry-1', boottype: 1 });
  assert.deepEqual(JSON.parse(command(device, 'power').slice(4)), { registryid: 'registry-1', boottype: 0 });
  assert.deepEqual(JSON.parse(command({ ...device, State: 0 }, 'power-record').slice(4)), { registryid: 'registry-1', boottype: 2 });
  assert.equal(command(device, 'record'), 'COM:{"id":"registry-1","command":"RecordStart","message":"チューナー1"}');
  assert.equal(command({ ...device, State: 3 }, 'stop'), 'COM:{"id":"registry-1","command":"RecordStop","message":"チューナー1"}');
});
test('capture uses REQ for both watching and recording', () => {
  for (const State of [2, 3]) assert.equal(command({ ...device, State }, 'capture'), 'REQ:{"id":"registry-1","command":"Capture","message":"チューナー1"}');
});
test('invalid states cannot stop or start recordings or power off a recording device', () => {
  for (const State of [0, 1, 3, 4]) assert.throws(() => command({ ...device, State }, 'record'));
  for (const State of [1, 3, 4]) assert.throws(() => command({ ...device, State }, 'power'));
  for (const State of [1, 2, 3, 4]) assert.throws(() => command({ ...device, State }, 'power-record'));
  for (const State of [0, 1, 4]) assert.throws(() => command({ ...device, State }, 'stop'));
  assert.throws(() => command({ ...device, CurrentProgram: null }, 'record'));
  assert.throws(() => command(undefined, 'capture'));
  assert.throws(() => command(device, 'invalid'));
});
test('watching reservations use the scheduler stop command', () => {
  assert.equal(command(device, 'stop'), 'COM:{"id":"registry-1","command":"RecordStop","message":"チューナー1"}');
});
