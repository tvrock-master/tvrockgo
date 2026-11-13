(function (root) {
  const names = { 1: '地上波', 2: 'BS', 3: 'CS', 4: 'ワンセグ' };
  function wave(device, stations) {
    if (!device) return 0;
    const station = stations.find(item => item.Sid === device.CurrentService);
    const tsp = station?.Tsp || (device.CurrentService >>> 16);
    return names[tsp] ? tsp : 0;
  }
  function satellite(device, stations) {
    return device?.TunerType === 1 || [2, 3].includes(wave(device, stations));
  }
  function switchWaves(device, stations) {
    if (satellite(device, stations)) return [2, 3];
    return device?.Hybrid ? [1, 4] : [];
  }
  function services(device, stations, displayWave = wave(device, stations), multi = false) {
    const tsp = switchWaves(device, stations).includes(displayWave) ? displayWave : wave(device, stations);
    return stations.filter(item => tsp && item.Tsp === tsp && item.View > 0)
      .filter(item => multi || ![1, 2].includes(tsp) || !item.Parent)
      // DeviceController enumerates station objects keyed by numeric Sid in ascending order.
      .sort((a, b) => a.Sid - b.Sid);
  }
  function current(programs, now) {
    return (programs || []).find(item => item.Start <= now && now < item.End) || null;
  }
  function progress(program, now) {
    if (!program || !Number.isFinite(now) || !Number.isFinite(program.Start) ||
      !Number.isFinite(program.End) || program.End <= program.Start) return null;
    const duration = program.End - program.Start;
    const elapsed = Math.max(0, Math.min(duration, now - program.Start));
    const percent = Math.floor(elapsed / duration * 100);
    const minutes = value => Number((value / 60).toFixed(1));
    return { percent, ratio: elapsed / duration, segments: Math.floor(elapsed / duration * 20),
      text: `${Math.floor(elapsed / 60)} / ${minutes(duration)}min(${percent}%)` };
  }
  const api = { names, wave, satellite, switchWaves, services, current, progress };
  if (typeof module !== 'undefined') module.exports = api;
  else root.programGuide = api;
})(globalThis);
