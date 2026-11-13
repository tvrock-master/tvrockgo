// Retain outgoing lines until the scroll finishes, then keep only the latest two.
class CaptionScroller {
  constructor(viewport, content) {
    this.viewport = viewport;
    this.content = content;
    this.key = '';
    this.items = [];
    this.frame = 0;
    this.motion = window.matchMedia('(prefers-reduced-motion: reduce)');
    let width = viewport.offsetWidth;
    let height = viewport.offsetHeight;
    this.resize = new ResizeObserver(() => {
      const nextWidth = viewport.offsetWidth;
      const nextHeight = viewport.offsetHeight;
      if (width === nextWidth && height === nextHeight) return;
      width = nextWidth;
      height = nextHeight;
      this.finish();
    });
    this.resize.observe(viewport, { box: 'border-box' });
  }
  row(item) {
    return rockbarCaptions.captionRow(item);
  }
  finish() {
    cancelAnimationFrame(this.frame);
    this.frame = 0;
    this.content.replaceChildren(...this.items.map(item => this.row(item)));
    this.viewport.scrollTop = this.viewport.scrollHeight;
  }
  update(key, items) {
    const sameProgram = this.key === key;
    const previousIDs = new Set(this.items.map(item => String(item.ID)));
    const overlaps = items.some(item => previousIDs.has(String(item.ID)));
    this.key = key;
    this.items = items;
    cancelAnimationFrame(this.frame);
    if (!sameProgram || !overlaps || this.motion.matches) { this.finish(); return; }

    const rows = new Map([...this.content.children].map(row => [row.dataset.captionId, row]));
    for (const item of items) {
      const row = rows.get(String(item.ID));
      if (row) { row.replaceWith(this.row(item)); }
      else this.content.append(this.row(item));
    }
    // A burst of captions must not accumulate an unbounded animation backlog.
    while (this.content.children.length > 6) {
      const first = this.content.firstElementChild;
      const top = this.viewport.scrollTop - first.offsetHeight;
      first.remove(); this.viewport.scrollTop = Math.max(0, top);
    }
    const start = this.viewport.scrollTop;
    const target = Math.max(0, this.viewport.scrollHeight - this.viewport.clientHeight);
    if (target <= start) { this.finish(); return; }
    const started = performance.now();
    const step = now => {
      const progress = Math.min(1, (now - started) / 260);
      this.viewport.scrollTop = start + (target - start) * (1 - Math.pow(1 - progress, 3));
      if (progress < 1) this.frame = requestAnimationFrame(step);
      else this.finish();
    };
    this.frame = requestAnimationFrame(step);
  }
  setActive(active) {
    this.finish();
    this.resize.disconnect();
    if (active) this.resize.observe(this.viewport, { box: 'border-box' });
  }
}
