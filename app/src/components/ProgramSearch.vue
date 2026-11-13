<template>

  <div class="program-search" :style="{ '--search-app-top': $vuetify.application.top + 'px' }">

    <v-text-field
    label="input title"
    v-model="searchTitle"
    hide-details="auto"
    @change="changeWord()"
    ></v-text-field>
    <v-text-field
    label="input content"
    v-model="searchContent"
    hide-details="auto"
    @change="changeWord()"
    ></v-text-field>

    <SearchResults
    :api = 'api'
    :searchResults = 'searchResults'
    :searchCondition = 'searchCondition'
    :schedule = 'schedule'
    :station = 'station'
    :deviceList = 'deviceList'
    :configure = 'configure'
    :options = 'options'
    :searchProgress = 'searchProgress'
    @searchoptionchange = 'changeSearchOption'
    @searchoptionreset = 'resetSearchOption'
    @keywordreserveadd = '$emit("keywordreserveadd")'
    @select = '$emit("select", $event)'
    />

  </div>

</template>

<script>
import utility from '../utility.js';
import SearchResults from './SearchResults';
import axios from 'axios'

let searchRequestSequence = 0;

export default {
  name: 'ProgramSearch',
  components: {
    SearchResults,
  },
  props: [
    'UID',
    'api',
    'station',
    'searchResults',
    'searchProgress',
    'schedule',
    'keywordSearchCondition',
    'deviceList',
    'configure',
    'options',
    'refreshToken',
  ],

  data() {
    return {
      searchID: 0,
      searchTitle: '',
      searchContent: '',
      searchDays: 24*10,
      searchSids: [],
      searchWeek: 0,
      searchCategories: [],
      searchAndSearch: false,
      searchFuzzySearch: true,
      searchArchiveSearch: false,
      searchCaptionSearch: false,
      searchStartTime: '',
      searchEndTime: '',
      searchStartHours: 0,
      searchHours: 24,
      searchIdleTime: 60,
      searchReadyTime: 30,
      searchTailTime: 0,
      searchRecordOnly: true,
      searchWatchOnly: false,
      searchShutdown: true,
      searchEpgFollow: true,
      searchCustomComID: '',
      searchHashTag: '',
      searchDeviceID: '',
    };
  },

  computed: {
    categoryMask() {
      if (!this.searchCategories || this.searchCategories.length < 1) {
        return 0xffff;
      }
      return this.searchCategories.reduce((mask, v) => mask | (1 << v), 0);
    },

    searchCondition() {
      return {
        searchid: this.searchID,
        title: utility.htmlEscape(this.searchTitle),
        content: utility.htmlEscape(this.searchContent),
        days: this.searchDays,
        sid: this.searchSids,
        week: this.searchWeek,
        category: this.categoryMask,
        andsearch: this.searchAndSearch,
        fuzzysearch: this.searchFuzzySearch,
        archivesearch: this.searchArchiveSearch,
        captionsearch: this.searchCaptionSearch,
        starttime: this.searchStartTime,
        endtime: this.searchEndTime,
        starthours: this.searchStartHours,
        hours: this.searchHours,
        idletime: this.searchIdleTime,
        readytime: this.searchReadyTime,
        tailtime: this.searchTailTime,
        recordonly: this.searchRecordOnly,
        watchonly: this.searchWatchOnly,
        shutdown: this.searchShutdown,
        epgfollow: this.searchEpgFollow,
        customcomid: this.searchCustomComID,
        hashtag: this.searchHashTag,
        deviceid: this.searchDeviceID,
      };
    },

  },

  methods: {
    applyKeywordSearchCondition(v) {
      if (!v) {
        return;
      }
      this.searchID = v.searchid || 0;
      this.searchTitle = v.title || '';
      this.searchContent = v.content || '';
      this.searchDays = v.days || 24*10;
      this.searchSids = v.sid || [];
      this.searchWeek = v.week || 0;
      this.searchCategories = this.categoryMaskToList(v.category);
      this.searchAndSearch = !!v.andsearch;
      this.searchFuzzySearch = !!v.fuzzysearch;
      this.searchArchiveSearch = !!v.archivesearch;
      this.searchCaptionSearch = !!v.captionsearch;
      this.searchStartTime = v.starttime || '';
      this.searchEndTime = v.endtime || '';
      this.searchStartHours = v.starthours || 0;
      this.searchHours = v.hours || 24;
      this.searchIdleTime = v.idletime || 0;
      this.searchReadyTime = v.readytime || 0;
      this.searchTailTime = v.tailtime || 0;
      this.searchRecordOnly = v.recordonly !== false;
      this.searchWatchOnly = !!v.watchonly;
      this.searchShutdown = v.shutdown !== false;
      this.searchEpgFollow = v.epgfollow !== false;
      this.searchCustomComID = v.customcomid || '';
      this.searchHashTag = v.hashtag || '';
      this.searchDeviceID = v.deviceid || '';
      this.$nextTick(() => {
        this.changeWord();
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

    changeSearchOption(option) {
      this.searchID = option.id || 0;
      this.searchDays = option.days || 24*10;
      this.searchSids = option.sid || [];
      this.searchWeek = option.week || 0;
      this.searchCategories = this.categoryMaskToList(option.category);
      this.searchAndSearch = !!option.andsearch;
      this.searchFuzzySearch = !!option.fuzzysearch;
      this.searchArchiveSearch = !!option.archivesearch;
      this.searchCaptionSearch = !!option.captionsearch;
      this.searchStartTime = option.starttime || '';
      this.searchEndTime = option.endtime || '';
      this.searchStartHours = option.starthours || 0;
      this.searchHours = option.hours || 24;
      this.searchIdleTime = option.idletime || 0;
      this.searchReadyTime = option.readytime || 0;
      this.searchTailTime = option.tailtime || 0;
      this.searchRecordOnly = option.recordonly !== false;
      this.searchWatchOnly = !!option.watchonly;
      this.searchShutdown = option.shutdown !== false;
      this.searchEpgFollow = option.epgfollow !== false;
      this.searchCustomComID = option.customcomid || '';
      this.searchHashTag = option.hashtag || '';
      this.searchDeviceID = option.deviceid || '';
      this.$nextTick(() => {
        this.changeWord();
      });
    },

    resetSearchOption() {
      this.searchID = 0;
      this.searchTitle = '';
      this.searchContent = '';
      this.searchDays = 24*10;
      this.searchSids = [];
      this.searchWeek = 0;
      this.searchCategories = [];
      this.searchAndSearch = false;
      this.searchFuzzySearch = true;
      this.searchArchiveSearch = false;
      this.searchCaptionSearch = false;
      this.searchStartTime = '';
      this.searchEndTime = '';
      this.searchStartHours = 0;
      this.searchHours = 24;
      this.searchIdleTime = 60;
      this.searchReadyTime = 30;
      this.searchTailTime = 0;
      this.searchRecordOnly = true;
      this.searchWatchOnly = false;
      this.searchShutdown = true;
      this.searchEpgFollow = true;
      this.searchCustomComID = '';
      this.searchHashTag = '';
      this.searchDeviceID = '';
      this.$nextTick(() => {
        this.changeWord();
      });
    },

    changeWord() {
      let now = Math.floor(utility.now().getTime() / 1000);
      const requestId = Date.now() + ':' + (++searchRequestSequence);
      this._latestSearchRequest = requestId;
      this.$emit('searchstart', requestId);
      let opt = {
        requestId,
        uid: this.UID,
        id: this.searchID,
        title: this.searchCondition.title,
        content: this.searchCondition.content,
        start: now,
        days: this.searchDays,
        sid: this.searchCondition.sid,
        week: this.searchCondition.week,
        category: this.searchCondition.category,
        andsearch: this.searchCondition.andsearch,
        fuzzysearch: this.searchCondition.fuzzysearch,
        archivesearch: this.searchCondition.archivesearch,
        captionsearch: this.searchCondition.captionsearch,
        starttime: this.searchCondition.starttime,
        endtime: this.searchCondition.endtime,
        starthours: this.searchCondition.starthours,
        hours: this.searchCondition.hours,
        idletime: this.searchCondition.idletime,
        readytime: this.searchCondition.readytime,
        tailtime: this.searchCondition.tailtime,
        recordonly: this.searchCondition.recordonly,
        watchonly: this.searchCondition.watchonly,
        shutdown: this.searchCondition.shutdown,
        epgfollow: this.searchCondition.epgfollow,
        customcomid: this.searchCondition.customcomid,
        deviceid: this.searchCondition.deviceid,
      };

      // APIの登録順を保つ。条件を続けて変えた場合は未送信の古い検索を省く。
      this._searchSubmission = (this._searchSubmission || Promise.resolve()).then(() => {
        if (this._isDestroyed || this._isBeingDestroyed || this._latestSearchRequest !== requestId) return;
        return axios.post(this.api.search, JSON.stringify(opt));
      }).catch(err => {
        utility.err(`request search error: ${err}`);
        if (this._latestSearchRequest === requestId) this.$emit('searcherror', requestId);
      });

    }
  },

  watch: {
    keywordSearchCondition: {
      handler(v) {
        this.applyKeywordSearchCondition(v);
      },
      deep: true,
      immediate: true,
    },
  },

  created() {
  },
  destroyed() {
  }
};
</script>

<style scoped>
.program-search {
  display: flex;
  flex-direction: column;
  height: calc(100vh - var(--search-app-top));
  height: calc(100dvh - var(--search-app-top));
  min-height: 0;
}

.program-search > .v-input {
  flex: 0 0 auto;
}
</style>
