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
      window.operations=[];
      window.rockbar={onUpdate:cb=>window.updateRockbar=cb,settings:async()=>({server:'http://localhost'}),connect:async()=>({}),watchDevice:async()=>{},setMode:async()=>{},playback:async(...args)=>{
        operations.push(args);
        if(args[1]==='bookmark-add'){pbk.Playback.manualBookmarks=[...(pbk.Playback.manualBookmarks||[]),{positionMs:args[2].positionMs,kind:'manual'}];sendPlayback();}
        if(args[1]==='bookmark-delete'){pbk.Playback.manualBookmarks=pbk.Playback.manualBookmarks.filter(row=>row.positionMs!==args[2].positionMs);sendPlayback();}
        return {ok:true};
      }};
    });
    await page.goto(pathToFileURL(path.resolve(__dirname,'../src/index.html')).href);
    await page.evaluate(()=>{
      window.pbk={ID:'PBK01',Kind:'playback',State:2,Playback:{session:'cm-test',title:'CM候補表示の動作確認（テストデータ）',phase:'paused',generation:1,positionMs:20000,durationMs:90000,cmStatus:'splice-candidates',cmBoundaries:[{positionMs:10000,kind:'splice-out'},{positionMs:35000,kind:'splice-in'},{positionMs:65000,kind:'splice-out'}]}};
      window.sendPlayback=()=>updateRockbar({type:'devices',value:[{ID:'REC01',State:0},pbk]});
      updateRockbar({type:'status',value:{connected:true}});sendPlayback();
    });
    assert.equal(await page.locator('.playback-cm-marker').count(),3);
    assert.match(await page.locator('#playback-cm-status').textContent(),/3件.*未確定/);
    assert.match(await page.locator('.playback-cm-marker').first().getAttribute('title'),/開始候補 00:00:10/);
    const placement=await page.evaluate(()=>{
      const last=document.querySelector('[data-playback="seek-end"]').getBoundingClientRect(),left=document.querySelector('#playback-cm-previous').getBoundingClientRect(),add=document.querySelector('#playback-bookmark-add').getBoundingClientRect(),right=document.querySelector('#playback-cm-next').getBoundingClientRect();
      return {gap:left.left-last.right,sameLine:left.y===last.y&&right.y===last.y&&add.y===last.y,inOrder:add.left>left.right&&right.left>add.right};
    });
    assert.ok(placement.gap>=18&&placement.sameLine&&placement.inOrder,'CM buttons follow regular controls with a gap');
    await page.locator('#playback-cm-previous').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','seek-to',10]);
    assert.equal(await page.locator('#playback-cm-previous').isDisabled(),true);
    await page.locator('#playback-cm-next').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','seek-to',35]);
    await page.locator('#playback-cm-next').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','seek-to',65]);
    assert.equal(await page.locator('#playback-cm-next').isDisabled(),true);
    await page.locator('.playback-cm-marker').nth(1).click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','seek-to',35]);
    const same=await page.evaluate(()=>{
      const marker=document.querySelector('.playback-cm-marker');pbk.Playback.positionMs=35000;pbk.Playback.generation++;pbk.Playback.cmBoundaries=JSON.parse(JSON.stringify(pbk.Playback.cmBoundaries));sendPlayback();return marker===document.querySelector('.playback-cm-marker');
    });
    assert.equal(same,true,'position updates preserve marker DOM');
    await page.evaluate(()=>{pbk.Playback.positionMs=50000;pbk.Playback.generation++;sendPlayback();});
    await page.locator('#playback-bookmark-add').click();
    assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','bookmark-add',{positionMs:50000,generation:3}]);
    assert.equal(await page.locator('.playback-cm-marker.manual').count(),1);
    assert.match(await page.locator('#playback-cm-status').textContent(),/しおり 1件/);
    assert.match(await page.locator('.playback-cm-marker.manual').getAttribute('title'),/しおり 00:00:50.*削除/);
    await page.locator('.playback-cm-marker.manual').click({button:'right'});
    assert.equal(await page.locator('#playback-bookmark-menu').isVisible(),true);
    await page.locator('#playback-bookmark-delete').click();
    assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','bookmark-delete',{positionMs:50000,generation:3}]);
    assert.equal(await page.locator('.playback-cm-marker.manual').count(),0);
    assert.equal(await page.locator('#playback-bookmark-menu').isVisible(),false);
    await page.locator('#playback-bookmark-add').click();
    await page.locator('.playback-cm-marker.manual').focus();await page.keyboard.press('Delete');
    assert.equal(await page.locator('.playback-cm-marker.manual').count(),0,'Delete key removes only the focused manual bookmark');
    await page.locator('#playback-bookmark-add').click();
    await page.evaluate(()=>{pbk.Playback.programBoundaries=[{positionMs:25000,kind:'program-change',previousEventId:21,eventId:22}];sendPlayback();});
    assert.equal(await page.locator('.playback-cm-marker.program-boundary').count(),1);
    assert.match(await page.locator('.program-boundary').getAttribute('title'),/番組切替 00:00:25.*21 → 22/);
    assert.match(await page.locator('#playback-cm-status').textContent(),/番組切替 1件/);
    await page.locator('.program-boundary').focus();const countBefore=await page.evaluate(()=>operations.length);await page.keyboard.press('Delete');
    assert.equal(await page.evaluate(()=>operations.length),countBefore,'automatic program boundaries are not deleted as manual bookmarks');
    await page.locator('.program-boundary').click();assert.deepEqual(await page.evaluate(()=>operations.at(-1)),['PBK01','seek-to',25]);
    await page.evaluate(()=>{pbk.Playback.positionMs=25000;pbk.Playback.generation++;sendPlayback();});
    await page.screenshot({path:path.resolve(__dirname,'../../.tmp/playback-cm-preview.png')});
    await page.evaluate(()=>{pbk.Playback.cmBoundaries=[];pbk.Playback.manualBookmarks=[];pbk.Playback.programBoundaries=[];pbk.Playback.cmStatus='not-signaled';sendPlayback();});
    assert.equal(await page.locator('.playback-cm-marker').count(),0);
    assert.match(await page.locator('#playback-cm-status').textContent(),/対応信号なし/);
    assert.equal(await page.locator('#playback-cm-previous').isDisabled(),true);assert.equal(await page.locator('#playback-cm-next').isDisabled(),true);
    await page.evaluate(()=>{pbk.Playback={...pbk.Playback,session:'another',cmStatus:undefined,cmBoundaries:undefined};sendPlayback();});
    assert.match(await page.locator('#playback-cm-status').textContent(),/情報なし/);
    assert.deepEqual(errors,[]);console.log('PASS: CM marker placement, labels, previous/next, marker click, reused DOM and absent metadata');
  } finally {await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
