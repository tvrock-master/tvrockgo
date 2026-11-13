const {test}=require('node:test');
const assert=require('node:assert/strict');
const {EventEmitter}=require('node:events');
const {splitArguments,validateSettings,buildLaunch,RemoteViewer}=require('../electron/remote-viewer.cjs');
const registry='a'.repeat(32),session='b'.repeat(64),base='http://192.0.2.1';
const setting={enabled:true,executable:'C:\\TVTest\\TVTest.exe',options:'/d "BonDriver_UDP.dll" /nd',port:1234};
test('SRT uses listener channel index and validates options without command-line secrets',()=>{
 const srt={...setting,transport:'srt',options:'/d BonDriver_SRT.dll /nd',port:3232,srt:{basePort:3230,latencyMs:240,passphrase:'replace-this-secret'}};
 const launch=buildLaunch(srt,base,{session,device:registry,port:3232});
 assert.deepEqual(launch.args.slice(-4),['/chspace','0','/chi','2']);
 assert.ok(!launch.args.includes('replace-this-secret'));
 assert.ok(!launch.args.includes('/port'));
 assert.throws(()=>validateSettings({...srt,port:3240}));
 assert.throws(()=>validateSettings({...srt,srt:{latencyMs:60001}}));
 assert.throws(()=>validateSettings({...srt,srt:{passphrase:'short'}}));
 assert.throws(()=>validateSettings({...srt,options:'/d BonDriver_SRT.dll /chi 3'}));
 assert.equal(validateSettings(setting).transport,'udp');
});
test('SRT start sends only transport parameters to the server',t=>{
 const f=fixture(t);
 f.config.viewerByURL[base][registry]={...setting,transport:'srt',options:'/d BonDriver_SRT.dll /nd',port:3230,srt:{latencyMs:120,passphrase:'replace-this-secret',basePort:3230}};
 f.viewer.toggle({RegistryCode:registry,State:0});
 const message=JSON.parse(f.sent[0].slice(4));
 assert.equal(message.transport,'srt');
 assert.deepEqual(message.srt,{latencyMs:120,passphrase:'replace-this-secret'});
 assert.equal(message.executable,undefined);
});
test('SRT sessions can receive on different ports while duplicate pending ports are rejected',t=>{
 const f=fixture(t), second='c'.repeat(32), third='d'.repeat(32);
 const srt={...setting,transport:'srt',options:'/d BonDriver_SRT.dll',port:3230};
 f.config.viewerByURL[base][registry]=srt;
 f.config.viewerByURL[base][second]={...srt,port:3231};
 f.config.viewerByURL[base][third]={...srt,port:3231};
 f.viewer.toggle({RegistryCode:registry,State:0});
 f.viewer.toggle({RegistryCode:second,State:0});
 assert.throws(()=>f.viewer.toggle({RegistryCode:third,State:0}),/受信ポートが重複/);
 assert.equal(f.sent.length,2);
 assert.deepEqual(f.sent.map(s=>JSON.parse(s.slice(4)).port),[3230,3231]);
});
test('local options use Windows quoting and never a shell',()=>{
 assert.deepEqual(splitArguments('/d "C:\\My Drivers\\UDP.dll" /nd'),['/d','C:\\My Drivers\\UDP.dll','/nd']);
 assert.deepEqual(splitArguments('"$(evil) & echo"'),['$(evil) & echo']);
 assert.throws(()=>splitArguments('/HOST evil'),/自動管理/);
 assert.throws(()=>splitArguments('"unterminated'),/引用符/);
 assert.throws(()=>validateSettings({...setting,executable:'cmd.bat'}));
 assert.throws(()=>validateSettings({...setting,port:0}));
});
test('network fields cannot change executable, options, server URL or port',()=>{
 const result=buildLaunch(setting,base,{session,device:registry,port:1234,executable:'C:\\evil.exe',options:'& evil',server:'http://evil'});
 assert.equal(result.executable,setting.executable);
 assert.deepEqual(result.args,['/d','BonDriver_UDP.dll','/nd','/host',base,'/pid',session,'/parent',registry,'/DID','A','/port','1234']);
 assert.throws(()=>buildLaunch(setting,base,{session:'" & evil',device:registry,port:1234}));
 assert.throws(()=>buildLaunch(setting,base,{session,device:registry,port:4444}));
});
function fixture(t){
 const sent=[],spawns=[],events=[];
 const client={base,connected:true,socket:{readyState:1,send:s=>sent.push(s)}};
 const config={viewerByURL:{[base]:{[registry]:setting}}};
 const viewer=new RemoteViewer({client,config,save(){},emit:(...e)=>events.push(e),spawnProcess:(exe,args,options)=>{const child=new EventEmitter();child.exitCode=null;child.kill=()=>{child.exitCode=0;};spawns.push({exe,args,options,child});return child;}});
 t.after(()=>{viewer.shutdown();for(const v of viewer.views.values())viewer.closeChild(v)});
 return {viewer,client,config,sent,spawns,events};
}
test('viewer only starts for a matching local request and isolates settings per URL',t=>{
 const {viewer,spawns,sent}=fixture(t);
 viewer.accept({session,device:registry,port:1234,phase:'preparing'});
 assert.equal(spawns.length,0);
 viewer.toggle({RegistryCode:registry,State:3});
 viewer.accept({session,device:registry,port:1234,phase:'preparing',executable:'evil',options:'evil'});
 assert.equal(spawns.length,1);assert.equal(spawns[0].options.shell,false);
 assert.equal(spawns[0].exe,setting.executable);
 assert.ok(sent.every(s=>!s.includes('TVTest.exe') && !s.includes('BonDriver')));
 assert.equal(viewer.settings('http://other',registry),undefined);
 viewer.accept({session:'c'.repeat(64),device:registry,port:1234,phase:'stopped'});
 assert.equal(viewer.views.size,1);assert.equal(spawns[0].child.exitCode,null);
});
test('視聴アプリの手動終了で対応セッションを停止し、再接続はトークンで再開する',t=>{
 const {viewer,sent,spawns}=fixture(t);
 viewer.toggle({RegistryCode:registry,State:2});
 viewer.accept({session,device:registry,port:1234,phase:'receiving'});
 viewer.connection(false);viewer.connection(true);
 assert.ok(sent.some(s=>s.includes('"action":"resume"') && s.includes(session)));
 spawns[0].child.exitCode=0;spawns[0].child.emit('exit',0);
 assert.ok(sent.some(s=>s.includes('"action":"stop"') && s.includes(session)));
 assert.equal(viewer.views.size,0);
});
test('settings updates are local and URL scoped',t=>{
 const {viewer,config,sent}=fixture(t);
 viewer.saveSettings('http://second/',registry,{...setting,port:2222});
 assert.equal(config.viewerByURL['http://second'][registry].port,2222);
 assert.equal(config.viewerByURL[base][registry].port,1234);
 assert.equal(sent.length,0);
});

test('サーバーの停止応答は視聴アプリの正常終了を待つ',t=>{
 const {viewer,spawns}=fixture(t);
 viewer.toggle({RegistryCode:registry,State:2});
 viewer.accept({session,device:registry,port:1234,phase:'receiving'});
 viewer.stop(viewer.views.get(registry));
 viewer.accept({session,device:registry,port:1234,phase:'stopped'});
 assert.equal(spawns[0].child.exitCode,null,'停止応答で視聴アプリが即時強制終了された');
 assert.equal(viewer.retiring.size,1);
 assert.throws(()=>viewer.toggle({RegistryCode:registry,State:0}),/終了/);
 spawns[0].child.exitCode=0;spawns[0].child.emit('exit',0);
 assert.equal(viewer.retiring.size,0);
});

test('failed reconnect attempts do not extend the disconnected viewer grace',t=>{
 t.mock.timers.enable({apis:['setTimeout']});
 const {viewer,spawns,client}=fixture(t);
 viewer.toggle({RegistryCode:registry,State:2});
 viewer.accept({session,device:registry,port:1234,phase:'receiving',graceSeconds:5});
 client.connected=false; viewer.connection(false);
 t.mock.timers.tick(3000); viewer.connection(false);
 t.mock.timers.tick(2001);
 assert.equal(spawns[0].child.exitCode,0,'retry reset the grace deadline');
 assert.equal(viewer.views.size,0);
});

test('stream defaults and options are local, validated and sent as booleans',t=>{
 assert.deepEqual(validateSettings(setting).stream,{currentServiceOnly:true,caption:true,dataBroadcast:true,oneSeg:false});
 assert.throws(()=>validateSettings({...setting,stream:{caption:'false'}}));
 const {viewer,config,sent}=fixture(t);
 config.viewerByURL[base][registry]={...setting,stream:{currentServiceOnly:false,caption:false,dataBroadcast:true,oneSeg:true}};
 viewer.toggle({RegistryCode:registry,State:2});
 const command=JSON.parse(sent[0].slice(4));
 assert.deepEqual(command.stream,{currentServiceOnly:true,caption:false,dataBroadcast:true,oneSeg:true});
 assert.equal(command.executable,undefined);assert.equal(command.options,undefined);
});

function scheduledFixture(t) {
 const f=fixture(t);f.client.devices=new Map([['device',{RegistryCode:registry,State:2}]]);return f;
}
test('schedule starts every configured PC once using only its local executable',t=>{
 const a=scheduledFixture(t),b=scheduledFixture(t),schedule='c'.repeat(64);
 for(const f of [a,b]) {
  f.viewer.accept({action:'schedule',schedule,device:registry,executable:'evil.exe'});
  f.viewer.accept({action:'schedule',schedule,device:registry});
  assert.equal(f.sent.filter(s=>JSON.parse(s.slice(4)).action==='start').length,1);
  assert.equal(JSON.parse(f.sent[0].slice(4)).schedule,schedule);
  f.viewer.accept({session,device:registry,port:1234,phase:'preparing'});
  assert.equal(f.spawns.length,1);assert.equal(f.spawns[0].exe,setting.executable);
  f.spawns[0].child.exitCode=0;f.spawns[0].child.emit('exit',0);
  f.viewer.accept({action:'schedule',schedule,device:registry});
  assert.equal(f.spawns.length,1,'manual close was immediately reopened');
 }
});
test('schedule ignores other URLs, disabled devices and invalid requests',t=>{
 const f=scheduledFixture(t),notice={action:'schedule',schedule:'c'.repeat(64),device:registry};
 f.config.viewerByURL[base][registry]={...setting,enabled:false};f.viewer.accept(notice);
 f.client.base='http://other';f.viewer.accept(notice);
 f.client.base=base;f.config.viewerByURL[base][registry]=setting;
 f.viewer.accept({...notice,schedule:'evil'});f.viewer.accept({...notice,device:'evil'});
 assert.equal(f.sent.length,0);assert.equal(f.spawns.length,0);
});
test('schedule reuses a manual viewer instead of toggling it off',t=>{
 const f=scheduledFixture(t);
 f.viewer.toggle({RegistryCode:registry,State:2});
 f.viewer.accept({session,device:registry,port:1234,phase:'receiving'});
 f.viewer.accept({action:'schedule',schedule:'c'.repeat(64),device:registry});
 assert.equal(f.spawns.length,1);assert.equal(f.viewer.views.get(registry).closing,false);
 assert.ok(!f.sent.some(s=>JSON.parse(s.slice(4)).action==='stop'));
});
