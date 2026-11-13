(function (root) {
  class ServerClock {
    constructor(tick = () => performance.now()) { this.tick = tick; this.reset(); }
    reset() { this.server = null; this.anchor = 0; }
    sync(milliseconds) {
      if (!Number.isFinite(milliseconds) || milliseconds <= 0) return false;
      this.server = milliseconds; this.anchor = this.tick(); return true;
    }
    now() { return this.server === null ? null : this.server + this.tick() - this.anchor; }
  }
  // Advance locally between real playback reports, including across normal
  // device updates which merely republish the cached Playback snapshot.
  class PlaybackClock {
    constructor(tick = () => performance.now()) { this.tick = tick; this.reset(); }
    reset() { this.sample = ''; this.position = 0; this.duration = 0; this.anchor = 0; this.playing = false; }
    sync(state) {
      const sample = JSON.stringify([state.session, state.generation, state.revision, state.positionMs, state.durationMs, state.phase, state.waitingForData]);
      if (sample === this.sample) return;
      this.sample = sample;
      this.duration = Math.max(0, Number(state.durationMs) || 0);
      this.position = Math.max(0, Math.min(this.duration, Number(state.positionMs) || 0));
      this.anchor = this.tick();
      this.playing = state.phase === 'playing' && !state.waitingForData;
    }
    now() { return Math.min(this.duration, this.position + (this.playing ? Math.max(0, this.tick() - this.anchor) : 0)); }
    freeze() { this.position = this.now(); this.playing = false; }
  }
  if (typeof module !== 'undefined') module.exports = { ServerClock, PlaybackClock };
  else root.rockbarTime = { ServerClock, PlaybackClock };
})(globalThis);
