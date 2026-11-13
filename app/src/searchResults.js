// 番組の同一性。タイトルが同じ再放送などは別結果として残す。
function key(program) {
  return [program.Sid, program.Eid, program.Start].join(':');
}

function reservationIndex(schedule) {
  const index = new Map();
  for (const item of schedule || []) {
    const id = item.sid + ':' + item.eid;
    if (item.validate && !index.has(id)) index.set(id, item);
  }
  return index;
}

function recordingInfo(item, index) {
  const schedule = item && index.get(item.Sid + ':' + item.Eid);
  return schedule ? { isRec: true, isWatch: !!schedule.watchonly && !schedule.recordonly,
    deviceID: schedule.deviceid, complete: schedule.complete } : {
    isRec: !!(item && item.IsRec), isWatch: !!(item && item.IsWatch),
    deviceID: (item && item.DeviceID) || '', complete: !!(item && item.Complete),
  };
}

// 非リアクティブ領域で受信し、小分けに処理する。Vueへの反映は最大10回/秒。
function create({ publish, complete, setTimer = setTimeout, clearTimer = clearTimeout, now = Date.now }) {
  let groups = new Map(), packets = [], offset = 0, timer = null, lastPublish = -Infinity;
  let finished = false, disposed = false, dirty = false;
  const flush = () => {
    timer = null;
    if (disposed) return;
    let remaining = 200;
    while (packets.length && remaining-- > 0) {
      const program = packets[0][offset++];
      if (offset >= packets[0].length) { packets.shift(); offset = 0; }
      const id = key(program);
      if (groups.has(id)) continue;
      groups.set(id, program);
      dirty = true;
    }
    const done = finished && packets.length === 0;
    if (done || (dirty && now() - lastPublish >= 100)) {
      const rows = [...groups.values()];
      if (done) rows.sort((a, b) => a.Start - b.Start || a.Sid - b.Sid || a.Eid - b.Eid);
      // 配列の深い監視を避ける。予約操作の反映は子のrevisionで明示する。
      publish(Object.freeze(rows));
      lastPublish = now(); dirty = false;
    }
    if (done) { complete(groups.size); return; }
    if (packets.length || dirty) timer = setTimer(flush, packets.length ? 0 : 100);
  };
  return {
    receive(items) {
      if (disposed) return;
      if (items && items.length) packets.push(items);
      if (timer === null) timer = setTimer(flush, 0);
    },
    finish() {
      if (disposed) return;
      finished = true; dirty = true; // 最終結果は並べ替えてから公開する。
      if (timer === null) timer = setTimer(flush, 0);
    },
    dispose() { disposed = true; packets = []; groups.clear(); if (timer !== null) clearTimer(timer); },
  };
}

module.exports = { key, reservationIndex, recordingInfo, create };
