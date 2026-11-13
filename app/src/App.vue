<template>
  <v-app>
    <v-app-bar app color="appcolor" class="main-tab-bar" dark>
      <div class="d-flex align-center">
        <v-img
          alt="Vuetify Logo"
          :class="['shrink', 'mr-2', powerManagerEnabled ? 'app-logo-clickable' : '']"
          contain
          :src="appLogoSrc"
          transition="scale-transition"
          width="40"
          @click="toggleSleepTimerPause"
        />
        <span class="brand hidden-sm-and-down">TvRockGO</span>

      </div>

      <DigitalClock
      class="main-tab-clock"
      :blink='digitalclock.blink'
      :displaySeconds='digitalclock.displaySeconds'
      :displayDelay='digitalclock.displayDelay'
      :twelveHour='digitalclock.twelveHour'
      :twentyEight='digitalclock.twentyEight'>
      </DigitalClock>

      <v-spacer></v-spacer>

      <v-card flat color="transparent" class="main-tab-navigation">
        <v-tabs class="main-tabs" v-model="maintab" slider-color="white" background-color="transparent" fixed-tabs center-active>

          <v-tab class="align-self-center pa-md-8" :key="'timetable'" aria-label="TimeTable" title="TimeTable">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                mdi-table
              </v-icon>
              <div class="text-caption">TimeTable</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'schedule'"
            :aria-label="scheduleByDevice ? 'Schedule（デバイス別）' : 'Schedule（日時順）'"
            :title="scheduleByDevice ? 'Schedule（再クリックで日時順）' : 'Schedule（再クリックでデバイス別）'"
            @click="toggleScheduleGrouping">
            <v-layout align-center justify-center column fill-height>
            <v-icon aria-hidden="false">
              mdi-clock-outline
            </v-icon>
            <div class="text-caption">Schedule</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'search'" aria-label="Search" title="Search">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                mdi-archive-search-outline
              </v-icon>
              <div class="text-caption">Search</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'keyword'" aria-label="Keyword" title="Keyword">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                <!-- mdi-feature-search -->
                mdi-key-variant
              </v-icon>
              <div class="text-caption">Keyword</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'device'" aria-label="Device" title="Device">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                mdi-console-network-outline
              </v-icon>
              <div class="text-caption">Device</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'setting'" aria-label="Setting" title="Setting">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                mdi-cog
              </v-icon>
              <div class="text-caption">Setting</div>
            </v-layout>
          </v-tab>

          <v-tab class="align-self-center mr-4" :key="'test'" v-if="label.MPEG2TS" aria-label="Test" title="Test">
            <v-layout align-center justify-center column fill-height>
              <v-icon aria-hidden="false">
                mdi-play-circle-outline
              </v-icon>
              <div class="text-caption">Test</div>
            </v-layout>
          </v-tab>

        </v-tabs>
      </v-card>

      <v-btn icon @click.stop="sidenavi = !sidenavi">
        <v-icon>mdi-dots-vertical</v-icon>
      </v-btn>

    </v-app-bar>

    <v-main :style="{ '--app-content-top': $vuetify.application.top + 'px' }">

      <v-card class="main-tab-card">

        <div class="tab-split-layout" :class="{ 'tab-split-layout--contained': maintab === 4 || maintab === 5 }">

        <v-tabs-items v-model="maintab" class="tab-main-content">

          <v-tab-item :key="'timetable'" :transition="false">
            <TimeTable
            ref="timetable"
            :api = 'api'
            :timetable = 'timetable'
            :station = 'station'
            :options = 'options'
            :schedule = 'schedule'
            @redrawtt = 'redrawTT'
            @extendtt = 'extendTimeTable'
            @timechange = 'options.current = $event'
            @programsearch = 'searchTimeTableProgram'
            />
          </v-tab-item>
        
          <v-tab-item :key="'schedule'" :transition="false">
            <RecordingSchedule
            :group-by-device="scheduleByDevice"
            :recording-status-revision="recordingStatusRevision"
            :api = 'api'
            :station = 'station'
            :schedule = 'schedule'
            :deviceList = 'deviceList'
            :configure = 'configure'
            @redrawtt = 'redrawTT'
            @programsearch = 'searchTimeTableProgram'
            @select = 'selectProgram'
            />
          </v-tab-item>

          <v-tab-item :key="'search'" :transition="false">
            <ProgramSearch
            :UID = 'UID'
            :api = 'api'
            :station = 'station'
            :searchResults = 'searchResults'
            :searchProgress = 'searchProgress'
            :schedule = 'schedule'
            :keywordSearchCondition = 'keywordSearchCondition'
            :deviceList = 'deviceList'
            :options = 'options'
            @searchstart = 'beginSearchResults'
            @searcherror = 'failSearchResults'
            :refreshToken = 'keywordReserveRevision'
            :configure = 'configure'
            @keywordreserveadd = 'updateKeywordReserveRevision'
            @select = 'selectProgram'
            />
          </v-tab-item>

          <v-tab-item :key="'keyword'" :transition="false">
            <KeywordReserve
            :api = 'api'
            :schedule = 'schedule'
            :station = 'station'
            :deviceList = 'deviceList'
            @searchstart = 'beginSearchResults'
            @searcherror = 'failSearchResults'
            :refreshToken = 'keywordReserveRevision'
            :configure = 'configure'
            @select = 'searchKeywordReserve'
            @keywordreservedelete = 'updateSchedule'
            />
          </v-tab-item>

          <v-tab-item :key="'device'" class="device-scroll-tab" :transition="false">
            <DeviceController
            :api = 'api'
            :socket = 'socket'
            :device = 'device'
            :captions = 'captions'
            :bml = 'bml'
            :station = 'station'
            :schedule = 'schedule'
            :options = 'options'
            @redrawtt = 'redrawTT'
            />
          </v-tab-item>

          <v-tab-item :key="'setting'" class="settings-tab" :transition="false">
            <v-card flat class="settings-tab-card">
              <SystemSetting
              :options = 'options'
              :station = 'station'
              :api = 'api'
              :configure = 'configure'
              :schedule = 'schedule'
              :storageInfo = 'storageInfo'
              :recorderSettingRevision = 'recorderSettingRevision'
              :powerManagerSettingRevision = 'powerManagerSettingRevision'
              @redrawtt = 'redrawTT'
              @optionchange = 'applyDisplayOptions'
              @powermanagerchange = 'handlePowerManagerSettingChange'
              @select = 'selectProgram'
              />
            </v-card>
          </v-tab-item>
          
          <v-tab-item v-if="label.MPEG2TS" :key="'test'" :transition="false">
            <v-card flat class="pa-4">
              <div class="text-h6 mb-4">mpegts.js Test Player</div>
              <div class="d-flex justify-start mb-4">
                <v-btn color="primary" @click="startPlayback" class="mr-2">Play</v-btn>
                <v-btn color="error" @click="stopPlayback">Stop</v-btn>
              </div>
              <video ref="videoElement" controls autoplay style="width: 100%; max-width: 800px; background-color: black;"></video>
            </v-card>
          </v-tab-item>

        </v-tabs-items>

        <v-expand-x-transition>
          <div v-show="sidenavi" class="tab-side-navigation">
            <SideNavigation
            :api = 'api'
            :schedule = 'schedule'
            :timetable = 'toptimetable'
            :feature = 'feature'
            :station = 'station'
            :options = 'options'
            @programsearch = 'searchTimeTableProgram'
            @bcastchange = 'changeTimeTableBroadcast'
            @hourchange = 'hourChange'
            @select = 'selectProgram'
            />
          </div>
        </v-expand-x-transition>

        </div>

      </v-card>

    </v-main>

  </v-app>
</template>

<script>
import label from './define.js';
import utility from './utility.js';
import { getServiceDisplayCount } from './timeTableRenderer';
import TimeTable from './components/TimeTable';
import RecordingSchedule from './components/RecordingSchedule';
import ProgramSearch from './components/ProgramSearch';
import KeywordReserve from './components/KeywordReserve';
import DeviceController from './components/DeviceController';
import DigitalClock from './components/DigitalClock';
import SystemSetting from './components/SystemSetting';
import SideNavigation from './components/SideNavigation';
import axios from 'axios';
import clientIdentity from './clientIdentity';
import CaptionPlayback from './captionPlayback';
import SystemTheme from './systemTheme';
import { normalizeGenreColors } from './timeTableGenreColors';
import UIRefreshQueue from './uiRefreshQueue';
import SearchResultStream from './searchResults';

const appLogo = require('./assets/tvrockgo.png');
const appSleepLogo = require('./assets/tvrockgo_sleep.png');

const getHTTPBaseURL = () => {
  const loc = window.location;
  return loc.origin || `${loc.protocol}//${loc.host}`;
}

const getWSBaseURL = () => {
  const loc = window.location;
  const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${loc.host}`;
}

const countServices = (ar) => {
  let n = 0;
  Object.keys(ar).forEach((i) => {
    let v = ar[i]
    if (v.View>0)
      n++;
  });
  return n;
}

const isrecp = (p, sch, thr) => {
  if (p.Start > thr) { // 過去データで被る場合があるのでここで消す
    for (let i = 0; i < (sch || []).length; i++) {
      let s = sch[i];
      if (s.validate && s.sid == p.Sid && s.eid == p.Eid) {
        return {id: s.id, devID: s.deviceid, searchID: s.searchid, isRec: true, comp: s.complete, isWatch: !!s.watchonly && !s.recordonly}
      }
    }
  }
  return {id: 0, devID: '', searchID: 0, isRec: false}
}

const enrichProgramData = (program, schedule, threshold) => {
  program.StartM = parseInt(program.Start / label.SEC);
  program.EndM = parseInt(program.End / label.SEC);
  program.DurationM = program.EndM - program.StartM;
  program.StartD = new Date(program.Start * label.MSEC);
  program.EndD = new Date(program.End * label.MSEC);
  const rc = isrecp(program, schedule, threshold);
  program.ID = rc.id;
  program.IsRec = rc.isRec;
  program.IsWatch = !!rc.isWatch;
  program.DeviceID = rc.devID;
  program.Complete = rc.comp;
  program.SearchID = rc.searchID;
}

const addMoreProp = (dt, sch) => {
  const thr = utility.now().getTime() / label.MSEC - label.WEEK * 24 * 60 * 60;
  dt.services = [];
  if (dt.Week) {
    dt.services[0] = Object.keys(dt.Sta)[0];
    for (let j = 0; j < label.WEEK; j++) {
      let list = dt.Tvp[j] || [];
      list.forEach(p => enrichProgramData(p, sch, thr));
    }
  } else {
    dt.services = Object.keys(dt.Sta);
    dt.services.forEach(key => (dt.Tvp[key] || []).forEach(p => enrichProgramData(p, sch, thr)));
  }
  return dt;
}

const addMoreSchProp = (sch, sta, te) => {
  for (let i = 0; i < sch.length; i++) {
    let s = sch[i];
    sch[i].ServiceName = s.sid in sta.all ? sta.all[s.sid].Name : "unknown";
    sch[i].DateW = utility.uxtmtoymdhmj(s.starttime, te) + '～' + utility.uxtmtohm(s.endtime, te)
    sch[i].DateMD = utility.uxtmtomd(s.starttime, te);
    sch[i].DateHM = utility.uxtmtohm(s.starttime, te) + '～' + utility.uxtmtohm(s.endtime, te)
    sch[i].StartD = new Date(s.starttime * label.MSEC);
    sch[i].EndD = new Date(s.endtime * label.MSEC);
  }
}

const addMoreDevProp = (dev, st) => {
  let sa = svtoselarry(dev.TunerType === 0 ? {...st.tsall, ...st.os} : {...st.bsall, ...st.cs}, dev.CurrentService);
  dev.services = sa.services;
  dev.current = sa.current
}

const svtoselarry = (v, sid) => {
  let ret = [];
  let cur = {
    Name: '',
    Sid: 0,
  };
  Object.keys(v).forEach(no => {
    let s = v[no];
    if (s.View > 0) {
      if (s.Sid === sid) {
        cur = {
          Name: s.Name + ' (' + (s.Sid & 0xffff) + ')',
          Sid: s.Sid,
        }
      }
      ret.push({
        Name: s.Name + ' (' + (s.Sid & 0xffff) + ')',
        Sid: s.Sid,
      });
      ret.push({
        divider: true
      });
    }
  });
  return {
    services: ret,
    current: cur
  };
}

const createStationIns = () => {
  return {
    viewAll: 0,
    viewTS: 0,
    viewBS: 0,
    viewCS: 0,
    viewOS: 0,
    viewTSAll: 0,
    viewBSAll: 0,
    all: {},
    ts: {},
    bs: {},
    cs: {},
    os: {},
    bscs: {},
    tsall: {},
    bsall: {},
    csall: {},
  };
}

const createDefaultOptions = () => {
  let d = utility.now();
  return {
    current: new Date(d.getFullYear(),d.getMonth(),d.getDate(),d.getHours(),0),
    hours: 4,
    page: [0,0,0,0,0,0,0,0],
    broadcast: 1,
    dispservice: 0,
    displogo: true,
    weekly: false,
    guidetype: 0,
    subtitlemin: 30,
    contentmin: 60,
    divider: 3,
    twentyEight: true,
    stationsplitscroll: true, // 旧設定との互換用。局名行は常に固定する。
    sideselection: false,
    captionon: true,
    bmlon: false,
    captiontimetable: true,
    devconpin: {},
    autopin: false,
    forcetheme: 0,
    genreColors: normalizeGenreColors(),
    watchbutton: false,
  };
}

const optionsCookieName = 'tvrockgo_options';
const optionsCookieMaxAge = 60 * 60 * 24 * 365;
const optionCookieKeys = [
  'current',
  'hours',
  'page',
  'broadcast',
  'dispservice',
  'displogo',
  'weekly',
  'guidetype',
  'subtitlemin',
  'contentmin',
  'divider',
  'twentyEight',
  'stationsplitscroll',
  'sideselection',
  'captionon',
  'bmlon',
  'captiontimetable',
  'devconpin',
  'autopin',
  'forcetheme',
  'genreColors',
  'watchbutton',
];

const filterOptionsForCookie = (options) => {
  let filtered = {};
  optionCookieKeys.forEach(key => {
    filtered[key] = options[key];
  });
  return filtered;
}

const loadOptionsFromCookie = () => {
  let options = createDefaultOptions();
  let cookie = document.cookie
    .split('; ')
    .find(row => row.startsWith(optionsCookieName + '='));
  if (!cookie) {
    return options;
  }

  try {
    let saved = JSON.parse(decodeURIComponent(cookie.substring(optionsCookieName.length + 1)));
    let d = utility.now();
    options = Object.assign(options, saved);
    options.genreColors = normalizeGenreColors(options.genreColors);
    if (![2, 4, 8].includes(options.hours)) options.hours = 4;
    options.current = new Date(d.getFullYear(),d.getMonth(),d.getDate(),d.getHours(),0);
//    options.current = saved.current ? new Date(saved.current) : options.current;
    if (!(options.current instanceof Date) || isNaN(options.current.getTime())) {
      options.current = createDefaultOptions().current;
    }
    if (!Array.isArray(options.page)) {
      options.page = createDefaultOptions().page;
    }
  } catch (e) {
    utility.err(`load options cookie error: ${e}`);
  }
  return options;
}

const saveOptionsToCookie = (options) => {
  document.cookie = optionsCookieName + '=' +
    encodeURIComponent(JSON.stringify(filterOptionsForCookie(options))) +
    '; max-age=' + optionsCookieMaxAge +
    '; path=/; SameSite=Lax';
}

export default {
  name: 'App',

  components: {
    TimeTable,
    RecordingSchedule,
    ProgramSearch,
    KeywordReserve,
    DeviceController,
    SystemSetting,
    DigitalClock,
    SideNavigation
  },

  data() {
    return {
      label: null,
      utility: null,
      UID: '',
      host: getHTTPBaseURL(),
      api: {
        caption: '',
        configure: '',
        timetable: '',
        feature: '',
        station:  '',
        schedule: '',
        recordingStatus: '',
        control: '',
        device: '',
        search: '',
        keyword: '',
        fetchProgramSchedule: '',
        recorderSetting: '',
        playback: '',
        powerManagerSetting: '',
        log: '',
      },
      maintab: 0,
      scheduleByDevice: false,
      socket: null,
      socketReconnectTimer: null,
      socketReconnectDelay: 1000,
      socketMaxReconnectDelay: 30000,
      socketClosing: false,
      powerManagerSettingLoading: false,
      
      timetable: null,
      toptimetable: null,
      timetableRequestId: 0,
      topTimetableRequestId: 0,
      feature: null,
      sidenavi: false,
      befBC: -1,

      configure: null,

      station: createStationIns(),

      options: loadOptionsFromCookie(),

      schedule: null,
      recordingStatusRevision: 0,

      storageInfo: null,

      device: [],
      captions: {},
      bml: {},
      captionTimers: {},
      captionPlayback: {},
      deviceList: [],

      searchResults: [],
      searchProgress: {
        running: false,
        percent: 0,
        scanned: 0,
        hits: 0,
      },
      keywordReserveRevision: 0,
      recorderSettingRevision: 0,
      powerManagerSettingRevision: 0,
      keywordSearchCondition: null,

      // clock
      digitalclock: {
        blink: false,
        displaySeconds: true,
        displayDelay: true,
        twelveHour: false,
        twentyEight: true,
      },
      player: null,

    }
  },

  computed: {
    powerManagerSetting() {
      if (!this.configure || !this.configure.PowerManagerSetting) {
        return null;
      }
      return this.configure.PowerManagerSetting;
    },

    powerManagerEnabled() {
      return !!(this.powerManagerSetting && this.powerManagerSetting.powermanager);
    },

    appLogoSrc() {
      if (!this.powerManagerEnabled) {
        return appLogo;
      }
      return this.powerManagerSetting.sleeptimerpause ? appLogo : appSleepLogo;
    },
  },

  methods: {
    beginSearchResults(requestId = null) {
      this._searchRequestID = requestId;
      if (this._searchResultStream) this._searchResultStream.dispose();
      this.searchResults = Object.freeze([]);
      this.searchProgress = { running: true, percent: 0, scanned: 0, hits: 0 };
      this._searchResultStream = SearchResultStream.create({
        publish: results => { this.searchResults = results; },
        complete: hits => {
          this.searchProgress = { ...this.searchProgress, running: false, percent: 100, hits };
        },
      });
    },

    failSearchResults(requestId) {
      if (this._searchRequestID !== requestId) return;
      if (this._searchResultStream) this._searchResultStream.dispose();
      this.searchProgress = { ...this.searchProgress, running: false, warning: '検索要求に失敗しました。再度検索してください。' };
    },

    requestUiRefresh(...flags) {
      if (this._isBeingDestroyed || this._isDestroyed) return;
      if (!this._uiRefreshQueue) {
        this._uiRefreshQueue = UIRefreshQueue.create({
          run: async pending => {
            if (this._initialDataReady) await this._initialDataReady;
            if (this._isBeingDestroyed || this._isDestroyed) return;
            const stationChanged = pending.has('station');
            const guideChanged = stationChanged || pending.has('guide');
            if (stationChanged) await this.updateService();
            if (stationChanged || pending.has('schedule')) {
              await this.updateSchedule();
              if (this._isBeingDestroyed || this._isDestroyed) return;
              this.recordingStatusRevision++;
            }
            if (guideChanged) {
              const result = await this.updateTvProgram(false, true);
              if (result === 'deferred') this.requestUiRefresh('guide');
              this._sidebarDirty = true;
            }
            if (this._isBeingDestroyed || this._isDestroyed) return;
            if (this.sidenavi && (this._sidebarDirty || pending.has('sidebar'))) {
              this._sidebarDirty = false;
              await Promise.all([this.updateTopTvProgram(), this.updateTvFeature()]);
            }
          },
          onError: err => utility.err(`background refresh error: ${err}`),
        });
      }
      this._uiRefreshQueue.request(...flags);
    },

    patchReservationViews() {
      if (this._isBeingDestroyed || this._isDestroyed) return;
      if (this._foregroundTableLoads > 0) {
        this._reservationPatchPending = true;
        return;
      }
      this._reservationPatchPending = false;
      const index = UIRefreshQueue.reservationIndex(this.schedule);
      const threshold = utility.now().getTime() / label.MSEC - label.WEEK * 86400;
      this.timetable = UIRefreshQueue.patchTable(this.timetable, index, threshold);
      this.toptimetable = UIRefreshQueue.patchTable(this.toptimetable, index, threshold);
      this.feature = UIRefreshQueue.patchFeatures(this.feature, index, threshold);
    },

    toggleScheduleGrouping() {
      if (this.maintab === 1) this.scheduleByDevice = !this.scheduleByDevice;
    },


    // プログラム更新
    async updateTvProgram(resetRange = false, background = false) {
      if (background && this._foregroundTableLoads > 0) return 'deferred';
      // 自動更新の追加通知では世代を進めない。操作による取得だけが古い応答を無効化する。
      const requestId = background ? this.timetableRequestId : ++this.timetableRequestId;
      if (!background) this._foregroundTableLoads = (this._foregroundTableLoads || 0) + 1;
      try {
        let sord = {
          0: this.station.all,
          1: this.station.ts,
          2: this.station.bs,
          3: this.station.cs,
          4: this.station.os,
          5: this.station.all,
          6: this.station.tsall,
          7: this.station.bsall
        };
        let sview = {
          0: this.station.viewAll,
          1: this.station.viewTS,
          2: this.station.viewBS,
          3: this.station.viewCS,
          4: this.station.viewOS,
          5: this.station.viewAll,
          6: this.station.viewTSAll,
          7: this.station.viewBSAll
        };
        let cur = this.options.current;
        let par = {
          services: [],
          start: utility.tmtoux(cur) - 3600,
          hours: 4,
          week: this.options.weekly,
        };
        // 通知による更新では、スクロールしている範囲を再取得する。
        if (!resetRange && this.timetable && Boolean(this.timetable.Week) === Boolean(par.week)) {
          par.start = this.timetable.Start;
          par.hours = (this.timetable.End - this.timetable.Start) / 3600;
        }
        let bc = this.options.broadcast;
        if (bc < label.TSP_ALL || bc > label.TSP_BSALL) {
          this.options.broadcast = bc = label.TSP_TS;
        }

        let sd = sord[bc];
        let vn = sview[bc];
        if (sd && vn > 0) {
          const table = this.$refs && this.$refs.timetable;
          const vs = par.week ? 1 : getServiceDisplayCount(this.options, table ? table.serviceDisplayWidth : 0);
          const visibleServices = Object.values(sd).filter(service => service.View > 0);
          if (this.options.dispservice === 0 && !par.week) {
            const offset = this.options.page[bc] || 0;
            this.options.page[bc] = Math.min(Math.floor(offset / vs), Math.ceil(vn / vs) - 1) * vs;
          }
          for(let i = 0; i < vs; i++) {
            let j = ((this.options.page[bc] || 0) + i) % vn;
            par.services.push(visibleServices[j].Sid);
          }
        }

        if (par.services.length === 0) {
          this.timetable = null;
          return;
        }

        // 番組表読み込み
        await axios.put(this.api.timetable, par).then(res => {
          if (requestId !== this.timetableRequestId || this._isBeingDestroyed || this._isDestroyed) return;
          utility.log("tvprogram status: "+res.status);
          addMoreProp(res.data, this.schedule);
          this.timetable = res.data;
          utility.setServerDate(res.data.CurrentMilliseconds);
        }).catch(err => {
          utility.err(`request tvprogram error: ${err}`);
        });

        // サイドバー用
        if (requestId !== this.timetableRequestId || this._isBeingDestroyed || this._isDestroyed) return;
        if (this.befBC !== bc) {
          this.updateTopTvProgram()
          this.befBC = bc;
        }
      } finally {
        if (!background) {
          this._foregroundTableLoads--;
          if (this._reservationPatchPending) this.patchReservationViews();
        }
      }
    },

    async extendTimeTable({ direction, maxHours = 12, done }) {
      const previous = this.timetable;
      if (!previous) { done(); return; }
      const requestId = ++this.timetableRequestId;
      this._foregroundTableLoads = (this._foregroundTableLoads || 0) + 1;
      let applied = false;
      try {
        const response = await axios.put(this.api.timetable, {
          // servicesはObject.keys由来の文字列。Goの[]uintへ送る前に数値に戻す。
          services: (previous.Week ? previous.services : previous.ServiceList).map(Number),
          start: direction < 0 ? previous.Start - 14400 : previous.End,
          hours: 4,
          week: Boolean(previous.Week),
        });
        if (requestId !== this.timetableRequestId || this.timetable !== previous ||
            this._isBeingDestroyed || this._isDestroyed) return;
        const extra = response.data;
        const retainedSeconds = Math.min(24, Math.max(12, maxHours)) * 3600;
        const start = direction < 0 ? extra.Start : Math.max(previous.Start, extra.End - retainedSeconds);
        const end = direction > 0 ? extra.End : Math.min(previous.End, extra.Start + retainedSeconds);
        const merged = { ...previous, Start: start, End: end, Tvp: {} };
        const columns = previous.Week ? Array.from({ length: label.WEEK }, (_, i) => i) : previous.ServiceList;
        for (const sid of columns) {
          const offset = previous.Week ? sid * 86400 : 0;
          const programs = new Map();
          for (const program of [...(previous.Tvp[sid] || []), ...(extra.Tvp[sid] || [])]) {
            if (program.Start < end + offset && program.End > start + offset) {
              programs.set(program.Sid + '_' + program.Eid + '_' + program.Start, program);
            }
          }
          merged.Tvp[sid] = [...programs.values()].sort((a, b) => a.Start - b.Start);
        }
        addMoreProp(merged, this.schedule);
        this.timetable = merged;
        applied = true;
      } catch (err) {
        utility.err(`request extended tvprogram error: ${err}`);
      } finally {
        this._foregroundTableLoads--;
        if (this._reservationPatchPending) this.patchReservationViews();
        done(applied);
      }
    },

    async updateTopTvProgram() {
      const requestId = ++this.topTimetableRequestId;
      let sord = {
        0: this.station.all,
        1: this.station.ts,
        2: this.station.bs,
        3: this.station.cs,
        4: this.station.os,
        5: this.station.all,
        6: this.station.tsall,
        7: this.station.bsall
      };
      let sview = {
        0: this.station.viewAll,
        1: this.station.viewTS,
        2: this.station.viewBS,
        3: this.station.viewCS,
        4: this.station.viewOS,
        5: this.station.viewAll,
        6: this.station.viewTSAll,
        7: this.station.viewBSAll
      };
      let d = utility.now();
      let cur = new Date(d.getFullYear(),d.getMonth(),d.getDate(),d.getHours(),0);
      let par = {
        services: [],
        start: utility.tmtoux(cur),
        hours: 3,
        week: false,
      };
      let bc = this.options.broadcast;
      if (bc < label.TSP_ALL || bc > label.TSP_BSALL) {
        this.options.broadcast = bc = label.TSP_TS;
      }

      let sd = sord[bc];
      let vn = sview[bc];
      if (sd && vn > 0) {
        Object.keys(sd).forEach(function(no) {
          let v = sd[no];
          if (v.View > 0) {
              par.services.push(v.Sid);
          }
        });
      }

      if (par.services.length === 0) {
        this.toptimetable = null;
        return;
      }

      // 番組表読み込み
      await axios.put(this.api.timetable, par).then(res => {
        if (requestId !== this.topTimetableRequestId || this._isBeingDestroyed || this._isDestroyed) return;
        utility.log("tvprogram status: "+res.status);
        addMoreProp(res.data, this.schedule);
        this.toptimetable = res.data;
      }).catch(err => {
        utility.err(`request toptvprogram error: ${err}`);
      });
    },

    async updateTvFeature() {
      await axios.get(this.api.feature).then(res => {
        if (this._isBeingDestroyed || this._isDestroyed) return;
        utility.log("feature status: "+res.status);
        const thr = utility.now().getTime() / label.MSEC - label.WEEK * 24 * 60 * 60;
        for (const fet of res.data) {
          for (let key in fet) { 
            let tvp = fet[key]
            for (const prg of tvp) {
              enrichProgramData(prg, this.schedule, thr);
            }
          }
        }
        this.feature = res.data;
      }).catch(err => {
        utility.err(`request toptvprogram error: ${err}`);
      });
    },

    // サービス更新
    async updateService() {
      await axios.get(this.api.station).then(res => {
        utility.log("station status: "+res.status);

        let t = createStationIns();
        for (let key in res.data) { 
          let val = res.data[key];
          val.OrgSid = val.Sid & 0xffff;
          t.all[key] = val;
          switch (val.Tsp) {
            case label.TSP_TS:
              if (val.Parent === 0) {
                t.ts[key] = val;
              }
              t.tsall[key] = val;
              break;
            case label.TSP_BS:
              if (val.Parent === 0) {
                t.bs[key] = val; 
              }
              t.bsall[key] = val; 
              t.bscs[key] = val;
              break;
            case label.TSP_CS: t.cs[key] = val; t.bscs[key] = val; break;
            case label.TSP_OS: t.os[key] = val; break;
          }
        }

        t.viewAll = countServices(t.all);
        t.viewTS = countServices(t.ts);
        t.viewBS = countServices(t.bs);
        t.viewCS = countServices(t.cs);
        t.viewOS = countServices(t.os);
        t.viewTSAll = countServices(t.tsall);
        t.viewBSAll = countServices(t.bsall);

        this.station = t;
        this._scheduleSignature = null;

      }).catch(err => {
        utility.err(`request service error: ${err}`);
      });
    },

    // 予約更新
    async updateSchedule() {
      await axios.get(this.api.schedule).then(res => {
        if (this._isBeingDestroyed || this._isDestroyed) return;
        utility.log("schedule status: "+res.status);
        const signature = JSON.stringify(res.data);
        if (signature === this._scheduleSignature) return;
        this._scheduleSignature = signature;
        addMoreSchProp(res.data, this.station, this.options.twentyEight)
        this.schedule = res.data;
        this.patchReservationViews();
      }).catch(err => {
        utility.err(`request schedule error: ${err}`);
      });
    },

    // デバイス更新
    async updateDeviceAll() {
      await axios.get(this.api.device).then(res => {
        utility.log("device status: "+res.status);
        let dat = res.data;
        for (let i = 0; i < dat.length; i++) {
          addMoreDevProp(dat[i], this.station);
        }
        let list = [];
        for (const dev of res.data) {
          list.push({text: dev.ID, value: dev.ID});
        }
        this.device = res.data;
        this.pruneCaptions();
        this.deviceList = list;
      }).catch(err => {
        utility.err(`request device error: ${err}`);
      });
    },

    // デバイス更新
    updateDevice(dat) {
      let dev = [];
      let i;
      let add = false;

      for (i = 0; i < this.device.length; i++) {
        if (this.device[i].ID == dat.ID) {
          addMoreDevProp(dat, this.station);
          dev.push(dat);
          add = true;
        } else {
          dev.push(this.device[i]);
        }
      }
      if (!add) {
          addMoreDevProp(dat, this.station);
          dev.push(dat);
      }

      this.device = dev;
      this.pruneCaptions();
    },

    pruneCaptions() {
      Object.keys(this.bml).forEach(id => {
        const dev = this.device.find(d => d.ID === id);
        if (!dev || dev.CurrentService !== this.bml[id].Sid ||
          (dev.State !== label.DSTAT_READY && dev.State !== label.DSTAT_RECORDING)) this.$delete(this.bml, id);
      });
      const ids = new Set([...Object.keys(this.captions), ...Object.keys(this.captionPlayback)]);
      ids.forEach(id => {
        const dev = this.device.find(item => item.ID === id);
        const caption = this.captions[id] || this.captionPlayback[id].caption;
        if (!dev || !this.captionMatchesDevice(caption, dev)) {
          this.clearCaption(id);
        }
      });
    },

    captionMatchesDevice(caption, dev) {
      const program = dev.CurrentProgram;
      return (dev.State === label.DSTAT_READY || dev.State === label.DSTAT_RECORDING) &&
        program && program.Sid === caption.Sid && program.Eid === caption.Eid &&
        program.Start === caption.Start;
    },

    clearCaption(id) {
      if (this.captionPlayback[id]) {
        this.captionPlayback[id].player.dispose();
        delete this.captionPlayback[id];
      }
      clearTimeout(this.captionTimers[id]);
      delete this.captionTimers[id];
      this.$delete(this.captions, id);
    },

    queueCaption(caption) {
      this.device.forEach(dev => {
        if (!this.captionMatchesDevice(caption, dev)) return;
        let playback = this.captionPlayback[dev.ID];
        if (playback && (!this.captionMatchesDevice(playback.caption, dev) ||
          playback.caption.Pid !== caption.Pid || playback.caption.Lang !== caption.Lang)) {
          this.clearCaption(dev.ID);
          playback = null;
        }
        if (!playback) {
          playback = {
            caption,
            player: new CaptionPlayback(packet => this.updateCaption(packet, dev.ID)),
          };
          this.captionPlayback[dev.ID] = playback;
        }
        playback.player.push(caption);
        // Inactivity is measured from receipt, not delayed presentation.
        clearTimeout(this.captionTimers[dev.ID]);
        this.captionTimers[dev.ID] = setTimeout(() => this.clearCaption(dev.ID), 30000);
      });
    },

    updateCaption(caption, deviceID) {
      this.device.forEach(dev => {
        if (dev.ID !== deviceID || !this.captionMatchesDevice(caption, dev)) return;
        const previous = this.captions[dev.ID];
        let items = !caption.Clear && previous &&
          this.captionMatchesDevice(previous, dev) ? previous.items.slice() : [];
        if (caption.Text) {
          // A new fragment at the same position replaces the previous fragment.
          if (caption.X >= 0 && caption.Y >= 0) {
            items = items.filter(item => {
              if (item.Layout && caption.Layout) {
                return item.Layout.X !== caption.Layout.X || item.Layout.Y !== caption.Layout.Y;
              }
              return item.X !== caption.X || item.Y !== caption.Y;
            });
          }
          items.push(caption);
        }
        this.$set(this.captions, dev.ID, {
          Sid: caption.Sid, Eid: caption.Eid, Start: caption.Start, items,
        });
      });
    },

    // コンフィグレーション
    async updateConfigure() {
      await axios.get(this.api.configure).then(res => {
        utility.log("configure status: "+res.status);
        this.configure = res.data;
      }).catch(err => {
        utility.err(`request configure error: ${err}`);
      });
    },

    async handlePowerManagerSettingChange() {
      this.powerManagerSettingRevision++;
      await this.updateConfigure();
    },

    async toggleSleepTimerPause() {
      if (!this.powerManagerEnabled || !this.api.powerManagerSetting || this.powerManagerSettingLoading) {
        return;
      }

      this.powerManagerSettingLoading = true;
      await axios.put(this.api.powerManagerSetting, {
        sleeptimerpause: !this.powerManagerSetting.sleeptimerpause,
      }).then(res => {
        if (this.configure && res.data) {
          this.configure = Object.assign({}, this.configure, {
            PowerManagerSetting: res.data,
          });
        }
        this.powerManagerSettingRevision++;
      }).catch(err => {
        utility.err(`update power manager sleep timer pause error: ${err}`);
        this.updateConfigure();
      }).finally(() => {
        this.powerManagerSettingLoading = false;
      });
    },

    // リスナー
    handleSocketMessage(ev) {
      let com = ev.data.substr(0,4);
      switch(com) {
        case 'uid:':
          this.UID = ev.data.substr(4);
          break;

        case 'dev:':
          this.updateDevice(JSON.parse(ev.data.substr(4)));
          utility.log("ws: device update");
          break;

        case 'utt:':
          this.requestUiRefresh('guide');
          utility.log("ws: time table update");
          break;

        case 'ust:':
          this.requestUiRefresh('station');
          utility.log("ws: station update");
          break;

        case 'usc:':
          this.requestUiRefresh('schedule');
          utility.log("ws: schedule update");
          break;

        case 'urs:':
          this.recorderSettingRevision++;
          utility.log("ws: recorder setting update");
          break;

        case 'ups:':
          this.handlePowerManagerSettingChange();
          utility.log("ws: power manager setting update");
          break;

        case 'sti:':
          this.storageInfo = JSON.parse(ev.data.substr(4));
          utility.log("ws: storage infomation update");
          break

        case 'ukr:':
          this.updateKeywordReserveRevision();
          utility.log("ws: keyword reserve update");
          break;

        // 結果は非リアクティブ領域で蓄積し、描画用の反映をまとめる。
        case 'rss:':
          {
            const reset = ev.data.length > 4 ? JSON.parse(ev.data.substr(4)) : {};
            if (this._searchRequestID && reset.requestId !== this._searchRequestID) break;
            this.beginSearchResults(reset.requestId || null);
          }
          break;

        case 'res:':
          {
            const response = JSON.parse(ev.data.substr(4));
            if (this._searchRequestID && response.requestId !== this._searchRequestID) break;
            const packet = response.requestId ? response.data : response;
            if (!this._searchResultStream) this.beginSearchResults(response.requestId || null);
            if (Object.prototype.hasOwnProperty.call(packet, 'progress')) {
              this.searchProgress = { ...packet, running: true };
            } else if (Object.prototype.hasOwnProperty.call(packet, 'hits')) {
              this._searchResultStream.finish();
            } else {
              this._searchResultStream.receive(Array.isArray(packet) ? packet : [packet]);
            }
          }
          break;

        case 'bml:':
          {
            const packet = JSON.parse(ev.data.substr(4));
            const dev = this.device.find(d => d.ID === packet.DeviceID);
            if (dev && dev.CurrentService === packet.Sid &&
              (dev.State === label.DSTAT_READY || dev.State === label.DSTAT_RECORDING)) {
              const pmt = packet.Messages.find(m => m.type === 'pmt');
              if (pmt && !pmt.components.length) { this.$delete(this.bml, dev.ID); break; }
              if (packet.Mode === 'event') {
                const prev = this.bml[dev.ID];
                if (prev && prev.Session === packet.Session) {
                  const types = new Set(packet.Messages.map(m => m.type + ':' + (m.componentId || 0)));
                  this.$set(this.bml, dev.ID, { ...prev, Messages: [
                    ...prev.Messages.filter(m => !types.has(m.type + ':' + (m.componentId || 0))), ...packet.Messages,
                  ] });
                }
              } else this.$set(this.bml, dev.ID, packet);
            }
          }
          break;
        case 'cap:':
          {
            let js = JSON.parse(ev.data.substr(4));
            this.queueCaption(js);
            if (this.$refs.timetable) this.$refs.timetable.queueHistoryCaption(js);
          }
          break;
      }
    },

    // WebSocket
    connectSocket() {
      if (this.socket &&
        (this.socket.readyState === WebSocket.OPEN || this.socket.readyState === WebSocket.CONNECTING)) {
        return;
      }

      if (this.socketReconnectTimer) {
        clearTimeout(this.socketReconnectTimer);
        this.socketReconnectTimer = null;
      }

      const socket = new WebSocket(clientIdentity.socketURL(`${getWSBaseURL()}/con`));
      this.socket = socket;

      socket.onmessage = this.handleSocketMessage;

      socket.onopen = (ev) => {
        this.socketReconnectDelay = 1000;
        // 切断中に変わった状態も含め、接続ごとに全デバイスを再取得する。
        socket.send('REL:');
        this.requestUiRefresh('schedule', 'guide');
        this.handlePowerManagerSettingChange();
        utility.log("socket connected "+ JSON.stringify(ev))
      }

      socket.onclose = () => {
        this.bml = {};
        if (this.socket === socket) {
          this.socket = null;
        }
        utility.log("socket closed");
        this.scheduleSocketReconnect();
      }

      socket.onerror = (err) => {
        utility.log("socket error "+JSON.stringify(err));
        socket.close();
      }
    },

    scheduleSocketReconnect() {
      if (this.socketClosing || this.socketReconnectTimer) {
        return;
      }

      const delay = this.socketReconnectDelay;
      utility.log("socket reconnect in "+delay+"ms");
      this.socketReconnectTimer = setTimeout(() => {
        this.socketReconnectTimer = null;
        this.connectSocket();
      }, delay);
      this.socketReconnectDelay = Math.min(delay * 2, this.socketMaxReconnectDelay);
    },

    closeSocket() {
      this.socketClosing = true;
      if (this.socketReconnectTimer) {
        clearTimeout(this.socketReconnectTimer);
        this.socketReconnectTimer = null;
      }
      if (this.socket) {
        this.socket.close();
        this.socket = null;
      }
    },

    //
    applyDisplayOptions(opt) {
      this.options = opt;
    },

    redrawTT: function(opt = null, preserveRange = false) {
      if (opt !== null) {
        this.options = opt;
        saveOptionsToCookie(this.options);
      }
      this.updateTvProgram(opt !== null && !preserveRange);
    },

    updateKeywordReserveRevision() {
      this.keywordReserveRevision++;
    },

    changeTimeTableBroadcast(v) {
      this.maintab = 0;
      this.$nextTick(() => {
        if (this.$refs.timetable) {
          this.$refs.timetable.setBroadcast(v);
        }
      });
    },
    
    hourChange(v) {
      this.maintab = 0;
      this.$nextTick(() => {
        if (this.$refs.timetable) {
          this.$refs.timetable.setHoursImmediately(v);
        }
      });
    },

    async selectProgram(item) {
      this.maintab = 0;
      // 既存の番組DOMがあっても、非表示タブではスクロール位置を設定できない。
      await this.$nextTick();
      if (this.$refs.timetable) {
        this.$refs.timetable.changeProgramDate(item.sid, item.starttime);
      }
      utility.scrollToTarget('L' + item.sid  + '_' + item.eid);
    },

    searchKeywordReserve(item) {
      this.keywordSearchCondition = {
        searchid: item.id || 0,
        title: item.title || '',
        content: item.content || '',
        days: item.days || 24*10,
        sid: item.sid || [],
        week: item.week || 0,
        category: item.category || 0xffff,
        andsearch: !!item.andsearch,
        fuzzysearch: !!item.fuzzysearch,
        archivesearch: !!item.archivesearch,
        captionsearch: !!item.captionsearch,
        starthours: item.starthour || 0,
        hours: item.hours || 24,
        idletime: item.idletime || 0,
        readytime: item.readytime || 0,
        tailtime: item.tailtime || 0,
        recordonly: item.recordonly !== false,
        watchonly: !!item.watchonly,
        shutdown: item.shutdown !== false,
        epgfollow: item.epgfollow !== false,
        customcomid: item.customcomid || '',
        hashtag: item.hashtag || '',
        deviceid: item.deviceid || '',
      };
      this.maintab = 2;
    },

    searchTimeTableProgram(item) {
      this.searchResults = [];
      this.keywordSearchCondition = {
        searchid: item.id || 0,
        title: item.title || '',
        content: item.content || '',
        days: item.days || 24*10,
        sid: item.ssid ? [item.ssid] : (item.sid ? item.sid : []),
        week: 0,
        category: 0xffff,
        andsearch: false,
        fuzzysearch: true,
        archivesearch: false,
        captionsearch: false,
        starttime: 0,
        endtime: 0,
        starthours: 0,
        hours: 24,
        idletime: 60,
        readytime: 30,
        tailtime: 0,
        recordonly: true,
        watchonly: false,
        shutdown: true,
        epgfollow: true,
        customcomid: '',
        hashtag: '',
        deviceid: '',
      };
      this.maintab = 2;
    },

    loadMpegts() {
      return new Promise((resolve, reject) => {
        if (window.mpegts) {
          resolve(window.mpegts);
          return;
        }

        const script = document.createElement('script');
        script.src = `${getHTTPBaseURL()}/static/mpegts.js`;
        script.onload = () => {
          if (window.mpegts) {
            resolve(window.mpegts);
          } else {
            reject(new Error("mpegts is not defined on window"));
          }
        };
        script.onerror = (err) => {
          reject(err);
        };
        document.head.appendChild(script);
      });
    },

    async startPlayback() {
      if (!label.MPEG2TS) {
        console.warn("MPEG2TS is disabled by config");
        return;
      }

      let mpegtsInstance = window.mpegts;
      if (!mpegtsInstance) {
        try {
          mpegtsInstance = await this.loadMpegts();
        } catch (e) {
          utility.err("Failed to load mpegts.js:", e);
          return;
        }
      }

      if (this.player) {
        this.stopPlayback();
      }

      if (mpegtsInstance && mpegtsInstance.getFeatureList().mseLivePlayback) {
        const videoElement = this.$refs.videoElement;
        const streamUrl = `${getHTTPBaseURL()}/static/mytest.ts`;

        this.player = mpegtsInstance.createPlayer({
          type: 'mse',
          isLive: true,
          url: streamUrl
        });

        this.player.attachMediaElement(videoElement);
        this.player.load();
        this.player.play();
      } else {
        utility.err("mseLivePlayback is not supported or mpegts is not loaded");
      }
    },

    stopPlayback() {
      if (!label.MPEG2TS) {
        return;
      }
      if (this.player) {
        this.player.pause();
        this.player.unload();
        this.player.detachMediaElement();
        this.player.destroy();
        this.player = null;
      }
    }

  },

  created() {
    this._initialDataReady = new Promise(resolve => { this._resolveInitialData = resolve; });
    this.systemTheme = SystemTheme.followSystemTheme(this.$vuetify.theme, () => this.options.forcetheme);
    this.label = label;
    this.utility = utility;
//    this.host = "http://localhost"
    this.api.configure = this.host + '/cnf';
    this.api.caption = this.host + '/caption';
    this.api.station = this.host + '/stl';
    this.api.timetable = this.host + '/tvp';
    this.api.feature = this.host + '/tvf';
    this.api.schedule = this.host + '/sch';
    this.api.recordingStatus = this.host + '/rst';
    this.api.device = this.host + '/dvs';
    this.api.search = this.host + '/src';
    this.api.keyword = this.host + '/kwr';
    this.api.fetchProgramSchedule = this.host + '/fps';
    this.api.recorderSetting = this.host + '/rcs';
    this.api.playback = this.host + '/playback';
    this.api.powerManagerSetting = this.host + '/pms';
    this.api.log = this.host + '/log';

    // websocket
    this.connectSocket();

    (async () => {
      try {
        await this.updateConfigure();
        await this.updateService();
        await this.updateSchedule();
        await this.updateTvFeature();
        await this.updateTvProgram();
        await this.updateDeviceAll();
      } finally {
        this._resolveInitialData();
      }
    })();

    utility.log("created");
  },

  /*
  mounted() {
    console.table("mounted");
  }
  */

  watch: {
    sidenavi(visible) {
      if (visible && this._sidebarDirty) this.requestUiRefresh('sidebar');
    },
    maintab: function(newVal) {
      if (newVal !== 1) this.scheduleByDevice = false;
      if (newVal === 0) {
        // Measure after v-tab-item becomes visible, including without ResizeObserver.
        this.$nextTick(() => {
          if (this.$refs.timetable) this.$refs.timetable.syncStationHeaderLayout();
        });
      }
      if (newVal !== 6) {
        this.stopPlayback();
      }
    },
    'options.forcetheme': function() {
      this.systemTheme.refresh();
    },
    options: {
      handler: function(newVal) {
        saveOptionsToCookie(newVal);
      },
      deep: true,
    }
  },

  beforeDestroy() {
    if (this._searchResultStream) this._searchResultStream.dispose();
    if (this._uiRefreshQueue) this._uiRefreshQueue.dispose();
    this.systemTheme.dispose();
    this.closeSocket();
    Object.values(this.captionPlayback).forEach(playback => playback.player.dispose());
    Object.keys(this.captionTimers).forEach(id => clearTimeout(this.captionTimers[id]));
    this.stopPlayback();
  }

};
</script>

<style scoped>
.main-tab-bar {
  /* Soft, uneven shadows over the existing theme colour, shared by the whole bar. */
  background-image:
    radial-gradient(ellipse 21% 180% at 9% -35%, rgba(9, 53, 94, 0.32), transparent 78%),
    radial-gradient(ellipse 17% 150% at 31% 120%, rgba(15, 78, 94, 0.32), transparent 80%),
    radial-gradient(ellipse 26% 210% at 53% 15%, rgba(14, 99, 105, 0.32), transparent 76%),
    radial-gradient(ellipse 15% 170% at 74% -25%, rgba(13, 58, 100, 0.32), transparent 82%),
    radial-gradient(ellipse 22% 160% at 94% 115%, rgba(27, 66, 117, 0.32), transparent 78%);
}

.main-tab-card {
  overflow: hidden;
}

.main-tab-clock {
  padding-left: 24px;
}

@media (max-width: 1263px) {
  .main-tab-clock {
    padding-left: 8px;
  }

  .main-tab-navigation {
    flex: 0 1 auto;
    min-width: 0;
  }

  .main-tabs .v-tab {
    flex: 0 0 48px;
    min-width: 48px;
    max-width: 48px;
    padding: 0 !important;
    margin: 0 !important;
  }

  .main-tabs .text-caption {
    display: none;
  }
}

.app-logo-clickable {
  cursor: pointer;
}

.brand{
  font-weight: 600;
  opacity: 0.85;
  text-shadow: 2px 2px 2px rgba(0.1, 0.1, 0.15, 0.3);
  display:flex;align-items:center;min-width:0
}

.tab-split-layout {
  display: flex;
  align-items: stretch;
  width: 100%;
}

.tab-main-content {
  flex: 1 1 auto;
  min-width: 0;
}

.tab-split-layout--contained {
  height: calc(100vh - var(--app-content-top, 64px));
  height: calc(100dvh - var(--app-content-top, 64px));
  min-height: 0;
  overflow: hidden;
}

.tab-split-layout--contained .tab-main-content {
  height: 100%;
  min-height: 0;
}

.tab-split-layout--contained ::v-deep .tab-main-content > .v-window__container {
  height: 100%;
}

.device-scroll-tab,
.settings-tab,
.settings-tab-card {
  height: 100%;
  min-height: 0;
}

.device-scroll-tab {
  overflow: auto;
  overscroll-behavior: contain;
  padding-bottom: 16px;
}

.settings-tab,
.settings-tab-card {
  overflow: hidden;
}

.tab-side-navigation {
  /* Keep sticky sidebar headers below the app bar (z-index: 5). */
  position: relative;
  z-index: 0;
  flex: 0 0 360px;
  width: 360px;
  max-width: 40vw;
  border-left: 1px solid rgba(0, 0, 0, 0.12);
  overflow-y: auto;
  max-height: calc(100vh - 64px);
}

.v-tabs-items.full-height-tab  .v-window-item {
  height: calc(100vh - 270px); /* Adjust 270px to suits your needs */
  overflow-y: auto;
}

@media (max-width: 960px) {
  .tab-split-layout {
    flex-direction: column;
  }

  .tab-side-navigation {
    flex-basis: auto;
    width: 100%;
    max-width: none;
    max-height: 40vh;
    border-left: 0;
    border-top: 1px solid rgba(0, 0, 0, 0.12);
  }
}
</style>
