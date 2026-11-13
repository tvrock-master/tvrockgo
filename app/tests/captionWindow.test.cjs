const assert = require('assert');
const fs = require('fs');
const path = require('path');

async function main() {
  const source = fs.readFileSync(path.join(__dirname, '../src/components/CaptionWindow.vue'), 'utf8');
  const script = source.match(/<script>([\s\S]*?)<\/script>/)[1];
  const { default: component } = await import('data:text/javascript;base64,' + Buffer.from(script).toString('base64'));
  const items = [
    { Text: '本文', Size: 'normal', BgColor: 65, Layout: { X: 170, Y: 449, CellWidth: 40, CellHeight: 60, GlyphWidth: 36, GlyphHeight: 36, Cells: 2 } },
    { Text: 'ほんぶん', Size: 'small', BgColor: 8, Layout: { X: 170, Y: 429, CellWidth: 20, CellHeight: 30, GlyphWidth: 18, GlyphHeight: 18, Cells: 4 } },
  ];
  const fragments = component.computed.fragments.call({ items });
  assert.equal(fragments[0].height, 36);
  assert.equal(fragments[1].height, 18);
  assert.equal(fragments[0].x, fragments[1].x);
  assert(fragments[1].y < fragments[0].y);
  assert.equal(component.methods.captionColor(65, ''), 'rgba(0,0,0,0.5019607843137255)');
  assert.equal(component.methods.captionColor(8, ''), 'rgba(0,0,0,0)');
  const legacy = component.computed.fragments.call({ items: [{ Text: 'old', Size: 'small', X: 2, Y: 1 }] });
  assert.equal(legacy[0].height, 18);
  assert.equal(legacy[0].x, 36);
  assert(component.computed.viewBox.call({ fragments }).startsWith('170 429 '));
  const short = component.computed.fragments.call({ items: [items[0]] });
  const long = component.computed.fragments.call({ items: [{ ...items[0], Text: '長期金利の代表的な指標である、' }] });
  const box = fragments => component.computed.viewBox.call({ fragments }).split(' ').slice(2);
  assert.deepStrictEqual(box(short), box(long));
  assert(source.indexOf("'background-' + index") < source.indexOf("'text-' + index"));
  const backgroundRects = items => component.computed.backgrounds.call({
    fragments: component.computed.fragments.call({ items }),
    captionColor: component.methods.captionColor,
  });
  const body = items[0];
  const next = { ...body, Text: '続き', Color: 1, Layout: { ...body.Layout, X: 250 } };
  const joined = backgroundRects([body, next]);
  assert.equal(joined.length, 1, 'adjacent backgrounds join even when text colors differ');
  assert.equal(joined[0].width, 160);
  assert.equal(joined[0].fill, component.methods.captionColor(65, ''));
  assert.equal(backgroundRects([body, { ...next, BgColor: 1 }]).length, 2);
  assert.equal(backgroundRects([body, { ...next, Layout: { ...next.Layout, X: 251 } }]).length, 2, 'intentional gaps remain');
  assert.equal(backgroundRects([body, { ...next, Layout: { ...next.Layout, Y: 509 } }]).length, 2);
  assert.equal(backgroundRects(items).length, 2, 'ruby backgrounds retain separate positions');
  console.log('Caption layout / ruby / transparency / background seams tests: OK');
}
main().catch(error => { console.error(error); process.exitCode = 1; });
