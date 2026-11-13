const {test}=require('node:test');
const assert=require('node:assert/strict');
const {EventEmitter}=require('node:events');
const {CaptionFeed}=require('../src/captions.js');
const {Client}=require('../src/client.js');
const {PlaybackViewer}=require('../electron/playback-viewer.cjs');
const {PlaybackClock}=require('../src/clock.js');
const {PlaybackStatus}=require('../src/playback-status.js');

test('playback receipt status survives device selection and retains local failures until the next request',()=>{
  const status=new PlaybackStatus();
  assert.match(status.summary(true),/再生要求待ち/);
  assert.match(status.summary(false),/未接続/);
  status.receive({action:'request',session:'one',deviceId:'PBK01'});
  assert.match(status.summary(true),/PBK01.*再生要求受信済み.*ファイル解析中/);
  status.receive({action:'status',session:'one',phase:'failed',error:'視聴アプリを設定してください'});
  status.receive({action:'offer',session:'one'});
  status.receive({action:'close',session:'one'});
  assert.match(status.summary(true),/失敗.*視聴アプリを設定/);
  status.receive({action:'request',session:'two',deviceId:'PBK02'});
  assert.doesNotMatch(status.summary(true),/PBK01/);
  for(const [phase,label] of [['waiting-launch','視聴アプリ起動待ち'],['waiting-receiver','視聴アプリ接続待ち'],['waiting-srt','SRT接続待ち'],['playing','再生中'],['paused','一時停止中']]){
    status.receive({action:'status',session:'two',phase});assert.ok(status.summary(true).includes(label));
  }
  status.receive({action:'status',session:'two',phase:'playing',generation:3});
  status.receive({action:'status',session:'two',phase:'waiting-srt',generation:2});
  assert.match(status.summary(true),/再生中/);
  status.receive({action:'caption',session:'other',caption:{Text:'字幕'}});
  assert.equal(status.sessions.size,1);
  status.receive({action:'close',session:'two'});assert.match(status.summary(true),/再生終了/);
  status.reset();assert.match(status.summary(true),/再生要求待ち/);
  status.receive({error:'要求を開始できません'});assert.doesNotMatch(status.summary(true),/受信済み/);
});

test('local Playback validation failure is available to the persistent status bar',()=>{
  const events=[],sent=[];
  const client={connected:true,base:'http://localhost',socket:{readyState:1,send:text=>sent.push(JSON.parse(text.slice(4)))}};
  const playback=new PlaybackViewer({client,viewer:{settings:()=>undefined},emit:(type,value)=>events.push({type,value})});
  playback.accept({action:'offer',session:'a'.repeat(64),device:'b'.repeat(32),deviceId:'PBK01'});
  assert.equal(sent[0].action,'decline');
  const failure=events.find(event=>event.type==='playback-status'&&event.value.phase==='failed');
  assert.equal(failure.value.deviceId,'PBK01');
  assert.ok(failure.value.error);
  assert.equal(playback.views.size,0);
});

test('status messages replace normal content temporarily and repeated reports do not extend them',()=>{
  let now=0;
  const status=new PlaybackStatus(()=>now,10000);
  assert.equal(status.message(),'');
  status.receive({action:'request',session:'one',deviceId:'PBK01'});
  assert.match(status.message(),/再生要求受信済み/);
  status.receive({action:'status',session:'one',phase:'failed',error:'SRT接続に失敗しました'});
  assert.equal(status.message(),'SRT接続に失敗しました');
  now=9000;status.receive({action:'status',session:'one',phase:'failed',error:'SRT接続に失敗しました'});
  now=10000;assert.equal(status.message(),'');
  status.show('操作できません');assert.equal(status.message(),'操作できません');
  now=15000;status.show('操作できません');
  now=20000;assert.equal(status.message(),'');
});

test('playback clock advances between reports, corrects to new reports and freezes while paused',()=>{
  let tick=100;
  const clock=new PlaybackClock(()=>tick);
  const state={session:'one',generation:1,revision:1,phase:'playing',positionMs:15000,durationMs:3600000};
  clock.sync(state);tick+=1250;assert.equal(clock.now(),16250);
  clock.sync({...state});tick+=250;assert.equal(clock.now(),16500,'cached device publication must not rewind the clock');
  clock.sync({...state,revision:2,positionMs:16000});assert.equal(clock.now(),16000);
  tick+=500;assert.equal(clock.now(),16500);
  clock.sync({...state,revision:3,generation:2,phase:'paused',positionMs:17000});tick+=5000;assert.equal(clock.now(),17000);
  clock.sync({...state,revision:4,generation:3,positionMs:3599900});tick+=500;assert.equal(clock.now(),3600000);
  clock.sync({...state,revision:5,positionMs:30000});tick+=400;clock.freeze();tick+=1000;assert.equal(clock.now(),30400);
  clock.sync({...state,session:'two',revision:1,positionMs:0});assert.equal(clock.now(),0);
});

test('only allocated Playback devices enter the device list and closing removes them',()=>{
  const client=new Client(()=>{});
  client.devices.set('REC01',{ID:'REC01'});
  client.applyPlaybackDevices([{ID:'PBK01',Kind:'playback',Playback:{session:'one',generation:1}},{ID:'PBK02',Kind:'playback'}]);
  assert.deepEqual([...client.devices.keys()],['REC01','PBK01']);
  client.applyPlaybackDevices([]);
  assert.deepEqual([...client.devices.keys()],['REC01']);
});

test('growing playback clock freezes while waiting for appended recording data',()=>{
  let tick=0;const clock=new PlaybackClock(()=>tick);
  const state={session:'growing',generation:1,phase:'playing',positionMs:1000,durationMs:5000,waitingForData:true};
  clock.sync(state);tick=10000;assert.equal(clock.now(),1000);
  clock.sync({...state,waitingForData:false});tick+=500;assert.equal(clock.now(),1500);
});

test('Playback captions are isolated by session, receiver and seek generation',()=>{
  let latest;
  const feed=new CaptionFeed(null,value=>{latest=value;});
  let device={ID:'PBK01',Kind:'playback',Playback:{session:'one',generation:1}};
  feed.syncDevices([device]);feed.select(device);
  feed.receivePlayback({device:'PBK01',session:'one',generation:1,caption:{Text:'字幕',Pts:90,Lang:1}});
  assert.equal(latest.items[0].Text,'字幕');
  device={...device,Playback:{session:'one',generation:2}};
  feed.syncDevices([device]);feed.select(device);
  feed.receivePlayback({device:'PBK01',session:'one',generation:1,caption:{Text:'古い字幕'}});
  feed.receivePlayback({device:'PBK02',session:'one',generation:2,caption:{Text:'別の受信元'}});
  assert.equal(latest.items.length,0);
  feed.receivePlayback({device:'PBK01',session:'one',generation:2,caption:{Text:'移動後',HistoryID:900,Pts:90,Lang:1}});
  assert.equal(latest.items[0].Text,'移動後');
});

test('Playback launch uses local settings and per-receiver token, and closes only its child',()=>{
  const sent=[],spawned=[],events=[];
  const scheduled=[];
  const session='a'.repeat(64),receiver='b'.repeat(64),registry='c'.repeat(32);
  const client={connected:true,base:'http://localhost',socket:{readyState:1,send:value=>sent.push(JSON.parse(value.slice(4)))}};
  const setting={enabled:true,executable:'C:\\TVTest\\TVTest.exe',options:'/d BonDriver_SRT.dll /nd',port:3232,transport:'srt',srt:{basePort:3230,latencyMs:120,passphrase:''}};
  const viewer={settings:()=>setting,views:new Map(),pending:new Map(),retiring:new Map()};
  const child=new EventEmitter();child.exitCode=null;let killed=0;child.kill=()=>{killed++;};
  const playback=new PlaybackViewer({client,viewer,emit:(type,value)=>events.push({type,value}),schedule:callback=>{scheduled.push(callback);return null;},spawnProcess:(...args)=>{spawned.push(args);return child;}});
  playback.accept({action:'offer',session,device:registry});
  assert.equal(sent[0].action,'join');
  assert.equal(events.at(-1).value.phase,'waiting-launch');
  playback.accept({action:'launch',session,receiver,device:registry,port:3232,service:101,executable:'evil.exe'});
  assert.equal(spawned[0][0],setting.executable);
  assert.equal(events.at(-1).value.phase,'waiting-receiver');
  assert.ok(spawned[0][1].includes(receiver));
  assert.ok(spawned[0][1].includes('/sid'));
  assert.ok(!spawned[0][1].includes(session));
  playback.accept({action:'status',session,phase:'idle'});
  assert.equal(playback.views.size,1,'待受中も視聴アプリを維持する');assert.equal(killed,0);
  playback.accept({action:'close',session:'d'.repeat(64)});assert.equal(killed,0);
  playback.accept({action:'close',session});assert.equal(killed,0);
  scheduled[0]();assert.equal(killed,1);
  playback.shutdown();
});
