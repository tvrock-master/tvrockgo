'use strict';
const path=require('node:path');
const {spawn}=require('node:child_process');
const {validateSettings,buildLaunch}=require('./remote-viewer.cjs');

// Playback uses per-receiver credentials, never a normal device registration code.
class PlaybackViewer {
  constructor({client,viewer,emit,spawnProcess=spawn,schedule=setTimeout}) {
    Object.assign(this,{client,viewer,emit,spawnProcess,schedule});
    this.pending=new Map(); this.views=new Map(); this.closing=new Map();
  }
  send(value) {
    if (this.client.connected && this.client.socket?.readyState===1) this.client.socket.send('PLY:'+JSON.stringify(value));
  }
  status(message, phase, error) {
    this.emit('playback-status',{action:'status',session:message.session,deviceId:message.deviceId,phase,...(error?{error}: {})});
  }
  accept(message) {
    if (!message || typeof message!=='object') return;
    if (message.action==='offer') {
      if (!/^[a-f0-9]{64}$/.test(message.session||'') || !/^[a-f0-9]{32,64}$/i.test(message.device||'') || this.pending.has(message.session)) return;
      this.status(message,'received');
      try {
        const setting=validateSettings(this.viewer.settings(this.client.base,message.device));
        if (!setting.enabled || setting.transport!=='srt') throw new Error('PlaybackデバイスのSRT視聴アプリを設定してください');
        const used=[...this.viewer.views.values(),...this.viewer.pending.values(),...this.viewer.retiring.values(),...this.views.values(),...this.pending.values(),...this.closing.values()];
        if (used.some(v=>v.setting.port===setting.port)) throw new Error('このPCの受信ポートが使用中です');
        const timeout=setTimeout(()=>{this.pending.delete(message.session);this.status(message,'failed','Playback起動の応答がありません');this.send({action:'decline',session:message.session,error:'Playback起動の応答がありません'});},15000);
        timeout.unref?.();
        this.pending.set(message.session,{setting,device:message.device,timeout});
        this.send({action:'join',session:message.session,device:message.device,port:setting.port,srt:{latencyMs:setting.srt.latencyMs,passphrase:setting.srt.passphrase}});
        this.status(message,'waiting-launch');
      } catch(error) {
        this.status(message,'failed',error.message);
        this.send({action:'decline',session:message.session,error:error.message});
        this.emit('notice',error.message);
      }
    }
    if (message.action==='launch') {
      const pending=this.pending.get(message.session);
      if (!pending || message.device!==pending.device || message.port!==pending.setting.port || !/^[a-f0-9]{64}$/.test(message.receiver||'')) return;
      clearTimeout(pending.timeout);this.pending.delete(message.session);
      try {
        const launch=buildLaunch(pending.setting,this.client.base,{session:message.receiver,device:message.device,port:message.port});
        if (Number.isInteger(message.service) && message.service>0 && message.service<=65535) launch.args.push('/sid',String(message.service));
        const child=this.spawnProcess(launch.executable,launch.args,{shell:false,windowsHide:false,cwd:path.win32.dirname(launch.executable),stdio:'ignore'});
        const view={...pending,child};this.views.set(message.session,view);
        this.status(message,'waiting-receiver');
        child.once('error',error=>{if(this.views.get(message.session)!==view)return;this.status(message,'failed','視聴アプリを起動できません: '+error.message);this.emit('notice','再生デバイスの視聴アプリを起動できません: '+error.message);this.leave(message.session);});
        child.once('exit',()=>{clearTimeout(view.closeTimer);this.closing.delete(message.session);if(this.views.get(message.session)===view){this.status(message,'ended');this.leave(message.session);}});
      } catch(error) {this.status(message,'failed',error.message);this.emit('notice',error.message);this.leave(message.session);}
    }
    if (message.action==='close') this.close(message.session);
  }
  leave(session) {this.send({action:'decline',session});this.close(session);}
  close(session) {
    const pending=this.pending.get(session);if(pending)clearTimeout(pending.timeout);this.pending.delete(session);
    const view=this.views.get(session);this.views.delete(session);
    // The server delivers PowerOff to the plugin. Terminate only our own child
    // as a bounded fallback when the plugin or control connection is unavailable.
    if(view?.child?.exitCode===null) {
      this.closing.set(session,view);
      view.closeTimer=this.schedule(()=>{if(view.child.exitCode===null)view.child.kill();this.closing.delete(session);},5000);
      view.closeTimer?.unref?.();
    }
  }
  hasChildren() {return [...this.views.values(),...this.closing.values()].some(view=>view.child?.exitCode===null);}
  async shutdown() {
    for(const session of new Set([...this.pending.keys(),...this.views.keys()]))this.leave(session);
    const deadline=Date.now()+5500;
    while(this.hasChildren() && Date.now()<deadline)await new Promise(resolve=>setTimeout(resolve,25));
  }
}
module.exports={PlaybackViewer};
