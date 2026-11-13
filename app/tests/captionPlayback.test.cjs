/* eslint-env es2020 */
const assert = require('assert');
const fs = require('fs');
const path = require('path');

async function main() {
  const source = fs.readFileSync(path.join(__dirname, '../src/captionPlayback.js'), 'utf8');
  const { default: CaptionPlayback } = await import('data:text/javascript;base64,' + Buffer.from(source).toString('base64'));
  function setup() {
    let now = 0;
    let id = 0;
    const timers = new Map();
    const output = [];
    const player = new CaptionPlayback(packet => output.push(packet), {
      now: () => now,
      setTimer: (callback, delay) => {
        timers.set(++id, { callback, at: now + delay });
        return id;
      },
      clearTimer: timer => timers.delete(timer),
    });
    const tick = delta => {
      now += delta;
      for (const [timer, entry] of [...timers].sort((a, b) => a[1].at - b[1].at)) {
        if (entry.at <= now && timers.has(timer)) {
          timers.delete(timer);
          entry.callback();
        }
      }
    };
    return { player, output, tick, timers };
  }

  // A burst of packets is displayed at PTS intervals; Clear precedes its text.
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 90000, Clear: true });
    player.push({ Pts: 90000, Text: 'first' });
    player.push({ Pts: 135000, Text: 'second' });
    tick(999); assert.equal(output.length, 0);
    tick(1); assert.deepStrictEqual(output.map(p => p.Text || 'clear'), ['clear', 'first']);
    tick(499); assert.equal(output.length, 2);
    tick(1); assert.equal(output[2].Text, 'second');
  }
  // Receiving the next packet later must not add a fresh one-second delay.
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 0 });
    tick(800);
    player.push({ Pts: 90000 });
    tick(200); assert.equal(output.length, 1);
    tick(999); assert.equal(output.length, 1);
    tick(1); assert.equal(output.length, 2);
  }
  // The 33-bit wrap is continuous.
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 2 ** 33 - 45000 });
    player.push({ Pts: 45000 });
    tick(1000); assert.equal(output.length, 1);
    tick(1000); assert.equal(output.length, 2);
  }
  // A discontinuity cancels stale queued captions and starts a new timeline.
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 9000000, Text: 'old' });
    player.push({ Pts: 0, Text: 'new' });
    tick(1000); assert.deepStrictEqual(output.map(p => p.Text), ['new']);
    player.push({ Pts: 9000000, Text: 'jump' });
    tick(1000); assert.equal(output[1].Text, 'jump');
  }
  // Missing PTS still displays; disposal prevents callbacks after channel changes.
  {
    const { player, output, tick, timers } = setup();
    player.push({ Text: 'fallback' });
    tick(1000); assert.equal(output[0].Text, 'fallback');
    player.push({ Pts: -1, Text: 'cancel' });
    player.dispose();
    assert.equal(timers.size, 0);
    tick(30000); assert.equal(output.length, 1);
  }
  // A later clear may carry the same PTS as the text (observed in caption.db).
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 90000, Clear: true });
    player.push({ Pts: 90000, Text: 'readable' });
    tick(1000);
    tick(200);
    player.push({ Pts: 90000, Clear: true });
    tick(1000);
    assert.equal(output.length, 2);
    tick(299); assert.equal(output.length, 2);
    tick(1); assert.equal(output.length, 3);
    assert(output[2].Clear);
  }
  // Clear/text/clear in a burst must leave the text readable, even with equal PTS.
  {
    const { player, output, tick } = setup();
    player.push({ Pts: 0, Clear: true });
    player.push({ Pts: 0, Text: 'first' });
    player.push({ Pts: 0, Clear: true });
    tick(1000);
    assert.equal(output.length, 2);
    player.push({ Pts: 90000, Text: 'replacement' });
    tick(1000);
    assert.equal(output.length, 3);
    assert.deepStrictEqual(output[2], { Pts: 90000, Text: 'replacement', Clear: true });
    tick(1000); assert.equal(output.length, 3);
    player.push({ Pts: 180000, Clear: true });
    tick(1000); assert.equal(output.length, 4);
  }
  // Disposing also cancels a clear postponed for minimum reading time.
  {
    const { player, output, tick, timers } = setup();
    player.push({ Pts: 0, Text: 'keep' });
    player.push({ Pts: 0, Clear: true });
    tick(1000);
    player.dispose();
    assert.equal(timers.size, 0);
    tick(5000); assert.equal(output.length, 1);
  }
  // Window timers reject a foreign receiver (unlike the injected test timers).
  {
    const originalSetTimeout = globalThis.setTimeout;
    const originalClearTimeout = globalThis.clearTimeout;
    const pending = new Map();
    let id = 0;
    const output = [];
    try {
      globalThis.setTimeout = function(callback) {
        if (this !== globalThis) throw new TypeError('Illegal invocation');
        pending.set(++id, callback);
        return id;
      };
      globalThis.clearTimeout = function(timer) {
        if (this !== globalThis) throw new TypeError('Illegal invocation');
        pending.delete(timer);
      };
      const player = new CaptionPlayback(packet => output.push(packet));
      player.push({ Pts: 0, Text: 'visible' });
      pending.get(1)();
      assert.equal(output[0].Text, 'visible');
      player.push({ Pts: 90000, Text: 'cancel' });
      player.dispose();
      assert(!pending.has(2));
    } finally {
      globalThis.setTimeout = originalSetTimeout;
      globalThis.clearTimeout = originalClearTimeout;
    }
  }
  // An actual TS pattern: text and TIME 4.2 seconds + clear share one PES PTS.
  {
    const { player, output, tick } = setup();
    player.push({ Timing: 1, Pts: 90000, Clear: true, DelayMs: 0 });
    player.push({ Timing: 1, Pts: 90000, Text: 'body', DelayMs: 0 });
    player.push({ Timing: 1, Pts: 90000, Text: 'ruby', DelayMs: 0 });
    player.push({ Timing: 1, Pts: 90000, Clear: true, DelayMs: 4200 });
    tick(1000); assert.deepStrictEqual(output.map(p => p.Text || 'clear'), ['clear', 'body', 'ruby']);
    tick(4199); assert.equal(output.length, 3);
    tick(1); assert.equal(output.length, 4); assert(output[3].Clear);
  }
  // A future old clear neither delays the next statement nor deletes its text.
  {
    const { player, output, tick } = setup();
    player.push({ Timing: 1, Pts: 0, Text: 'old' });
    player.push({ Timing: 1, Pts: 0, Clear: true, DelayMs: 20000 });
    player.push({ Timing: 1, Pts: 90000, Clear: true });
    player.push({ Timing: 1, Pts: 90000, Text: 'new' });
    tick(1000); assert.equal(output[0].Text, 'old');
    tick(1000); assert.equal(output[2].Text, 'new');
    tick(20000); assert.equal(output.length, 3);
  }
  // Explicit timing also preserves intentional short durations and 33-bit wrap.
  {
    const { player, output, tick, timers } = setup();
    player.push({ Timing: 1, Pts: 2 ** 33 - 45000, Text: 'short' });
    player.push({ Timing: 1, Pts: 2 ** 33 - 45000, Clear: true, DelayMs: 200 });
    player.push({ Timing: 1, Pts: 45000, Text: 'wrapped' });
    tick(1000); assert.equal(output.length, 1);
    tick(200); assert(output[1].Clear);
    tick(800); assert.equal(output[2].Text, 'wrapped');
    player.push({ Timing: 1, Pts: 45000, Clear: true, DelayMs: 6000 });
    player.dispose(); assert.equal(timers.size, 0);
  }
  // Slow delivery crosses the old clock-reset threshold in the middle of a PES.
  // Its leading clear and body must not be cancelled when punctuation arrives.
  {
    const { player, output, tick } = setup();
    player.push({ Timing: 1, Pts: 0, Text: 'anchor' });
    tick(1000);
    tick(2999);
    player.push({ Timing: 1, Pts: 90000, Clear: true });
    player.push({ Timing: 1, Pts: 90000, Text: 'body' });
    tick(2);
    player.push({ Timing: 1, Pts: 90000, Text: '、' });
    tick(1000);
    assert.deepStrictEqual(output.map(p => p.Text || 'clear'), ['anchor', 'clear', 'body', '、']);
  }
  // The synthetic sample splits its final comma into a half-width fragment.
  // All fragments with one PTS share a timer even with fractional arrival times.
  {
    const { player, output, tick, timers } = setup();
    const sample = JSON.parse(fs.readFileSync(path.join(__dirname, 'captionSample.json'), 'utf8'));
    sample.forEach(packet => { player.push(packet); tick(0.125); });
    assert.equal(timers.size, 1);
    tick(1000);
    assert.deepStrictEqual(output, sample);
    assert.equal(output.at(-1).Text, '、');
  }
  // A delayed receive burst must retain the PTS interval, not flash and clear immediately.
  {
    const { player, output, tick } = setup();
    player.push({ Timing: 1, Pts: 0, Text: 'anchor' });
    tick(1000);
    tick(2000);
    player.push({ Timing: 1, Pts: 90000, Text: 'readable' });
    player.push({ Timing: 1, Pts: 135000, Clear: true });
    tick(1000);
    assert.equal(output.at(-1).Text, 'readable');
    tick(500);
    assert(output.at(-1).Clear);
  }
  console.log('Caption PTS playback tests: OK');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
