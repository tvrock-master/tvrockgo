const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const Vue = require('vue');
const label = require('../src/define');

const sockets = [];
const timers = new Map();
let timerId = 0;
class FakeWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  constructor(url) {
    this.url = url;
    this.readyState = FakeWebSocket.CONNECTING;
    this.sent = [];
    sockets.push(this);
  }
  open() { this.readyState = FakeWebSocket.OPEN; this.onopen({}); }
  close() { this.readyState = 3; this.onclose(); }
  send(message) {
    assert.equal(this.readyState, FakeWebSocket.OPEN);
    this.sent.push(message);
  }
  receive(message) { this.onmessage({ data: message }); }
}

function loadComponent(file) {
  const source = fs.readFileSync(path.join(__dirname, '../src', file), 'utf8');
  const script = source.match(/<script>([\s\S]*?)<\/script>/)[1]
    .replace(/import\s+([^;]+?)\s+from\s+['"]([^'"]+)['"];?/g,
      (_, name, source) => {
        const dependency = source.includes('timeTableRenderer') ? 'renderer' :
          source.includes('timeTableGenreColors') ? 'genreColors' : name;
        return `const ${name} = dependencies.${dependency} || {};`;
      })
    .replace('export default', 'module.exports =');
  const context = { module: { exports: {} },
    dependencies: { label, utility: { log() {} }, clientIdentity: { socketURL: url => url },
      genreColors: require('../src/timeTableGenreColors') }, require: () => '',
    window: { location: { protocol: 'http:', host: 'example.test' } },
    WebSocket: FakeWebSocket,
    setTimeout(callback, delay) { timers.set(++timerId, { callback, delay }); return timerId; },
    clearTimeout(id) { timers.delete(id); },
  };
  vm.runInNewContext(script, context, { filename: file });
  return context.module.exports;
}

async function main() {
  const app = loadComponent('App.vue');
  const controller = loadComponent('components/DeviceController.vue');
  const state = { socket: null, socketReconnectTimer: null, socketReconnectDelay: 1000,
    socketMaxReconnectDelay: 30000, socketClosing: false, bml: {},
    device: [{ ID: 'TEST01', RegistryCode: 'registry', State: label.DSTAT_READY }],
    station: { tsall: {}, os: {}, bscs: {} },
    handlePowerManagerSettingChange() {}, pruneCaptions() {},
    requestUiRefresh() {},
  };
  for (const name of ['connectSocket', 'scheduleSocketReconnect', 'closeSocket',
    'handleSocketMessage', 'updateDevice']) state[name] = app.methods[name].bind(state);
  // Use Vue's prop updates to check that the controller receives the replacement socket.
  const parent = new Vue({ data: state });
  const child = new (Vue.extend(controller))({ propsData: {
    socket: null, device: state.device, options: {},
  } });
  parent.$watch('socket', socket => { child.socket = socket; });
  parent.$watch('device', devices => { child.device = devices; });

  state.connectSocket();
  const first = sockets[0];
  first.open();
  assert.deepEqual(first.sent, ['REL:'], 'initial connection requests all devices');
  first.close();
  await Vue.nextTick();
  child.bootDevice('registry', label.DBOOT_TYPE_WATCH);
  assert.equal(first.sent.length, 1, 'no commands are sent on the closed connection');

  // One failed attempt while the server is down, then a successful reconnect.
  for (const delay of [1000, 2000]) {
    const timer = timers.get(state.socketReconnectTimer);
    assert.equal(timer.delay, delay);
    timers.delete(state.socketReconnectTimer);
    timer.callback();
    if (delay === 1000) sockets.at(-1).close();
  }
  const current = sockets.at(-1);
  current.open();
  await Vue.nextTick();
  assert.equal(child.socket, current);
  assert.deepEqual(current.sent, ['REL:'], 'reconnection requests a fresh device snapshot');
  assert.equal(state.socketReconnectDelay, 1000);
  current.receive('dev:' + JSON.stringify({
    ID: 'TEST01', RegistryCode: 'registry', State: label.DSTAT_OFF, TunerType: 0,
  }));
  await Vue.nextTick();
  assert.equal(state.device[0].State, label.DSTAT_OFF, 'stale state is replaced after restart');
  assert.equal(child.device[0].State, label.DSTAT_OFF, 'controller receives the latest device state');
  child.bootDevice(state.device[0].RegistryCode, label.DBOOT_TYPE_WATCH);
  child.comSend('registry', label.COM_RECORD_START, 'TEST01');
  child.reqSend('registry', 'StreamChange', 101);
  assert.deepEqual(current.sent.map(message => message.slice(0, 4)), ['REL:', 'BDV:', 'COM:', 'REQ:']);
  assert.equal(first.sent.length, 1);
  state.closeSocket();
  assert.equal(timers.size, 0, 'intentional shutdown does not reconnect');
  child.$destroy();
  parent.$destroy();
  console.log('Device state refresh / reconnect / controller commands: OK');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
