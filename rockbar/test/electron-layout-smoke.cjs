const {_electron}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const assert=require('node:assert/strict');
(async()=>{
 const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
 const app=await _electron.launch({executablePath:require('electron'),args:[path.join(__dirname,'fixtures/resize-main.cjs')],env});
 try {
  const page=await app.firstWindow();await page.locator('#window-layout').waitFor({state:'visible'});
  await page.evaluate(()=>{
   document.querySelectorAll('.foldable').forEach(section=>section.open=true);document.querySelector('#captions-section').open=false;
   testUpdate({type:'status',value:{connected:true}});
   testUpdate({type:'devices',value:[{ID:'PBK01',Kind:'playback',State:2,Playback:{session:'layout',phase:'paused',durationMs:600000,positionMs:1000}}]});
  });
  const area=await app.evaluate(({BrowserWindow,screen})=>{
   const win=BrowserWindow.getAllWindows()[0];win.webContents.setZoomFactor(1);
   const a=screen.getDisplayMatching(win.getBounds()).workArea;
   win.setBounds({x:a.x+a.width-900,y:a.y+a.height-600,width:900,height:600});return a;
  });
  async function check(width,height) {
   const actual=await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].getBounds());
   assert.deepEqual(actual,{x:area.x+area.width-width,y:area.y+area.height-height,width,height});
  }
  await page.locator('#window-layout').click();await check(680,240);
  assert.equal(await page.locator('#current-section').getAttribute('open'),null);
  assert.equal(await page.locator('#playback-panel').isVisible(),true);
  assert.equal(await page.locator('#playback-files').isVisible(),false);
  // Reload while compact must retain the saved normal folds.
  await page.reload();await page.locator('#window-layout[aria-pressed="true"]').waitFor();
  await app.evaluate(({BrowserWindow},area)=>BrowserWindow.getAllWindows()[0].setBounds({x:area.x+area.width-720,y:area.y+area.height-280,width:720,height:280}),area);
  await page.locator('#window-layout').click();await check(900,600);
  assert.equal(await page.locator('#current-section').getAttribute('open'),'');
  assert.equal(await page.locator('#captions-section').getAttribute('open'),null);
  assert.equal(await page.locator('#playback-files-section').getAttribute('open'),'');
  await page.locator('#window-layout').click();await check(720,280);
  console.log('PASS: compact toggle, right/bottom anchoring, remembered sizes, folds and reload');
 }finally{await app.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
