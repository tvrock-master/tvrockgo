const {_electron}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path'),assert=require('node:assert/strict');
(async()=>{
 const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
 const app=await _electron.launch({executablePath:require('electron'),args:[path.join(__dirname,'fixtures/resize-main.cjs')],env});
 try {
  const page=await app.firstWindow();await page.locator('#window-layout').waitFor({state:'visible'});
  await page.locator('#settings').click();await page.locator('#config-options-tab').click();
  const area=await app.evaluate(({BrowserWindow,screen})=>screen.getDisplayMatching(BrowserWindow.getAllWindows()[0].getBounds()).workArea);
  async function moveNearEdge(){return app.evaluate(async({BrowserWindow},area)=>{
    const win=BrowserWindow.getAllWindows()[0];const {width,height}=win.getBounds();
    const proposed={x:area.x+area.width-width-10,y:area.y+area.height-height-10,width,height};
    let prevented=false;win.emit('will-move',{preventDefault:()=>prevented=true},proposed);
    win.setPosition(proposed.x,proposed.y);win.emit('moved');
    await new Promise(resolve=>setImmediate(resolve));
    const snapped=win.getBounds();
    win.emit('will-move',{preventDefault:()=>prevented=true},{...proposed,x:proposed.x-100,width:width+300});
    win.setPosition(proposed.x-100,area.y+100);win.emit('moved');
    await new Promise(resolve=>setImmediate(resolve));
    const pulled=win.getBounds();
    return {prevented,bounds:snapped,pulled,width,height};
  },area);}
  for(const compact of [false,true]){
    await page.locator('#options-close').click();
    if(compact)await page.locator('#window-layout').click();
    await page.locator('#settings').click();await page.locator('#config-options-tab').click();
    await page.locator('#snap-to-edges').check();
    let result=await moveNearEdge();assert.equal(result.prevented,false);
    assert.equal(result.pulled.width,result.width);assert.equal(result.pulled.height,result.height);
    assert.equal(result.pulled.x,area.x+area.width-result.width-110);
    assert.equal(result.bounds.x+result.bounds.width,area.x+area.width);assert.equal(result.bounds.y+result.bounds.height,area.y+area.height);
    await page.locator('#snap-to-edges').uncheck();
    result=await moveNearEdge();assert.equal(result.prevented,false);
    assert.equal(result.bounds.x+result.bounds.width,area.x+area.width-10);
  }
  await page.reload();await page.locator('#settings').click();await page.locator('#config-options-tab').click();
  assert.equal(await page.locator('#snap-to-edges').isChecked(),false);
  console.log('PASS Electron: move-event snapping and OFF in normal/compact modes; option restored on reload');
 }finally{await app.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
