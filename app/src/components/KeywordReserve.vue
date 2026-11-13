<template>
  <div class="keywordreserve">
  <v-simple-table class="keywordreserve-table" dense fixed-header>
    <template v-slot:default>
      <thead>
        <tr class="keywordreserve-head">
          <th class="text-left">
            Enable
          </th>
          <th class="text-left">
            Title
          </th>
          <th class="text-left">
            Content
          </th>
          <th class="text-left">
            Service
          </th>
          <th class="text-left">
            Category
          </th>
          <th class="text-left">
            AND
          </th>
          <th class="text-left">
            Fuzzy
          </th>
          <th class="text-left">
            Detail
          </th>
          <th class="text-left">
            Reserved
          </th>
          <th class="text-left">
            Action
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-for="(item, index) in displayedKeywordReserves">
        <tr
          v-if="index === firstUnreservedKeywordIndex && keywordReserveThresholdDays !== null"
          :key="'unreserved-keywords-' + item.id"
        >
          <td colspan="10" class="keywordreserve-separator">
            予約が０件のキーワードは{{ keywordReserveThresholdDays }}日経過すると削除されます
          </td>
        </tr>
        <tr
          :key="item.id"
          :class="item.validate ? '' : 'keywordreserve-off'"
          class="keywordreserve-row"
          @click="selectResults(item)"
        >
          <td @click.stop>
            <v-switch
              :input-value="item.validate"
              :label="label.DEBUG ? ''+item.id : ''"
              class="keywordreserve-switch ma-0 pa-0"
              dense
              hide-details
              @change="changeValidate(item)"
            >
            </v-switch>
          </td>
          <td class="keywordreserve-text" :title="item.title">{{ item.title }}</td>
          <td class="keywordreserve-text" :title="item.content">{{ item.content }}</td>
          <td class="keywordreserve-text" :title="stationNames(item.sid)">{{ stationNames(item.sid) }}</td>
          <td class="keywordreserve-text" :title="categoryNames(item.category)"><span class="keywordreserve-category">{{ categoryNames(item.category) }}</span></td>
          <td><v-icon left small>{{ item.andsearch ? 'mdi-check-circle' : '' }}</v-icon></td>
          <td><v-icon left small>{{ item.fuzzysearch ? 'mdi-check-circle' : '' }}</v-icon></td>
          <td @click.stop>
            <v-btn
              class="keywordreserve-detail-toggle"
              small
              outlined
              @click.stop="toggleDetail(item.id)"
            >
              <v-icon left small>{{ expandedID === item.id ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
              詳細
            </v-btn>
          </td>
          <td>{{ scheduleCount(item.id) }}</td>
          <td @click.stop>
            <v-btn icon small @click.stop="deleteReserve(item)">
              <v-icon small>mdi-delete</v-icon>
            </v-btn>
          </td>
        </tr>
        <tr
          v-if="expandedID === item.id"
          :key="'detail-' + item.id"
          class="keywordreserve-detail-row"
          @click.stop
        >
          <td colspan="10">
            <div class="keywordreserve-detail">
              <v-select
                v-model="item.sid"
                class="keywordreserve-detail-option"
                :items="stationOptions"
                item-text="text"
                item-value="value"
                label="局名"
                multiple
                dense
                hide-details
                clearable
              ></v-select>
              <v-select
                v-model="item.selectedCategories"
                class="keywordreserve-detail-option"
                :items="categoryOptions"
                item-text="text"
                item-value="value"
                label="ジャンル"
                multiple
                dense
                hide-details
                clearable
              ></v-select>
              <v-select
                v-model="item.deviceid"
                class="keywordreserve-detail-custom"
                :items="deviceList"
                item-text="text"
                item-value="value"
                label="デバイスID"
                dense
                hide-details
                clearable
              ></v-select>
              <v-select
                :value="weekMaskToList(item.week)"
                class="keywordreserve-detail-option keywordreserve-detail-weekdays"
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
                @input="$set(item, 'week', weekListToMask($event))"
              ></v-select>
              <v-select
                v-model="item.days"
                class="keywordreserve-detail-days"
                :items="dayOptions"
                item-text="text"
                item-value="value"
                label="検索日数"
                dense
                hide-details
              ></v-select>
              <v-checkbox
                v-model="item.andsearch"
                class="keywordreserve-detail-check"
                label="AND"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.fuzzysearch"
                class="keywordreserve-detail-check"
                label="あいまい"
                dense
                hide-details
              ></v-checkbox>
              <div class="keywordreserve-detail-break"></div>
              <v-text-field
                v-model.number="item.starthours"
                class="keywordreserve-detail-number"
                label="検索開始時刻"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.hours"
                class="keywordreserve-detail-number"
                label="検索時間"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.idletime"
                class="keywordreserve-detail-number"
                label="待機時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.readytime"
                class="keywordreserve-detail-number"
                label="録画開始時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.tailtime"
                class="keywordreserve-detail-number"
                label="録画終了時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-select
                v-model="item.customcomid"
                class="keywordreserve-detail-custom"
                label="カスタムコマンド"
                :items="customCommandList"
                item-text="text"
                item-value="value"
                dense
                hide-details
              ></v-select>
              <v-text-field
                v-model.number="item.hashtag"
                class="keywordreserve-detail-custom"
                label="ハッシュタグ"
                dense
                hide-details
              ></v-text-field>
              <div class="keywordreserve-detail-break"></div>
              <v-checkbox
                v-model="item.recordonly"
                v-bind:disabled="item.watchonly"
                class="keywordreserve-detail-check"
                label="録画のみ"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.watchonly"
                v-bind:disabled="item.recordonly"
                class="keywordreserve-detail-check"
                label="視聴のみ"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.shutdown"
                class="keywordreserve-detail-check"
                label="録画終了後アプリ終了"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.epgfollow"
                class="keywordreserve-detail-check"
                label="番組時間追従"
                dense
                hide-details
              ></v-checkbox>
              <div>最終更新日時:{{utility.uxtmtomdhmj(item.updatedat)}}</div>

              <v-btn
                class="keywordreserve-detail-save"
                color="primary"
                small
                :loading="savingID === item.id"
                @click.stop="saveReserve(item)"
              >
                <v-icon left small>mdi-content-save</v-icon>
                保存
              </v-btn>
            </div>
          </td>
        </tr>
        </template>
      </tbody>
    </template>
  </v-simple-table>
  <confirm-dialog ref="confirmDialog"></confirm-dialog>
  </div>
</template>

<script>
import axios from 'axios'
import label from '../define.js';
import utility from '../utility.js';
import ConfirmDialog from './ConfirmDialog.vue';

export default {
  name: "KeywordReserve",
  components: {
    ConfirmDialog,
  },
  props: [
    'api',
    'schedule',
    'station',
    'deviceList',
    'configure',
    'refreshToken',
  ],
  data() {
    return {
      label: null,
      utility: null,
      keywordReserves: [],
      expandedID: null,
      savingID: null,
    };
  },
  computed: {
    displayedKeywordReserves() {
      const reserved = [];
      const unreserved = [];
      for (const item of this.keywordReserves) {
        (this.scheduleCount(item.id) > 0 ? reserved : unreserved).push(item);
      }
      return reserved.concat(unreserved);
    },

    firstUnreservedKeywordIndex() {
      return this.displayedKeywordReserves.findIndex(item => this.scheduleCount(item.id) === 0);
    },

    keywordReserveThresholdDays() {
      const seconds = this.configure && this.configure.CurrentKeywordReserveThreshold;
      return seconds == null ? null : seconds / (24 * 60 * 60);
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
        .sort((a, b) => a.No - b.No)
        .map(item => ({
          text: item.Name + ' (' + (item.Sid & 0xffff) + ')',
          value: item.Sid,
        }));
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
    weekMaskToList: utility.weekMaskToList,
    weekListToMask: utility.weekListToMask,
    updateKeywordReserve() {
      axios.get(this.api.keyword+'/0').then(res => {
        this.keywordReserves = res.data;

        this.keywordReserves.sort((a,b) => {
          if (a.updatedat > b.updatedat) return -1;
          else if (a.updatedat < b.updatedat) return 1;
          return 0;
        });

      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      });
    },

    changeValidate(item) {
      let val = Object.assign({}, item);
      val.validate = !val.validate;
      axios.put(this.api.keyword, JSON.stringify(val)).then(() => {
        this.updateKeywordReserve();
      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      });
    },

    toggleDetail(id) {
      const item = this.keywordReserves.find(v => v.id === id);
      if (item && !Object.prototype.hasOwnProperty.call(item, 'selectedCategories')) {
        this.$set(item, 'selectedCategories', this.categoryMaskToList(item.category));
      }
      this.expandedID = this.expandedID === id ? null : id;
    },

    saveReserve(item) {
      this.savingID = item.id;
      const val = Object.assign({}, item);
      val.category = this.categoryListToMask(item.selectedCategories);
      delete val.selectedCategories;
      axios.put(this.api.keyword, JSON.stringify(val)).then(() => {
        this.updateKeywordReserve();
      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      }).finally(() => {
        this.savingID = null;
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

    categoryListToMask(list) {
      if (!list || list.length < 1) {
        return 0xffff;
      }
      return list.reduce((mask, v) => mask | (1 << v), 0);
    },

    async deleteReserve(item) {
      const id = item.id;
      const confirmed = await this.$refs.confirmDialog.open({
        title: 'キーワード予約削除',
        message: `タイトル：「${item.title || ''}」\nコンテンツ：「${item.content || ''}」\nを削除します。よろしいですか？`,
        okText: '削除',
        okColor: 'error',
      });
      if (!confirmed) {
        return;
      }

      axios.delete(this.api.keyword + '/' + id).then(res => {
        this.keywordReserves = res.data;
        this.$emit('keywordreservedelete', id);
      }).catch(err => {
        utility.err(`request keyword reserve error: ${err}`);
      });
    },

    selectResults(item) {
      this.$emit('select', item);
    },

    scheduleCount(searchID) {
      if (!this.schedule) {
        return 0;
      }
      const now = Math.floor(utility.now() / 1000);      
      return this.schedule.filter(item => item.searchid === searchID && item.endtime > now).length;
    },

    stationNames(sid) {
      if (!sid || sid.length < 1) {
        return '全局';
      }
      if (!this.station || !this.station.all) {
        return sid.join(', ');
      }
      return sid.map(id => this.station.all[id] ? this.station.all[id].Name : id).join(', ');
    },

    categoryNames(category) {
      if (!category || category === 0xffff) {
        return '全ジャンル';
      }
      const names = [
        'ニュース／報道',
        'スポーツ',
        '情報／ワイドショー',
        'ドラマ',
        '音楽',
        'バラエティ',
        '映画',
        'アニメ／特撮',
        'ドキュメンタリー／教養',
        '劇場／公演',
        '趣味／教育',
        '福祉',
        '事業体定義／予備１',
        '事業体定義／予備２',
        '拡張（拡張用フラグ）／予備',
        'その他',
      ];
      let ret = [];
      for (let i = 0; i < names.length; i++) {
        if ((category & (1 << i)) !== 0) {
          ret.push(names[i]);
        }
      }
      return ret.join(', ');
    },
  },

  created() {
    this.updateKeywordReserve();
    this.label = label;
    this.utility = utility;
  },
  
  watch: {
    refreshToken() {
      this.updateKeywordReserve();
    },
  },
};
</script>

<style scoped>

.keywordreserve-table {
  --keywordreserve-header-height: 32px;
}

.keywordreserve-table ::v-deep .v-data-table__wrapper {
  max-height: calc(100vh - 64px);
  overflow-y: auto;
}

.keywordreserve-head {
  color: var(--app-text, #202020F0);
  background-color: var(--app-table-header, rgb(140, 180, 220));
  background-image: linear-gradient(to right, transparent 75%, rgba(0, 0, 0, 0.12) 100%);
}

/* Keep one continuous gradient across the sticky header row. */
.keywordreserve-table ::v-deep thead {
  position: sticky;
  top: 0;
  z-index: 3;
}

.keywordreserve-table ::v-deep thead th {
  height: var(--keywordreserve-header-height);
  background-color: transparent !important;
  color: var(--app-text, #404040F0) !important;
  position: sticky;
  top: 0;
  z-index: 3;
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.12);
}

.keywordreserve-table ::v-deep th,
.keywordreserve-table ::v-deep td {
  height: 34px;
  padding-top: 2px;
  padding-bottom: 2px;
  line-height: 1.25;
}

.keywordreserve-switch {
  min-height: 28px;
}

.keywordreserve-switch ::v-deep .v-input__control,
.keywordreserve-switch ::v-deep .v-input__slot {
  min-height: 28px;
}

.keywordreserve-off {
  background-color: var(--app-disabled, rgb(150, 150, 150));
}

.keywordreserve-row {
  cursor: pointer;
}

.keywordreserve-table ::v-deep .keywordreserve-separator {
  position: sticky;
  top: var(--keywordreserve-header-height);
  z-index: 2;
  background-color: var(--app-surface, #fff);
  height: auto !important;
  padding-top: 8px !important;
  padding-bottom: 2px;
  line-height: 1.25;
  color: var(--app-text-secondary, #606060F0);
  border-bottom: none !important;
  background-image: linear-gradient(to right, rgba(0, 0, 0, 0.25) 0%, rgba(0, 0, 0, 0.25) 50%, transparent 100%);
  background-size: 100% 2px;
  background-repeat: no-repeat;
  background-position: bottom;
}

.keywordreserve-detail-toggle {
  min-width: 72px;
}

.keywordreserve-detail-toggle.theme--light {
  border-color: rgba(0, 0, 0, 0.12);
}

.keywordreserve-detail-toggle.theme--dark {
  border-color: rgba(255, 255, 255, 0.12);
}

.keywordreserve-detail-row {
  background: var(--app-surface-soft, rgba(245, 248, 252, 0.95));
}

.keywordreserve-detail {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 10px 0;
}

.keywordreserve-detail-number {
  max-width: 140px;
}

.keywordreserve-detail-option {
  max-width: 280px;
}

.keywordreserve-detail-weekdays {
  max-width: 140px;
}

.keywordreserve-detail-days {
  max-width: 120px;
}

.keywordreserve-detail-break {
  flex-basis: 100%;
  height: 0;
}

.keywordreserve-detail-custom {
  max-width: 180px;
}

.keywordreserve-detail-check {
  margin-top: 0;
}

.keywordreserve-detail-save {
  margin-left: auto;
}

.keywordreserve-category {
  display: block;
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1263px) {
  .keywordreserve-table ::v-deep table {
    table-layout: fixed;
    min-width: 760px;
  }

  .keywordreserve-table ::v-deep th,
  .keywordreserve-text,
  .keywordreserve-separator {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .keywordreserve-table ::v-deep th,
  .keywordreserve-row > td {
    padding-left: 8px;
    padding-right: 8px;
  }

  .keywordreserve-table ::v-deep th:nth-child(1) { width: 60px; }
  .keywordreserve-table ::v-deep th:nth-child(3) { width: 16%; }
  .keywordreserve-table ::v-deep th:nth-child(4),
  .keywordreserve-table ::v-deep th:nth-child(5) { width: 11%; }
  .keywordreserve-table ::v-deep th:nth-child(6) { width: 44px; }
  .keywordreserve-table ::v-deep th:nth-child(7) { width: 48px; }
  .keywordreserve-table ::v-deep th:nth-child(8) { width: 112px; }
  .keywordreserve-table ::v-deep th:nth-child(9) { width: 64px; }
  .keywordreserve-table ::v-deep th:nth-child(10) { width: 56px; }
}
</style>
