const {chromium}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const {pathToFileURL}=require('node:url');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({channel:'msedge',headless:true});
 try {
  const page=await browser.newPage({viewport:{width:1000,height:900}});
  const errors=[];page.on('pageerror',error=>errors.push(error.message));
  await page.route('https://fonts.googleapis.com/**',route=>route.abort());
  await page.addInitScript(()=>{
   const setting={enabled:true,executable:'C:\\Viewer\\Viewer.exe',options:'/d BonDriver_UDP.dll /nd',transport:'udp',port:1237};
   window.saved=[];
   window.rockbar={settings:async()=>({server:'http://localhost',nativeViewer:true}),connect:async()=>({}),onUpdate:()=>{},watchDevice:async()=>{},
    viewerSettings:async()=>({devices:[{id:'REC01',kind:'normal',registry:'normal',setting:{...setting}},{id:'PBK01',kind:'playback',registry:'playback',setting:{...setting}}]}),
    saveViewerSettings:async(base,registry,value)=>{saved.push({registry,value});return {setting:value};},closeViewerSettings:async()=>{}};
  });
  await page.goto(pathToFileURL(path.resolve(__dirname,'../src/index.html')).href);
  await page.locator('#settings').click();await page.locator('#config-viewer-tab').click();
  const transport=page.locator('#viewer-transport');
  assert.equal(await transport.isEnabled(),true);assert.equal(await transport.locator('option').count(),2);
  assert.equal(await transport.inputValue(),'udp');
  await page.locator('#viewer-device').selectOption('playback');
  assert.equal(await transport.isDisabled(),true);assert.equal(await transport.locator('option').count(),1);
  assert.equal(await transport.inputValue(),'srt');assert.equal(await page.locator('#viewer-srt').isVisible(),true);
  assert.match(await page.locator('#viewer-options').inputValue(),/BonDriver_SRT\.dll/);
  assert.equal(await page.locator('#viewer-port').inputValue(),'3233');
  await page.locator('#viewer-save').click();
  await page.waitForFunction(()=>saved.length===1);
  assert.equal(await page.evaluate(()=>saved[0].value.transport),'srt');
  await page.locator('#viewer-device').selectOption('normal');
  assert.equal(await transport.isEnabled(),true);assert.equal(await transport.inputValue(),'udp');
  assert.equal(await transport.locator('option').count(),2);assert.equal(await page.locator('#viewer-playback-transport').isVisible(),false);
  await transport.selectOption('srt');await transport.selectOption('udp');
  assert.match(await page.locator('#viewer-options').inputValue(),/BonDriver_UDP\.dll/);
  assert.deepEqual(errors,[]);console.log('PASS: Playback is SRT-only; legacy UDP conversion requires save; normal devices retain UDP/SRT');
 }finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
