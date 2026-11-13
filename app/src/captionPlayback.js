/* eslint-env es2020 */
// Approximate playback without access to the external player's video clock.
export const CAPTION_DELAY_MS = 1000;
export const CAPTION_MIN_VISIBLE_MS = 1500;
const PTS_WRAP = 2 ** 33;

export default class CaptionPlayback {
  constructor(display, {
    now = () => performance.now(),
    // Do not call Window timer functions with CaptionPlayback as their receiver.
    setTimer = (callback, delay) => globalThis.setTimeout(callback, delay),
    clearTimer = timer => globalThis.clearTimeout(timer),
    delay = CAPTION_DELAY_MS,
  } = {}) {
    Object.assign(this, { display, now, setTimer, clearTimer, delay });
    this.pending = new Set();
    this.anchor = null;
    this.lastDue = 0;
    this.visibleUntil = 0;
    this.deferredClear = null;
    this.statement = 0;
    this.presentedStatement = 0;
    this.statementPts = null;
    this.statementDue = 0;
    this.timedQueue = [];
    this.timedTimer = null;
  }

  push(caption) {
    if (caption.Timing === 1) { this.pushTimed(caption); return; }
    const now = this.now();
    const pts = caption.Pts;
    let due = now + this.delay;
    if (Number.isInteger(pts) && pts >= 0 && pts < PTS_WRAP) {
      if (this.anchor) {
        // PES PTS is a 90 kHz, 33-bit clock; use the nearest wrap interval.
        const delta = ((pts - this.anchor.pts + PTS_WRAP / 2 + PTS_WRAP) % PTS_WRAP) - PTS_WRAP / 2;
        due = this.anchor.time + delta / 90;
        if (due < now - 2000 || due > now + 10000) {
          this.dispose();
          due = now + this.delay;
        }
      }
      if (!this.anchor) this.anchor = { pts, time: due };
    } else {
      // Missing PTS: retain receive order and fall back to the fixed delay.
      this.anchor = null;
    }
    // Some senders reuse the text PTS for a later clear-only packet. Do not
    // collapse its receive-time spacing into an already elapsed PTS deadline.
    due = Math.max(now + this.delay, this.lastDue, due);
    this.lastDue = due;
    const timer = this.setTimer(() => {
      this.pending.delete(timer);
      this.present(caption);
    }, due - now);
    this.pending.add(timer);
  }

  // New senders preserve B24 TIME offsets instead of flattening clear/text into one PTS.
  pushTimed(caption) {
    const now = this.now();
    const pts = caption.Pts;
    const validPts = Number.isInteger(pts) && pts >= 0 && pts < PTS_WRAP;
    if (this.statementPts !== pts || !validPts || this.statement === 0) {
      let baseDue = now + this.delay;
      if (validPts && this.anchor) {
        const delta = ((pts - this.anchor.pts + PTS_WRAP / 2 + PTS_WRAP) % PTS_WRAP) - PTS_WRAP / 2;
        baseDue = this.anchor.time + delta / 90;
        if (baseDue < now - 2000 || baseDue > now + 10000) {
          this.dispose();
          baseDue = now + this.delay;
        } else if (baseDue < now) {
          // Move the clock once for a late statement. Its following clear keeps
          // the original PTS interval instead of sharing an immediate deadline.
          const shift = now + this.delay - baseDue;
          this.anchor.time += shift;
          baseDue += shift;
        }
      }
      if (validPts && !this.anchor) this.anchor = { pts, time: baseDue };
      if (!validPts) this.anchor = null;
      this.statement++;
      this.statementPts = pts;
      this.statementDue = baseDue;
    }
    const statement = this.statement;
    const offset = Number.isInteger(caption.DelayMs) && caption.DelayMs >= 0
      ? Math.min(caption.DelayMs, 3600000) : 0;
    this.timedQueue.push({ caption, statement, due: this.statementDue + offset });
    this.timedQueue.sort((a, b) => a.due - b.due);
    this.scheduleTimed();
  }

  scheduleTimed() {
    if (this.timedTimer !== null) this.clearTimer(this.timedTimer);
    this.timedTimer = null;
    if (!this.timedQueue.length) return;
    this.timedTimer = this.setTimer(() => {
      this.timedTimer = null;
      const now = this.now();
      // One callback keeps clear/body/punctuation/ruby in receive order and
      // lets Vue render the complete update in a single DOM refresh.
      while (this.timedQueue.length && this.timedQueue[0].due <= now) {
        const { caption, statement } = this.timedQueue.shift();
        if (statement < this.presentedStatement) continue;
        this.presentedStatement = statement;
        this.display(caption);
      }
      this.scheduleTimed();
    }, Math.max(0, this.timedQueue[0].due - this.now()));
  }

  present(caption) {
    if (caption.Clear && !caption.Text && this.now() < this.visibleUntil) {
      if (this.deferredClear !== null) this.clearTimer(this.deferredClear);
      this.deferredClear = this.setTimer(() => {
        this.deferredClear = null;
        this.display(caption);
      }, this.visibleUntil - this.now());
      return;
    }
    let packet = caption;
    if (this.deferredClear !== null) {
      this.clearTimer(this.deferredClear);
      this.deferredClear = null;
      // Apply the postponed clear together with the replacement text.
      packet = { ...caption, Clear: true };
    }
    if (caption.Text) this.visibleUntil = this.now() + CAPTION_MIN_VISIBLE_MS;
    this.display(packet);
  }

  dispose() {
    if (this.timedTimer !== null) this.clearTimer(this.timedTimer);
    this.timedTimer = null;
    this.timedQueue = [];
    this.statementDue = 0;
    if (this.deferredClear !== null) this.clearTimer(this.deferredClear);
    this.deferredClear = null;
    this.visibleUntil = 0;
    this.pending.forEach(timer => this.clearTimer(timer));
    this.pending.clear();
    this.anchor = null;
    this.lastDue = 0;
    this.statement = 0;
    this.presentedStatement = 0;
    this.statementPts = null;
  }
}
