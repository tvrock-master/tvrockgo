const {_electron}=require(process.env.PLAYWRIGHT_PATH||'playwright');
const path=require('node:path');
const assert=require('node:assert/strict');
(async()=>{
  const env={...process.env};delete env.ELECTRON_RUN_AS_NODE;
  const app=await _electron.launch({executablePath:require('electron'),args:[path.join(__dirname,'fixtures/resize-main.cjs')],env});
  try {
    const page=await app.firstWindow();await page.waitForFunction(()=>typeof testUpdate==='function' && document.querySelector('#current-program .program-scroll-content'));
    await page.evaluate(()=>{testUpdate({type:'status',value:{connected:true}});testUpdate({type:'devices',value:[{ID:'PBK01',State:2,Kind:'playback',Playback:{session:'one',phase:'playing',title:'再生タイトル',generation:1,positionMs:1000,durationMs:600000}}]});});
    await page.waitForFunction(()=>document.querySelectorAll('[data-file]').length===2000);
    assert.deepEqual(await page.evaluate(()=>['body','main','footer','header button'].map(selector=>getComputedStyle(document.querySelector(selector)).webkitAppRegion)),['drag','no-drag','no-drag','no-drag'],'margin moves the window while content and controls accept input');
    assert.deepEqual(await page.evaluate(()=>['header .brand','header .brand small','header #connection'].map(selector=>getComputedStyle(document.querySelector(selector)).webkitAppRegion)),['drag','drag','drag'],'title text and connection indicator explicitly keep the drag region');
    await page.evaluate(()=>{playbackFileOrder='title-asc';render();});
    for(const zoom of [1,1.25,1.5]) {
      await app.evaluate(({BrowserWindow},zoom)=>BrowserWindow.getAllWindows()[0].webContents.setZoomFactor(zoom),zoom);
      for(const size of [[1034,836],[1374,1092],[680,360],[1200,700]]) {
        const native=await app.evaluate(({BrowserWindow},size)=>{const win=BrowserWindow.getAllWindows()[0];win.setAlwaysOnTop(true);win.show();win.focus();win.setSize(...size);return {size:win.getContentSize()};},size);
        await page.waitForFunction(({size,zoom})=>{
          const bounds=document.body.getBoundingClientRect();
          return Math.abs(bounds.width*zoom-size[0])<=2 && Math.abs(bounds.height*zoom-size[1])<=2;
        },{size:native.size,zoom},{timeout:5000});
        const actual=await page.evaluate(()=>({viewport:[innerWidth,innerHeight],body:[document.body.getBoundingClientRect().width,document.body.getBoundingClientRect().height],footer:document.querySelector('footer').getBoundingClientRect().bottom}));
        console.log(JSON.stringify({zoom,native,...actual}));
        for(let axis=0;axis<2;axis++)assert.ok(Math.abs(actual.body[axis]*zoom-native.size[axis])<=2,'body must fill native content after resize');
      }
    }
    console.log('PASS Electron native resize at 100%, 125% and 150% zoom');
  } finally {await app.close();}
})().catch(error=>{console.error(error);process.exitCode=1;});
