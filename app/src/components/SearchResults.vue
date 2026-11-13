<template>

<div class="search-results">
  <v-toolbar dense flat>
    <v-select
      v-model="selectedSids"
      class="search-option"
      :items="stationOptions"
      item-text="text"
      item-value="value"
      label="局名"
      multiple
      dense
      hide-details
      clearable
      @change="changeSearchOption"
    ></v-select>
    <v-select
      v-model="selectedCategories"
      class="search-option"
      :items="categoryOptions"
      item-text="text"
      item-value="value"
      label="ジャンル"
      multiple
      dense
      hide-details
      clearable
      @change="changeSearchOption"
    ></v-select>
    <v-checkbox
      v-model="andSearch"
      class="search-and"
      label="AND"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-model="fuzzySearch"
      class="search-fuzzy"
      label="あいまい"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-model="archiveSearch"
      class="search-bleve"
      label="過去番組検索"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-if="archiveSearch"
      v-model="captionSearch"
      class="search-bleve"
      label="字幕"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-btn
      class="search-detail-toggle"
      small
      outlined
      @click="detailOpen = !detailOpen"
    >
      <v-icon left small>{{ detailOpen ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
      {{ archiveSearch ? '詳細設定' : '検索予約設定' }}
    </v-btn>
    <v-spacer></v-spacer>
    <v-btn
      class="search-option-reset"
      small
      @click="$emit('searchoptionreset')"
    >
      <v-icon left small>mdi-refresh</v-icon>
      リセット
    </v-btn>
    <v-btn
      v-if="searchID === 0"
      color="primary"
      small
      :disabled="!canReserve || reserving"
      :loading="reserving"  
      @click="addKeywordReserve"
    >
      <v-icon left small>mdi-key-plus</v-icon>
      検索予約登録
    </v-btn>

    <v-btn
      v-if="searchID !== 0"
      color="primary"
      small
      :disabled="!canReserve || reserving"
      :loading="reserving"
      @click="saveKeywordReserve"
    >
      <v-icon left small>mdi-content-save</v-icon>
      検索予約保存
    </v-btn>

  </v-toolbar>

  <div v-if="archiveSearch" class="search-detail">
    <v-menu v-model="startDateOpen" :close-on-content-click="false" offset-y min-width="290px">
      <template v-slot:activator="{ on, attrs }">
        <v-text-field
          v-model="startTime"
          class="search-detail-custom"
          label="検索開始日"
          placeholder="指定なし"
          prepend-inner-icon="mdi-calendar"
          readonly
          clearable
          dense
          hide-details
          v-bind="attrs"
          v-on="on"
          @click:clear="changeSearchOption"
        ></v-text-field>
      </template>
      <v-date-picker
        v-model="startTime"
        locale="ja-jp"
        :max="endTime || undefined"
        @input="startDateOpen = false; changeSearchOption()"
      ></v-date-picker>
    </v-menu>
    <v-menu v-model="endDateOpen" :close-on-content-click="false" offset-y min-width="290px">
      <template v-slot:activator="{ on, attrs }">
        <v-text-field
          v-model="endTime"
          class="search-detail-custom"
          label="検索終了日"
          placeholder="現在まで"
          prepend-inner-icon="mdi-calendar"
          readonly
          clearable
          dense
          hide-details
          v-bind="attrs"
          v-on="on"
          @click:clear="changeSearchOption"
        ></v-text-field>
      </template>
      <v-date-picker
        v-model="endTime"
        locale="ja-jp"
        :min="startTime || undefined"
        @input="endDateOpen = false; changeSearchOption()"
      ></v-date-picker>
    </v-menu>
    <span class="text-caption">終了日を含む期間を検索します。未指定の場合は全期間が対象です。</span>
  </div>

  <v-alert v-if="searchProgress && searchProgress.warning" type="warning" outlined>
    {{ searchProgress.warning }}
  </v-alert>

  <div v-if="isSearching" class="search-progress">
    <v-progress-linear
      :value="progressPercent"
      height="4"
      color="primary"
    ></v-progress-linear>
    <div class="search-progress-text">
      Searching... {{ progressPercent }}% / {{ progressHits }} hits / {{ progressScanned }} scanned
    </div>
  </div>

  <div v-if="detailOpen" class="search-detail">
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="startHours"
      class="search-detail-number"
      label="検索開始時刻"
      type="number"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-text-field>
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="hours"
      class="search-detail-number"
      label="検索時間"
      type="number"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-text-field>
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="idleTime"
      class="search-detail-number"
      label="待機時間(秒)"
      type="number"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-text-field>
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="readyTime"
      class="search-detail-number"
      label="録画開始時間(秒)"
      type="number"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-text-field>
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="tailTime"
      class="search-detail-number"
      label="録画終了時間(秒)"
      type="number"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-text-field>
    <v-select
      v-if="!archiveSearch"
      v-model="selectedWeekdays"
      class="search-option search-weekdays"
      :items="weekOptions"
      item-text="text"
      item-value="value"
      label="曜日"
      placeholder="全曜日"
      persistent-placeholder
      multiple
      dense
      hide-details
      clearable
      @change="changeSearchOption"
    ></v-select>
    <v-select
      v-if="!archiveSearch"
      v-model="selectedDays"
      class="search-days"
      :items="dayOptions"
      item-text="text"
      item-value="value"
      label="検索日数"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-select>
    <div v-if="!archiveSearch" class="search-result-detail-break"></div>
    <v-select
      v-if="!archiveSearch"
      v-model="deviceID"
      class="search-detail-custom"
      :items="deviceList"
      item-text="text"
      item-value="value"
      label="デバイスID"
      dense
      hide-details
      clearable
    ></v-select>
    <v-select
      v-if="!archiveSearch"
      v-model="customComID"
      class="search-detail-custom"
      label="カスタムコマンド"
      :items="customCommandList"
      item-text="text"
      item-value="value"
      dense
      hide-details
    ></v-select>
    <v-text-field
      v-if="!archiveSearch"
      v-model.number="hashTag"
      class="search-detail-custom"
      label="ハッシュタグ"
      dense
      hide-details
    ></v-text-field>
    <v-checkbox
      v-if="!archiveSearch"
      v-model="recordOnly"
      v-bind:disabled="watchOnly"
      class="search-detail-check"
      label="録画のみ"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-if="!archiveSearch"
      v-model="watchOnly"
      v-bind:disabled="recordOnly"
      class="search-detail-check"
      label="視聴のみ"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-if="!archiveSearch"
      v-model="shutdown"
      class="search-detail-check"
      label="録画終了後アプリ終了"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>
    <v-checkbox
      v-if="!archiveSearch"
      v-model="epgFollow"
      class="search-detail-check"
      label="番組時間追従"
      dense
      hide-details
      @change="changeSearchOption"
    ></v-checkbox>

    <v-text-field v-if="label.DEBUG" v-bind:disabled="true"
      v-model.number="searchID"
      type="number"
      dense
      hide-details
      >
    </v-text-field>

  </div>

  <v-simple-table class="search-results-table" :class="{ 'search-results-with-watch': !archiveSearch && options && options.watchbutton }" dense fixed-header>
    <template v-slot:default>

      <thead class="search-result-head">
        <tr>
          <th class="text-left search-result-date-column">
            Date
          </th>
          <th class="text-left search-result-service-column">
            Service
          </th>
          <th class="text-left">
            Title
          </th>
          <th class="text-left search-result-subtitle-column">
            Subtitle
          </th>
          <th class="text-left search-result-actions">
          </th>
        </tr>
      </thead>

      <tbody>
        <tr
        v-for="(item,key) in displaySearchResults"
        :key="programKey(item)"
        class="search-result-row"
        :class="{ 'search-result-recording': recordingInfo(item).isRec,
          'search-result-watch': recordingInfo(item).isRec && recordingInfo(item).isWatch }"
        @click="selectProgram(item)"
        >
          <td class="search-result-text" :title="item.DateW">{{ item.DateW }}</td>
          <td class="search-result-text" :title="item.ServiceName">{{ item.ServiceName }}</td>
          <td :title="item.Title">
            <div class="search-result-title">
              <v-icon
                v-if="recordingInfo(item).isRec"
                class="search-result-record-icon"
                x-small
              >
                mdi-clock-outline
              </v-icon>
              <span
                v-if="recordingInfo(item).isRec && !archiveSearch"
                class="search-result-device"
              >
                {{ (recordingInfo(item).complete ? '※終了 - ' : '') + recordingInfo(item).deviceID }}
              </span>
              <span class="search-result-text">{{ item.Title + (label.DEBUG ? '(' +  item.Eid + ')' : '') }}</span>
            </div>
          </td>
          <td :title="item.Subtitle">
            <div class="search-result-title">
              <span class="search-result-text">{{ item.Subtitle }}</span>
            </div>
          </td>
          <td class="search-result-actions">
            <v-btn
              v-if="!archiveSearch"
              class="search-result-reserve search-result-record-button"
              x-small
              :disabled="!canReserveProgram(item) || recordingInfo(item).isRec"
              :loading="recordingKey === programKey(item, key)"
              @click.stop="addProgramReserve(item, key)"
            >
              <v-icon left x-small>mdi-clock-outline</v-icon>
              {{ recordingInfo(item).isRec ? '予約済み' : '録画予約' }}
            </v-btn>
            <v-btn
              v-if="!archiveSearch && options && options.watchbutton"
              class="search-result-reserve search-result-watch-button"
              x-small
              :disabled="!canReserveProgram(item) || recordingInfo(item).isRec"
              :loading="recordingKey === programKey(item, key)"
              @click.stop="addProgramReserve(item, key, true)"
            >
              <v-icon left x-small>mdi-television</v-icon>
              視聴予約
            </v-btn>
          </td>
        </tr>
      </tbody>

    </template>

  </v-simple-table>

  <div v-if="searchResults && searchResults.length" class="search-result-pagination text-center ma-3">
    <span>{{ (resultPage - 1) * pageSize + 1 }}～{{ Math.min(resultPage * pageSize, searchResults.length) }} / {{ searchResults.length }} 件</span>
    <v-pagination v-model="resultPage" :length="pageCount" :total-visible="7"></v-pagination>
  </div>
</div>

</template>

<script>
import label from '../define.js';
import utility from '../utility.js';
import axios from 'axios';
import SearchResultStream from '../searchResults';

export default {
  name: 'SearchResults',
  props: [
    'api',
    'searchResults',
    'searchProgress',
    'searchCondition',
    'schedule',
    'station',
    'deviceList',
    'configure',
    'options',
  ],

  data() {
    return {
      label: null,
      searchID: 0,
      reserving: false,
      recordingKey: null,
      selectedDays: 24*10,
      selectedSids: [],
      selectedWeekdays: [],
      selectedCategories: [],
      andSearch: false,
      fuzzySearch: true,
      archiveSearch: false,
      captionSearch: false,
      detailOpen: false,
      startTime: '',
      endTime: '',
      startDateOpen: false,
      endDateOpen: false,
      startHours: 0,
      hours: 24,
      idleTime: 60,
      readyTime: 30,
      tailTime: 0,
      recordOnly: true,
      watchOnly: false,
      shutdown: true,
      epgFollow: true,
      customComID: '',
      hashTag: '',
      deviceID: '',
      syncingSearchOption: false,
      resultPage: 1,
      pageSize: 100,
      reservationRevision: 0,
    };
  },

  computed: {
    pageCount() {
      return Math.max(1, Math.ceil((this.searchResults || []).length / this.pageSize));
    },

    reservationIndex() {
      return SearchResultStream.reservationIndex(this.schedule);
    },

    displaySearchResults() {
      // 大量の日時整形と深い監視を避け、表示するページだけを整形する。
      void this.reservationRevision;
      // computedを行ごとに参照するとVue 2が予約全件の依存を毎回たどるため、一度だけ読む。
      const reservations = this.reservationIndex;
      const start = (this.resultPage - 1) * this.pageSize;
      return (this.searchResults || []).slice(start, start + this.pageSize).map(item => {
        item.ServiceName = this.station && this.station.all && this.station.all[item.Sid] ? this.station.all[item.Sid].Name : 'unknown';
        item.DateW = utility.uxtmtoymdhmj(item.Start, this.options && this.options.twentyEight) + '～' +
          utility.uxtmtohm(item.End, this.options && this.options.twentyEight);
        item.SearchRecordingInfo = SearchResultStream.recordingInfo(item, reservations);
        return item;
      });
    },

    isSearching() {
      return !!(this.searchProgress && this.searchProgress.running);
    },

    progressPercent() {
      if (!this.searchProgress) return 0;
      return Math.max(0, Math.min(100, this.searchProgress.percent || 0));
    },

    progressHits() {
      return this.searchProgress ? (this.searchProgress.hits || 0) : 0;
    },

    progressScanned() {
      return this.searchProgress ? (this.searchProgress.scanned || 0) : 0;
    },

    dayOptions() {
      return [
        { text: '4 hours', value: 4 },
        { text: '8 hours', value: 8 },
        { text: '12 hours', value: 12 },
        { text: '1 day', value: 24 },
        { text: '2 day', value: 24*2 },
        { text: '3 days', value: 24*3 },
        { text: '4 days', value: 24*4 },
        { text: '5 days', value: 24*5 },
        { text: '6 days', value: 24*6 },
        { text: '7 days', value: 24*7 },
        { text: '10 days', value: 24*10 },
      ];
    },

    categoryOptions() {
      return [
        { text: 'ニュース／報道', value: 0 },
        { text: 'スポーツ', value: 1 },
        { text: '情報／ワイドショー', value: 2 },
        { text: 'ドラマ', value: 3 },
        { text: '音楽', value: 4 },
        { text: 'バラエティ', value: 5 },
        { text: '映画', value: 6 },
        { text: 'アニメ／特撮', value: 7 },
        { text: 'ドキュメンタリー／教養', value: 8 },
        { text: '劇場／公演', value: 9 },
        { text: '趣味／教育', value: 10 },
        { text: '福祉', value: 11 },
        { text: '事業体定義／予備１', value: 12 },
        { text: '事業体定義／予備２', value: 13 },
        { text: '拡張（拡張用フラグ）／予備', value: 14 },
        { text: 'その他', value: 15 },
      ];
    },

    weekOptions() {
      return utility.weekOptions;
    },

    stationOptions() {
      if (!this.station || !this.station.all) {
        return [];
      }
      return Object.keys(this.station.all).map(key => this.station.all[key])
        .filter(item => item.View > 0)
        // 放送種別（地上波・BS・CS・ワンセグ）の順に、各種別内はサービスID順に並べる。
        .sort((a, b) => (a.Tsp - b.Tsp) || ((a.Sid & 0xffff) - (b.Sid & 0xffff)))
        .map(item => ({
          text: item.Name + ' (' + (item.Sid & 0xffff) + ')',
          value: item.Sid,
        }));
    },

    categoryMask() {
      if (!this.selectedCategories || this.selectedCategories.length < 1) {
        return 0xffff;
      }
      return this.selectedCategories.reduce((mask, v) => mask | (1 << v), 0);
    },

    canReserve() {
      return this.api && this.api.keyword && this.searchCondition &&
        (this.searchCondition.title || this.searchCondition.content);
    },

    customCommandList() {
      let ret = [];
      for (const v of this.configure.CustomCommand) {
        ret.push({
          text: v.ID,
          value: v.ID,
        });
      }
      return ret;
    },
  },

  methods: {
    currentSearchOption() {
      return {
        id: this.searchID || 0,
        title: this.searchCondition ? this.searchCondition.title : '',
        content: this.searchCondition ? this.searchCondition.content : '',
        days: this.selectedDays || 24*10,
        sid: this.selectedSids || [],
        week: utility.weekListToMask(this.selectedWeekdays),
        category: this.categoryMask,
        andsearch: this.andSearch,
        fuzzysearch: this.fuzzySearch,
        archivesearch: this.archiveSearch,
        captionsearch: this.captionSearch,
        starttime: this.startTime || '',
        endtime: this.endTime || '',
        starthours: this.startHours || 0,
        hours: this.hours || 24,
        idletime: this.idleTime || 0,
        readytime: this.readyTime || 0,
        tailtime: this.tailTime || 0,
        recordonly: this.recordOnly,
        watchonly: this.watchOnly,
        shutdown: this.shutdown,
        epgfollow: this.epgFollow,
        customcomid: this.customComID || '',
        hashtag: this.hashTag || '',
        deviceid: this.deviceID || '',
      };
    },

    programKey(item) {
      return SearchResultStream.key(item);
    },

    canReserveProgram(item) {
      return this.api && this.api.schedule && item &&
        item.Sid != null && item.Eid != null &&
        item.Start != null && item.End != null;
    },

    changeSearchOption() {
      if (this.syncingSearchOption) {
        return;
      }
      this.$nextTick(() => {
        this.$emit('searchoptionchange', this.currentSearchOption());
      });
    },

    categoryMaskToList(mask) {
      if (!mask || mask === 0xffff) {
        return [];
      }
      let ret = [];
      for (let i = 0; i < 16; i++) {
        if ((mask & (1 << i)) !== 0) {
          ret.push(i);
        }
      }
      return ret;
    },

    recordingInfo(item) {
      return item && item.SearchRecordingInfo || SearchResultStream.recordingInfo(item, this.reservationIndex);
    },

    addProgramReserve(item, key, watch = false) {
      if (!this.canReserveProgram(item) || this.recordingInfo(item).isRec) {
        return;
      }

      this.recordingKey = this.programKey(item, key);
      let par = {
        sid: item.Sid,
        eid: item.Eid,
        start: item.Start,
        end: item.End
      };

      if (watch) par.watch = true;
      axios.post(this.api.schedule, JSON.stringify(par)).then(res => {
        utility.log("api schedule results:" + res.status);
        this.$set(item, 'IsRec', true);
        this.$set(item, 'IsWatch', watch);
        this.reservationRevision++;
      }).catch(err => {
        utility.err(`request schedule error: ${err}`);
      }).finally(() => {
        this.recordingKey = null;
      });
    },

    addKeywordReserve() {
      if (!this.canReserve) {
        return;
      }
      this.reserving = true;
      const opt = this.currentSearchOption();
      axios.post(this.api.keyword, JSON.stringify({
        validate: true,
        title: opt.title,
        content: opt.content,
        days: opt.days,
        sid: opt.sid,
        week: opt.week,
        category: opt.category,
        andsearch: opt.andsearch,
        fuzzysearch: opt.fuzzysearch,
        archivesearch: opt.archivesearch,
        captionsearch: opt.captionsearch,
        starthours: opt.starthours,
        hours: opt.hours,
        idletime: opt.idletime,
        readytime: opt.readytime,
        tailtime: opt.tailtime,
        recordonly: opt.recordonly,
        watchonly: opt.watchonly,
        shutdown: opt.shutdown,
        epgfollow: opt.epgfollow,
        customcomid: opt.customcomid,
        hashtag: opt.hashtag,
        deviceid: opt.deviceid,
      })).then(res => {
        utility.log("keyword reserve status: " + res.status);
        return axios.post(this.api.keyword + '/' + res.data.id + '/schedule').then(sres => {
          utility.log("keyword reserve schedule status: " + sres.status);
          this.markSearchResultsRecording(opt.watchonly && !opt.recordonly);
          this.$emit('keywordreserveadd', res.data);
        });
      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      }).finally(() => {
        this.reserving = false;
      });
    },

    saveKeywordReserve() {
      if (!this.canReserve || this.searchID === 0) {
        return;
      }
      this.reserving = true;
      const opt = this.currentSearchOption();
      axios.put(this.api.keyword, JSON.stringify({
        id: opt.id,
        validate: true,
        title: opt.title,
        content: opt.content,
        days: opt.days,
        sid: opt.sid,
        week: opt.week,
        category: opt.category,
        andsearch: opt.andsearch,
        fuzzysearch: opt.fuzzysearch,
        archivesearch: opt.archivesearch,
        captionsearch: opt.captionsearch,
        starthours: opt.starthours,
        hours: opt.hours,
        idletime: opt.idletime,
        readytime: opt.readytime,
        tailtime: opt.tailtime,
        recordonly: opt.recordonly,
        watchonly: opt.watchonly,
        shutdown: opt.shutdown,
        epgfollow: opt.epgfollow,
        customcomid: opt.customcomid,
        hashtag: opt.hashtag,
        deviceid: opt.deviceid,
      })).then(res => {
        utility.log("keyword reserve schedule status: " + res.status);
        this.markSearchResultsRecording(opt.watchonly && !opt.recordonly);
        this.$emit('keywordreserveadd', res.data);
      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      }).finally(() => {
        this.reserving = false;
      });
    },

    markSearchResultsRecording(watch = false) {
      for (const v of this.searchResults) {
        this.$set(v, 'IsRec', true);
        this.$set(v, 'IsWatch', watch);
      }
      this.reservationRevision++;
    },

    selectProgram(item) {
      this.$emit('select', {
        sid: item.Sid,
        eid: item.Eid,
        starttime: item.Start,
        endtime: item.End,
      });
    },
  },

  watch: {
    searchResults(val) {
      if (!val || val.length === 0) {
        this.resultPage = 1;
      }
    },
    pageCount(count) {
      this.resultPage = Math.min(this.resultPage, count);
    },
    resultPage() {
      this.$nextTick(() => {
        const wrapper = this.$el.querySelector(".v-data-table__wrapper");
        if (wrapper) wrapper.scrollTop = 0;
      });
    },
    searchCondition: {
      handler(v) {
        if (!v) {
          return;
        }
        this.searchID = v.searchid || 0;
        this.syncingSearchOption = true;
        this.selectedDays = v.days || 24*10;
        this.selectedSids = v.sid || [];
        this.selectedWeekdays = utility.weekMaskToList(v.week);
        this.selectedCategories = this.categoryMaskToList(v.category);
        this.andSearch = !!v.andsearch;
        this.fuzzySearch = !!v.fuzzysearch;
        this.archiveSearch = !!v.archivesearch;
        this.captionSearch = !!v.captionsearch;
        this.startTime = v.starttime || '';
        this.endTime = v.endtime || '';
        this.startHours = v.starthours || 0;
        this.hours = v.hours || 24;
        this.idleTime = v.idletime || 0;
        this.readyTime = v.readytime || 0;
        this.tailTime = v.tailtime || 0;
        this.recordOnly = v.recordonly !== false;
        this.watchOnly = !!v.watchonly;
        this.shutdown = v.shutdown !== false;
        this.epgFollow = v.epgfollow !== false;
        this.customComID = v.customcomid || '';
        this.hashTag = v.hashtag || '';
        this.deviceID = v.deviceid || '';
        this.$nextTick(() => {
          this.syncingSearchOption = false;
        });
      },
      deep: true,
      immediate: true,
    },
  },

  created() {
    this.label = label;
  },
  destroyed() {
  }
};
</script>

<style scoped>

.search-results {
  display: flex;
  flex-direction: column;
  flex: 1 1 0;
  min-height: 0;
}

.search-results > * {
  flex: 0 0 auto;
}

.search-results > .search-results-table {
  --search-result-action-width: 112px;
  --search-result-min-width: 900px;
  display: flex;
  flex-direction: column;
  flex: 1 1 0;
  min-height: 0;
}

.search-results > .search-results-with-watch {
  --search-result-action-width: 200px;
  --search-result-min-width: 980px;
}

.search-results-table ::v-deep table {
  table-layout: fixed;
  min-width: var(--search-result-min-width);
}

.search-results-table ::v-deep th,
.search-results-table ::v-deep td,
.search-result-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-result-date-column { width: 260px; }
.search-result-service-column { width: 140px; }
.search-result-subtitle-column {
  /* Use the scroll viewport's width: percentage calculations on fixed table
     columns otherwise fall back to equally sized title/subtitle columns. */
  width: calc((max(var(--search-result-min-width), 100cqw) - 260px - 140px - var(--search-result-action-width)) / 3);
}

.search-results-table ::v-deep .v-data-table__wrapper {
  container-type: inline-size;
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

.search-results-table ::v-deep thead th {
  background-color: transparent !important;
}

/* Keep one continuous gradient across the sticky header row. */
.search-results-table ::v-deep thead {
  position: sticky;
  top: 0;
  z-index: 3;
}

.search-result-head {
  color: var(--app-text, #202020F0);
  background-color: var(--app-table-header, rgb(140, 180, 220));
  background-image: linear-gradient(to right, transparent 75%, rgba(0, 0, 0, 0.12) 100%);
}

.search-result-actions {
  width: var(--search-result-action-width);
  white-space: nowrap;
}

.search-result-actions .search-result-reserve + .search-result-reserve {
  margin-left: 4px;
}

.search-result-record-button {
  background-color: var(--tt-record-button, #f08080f0) !important;
  color: var(--app-text, #323232) !important;
}

.search-result-actions .search-result-watch-button.v-btn {
  background-color: var(--tt-watch-button, #f5d77af0) !important;
  color: var(--app-text, #323232) !important;
}

.search-option {
  max-width: 280px;
  margin-right: 12px;
}

.search-weekdays {
  max-width: 140px;
}

.search-days {
  max-width: 120px;
  margin-right: 12px;
}

.search-and,
.search-fuzzy,
.search-bleve {
  margin-right: 12px;
}

.search-option-reset {
  margin-right: 8px;
}

.search-detail-toggle {
  min-width: 72px;
  margin-right: 12px;
}

.search-detail-toggle.theme--light {
  border-color: rgba(0, 0, 0, 0.12);
}

.search-detail-toggle.theme--dark {
  border-color: rgba(255, 255, 255, 0.12);
}

.search-progress {
  padding: 6px 16px 8px;
  background: var(--app-surface-soft, rgba(245, 248, 252, 0.95));
}

.search-progress-text {
  margin-top: 4px;
  color: var(--app-text-secondary, rgba(32, 32, 32, 0.72));
  font-size: 12px;
  line-height: 1.4;
}

.search-detail {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 10px 16px;
  background: var(--app-surface-soft, rgba(245, 248, 252, 0.95));
}

.search-detail-number {
  max-width: 140px;
}

.search-detail-custom {
  max-width: 180px;
}

.search-detail-check {
  margin-top: 0;
}

.search-result-recording {
  box-shadow: inset 4px 0 0 rgba(240, 40, 40, 0.75);
}

.search-result-device {
  display: inline-block;
  min-width: 0;
  max-width: 40%;
  flex-shrink: 0;
  padding: 0 5px;
  margin-right: 6px;
  text-align: center;
  background: rgba(220, 80, 80, 0.80);
  color: rgba(240, 240, 240, 0.9);
  font-size: 0.75em;
  line-height: 1.4;
  font-weight: bold;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.search-result-watch {
  box-shadow: inset 4px 0 0 #d5af3c;
}

.search-result-watch .search-result-device {
  background-color: var(--tt-watch-button, #f5d77af0) !important;
  color: var(--app-text, #323232) !important;
}

.search-result-title {
  display: flex;
  align-items: center;
  min-width: 0;
  overflow: hidden;
}

.search-result-title > .search-result-text {
  display: block;
  flex: 1 1 auto;
  min-width: 0;
}

.search-result-record-icon {
  flex-shrink: 0;
  color: #f04040F0;
  margin-right: 4px;
}

.search-result-reserve {
  opacity: 0;
  pointer-events: none;
}

.search-result-row:hover .search-result-reserve,
.search-result-reserve.v-btn--loading {
  opacity: 1;
  pointer-events: auto;
}

.search-result-detail-break {
  flex-basis: 100%;
  height: 0;
}

</style>
