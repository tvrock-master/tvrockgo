const {test}=require('node:test');
const assert=require('node:assert/strict');
const {Client}=require('../src/client.js');
const flush=()=>new Promise(resolve=>setImmediate(resolve));
test('station, schedule and power updates use notifications without periodic HTTP or full snapshots',async t=>{
 t.mock.timers.enable({apis:['setInterval','setTimeout']});
 const urls=[],sent=[],events=[],listeners={};
 const socket={readyState:1,addEventListener:(name,callback)=>{listeners[name]=callback;},send:text=>sent.push(text),close:()=>{}};
 const client=new Client((type,value)=>events.push({type,value}),async url=>{
  urls.push(url);const resource=new URL(url).pathname;
  const data=resource==='/dvs'?[{ID:'REC01',State:2}]:resource==='/sch'?[]:resource==='/rst'?{recording:[],currentmilliseconds:1700000000000}:resource==='/stl'?{1:{Sid:1,Name:'放送局',Tsp:1,View:1}}:resource==='/tvp'?{Tvp:{1:[]},CurrentMilliseconds:1700000000000}:{powermanager:true};
  return Response.json(data,{headers:{date:new Date(1700000000000).toUTCString()}});
 },async()=>socket);
 t.after(()=>client.close());await client.connect('http://test');listeners.open();await flush();await flush();
 const initial=urls.length,initialSent=sent.length;t.mock.timers.tick(120000);await flush();
 assert.equal(urls.length,initial);assert.equal(sent.length,initialSent);
 const count=path=>urls.filter(url=>url.endsWith(path)).length;
 for(const [notification,path] of [['usc:','/sch'],['ups:','/pms'],['ust:','/stl']]){
  const before=count(path);listeners.message({data:notification});await flush();await flush();assert.equal(count(path),before+1);
 }
 assert.ok(events.some(event=>event.type==='schedule-update'));
 client.devices.set('PBK01',{ID:'PBK01',Kind:'playback'});client.watchDevice('PBK01');
 const before=count('/tvp');listeners.message({data:'utt:'});await flush();assert.equal(count('/tvp'),before,'Playback never requests the broadcast guide');
 client.watchDevice('REC01');await flush();await flush();assert.ok(count('/tvp')>before,'normal mode fetches deferred guide updates');
});
