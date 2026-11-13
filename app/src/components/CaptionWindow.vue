<template>
  <div class="caption-window" role="log" aria-label="字幕" aria-live="polite">
    <svg width="100%" height="100%" :viewBox="viewBox" preserveAspectRatio="xMinYMin meet">
      <g shape-rendering="crispEdges">
        <rect v-for="(background, index) in backgrounds" :key="'background-' + index"
          :x="background.x" :y="background.y"
          :width="background.width" :height="background.height"
          :fill="background.fill" />
      </g>
      <g v-for="(fragment, index) in fragments" :key="'text-' + index">
        <g v-for="(line, lineIndex) in fragment.lines" :key="lineIndex">
          <text v-for="(character, column) in Array.from(line)" :key="column"
            :x="fragment.x + column * fragment.width + (fragment.width - fragment.glyphWidth) / 2"
            :y="fragment.y + lineIndex * fragment.lineHeight + (fragment.lineHeight - fragment.height) / 2"
            dominant-baseline="text-before-edge"
            :font-size="fragment.height"
            :textLength="fragment.glyphWidth"
            lengthAdjust="spacingAndGlyphs"
            :fill="captionColor(fragment.caption.Color, '#ffffff')"
          >{{ character }}</text>
        </g>
      </g>
    </svg>
  </div>
</template>

<script>
// ARIB STD-B24 default CLUT: preserve the palette and transparency bits.
// Reference: https://github.com/xqq/libaribcaption/blob/master/src/decoder/b24_colors.cpp
const colors = [
  [0, 0, 0], [255, 0, 0], [0, 255, 0], [255, 255, 0],
  [0, 0, 255], [255, 0, 255], [0, 255, 255], [255, 255, 255],
  [0, 0, 0], [170, 0, 0], [0, 170, 0], [170, 170, 0],
  [0, 0, 170], [170, 0, 170], [0, 170, 170], [170, 170, 170],
];
for (const red of [0, 85, 170, 255]) {
  for (const green of [0, 85, 170, 255]) {
    for (const blue of [0, 85, 170, 255]) {
      if (!colors.some(color => color[0] === red && color[1] === green && color[2] === blue)) {
        colors.push([red, green, blue]);
      }
    }
  }
}
const palette = colors.map((color, index) => `rgba(${color.join(',')},${index === 8 ? 0 : 1})`);
for (let index = 65; index < 128; index++) {
  const color = colors[index <= 72 ? index - 65 : index - 64];
  palette.push(`rgba(${color.join(',')},${128 / 255})`);
}
const sizes = {
  small: [18, 18], medium: [18, 36], normal: [36, 36], micro: [9, 9],
  high_w: [36, 72], width_w: [72, 36], w: [72, 72],
  // Special sizes have no dimensions in the packet; use the normal size.
  special_1: [36, 36], special_2: [36, 36],
};

export default {
  name: 'CaptionWindow',
  props: { items: { type: Array, default: () => [] } },
  computed: {
    fragments() {
      let cursorX = 0;
      let cursorY = 0;
      return this.items.map(caption => {
        const [sourceWidth, sourceHeight] = sizes[caption.Size] || sizes.normal;
        const layout = caption.Layout;
        const positioned = layout && ['X', 'Y', 'CellWidth', 'CellHeight', 'GlyphWidth', 'GlyphHeight']
          .every(key => Number.isFinite(layout[key])) && layout.CellWidth > 0 && layout.CellHeight > 0;
        // Preserve ruby dimensions and broadcast coordinates; never scale small text up to body size.
        const height = positioned ? layout.GlyphHeight : sourceHeight;
        const width = positioned ? layout.CellWidth : sourceWidth;
        const glyphWidth = positioned ? layout.GlyphWidth : sourceWidth;
        const lineHeight = positioned ? layout.CellHeight : height * 4 / 3;
        const x = positioned ? layout.X : (caption.X >= 0 ? caption.X * width : cursorX);
        const y = positioned ? layout.Y : (caption.Y >= 0 ? caption.Y * lineHeight : cursorY);
        const lines = String(caption.Text).split(/\r\n|\r|\n/);
        cursorX = x + Array.from(lines[lines.length - 1]).length * width;
        cursorY = y + (lines.length - 1) * lineHeight;
        return { caption, width, glyphWidth, height, lineHeight, x, y, lines };
      });
    },
    backgrounds() {
      const backgrounds = [];
      for (const fragment of this.fragments) {
        const fill = this.captionColor(fragment.caption.BgColor, '#000000');
        fragment.lines.forEach((line, index) => {
          const width = Array.from(line).length * fragment.width;
          if (!width) return;
          const y = fragment.y + index * fragment.lineHeight;
          const last = backgrounds[backgrounds.length - 1];
          // A single rectangle avoids antialiasing seams without overlapping
          // translucent backgrounds and making the joins darker.
          if (last && last.fill === fill && last.y === y && last.height === fragment.lineHeight &&
              Math.abs(last.x + last.width - fragment.x) < 0.000001) {
            last.width = fragment.x + width - last.x;
          } else {
            backgrounds.push({ x: fragment.x, y, width, height: fragment.lineHeight, fill });
          }
        });
      }
      return backgrounds;
    },
    viewBox() {
      // Fit the subtitle area, not the unused space from the broadcast origin.
      const left = this.fragments.length ? Math.min(...this.fragments.map(item => item.x)) : 0;
      const top = this.fragments.length ? Math.min(...this.fragments.map(item => item.y)) : 0;
      // Keep a short line and a full-width line at the same scale.
      const right = Math.max(left + 960, ...this.fragments.map(item =>
        item.x + Math.max(...item.lines.map(line => Array.from(line).length)) * item.width));
      const bottom = Math.max(top + 180, ...this.fragments.map(item =>
        item.y + item.lines.length * item.lineHeight));
      return `${left} ${top} ${right - left} ${bottom - top}`;
    },
  },
  methods: {
    captionColor(value, fallback) {
      return palette[value] || fallback;
    },
  },
};
</script>

<style scoped>
.caption-window {
  height: 120px;
  overflow: hidden;
}
.caption-window svg {
  display: block;
  font-family: "Roboto", "Noto Sans JP", "Noto Sans Symbols 2", "Noto Sans Symbols", sans-serif;
  white-space: pre;
}
</style>
