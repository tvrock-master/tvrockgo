const { test } = require('node:test');
const assert = require('node:assert/strict');
const { wave, switchWaves, services, current, progress } = require('../src/programs.js');
const { tuneCommands } = require('../electron/protocol.cjs');
const stations = [
 {Sid:65537,Tsp:1,View:1,No:2,Name:'地上A'},
 {Sid:65538,Tsp:1,View:1,No:1,Name:'地上B',Parent:65537},
 {Sid:65539,Tsp:1,View:0,No:3,Name:'非表示'},
 {Sid:131073,Tsp:2,View:1,No:1,Name:'BS'},
 {Sid:196609,Tsp:3,View:1,No:1,Name:'CS'},
 {Sid:262145,Tsp:4,View:1,No:1,Name:'ワンセグ',Parent:65537},
];
const device = {ID:'T1',RegistryCode:'R1',State:2,CurrentService:65537};

test('program progress uses elapsed broadcast time and clamps to start/end', () => {
 const program = { Start: 1000, End: 4600 };
 assert.deepEqual(progress(program, 2200), { percent: 33, ratio: 1 / 3, segments: 6, text: '20 / 60min(33%)' });
 assert.deepEqual(progress(program, 900), { percent: 0, ratio: 0, segments: 0, text: '0 / 60min(0%)' });
 assert.deepEqual(progress(program, 4700), { percent: 100, ratio: 1, segments: 20, text: '60 / 60min(100%)' });
 assert.equal(progress(program, 2170).ratio * 20, 6.5);
 assert.ok(progress(program, 2179).ratio > progress(program, 2170).ratio);
 assert.equal(progress({ Start: 1000, End: 1090 }, 1045).text, '0 / 1.5min(50%)');
 for (const value of [null, {}, { Start: 1000, End: 1000 }, { Start: 1000, End: 999 }]) {
  assert.equal(progress(value, 2200), null);
 }
 assert.equal(progress(program, NaN), null);
});
test('four broadcast bands are isolated and hidden services excluded',()=>{
 for(const tsp of [1,2,3,4]) {
  const d={...device,CurrentService:(tsp<<16)|1};
  assert.equal(wave(d,stations),tsp);
  assert.ok(services(d,stations).every(s=>s.Tsp===tsp&&s.View>0));
 }
 assert.deepEqual(services(device,stations).map(s=>s.Sid),[65537]);
 assert.equal(wave({...device,CurrentService:0},stations),0);
});

test('hybrid devices can browse and tune terrestrial and one-seg in both directions',()=>{
 for (const TunerType of [0,2]) {
  for (const CurrentService of [65537,262145]) {
   const hybrid={...device,TunerType,Hybrid:true,CurrentService};
   assert.deepEqual(switchWaves(hybrid,stations),[1,4]);
   assert.deepEqual(services(hybrid,stations,1).map(s=>s.Sid),[65537]);
   assert.deepEqual(services(hybrid,stations,1,true).map(s=>s.Sid),[65537,65538]);
   assert.deepEqual(services(hybrid,stations,4).map(s=>s.Sid),[262145]);
   const messages=sid=>tuneCommands(hybrid,sid,stations).map(s=>JSON.parse(s.slice(4)).message);
   assert.deepEqual(messages(65537),['65537']);
   assert.deepEqual(messages(262145),['524289','65537']);
   for (const sid of [131073,196609,65539]) assert.throws(()=>tuneCommands(hybrid,sid,stations));
   assert.throws(()=>tuneCommands({...hybrid,State:3},262145,stations));
   assert.equal(hybrid.CurrentService,CurrentService);
   const standard={...hybrid,Hybrid:false};
   assert.deepEqual(switchWaves(standard,stations),[]);
   assert.throws(()=>tuneCommands(standard,CurrentService===65537?262145:65537,stations));
  }
 }
 assert.deepEqual(switchWaves({...device,Hybrid:true,TunerType:1},stations),[2,3]);
 assert.deepEqual(switchWaves(undefined,stations),[]);
});
test('multi includes child services only for terrestrial and BS; CS and one-seg stay unchanged',()=>{
 for (const tsp of [1,2,3,4]) {
  const sid = tsp << 16;
  const list = [
   {Sid:sid|1,Tsp:tsp,View:1,Parent:0},
   {Sid:sid|2,Tsp:tsp,View:1,Parent:sid|1},
   {Sid:sid|3,Tsp:tsp,View:1},
   {Sid:sid|4,Tsp:tsp,View:0,Parent:sid|1},
  ];
  const d = {...device,CurrentService:sid|1};
  assert.deepEqual(services(d,list).map(s=>s.Sid),
   [1,2].includes(tsp) ? [sid|1,sid|3] : [sid|1,sid|2,sid|3]);
  assert.deepEqual(services(d,list,tsp,true).map(s=>s.Sid),[sid|1,sid|2,sid|3]);
  assert.deepEqual(tuneCommands(d,sid|2,list).map(s=>JSON.parse(s.slice(4)).message),
   [String((8<<16)|2),String(sid|1)]);
 }
});

test('service order matches DeviceController regardless of addition order or No',()=>{
 for (const tsp of [1,2,3,4]) {
  const sid = tsp << 16;
  const list = [
   {Sid:sid|3,Tsp:tsp,View:1,No:1},
   {Sid:sid|1,Tsp:tsp,View:1,No:3},
   {Sid:sid|2,Tsp:tsp,View:1,No:2},
  ];
  const d = {...device,CurrentService:sid|1};
  assert.deepEqual(services(d,list).map(s=>s.Sid),[sid|1,sid|2,sid|3]);
  assert.deepEqual(list.map(s=>s.Sid),[sid|3,sid|1,sid|2]);
 }
});

test('program switches at exact start/end boundary and expired programs disappear',()=>{
 const a={Start:10,End:20,Title:'A'},b={Start:20,End:30,Title:'B'};
 assert.equal(current([a,b],19),a);assert.equal(current([a,b],20),b);
 assert.equal(current([a,b],30),null);assert.equal(current(undefined,20),null);
});
test('tuning uses string messages and sends subchannel before parent',()=>{
 const decode=sid=>tuneCommands(device,sid,stations).map(s=>{assert.ok(s.startsWith('REQ:'));return JSON.parse(s.slice(4));});
 assert.deepEqual(decode(65537),[{id:'R1',command:'StreamChange',message:'65537'}]);
 assert.deepEqual(decode(65538).map(s=>s.message),['524290','65537']);
 assert.deepEqual(tuneCommands({...device,CurrentService:262145},262145,stations).map(s=>JSON.parse(s.slice(4)).message),['524289','65537']);
 for(const State of [0,1,3,4]) assert.throws(()=>tuneCommands({...device,State},65537,stations));
 for(const sid of [131073,196609,262145,65539,'65537',999]) assert.throws(()=>tuneCommands(device,sid,stations));
});
