const path = require('node:path');
const { serverURL } = require('./protocol.cjs');

class GuideWindow {
  constructor(BrowserWindow, nativeImage, emit, openLink = () => {}) {
    this.BrowserWindow = BrowserWindow;
    this.nativeImage = nativeImage;
    this.emit = emit;
    this.openLink = openLink;
    this.window = null;
    this.base = null;
  }
  close() {
    if (this.window && !this.window.isDestroyed()) this.window.destroy();
  }
  changeServer(base) {
    if (this.base !== base) this.close();
  }
  toggle(server) {
    const base = serverURL(server);
    this.changeServer(base);
    if (this.window) {
      if (this.window.isVisible() && !this.window.isMinimized()) this.window.hide();
      else { this.window.restore(); this.window.show(); this.window.focus(); }
      return;
    }
    const win = new this.BrowserWindow({
      width: 1200, height: 800, minWidth: 480, minHeight: 320,
      title: 'tvrockgo 番組表', show: false, resizable: true,
      icon: this.nativeImage.createFromPath(path.join(__dirname, '../src/assets/guide-window.png')).resize({ width: 32, height: 32 }),
      webPreferences: { contextIsolation: true, nodeIntegration: false, sandbox: true },
    });
    this.window = win;
    this.base = base;
    win.setMenu(null);
    win.webContents.setWindowOpenHandler(({ url }) => {
      void this.openLink(url);
      return { action: 'deny' };
    });
    const restrict = (event, url) => {
      if (new URL(url).origin !== new URL(base).origin) event.preventDefault();
    };
    win.webContents.on('will-navigate', restrict);
    win.webContents.on('will-redirect', restrict);
    win.on('show', () => this.emit('guide-visible', true));
    win.on('hide', () => this.emit('guide-visible', false));
    win.on('closed', () => { this.window = null; this.emit('guide-visible', false); });
    // Show immediately so a second click can hide even while the page is loading.
    win.show();
    void win.loadURL(base + '/static/index.html').catch(error => {
      if (!win.isDestroyed()) this.emit('notice', '番組表を開けませんでした: ' + error.message);
    });
  }
}
module.exports = { GuideWindow };
