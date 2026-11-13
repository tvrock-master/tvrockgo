<template>
  <div class="caption-history-host">
  <v-card ref="window" v-show="displayItems.length > 0 || error" class="caption-history" :style="position" role="dialog" aria-label="番組の字幕履歴" elevation="12" @keydown.esc="$emit('close')">
    <div class="history-header">
      <strong>字幕履歴</strong>
      <v-btn icon small aria-label="字幕履歴を閉じる" @click="$emit('close')"><v-icon small>mdi-close</v-icon></v-btn>
    </div>
    <div class="history-title">{{ program.Title }}</div>
    <div ref="body" class="history-body" tabindex="0" @scroll="onScroll">
      <div v-for="(item, index) in displayItems" :key="item.ID" class="history-line">
        <v-btn v-if="item.Gap" small text :disabled="loading" @click="load">過去の字幕の続きを表示</v-btn>
        <template v-else><span class="history-number">{{ index + 1 }}</span><span>{{ item.Text }}</span></template>
      </div>
      <p v-if="loading" role="status">字幕を読み込み中…</p>
      <p v-else-if="error" role="alert">{{ error }} <button @click="load">再試行</button></p>
      <v-btn v-else-if="hasMore && !liveItems.length" small text @click="load">続きを表示</v-btn>
    </div>
  </v-card>
  </div>
</template>

<script>
import axios from 'axios';
import utility from '../utility.js';

export default {
  name: 'CaptionHistoryWindow',
  props: { program: Object, anchor: Object, api: String },
  data: () => ({ items: [], liveItems: [], pending: [], flushTimer: null, snapshot: null,
    liveBoundary: false, liveSequence: 0, lastHistoryID: 0,
    loading: false, error: '', hasMore: true, position: {}, disposed: false }),
  computed: {
    displayItems() {
      const history = this.items.slice();
      const live = this.liveItems.slice();
      const last = history[history.length - 1];
      const first = live[0];
      // The snapshot may end in the middle of a statement. Join its continuation
      // only after every history page has loaded and no clear separated them.
      if (!this.hasMore && last && first && !first.BreakBefore && last.ID === this.snapshot &&
          last.Pts === first.Pts && last.Lang === first.Lang) {
        history[history.length - 1] = { ...last, Text: last.Text + first.Text };
        live.shift();
      }
      if (this.hasMore && live.length) history.push({ ID: 'gap', Gap: true });
      return history.concat(live);
    },
  },
  mounted() {
    // Keep the root in Vue's parent tree so sibling insertions remain valid.
    document.body.appendChild(this.$refs.window.$el);
    this.place();
    window.addEventListener('resize', this.place);
    window.addEventListener('scroll', this.place, true);
    document.addEventListener('keydown', this.keydown);
    this.load();
  },
  beforeDestroy() {
    this.disposed = true;
    clearTimeout(this.flushTimer);
    this.pending = [];
    window.removeEventListener('resize', this.place);
    window.removeEventListener('scroll', this.place, true);
    document.removeEventListener('keydown', this.keydown);
    const panel = this.$refs.window.$el;
    if (panel.parentNode) panel.parentNode.removeChild(panel);
  },
  methods: {
    enqueue(packet) {
      if (this.disposed || packet.Sid !== this.program.Sid || packet.Eid !== this.program.Eid ||
          packet.Start !== this.program.Start) return;
      this.pending.push(packet);
      if (this.flushTimer === null) this.flushTimer = setTimeout(() => {
        this.flushTimer = null;
        this.flushPending();
      }, 120);
    },
    flushPending() {
      if (this.disposed || this.snapshot === null) return;
      const body = this.$refs.body;
      const follow = body && !this.hasMore && body.scrollHeight - body.scrollTop - body.clientHeight < 40;
      for (const packet of this.pending.splice(0)) {
        if (packet.HistoryID && packet.HistoryID <= Math.max(this.snapshot, this.lastHistoryID)) continue;
        if (packet.HistoryID) this.lastHistoryID = packet.HistoryID;
        this.liveBoundary = this.liveBoundary || packet.Clear;
        // Ruby fragments must still advance HistoryID and preserve clear boundaries.
        if (packet.Size === 'small' || packet.Size === 'micro' || !String(packet.Text || '').trim()) continue;
        const last = this.liveItems[this.liveItems.length - 1];
        if (last && !this.liveBoundary && last.Pts === packet.Pts && last.Lang === packet.Lang) {
          last.Text += packet.Text;
        } else {
          this.liveItems.push({ ID: 'live-' + ++this.liveSequence, Text: packet.Text,
            Pts: packet.Pts, Lang: packet.Lang, BreakBefore: Boolean(this.liveBoundary) });
        }
        this.liveBoundary = false;
      }
      if (follow) this.$nextTick(() => {
        if (!this.disposed && this.$refs.body) this.$refs.body.scrollTop = this.$refs.body.scrollHeight;
      });
    },
    keydown(event) { if (event.key === 'Escape') this.$emit('close'); },
    place() {
      if (!this.anchor.isConnected) { this.$emit('close'); return; }
      const rect = this.anchor.getBoundingClientRect();
      const width = Math.min(360, window.innerWidth - 16);
      const height = Math.min(520, window.innerHeight - 16);
      const right = window.innerWidth - rect.right;
      const left = right >= width + 8 || right >= rect.left ? rect.right + 8 : rect.left - width - 8;
      this.position = {
        left: Math.max(8, Math.min(left, window.innerWidth - width - 8)) + 'px',
        top: Math.max(8, Math.min(rect.top, window.innerHeight - height - 8)) + 'px',
        width: width + 'px', maxHeight: height + 'px',
      };
    },
    async load() {
      if (this.loading || !this.hasMore) return;
      this.loading = true;
      this.error = '';
      try {
        const response = await axios.get(this.api, { params: {
          sid: this.program.Sid, eid: this.program.Eid, start: this.program.Start,
          after: this.items.length ? this.items[this.items.length - 1].ID : 0,
          ...(this.snapshot === null ? {} : { snapshot: this.snapshot }),
        } });
        if (this.disposed) return;
        this.items.push(...response.data.items);
        this.snapshot = response.data.snapshot;
        this.hasMore = response.data.hasMore;
        this.flushPending();
        // Keep an invisible listener for a current program's first caption.
        if (!this.items.length && !this.liveItems.length && !this.hasMore &&
            !(this.program.End * 1000 > utility.now())) this.$emit('close');
      } catch (error) {
        if (!this.disposed) this.error = '字幕の取得に失敗しました。';
      } finally {
        this.loading = false;
      }
    },
    onScroll() {
      const body = this.$refs.body;
      if (!this.error && body.scrollHeight - body.scrollTop - body.clientHeight < 100) this.load();
    },
  },
};
</script>

<style scoped>
.caption-history-host { display: none; }
.caption-history { position: fixed; z-index: 210; display: flex; flex-direction: column; overflow: hidden; }
.history-header { display: flex; align-items: center; justify-content: space-between; padding: 8px 12px 0; }
.history-title { flex-shrink: 0; font-size: 12px; line-height: 1.6; padding: 4px 12px 8px; min-height: calc(2em * 1.6 + 12px); max-height: 70px; overflow: auto; white-space: normal; overflow-wrap: anywhere; }
.history-body { overflow-y: auto; overscroll-behavior: contain; padding: 0 12px 12px; min-height: 0; font-size: 12px; line-height: 1.6; }
.history-line { display: flex; gap: 8px; padding: 5px 0; border-top: 1px solid #8883; white-space: pre-wrap; overflow-wrap: anywhere; }
.history-number { flex: 0 0 30px; opacity: .55; font-size: 10px; }
</style>
