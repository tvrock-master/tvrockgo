const {chromium}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({channel:'msedge',headless:true});
 try{
  const page=await browser.newPage();
  await page.setContent('<style>#view{height:32px;width:180px;overflow:auto;overflow-anchor:none;font:16px/16px monospace;overflow-wrap:anywhere}#lines>div{min-height:16px}</style><div id="view"><div id="lines"></div></div>');
  await page.addScriptTag({path:path.resolve(__dirname,'../src/captions.js')});
  await page.addScriptTag({path:path.resolve(__dirname,'../src/caption-scroll.js')});
  await page.evaluate(()=>{window.scroller=new CaptionScroller(document.querySelector('#view'),document.querySelector('#lines'));scroller.update('a',[{ID:1,Text:'one'},{ID:2,Text:'two'}]);});
  await page.waitForTimeout(50);
  const start=await page.evaluate(()=>{scroller.update('a',[{ID:2,Text:'two'},{ID:3,Text:'three'}]);window.positions=[];const sample=()=>{const v=document.querySelector('#view');positions.push({top:v.scrollTop,target:v.scrollHeight-v.clientHeight});if(scroller.frame)requestAnimationFrame(sample);};requestAnimationFrame(sample);return {count:document.querySelector('#lines').children.length,top:document.querySelector('#view').scrollTop};});
  assert.deepEqual(start,{count:3,top:0});
  await page.waitForFunction(()=>document.querySelector('#lines').children.length===2);
  assert.ok(await page.evaluate(()=>positions.some(p=>p.top>0&&p.top<p.target)), JSON.stringify(await page.evaluate(()=>positions)));
  assert.equal(await page.locator('#lines').innerText(),'two\nthree');
  await page.evaluate(()=>{scroller.update('a',[{ID:3,Text:'three'},{ID:4,Text:'four'}]);scroller.update('b',[{ID:5,Text:'other program'}]);});
  assert.equal(await page.locator('#lines').innerText(),'other program');
  await page.waitForTimeout(300);assert.equal(await page.locator('#lines').innerText(),'other program');
  await page.evaluate(()=>{for(let n=6;n<50;n++)scroller.update('b',[{ID:n-1,Text:'line '+(n-1)},{ID:n,Text:'line '+n}]);});
  assert.ok(await page.locator('#lines').evaluate(el=>el.children.length)<=6);
  await page.waitForFunction(()=>document.querySelector('#lines').children.length===2);
  assert.equal(await page.locator('#lines').innerText(),'line 48\nline 49');
  await page.emulateMedia({reducedMotion:'reduce'});
  await page.evaluate(()=>scroller.update('b',[{ID:49,Text:'line 49'},{ID:50,Text:'line 50'}]));
  assert.equal(await page.locator('#lines').innerText(),'line 49\nline 50');
  assert.equal(await page.locator('#lines').evaluate(el=>el.children.length),2);
  console.log('PASS caption scroll: intermediate animation frames, final two lines, cancellation on switch, bounded burst updates, reduced motion');
 }finally{await browser.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});

