(function (root, factory) {
  if (typeof module !== 'undefined') module.exports = factory();
  else root.bindLongPress = factory();
})(globalThis, function () {
  return function bindLongPress(button, canStart, activate) {
    let timer, pointer, origin, consumed = false;
    const cancel = () => { clearTimeout(timer); pointer = null; };
    button.addEventListener('pointerdown', event => {
      if (event.button !== 0 || !event.isPrimary) return;
      cancel();
      consumed = false;
      if (button.disabled || !canStart()) return;
      pointer = event.pointerId;
      origin = { x: event.clientX, y: event.clientY };
      timer = setTimeout(() => {
        pointer = null;
        // Consume the release click even if the device state changed while held.
        consumed = true;
        if (button.isConnected && !button.disabled && canStart()) activate();
      }, 600);
    });
    button.addEventListener('pointermove', event => {
      if (event.pointerId === pointer && Math.hypot(event.clientX - origin.x, event.clientY - origin.y) > 10) cancel();
    });
    for (const type of ['pointerup', 'pointercancel', 'pointerleave', 'lostpointercapture', 'blur']) {
      button.addEventListener(type, cancel);
    }
    button.addEventListener('keydown', () => { cancel(); consumed = false; });
    for (const type of ['click', 'dblclick', 'contextmenu']) {
      button.addEventListener(type, event => {
        if (!consumed) return;
        event.preventDefault();
        event.stopImmediatePropagation();
      }, true);
    }
  };
});
