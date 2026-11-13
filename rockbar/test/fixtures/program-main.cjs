const { app, BrowserWindow, session, nativeTheme } = require('electron');
const path = require('node:path');
app.setPath('userData', path.resolve(__dirname, '../../test-output/program-electron-profile'));
nativeTheme.themeSource = 'dark';
app.whenReady().then(() => {
  session.defaultSession.webRequest.onBeforeRequest({ urls: ['https://fonts.googleapis.com/*', 'https://fonts.gstatic.com/*'] }, (_details, callback) => callback({ cancel: true }));
  const window = new BrowserWindow({ width: 1200, height: 700, show: false,
    webPreferences: { preload: path.join(__dirname, 'program-preload.cjs'), contextIsolation: true, nodeIntegration: false, sandbox: true },
  });
  window.loadFile(path.resolve(__dirname, '../../src/index.html'));
});
