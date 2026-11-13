const {test}=require('node:test');
const assert=require('node:assert/strict');
const files=require('../src/playback-files.js');
const {Client}=require('../src/client.js');
test('unchanged polling reuses the list and changed metadata replaces only its row',()=>{
 const rows=[{id:1,slot:0,title:'番組1'},{id:2,slot:0,title:'番組2'}];
 assert.equal(files.reconcile(rows,JSON.parse(JSON.stringify(rows))),rows);
 const changed=files.reconcile(rows,[{...rows[0]},{...rows[1],recording:true}]);
 assert.equal(changed[0],rows[0]);assert.notEqual(changed[1],rows[1]);
 assert.equal(changed[1].recording,true);
});
test('recording list sorts dates and Japanese titles in all four orders',()=>{
 const rows=[{id:1,slot:0,title:'い',starttime:10},{id:2,slot:0,title:'あ',starttime:30}];
 assert.deepEqual(files.sorted(rows,'date-desc').map(r=>r.id),[2,1]);
 assert.deepEqual(files.sorted(rows,'date-asc').map(r=>r.id),[1,2]);
 assert.deepEqual(files.sorted(rows,'title-asc').map(r=>r.id),[2,1]);
 assert.deepEqual(files.sorted(rows,'title-desc').map(r=>r.id),[1,2]);
 assert.deepEqual(rows.map(r=>r.id),[1,2]);
 assert.notEqual(files.key({id:1,slot:0}),files.key({id:1,slot:1}));
 assert.match(files.details({title:'番組',recording:true,filename:'recording.ts'}),/録画中[\s\S]*recording.ts/);
});
test('file selection targets only its device session and creates a local idle device',()=>{
 const sent=[];const c=new Client(()=>{});c.connected=true;c.nativePlayback=true;c.socket={readyState:1,send:text=>sent.push(JSON.parse(text.slice(4)))};
 c.devices.set('PBK01',{Kind:'playback',Playback:{session:'one'}});c.devices.set('PBK02',{Kind:'playback',Playback:{session:'two'}});
 c.playback('','create');c.playback('PBK02','load',{id:17,slot:1});c.playback('PBK02','close',0);
 assert.deepEqual(sent,[{action:'create'},{action:'load',session:'two',scheduleId:17,slot:1},{action:'close',session:'two',seconds:0}]);
});
test('recording list responses from a previous connection are discarded',async()=>{
 let resolve;const c=new Client(()=>{},()=>new Promise(r=>resolve=r));c.connected=true;c.base='http://localhost';
 const pending=c.playbackRecordings();c.generation++;resolve({ok:true,json:async()=>[]});
 await assert.rejects(pending,/接続先/);
});
