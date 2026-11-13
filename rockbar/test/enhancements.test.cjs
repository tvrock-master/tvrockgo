const {test}=require('node:test');
const assert=require('node:assert/strict');
const {ServerClock}=require('../src/clock.js');
const {CaptionFeed,programKey}=require('../src/captions.js');
const {services}=require('../src/programs.js');
const {tuneCommands}=require('../src/protocol.js');
const {Client}=require('../src/client.js');
const dev={ID:'T1',State:2,RegistryCode:'r1',CurrentService:65537,CurrentProgram:{Sid:65537,Eid:10,Start:100}};
const packet=(HistoryID,Text,extra={})=>({DeviceID:dev.ID,Sid:65537,Eid:10,Start:100,HistoryID,Text,Pts:50,Lang:0,...extra});
const until=async fn=>{for(let i=0;i<100;i++){if(fn())return;await new Promise(r=>setTimeout(r,5));}assert.fail('event timed out');};
test('server clock uses server epoch and monotonic elapsed time, even for large local clock differences',()=>{
 let tick=50;const clock=new ServerClock(()=>tick);assert.equal(clock.now(),null);
 clock.sync(1700000000000);tick+=1234;assert.equal(clock.now(),1700000001234);
 assert.equal(clock.sync(NaN),false);assert.equal(clock.now(),1700000001234);clock.reset();assert.equal(clock.now(),null);
});
test('satellite list toggles without tuning and accepts BS to CS commands only for satellite devices',()=>{
 const stations=[{Sid:131073,Tsp:2,View:1,No:1},{Sid:196941,Tsp:3,View:1,No:1},{Sid:65537,Tsp:1,View:1,No:1}];
 const satellite={...dev,TunerType:1,CurrentService:131073};
 assert.deepEqual(services(satellite,stations,3).map(s=>s.Sid),[196941]);
 assert.equal(satellite.CurrentService,131073);
 assert.equal(JSON.parse(tuneCommands(satellite,196941,stations)[0].slice(4)).message,'196941');
 assert.equal(JSON.parse(tuneCommands({...satellite,CurrentService:196941},131073,stations)[0].slice(4)).message,'131073');
 assert.throws(()=>tuneCommands(dev,196941,stations));
 assert.throws(()=>tuneCommands({...satellite,State:3},196941,stations));
 assert.equal(196941&0xffff,333);
});
test('client routes background device captions into their own buffers and clears them on disconnect',async()=>{
 const other={...dev,ID:'T2'},handlers={};
 const socket={readyState:1,addEventListener:(type,handler)=>handlers[type]=handler,send:()=>{},close:()=>{}};
 const client=new Client(()=>{},async url=>Response.json(url.endsWith('/dvs')?[dev,other]:url.endsWith('/stl')?[]:{}),()=>socket);
 try {
  await client.connect('http://test');handlers.open();client.watchDevice(dev.ID);
  const receive=value=>handlers.message({data:'cap:'+JSON.stringify(value)});
  receive(packet(1,'REC01'));receive(packet(2,'REC02',{DeviceID:other.ID}));
  assert.equal(client.captionFeed.items[0].Text,'REC01');
  client.watchDevice(other.ID);assert.equal(client.captionFeed.items[0].Text,'REC02');
  client.watchDevice(dev.ID);assert.equal(client.captionFeed.items[0].Text,'REC01');
  handlers.message({data:'dev:'+JSON.stringify({...other,State:0})});
  assert.equal(client.captionFeed.buffers.has(other.ID),false);
 } finally { client.close(); }
 assert.equal(client.captionFeed.buffers.size,0);
});

test('device buffers keep identical broadcasts separate across repeated selection changes',()=>{
 const events=[];
 const feed=new CaptionFeed(()=>assert.fail('shared caption history must not be requested'),value=>events.push(value));
 const other={...dev,ID:'T2'};
 feed.syncDevices([dev,other]);feed.select(dev);
 feed.receive(packet(1,'同じ字幕'));feed.receive(packet(2,'同じ字幕',{DeviceID:'T2'}));
 assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕']);
 const count=events.length;
 feed.receive(packet(3,'別の続き',{DeviceID:'T2'}));
 assert.equal(events.length,count,'background captions do not publish into the selected view');
 for(let n=0;n<5;n++) {
  feed.select(other);assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕別の続き']);
  assert.equal(events.at(-1).deviceID,'T2');
  feed.select(dev);assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕']);
 }
 feed.receive(packet(4,'',{DeviceID:'T2',Clear:true}));
 feed.receive(packet(5,'次の字幕',{DeviceID:'T2'}));
 feed.select(other);assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕別の続き','次の字幕']);
 feed.select(dev);feed.receive(packet(6,'の続き'));
 assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕の続き']);
 feed.receive(packet(7,'送信元なし',{DeviceID:undefined}));
 feed.receive(packet(8,'不明なデバイス',{DeviceID:'unknown'}));
 feed.receive(packet(9,'別番組',{Eid:11}));
 assert.deepEqual(feed.items.map(i=>i.Text),['同じ字幕の続き']);
 feed.reset();assert.equal(feed.buffers.size,0);assert.deepEqual(feed.items,[]);
});

test('program changes, stopped and removed devices discard only the affected buffer',()=>{
 const feed=new CaptionFeed(null,()=>{}),other={...dev,ID:'T2'};
 feed.syncDevices([dev,other]);feed.select(dev);
 feed.receive(packet(1,'旧番組'));feed.receive(packet(2,'維持',{DeviceID:'T2'}));
 const next={...dev,CurrentProgram:{...dev.CurrentProgram,Eid:11}};
 feed.syncDevices([next,other]);feed.select(next);assert.deepEqual(feed.items,[]);
 feed.receive(packet(3,'遅延した旧字幕'));assert.deepEqual(feed.items,[]);
 feed.receive(packet(4,'新番組',{Eid:11}));assert.equal(feed.items[0].Text,'新番組');
 feed.select(other);assert.equal(feed.items[0].Text,'維持');
 feed.syncDevices([{...next,State:0},other]);assert.equal(feed.buffers.has(dev.ID),false);
 feed.syncDevices([next,other]);feed.select(next);assert.deepEqual(feed.items,[]);
 feed.syncDevices([next]);assert.equal(feed.buffers.has(other.ID),false);
 assert.equal(programKey({...dev,State:0}),'');
});

test('each device retains fifty statements, independent colors, cursors and clear boundaries',()=>{
 const events=[],feed=new CaptionFeed(null,value=>events.push(value)),other={...dev,ID:'T2'};
 feed.syncDevices([dev,other]);feed.select(dev);
 for(let n=1;n<=52;n++) {
  feed.receive(packet(n,'字幕'+n,{Pts:n,Color:3}));
  feed.receive(packet(n,'別字幕'+n,{DeviceID:'T2',Pts:n,Color:6}));
 }
 assert.equal(feed.items.length,50);assert.equal(feed.items[0].Text,'字幕3');
 const previous=events.at(-1);
 feed.receive(packet(53,'続き',{Pts:52,Color:6}));
 assert.equal(previous.items.at(-1).Text,'字幕52');
 assert.equal(previous.items.at(-1).Segments.length,1);
 assert.deepEqual(feed.items.at(-1).Segments.map(s=>s.Color),[3,6]);
 feed.receive(packet(53,'重複',{Pts:52}));assert.equal(feed.items.at(-1).Text,'字幕52続き');
 feed.receive(packet(54,'ルビ',{Size:'small',Clear:true}));
 feed.receive(packet(55,'次',{Pts:52}));assert.equal(feed.items.at(-1).Text,'次');
 feed.receive(packet(56,'よみ',{Size:'micro'}));assert.equal(feed.items.length,50);
 feed.select(other);assert.equal(feed.items.length,50);assert.equal(feed.items.at(-1).Text,'別字幕52');
 feed.receive(packet(53,'続き',{DeviceID:'T2',Pts:52}));assert.equal(feed.items.at(-1).Text,'別字幕52続き');
});
