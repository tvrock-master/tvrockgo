(function (root) {
  const startTime = item => item.starttime - (Number(item.readytime) || 0);
  function select(schedules, deviceID, now) {
    if (!deviceID || !Number.isFinite(now)) return null;
    const available = (schedules || []).filter(item => item.deviceid === deviceID && item.validate && !item.complete &&
      Number.isFinite(item.starttime) && Number.isFinite(item.endtime) && now < item.endtime + (Number(item.tailtime) || 0))
      .sort((a, b) => startTime(a) - startTime(b) || a.id - b.id);
    const active = available.filter(item => startTime(item) - (Number(item.idletime) || 0) <= now);
    return active.at(-1) || available[0] || null;
  }
  function duration(seconds) {
    const value = Math.max(0, Math.floor(seconds));
    return [Math.floor(value / 3600), Math.floor(value / 60) % 60, value % 60]
      .map(part => String(part).padStart(2, '0')).join(':');
  }
  function display(item, now) {
    if (!item) return { label: 'このデバイスでの予約はありません。', time: '', title: '' };
    const start = startTime(item), end = item.endtime + (Number(item.tailtime) || 0);
    let label, time;
    if (now < start - (Number(item.idletime) || 0)) {
      label = '次の予約';
      const date = new Date(item.starttime * 1000);
      const endDate = new Date(item.endtime * 1000);
      const today = new Date(now * 1000);
      const sameDay = (a, b) => a.getFullYear() === b.getFullYear() &&
        a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
      const format = (value, reference) => (sameDay(value, reference) ? '' : `${value.getMonth() + 1}/${value.getDate()} `) +
        `${String(value.getHours()).padStart(2, '0')}:${String(value.getMinutes()).padStart(2, '0')}`;
      time = `${format(date, today)}～${format(endDate, date)}`;
    } else if (now < start) {
      label = '予約開始まで'; time = duration(start - now);
    } else if (now >= end - 30) {
      label = '予約終了まで'; time = duration(end - now);
    } else {
      label = '予約実行中'; time = `+${duration(now - start)} / ${duration(end - start)}`;
    }
    return { label, time, title: item.title || '' };
  }
  function timekeeper(device, schedules, stations, now, recordings = null, recordingIDs = null) {
    const blank = '--:--:--';
    const emptyRange = `${blank}～${blank}`;
    const available = (schedules || []).filter(item => item.deviceid === device.ID && item.validate && !item.complete &&
      Number.isFinite(item.starttime) && Number.isFinite(item.endtime) && now < item.endtime + (Number(item.tailtime) || 0))
      .sort((a, b) => startTime(a) - startTime(b) || a.id - b.id);
    const active = available.filter(item => !item.watchonly && startTime(item) <= now &&
      (!device.CurrentService || item.sid === device.CurrentService));
    const runtime = recordings?.[device.ID];
    const running = runtime && { ...runtime, ...(schedules || []).find(item => item.id === runtime.id), deviceid: device.ID };
    const sameService = item => !device.CurrentService || item.sid === device.CurrentService;
    // Legacy servers expose running IDs without their actual device bindings.
    // Resolve a unique running ID using the received service/event, not the SQL assignment.
    const byEvent = device.CurrentProgram?.Eid && Array.isArray(recordingIDs) ? (schedules || []).filter(item =>
      recordingIDs.includes(item.id) && sameService(item) && item.eid === device.CurrentProgram.Eid) : [];
    const identified = running && sameService(running) ? running : byEvent.length === 1 ? byEvent[0] : null;
    const valid = identified && !identified.watchonly && Number.isFinite(identified.starttime) &&
      Number.isFinite(identified.endtime) && identified.endtime + (Number(identified.tailtime) || 0) > startTime(identified) && startTime(identified) <= now;
    const recording = device.State === 3 ? (valid ? identified : active.find(item => item.eid && item.eid === device.CurrentProgram?.Eid) || active.at(-1)) : null;
    const waiting = available.find(item => !item.watchonly &&
      item.starttime - (Number(item.idletime) || 0) <= now && now < startTime(item));
    const current = recording || waiting;
    const next = available.find(item => item.id !== current?.id && startTime(item) > now);
    const station = sid => (stations || []).find(item => item.Sid === sid)?.Name || (sid ? `SID ${sid}` : '');
    const date = value => new Date(value * 1000);
    const clock = (value, reference = value) => {
      const d = date(value), r = date(reference);
      const day = new Date(d.getFullYear(), d.getMonth(), d.getDate()) - new Date(r.getFullYear(), r.getMonth(), r.getDate());
      return [d.getHours() + Math.round(day / 86400000) * 24, d.getMinutes(), d.getSeconds()]
        .map(part => String(part).padStart(2, '0')).join(':');
    };
    const range = (start, end) => `${clock(start)}～${clock(end, start)}`;
    const start = current && startTime(current), end = current && current.endtime + (Number(current.tailtime) || 0);
    // A recording without an identifiable reservation must not invent a start time.
    const title = current?.title || (device.State === 3 ? device.CurrentProgram?.Title || '録画情報を待っています' : '予約はありません');
    const values = [
      title, current || device.State === 3 ? station(current?.sid || device.CurrentService) : '',
      current ? range(start, end) : emptyRange,
      current ? `${now < start ? '-' : ''}${duration(Math.abs(now - start))} / ${duration(end - start)}` : `${blank} / ${blank}`,
      current ? duration(end - now) : blank,
      next?.title || '予約はありません', next ? station(next.sid) : '',
      next ? `${date(startTime(next)).getMonth() + 1}/${date(startTime(next)).getDate()} ${range(startTime(next), next.endtime + (Number(next.tailtime) || 0))}` : emptyRange,
    ];
    return { values, currentID: current?.id ?? null, nextID: next?.id ?? null, waiting: current === waiting && !!waiting,
      ratio: current ? Math.max(0, Math.min(1, (now - start) / (end - start))) : 0 };
  }
  const api = { select, duration, display, timekeeper };
  if (typeof module !== 'undefined') module.exports = api;
  else root.recordingSchedules = api;
})(globalThis);
