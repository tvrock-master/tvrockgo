const {chromium}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const {pathToFileURL}=require('node:url');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({channel:'msedge',headless:true});
 try {
  const page=await browser.newPage({viewport:{width:1000,height:600}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.clock.install();
  await page.route('https://fonts.googleapis.com/**',route=>route.abort());
  await page.addInitScript(()=>{
   window.operations=[];window.playbackRequests=0;
   window.records=[{id:1,slot:0,title:'あの番組',starttime:100,endtime:200,deviceid:'REC01',filename:'recording-one.ts',complete:true},{id:2,slot:1,title:'いの番組',starttime:300,endtime:400,deviceid:'REC02',filename:'recording-two.ts',complete:true},{id:3,slot:0,title:'録画中の番組',starttime:500,endtime:900,filename:'recording-live.ts',recording:true}];
   window.rockbar={onUpdate:cb=>window.updateRockbar=cb,settings:async()=>({server:'http://localhost'}),connect:async()=>({}),watchDevice:async()=>{},setMode:async()=>{},playback:async(...args)=>{operations.push(args);return {ok:true};},playbackRecordings:async()=>{playbackRequests++;return JSON.parse(JSON.stringify(records));}};
  });
  await page.goto(pathToFileURL(path.resolve(__dirname,'../src/index.html')).href);
  await page.evaluate(()=>{updateRockbar({type:'status',value:{connected:true}});updateRockbar({type:'devices',value:[{ID:'REC01',State:0}]});});
  await page.locator('#playback-device').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['','create',0]);
  await page.evaluate(()=>{window.pbk={ID:'PBK01',Kind:'playback',State:2,Playback:{session:'one',phase:'idle',generation:1,positionMs:0,durationMs:0}};updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});});
  assert.equal(await page.locator('[data-device="PBK01"]').getAttribute('aria-pressed'),'true');
  await page.waitForFunction(()=>document.querySelectorAll('[data-file]').length===3);
  assert.equal(await page.locator('#playback-files-section').evaluate(node=>node.closest('#current-section')!==null),true);
  await page.locator('#current-section > summary').click();
  assert.equal(await page.locator('#playback-file-list').isVisible(),false);
  assert.equal(await page.locator('#playback-panel').isVisible(),true);
  assert.equal(await page.locator('#playback-panel').evaluate(node=>getComputedStyle(node).borderTopWidth),'0px');
  await page.locator('#current-section > summary').click();
  assert.equal(await page.locator('#playback-file-list').isVisible(),true);
  assert.equal(await page.locator('#playback-panel').evaluate(node=>getComputedStyle(node).borderTopWidth),'1px');
  await page.locator('#playback-files-section > summary').click();
  assert.equal(await page.locator('#playback-file-list').isVisible(),false);
  assert.equal(await page.locator('#playback-panel').isVisible(),true);
  await page.locator('#playback-files-section > summary').click();
  assert.equal(await page.evaluate(()=>playbackRequests),1);
  await page.clock.runFor(60000);
  assert.equal(await page.evaluate(()=>playbackRequests),1,'elapsed time never polls the recording list');
  await page.evaluate(()=>updateRockbar({type:'schedule-update'}));
  await page.waitForFunction(()=>playbackRequests===2 && !playbackFilesLoading);
  await page.locator('[data-device="REC01"]').click();
  await page.evaluate(()=>updateRockbar({type:'schedule-update'}));
  assert.equal(await page.evaluate(()=>playbackRequests),2,'updates are deferred outside Playback mode');
  await page.locator('[data-device="PBK01"]').click();
  await page.waitForFunction(()=>playbackRequests===3 && !playbackFilesLoading);
  await page.evaluate(()=>{
    rockbar.playbackRecordings=()=>{playbackRequests++;return playbackRequests===4?new Promise(resolve=>window.resolveRecordingList=resolve):Promise.resolve(JSON.parse(JSON.stringify(records)));};
    updateRockbar({type:'schedule-update'});updateRockbar({type:'schedule-update'});updateRockbar({type:'schedule-update'});
    resolveRecordingList(JSON.parse(JSON.stringify(records)));
  });
  await page.waitForFunction(()=>playbackRequests===5 && !playbackFilesLoading);
  const order=()=>page.locator('[data-file]').evaluateAll(nodes=>nodes.map(n=>n.dataset.file));
  assert.deepEqual(await order(),['0:3','1:2','0:1']);
  const idleChanges=await page.evaluate(()=>{
    const list=document.querySelector('#playback-file-list');const observer=new MutationObserver(()=>{});
    observer.observe(list,{subtree:true,childList:true,attributes:true});
    for(let index=0;index<5;index++)render();
    const count=observer.takeRecords().length;observer.disconnect();return count;
  });
  assert.equal(idleChanges,0,'clock/device refreshes do not rebuild the unchanged recording list');
  const refreshed=await page.evaluate(async()=>{
    const list=document.querySelector('#playback-file-list');const first=list.firstElementChild;
    let changes=0;const observer=new MutationObserver(records=>{changes+=records.length;});
    observer.observe(list,{subtree:true,childList:true,attributes:true});
    records=JSON.parse(JSON.stringify(records));await refreshPlaybackFiles();await Promise.resolve();
    changes+=observer.takeRecords().length;observer.disconnect();return {changes,same:first===list.firstElementChild};
  });
  assert.deepEqual(refreshed,{changes:0,same:true},'identical server polls preserve rows without DOM writes');
  const sort=page.locator('#playback-file-sort');assert.equal(await sort.count(),1);
  assert.equal(await sort.textContent(),'日付 ↓');
  await sort.click();assert.equal(await sort.textContent(),'日付 ↑');assert.deepEqual(await order(),['0:1','1:2','0:3']);
  await sort.click();assert.equal(await sort.textContent(),'タイトル ↑');assert.deepEqual(await order(),['0:1','1:2','0:3']);
  await sort.click();assert.equal(await sort.textContent(),'タイトル ↓');assert.deepEqual(await order(),['0:3','1:2','0:1']);
  await sort.click();assert.equal(await sort.textContent(),'日付 ↓');assert.deepEqual(await order(),['0:3','1:2','0:1']);
  const before=await page.evaluate(()=>operations.length);await page.locator('[data-file="0:1"]').click();assert.equal(await page.evaluate(()=>operations.length),before);
  assert.match(await page.locator('[data-file="0:1"]').getAttribute('title'),/あの番組[\s\S]*REC01[\s\S]*recording-one.ts/);
  await page.locator('[data-file="0:3"]').click();
  assert.equal(await page.locator('#playback-pause').isEnabled(),true);
  await page.locator('[data-file="0:3"]').dblclick();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','load',{id:3,slot:0}]);
  await page.locator('[data-file="1:2"]').dblclick();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','load',{id:2,slot:1}]);
  await page.evaluate(()=>{pbk.Playback={...pbk.Playback,phase:'playing',generation:2,scheduleId:2,slot:1,title:'いの番組',durationMs:600000,positionMs:30000};updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});});
  await page.locator('#playback-stop').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','stop',0]);
  await page.evaluate(()=>{pbk.Playback={...pbk.Playback,phase:'idle',generation:3,durationMs:0,positionMs:0};updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});});
  assert.equal(await page.locator('[data-device="PBK01"]').count(),1);
  assert.equal(await page.locator('#power').isEnabled(),true);
  await page.evaluate(()=>{
    records=Array.from({length:80},(_,index)=>({id:100+index,slot:0,title:`録画 ${index}`,starttime:1000+index,complete:true,filename:`${index}.ts`}));
    playbackFileSelection.set('one','0:100');
  });
  await page.evaluate(()=>refreshPlaybackFiles());
  const scrollCalls=await page.evaluate(()=>{
    const list=document.querySelector('#playback-file-list');window.fileScrollCalls=[];
    const scroll=list.scrollTo.bind(list);list.scrollTo=options=>{fileScrollCalls.push(options);scroll({...options,behavior:'instant'});};
    list.scrollTop=0;document.querySelector('#service').click();
    return {calls:fileScrollCalls,top:list.scrollTop};
  });
  assert.equal(scrollCalls.calls.length,1);assert.equal(scrollCalls.calls[0].behavior,'smooth');assert.ok(scrollCalls.top>0,'heading scrolls to the selected file');
  await page.locator('[data-device="REC01"]').click();
  await page.evaluate(()=>{document.querySelector('#playback-file-list').scrollTop=0;fileScrollCalls=[];});
  await page.locator('[data-device="PBK01"]').click();
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),1,'returning to Playback scrolls once');
  await page.evaluate(()=>{fileScrollCalls=[];render();render();});
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),0,'ordinary updates preserve manual scrolling');
  await page.evaluate(()=>{
    pbk.Playback={...pbk.Playback,phase:'playing',scheduleId:120,slot:0,durationMs:600000,positionMs:30000};
    updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});
    playbackFileSelection.set('one','0:100');render();fileScrollCalls=[];
    document.querySelector('#service').click();
  });
  assert.equal(await page.locator('[data-file="0:120"]').getAttribute('aria-selected'),'true','heading selects the playing file over another selection');
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),1);assert.equal(await page.evaluate(()=>fileScrollCalls[0].behavior),'smooth');
  for(const phase of ['playing','paused']) {
    await page.evaluate(phase=>{
      pbk.Playback.phase=phase;playbackFileSelection.set('one','0:100');
      updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});
    },phase);
    await page.locator('[data-device="REC01"]').click();
    await page.evaluate(()=>{document.querySelector('#playback-file-list').scrollTop=0;fileScrollCalls=[];});
    await page.locator('[data-device="PBK01"]').click();
    assert.equal(await page.locator('[data-file="0:120"]').getAttribute('aria-selected'),'true',`${phase}: device selection follows the current playback file`);
    assert.equal(await page.evaluate(()=>fileScrollCalls.length),1);
    assert.equal(await page.evaluate(()=>fileScrollCalls[0].behavior),'smooth');
  }
  await page.locator('[data-file="0:100"]').click();
  await page.evaluate(()=>{fileScrollCalls=[];});
  await page.locator('[data-device="PBK01"]').click();
  assert.equal(await page.locator('[data-file="0:120"]').getAttribute('aria-selected'),'true','clicking the already selected Playback device also follows its current file');
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),1);
  await page.evaluate(()=>{
    pbk.Playback.phase='paused';playbackFileSelection.set('one','0:100');
    updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});fileScrollCalls=[];document.querySelector('#service').click();
  });
  assert.equal(await page.locator('[data-file="0:120"]').getAttribute('aria-selected'),'true','paused file is still the current file');
  await page.evaluate(()=>{
    pbk.Playback.phase='idle';playbackFileSelection.set('one','0:100');
    updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});fileScrollCalls=[];document.querySelector('#service').click();
  });
  assert.equal(await page.locator('[data-file="0:100"]').getAttribute('aria-selected'),'true','stopped playback uses the existing selection, not the last schedule ID');
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),1);
  await page.evaluate(()=>{
    playbackFileSelection.delete('one');renderPlaybackFiles();fileScrollCalls=[];document.querySelector('#service').click();
  });
  assert.equal(await page.locator('[data-file][aria-selected="true"]').count(),0,'stopped playback with no selection does not select the old file');
  assert.equal(await page.evaluate(()=>fileScrollCalls.length),0,'no playback and no selection means no scrolling');
  await page.evaluate(()=>{playbackFileSelection.set('one','0:100');renderPlaybackFiles();});
  await page.screenshot({path:path.resolve(__dirname,'../../.tmp/playback-files-rockbar.png')});
  const beforeTimekeeper=await page.evaluate(()=>operations.length);
  await page.locator('#timekeeper-toggle').click();
  assert.equal(await page.locator('[data-device="PBK01"]').count(),0,'TimeKeeper excludes Playback devices');
  assert.equal(await page.locator('[data-device="REC01"]').count(),1);
  assert.match(await page.locator('#ticker').textContent(),/0 台録画中 \/ 1 台/);
  await page.clock.runFor(2100);
  assert.equal(await page.locator('[data-device="PBK01"]').count(),0);
  await page.locator('#timekeeper-toggle').click();
  assert.equal(await page.locator('[data-device="PBK01"]').getAttribute('aria-pressed'),'true','normal view restores the selected Playback device');
  assert.equal(await page.evaluate(()=>operations.length),beforeTimekeeper,'hiding the device does not stop playback');
  await page.locator('#power').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','close',0]);
  await page.evaluate(()=>updateRockbar({type:'devices',value:[{ID:'REC01',State:0}]}));assert.equal(await page.locator('[data-device="PBK01"]').count(),0);
  assert.deepEqual(errors,[]);console.log('PASS: create, auto-select, four sorts, details, local load, stop-to-idle and power-close');
 } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
