const {app,BrowserWindow,session,ipcMain,screen}=require('electron');
const {createWindowLayout,attachEdgeSnapping}=require('../../electron/window-layout.cjs');
const path=require('node:path');
app.setPath('userData',path.resolve(__dirname,'../../../.tmp/rockbar-resize-profile'));
app.whenReady().then(()=>{
  session.defaultSession.webRequest.onBeforeRequest({urls:['https://fonts.googleapis.com/*','https://fonts.gstatic.com/*']},(_details,callback)=>callback({cancel:true}));
  const win=new BrowserWindow({width:1000,height:600,minWidth:370,minHeight:150,frame:false,show:true,
    webPreferences:{preload:path.join(__dirname,'resize-preload.cjs'),contextIsolation:true,nodeIntegration:false,sandbox:true}});
  const config={};const layout=createWindowLayout(win,screen,config);
  attachEdgeSnapping(win,screen,()=>config.snapToEdges!==false);
  ipcMain.handle('test-window-layout',()=>layout.toggle());
  ipcMain.handle('test-settings',()=>({server:'http://localhost',compact:layout.compact,snapToEdges:config.snapToEdges!==false}));
  ipcMain.handle('test-window-options',(_event,value)=>{config.snapToEdges=value.snapToEdges;return {snapToEdges:config.snapToEdges};});
  win.loadFile(path.resolve(__dirname,'../../src/index.html'));
});
