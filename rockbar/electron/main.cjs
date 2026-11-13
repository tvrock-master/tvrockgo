const { app, BrowserWindow, ipcMain, screen, session, nativeImage, nativeTheme, dialog, shell } = require('electron');
const { openProgramLink } = require('./external-links.cjs');
const { GuideWindow } = require('./guide.cjs');
const path = require('node:path');
const fs = require('node:fs');
const { Client } = require('./client.cjs');
const { serverURL } = require('./protocol.cjs');
const { RemoteViewer } = require('./remote-viewer.cjs');
const { PlaybackViewer } = require('./playback-viewer.cjs');
const { createWindowLayout, attachEdgeSnapping } = require('./window-layout.cjs');
let playbackViewer;
let win, client, viewer, loginWindow, viewerSettingsBounds, config = {};
function emitUpdate(type,value) { if (win && !win.isDestroyed()) win.webContents.send('update',{type,value}); }
function requireLocalSettings(event) {
  if (!win || event.sender!==win.webContents || event.senderFrame!==win.webContents.mainFrame || !event.senderFrame.url.startsWith('file:')) throw new Error('ローカル設定画面からのみ操作できます');
}
const settingsPath = () => path.join(app.getPath('userData'), 'settings.json');
function save() { fs.writeFileSync(settingsPath(), JSON.stringify(config, null, 2)); }
if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on('second-instance', () => { if (win) { win.restore(); win.show(); win.focus(); } });
  app.whenReady().then(() => {
    try { config = JSON.parse(fs.readFileSync(settingsPath(), 'utf8')); } catch {}
    let bounds = config.bounds || { width: 680, height: 240 };
    if (!screen.getAllDisplays().some(({ workArea: a }) => bounds.x >= a.x && bounds.x < a.x + a.width && bounds.y >= a.y && bounds.y < a.y + a.height)) {
      bounds = { width: bounds.width, height: bounds.height };
    }
    win = new BrowserWindow({ ...bounds, minWidth: 370, minHeight: 150, frame: false,
      backgroundColor: nativeTheme.shouldUseDarkColors ? '#17191c' : '#8795a4', show: false, autoHideMenuBar: true,
      webPreferences: { preload: path.join(__dirname, 'preload.cjs'), contextIsolation: true, nodeIntegration: false, sandbox: true },
    });
    const windowLayout=createWindowLayout(win,screen,config);
    attachEdgeSnapping(win,screen,()=>config.snapToEdges!==false);
    ipcMain.handle('window-options',(event,value)=>{
      try {
        requireLocalSettings(event);
        if(!value || typeof value.snapToEdges!=='boolean')throw new Error('吸着設定はON/OFFで指定してください');
        const previous=config.snapToEdges;
        config.snapToEdges=value.snapToEdges;
        try {save();} catch(error) {if(previous===undefined)delete config.snapToEdges;else config.snapToEdges=previous;throw error;}
        return {snapToEdges:config.snapToEdges};
      } catch(error) {return {error:error.message};}
    });
    ipcMain.handle('window-layout',event=>{
      requireLocalSettings(event);
      if(viewerSettingsBounds)return {compact:windowLayout.compact,error:'視聴アプリ設定を閉じてから切り替えてください。'};
      return windowLayout.toggle();
    });
    nativeTheme.on('updated', () => {
      if (!win.isDestroyed()) win.setBackgroundColor(nativeTheme.shouldUseDarkColors ? '#17191c' : '#8795a4');
    });
    win.webContents.setWindowOpenHandler(({ url }) => {
      void openProgramLink(shell, url);
      return { action: 'deny' };
    });
    win.webContents.on('will-navigate', event => event.preventDefault());
    win.webContents.session.setPermissionRequestHandler((_wc, _permission, callback) => callback(false));
    const guide = new GuideWindow(BrowserWindow, nativeImage, (type, value) => {
      if (!win.isDestroyed()) win.webContents.send('update', { type, value });
    }, url => openProgramLink(shell, url));
    ipcMain.handle('guide', () => {
      try { guide.toggle(config.server || 'http://localhost'); return { ok: true }; }
      catch (error) { return { error: error.message }; }
    });
    client = new Client((type, value) => {
      if (type==='remote-watch') viewer?.accept(value);
      if (type==='playback') playbackViewer?.accept(value);
      if (type==='status' && !value.connected) playbackViewer?.shutdown();
      if (type==='status') { viewer?.connection(value.connected); viewer?.publish(); }
      emitUpdate(type,value);
    },
      (url, options) => session.defaultSession.fetch(url, { ...options, credentials: 'include' }),
      url => session.defaultSession.cookies.get({ url }));
    viewer=new RemoteViewer({client,config,save,emit:emitUpdate});
    playbackViewer=new PlaybackViewer({client,viewer,emit:emitUpdate});
    viewer.externalPorts=()=>[...playbackViewer.views.values(),...playbackViewer.pending.values(),...playbackViewer.closing.values()].map(value=>value.setting.port);
    ipcMain.handle('playback', (_event,id,action,seconds) => {
      try {client.playback(id,action,seconds);return {ok:true};} catch(error) {return {error:error.message};}
    });
    ipcMain.handle('playback-recordings', () => client.playbackRecordings());
    ipcMain.handle('viewer-settings', (event,value) => {
      try {
        requireLocalSettings(event);
        const base=serverURL(value);
        if (base!==client.base || !client.connected) throw new Error('設定するURLに接続してから開いてください');
        if (!viewerSettingsBounds) {
          viewerSettingsBounds=win.getNormalBounds();
          const area=screen.getDisplayMatching(viewerSettingsBounds).workArea;
          const width=Math.min(Math.max(viewerSettingsBounds.width,640),area.width);
          const height=Math.min(Math.max(viewerSettingsBounds.height,720),area.height);
          win.setBounds({x:Math.max(area.x,Math.min(viewerSettingsBounds.x,area.x+area.width-width)),y:Math.max(area.y,Math.min(viewerSettingsBounds.y,area.y+area.height-height)),width,height});
        }
        const devices=[...client.devices.values()].filter(d=>d.Kind!=='playback').concat((client.playbackDefinitions||[]).map(d=>({...d,Kind:'playback'})));
        return {devices:devices.map((d,index)=>({id:d.ID,kind:d.Kind||'normal',registry:d.RegistryCode,setting:viewer.settings(base,d.RegistryCode)||{enabled:false,executable:'',options:'/d BonDriver_SRT.dll /nd',port:3230+index%10,transport:'srt'}}))};
      } catch(error) { return {error:error.message}; }
    });
    ipcMain.handle('save-viewer-settings', (event,base,registry,value) => {
      try {
        requireLocalSettings(event);
        const playbackDevice=(client.playbackDefinitions||[]).some(d=>d.RegistryCode===registry) || [...client.devices.values()].some(d=>d.Kind==='playback' && d.RegistryCode===registry);
        if(playbackDevice && value?.transport!=='srt')throw new Error('再生デバイスの配信方式はSRTを指定してください');
        if([...playbackViewer.views.values(),...playbackViewer.pending.values()].some(view=>view.device===registry))throw new Error('再生を停止してから設定を変更してください');
        return {setting:viewer.saveSettings(base,registry,value)};
      }
      catch(error) { return {error:error.message}; }
    });
    ipcMain.handle('close-viewer-settings',event=>{
      requireLocalSettings(event);
      if (viewerSettingsBounds) { const bounds=viewerSettingsBounds; viewerSettingsBounds=null; win.setBounds(bounds); }
    });
    ipcMain.handle('pick-viewer', async event => {
      try {
        requireLocalSettings(event);
        const result=await dialog.showOpenDialog(win,{title:'視聴アプリの実行ファイル',properties:['openFile'],filters:[{name:'実行ファイル',extensions:['exe']}]});
        return result.canceled ? {} : {path:result.filePaths[0]};
      } catch(error) { return {error:error.message}; }
    });
    ipcMain.handle('login', async (_event, value) => {
      try {
        const base = serverURL(value);
        guide.changeServer(base);
        if (loginWindow && !loginWindow.isDestroyed()) loginWindow.close();
        viewer.changeServer();
        client.close();
        config.server = base; save();
        const login = new BrowserWindow({ width: 700, height: 620, parent: win, autoHideMenuBar: true,
          webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true },
        });
        loginWindow = login;
        login.webContents.setWindowOpenHandler(() => ({ action: 'deny' }));
        const restrict = (event, url) => { if (new URL(url).origin !== new URL(base).origin) event.preventDefault(); };
        login.webContents.on('will-navigate', restrict);
        login.webContents.on('will-redirect', restrict);
        login.webContents.on('did-navigate', (_event, url) => {
          if (url.startsWith(base + '/static/')) login.close();
        });
        login.on('closed', () => {
          if (loginWindow !== login) return;
          loginWindow = null;
          if (!win.isDestroyed()) void client.connect(config.server);
        });
        await login.loadURL(base + '/login');
        return { ok: true };
      } catch (error) { return { error: error.message }; }
    });
    ipcMain.handle('tune', (_event, id, sid) => {
      try { client.tune(id, sid); return { ok: true }; } catch (error) { return { error: error.message }; }
    });
    ipcMain.handle('watch-device', (_event, id) => { client.watchDevice(id); });
    ipcMain.handle('display-mode', (_event, mode) => { client.setMode(mode); });
    ipcMain.handle('sleep-timer-pause', async (_event, paused) => {
      try { return await client.setSleepTimerPause(paused); }
      catch (error) { return { error: error.message }; }
    });
    ipcMain.handle('settings', () => ({ server: config.server || 'http://localhost', nativeViewer:true, compact:windowLayout.compact, snapToEdges:config.snapToEdges!==false }));
    ipcMain.handle('connect', async (_event, value) => {
      try { const base=serverURL(value); if (client.base && base!==client.base) viewer.changeServer(); config.server = base; guide.changeServer(config.server); save(); void client.connect(config.server); return { ok: true }; }
      catch (error) { return { error: error.message }; }
    });
    ipcMain.handle('action', (event, id, action) => {
      try { requireLocalSettings(event); } catch(error) { return {error:error.message}; }
      try {
        const device=client.devices.get(id);
        if (device && action==='power' && viewer.configured(device.RegistryCode)) viewer.toggle(device);
        else client.send(id, action);
        return {ok:true};
      } catch (error) { return { error: error.message }; }
    });
    ipcMain.handle('window', (_event, action) => {
      if (action === 'minimize') win.minimize();
      if (action === 'close' && dialog.showMessageBoxSync(win, {
        type: 'question', title: 'Rockbar', message: 'rockbarを終了しますか？',
        buttons: ['はい', 'いいえ'], defaultId: 1, cancelId: 1, noLink: true,
      }) === 0) win.close();
      if (action === 'pin') { win.setAlwaysOnTop(!win.isAlwaysOnTop()); return win.isAlwaysOnTop(); }
    });
    let waitingForViewerExit=false;
    win.on('close', event => {
      if (!waitingForViewerExit && (viewer.hasChildren() || playbackViewer.hasChildren())) {
        event.preventDefault(); waitingForViewerExit=true;
        void Promise.all([viewer.shutdown(),playbackViewer.shutdown()]).finally(()=>{ if (!win.isDestroyed()) win.close(); });
        return;
      }
      void viewer.shutdown(); guide.close(); config.bounds=viewerSettingsBounds || win.getNormalBounds();
      windowLayout.remember(config.bounds);
      void playbackViewer.shutdown();
      try { save(); } catch {} client.close();
    });
    win.once('ready-to-show', () => win.show());
    win.loadFile(path.join(__dirname, '../src/index.html'));
  });
  app.on('window-all-closed', () => app.quit());
}
