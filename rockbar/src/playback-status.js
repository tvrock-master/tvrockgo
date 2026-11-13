(function (root) {
  const labels = {
    received: '再生要求を受信', indexing: 'ファイル解析中',
    'waiting-launch': '視聴アプリ起動待ち', 'waiting-receiver': '視聴アプリ接続待ち',
    'waiting-srt': 'SRT接続待ち', preparing: '再生準備中', playing: '再生中',
    idle: '受信待受中', paused: '一時停止中', ended: '再生終了', failed: '開始・再生失敗',
  };
  class PlaybackStatus {
    constructor(tick = () => performance.now(), duration = 10000) { this.tick = tick; this.duration = duration; this.reset(); }
    reset() { this.sessions = new Map(); this.notification = null; }
    show(text) {
      text = String(text || '').slice(0, 600);
      if (!text) return;
      if (this.notification?.text === text && this.tick() < this.notification.until) return;
      this.notification = {text, until: this.tick() + this.duration};
    }
    message() { return this.notification && this.tick() < this.notification.until ? this.notification.text : ''; }
    remaining() { return this.notification ? Math.max(0, this.notification.until - this.tick()) : 0; }
    receive(message) {
      if (!message || typeof message !== 'object') return;
      const action = message.action;
      if (!['request', 'offer', 'launch', 'status', 'close'].includes(action) && !message.error) return;
      const key = message.session || (message.error ? 'request-error' : '');
      if (!key) return;
      if (action === 'request') {
        for (const [id, entry] of this.sessions) if (['ended', 'failed'].includes(entry.phase)) this.sessions.delete(id);
      }
      const previous = this.sessions.get(key);
      if (Number.isInteger(message.generation) && message.generation < (previous?.generation || 0)) return;
      const newerGeneration = Number.isInteger(message.generation) && message.generation > (previous?.generation || 0);
      const entry = {...previous, received: !!message.session || previous?.received, device: message.deviceId || previous?.device || 'Playback'};
      entry.generation = message.generation ?? previous?.generation ?? 0;
      let phase = message.error ? 'failed' : action === 'request' ? 'indexing' : action === 'offer' ? 'received' : action === 'launch' ? 'waiting-receiver' : action === 'close' ? 'ended' : message.phase;
      if (!labels[phase]) return;
      // Local validation happens before the raw offer/launch reaches the renderer.
      // A later copy of that event must not hide the local failure or waiting stage.
      if (previous?.phase === 'failed' && phase !== 'failed' && !newerGeneration) return;
      if (action === 'offer' && previous && !['received', 'indexing'].includes(previous.phase)) return;
      if (action === 'launch' && previous && !['received', 'indexing', 'waiting-launch'].includes(previous.phase)) return;
      entry.phase = phase;
      entry.error = message.error ? String(message.error).slice(0, 300) : '';
      this.sessions.set(key, entry);
      if (!previous || previous.phase !== entry.phase || previous.error !== entry.error || previous.generation !== entry.generation) {
        this.show(entry.error || `${entry.device} / 再生要求受信済み / ${labels[entry.phase]}`);
      }
      while (this.sessions.size > 8) {
        const completed = [...this.sessions].find(([, entry]) => ['ended', 'failed'].includes(entry.phase));
        this.sessions.delete(completed ? completed[0] : this.sessions.keys().next().value);
      }
    }
    summary(connected) {
      if (!connected) return 'Playback: 未接続';
      if (!this.sessions.size) return 'Playback: 再生要求待ち';
      return 'Playback: ' + [...this.sessions.values()].map(entry =>
        `${entry.device} / ${entry.received ? '再生要求受信済み' : '再生要求エラー'} / ${labels[entry.phase]}${entry.error ? '（' + entry.error + '）' : ''}`
      ).join(' · ');
    }
  }
  if (typeof module !== 'undefined') module.exports = {PlaybackStatus};
  else root.rockbarPlaybackStatus = {PlaybackStatus};
})(globalThis);
