const {contextBridge,ipcRenderer}=require('electron');
let update;
contextBridge.exposeInMainWorld('rockbar',{
  onUpdate:callback=>{update=callback;},
  settings:()=>ipcRenderer.invoke('test-settings'),connect:async()=>({}),
  windowLayout:()=>ipcRenderer.invoke('test-window-layout'),
  windowOptions:value=>ipcRenderer.invoke('test-window-options',value),
  watchDevice:async()=>{},setMode:async()=>{},playbackRecordings:async()=>Array.from({length:2000},(_,id)=>({id,slot:0,title:`録画番組 ${id}`,starttime:1700000000+id,filename:`recording-${id}.ts`,complete:true})),
});
contextBridge.exposeInMainWorld('testUpdate',message=>update(message));
