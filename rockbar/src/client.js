(function (root, factory) {
  if (typeof module !== 'undefined') module.exports = factory(require('./protocol.js'), require('./clock.js'), require('./captions.js'));
  else root.rockbarClient = factory(root.rockbarProtocol, root.rockbarTime, root.rockbarCaptions);
})(globalThis, function (protocol, time, captions) {

const { serverURL, command, tuneCommands } = protocol;

class Client {
  constructor(emit, request = fetch, createSocket = url => new globalThis.WebSocket(url)) {
    this.emit = emit;
    this.request = request;
    this.createSocket = createSocket;
    this.stationList = [];
    this.generation = 0;
    this.devices = new Map();
    this.playbackRevision = 0;
    this.clock = new time.ServerClock();
    this.mode = 'device';
    this.captionFeed = new captions.CaptionFeed(request, value => emit('captions', value));
  }
  close() {
    this.generation++;
    clearTimeout(this.retry);
    clearInterval(this.refresh);
    clearInterval(this.guideRefresh);
    this.guideAbort?.abort();
    this.abort?.abort();
    if (this.socket) {
      if (this.socket.terminate) this.socket.terminate();
      else this.socket.close();
    }
    this.socket = null;
    this.connected = false;
    this.powerManagerSetting = null;
    this.playbackDefinitions=[];
    this.emit('power-manager', null);
    this.captionFeed.reset();
    this.clock.reset();
    this.emit('server-time', null);
  }
  async connect(value) {
    const base = serverURL(value);
    this.close();
    this.base = base;
    this.devices.clear();
    this.stationList = [];
    this.emit('programs', {});
    this.emit('devices', []);
    this.emit('schedules', null);
    this.emit('recording-status', null);
    const generation = this.generation;
    this.emit('status', { connected: false, message: '接続中…' });
    const abort = new AbortController();
    this.abort = abort;
    const timeout = setTimeout(() => abort.abort(), 8000);
    try {
      const response = await this.request(base + '/dvs', { signal: this.abort.signal, redirect: 'follow' });
      if (generation !== this.generation) return;
      if (response.status === 401 || (response.url && new URL(response.url).pathname.endsWith('/login')) ||
          (response.ok && response.headers.get('content-type')?.includes('text/html'))) {
        this.emit('status', { connected: false, message: 'ログインが必要です。設定の「ログイン」を押してください。' });
        return;
      }
      if (!response.ok) throw new Error('HTTP ' + response.status);
      this.syncTime(Date.parse(response.headers.get('date')));
      const devices = await response.json();
      if (!Array.isArray(devices)) throw new Error('デバイス一覧の形式が不正です');
      if (generation !== this.generation) return;
      for (const device of devices) this.devices.set(device.ID, device);
      this.publish();
      const socket = await this.createSocket(base.replace(/^http/, 'ws') + '/con');
      if (generation !== this.generation) {
        // A server change can occur while Electron reads the login cookies.
        socket.addEventListener('error', () => {});
        if (socket.terminate) socket.terminate();
        else socket.close();
        return;
      }
      this.socket = socket;
      socket.addEventListener('open', () => {
        if (generation !== this.generation) return;
        this.connected = true;
        this.emit('status', { connected: true, message: '接続済み' });
        socket.send('REL:');
        socket.send('PLY:' + JSON.stringify({action:'hello',native:!!this.nativePlayback}));
        this.schedules(generation);
        this.loadPowerManager(generation);
        this.updateCaptionTarget();
      });
      socket.addEventListener('message', event => {
        if (generation !== this.generation) return;
        const text = String(event.data);
        if (text.startsWith('pbs:')) {
          try { this.applyPlaybackDevices(JSON.parse(text.slice(4))); } catch {}
        }
        if (text.startsWith('ply:')) {
          try {
            const message=JSON.parse(text.slice(4));
            if (message.action==='state') this.applyPlaybackDevices(message.devices);
            if (message.action==='devices') this.playbackDefinitions=message.devices;
            if (message.action==='caption') this.captionFeed.receivePlayback(message);
            if (message.error) this.emit('notice',message.error);
            this.emit('playback',message);
          } catch { this.emit('notice','再生状態の読み取りに失敗しました'); }
        }
        if (text.startsWith('wat:')) {
          try { this.emit('remote-watch',JSON.parse(text.slice(4))); } catch { this.emit('notice','視聴状態の読み取りに失敗しました'); }
        }
        if (text.startsWith('dev:')) {
          try {
            const device = JSON.parse(text.slice(4));
            if (typeof device.ID !== 'string') return;
            const previous=this.devices.get(device.ID);
            this.devices.set(device.ID, device);
            this.publish();
            this.updateCaptionTarget();
            if(JSON.stringify([previous?.State,previous?.CurrentService,previous?.CurrentProgram?.Eid,previous?.CurrentProgram?.Start])!==JSON.stringify([device.State,device.CurrentService,device.CurrentProgram?.Eid,device.CurrentProgram?.Start]))this.programs(generation);
          } catch { this.emit('notice', 'デバイス情報の読み取りに失敗しました'); }
        }
        if (text.startsWith('ust:')) this.stations(generation);
        if (text.startsWith('utt:')) this.programs(generation);
        if (text.startsWith('usc:')) { this.emit('schedule-update'); this.schedules(generation); }
        if (text.startsWith('ups:')) this.loadPowerManager(generation);
        if (text.startsWith('cap:') && this.mode === 'device') {
          try { this.captionFeed.receive(JSON.parse(text.slice(4))); } catch {}
        }
      });
      socket.addEventListener('error', () => {});
      socket.addEventListener('close', () => this.reconnect(generation, '接続が切れました'));
      this.stations(generation);
    } catch (error) {
      this.reconnect(generation, '接続できません: ' + error.message);
    } finally { clearTimeout(timeout); }
  }
  async schedules(generation) {
    const requestID = this.scheduleRequestID = (this.scheduleRequestID || 0) + 1;
    await Promise.all([this.loadSchedules(generation, requestID), this.loadRecordingStatus(generation, requestID)]);
  }
  async loadSchedules(generation, requestID) {
    try {
      const response = await this.request(this.base + '/sch', { signal: AbortSignal.timeout(8000), redirect: 'error' });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const data = await response.json();
      if (data !== null && !Array.isArray(data)) throw new Error('予約一覧の形式が不正です');
      if (generation === this.generation && requestID === this.scheduleRequestID) this.emit('schedules', data || []);
    } catch {
      if (generation === this.generation && requestID === this.scheduleRequestID) this.emit('schedule-error', '予約一覧を取得できません');
    }
  }
  async loadRecordingStatus(generation, requestID) {
    try {
      const response = await this.request(this.base + '/rst', { signal: AbortSignal.timeout(8000), redirect: 'error' });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const data = await response.json();
      if (!data || !Array.isArray(data.recording)) throw new Error('録画状態の形式が不正です');
      if (generation !== this.generation || requestID !== this.scheduleRequestID) return;
      const milliseconds = Number.isFinite(data.currentmilliseconds) ? data.currentmilliseconds : Date.parse(response.headers?.get('date'));
      this.syncTime(milliseconds);
      this.emit('recording-status', data);
    } catch {
      if (generation === this.generation && requestID === this.scheduleRequestID) this.emit('recording-status-error', '実行中の録画情報を取得できません');
    }
  }
  async loadPowerManager(generation) {
    if(generation!==this.generation)return;
    if(this.powerManagerBusy===generation){this.powerManagerReload=true;return;}
    this.powerManagerReload=false;
    this.powerManagerBusy = generation;
    try {
      const response = await this.request(this.base + '/pms', { signal: AbortSignal.timeout(8000), redirect: 'error' });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const setting = await response.json();
      if (generation === this.generation) {
        this.powerManagerSetting = setting;
        this.emit('power-manager', setting);
      }
    } catch {
      if (generation === this.generation) {
        this.powerManagerSetting = null;
        this.emit('power-manager', null);
      }
    } finally {
      if (this.powerManagerBusy === generation) {this.powerManagerBusy=null;if(this.powerManagerReload && generation===this.generation)void this.loadPowerManager(generation);}
    }
  }
  async setSleepTimerPause(paused) {
    if (!this.connected) throw new Error('サーバーに接続していません');
    if (!this.powerManagerSetting?.powermanager) throw new Error('電源管理が有効ではありません');
    const generation = this.generation;
    if (this.powerManagerBusy === generation) throw new Error('スリープタイマー設定を更新中です');
    if (typeof paused !== 'boolean') throw new Error('スリープタイマー設定が不正です');
    this.powerManagerBusy = generation;
    try {
      const response = await this.request(this.base + '/pms', {
        method: 'PUT', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ sleeptimerpause: paused }),
        signal: AbortSignal.timeout(8000), redirect: 'error',
      });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const setting = await response.json();
      if (generation !== this.generation) throw new Error('接続先が変更されました');
      this.powerManagerSetting = setting;
      this.emit('power-manager', setting);
      return { ok: true };
    } finally {
      if (this.powerManagerBusy === generation) {this.powerManagerBusy=null;if(this.powerManagerReload && generation===this.generation)void this.loadPowerManager(generation);}
    }
  }
  async stations(generation) {
    try {
      const response = await this.request(this.base + '/stl', { signal: AbortSignal.timeout(8000), redirect: 'error' });
      if (!response.ok) throw new Error();
      const data = await response.json();
      if (generation === this.generation) {
        this.stationList = Object.values(data || {});
        this.emit('stations', data);
        this.programs(generation);
      }
    } catch {
      if (generation === this.generation) this.emit('notice', 'サービス名を取得できません（SIDを表示します）');
    }
  }
  async programs(generation) {
    if(generation!==this.generation)return;
    if(this.mode!=='device' || this.devices.get(this.selectedDevice)?.Kind==='playback' || this.guideBusy===generation || !this.stationList.length){this.guideDirty=true;return;}
    this.guideDirty=false;
    const abort = new AbortController();
    this.guideAbort = abort;
    this.guideBusy = generation;
    try {
      const services = this.stationList.filter(item => item.View > 0 && [1, 2, 3, 4].includes(item.Tsp)).map(item => item.Sid);
      const programs = {};
      if (this.clock.now() === null && services.length) {
        const response = await this.request(this.base + '/tvp', {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ services: services.slice(0, 1), start: 0, hours: 0, week: false }),
          signal: AbortSignal.any([abort.signal, AbortSignal.timeout(8000)]), redirect: 'error',
        });
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const data = await response.json();
        if (generation !== this.generation || abort.signal.aborted) return;
        this.syncTime(data.CurrentMilliseconds);
        if (this.clock.now() === null) throw new Error('サーバー時刻を取得できません');
      }
      for (let offset = 0; offset < services.length; offset += 32) {
        if (abort.signal.aborted) return;
        const response = await this.request(this.base + '/tvp', {
          method: 'PUT', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ services: services.slice(offset, offset + 32), start: Math.floor(this.clock.now() / 1000), hours: 1, week: false }),
          signal: AbortSignal.any([abort.signal, AbortSignal.timeout(8000)]), redirect: 'error',
        });
        if (generation !== this.generation || abort.signal.aborted) return;
        if (!response.ok) throw new Error('HTTP ' + response.status);
        const data = await response.json();
        if (generation !== this.generation || abort.signal.aborted) return;
        this.syncTime(data.CurrentMilliseconds);
        if (!data.Tvp || typeof data.Tvp !== 'object') throw new Error('番組表の形式が不正です');
        Object.assign(programs, data.Tvp);
      }
      if (generation === this.generation && !abort.signal.aborted) this.emit('programs', programs);
    } catch {
      if (generation === this.generation && !abort.signal.aborted) this.emit('guide-error', '番組一覧を取得できません。ログイン状態と接続を確認してください。');
    } finally {
      if (this.guideAbort === abort) {this.guideBusy=null;if(this.guideDirty && !abort.signal.aborted && this.connected)void this.programs(this.generation);}
    }
  }
  tune(id, sid) {
    if (!this.connected || this.socket?.readyState !== 1) throw new Error('サーバーに接続していません');
    for (const message of tuneCommands(this.devices.get(id), sid, this.stationList)) this.socket.send(message);
  }
  syncTime(milliseconds) {
    if (this.clock.sync(milliseconds)) this.emit('server-time', milliseconds);
  }
  watchDevice(id) {
    const playback=this.devices.get(this.selectedDevice)?.Kind==='playback';
    this.selectedDevice=id;this.updateCaptionTarget();
    if(this.devices.get(id)?.Kind==='playback'){this.guideAbort?.abort();this.guideBusy=null;this.guideDirty=true;}
    else if(playback && this.connected)void this.programs(this.generation);
  }
  setMode(mode) {
    if (!['device', 'timekeeper'].includes(mode)) throw new Error('表示モードが不正です');
    if (mode === this.mode) return;
    this.mode = mode;
    this.guideAbort?.abort();
    this.guideBusy = null;
    clearInterval(this.guideRefresh);
    this.updateCaptionTarget();
    if (this.connected && mode === 'device') {
      this.programs(this.generation);
    }
  }
  updateCaptionTarget() {
    this.captionFeed.syncDevices(this.connected ? this.devices.values() : []);
    this.captionFeed.select(this.connected && this.mode === 'device' ? this.devices.get(this.selectedDevice) : null, this.base);
  }
  publish() { this.emit('devices', [...this.devices.values()]); }
  applyPlaybackDevices(devices) {
    if (!Array.isArray(devices)) return;
    for (const [id,device] of this.devices) if (device.Kind==='playback') this.devices.delete(id);
    for (const device of devices) if (device.Kind==='playback' && device.Playback) this.devices.set(device.ID,{...device,Playback:{...device.Playback,revision:++this.playbackRevision}});
    this.updateCaptionTarget(); this.publish();
  }
  playback(id, action, seconds) {
    if (!this.connected || this.socket?.readyState!==1) throw new Error('サーバーに接続していません');
    if(action==='create') {
      if(!this.nativePlayback)throw new Error('Electron版Rockbarで追加してください');
      this.socket.send('PLY:'+JSON.stringify({action}));return;
    }
    const device=this.devices.get(id);
    if (device?.Kind!=='playback' || !device.Playback) throw new Error('再生デバイスを選択してください');
    const extra=action==='load'?{scheduleId:seconds.id,slot:seconds.slot}:action==='bookmark-add'||action==='bookmark-delete'?{positionMs:seconds.positionMs,generation:seconds.generation}:{seconds};
    this.socket.send('PLY:'+JSON.stringify({action,session:device.Playback.session,...extra}));
  }
  async playbackRecordings() {
    if(!this.connected)throw new Error('サーバーに接続していません');
    const generation=this.generation;
    const response=await this.request(this.base+'/playback/recordings');
    if(!response.ok)throw new Error('録画一覧を取得できません');
    const data=await response.json();
    if(generation!==this.generation)throw new Error('接続先が変更されました');
    if(!Array.isArray(data))throw new Error('録画一覧の形式が不正です');
    return data;
  }
  reconnect(generation, message) {
    if (generation !== this.generation) return;
    this.connected = false;
    this.captionFeed.reset();
    clearInterval(this.refresh);
    clearInterval(this.guideRefresh);
    clearTimeout(this.retry);
    this.emit('status', { connected: false, message: message + ' · 3秒後に再接続' });
    this.retry = setTimeout(() => this.connect(this.base), 3000);
  }
  send(id, action) {
    if (!this.connected || this.socket?.readyState !== 1) throw new Error('サーバーに接続していません');
    this.socket.send(command(this.devices.get(id), action));
  }
}
return { Client };

});
