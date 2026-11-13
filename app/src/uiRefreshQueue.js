// 通知による取得だけを直列化する。画面操作による取得はこのキューに入れない。
function create({ run, interval = 1000, now = Date.now, setTimer = setTimeout,
  clearTimer = clearTimeout, onError = () => {} }) {
  let pending = new Set();
  let running = false;
  let timer = null;
  let lastStarted = -Infinity;
  let disposed = false;

  const pump = () => {
    if (disposed || running || timer !== null || pending.size === 0) return;
    const delay = Math.max(0, interval - (now() - lastStarted));
    if (delay > 0) {
      timer = setTimer(() => { timer = null; pump(); }, delay);
      return;
    }
    const flags = pending;
    pending = new Set();
    running = true;
    lastStarted = now();
    Promise.resolve().then(() => disposed ? undefined : run(flags)).catch(onError).finally(() => {
      running = false;
      pump();
    });
  };
  return {
    request(...flags) {
      if (disposed) return;
      flags.forEach(flag => pending.add(flag));
      pump();
    },
    dispose() {
      disposed = true;
      pending.clear();
      if (timer !== null) clearTimer(timer);
      timer = null;
    },
  };
}

function reservationIndex(schedules) {
  const index = new Map();
  for (const schedule of schedules || []) {
    const key = schedule.sid + ':' + schedule.eid;
    if (schedule.validate && !index.has(key)) index.set(key, schedule);
  }
  return index;
}

function patchPrograms(programs, index, threshold) {
  let changed = false;
  const result = (programs || []).map(program => {
    const schedule = program.Start > threshold ? index.get(program.Sid + ':' + program.Eid) : null;
    const flags = {
      ID: schedule ? schedule.id : 0,
      IsRec: !!schedule,
      IsWatch: !!(schedule && schedule.watchonly && !schedule.recordonly),
      DeviceID: schedule ? schedule.deviceid : '',
      Complete: schedule ? schedule.complete : undefined,
      SearchID: schedule ? schedule.searchid : 0,
    };
    if (Object.keys(flags).every(key => program[key] === flags[key])) return program;
    changed = true;
    return { ...program, ...flags };
  });
  return changed ? result : programs;
}

function patchTable(table, index, threshold) {
  if (!table || !table.Tvp) return table;
  let changed = false;
  const programs = { ...table.Tvp };
  for (const key of Object.keys(programs)) {
    programs[key] = patchPrograms(programs[key], index, threshold);
    changed = changed || programs[key] !== table.Tvp[key];
  }
  return changed ? { ...table, Tvp: programs } : table;
}

function patchFeatures(features, index, threshold) {
  if (!features) return features;
  let changed = false;
  const result = features.map(group => {
    const patched = {};
    let groupChanged = false;
    for (const key of Object.keys(group)) {
      patched[key] = patchPrograms(group[key], index, threshold);
      groupChanged = groupChanged || patched[key] !== group[key];
    }
    changed = changed || groupChanged;
    return groupChanged ? patched : group;
  });
  return changed ? result : features;
}

module.exports = { create, reservationIndex, patchTable, patchFeatures };
