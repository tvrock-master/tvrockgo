const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');

function setup() {
  const source=fs.readFileSync(path.join(__dirname,'../src/renderer.js'),'utf8');
  const start=source.indexOf("document.addEventListener('keydown', handlePlaybackShortcut);");
  const end=source.indexOf('async function changePlaybackBookmark',start);
  const clicks=[],buttons=new Map();
  const button=id=>{
    if(!buttons.has(id))buttons.set(id,{disabled:false,click(){if(!this.disabled)clicks.push(id);}});
    return buttons.get(id);
  };
  let handler;
  const context={connected:true,timekeeperMode:false,selected:'PBK01',devices:[{ID:'PBK01',Kind:'playback',Playback:{phase:'playing'}}],dialog:false,$:button};
  context.document={addEventListener(type,callback){assert.equal(type,'keydown');handler=callback;},querySelector(selector){return selector==='dialog[open]'?context.dialog:button(selector);}};
  vm.runInNewContext(source.slice(start,end),context);
  function press(key,options={}) {
    const event={key,target:{closest:()=>false},preventDefault(){this.defaultPrevented=true;},...options};
    handler(event);
    return Boolean(event.defaultPrevented);
  }
  return {context,clicks,button,press};
}

test('playback shortcuts use the same controls for pause, resume, seeking and boundaries',()=>{
  const {context,clicks,press}=setup();
  assert.equal(press(' '),true);
  context.devices[0].Playback.phase='paused';
  assert.equal(press(' '),true);
  for(const key of ['ArrowRight','ArrowLeft']) {
    assert.equal(press(key),true);
    assert.equal(press(key,{ctrlKey:true}),true);
    assert.equal(press(key,{shiftKey:true}),true);
  }
  assert.deepEqual(clicks,['playback-pause','playback-pause',
    '[data-playback="seek"][data-seconds="15"]','[data-playback="seek"][data-seconds="30"]','playback-cm-next',
    '[data-playback="seek"][data-seconds="-15"]','[data-playback="seek"][data-seconds="-30"]','playback-cm-previous']);
});

test('shortcuts leave editing, dialogs and unrelated keyboard operations alone',()=>{
  const {press,clicks}=setup();
  for(const options of [{repeat:true},{isComposing:true},{altKey:true},{metaKey:true},
    {ctrlKey:true,shiftKey:true},{target:{closest:()=>true}}])assert.equal(press('ArrowRight',options),false);
  for(const options of [{ctrlKey:true},{shiftKey:true}])assert.equal(press(' ',options),false);
  assert.equal(press('Enter'),false);
  press(' ',{defaultPrevented:true});
  assert.deepEqual(clicks,[]);
});

test('shortcuts require a connected active playback device and respect disabled boundaries',()=>{
  for(const change of [ctx=>ctx.connected=false,ctx=>ctx.timekeeperMode=true,ctx=>ctx.dialog=true,
    ctx=>ctx.selected='REC01',ctx=>ctx.devices[0].Kind='normal',
    ...['idle','indexing','preparing','ended','failed'].map(phase=>ctx=>ctx.devices[0].Playback.phase=phase)]) {
    const {context,press,clicks}=setup();
    change(context);
    assert.equal(press(' '),false);
    assert.equal(press('ArrowRight'),false);
    assert.deepEqual(clicks,[]);
  }
  const {button,press,clicks}=setup();
  button('playback-cm-next').disabled=true;
  assert.equal(press('ArrowRight',{shiftKey:true}),true);
  assert.deepEqual(clicks,[]);
});
