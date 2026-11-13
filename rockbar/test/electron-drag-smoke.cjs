const {_electron}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const assert=require('node:assert/strict');
const {execFile}=require('node:child_process');
const run=require('node:util').promisify(execFile);
(async()=>{
 if(process.platform!=='win32'){console.log('SKIP: Windows native hit testing');return;}
 const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
 const app=await _electron.launch({executablePath:require('electron'),args:[path.join(__dirname,'fixtures/resize-main.cjs')],env});
 try {
  const page=await app.firstWindow();await page.waitForFunction(()=>typeof testUpdate==='function'&&document.querySelector('#current-program .program-scroll-content'));
  const handle=await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].getNativeWindowHandle().readBigUInt64LE().toString(16));
  async function check(label) {
   const data=await page.evaluate(()=>{
    const brand=document.querySelector('header .brand').getBoundingClientRect(),settings=document.querySelector('#settings').getBoundingClientRect(),panel=document.querySelector('main').getBoundingClientRect();
    return {width:innerWidth,height:innerHeight,points:[{x:brand.left+25,y:brand.top+brand.height/2},{x:brand.left+brand.width/2,y:brand.top+brand.height/2},{x:brand.right-12,y:brand.top+brand.height/2},{x:settings.left+settings.width/2,y:settings.top+settings.height/2},{x:panel.left+30,y:panel.top+30}]};
   });
   const {stdout}=await run('powershell.exe',['-NoProfile','-File',path.join(__dirname,'fixtures/native-hit-test.ps1'),'-WindowHandle',handle,'-ViewportWidth',String(data.width),'-ViewportHeight',String(data.height),'-Points',JSON.stringify(data.points)],{windowsHide:true});
   // HTCAPTION=2 moves the window; HTCLIENT=1 keeps controls interactive.
   assert.deepEqual(JSON.parse(stdout.trim()),[2,2,2,1,1],label);
   console.log('PASS native drag:',label);
  }
  await check('startup');
  await page.evaluate(()=>{
   testUpdate({type:'status',value:{connected:true}});testUpdate({type:'server-time',value:2000000});
   window.dragDevices=[{ID:'REC01',State:2,CurrentService:1},{ID:'REC02',State:2,CurrentService:150},{ID:'PBK01',State:2,Kind:'playback',Playback:{session:'drag',phase:'paused',scheduleId:1500,generation:1,positionMs:1000,durationMs:600000}}];
   testUpdate({type:'stations',value:Array.from({length:200},(_,i)=>({Sid:i+1,Name:`局 ${i+1}`,Tsp:1,View:1}))});
   testUpdate({type:'devices',value:dragDevices});
  });
  await page.waitForFunction(()=>document.querySelectorAll('[data-file]').length===2000);
  for(const zoom of [1,1.25,1.5]) {
   await app.evaluate(({BrowserWindow},zoom)=>BrowserWindow.getAllWindows()[0].webContents.setZoomFactor(zoom),zoom);
   await page.locator('[data-device="PBK01"]').click();
   await page.evaluate(()=>document.querySelector('#playback-file-list').scrollTo({top:100000,behavior:'instant'}));
   await check(`Playback list scrolled, zoom ${zoom}`);
   await page.locator('[data-device="REC01"]').click();await check(`REC01, zoom ${zoom}`);
   await page.locator('[data-device="REC02"]').click();await check(`REC02 with station list scrolled, zoom ${zoom}`);
   await page.locator('[data-device="PBK01"]').click();await check(`return to Playback, zoom ${zoom}`);
  }
 } finally {await app.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
