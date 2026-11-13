const {test}=require('node:test');
const assert=require('node:assert/strict');
const {resizedBounds,snappedBounds,attachEdgeSnapping,createWindowLayout}=require('../electron/window-layout.cjs');
const {EventEmitter}=require('node:events');
test('size changes anchor nearby edges, clamp to work area and support negative monitor coordinates',()=>{
  const area={x:-1920,y:40,width:1920,height:1040};
  assert.deepEqual(resizedBounds({x:-900,y:480,width:900,height:600},{width:680,height:240},area),{x:-680,y:840,width:680,height:240});
  assert.deepEqual(resizedBounds({x:-1910,y:50,width:900,height:600},{width:680,height:240},area),{x:-1920,y:40,width:680,height:240});
  assert.deepEqual(resizedBounds({x:-1200,y:200,width:900,height:600},{width:680,height:240},area),{x:-1200,y:200,width:680,height:240});
  assert.deepEqual(resizedBounds({x:-500,y:900,width:400,height:150},{width:900,height:600},area),{x:-900,y:480,width:900,height:600});
  assert.deepEqual(resizedBounds({x:0,y:0,width:900,height:600},{width:3000,height:2000},area),area);
});
test('drag snapping attracts nearby edges without trapping movement between monitors',()=>{
  const area={x:-1920,y:40,width:1920,height:1040};
  assert.deepEqual(snappedBounds({x:-1910,y:45,width:680,height:240},area),{x:-1920,y:40,width:680,height:240});
  assert.deepEqual(snappedBounds({x:-690,y:830,width:680,height:240},area),{x:-680,y:840,width:680,height:240});
  const middle={x:-1300,y:200,width:900,height:600};assert.deepEqual(snappedBounds(middle,area),middle);
  const crossing={x:-300,y:200,width:680,height:240};assert.deepEqual(snappedBounds(crossing,area),crossing);
});
test('snap waits until manual dragging ends and never changes size',async()=>{
  const win=new EventEmitter();let enabled=true,maximized=false,fullscreen=false,applied=null;
  let bounds={x:0,y:0,width:680,height:240};
  win.isDestroyed=()=>false;win.isMaximized=()=>maximized;win.isFullScreen=()=>fullscreen;
  win.getBounds=()=>({...bounds});win.setPosition=(x,y)=>{bounds={...bounds,x,y};applied={...bounds};win.emit('moved');};
  const detach=attachEdgeSnapping(win,{getDisplayMatching:()=>({workArea:{x:0,y:0,width:1920,height:1040}})},()=>enabled);
  async function drag(x,y){
    applied=null;let prevented=false;
    for(let i=0;i<4;i++)win.emit('will-move',{preventDefault:()=>prevented=true},{x,y,width:bounds.width+100*i,height:bounds.height+100*i});
    assert.equal(prevented,false,'Windows drag must not be cancelled');assert.equal(applied,null,'drag must not reposition the window');
    bounds={...bounds,x,y};win.emit('moved');assert.equal(applied,null,'snap is deferred outside native move callback');
    await new Promise(resolve=>setImmediate(resolve));
  }
  for(const size of [{width:680,height:240},{width:900,height:600}]){
    bounds={x:200,y:200,...size};
    await drag(1920-size.width-10,1040-size.height-10);
    assert.deepEqual(bounds,{x:1920-size.width,y:1040-size.height,...size});
    await drag(300,200);assert.deepEqual(bounds,{x:300,y:200,...size},'pulling away preserves size');
  }
  enabled=false;await drag(5,5);assert.equal(applied,null);
  enabled=true;maximized=true;await drag(5,5);assert.equal(applied,null);
  maximized=false;fullscreen=true;await drag(5,5);assert.equal(applied,null);
  detach();assert.equal(win.listenerCount('will-move'),0);assert.equal(win.listenerCount('moved'),0);
});
test('toggle remembers independently adjusted normal and compact sizes and uses the current monitor',()=>{
  let bounds={x:1100,y:440,width:800,height:600},maximized=false;
  const config={};const win={getNormalBounds:()=>({...bounds}),setBounds:value=>bounds=value,isMaximized:()=>maximized,unmaximize:()=>maximized=false};
  const screen={getDisplayMatching:()=>({workArea:{x:0,y:0,width:1920,height:1040}})};
  const layout=createWindowLayout(win,screen,config);
  assert.deepEqual(layout.toggle(),{compact:true});
  bounds={...bounds,width:700,height:260};
  layout.toggle();assert.equal(bounds.width,800);assert.equal(bounds.height,600);
  bounds={...bounds,width:900,height:650};maximized=true;
  layout.toggle();assert.equal(maximized,false);assert.equal(bounds.width,700);assert.equal(bounds.height,260);
  layout.remember();assert.deepEqual(config.windowLayout.normalSize,{width:900,height:650});
  assert.equal(createWindowLayout(win,screen,config).compact,true);
});
