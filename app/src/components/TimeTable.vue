<template>

 <div :class="{ 'tt-side-layout': options.sideselection }" @click="onProgramClick" @keydown="onProgramKeydown">
 <div v-if="popupHtml" ref="programPopup" class="tt-program-popup" :style="popupStyle" :data-caption-key="popupCaptionKey"
   @mouseenter="cancelPopupClose" @mouseleave="closeHoveredProgramPopup" @keydown.esc="closeProgramWindows"
   v-html="popupHtml"></div>
 <CaptionHistoryWindow ref="captionHistory" v-if="options.captiontimetable && captionProgram" :key="captionKey" :program="captionProgram" :anchor="captionAnchor" :api="api.caption" @close="closeProgramWindows" />
 <v-card class="mx-auto tt-main-card" outlined>

  <v-container fluid fill-height class="tt-toolbar" :class="{ 'tt-side-toolbar': options.sideselection }">

    <v-row dense class="timetable-toolbar">

      <v-col cols="12" lg="3" class="d-flex flex-row toolbar-group toolbar-date-hours">
        <v-card class="pa-0 ma-0" flat>
          <v-card-text class="text-h5 px-0 mx-0 py-0 ma-0 text--darken2">
            <p class="toolbar-current-date" v-html="tvpdate"></p>
          </v-card-text>
        </v-card>
        <v-btn class="chgh-btn" density="compact" @click="changeHours(0)" icon="">
          <v-icon>
            mdi-home
          </v-icon>
        </v-btn>
        <v-btn class="chgh-btn" density="compact" @click="changeHours(-1)" icon="">
          <v-icon>
            mdi-chevron-double-up
          </v-icon>
        </v-btn>
        <v-btn class="chgh-btn" density="compact" @click="changeHours(1)" icon="">
          <v-icon>
            mdi-chevron-double-down
          </v-icon>
        </v-btn>
        <v-btn class="chgh-btn" density="compact" aria-label="カレンダーから日付を選択" title="カレンダーから日付を選択" @click="dpmodal = true; tmpdate=dpdate" icon="">
          <v-icon>
            mdi-calendar-month
          </v-icon>
        </v-btn>

      </v-col>

      <v-col v-if="!options.sideselection" cols="12" lg="9" class="d-flex toolbar-group toolbar-broadcast">

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle" v-model="hoursSel" @change="setHours" outlined>
          <v-btn v-for="n in tvszone()" :key="n">
            <span class="text-caption">{{n}}</span>
          </v-btn>
        </v-btn-toggle>

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle toolbar-broadcast-types" v-model="bcast" mandatory outlined>

          <v-btn v-if="parseInt(bcast) !== 6" width="70" value="1" @click="onclicktsb(bcast)">
            <span class="broadcast-label">地上波</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) === 6" width="70" value="6" @click="onclicktsb(bcast)">
            <span class="broadcast-label">マルチ</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) !== 7" width="70" value="2" @click="onclicktsb(bcast)">
            <span class="broadcast-label">BS</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) === 7" width="70" value="7" @click="onclicktsb(bcast)">
            <span class="broadcast-label">マルチ</span>
          </v-btn>

          <v-btn width="70" value="3">
            <span class="broadcast-label">CS</span>
          </v-btn>

          <v-btn width="70" value="4">
            <span class="broadcast-label">ワンセグ</span>
          </v-btn>

          <v-btn width="70" value="5">
            <span class="broadcast-label">週間</span>
          </v-btn>

          <v-btn width="70" value="0">
            <span class="broadcast-label">全局</span>
          </v-btn>

        </v-btn-toggle>

      </v-col>

    </v-row>

    <v-row dense class="timetable-toolbar"> <!-- class="d-flex flex-column"-->
      <v-col v-if="!options.sideselection" cols="12" lg="8" class="toolbar-group">

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle toolbar-dates" v-model="dateSel" @change="changeDate" outlined>
          <v-btn v-for="v in dateGroups" :key="v.Key" :aria-label="v.Name" :title="v.Name">
          <span class="text-caption date-label-full">{{v.Name}}</span>
          <span class="text-caption date-label-short" aria-hidden="true">{{v.ShortName}}</span>
          </v-btn>
        </v-btn-toggle>

      </v-col>

      <v-col v-if="maxPage > 1" cols="12" lg="4" class="d-flex align-end flex-column toolbar-group toolbar-pagination">
        <div class="ml-auto pagination-wrap">
          <v-pagination
          v-model="page"
          :length="maxPage"
          :total-visible="8"
          @input="changePage">
          </v-pagination>
        </div>
      </v-col>

    </v-row>
  </v-container>

  <div class="tt-split-scroll">
    <div ref="stationHeaderWrap" class="tt-station-header-wrap">
      <div ref="stationHeaderTable" class="tt1 tt-station-header" id="gst-header" v-timetable-html="stationHeaderContent"></div>
    </div>
    <div ref="programScrollBody" class="tt-scroll-body">
      <div ref="programTable" class="tt1" id="gst" :style="genreColorStyle" v-timetable-html="content"></div>
    </div>
  </div>

</v-card>

 <v-card v-if="options.sideselection" outlined class="tt-side-selection" aria-label="番組表の表示選択">
        <div class="tt-side-hours">
          <v-btn outlined tile aria-label="現在の時間に戻る" @click="changeHours(0)">
            <v-icon>mdi-home</v-icon>
          </v-btn>
          <v-btn outlined tile aria-label="前の時間帯" @click="changeHours(-1)">
            <v-icon>mdi-chevron-double-up</v-icon>
          </v-btn>
          <v-btn outlined tile aria-label="次の時間帯" @click="changeHours(1)">
            <v-icon>mdi-chevron-double-down</v-icon>
          </v-btn>
        </div>

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle" v-model="hoursSel" @change="setHours" outlined>
          <v-btn v-for="n in tvszone()" :key="n">
            <span class="text-caption">{{n}}</span>
          </v-btn>
        </v-btn-toggle>

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle" v-model="bcast" mandatory outlined>

          <v-btn v-if="parseInt(bcast) !== 6" width="70" value="1" @click="onclicktsb(bcast)">
            <span class="broadcast-label">地上波</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) === 6" width="70" value="6" @click="onclicktsb(bcast)">
            <span class="broadcast-label">マルチ</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) !== 7" width="70" value="2" @click="onclicktsb(bcast)">
            <span class="broadcast-label">BS</span>
          </v-btn>

          <v-btn v-if="parseInt(bcast) === 7" width="70" value="7" @click="onclicktsb(bcast)">
            <span class="broadcast-label">マルチ</span>
          </v-btn>

          <v-btn width="70" value="3">
            <span class="broadcast-label">CS</span>
          </v-btn>

          <v-btn width="70" value="4">
            <span class="broadcast-label">ワンセグ</span>
          </v-btn>

          <v-btn width="70" value="5">
            <span class="broadcast-label">週間</span>
          </v-btn>

          <v-btn width="70" value="0">
            <span class="broadcast-label">全局</span>
          </v-btn>

        </v-btn-toggle>

        <v-btn-toggle class="pa-0 ma-0 toolbar-toggle toolbar-dates" v-model="dateSel" @change="changeDate" outlined>
          <v-btn v-for="v in dateGroups" :key="v.Key" :aria-label="v.Name" :title="v.Name">
          <span class="text-caption date-label-full">{{v.Name}}</span>
          <span class="text-caption date-label-short" aria-hidden="true">{{v.ShortName}}</span>
          </v-btn>
        </v-btn-toggle>

        <v-btn icon aria-label="カレンダーから日付を選択" title="カレンダーから日付を選択" @click="dpmodal = true; tmpdate=dpdate">
          <v-icon>mdi-calendar-month</v-icon>
        </v-btn>


 </v-card>
        <v-dialog ref="dialog" v-model="dpmodal" persistent width="300px">
          <v-date-picker v-model="dpdate" locale="jp" :day-format="dt => new Date(dt).getDate()" scrollable>
            <div class="flex-grow-1"></div>
            <v-btn text color="primary" @click="dpmodal = false; dpdate=tmpdate">Cancel</v-btn>
            <v-btn text color="primary" @click="datePicker">OK</v-btn>
          </v-date-picker>
        </v-dialog>
</div>
</template>

<script>
  import label from '../define.js';
  import utility from '../utility.js';
  import axios from 'axios';
  import CaptionHistoryWindow from './CaptionHistoryWindow.vue';
  import { timeTableGenres } from '../timeTableGenreColors';

  import { createMinuteGrid, fillMinuteGrid, renderStationHeaders, renderProgramGrid, getNextProgramTransitionDelay, patchTimeTableHtml, getPixelsPerMinute, getServiceDisplayCount } from '../timeTableRenderer';

  export default {
    name: 'TimeTable',
    components: { CaptionHistoryWindow },
    directives: {
      timetableHtml: {
        bind: (element, binding) => patchTimeTableHtml(element, binding.value),
        update: (element, binding) => {
          if (binding.value !== binding.oldValue) patchTimeTableHtml(element, binding.value);
        },
      },
    },

   data() {
        return {
          popupHtml: '',
          popupStyle: {},
          popupKey: '',
          popupCaptionKey: '',
          popupCloseTimer: null,
          expandedProgramCell: null,
          captionProgram: null,
          captionAnchor: null,
          captionKey: '',
          clickedProgramKey: '',
          programClickStage: '',
          content: '',
          tvpdate: '',
          bcast: ``+this.options.broadcast,
          page: 1,
          maxPage: 0,
          renderQueued: false,
          programRenderDeferred: false,

          dpmodal: false,

          dateSel: 0,
          stationHeaderContent: '',
          hoursSel: 0,
          befHour: 0,
          dateorg: (() => {
            let d = utility.now();
            return new Date(d.getFullYear(),d.getMonth(),d.getDate(),0,0);
          })(),

          dpdate: (() => {
            let d = utility.now();
            return d.getFullYear() + '-' + (d.getMonth()+1) + '-' + d.getDate();
          })(),

          tmpdate: null,

          nextRefresh: null,
          programs: {},
          resetProgramScrollOnRender: false,
          pendingRedrawTimetable: null,
          renderOnShow: false,
          syncStationHeaderResizeObserver: null,
          observedScrollBody: null,
          serviceDisplayWidth: 0,
          renderedStart: null,
          renderedPixelsPerMinute: 4,
          renderedHeaderDay: null,
          virtualStart: null,
          preserveVirtualStartOnRender: false,
          pendingProgramTime: null,
          recenterAfterRender: false,
          extendingPrograms: false,
          extensionRetryAfter: 0,
          programScrollFrame: null,
          prefetchAnchor: null,
          prefetchedRanges: [],

        }
    },

    props: [
        'api',
        'options',
        'timetable',
        'station',
        'schedule'
    ],

    computed: {
      genreColorStyle() {
        const theme = this.$vuetify.theme.dark ? 'dark' : 'light';
        const overrides = this.options.genreColors?.[theme] || {};
        const style = {};
        for (const genre of timeTableGenres) {
          style['--tt-genre-' + genre.code] = overrides[genre.code] || genre[theme];
        }
        return style;
      },
      dateGroups() {
        const selectedDate = new Date(this.dpdate);
        const date = new Date(selectedDate < this.dateorg ? selectedDate : this.dateorg);
        const groups = [];
        for (let i = 0; i <= 7; i++) {
          groups.push({
            Key: i,
            Name: (date.getMonth() + 1) + '月' + date.getDate() + '日(' + label.WEEKW[date.getDay()] + ')',
            ShortName: (date.getMonth() + 1) + '/' + date.getDate(),
          });
          date.setDate(date.getDate() + 1);
        }
        return groups;
      },
      renderingOptions() {
        // デバイスのピン留めや字幕設定だけの変更では表を再生成しない。
        const keys = ['displogo', 'guidetype', 'subtitlemin', 'contentmin', 'divider',
          'twentyEight', 'dispservice', 'watchbutton', 'hours'];
        return keys.map(key => this.options[key]);
      },
    },

    watch: {
      timetable: 'queueProgramRender',
      station: 'queueProgramRender',
      'options.captiontimetable'(enabled) {
        if (!enabled && this.captionProgram) this.closeProgramWindows();
      },
      'options.divider'() {
        if (this.options.dispservice === 0 && !this.options.weekly) {
          this.requestRedrawWithProgramScrollReset(this.options, true, true);
        }
      },
      renderingOptions: {
        handler() {
          this.closeProgramWindows();
          this.queueProgramRender();
        },
        immediate: true,
      },
      bcast: function(v) {
        if (Number(v) === this.options.broadcast) return;
        const opt = this.options;
        opt.broadcast = Number(v);
        opt.weekly = opt.broadcast === label.TSP_ST;
        this.requestRedrawWithProgramScrollReset(opt);
      },
    },

    methods: {
      displayServiceCount() {
        return getServiceDisplayCount(this.options, this.serviceDisplayWidth);
      },

      programScrollElement() {
        return this.$refs.programScrollBody;
      },

      programGridTop(body) {
        const grid = this.$el.querySelector('#gst .tt-program-grid');
        if (!grid) return 0;
        return grid.getBoundingClientRect().top + body.scrollTop - body.getBoundingClientRect().top - body.clientTop;
      },

      onTimeTableScroll() {
        if (this._isBeingDestroyed || this._isDestroyed) return;
        if (this.programScrollFrame !== null) return;
        this.programScrollFrame = requestAnimationFrame(() => {
          this.programScrollFrame = null;
          this.checkProgramScroll();
        });
      },

      onTimeTableScrollEnd(event) {
        const body = this.programScrollElement();
        if (!body || !this.timetable || this.virtualStart === null ||
            !this.$el.getClientRects().length) return;
        if (event && event.target !== body) return;
        if (this.resetProgramScrollOnRender) {
          this.recenterAfterRender = true;
          return;
        }
        const pixelsPerMinute = this.renderedPixelsPerMinute || getPixelsPerMinute(this.options);
        const visible = this.renderedStart + (body.scrollTop - this.programGridTop(body)) * (60 / pixelsPerMinute);
        this.recenterAfterRender = false;
        // 操作が終わってから端の座標を補充する。ドラッグ中のつまみは動かさない。
        // ショートでは画面内に6時間以上入るため、上端だけでなく下端の時刻も見る。
        const viewportSeconds = body.clientHeight * 60 / pixelsPerMinute;
        const availableSeconds = Math.max(0, 48 * 3600 - viewportSeconds);
        const marginSeconds = Math.min(21600, availableSeconds / 4);
        if (visible < this.virtualStart + marginSeconds ||
            visible + viewportSeconds > this.virtualStart + 48 * 3600 - marginSeconds) {
          // 表示領域全体の前後に同じだけスクロールの余地を確保する。
          const nextStart = visible - availableSeconds / 2;
          if (nextStart === this.virtualStart) return;
          this.virtualStart = nextStart;
          this.queueProgramRender();
        }
      },

      checkProgramScroll() {
        const tt = this.timetable;
        const body = this.programScrollElement();
        if (!tt || this.resetProgramScrollOnRender || !body || !body.clientHeight || !this.$el.getClientRects().length ||
            this.renderedStart !== tt.Start) return;
        const top = body.scrollTop - this.programGridTop(body);
        const pixelsPerMinute = this.renderedPixelsPerMinute || getPixelsPerMinute(this.options);
        const visible = tt.Start + top * (60 / pixelsPerMinute);
        this.tvpdate = utility.uxtmtomdh(visible, utility.now(), this.options.twentyEight) + '~';
        const current = new Date(Math.floor(visible / 3600) * 3600000);
        if (tt.Week && this.renderedHeaderDay !== current.toDateString()) this.queueProgramRender();
        if (this.options.current.getTime() !== current.getTime()) this.$emit('timechange', current);
        this.hoursSel = this.tvszone().indexOf(current.getHours());
        const date = new Date(current.getFullYear(), current.getMonth(), current.getDate());
        if (this.options.twentyEight && current.getHours() < 4) date.setDate(date.getDate() - 1);
        this.dpdate = date.getFullYear() + '-' + String(date.getMonth() + 1).padStart(2, '0') +
          '-' + String(date.getDate()).padStart(2, '0');
        const days = Math.round((date - this.dateorg) / 86400000);
        this.dateSel = days < 0 ? 0 : days <= 7 ? days : -1;
        if (this.extendingPrograms || Date.now() < this.extensionRetryAfter) return;
        // スクロールバーで遠くへ移動した場合は、その時刻を直接取得する。
        if (visible < tt.Start - 14400 || visible > tt.End + 14400) {
          this.pendingProgramTime = visible;
          this.requestRedrawWithProgramScrollReset({ ...this.options, current }, true);
          return;
        }
        const height = (tt.End - tt.Start) * pixelsPerMinute / 60;
        const margin = Math.min(720, Math.max(240, body.clientHeight));
        const direction = top < margin ? -1 : top + body.clientHeight > height - margin ? 1 : 0;
        if (!direction) return;
        // 読み込み完了による再判定で、破棄済みの前後範囲を往復取得しない。
        // 表示位置または領域の高さが変わった時だけ、先読み済み範囲をリセットする。
        if (!this.prefetchAnchor || Math.abs(visible - this.prefetchAnchor.time) > 15 ||
            this.prefetchAnchor.height !== body.clientHeight) {
          this.prefetchAnchor = { time: visible, height: body.clientHeight };
          this.prefetchedRanges = [];
        }
        const rangeKey = String(direction < 0 ? tt.Start - 14400 : tt.End);
        if (this.prefetchedRanges.includes(rangeKey)) return;
        this.prefetchedRanges.push(rangeKey);
        this.extendingPrograms = true;
        // 高い画面でも表示中の行を取り除かないよう、画面高と先読みに余裕を持たせる。
        const fourHourHeight = 240 * pixelsPerMinute;
        const maxHours = Math.max(12, Math.ceil((body.clientHeight + margin * 2 + fourHourHeight) / fourHourHeight) * 4);
        const requestedRanges = this.prefetchedRanges;
        this.$emit('extendtt', { direction, maxHours, done: applied => {
          this.extendingPrograms = false;
          if (!applied) {
            const index = requestedRanges.indexOf(rangeKey);
            if (index >= 0) requestedRanges.splice(index, 1);
          }
          // 失敗時や競合時はスクロール操作で再試行できる。
          this.extensionRetryAfter = applied ? 0 : Date.now() + 500;
          if (applied) this.$nextTick(this.onTimeTableScroll);
        } });
      },

      queueProgramRender() {
        if (this.renderQueued) return;
        this.renderQueued = true;
        this.$nextTick(() => {
          this.renderQueued = false;
          if (!this._isBeingDestroyed && !this._isDestroyed) this.renderProgram(this.timetable);
        });
      },
      onProgramKeydown(event) {
        if (event.key === 'Escape') {
          this.closeProgramWindows();
          return;
        }
        if (event.target.closest('a, button, label, .caption-history')) return;
        const cell = event.target.closest('[data-caption-key]');
        if (cell && (event.key === 'Enter' || event.key === ' ')) {
          event.preventDefault();
          this.onProgramClick(event);
        }
      },
      queueHistoryCaption(packet) {
        if (this.options.captiontimetable && this.$refs.captionHistory) this.$refs.captionHistory.enqueue(packet);
      },
      onProgramClick(event) {
        if (event.target.closest('a, button, label, .caption-history, [onclick]')) return;
        const header = event.target.closest('[data-week-start]');
        if (header) {
          const date = new Date(Number(header.dataset.weekStart) * label.MSEC);
          date.setHours(this.options.current.getHours(), 0, 0, 0);
          this.changeProgramDate(this.timetable.services[0], utility.tmtoux(date), true);
          return;
        }
        const cell = event.target.closest('[data-caption-key]');
        if (!cell) return;
        const key = cell.dataset.captionKey;
        const program = this.programs[key];
        if (!program) return;
        const stage = this.clickedProgramKey === key ? this.programClickStage : '';
        this.closeProgramWindows();
        if (stage === 'captions' || (stage === 'details' && !this.options.captiontimetable)) {
          return;
        }
        if (stage === 'details') {
          this.captionAnchor = document.getElementById('L' + program.Sid + '_' + program.Eid);
          this.captionKey = key;
          this.captionProgram = program;
          this.programClickStage = 'captions';
        } else {
          this.pgClick(program.Sid, program.Eid, 1, event.type === 'click' ? event : undefined);
          this.programClickStage = 'details';
        }
        this.clickedProgramKey = key;
      },
      closeProgramWindows() {
        this.captionProgram = null;
        this.closeProgramPopup();
        this.clickedProgramKey = '';
        this.programClickStage = '';
      },
      closeHoveredProgramPopup() {
        if (!this.programClickStage) this.closeProgramPopup();
      },
      syncStationHeaderLayout() {
        const body = this.$refs.programScrollBody;
        const headerWrap = this.$refs.stationHeaderWrap;
        const headerTable = this.$refs.stationHeaderTable;
        const programTable = this.$refs.programTable;
        if (!body || !headerWrap || !headerTable || !programTable) {
          return;
        }

        if (this.syncStationHeaderResizeObserver && this.observedScrollBody !== body) {
          this.syncStationHeaderResizeObserver.disconnect();
          this.syncStationHeaderResizeObserver.observe(body);
          this.syncStationHeaderResizeObserver.observe(headerWrap);
          const toolbar = this.$el.querySelector('.tt-toolbar');
          if (toolbar) this.syncStationHeaderResizeObserver.observe(toolbar, { box: 'border-box' });
          this.observedScrollBody = body;
        }
        const bodyWidth = body.clientWidth;
        // Inactive v-tab-items are display:none. Keep the last visible width:
        // writing 0px can leave mobile browsers with a wrapped, oversized header.
        if (bodyWidth <= 0) {
          return;
        }
        const previousCount = this.displayServiceCount();
        this.serviceDisplayWidth = bodyWidth;
        if (this.options.dispservice === 0 && !this.options.weekly &&
            previousCount !== this.displayServiceCount()) {
          const count = this.displayServiceCount();
          const opt = this.options;
          opt.page[opt.broadcast] = Math.floor((opt.page[opt.broadcast] || 0) / count) * count;
          this.requestRedrawWithProgramScrollReset(opt, true, true);
        }
        // Toolbar wrapping and the station header determine the space left below.
        const height = Math.max(0, window.innerHeight - body.getBoundingClientRect().top - 1) + 'px';
        if (body.style.getPropertyValue('--tt-scroll-height') !== height) {
          body.style.setProperty('--tt-scroll-height', height);
        }
        if (this.renderOnShow) this.queueProgramRender();
        const width = bodyWidth + 'px';
        for (const element of [headerWrap, headerTable, programTable]) {
          if (element.style.width !== width) element.style.width = width;
        }
      },

      requestRedrawWithProgramScrollReset(opt, preserveVirtualStart = false, preserveRange = false) {
        this.resetProgramScrollOnRender = true;
        this.pendingRedrawTimetable = this.timetable;
        this.closeProgramWindows();
        this.preserveVirtualStartOnRender = preserveVirtualStart;
        if (!preserveVirtualStart) this.pendingProgramTime = null;
        this.prefetchAnchor = null;
        this.prefetchedRanges = [];
        this.$emit('redrawtt', opt, preserveRange);
      },

      renderProgram(tt) {
        // 操作中はデータの受信を続け、描画要求だけを一つにまとめて保留する。
        if (!this.resetProgramScrollOnRender && (this.popupHtml || this.expandedProgramCell || this.captionProgram)) {
          this.programRenderDeferred = true;
          return;
        }
        const body = this.$el && this.programScrollElement();
        // 非表示タブは座標を測れない。再表示後に旧縮尺の座標から時刻を引き継ぐ。
        if (body && body.clientWidth === 0) {
          this.renderOnShow = true;
          return;
        }
        // 親がoptionsを更新しただけで、取得待ちの範囲を旧データで描画し直さない。
        // 待機フラグを解除すると毎フレーム同じ取得を開始し、応答を破棄し続けてしまう。
        if (this.resetProgramScrollOnRender && this.renderedStart !== null && tt && tt === this.pendingRedrawTimetable) return;
        this.renderOnShow = false;
        const anchor = body && this.renderedStart !== null ?
          this.renderedStart + (body.scrollTop - this.programGridTop(body)) * (60 / (this.renderedPixelsPerMinute || 4)) : null;
        const pixelsPerMinute = getPixelsPerMinute(this.options);
        if (pixelsPerMinute !== this.renderedPixelsPerMinute) {
          this.prefetchAnchor = null;
          this.prefetchedRanges = [];
        }
        const resetScroll = this.resetProgramScrollOnRender;
        this.programRenderDeferred = false;
        this.closeProgramWindows();
        clearTimeout(this.nextRefresh);
        if (tt == null) {
          this.content = '';
          this.stationHeaderContent = '';
          this.programs = Object.freeze({});
          this.maxPage = 0;
          this.tvpdate = '';
          this.renderedStart = null;
          this.virtualStart = null;
          return;
        }
        const sord = {
          0: this.station.viewAll,
          1: this.station.viewTS,
          2: this.station.viewBS,
          3: this.station.viewCS,
          4: this.station.viewOS,
          5: this.station.viewAll,
          6: this.station.viewTSAll,
          7: this.station.viewBSAll
        };

        // ページ設定
        let opt = this.options;
        const pageOffset = opt.page[opt.broadcast] || 0;
        this.page = opt.broadcast == label.TSP_ST ?
        pageOffset + 1:
        parseInt(pageOffset / this.displayServiceCount()) + 1;
        this.maxPage = opt.broadcast == label.TSP_ST ?
        sord[opt.broadcast] :
        parseInt(Math.ceil(sord[opt.broadcast] / this.displayServiceCount()));

        let gs = new Date(tt.Start * label.MSEC);
        let st = parseInt(tt.Start / label.SEC);
        let ed = parseInt(tt.End / label.SEC);
        let wh = ed - st;
        let sl = tt.Week ? label.WEEK : tt.ServiceList.length;
        const renderOptions = { ...opt, weekly: Boolean(tt.Week) };
        const displayedTime = resetScroll && this.preserveVirtualStartOnRender && anchor !== null ? anchor :
          resetScroll && this.pendingProgramTime !== null ? this.pendingProgramTime :
          resetScroll || anchor === null ? utility.tmtoux(opt.current) : anchor;
        const headerData = tt.Week ? { ...tt, HeaderStart: displayedTime } : tt;
        this.renderedHeaderDay = tt.Week ? new Date(displayedTime * label.MSEC).toDateString() : null;
        let rsu = renderStationHeaders(tt.services, headerData, renderOptions, -1);
        let rsd = renderStationHeaders(tt.services, headerData, renderOptions, 1);
        const programs = {};
        let rp = renderProgramGrid(fillMinuteGrid(tt.services, tt, createMinuteGrid(wh, sl), st, ed), gs, wh, sl, renderOptions, programs);
        // データの追加・破棄でスクロール領域の高さを変えない。
        // 48時間分の座標だけを確保し、番組DOMは取得した範囲だけに置く。
        if ((resetScroll && !this.preserveVirtualStartOnRender) || this.virtualStart === null) {
          this.virtualStart = displayedTime - 86400;
        }
        rp = '<div class="tt-virtual-timeline" style="position:relative;height:' + (48 * 60 * pixelsPerMinute) + 'px;overflow:clip">' +
          '<div class="tt-virtual-grid" style="position:absolute;width:100%;top:' +
          ((tt.Start - this.virtualStart) * pixelsPerMinute / 60) + 'px">' + rp + '</div></div>';
        this.programs = Object.freeze(programs);
        this.stationHeaderContent = rsu;
        this.content = rp + rsd;
        this.tvpdate = utility.uxtmtomdh(displayedTime, utility.now(), opt.twentyEight) + '~';
        this.renderedStart = tt.Start;
        this.renderedPixelsPerMinute = pixelsPerMinute;
        this.resetProgramScrollOnRender = false;
        this.pendingRedrawTimetable = null;
        this.preserveVirtualStartOnRender = false;
        this.pendingProgramTime = null;
        this.$nextTick(() => {
          if (this._isBeingDestroyed || this._isDestroyed) return;
          this.syncStationHeaderLayout();
          const scrollBody = this.programScrollElement();
          if (!scrollBody) return;
          const scrollTop = this.programGridTop(scrollBody) + (displayedTime - tt.Start) * pixelsPerMinute / 60;
          if (Math.abs(scrollBody.scrollTop - scrollTop) > 0.5) scrollBody.scrollTop = scrollTop;
          if (this.recenterAfterRender) this.onTimeTableScrollEnd();
          this.onTimeTableScroll();
        });

        // 開始・終了の色や操作状態は取得済みデータで更新できる。
        // データの再取得はスクロール操作とサーバーからの変更通知で行う。
        let npd = getNextProgramTransitionDelay(tt);
        if (npd > 0) {
          this.nextRefresh = setTimeout(this.queueProgramRender, Math.min(npd, 2147483647));
        }
      },

      changePage(v) {
        let opt = this.options;
        opt.page[opt.broadcast] = opt.broadcast == label.TSP_ST ?
        v - 1:
        parseInt((v-1) * this.displayServiceCount());
        this.requestRedrawWithProgramScrollReset(opt);
      },

      changeDate(v) {
        if (v == undefined || v == null) {
          this.dateSel = 0;
          v = 0;
        }
        let opt = this.options;
        let d = new Date(this.dateorg);
        let dd = new Date(this.dpdate);
        let oldsel = this.dateSel;
        if (dd < this.dateorg) {
          d = new Date(dd);
        }
        opt.current = new Date(d.getFullYear(),d.getMonth(),d.getDate()+v,opt.current.getHours(),0);
        this.dpdate = d.getFullYear()+'-'+(d.getMonth()+1)+'-'+(d.getDate()+v-oldsel);
        this.requestRedrawWithProgramScrollReset(opt);
        utility.log("change date "+opt.current.toLocaleString('ja-JP'));
      },

      datePicker() {
        let opt = this.options;
        let d = new Date(this.dpdate);
        opt.current = new Date(d.getFullYear(),d.getMonth(),d.getDate(),opt.current.getHours(),0);
        this.dateorg = new Date(d.getFullYear(),d.getMonth(),d.getDate(),0,0);
        this.dateSel = 0;
        this.befHour = 99;
        this.requestRedrawWithProgramScrollReset(opt);
        this.dpmodal = false;
      },

      changeHours(v) {
        let opt = this.options;
        if (v == 0) {
          let d = utility.now();
          opt.current = new Date(d.getFullYear(),d.getMonth(),d.getDate(),d.getHours(),0);
          if (opt.twentyEight && d.getHours() < 4) {
            d.setDate(d.getDate()-1);
          }
          this.dateorg  = new Date(d.getFullYear(),d.getMonth(),d.getDate(),0,0);
          this.dpdate = d.getFullYear()+'-'+(d.getMonth()+1)+'-'+d.getDate();
          this.dateSel = 0;
          this.befHour = 99;
        } else {
          let add = opt.current.getHours()+v
          opt.current.setHours(add);
          // 日にち指定更新
          if (add>=24) {
            this.dateSel++;
          } else if (add < 0) {
            this.dateSel--;
          }
        }

        // 時間帯指定更新
        let curh = opt.current.getHours();
        let hour = this.tvszone();
        let hs = -1;
        for (let i = 0; i < hour.length; i++) {
          if (hour[i] == curh) {
            hs = i;
            break;
          }
        }
        this.hoursSel = hs;
        this.befHour = v === 0 ? 99 : this.befHour;
//        this.dpdate = (new Date(opt.current.getFullYear(),opt.current.getMonth(),opt.current.getDate()+v,0,0)).toISOString().substr(0, 10);

        this.requestRedrawWithProgramScrollReset(opt);
      },

      setHours(v) {
        if (v == undefined || v == null) {
          this.hoursSel = 0;
          v = 0;
        }
        let opt = this.options;
        let hour = this.tvszone();
        if (v < 0 || v >= hour.length) {
          return
        }
        opt.current.setHours(hour[v]);
        this.befHour = 99;
        this.requestRedrawWithProgramScrollReset(opt);
      },

      setHoursImmediately(v) {
        if (v < 0 || v >= 24) {
          return
        }
        let opt = this.options;
        opt.current.setHours(v);
        this.befHour = 99;
        this.requestRedrawWithProgramScrollReset(opt);
      },

      stationBroadcast(station) {
        if (station.Tsp == label.TSP_TS && station.Parent > 0) return label.TSP_TSALL;
        if (station.Tsp == label.TSP_BS && station.Parent > 0) return label.TSP_BSALL;
        return station.Tsp;
      },

      // ページを設定
      calcStationPage(sid) {
        const sord = [
          this.station.ts,
          this.station.bs,
          this.station.cs,
          this.station.os,
          this.station.tsall,
          this.station.bsall,
        ];
        const nord = [
          this.station.viewTS,
          this.station.viewBS,
          this.station.viewCS,
          this.station.viewOS,
          this.station.viewTSAll,
          this.station.viewBSAll,
        ];
        let n = 0;
        for (const v of sord) {
          const mx = nord[n];
          let w = 0;
          for (const vv of Object.values(v)) {
            if (w >= mx) {
              break;
            }
            if (vv.View > 0 && vv.Sid == sid) {
              let opt = this.options;
              const tsp = this.stationBroadcast(vv);
              opt.page[tsp] = parseInt((w / this.displayServiceCount()))*this.displayServiceCount();
              opt.broadcast = tsp;
              this.bcast = ''+tsp;
              opt.weekly = false;
              this.page = parseInt(opt.page[tsp] / this.displayServiceCount()) + 1;
              return;
            }
            w = w + (vv.View > 0 ? 1 : 0)
          }
          n = n + 1;
        }
      },

      changeProgramDate(sid, v, leaveWeekly = false) {
        let d = new Date(v * label.MSEC);
        let opt = this.options;
        let cur = new Date(d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), 0);
        // 表示中のデータの範囲で判断する。移動先から終了時刻を作らない。
        const tt = this.timetable;
        const station = this.station.all[sid];
        // マルチ表にも通常局が含まれるため、表示種別が変わる場合は再取得する。
        const sameBroadcast = station && this.stationBroadcast(station) === opt.broadcast;
        if (!leaveWeekly && sameBroadcast && tt && !tt.Week && v >= tt.Start && v < tt.End &&
            tt.ServiceList.some(service => String(service) === String(sid))) {
          if (this.$el) {
            const body = this.programScrollElement();
            if (body) body.scrollTop = this.programGridTop(body) + (v - tt.Start) * getPixelsPerMinute(this.options) / 60;
          }
          return;
        }

        opt.current = cur;

        // 日付選択(dateSel)と時間選択(hoursSel)のインデックスも合わせて更新する
        let diffDays = Math.floor((opt.current.getTime() - this.dateorg.getTime()) / (24 * 60 * 60 * 1000));
        if (diffDays >= 0 && diffDays < 8) {
          this.dateSel = diffDays;
        } else {
          this.dateSel = 0;
        }

        let curh = opt.current.getHours();
        let hour = this.tvszone();
        let hs = -1;
        for (let i = 0; i < hour.length; i++) {
          if (hour[i] <= curh) {
            hs = i;
          }
        }

        this.hoursSel = hs;
        this.befHour = 99;

        this.calcStationPage(sid);

        this.requestRedrawWithProgramScrollReset(opt);
      },

      // tt functions
      tvpWeekly(v) {
        // ページの算出
        let sd = this.station.all;
        let p = 0;
        let n = 0;
        Object.keys(sd).forEach(function(key) {
          let s = sd[key];
          if (s.View > 0) {
            if (s.Sid == v) {
              p = n;
            }
            n++;
          }
        });

        let opt = this.options;
        opt.broadcast = label.TSP_ST;
        opt.weekly = true;
        opt.page[label.TSP_ST] = p;
        this.bcast = ''+label.TSP_ST;
        this.requestRedrawWithProgramScrollReset(opt);
      },

      tvpWeeklyReverse(v) {
        let sd = this.station.all;
        let tsp = label.TSP_TS;
        Object.keys(sd).forEach(function(key) {
          let s = sd[key];
          if (s.View > 0) {
            if (s.Sid == v) {
              tsp = s.Tsp;
            }
          }
        });

        let opt = this.options;
        opt.broadcast = tsp;
        opt.weekly = false;
        this.bcast = ''+tsp;
        this.requestRedrawWithProgramScrollReset(opt);
      },

      cancelPopupClose() {
        clearTimeout(this.popupCloseTimer);
      },

      closeProgramPopup() {
        this.cancelPopupClose();
        if (this.expandedProgramCell) {
          this.expandedProgramCell.classList.remove('tt-program-expanded');
          this.expandedProgramCell.style.removeProperty('min-height');
          this.expandedProgramCell.style.removeProperty('--tt-expanded-background');
          this.expandedProgramCell.style.removeProperty('--tt-expanded-offset');
          this.expandedProgramCell = null;
        }
        this.popupHtml = '';
        this.popupKey = '';
        if (this.programClickStage === 'details') {
          this.clickedProgramKey = '';
          this.programClickStage = '';
        }
        if (this.programRenderDeferred && !this.captionProgram) {
          this.programRenderDeferred = false;
          // 同時に届いたpropsの更新通知も、同じ描画にまとめる。
          this.$nextTick(this.queueProgramRender);
        }
      },

      onPopupScroll(event) {
        // 展開した番組欄は表と一緒にスクロールし、長い本文も最後まで読める。
        if (this.expandedProgramCell) return;
        const popup = this.$refs.programPopup;
        if (!popup || !popup.contains(event.target)) this.closeProgramPopup();
      },

      pgClick(sid, eid, md, event) {
        this.cancelPopupClose();
        if (this.programClickStage) return;
        if (md === 2) {
          this.popupCloseTimer = setTimeout(this.closeProgramPopup, 150);
          return;
        }
        const key = sid + '_' + eid;
        if (md === 0 && this.popupKey === key) {
          this.closeProgramPopup();
          return;
        }
        const content = document.getElementById('pcp' + key);
        const cell = document.getElementById('L' + key);
        if (!content || !cell) return;
        const guideType = Number(this.options.guidetype);
        if (guideType === 2 || guideType === 3) {
          if (this.expandedProgramCell === cell) return;
          this.closeProgramPopup();
          const rect = cell.getBoundingClientRect();
          const style = window.getComputedStyle(cell);
          const body = cell.querySelector('.pst');
          // stickyで追従中の本文位置を、展開後も通常フロー内の余白として維持する。
          // スクロール量そのものではなく実際の表示位置を使い、分割スクロールにも対応する。
          const contentTop = rect.top + parseFloat(style.borderTopWidth) + parseFloat(style.paddingTop);
          const offset = body ? Math.max(0, body.getBoundingClientRect().top - contentTop) : 0;
          cell.style.minHeight = rect.height + 'px';
          cell.style.setProperty('--tt-expanded-offset', offset + 'px');
          cell.style.setProperty('--tt-expanded-background', style.backgroundColor);
          cell.classList.add('tt-program-expanded');
          this.expandedProgramCell = cell;
          this.popupKey = key;
          return;
        }
        const rect = cell.getBoundingClientRect();
        const tableRect = cell.closest('.tt1').getBoundingClientRect();
        const pointerX = event ? event.clientX : (rect.left + rect.right) / 2;
        const gap = 8;
        const leftSpace = Math.max(0, rect.left - gap * 2);
        const rightSpace = Math.max(0, window.innerWidth - rect.right - gap * 2);
        let onLeft = pointerX >= (tableRect.left + tableRect.right) / 2;
        // 端の列で余白が足りない場合は、番組枠に重ならない反対側を使う。
        if ((onLeft ? leftSpace : rightSpace) < 160 &&
            (onLeft ? rightSpace : leftSpace) > (onLeft ? leftSpace : rightSpace)) {
          onLeft = !onLeft;
        }
        const sideSpace = onLeft ? leftSpace : rightSpace;
        const below = window.innerHeight - rect.bottom > rect.top;
        const beside = sideSpace >= 160;
        const width = Math.min(360, beside ? sideSpace : window.innerWidth - 16);
        const anchorTop = event ? Math.max(rect.top, event.clientY - 12) : rect.top;
        const left = beside ? (onLeft ? rect.left - width : rect.right) :
          Math.max(gap, Math.min(rect.left, window.innerWidth - width - gap));
        const maxHeight = beside ? window.innerHeight - gap * 2 :
          Math.max(0, (below ? window.innerHeight - rect.bottom : rect.top) - gap * 2);
        // 横に置けない狭い画面では上下の空き領域を使用する。
        if (maxHeight < 40) {
          this.closeProgramPopup();
          return;
        }
        this.popupKey = key;
        this.popupCaptionKey = cell.dataset.captionKey;
        this.popupHtml = content.innerHTML;
        this.popupStyle = {
          width: width + 'px',
          left: left + 'px',
          maxHeight: maxHeight + 'px',
          top: Math.max(8, anchorTop) + 'px',
        };
        this.$nextTick(() => {
          const popup = this.$refs.programPopup;
          if (!popup) return;
          this.popupStyle = { ...this.popupStyle,
            top: (beside ? Math.max(gap, Math.min(anchorTop, window.innerHeight - popup.offsetHeight - gap)) :
              (below ? rect.bottom + gap : rect.top - gap - popup.offsetHeight)) + 'px',
          };
        });
      },

      pgReserve(sid, eid, start, end, watch = false) {
        let par = {
          sid: sid,
          eid: eid,
          start: start,
          end: end
        };

        if (watch) par.watch = true;

        axios.post(this.api.schedule, JSON.stringify(par)).then(res => {
          utility.log("api schedule results:"+res.status);
          this.closeProgramPopup();
          // API updateを行った時点でwsから通知が来て再描画
        }).catch(err => {
          utility.err(`request add schedule error: ${err}`);
        });
      },

      pgSearch(key) {
        let p = this.programs[key];
        if (!p) {
          utility.err("program not found (" + key + ")")
          return;
        }

        this.$emit('programsearch', {
          title: utility.extractUnifiedTitle(p.Title),
          ssid: p.Sid,
        });
      },

      pgDelete(id) {
        for (const v of this.schedule) {
          if (id == v.id) {
            axios.delete(this.api.schedule+'/'+v.id).then(res => {
              utility.log("api schedule results:"+res.status);
              this.closeProgramPopup();
              // API updateを行った時点でwsから通知が来て再描画
            }).catch(err => {
              utility.err(`request delete schedule error: ${err}`);
            });
            return;
          }
      }
        utility.err("schedule not found ("+id+")")
      },

      pgInvalid(id) {
        for (let v of this.schedule) {
          if (id == v.id) {
            v.validate = false;
            axios.put(this.api.schedule, JSON.stringify(v)).then(res => {
              utility.log("api schedule results:"+res.status);
              this.closeProgramPopup();
              // API updateを行った時点でwsから通知が来て再描画
            }).catch(err => {
              utility.err(`request invalidate schedule error: ${err}`);
            });
            return;
          }
        }
        utility.err("schedule not found ("+id+")")
      },

      pgStop(id) {
        for (const v of this.schedule) {
          if (id == v.id) {
            axios.delete(this.api.schedule+'/stop/'+v.id).then(res => {
              utility.log("api schedule results:"+res.status);
              this.closeProgramPopup();
              // API updateを行った時点でwsから通知が来て再描画
            }).catch(err => {
              utility.err(`request stop schedule error: ${err}`);
            });
            return;
          }
        }
        utility.err("schedule not found ("+id+")")
      },

      tvszone() {
        return [4, 7, 12, 15, 18, 22]
      },

      setBroadcast(v) {
        this.bcast = '' + v;
      },

      onclicktsb(v) {
        if (v === ''+label.TSP_TS || v === ''+label.TSP_TSALL) {
          this.bcast = v === ''+label.TSP_TS ? ''+label.TSP_TSALL : ''+label.TSP_TS;
        } else if (v === ''+label.TSP_BS || v === ''+label.TSP_BSALL) {
          this.bcast = v === ''+label.TSP_BS ? ''+label.TSP_BSALL : ''+label.TSP_BS;
        } else {
          this.bcast = v
        }
      }

    },

    created() {
      // inner html側からの呼び出し用関数
      window.tvpWeekly = this.tvpWeekly;
      window.tvpWeeklyReverse = this.tvpWeeklyReverse;
      window.changeHours = this.changeHours;
      window.pgClick = this.pgClick;
      window.pgReserve = this.pgReserve;
      window.pgSearch = this.pgSearch;
      window.pgDelete = this.pgDelete;
      window.pgInvalid = this.pgInvalid;
      window.pgStop = this.pgStop;
      this.changeHours(0);
    },

    mounted() {
      window.addEventListener('scroll', this.onPopupScroll, true);
      window.addEventListener('scroll', this.onTimeTableScroll, true);
      window.addEventListener('scrollend', this.onTimeTableScrollEnd, true);
      window.addEventListener('resize', this.closeProgramPopup);
      window.addEventListener('resize', this.syncStationHeaderLayout);
      if (typeof ResizeObserver !== 'undefined') {
        this.syncStationHeaderResizeObserver = new ResizeObserver(this.syncStationHeaderLayout);
      }
      this.$nextTick(this.syncStationHeaderLayout);
    },

    beforeDestroy() {
      this.programRenderDeferred = false;
      this.closeProgramPopup();
      for (const name of ['tvpWeekly', 'tvpWeeklyReverse', 'changeHours', 'pgClick',
        'pgReserve', 'pgSearch', 'pgDelete', 'pgInvalid', 'pgStop']) {
        if (window[name] === this[name]) delete window[name];
      }
      clearTimeout(this.nextRefresh);
      if (this.programScrollFrame !== null) cancelAnimationFrame(this.programScrollFrame);
      window.removeEventListener('scroll', this.onTimeTableScroll, true);
      window.removeEventListener('scrollend', this.onTimeTableScrollEnd, true);
      window.removeEventListener('scroll', this.onPopupScroll, true);
      window.removeEventListener('resize', this.closeProgramPopup);
      window.removeEventListener('resize', this.syncStationHeaderLayout);
      if (this.syncStationHeaderResizeObserver) {
        this.syncStationHeaderResizeObserver.disconnect();
        this.syncStationHeaderResizeObserver = null;
      }
    }
}
</script>

<style scoped>
  .tt-side-layout {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    height: calc(100vh - 64px);
    overflow: hidden;
  }

  .tt-side-layout .tt-main-card {
    flex: 1 1 0;
    min-width: 0;
    height: 100%;
    display: flex;
    flex-direction: column;
  }

  .tt-side-toolbar {
    display: flex;
    flex: 0 0 auto;
    flex-wrap: nowrap;
    align-items: center;
    height: auto;
    gap: 8px;
  }

  .tt-side-toolbar .timetable-toolbar {
    display: contents;
  }

  .tt-side-toolbar .toolbar-date-hours {
    flex: 0 0 auto;
    width: auto;
    max-width: none;
    align-items: center;
    white-space: nowrap;
  }

  .tt-side-toolbar .toolbar-date-hours p {
    margin-bottom: 0;
  }

  .tt-side-toolbar .toolbar-pagination {
    flex: 1 1 auto;
    width: auto;
    max-width: none;
    align-items: flex-end !important;
  }

  .tt-side-toolbar .toolbar-pagination .pagination-wrap {
    margin-left: auto !important;
  }

  .tt-side-layout .tt-split-scroll {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .tt-side-layout .tt-station-header-wrap {
    flex: 0 0 auto;
  }

  .tt-side-layout .tt-scroll-body {
    flex: 1 1 auto;
    min-height: 0;
    max-height: none;
    overflow-y: auto;
  }

  .tt-side-selection {
    flex: 0 0 104px;
    display: flex;
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
    padding: 4px;
  }

  .tt-side-selection .toolbar-toggle,
  .tt-side-hours {
    display: flex;
    flex-direction: column;
    flex-wrap: nowrap;
  }

  .tt-side-selection .v-btn {
    width: 100% !important;
    min-width: 0 !important;
    height: clamp(18px, calc((100vh - 116px) / 24), 28px) !important;
    min-height: 0;
    padding: 0 4px !important;
    flex: 0 0 auto;
  }

  .tt-side-selection .toolbar-toggle > .v-btn + .v-btn {
    border-left-width: thin;
    border-top-width: 0;
  }

  .tt-side-hours > .theme--light.v-btn {
    color: rgba(0, 0, 0, 0.54);
    border-color: rgba(0, 0, 0, 0.12);
  }

  .tt-side-hours > .theme--dark.v-btn {
    color: var(--app-text, #fff);
    border-color: rgba(255, 255, 255, 0.12);
  }

  .tt-side-hours > .v-btn + .v-btn {
    border-top-width: 0;
  }

  .fb-btn.v-btn--outlined {
    border: thin solid #CCCCCC;
  }

  .chgh-btn {
    border: thin solid #CCCCCC;
  }

  .toolbar-group {
    min-width: 0;
  }

  .toolbar-date-hours {
    align-items: center;
    gap: 6px;
  }

  .date-label-short {
    display: none;
  }

  .toolbar-current-date {
    margin-bottom: 0;
    white-space: nowrap;
  }

  .toolbar-toggle {
    max-width: 100%;
    flex-wrap: wrap;
    overflow: visible;
  }

  .toolbar-broadcast {
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
    margin-left: 0;
  }

  .toolbar-broadcast .toolbar-toggle {
    flex: 0 0 auto;
    flex-wrap: nowrap;
  }

  .toolbar-broadcast .v-btn {
    margin-left: 0;
  }

  .broadcast-label {
    display: inline-block;
    max-width: 100%;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .toolbar-nav-buttons {
    gap: 16px;
  }

  .pagination-wrap {
    max-width: 100%;
    overflow: visible;
  }

  .tt-split-scroll {
    width: 100%;
  }

  .tt-station-header-wrap {
    overflow: hidden;
    overflow-x: hidden;
  }

  .tt-station-header {
    border-bottom: 0;
  }

  .tt-scroll-body {
    overflow-anchor: none;
    scrollbar-gutter: stable;
    max-height: var(--tt-scroll-height, calc(100vh - 224px));
    overflow-y: auto;
    overflow-x: hidden;
  }

  .tt-scroll-body .tt1 {
    border-top: 0;
  }

  .tt-scroll-body::-webkit-scrollbar {
    width: 16px;
  }

  .tt-scroll-body::-webkit-scrollbar-thumb {
    min-height: 64px;
    border: 3px solid transparent;
    border-radius: 8px;
    background-color: rgba(128, 128, 144, 0.65);
    background-clip: padding-box;
  }

  .tt-scroll-body::-webkit-scrollbar-thumb:hover {
    background-color: rgba(128, 128, 144, 0.9);
  }

  @media (max-width: 1263px) {
    .date-label-full {
      display: none;
    }

    .date-label-short {
      display: inline;
    }

    .toolbar-dates > .v-btn {
      min-width: 44px;
      padding: 0 6px;
    }

    .toolbar-current-date {
      font-size: 1rem;
      line-height: 1.4;
    }

    .toolbar-current-date ::v-deep small {
      display: none;
    }

    .tt-toolbar:not(.tt-side-toolbar) {
      display: flex;
      flex-wrap: wrap;
      align-content: flex-start;
      gap: 8px;
    }

    .tt-toolbar:not(.tt-side-toolbar) .timetable-toolbar,
    .tt-toolbar:not(.tt-side-toolbar) .toolbar-broadcast {
      display: contents !important;
    }

    .tt-toolbar:not(.tt-side-toolbar) .toolbar-group {
      flex: 0 1 auto;
      width: auto;
      max-width: 100%;
      padding: 0;
    }

    .tt-toolbar:not(.tt-side-toolbar) .toolbar-toggle > .v-btn {
      height: 36px;
      min-width: 32px;
      padding: 0 8px;
    }

    .tt-toolbar:not(.tt-side-toolbar) .toolbar-broadcast-types > .v-btn {
      width: auto !important;
      min-width: 44px;
      padding: 0 6px;
    }

    .tt-toolbar:not(.tt-side-toolbar) .toolbar-dates > .v-btn {
      min-width: 44px;
      padding: 0 6px;
    }

    .tt-toolbar:not(.tt-side-toolbar) .toolbar-pagination {
      flex: 1 1 auto;
    }

  }

  .tt-program-popup p.ptt a {
    color: inherit;
    text-decoration: underline;
  }

  .tt-program-popup p.ptt a:hover {
    opacity: 0.8;
  }

  .tt-program-popup p.ptt .tt-title-icon-link {
    text-decoration: none;
    margin-left: 6px;
    display: inline-block;
  }
</style>
