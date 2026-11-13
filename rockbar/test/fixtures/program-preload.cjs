const { contextBridge } = require('electron');
let update;
contextBridge.exposeInMainWorld('rockbar', {
  onUpdate: callback => { update = callback; },
  settings: async () => ({ server: 'http://localhost' }), connect: async () => ({}),
  watchDevice: async () => {}, setMode: async () => {}, tune: async () => ({}),
});
contextBridge.exposeInMainWorld('testUpdate', message => update(message));
