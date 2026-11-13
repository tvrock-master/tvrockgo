'use strict';
const path = require('node:path');
const { spawn } = require('node:child_process');
const { serverURL } = require('../src/protocol.js');
const tokenPattern = /^[a-f0-9]{64}$/;
const registryPattern = /^[a-f0-9]{32,64}$/i;

// Windows CommandLineToArgvW quoting rules, applied only to user-owned local
// settings. There is no shell, interpolation, environment expansion or eval.
function splitArguments(text) {
  if (typeof text !== 'string' || text.length > 8192 || /[\0\r\n]/.test(text)) throw new Error('起動オプションが不正です');
  const args=[]; let i=0;
  while (i<text.length) {
    while (/\s/.test(text[i] || '') && i<text.length) i++;
    if (i===text.length) break;
    let arg='', quoted=false;
    while (i<text.length && (quoted || !/\s/.test(text[i]))) {
      let slashes=0;
      while (text[i]==='\\') { slashes++; i++; }
      if (text[i]==='"') {
        arg+='\\'.repeat(Math.floor(slashes/2));
        if (slashes%2) arg+='"'; else quoted=!quoted;
        i++;
      } else { arg+='\\'.repeat(slashes); if (i<text.length && (quoted || !/\s/.test(text[i]))) arg+=text[i++]; else break; }
    }
    if (quoted) throw new Error('起動オプションの引用符が閉じていません');
    args.push(arg);
  }
  if (args.some(a=>/^[-/](host|pid|parent|did|port|p|s)$/i.test(a))) throw new Error('/host /pid /parent /DID /port /s は自動管理します');
  return args;
}
function validateSettings(value) {
  if (!value || typeof value!=='object' || typeof value.enabled!=='boolean') throw new Error('視聴アプリの設定が不正です');
  const executable=typeof value.executable==='string' ? value.executable.trim() : '';
  const options=typeof value.options==='string' ? value.options : '';
  const port=Number(value.port);
  if (!Number.isInteger(port) || port<1 || port>65535) throw new Error('ポートは1～65535で指定してください');
  if (executable && (!path.win32.isAbsolute(executable) || !/\.exe$/i.test(executable) || /[\0\r\n"]/.test(executable))) throw new Error('実行ファイルは.exeの絶対パスで指定してください');
  if (value.enabled && !executable) throw new Error('視聴アプリの実行ファイルを指定してください');
  splitArguments(options);
  const stream={currentServiceOnly:true,caption:true,dataBroadcast:true,oneSeg:false};
  if (value.stream!==undefined) {
    if (!value.stream || typeof value.stream!=='object' || Array.isArray(value.stream)) throw new Error('送信設定が不正です');
    for (const key of Object.keys(stream)) if (value.stream[key]!==undefined) {
      if (typeof value.stream[key]!=='boolean') throw new Error('送信設定はON/OFFで指定してください');
      stream[key]=value.stream[key];
    }
  }
  if(stream.oneSeg)stream.currentServiceOnly=true;
  const transport=value.transport ?? 'udp';
  if (!['udp','srt'].includes(transport)) throw new Error('配信方式が不正です');
  let srt;
  if (transport==='srt') {
    const basePort=value.srt?.basePort ?? 3230, latencyMs=value.srt?.latencyMs ?? 120, passphrase=value.srt?.passphrase ?? '';
    if (!Number.isInteger(basePort) || basePort<1 || basePort>65526 || port<basePort || port>=basePort+10) throw new Error('SRT受信ポートは基準ポートから10ポートの範囲で指定してください');
    if (!Number.isInteger(latencyMs) || latencyMs<0 || latencyMs>60000) throw new Error('SRT遅延は0～60000ミリ秒で指定してください');
    if (typeof passphrase!=='string' || (passphrase!=='' && !/^[\x20-\x7e]{10,79}$/.test(passphrase))) throw new Error('SRTパスフレーズは10～79文字の印字可能ASCIIで指定してください');
    srt={basePort,latencyMs,passphrase};
    const args=splitArguments(options);
    if (args.some(a=>/^[-/](chi|chspace|ch)$/i.test(a))) throw new Error('SRTの受信チャンネルは自動管理します');
    const driverIndex=args.findIndex(a=>/^[-/]d$/i.test(a));
    if (driverIndex<0 || !/\.dll$/i.test(args[driverIndex+1]||'')) throw new Error('SRT受信ドライバーを /d で指定してください');
  }
  return { enabled:value.enabled, executable, options, port, stream, transport, ...(srt?{srt}: {}) };
}
function buildLaunch(setting, base, view) {
  const s=validateSettings(setting);
  if (!s.enabled || !tokenPattern.test(view.session) || !registryPattern.test(view.device) || Number(view.port)!==s.port) throw new Error('視聴セッションが設定と一致しません');
  const server=serverURL(base);
  const tuning=s.transport==='srt' ? ['/chspace','0','/chi',String(s.port-s.srt.basePort)] : ['/port',String(s.port)];
  return { executable:s.executable, args:[...splitArguments(s.options), '/host',server,'/pid',view.session,'/parent',view.device,'/DID','A',...tuning] };
}
class RemoteViewer {
  constructor({ client, config, save, emit, spawnProcess=spawn }) {
    Object.assign(this,{client,config,save,emit,spawnProcess});
    this.views=new Map(); this.pending=new Map(); this.retiring=new Map(); this.closed=false; this.schedules=new Set();
    this.heartbeat=setInterval(()=>{
      for (const view of this.views.values()) this.send({action:'ping',session:view.session});
    },4000);
    this.heartbeat.unref?.();
  }
  settings(base, registry) { return this.config.viewerByURL?.[serverURL(base)]?.[registry]; }
  configured(registry) { return !!this.settings(this.client.base,registry)?.enabled; }
  saveSettings(base,registry,value) {
    if (!registryPattern.test(registry)) throw new Error('デバイス識別子が不正です');
    base=serverURL(base);
    if ([...this.views.values()].some(v=>v.base===base && v.device===registry)) throw new Error('視聴を終了してから設定を変更してください');
    const checked=validateSettings(value);
    const old=this.config.viewerByURL;
    this.config.viewerByURL={...old,[base]:{...old?.[base],[registry]:checked}};
    try { this.save(); } catch (error) { this.config.viewerByURL=old; throw error; }
    this.publish(); return checked;
  }
  send(message) {
    if (!this.client.connected || this.client.socket?.readyState!==1) return false;
    this.client.socket.send('WAT:'+JSON.stringify(message)); return true;
  }
  toggle(device, schedule) {
    if (this.closed || !device) throw new Error('現在の状態では視聴操作できません');
    const registry=device.RegistryCode;
    const view=this.views.get(registry);
    if (view) { this.stop(view); return; }
    if (![0,2,3].includes(device.State)) throw new Error('現在の状態では視聴操作できません');
    if (this.retiring.has(registry)) throw new Error('視聴アプリの終了を待っています');
    if (this.pending.has(registry)) throw new Error('視聴アプリの起動を待っています');
    const base=serverURL(this.client.base), setting=validateSettings(this.settings(base,registry));
    if(this.externalPorts?.().includes(setting.port))throw new Error('このPCで受信ポートを使用しています');
    if (!setting.enabled || !registryPattern.test(registry)) throw new Error('視聴アプリを設定してください');
    for (const other of [...this.views.values(),...this.retiring.values(),...this.pending.values()]) {
      if (other.setting.port===setting.port) throw new Error('このPCで受信ポートが重複しています');
    }
    const timeout=setTimeout(()=>{ this.pending.delete(registry); this.emit('notice','視聴開始の応答がありません'); this.publish(); },15000);
    this.pending.set(registry,{base,setting,timeout});
    if (!this.send({action:'start',device:registry,port:setting.port,stream:setting.stream,transport:setting.transport,...(setting.srt?{srt:{latencyMs:setting.srt.latencyMs,passphrase:setting.srt.passphrase}}: {}),...(schedule?{schedule}: {})})) { clearTimeout(timeout); this.pending.delete(registry); throw new Error('サーバーに接続していません'); }
    this.publish();
  }
  accept(message) {
    if (!message || typeof message!=='object') return;
    if (message.action==='schedule') {
      if (this.closed || !this.client.connected || !tokenPattern.test(message.schedule||'') || !registryPattern.test(message.device||'') || this.schedules.has(message.schedule)) return;
      if (!this.configured(message.device)) return;
      const device=[...(this.client.devices?.values()||[])].find(d=>d.RegistryCode===message.device);
      if (!device) return;
      const existing=this.views.get(message.device);
      if (existing) {
        this.schedules.add(message.schedule);
        this.send({action:'adopt',session:existing.session,schedule:message.schedule});
        return;
      }
      if(this.pending.has(message.device) || this.retiring.has(message.device)) return;
      try {this.toggle(device,message.schedule);this.schedules.add(message.schedule);}
      catch(error) {this.schedules.add(message.schedule);this.emit('notice','予約視聴を開始できません: '+error.message);}
      return;
    }
    if (!tokenPattern.test(message.session || '') || !registryPattern.test(message.device || '')) {
      if (typeof message.error==='string') {
        for (const p of this.pending.values()) clearTimeout(p.timeout);
        this.pending.clear(); this.emit('notice',message.error.slice(0,300)); this.publish();
      }
      return;
    }
    const registry=message.device;
    let view=this.views.get(registry);
    const pending=this.pending.get(registry);
    if (!view && pending && pending.base===this.client.base && message.port===pending.setting.port) {
      clearTimeout(pending.timeout); this.pending.delete(registry);
      view={session:message.session,device:registry,base:pending.base,setting:pending.setting,phase:'preparing',closing:false};
      this.views.set(registry,view);
      try {
        const launch=buildLaunch(view.setting,view.base,message);
        // The executable and all user options come exclusively from pending.setting.
        // None of the server's app/path/options fields are read.
        view.child=this.spawnProcess(launch.executable,launch.args,{shell:false,windowsHide:false,cwd:path.win32.dirname(launch.executable),stdio:'ignore'});
        view.child.once('error',error=>{ this.emit('notice','視聴アプリを起動できません: '+error.message); this.stop(view); });
        view.child.once('exit',()=>{ clearTimeout(view.closeTimer); clearTimeout(view.disconnectTimer); if (this.retiring.get(registry)===view) this.retiring.delete(registry); if (this.views.get(registry)===view) { this.send({action:'stop',session:view.session}); this.views.delete(registry); } this.publish(); });
      } catch (error) { this.emit('notice',error.message); this.stop(view); }
    }
    if (!view || message.session!==view.session || message.port!==view.setting.port) return;
    view.phase=String(message.phase || '').slice(0,50);
    view.graceSeconds=Math.max(1,Math.min(86400,Number(message.graceSeconds)||15));
    if (message.error) this.emit('notice',String(message.error).slice(0,300));
    if (view.phase==='stopped') this.retire(view);
    this.publish();
  }
  stop(view) {
    if (this.views.get(view.device)!==view) return;
    view.closing=true; view.phase='stopping';
    this.send({action:'stop',session:view.session});
    // PowerOff is delivered to this sibling by the server first. Kill only the
    // child we spawned, and only as a bounded fallback if graceful exit fails.
    clearTimeout(view.closeTimer);
    view.closeTimer=setTimeout(()=>{ this.closeChild(view); if (this.views.get(view.device)===view) this.views.delete(view.device); this.publish(); },5000);
    view.closeTimer.unref?.(); this.publish();
  }
  retire(view) {
    if (this.views.get(view.device)===view) this.views.delete(view.device);
    if (view.child?.exitCode===null) {
      this.retiring.set(view.device,view);
      if (!view.closeTimer) { view.closeTimer=setTimeout(()=>this.closeChild(view),5000); view.closeTimer.unref?.(); }
    }
  }
  hasChildren() { return [...this.views.values(),...this.retiring.values()].some(v=>v.child?.exitCode===null); }
  closeChild(view) { clearTimeout(view.closeTimer); clearTimeout(view.disconnectTimer); if (view.child && view.child.exitCode===null) view.child.kill(); }
  connection(connected) {
    for (const view of this.views.values()) {
      if (connected && view.base===this.client.base) {
        clearTimeout(view.disconnectTimer); view.disconnectTimer=null;
        this.send({action:view.closing?'stop':'resume',session:view.session});
      } else if (!view.disconnectTimer) {
        // Reconnection attempts also emit disconnected status. They must not
        // restart the grace period and keep an orphan viewer alive forever.
        view.disconnectTimer=setTimeout(()=>{ this.closeChild(view); if (this.views.get(view.device)===view) this.views.delete(view.device); this.publish(); },(view.graceSeconds||15)*1000);
        view.disconnectTimer.unref?.();
      }
    }
  }
  capture(registry) { const v=this.views.get(registry); if (!v) return false; this.send({action:'capture',session:v.session}); return true; }
  publish() {
    const configured=Object.entries(this.config.viewerByURL?.[this.client.base] || {}).filter(([,s])=>s.enabled).map(([rid])=>rid);
    this.emit('viewer-state',{configured,views:[...this.views.values()].map(v=>({device:v.device,phase:v.phase})),pending:[...this.pending.keys(),...this.retiring.keys()]});
  }
  shutdown() {
    if (this.shutdownPromise) return this.shutdownPromise;
    this.closed=true; clearInterval(this.heartbeat);
    for (const p of this.pending.values()) clearTimeout(p.timeout);
    this.pending.clear(); for (const view of this.views.values()) this.stop(view);
    const children=[...this.views.values(),...this.retiring.values()], deadline=Date.now()+5000;
    this.shutdownPromise=new Promise(resolve=>{
      const check=()=>{
        if (children.every(v=>!v.child || v.child.exitCode!==null)) { resolve(); return; }
        if (Date.now()>=deadline) { for (const view of children) this.closeChild(view); resolve(); return; }
        setTimeout(check,50);
      }; check();
    });
    return this.shutdownPromise;
  }
  changeServer() {
    for (const view of [...this.views.values()]) { this.stop(view); this.retire(view); }
    this.schedules.clear(); this.views.clear(); for (const p of this.pending.values()) clearTimeout(p.timeout); this.pending.clear(); this.publish();
  }
}
module.exports={splitArguments,validateSettings,buildLaunch,RemoteViewer};
