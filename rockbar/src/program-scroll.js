// Keep both displays inside the same fixed viewport during a program change.
class ProgramScroller {
  constructor(viewport) {
    this.viewport = viewport;
    this.content = document.createElement('div');
    this.content.className = 'program-scroll-content';
    this.content.append(...viewport.childNodes);
    viewport.classList.add('program-scroll');
    viewport.append(this.content);
    this.signature = null;
    this.programStart = null;
    this.animations = [];
    this.motion = window.matchMedia('(prefers-reduced-motion: reduce)');
    this.onMotionChange = () => { if (this.motion.matches) this.finish(); };
    this.motion.addEventListener('change', this.onMotionChange);
    this.resize = new ResizeObserver(() => this.finish());
    this.resize.observe(viewport);
  }
  finish() {
    for (const animation of this.animations) animation.cancel();
    this.animations = [];
    this.outgoing?.remove();
    this.outgoing = null;
    this.viewport.scrollTop = 0;
    this.viewport.scrollLeft = 0;
  }
  update(timing, fields) {
    // PF details and corrected end times can arrive in multiple updates.
    // Render those in place; scroll only when a known program start changes.
    const signature = JSON.stringify([timing, fields.map(([, text]) => text)]);
    if (signature === this.signature) return;
    const start = Number.isFinite(timing?.[0]) ? timing[0] : null;
    const changedProgram = this.programStart !== null && start !== null && this.programStart !== start;
    this.programStart = start;
    this.signature = signature;
    this.finish();
    const outgoing = this.content.cloneNode(true);
    outgoing.classList.add('program-scroll-outgoing');
    outgoing.setAttribute('aria-hidden', 'true');
    outgoing.inert = true;
    for (const element of outgoing.querySelectorAll('[id], [tabindex]')) {
      element.removeAttribute('id');
      element.removeAttribute('tabindex');
    }
    for (const [element, text] of fields) element.textContent = text;
    const height = this.viewport.clientHeight;
    if (!changedProgram || !height || this.motion.matches) return;
    this.outgoing = outgoing;
    this.viewport.append(outgoing);
    const options = { duration: 360, easing: 'cubic-bezier(.22,.61,.36,1)' };
    this.animations = [
      this.content.animate([{ top: `${height}px` }, { top: '0px' }], options),
      outgoing.animate([{ top: '0px' }, { top: `-${height}px` }], options),
    ];
    const incoming = this.animations[0];
    incoming.onfinish = () => { if (this.animations[0] === incoming) this.finish(); };
  }
  destroy() {
    this.finish();
    this.resize.disconnect();
    this.motion.removeEventListener('change', this.onMotionChange);
  }
  setActive(active) {
    this.reset();
    this.resize.disconnect();
    if (active) this.resize.observe(this.viewport);
  }
  reset() {
    this.finish();
    this.signature = null;
    this.programStart = null;
  }
}
