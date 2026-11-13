const { contextBridge, ipcRenderer } = require('electron');
contextBridge.exposeInMainWorld('rockbar', {
  settings: () => ipcRenderer.invoke('settings'),
  viewerSettings: base => ipcRenderer.invoke('viewer-settings',base),
  saveViewerSettings: (base,registry,value) => ipcRenderer.invoke('save-viewer-settings',base,registry,value),
  pickViewer: () => ipcRenderer.invoke('pick-viewer'),
  closeViewerSettings: () => ipcRenderer.invoke('close-viewer-settings'),
  guide: () => ipcRenderer.invoke('guide'),
  connect: value => ipcRenderer.invoke('connect', value),
  action: (id, action) => ipcRenderer.invoke('action', id, action),
  playback: (id, action, seconds) => ipcRenderer.invoke('playback', id, action, seconds),
  playbackRecordings: () => ipcRenderer.invoke('playback-recordings'),
  tune: (id, sid) => ipcRenderer.invoke('tune', id, sid),
  watchDevice: id => ipcRenderer.invoke('watch-device', id),
  setMode: mode => ipcRenderer.invoke('display-mode', mode),
  sleepTimerPause: paused => ipcRenderer.invoke('sleep-timer-pause', paused),
  login: value => ipcRenderer.invoke('login', value),
  window: action => ipcRenderer.invoke('window', action),
  windowLayout: () => ipcRenderer.invoke('window-layout'),
  windowOptions: value => ipcRenderer.invoke('window-options',value),
  onUpdate: callback => {
    const listener = (_event, data) => callback(data);
    ipcRenderer.on('update', listener);
    return () => ipcRenderer.removeListener('update', listener);
  },
});
