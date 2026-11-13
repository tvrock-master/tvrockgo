(function (root, factory) {
  if (typeof module !== 'undefined') module.exports = factory(require('./programs.js'));
  else root.rockbarProtocol = factory(root.programGuide);
})(globalThis, function (guide) {
function serverURL(value) {
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error('認証情報・クエリを含まない http:// または https:// のURLを指定してください');
  }
  return url.href.replace(/\/+$/, '');
}

function command(device, action) {
  if (!device?.RegistryCode) throw new Error('デバイスが見つかりません');
  const state = device.State;
  if (action === 'power-record' && state === 0) {
    return 'BDV:' + JSON.stringify({ registryid: device.RegistryCode, boottype: 2 });
  }
  if (action === 'power' && [0, 2].includes(state)) {
    return 'BDV:' + JSON.stringify({ registryid: device.RegistryCode, boottype: state === 0 ? 1 : 0 });
  }
  const commands = { record: 'RecordStart', stop: 'RecordStop', capture: 'Capture' };
  if ((action === 'record' && state === 2 && device.CurrentService && device.CurrentProgram) ||
      (action === 'stop' && [2, 3].includes(state)) || (action === 'capture' && [2, 3].includes(state))) {
    return (action === 'capture' ? 'REQ:' : 'COM:') + JSON.stringify({
      id: device.RegistryCode, command: commands[action], message: device.ID,
    });
  }
  throw new Error('現在のデバイス状態では操作できません');
}
function tuneCommands(device, sid, stations) {
  const { services } = guide;
  if (!device?.RegistryCode || device.State !== 2) throw new Error('視聴中のデバイスだけ選局できます');
  const target = stations.find(item => item.Sid === sid);
  const station = services(device, stations, target?.Tsp, true).find(item => item.Sid === sid);
  if (!station) throw new Error('この放送波では選局できないサービスです');
  const ids = station.Parent ? [(8 << 16) | (sid & 0xffff), station.Parent] : [sid];
  return ids.map(value => 'REQ:' + JSON.stringify({ id: device.RegistryCode, command: 'StreamChange', message: String(value) }));
}
return { serverURL, command, tuneCommands };

});
