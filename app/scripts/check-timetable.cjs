// Run with: node scripts/check-timetable.cjs
const assert = require('assert');
const fs = require('fs');
const vm = require('vm');
const source = fs.readFileSync(require('path').join(__dirname, '../src/components/TimeTable.vue'), 'utf8');
const script = source.split('<script>')[1].split('</script>')[0].replace(/^\s*import .*;$/gm, '').replace('export default', 'const component =');
const context = { label: { WEEK: 7, DAY: 1440 }, utility: {
  now: () => new Date(0), zpad: n => String(n).padStart(2, '0'),
  zpadTM: d => d.toISOString().slice(11, 16), replaceTextWithLinks: s => s,
  htmlEscape: s => s, zpadDT: d => d.toISOString().slice(0, 10),
}, CaptionHistoryWindow: {}, setTimeout, clearTimeout };
vm.createContext(context);
vm.runInContext(script + '\nthis.test = { initV, renderV, renderP, renderS, component };', context);
const { initV, renderV, renderP, renderS, component } = context.test;
const program = (start, duration, eid) => ({ StartM: start, EndM: start + duration,
  StartD: new Date(start * 60000), EndD: new Date((start + duration) * 60000),
  Start: start * 60, End: (start + duration) * 60, Sid: 1, Eid: eid,
  Title: 'title', Subtitle: 'subtitle', Content: 'full description', Cnib_a: 0, Cnib_b: 0 });
const opt = { divider: 2, guidetype: 2, subtitlemin: 30, contentmin: 30 };
const short = program(0, 5, 1), long = program(5, 120, 2);
const dt = { ServiceList: [1, 2], Tvp: { 1: [short, long] }, Sta: { 1: { Name: 'One' }, 2: { Name: 'Two' } } };
let matrix = renderV([1, 2], dt, initV(120, 2), 0, 120);
assert.equal(matrix[0][0], short);
assert.equal(matrix[119][0], long);
assert.equal(matrix[0][1], 0);
const overlap = renderV([1], { ServiceList: [1], Tvp: { 1: [program(-10, 20, 4), short] } }, initV(20, 1), 0, 20);
assert.equal(overlap[0][0].Eid, 4);
assert.equal(overlap[10][0], 0);
for (const guidetype of [0, 1, 2, 3]) {
  const html = renderP(matrix, new Date(0), new Date(7200000), 120, 2, { ...opt, guidetype }, {});
  assert(!/<(?:tr|td|table)\b/.test(html));
  assert(html.includes('height:480px'));
  assert(html.includes('grid-row:1 / span 5'));
  assert(html.includes('<span class="tt-short-time">00:00</span><span class="tt-title-text">title</span>'));
  assert(html.includes('grid-row:6 / span 115'));
  assert(html.includes('full description'));
  assert(html.includes('onmouseenter=') === (guidetype === 1 || guidetype === 3));
  assert(html.includes('https://google.com/search?q=title'));
  assert(html.includes('録画予約'));
}
assert(renderS([1, 2], dt, opt, -1).includes('24px minmax(0, 1fr) minmax(0, 1fr) 24px'));
const weekly = { ...dt, Week: true, Tvp: { 0: [short], 1: [program(1440, 15, 3)] } };
matrix = renderV([1], weekly, initV(60, 7), 0, 60);
assert.equal(matrix[0][1].Eid, 3);
assert.equal(matrix[15][1], 0);
const popup = { offsetHeight: 250 };
context.window = { innerWidth: 400, innerHeight: 300 };
let cellRect = { left: 380, right: 400, top: 290, bottom: 350 };
let tableRect = { left: 0, right: 400 };
context.document = { getElementById: id => id.startsWith('pcp') ? { innerHTML: 'details' } :
  { dataset: { captionKey: '1_1_0' }, getBoundingClientRect: () => cellRect,
    closest: () => ({ getBoundingClientRect: () => tableRect }) } };
const instance = { options: { guidetype: 1 }, popupHtml: '', popupStyle: {}, popupKey: '', $refs: { programPopup: popup }, $nextTick: fn => fn() };
for (const [name, fn] of Object.entries(component.methods)) instance[name] = fn.bind(instance);
instance.pgClick(1, 1, 1);
assert.equal(instance.popupStyle.left, '20px');
assert.equal(instance.popupStyle.top, '42px');
instance.pgClick(1, 1, 0);
assert.equal(instance.popupHtml, '');
context.window.innerHeight = 1000;
instance.pgClick(1, 1, 1, { clientX: 390, clientY: 600 });
assert.equal(instance.popupStyle.top, '588px');
instance.closeProgramPopup();
context.window.innerWidth = 1200;
tableRect = { left: 100, right: 1100 };
cellRect = { left: 200, right: 400, top: 100, bottom: 500 };
instance.pgClick(1, 1, 1, { clientX: 300, clientY: 150 });
assert.equal(instance.popupStyle.left, '400px');
cellRect = { left: 800, right: 1000, top: 100, bottom: 500 };
instance.pgClick(1, 1, 1, { clientX: 900, clientY: 150 });
assert.equal(instance.popupStyle.left, '440px');
// A narrow viewport uses free space below without covering the program.
context.window.innerWidth = 300;
cellRect = { left: 8, right: 292, top: 100, bottom: 200 };
instance.pgClick(1, 1, 1, { clientX: 150, clientY: 150 });
assert.equal(instance.popupStyle.top, '208px');
instance.closeProgramPopup();
console.log('Timetable checks passed: grid ranges, gaps, weekly offsets, guide modes, viewport clamping.');

// Expansion remains in the program column, survives scrolling, and restores on close.
const classes = new Set();
let expandedCellTop = 100;
let stickyBodyTop = 101;
const expandedCell = {
  style: { setProperty(key, value) { this[key] = value; }, removeProperty(key) { delete this[key]; } },
  classList: { add: name => classes.add(name), remove: name => classes.delete(name) },
  getBoundingClientRect: () => ({ top: expandedCellTop, height: 20 }),
  querySelector: () => ({ getBoundingClientRect: () => ({ top: stickyBodyTop }) }),
};
context.window.getComputedStyle = () => ({
  backgroundColor: 'rgba(212, 255, 200, 0.5)', borderTopWidth: '0px', paddingTop: '1px',
});
context.document.getElementById = id => id.startsWith('pcp') ? { innerHTML: 'details' } : expandedCell;
for (const guidetype of [2, 3]) {
  instance.options.guidetype = guidetype;
  instance.pgClick(1, 1, 1);
  assert(classes.has('tt-program-expanded'));
  assert.equal(expandedCell.style.minHeight, '20px');
  assert.equal(expandedCell.style['--tt-expanded-offset'], '0px');
  assert.equal(instance.popupHtml, '');
  instance.onPopupScroll({ target: {} });
  assert(classes.has('tt-program-expanded'));
  instance.pgClick(1, 1, 0);
  assert(!classes.has('tt-program-expanded'));
  assert.equal(instance.expandedProgramCell, null);
  // The program start has scrolled out of view, but its sticky title is still visible.
  // Cover both viewport scrolling and an inner scroll container below the toolbar.
  for (const visibleTop of [0, 80]) {
    expandedCellTop = -300;
    stickyBodyTop = visibleTop;
    instance.pgClick(1, 1, 1);
    const offset = parseFloat(expandedCell.style['--tt-expanded-offset']);
    assert.equal(expandedCellTop + 1 + offset, visibleTop);
    instance.closeProgramPopup();
    assert.equal(expandedCell.style['--tt-expanded-offset'], undefined);
  }
  expandedCellTop = 100;
  stickyBodyTop = 101;
}
console.log('Timetable expansion checks passed: click/hover modes, scroll, close.');

expandedCell.dataset = { captionKey: '1_1_0' };
instance.programs = { '1_1_0': short };
instance.options = { guidetype: 2, captiontimetable: false };
const clickEvent = { type: 'click', target: {
  closest: selector => selector === '[data-caption-key]' ? expandedCell : null,
} };
instance.onProgramClick(clickEvent);
assert(classes.has('tt-program-expanded'));
// The click-opened popup stays open when the pointer leaves the program.
instance.pgClick(1, 1, 2);
assert(classes.has('tt-program-expanded'));
instance.onProgramClick({ target: { closest: () => ({}) } });
assert(classes.has('tt-program-expanded')); // Links and reservation controls do not toggle it.
instance.onProgramClick(clickEvent);
assert(!classes.has('tt-program-expanded'));
instance.options.guidetype = 3;
instance.pgClick(1, 1, 1);
let pendingClose;
context.setTimeout = fn => { pendingClose = fn; return 1; };
instance.pgClick(1, 1, 2);
assert(pendingClose);
pendingClose();
assert(!classes.has('tt-program-expanded'));
console.log('Timetable interaction checks passed: click toggle, controls, hover leave.');

// Hiding the main tab must not replace the last valid widths with 0px.
const layout = { options: { stationsplitscroll: true }, $refs: {
  programScrollBody: { clientWidth: 640 },
  stationHeaderWrap: { style: {} }, stationHeaderTable: { style: {} },
  programTable: { style: {} },
} };
const syncLayout = () => component.methods.syncStationHeaderLayout.call(layout);
const widths = () => ['stationHeaderWrap', 'stationHeaderTable', 'programTable']
  .map(key => layout.$refs[key].style.width);
syncLayout();
assert.deepEqual(widths(), ['640px', '640px', '640px']);
layout.$refs.programScrollBody.clientWidth = 0;
syncLayout();
assert.deepEqual(widths(), ['640px', '640px', '640px']);
layout.$refs.programScrollBody.clientWidth = 360;
syncLayout();
assert.deepEqual(widths(), ['360px', '360px', '360px']);
layout.options.stationsplitscroll = false;
layout.$refs.programScrollBody.clientWidth = 800;
syncLayout();
assert.deepEqual(widths(), ['360px', '360px', '360px']);
console.log('Timetable layout checks passed: hidden tab, restored width, unsplit mode.');
