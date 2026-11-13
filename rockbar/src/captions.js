(function (root) {
  const colors = [
    [0, 0, 0], [255, 0, 0], [0, 255, 0], [255, 255, 0],
    [0, 0, 255], [255, 0, 255], [0, 255, 255], [255, 255, 255],
    [0, 0, 0], [170, 0, 0], [0, 170, 0], [170, 170, 0],
    [0, 0, 170], [170, 0, 170], [0, 170, 170], [170, 170, 170],
  ];
  for (const red of [0, 85, 170, 255]) for (const green of [0, 85, 170, 255]) for (const blue of [0, 85, 170, 255]) {
    if (!colors.some(color => color[0] === red && color[1] === green && color[2] === blue)) colors.push([red, green, blue]);
  }
  const names = { black: 0, red: 1, green: 2, yellow: 3, blue: 4, magenta: 5, cyan: 6, white: 7 };
  function captionColor(segment) {
    let index = Number.isInteger(segment.Color) ? segment.Color : names[segment.ColorName];
    if (index >= 65 && index < 128) index = index <= 72 ? index - 65 : index - 64;
    const color = colors[index] || colors[7];
    // Soften broadcast primaries towards the display's pale green phosphor.
    const ink = color.map((value, channel) => value * 0.65 + [210, 237, 224][channel] * 0.35);
    const luminance = ink[0] * 0.2126 + ink[1] * 0.7152 + ink[2] * 0.0722;
    const lift = Math.max(0, 125 - luminance);
    // Dim caption ink slightly while retaining the broadcast color balance.
    return `rgb(${ink.map(value => Math.round(Math.min(255, value + lift) * 0.9)).join(', ')})`;
  }
  function captionRow(item) {
    const row = document.createElement('div');
    row.dataset.captionId = String(item.ID);
    for (const segment of item.Segments?.length ? item.Segments : [item]) {
      const span = document.createElement('span');
      span.className = 'caption-ink';
      span.textContent = segment.Text;
      span.style.color = captionColor(segment);
      row.append(span);
    }
    return row;
  }
  function programKey(device) {
    if (device?.Kind==='playback') return device.Playback ? `playback:${device.Playback.session}:${device.Playback.generation}` : '';
    const p = device?.CurrentProgram;
    return [2, 3].includes(device?.State) && p && Number.isFinite(p.Eid) && p.Start > 0
      ? `${p.Sid || device.CurrentService}:${p.Eid}:${p.Start}` : '';
  }
  class CaptionFeed {
    constructor(request, emit) { this.emit = emit; this.reset(); }
    reset() {
      this.buffers = new Map(); this.key = ''; this.deviceID = '';
      this.publish();
    }
    get items() { return this.buffers.get(this.deviceID)?.items || []; }
    publish() {
      this.emit({ key: this.key, deviceID: this.deviceID, items: this.items.map(item => ({ ...item, Segments: item.Segments?.map(segment => ({ ...segment })) })) });
    }
    syncDevices(devices) {
      const active = new Set();
      for (const device of devices) {
        const key = programKey(device);
        if (!key) continue;
        active.add(device.ID);
        if (this.buffers.get(device.ID)?.key !== key) {
          this.buffers.set(device.ID, { key, items: [], lastID: 0, boundary: false, sequence: 0 });
        }
      }
      for (const id of this.buffers.keys()) if (!active.has(id)) this.buffers.delete(id);
    }
    select(device) {
      const key = programKey(device);
      const deviceID = key ? device.ID : '';
      if (key === this.key && deviceID === this.deviceID) return;
      this.key = key; this.deviceID = deviceID;
      this.publish();
    }
    receive(packet) {
      const buffer = this.buffers.get(packet.DeviceID);
      if (!buffer || `${packet.Sid}:${packet.Eid}:${packet.Start}` !== buffer.key) return;
      this.append(packet, buffer);
    }
    receivePlayback(message) {
      const buffer=this.buffers.get(message.device);
      if (!buffer || buffer.key!==`playback:${message.session}:${message.generation}`) return;
      this.append({...message.caption,DeviceID:message.device,HistoryID:0},buffer);
    }
    append(packet, buffer) {
      if (packet.HistoryID && packet.HistoryID <= buffer.lastID) return;
      if (packet.HistoryID) buffer.lastID = packet.HistoryID;
      buffer.boundary ||= Boolean(packet.Clear);
      // Hidden ruby still advances the cursor and preserves clear boundaries.
      if (packet.Size === 'small' || packet.Size === 'micro' || !String(packet.Text || '').trim()) return;
      const last = buffer.items.at(-1);
      const segment = { Text: packet.Text, Color: packet.Color, ColorName: packet.ColorName };
      if (last && !buffer.boundary && last.Pts === packet.Pts && last.Lang === packet.Lang) {
        last.Segments ||= [{ Text: last.Text, Color: last.Color, ColorName: last.ColorName }];
        last.Segments.push(segment);
        last.Text += packet.Text;
      } else buffer.items.push({ ID: 'live-' + ++buffer.sequence, Text: packet.Text, Pts: packet.Pts, Lang: packet.Lang, Segments: [segment] });
      buffer.items = buffer.items.slice(-50); buffer.boundary = false;
      if (packet.DeviceID === this.deviceID && buffer.key === this.key) this.publish();
    }
  }
  const api = { programKey, CaptionFeed, captionColor, captionRow };
  if (typeof module !== 'undefined') module.exports = api;
  else root.rockbarCaptions = api;
})(globalThis);
