const {test}=require('node:test');
const assert=require('node:assert/strict');
const {markers,adjacent,status}=require('../src/playback-boundaries.js');
const {Client}=require('../src/client.js');

test('CM candidates are ordered, bounded, deduplicated, and kept distinct from confirmed CM',()=>{
  const playback={durationMs:100000,cmStatus:'splice-candidates',cmBoundaries:[{positionMs:60000,kind:'splice-in'},{positionMs:30000,kind:'splice-out'},{positionMs:30000},{positionMs:-1},{positionMs:100001},{positionMs:NaN}]};
  const rows=markers(playback);
  assert.deepEqual(rows.map(row=>row.positionMs),[30000,60000]);
  assert.match(status(playback,rows),/2件.*未確定/);
  assert.equal(adjacent(rows,20000,-1),undefined);
  assert.equal(adjacent(rows,20000,1).positionMs,30000);
  assert.equal(adjacent(rows,28900,1).positionMs,60000,'rounded PCR seek does not repeat the current candidate');
  assert.equal(adjacent(rows,60500,-1).positionMs,30000);
  assert.equal(adjacent(rows,70000,1),undefined);
  assert.equal(playback.cmBoundaries[0].positionMs,60000,'input remains unchanged');
});

test('CM status distinguishes old servers, no signal, unsupported boundaries and analysis',()=>{
  assert.match(status({phase:'indexing'},[]),/解析中/);
  assert.match(status({cmStatus:'not-signaled'},[]),/対応信号なし/);
  assert.match(status({cmStatus:'no-supported-splice-boundary'},[]),/対応する境界なし/);
  assert.match(status({phase:'playing'},[]),/情報なし/);
  assert.deepEqual(markers({}),[]);
});

test('manual bookmarks share navigation with automatic boundaries and retain their source',()=>{
  const playback={durationMs:90000,cmBoundaries:[{positionMs:10000,kind:'splice-out'},{positionMs:35000,kind:'splice-in'}],manualBookmarks:[{positionMs:20000,kind:'manual'},{positionMs:35000,kind:'manual'}]};
  const rows=markers(playback);
  assert.deepEqual(rows.map(row=>[row.positionMs,row.kind]),[[10000,'splice-out'],[20000,'manual'],[35000,'manual']]);
  assert.equal(adjacent(rows,10000,1).positionMs,20000);
  assert.match(status(playback,rows),/しおり 2件.*境界候補 1件/);
});

test('bookmark edits carry millisecond position and generation to the selected session',()=>{
  const sent=[],client=new Client(()=>{});client.connected=true;client.socket={readyState:1,send:text=>sent.push(JSON.parse(text.slice(4)))};
  client.devices.set('PBK01',{Kind:'playback',Playback:{session:'one'}});
  for(const action of ['bookmark-add','bookmark-delete'])client.playback('PBK01',action,{positionMs:12345,generation:7});
  assert.deepEqual(sent,[{action:'bookmark-add',session:'one',positionMs:12345,generation:7},{action:'bookmark-delete',session:'one',positionMs:12345,generation:7}]);
});

test('p/f program boundaries join bookmark navigation without becoming CM candidates',()=>{
 const playback={durationMs:90000,cmBoundaries:[{positionMs:10000,kind:'splice-out'}],programBoundaries:[{positionMs:20000,kind:'program-change',previousEventId:10,eventId:11}],manualBookmarks:[{positionMs:30000,kind:'manual'}]};
 const rows=markers(playback);
 assert.equal(adjacent(rows,10000,1).kind,'program-change');
 assert.match(status(playback,rows),/しおり 1件.*番組切替 1件.*境界候補 1件/);
});
