const {chromium}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const {pathToFileURL}=require('node:url');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({channel:'msedge',headless:true});
 try {
  const page=await browser.newPage({viewport:{width:1000,height:700}});const errors=[];page.on('pageerror',e=>errors.push(e.message));
  await page.route('https://fonts.googleapis.com/**',route=>route.abort());
  await page.addInitScript(()=>{
   window.rockbar={onUpdate:()=>{},settings:async()=>({server:'http://localhost',nativeViewer:true,snapToEdges:localStorage.getItem('test-snap')!=='false'}),connect:async()=>({}),watchDevice:async()=>{},
    viewerSettings:async()=>({devices:[]}),windowOptions:async value=>{if(window.failOptionSave)return {error:'保存テスト失敗'};localStorage.setItem('test-snap',String(value.snapToEdges));return value;}};
  });
  const url=pathToFileURL(path.resolve(__dirname,'../src/index.html')).href;
  await page.goto(url);await page.locator('#settings').click();await page.locator('#config-options-tab').click();
  assert.equal(await page.locator('[role="tab"]:visible').count(),3);
  assert.equal(await page.locator('#config-theme-tab').count(),0);
  assert.equal(await page.locator('#config-options-tab').getAttribute('aria-selected'),'true');
  assert.equal(await page.locator('#color-theme').isVisible(),true);
  const snap=page.locator('#snap-to-edges');assert.equal(await snap.isChecked(),true);
  await snap.uncheck();await page.waitForFunction(()=>localStorage.getItem('test-snap')==='false');
  for(const [mode,name]of [['1','light'],['2','dark']]){
    await page.locator('#color-theme').selectOption(mode);
    await page.locator('#config-options-tab').blur();
    await page.locator('#config').screenshot({path:path.resolve(__dirname,`../../.tmp/rockbar-options-${name}.png`)});
  }
  await page.reload();await page.locator('#settings').click();await page.locator('#config-options-tab').click();
  assert.equal(await snap.isChecked(),false);assert.equal(await page.locator('#color-theme').inputValue(),'2');
  await page.evaluate(()=>window.failOptionSave=true);await snap.click();
  await page.waitForFunction(()=>document.querySelector('#window-options-error').textContent==='保存テスト失敗');
  assert.equal(await snap.isChecked(),false,'failed save restores the last saved value');
  await page.locator('#config-options-tab').focus();await page.keyboard.press('Home');
  assert.equal(await page.locator('#config-connection-tab').getAttribute('aria-selected'),'true');
  await page.keyboard.press('End');assert.equal(await page.locator('#config-options-tab').getAttribute('aria-selected'),'true');
  await page.locator('#options-close').click();assert.equal(await page.locator('#config').isVisible(),false);
  assert.deepEqual(errors,[]);console.log('PASS: options tab, saved snap toggle, failure rollback, theme, tab keyboard navigation');
 }finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
