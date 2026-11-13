const { test } = require('node:test');
const assert = require('node:assert/strict');
const bindLongPress = require('../src/long-press.js');

function setup(t) {
  t.mock.timers.enable({ apis: ['setTimeout'] });
  const button = new EventTarget();
  button.isConnected = true;
  let ready = true, boots = 0, clicks = 0, doubles = 0;
  bindLongPress(button, () => ready, () => boots++);
  button.addEventListener('click', () => clicks++);
  button.addEventListener('dblclick', () => doubles++);
  return {
    button,
    ready(value) { ready = value; },
    counts: () => [boots, clicks, doubles],
    fire(type, overrides = {}) {
      const event = new Event(type, { cancelable: true });
      Object.assign(event, { button: 0, isPrimary: true, pointerId: 1, clientX: 5, clientY: 5 }, overrides);
      button.dispatchEvent(event);
      return event;
    },
  };
}

test('short clicks and double clicks retain their normal actions', t => {
  const ui = setup(t);
  for (let i = 0; i < 2; i++) {
    ui.fire('pointerdown');
    t.mock.timers.tick(100);
    ui.fire('pointerup');
    ui.fire('click');
  }
  ui.fire('dblclick');
  t.mock.timers.tick(1000);
  assert.deepEqual(ui.counts(), [0, 2, 1]);
});

test('a long press boots once and consumes release click and double click', t => {
  const ui = setup(t);
  ui.fire('pointerdown');
  t.mock.timers.tick(599);
  assert.deepEqual(ui.counts(), [0, 0, 0]);
  t.mock.timers.tick(1);
  t.mock.timers.tick(2000);
  ui.fire('pointerup');
  assert.equal(ui.fire('click').defaultPrevented, true);
  ui.fire('dblclick');
  assert.deepEqual(ui.counts(), [1, 0, 0]);
  ui.fire('pointerdown');
  ui.fire('pointerup');
  ui.fire('click');
  assert.deepEqual(ui.counts(), [1, 1, 0]);
});

test('leaving, dragging, canceling or losing focus cancels the hold', t => {
  const ui = setup(t);
  for (const type of ['pointerleave', 'pointercancel', 'lostpointercapture', 'blur', 'pointermove']) {
    ui.fire('pointerdown');
    t.mock.timers.tick(300);
    ui.fire(type, { clientX: 30 });
    t.mock.timers.tick(600);
  }
  assert.deepEqual(ui.counts(), [0, 0, 0]);
});

test('keyboard activation remains available after a consumed long press', t => {
  const ui = setup(t);
  ui.fire('pointerdown');
  t.mock.timers.tick(600);
  ui.fire('pointerup');
  ui.fire('click');
  ui.fire('keydown');
  ui.fire('click');
  assert.deepEqual(ui.counts(), [1, 1, 0]);
});

test('disabled, unavailable, detached and non-primary targets cannot boot', t => {
  const ui = setup(t);
  ui.ready(false);
  ui.fire('pointerdown');
  t.mock.timers.tick(600);
  ui.ready(true);
  ui.button.disabled = true;
  ui.fire('pointerdown');
  t.mock.timers.tick(600);
  ui.button.disabled = false;
  for (const overrides of [{ button: 2 }, { isPrimary: false }]) {
    ui.fire('pointerdown', overrides);
    t.mock.timers.tick(600);
  }
  ui.fire('pointerdown');
  ui.ready(false);
  t.mock.timers.tick(600);
  ui.ready(true);
  ui.fire('pointerdown');
  ui.button.isConnected = false;
  t.mock.timers.tick(600);
  assert.deepEqual(ui.counts(), [0, 0, 0]);
});
