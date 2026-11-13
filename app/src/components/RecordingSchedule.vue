<template>

  <div class="recordingschedule">
  <v-simple-table class="recordingschedule-table" dense fixed-header>
    <template v-slot:default>
      <thead>
        <tr class="recordingschedule-head">
          <th class="text-left">
            Record
          </th>
          <th class="text-left">
            Date
          </th>
          <th class="text-left">
            Service
          </th>
          <th class="text-left">
            Title
          </th>
          <th class="text-left">
            Device
          </th>
          <th class="text-left">
            Detail
          </th>
          <th class="text-left">
            Action
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-for="(item,index) in activeSchedule">

        <tr v-if="index === 0 || scheduleGroup(item) !== scheduleGroup(activeSchedule[index-1])"
          :key="'splitmd-' + item.id"
          :data-schedule-group="scheduleGroup(item)"
        >
          <td colspan="7" class="text-subtitle-1 recordingschedule-date">
            <div class="recordingschedule-group-heading">
              <span class="recordingschedule-group-label" :title="groupByDevice ? (item.deviceid || 'デバイス未割り当て') : item.DateMD">
                {{ groupByDevice ? (item.deviceid || 'デバイス未割り当て') : item.DateMD }}
              </span>
              <span class="recordingschedule-group-count"
                :aria-label="(groupByDevice ? 'このデバイスの予約件数' : 'この日の予約件数') + ' ' + scheduleGroupCounts.get(scheduleGroup(item)) + '件、全体 ' + activeSchedule.length + '件'">
                {{ scheduleGroupCounts.get(scheduleGroup(item)) }}/{{ activeSchedule.length }}
              </span>
              <nav v-if="scheduleOverflow" class="recordingschedule-group-nav" :aria-label="groupByDevice ? '他のデバイスへ移動' : '他の日付へ移動'">
                <v-btn v-for="group in scheduleGroups" :key="group.key"
                  class="recordingschedule-group-jump" x-small outlined
                  :disabled="group.key === scheduleGroup(item)"
                  :aria-current="group.key === scheduleGroup(item) ? 'true' : null"
                  :title="group.label" :aria-label="group.label + (group.key === scheduleGroup(item) ? '（表示中）' : 'へ移動')"
                  @click.stop="scrollToScheduleGroup(group.key)">
                  <span class="recordingschedule-group-jump-label">{{ group.label }}</span>
                </v-btn>
              </nav>
            </div>
          </td>
        </tr>

        <tr v-if="!item.complete"
          :key="item.id"
          :class="rowClass(item)"
          @click="selectReserve(item)"
        >
          <td @click.stop>
            <v-switch
            :input-value="item.validate"
            :label="label.DEBUG ? String(item.id)+':'+String(item.searchid) : ''"
            class="recordingschedule-switch ma-0 pa-0"
            dense
            hide-details
            @change="changeRecord(item.id)"
            >
            </v-switch>
          </td>
          <td class="recordingschedule-text" :title="item.DateMD + ' ' + item.DateHM"><span v-if="groupByDevice">{{ item.DateMD }} </span>{{ item.DateHM }}</td>
          <td class="recordingschedule-text" :title="item.ServiceName">{{ item.ServiceName }}</td>
          <td class="recordingschedule-text" :title="item.title">{{ item.title }}</td>
          <td class="recordingschedule-text" :title="item.deviceid">{{ item.deviceid }}</td>
          <td>
            <v-btn
              class="recordingschedule-detail-toggle"
              small
              outlined
              @click.stop="toggleDetail(item.id)"
            >
              <v-icon left small>{{ expandedID === item.id ? 'mdi-chevron-up' : 'mdi-chevron-down' }}</v-icon>
              詳細
            </v-btn>
          </td>
          <td>
            <v-btn
              v-if="isRecording(item)"
              color="error"
              x-small
              :loading="stoppingScheduleID === item.id"
              @click.stop="stopRecording(item.id)"
            >
              <v-icon left small>mdi-stop</v-icon>
              Stop
            </v-btn>

            <v-btn icon small v-if="!isRecording(item) && item.searchid===0" @click.stop="deleteSchedule(item)">
              <v-icon small>mdi-delete</v-icon>
            </v-btn>

            <v-btn icon small v-if="!isRecording(item) && item.searchid!==0" @click.stop="keywordReserve(item.searchid)">
              <v-icon small>mdi-key-variant</v-icon>
            </v-btn>

          </td>
        </tr>
        <tr
          v-if="expandedID === item.id"
          :key="'detail-' + item.id"
          class="recordingschedule-detail-row"
          @click.stop
        >
          <td colspan="6">
            <div class="recordingschedule-detail">
              <!-- v-text-field
                v-model="item.deviceid"
                class="recordingschedule-detail-text"
                label="デバイスID"
                v-bind:disabled="item.autoselect || !canEditSchedule(item)"
                dense
                hide-details
                clearable
              ></v-text-field -->
              <v-select
                v-model="item.deviceid"
                class="recordingschedule-detail-text"
                :items="deviceList"
                item-text="text"
                item-value="value"
                label="デバイスID"
                v-bind:disabled="item.autoselect || !canEditSchedule(item)"
                dense
                hide-details
              ></v-select>

              <v-text-field
                v-model="item.title"
                class="recordingschedule-detail-title"
                label="タイトル"
                v-bind:disabled="!canEditSchedule(item)"
                dense
                hide-details
              ></v-text-field>

              <v-select
                v-model="item.customcomid"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-text"
                :items="customCommandList"
                item-text="text"
                item-value="value"
                label="カスタムコマンド"
                dense
                hide-details
              ></v-select>
              <!-- v-text-field
                v-model="item.filename"
                class="recordingschedule-detail-file"
                label="FileName"
                dense
                hide-details
                clearable
              ></v-text-field -->
              <div class="recordingschedule-detail-break"></div>
              <v-text-field
                v-model.number="item.idletime"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-number"
                label="待機時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.readytime"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-number"
                label="録画開始時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.tailtime"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-number"
                label="録画終了時間(秒)"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-text-field
                v-model.number="item.priority"
                 v-bind:disabled="!canEditSchedule(item)"
               class="recordingschedule-detail-number"
                label="プライオリティ"
                type="number"
                dense
                hide-details
              ></v-text-field>
              <v-select
                v-model.number="item.exitmode"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-number"
                :items="exitModeOptions"
                item-text="text"
                item-value="value"
                label="終了処理"
                dense
                hide-details
              ></v-select>
             <div class="recordingschedule-detail-break"></div>
              <v-checkbox
                v-model="item.watchonly"
                v-bind:disabled="!canEditSchedule(item)||item.recordonly"
                class="recordingschedule-detail-check"
                label="視聴のみ"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.recordonly"
                v-bind:disabled="!canEditSchedule(item)||item.watchonly"
                class="recordingschedule-detail-check"
                label="録画のみ"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.shutdown"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-check"
                label="録画終了後アプリ終了"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.epgfollow"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-check"
                label="番組時間追従"
                dense
                hide-details
              ></v-checkbox>
              <v-checkbox
                v-model="item.autoselect"
                v-bind:disabled="!canEditSchedule(item)"
                class="recordingschedule-detail-check"
                label="デバイス自動選択"
                dense
                hide-details
              ></v-checkbox>

              <v-btn v-if="canEditSchedule(item)"
                class="recordingschedule-detail-save"
                color="primary"
                small
                :loading="savingID === item.id"
                @click.stop="saveSchedule(item)"
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
  import label from '../define.js';
  import utility from '../utility.js';
  import axios from 'axios';
  import ConfirmDialog from './ConfirmDialog.vue';

  const fetchProgramCustomID = [
    "FetchProgramTS",
    "FetchProgramBS",
    "FetchProgramCS",
    "FetchProgramOS",
  ];

  export default {
    name: 'RecordingSchedule',

    components: {
      ConfirmDialog,
    },

    props: [
        'schedule',
        'station',
        'api',
        'deviceList',
        'configure',
        'groupByDevice',
        'recordingStatusRevision',
    ],

    computed: {
      exitModeOptions() {
        return [
          { text: '何もしない', value: 0 },
          { text: 'アプリ終了', value: 1 },
        ];
      },

      activeSchedule() {
        if (!this.schedule) {
          return [];
        }
        let now = utility.tmtoux(utility.now());
        return this.schedule.filter(item => !item.complete && item.endtime >= now).sort((a, b) => {
          if (this.groupByDevice) {
            const aDevice = a.deviceid || '';
            const bDevice = b.deviceid || '';
            if (aDevice !== bDevice) {
              if (!aDevice) return 1;
              if (!bDevice) return -1;
              return aDevice.localeCompare(bDevice, 'ja', { numeric: true }) || (aDevice < bDevice ? -1 : 1);
            }
          }
          return a.starttime - b.starttime || a.id - b.id;
        });
      },

      recordingIDMap() {
        let ret = {};
        for (let i = 0; i < this.recordingIDs.length; i++) {
          ret[this.recordingIDs[i]] = true;
        }
        return ret;
      },

      scheduleGroups() {
        const groups = new Map();
        for (const item of this.activeSchedule) {
          const key = this.scheduleGroup(item);
          if (!groups.has(key)) groups.set(key, {
            key,
            label: this.groupByDevice ? (item.deviceid || 'デバイス未割り当て') : item.DateMD,
          });
        }
        return Array.from(groups.values());
      },

      scheduleGroupCounts() {
        const counts = new Map();
        for (const item of this.activeSchedule) {
          const key = this.scheduleGroup(item);
          counts.set(key, (counts.get(key) || 0) + 1);
        }
        return counts;
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
      updateScheduleOverflow() {
        const container = this.$el && this.$el.querySelector('.v-data-table__wrapper');
        this.scheduleOverflow = !!(container && container.clientHeight > 0 &&
          container.scrollHeight > container.clientHeight + 1);
      },

      scheduleGroup(item) {
        return this.groupByDevice ? (item.deviceid || '') : item.DateMD;
      },

      scrollToScheduleGroup(key) {
        this.$nextTick(() => {
          const container = this.$el.querySelector('.v-data-table__wrapper');
          if (!container) return;
          const row = Array.from(container.querySelectorAll('[data-schedule-group]'))
            .find(element => element.dataset.scheduleGroup === key);
          if (!row) return;
          const header = container.querySelector('thead');
          const headerHeight = header ? header.getBoundingClientRect().height : 0;
          // Measure the row rather than its sticky cell to use its original position.
          const top = container.scrollTop + row.getBoundingClientRect().top -
            container.getBoundingClientRect().top - container.clientTop - headerHeight;
          container.scrollTo({
            top: Math.max(0, top),
            behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
          });
        });
      },

      isRecording(item) {
        return !!(item && this.recordingIDMap[item.id]);
      },

      canEditSchedule(item) {
        if (!item || item.searchid !== 0) {
          return false
        }
        return !fetchProgramCustomID.some(cid => cid === item.customcomid)
      },

      rowClass(item) {
        if (this.isRecording(item)) {
          return item.watchonly && !item.recordonly ? 'recordingschedule-watch' : 'recordingschedule-recording';
        }
        return item.validate ? (item.complete ? 'recordingschedule-comp' : '') : 'recordingschedule-off';
      },

      async updateRecordingStatus() {
        if (!this.api || !this.api.recordingStatus) {
          return;
        }
        if(this.recordingStatusLoading){this.recordingStatusPending=true;return;}
        this.recordingStatusLoading=true;
        try {const res=await axios.get(this.api.recordingStatus);
          if(this._isBeingDestroyed || this._isDestroyed)return;
          this.recordingIDs = res.data && res.data.recording ? res.data.recording : [];
        } catch(err) {
          utility.err(`request recording status error: ${err}`);
        } finally {
          this.recordingStatusLoading=false;
          if(this.recordingStatusPending && !this._isBeingDestroyed && !this._isDestroyed){this.recordingStatusPending=false;this.updateRecordingStatus();}
        }
      },

      async stopRecording(id) {
        const confirmed = await this.$refs.confirmDialog.open({
          title: '録画停止',
          message: '録画中の番組を停止します。よろしいですか？',
          okText: '停止',
          okColor: 'error',
        });
        if (!confirmed) {
          return;
        }

        this.stoppingScheduleID = id;
        axios.delete(this.api.schedule + '/stop/' + id).then(res => {
          utility.log("api schedule stop results:" + res.status);
          this.updateRecordingStatus();
        }).catch(err => {
          utility.err(`request schedule stop error: ${err}`);
        }).finally(() => {
          this.stoppingScheduleID = null;
        });
      },

      toggleDetail(id) {
        this.expandedID = this.expandedID === id ? null : id;
      },

      saveSchedule(item) {
        if (!this.canEditSchedule(item)) {
          return;
        }

        this.savingID = item.id;
        const val = {
          id: item.id,
          deviceid: item.deviceid || '',
          title: item.title || '',
          customcomid: item.customcomid || '',
//          filename: item.filename || '',
          validate: !!item.validate,
          watchonly: !!item.watchonly,
          recordonly: !!item.recordonly,
          shutdown: !!item.shutdown,
          epgfollow: !!item.epgfollow,
          autoselect: !!item.autoselect,
          idletime: Number(item.idletime) || 0,
          readytime: Number(item.readytime) || 0,
          tailtime: Number(item.tailtime) || 0,
          priority: Number(item.priority) || 0,
          exitmode: Number(item.exitmode) || 0,
        };

        axios.put(this.api.schedule, JSON.stringify(val)).then(res => {
          utility.log("api schedule update results:" + res.status);
          this.updateRecordingStatus();
        }).catch(err => {
          utility.err(`request schedule update error: ${err}`);
        }).finally(() => {
          this.savingID = null;
        });
      },

      async deleteSchedule(item) {
        const id = item.id;
        const confirmed = await this.$refs.confirmDialog.open({
          title: '予約削除',
          message: `「${item.title || ''}」を削除します。よろしいですか？`,
          okText: '削除',
          okColor: 'error',
        });
        if (!confirmed) {
          return;
        }

        axios.delete(this.api.schedule + '/' + id).then(res => {
          utility.log("api delete schedule results:" + res.status);
          this.updateRecordingStatus();
        }).catch(err => {
          utility.err(`request delete schedule error: ${err}`);
        }).finally(() => {
          this.stoppingScheduleID = null;
        });
      },

      keywordReserve(id) {
        axios.get(this.api.keyword+'/'+id).then(res => {
          this.$emit('programsearch', res.data);
        }).catch(err => {
          utility.err(`request keyword reserve error: ${err}`);
        });
      },

      // サービスのon/off
      changeRecord(id) {
        let sch = this.schedule;
        for (let i = 0; i < sch.length; i++) {
          if (id == sch[i].id) {
            let val = this.schedule[i];
            val.validate = !val.validate;
            axios.put(this.api.schedule, JSON.stringify(val)).then(res => {
              utility.log(res.status);
              // API updateを行った時点でwsから通知が来て再描画
            }).catch(err => {
              utility.err(`request schedule error: ${err}`);
            });
            break;
          }
        }
      },

      selectReserve(item) {
        this.$emit('select', item);
      },
    },

    data() {
        return {
          label: null,
          utility: null,
          recordingIDs: [],
          recordingStatusLoading: false,
          recordingStatusPending: false,
          stoppingScheduleID: null,
          expandedID: null,
          savingID: null,
          splitMD: null,
          scheduleOverflow: false,
        }
    },

    mounted() {
      this.updateRecordingStatus();
      this.updateScheduleOverflow();
      const container = this.$el.querySelector('.v-data-table__wrapper');
      if (container && typeof ResizeObserver !== 'undefined') {
        this._scheduleResizeObserver = new ResizeObserver(this.updateScheduleOverflow);
        this._scheduleResizeObserver.observe(container);
        const table = container.querySelector('table');
        if (table) this._scheduleResizeObserver.observe(table);
      }
      window.addEventListener('resize', this.updateScheduleOverflow);
    },
    updated() {
      this.updateScheduleOverflow();
    },
    beforeDestroy() {
      if (this._scheduleResizeObserver) this._scheduleResizeObserver.disconnect();
      window.removeEventListener('resize', this.updateScheduleOverflow);
    },
    watch: {
      recordingStatusRevision() {this.updateRecordingStatus();},
    },

    created() {
      this.label = label;
      this.utility = utility;
    }

}
</script>

<style scoped>

.recordingschedule-table {
  --recordingschedule-header-height: 32px;
}

.recordingschedule-table ::v-deep .v-data-table__wrapper {
  max-height: calc(100vh - 64px);
  overflow-y: auto;
}

.recordingschedule-head {
  color: var(--app-text, #202020F0);
  background-color: var(--app-table-header, rgb(140, 180, 220));
  background-image: linear-gradient(to right, transparent 75%, rgba(0, 0, 0, 0.12) 100%);
}

/* Keep one continuous gradient across the sticky header row. */
.recordingschedule-table ::v-deep thead {
  position: sticky;
  top: 0;
  z-index: 3;
}

.recordingschedule-table ::v-deep .v-data-table__wrapper > table > thead > tr > th {
  height: var(--recordingschedule-header-height);
  background-color: transparent !important;
  color: var(--app-text, #404040F0) !important;
  position: sticky;
  top: 0;
  z-index: 3;
  box-shadow: 0 1px 0 rgba(0, 0, 0, 0.12);
}

.recordingschedule-table ::v-deep th,
.recordingschedule-table ::v-deep td {
  height: 34px;
  padding-top: 2px;
  padding-bottom: 2px;
  line-height: 1.25;
}

.recordingschedule-switch {
  min-height: 28px;
}

.recordingschedule-switch ::v-deep .v-input__control,
.recordingschedule-switch ::v-deep .v-input__slot {
  min-height: 28px;
}

.recordingschedule-off {
  background-color: var(--app-disabled, rgb(150, 150, 150));
}
.recordingschedule-comp {
  background-color: var(--app-complete, rgb(175, 135, 135));
}
.recordingschedule-recording {
  background-color: var(--app-recording, rgb(226, 188, 188));
  color: black;
}

.recordingschedule-watch {
  background-color: var(--app-watch, rgb(255, 235, 156));
}

.recordingschedule-table.theme--dark .recordingschedule-recording,
.recordingschedule-table.theme--dark .recordingschedule-watch {
  color: #ffffff;
}

.recordingschedule-detail-toggle {
  min-width: 72px;
  margin-right: 4px;
}

.recordingschedule-detail-toggle.theme--light {
  border-color: rgba(0, 0, 0, 0.12);
}

.recordingschedule-detail-toggle.theme--dark {
  border-color: rgba(255, 255, 255, 0.12);
}

.recordingschedule-detail-row {
  background: var(--app-surface-soft, rgba(245, 248, 252, 0.95));
}

.recordingschedule-detail {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px 12px;
  padding: 10px 0;
}

.recordingschedule-detail-text {
  max-width: 180px;
}

.recordingschedule-detail-title {
  max-width: 320px;
}

.recordingschedule-detail-file {
  max-width: 320px;
}

.recordingschedule-detail-number {
  max-width: 140px;
}

.recordingschedule-detail-break {
  flex-basis: 100%;
  height: 0;
}

.recordingschedule-detail-check {
  margin-top: 0;
}

.recordingschedule-detail-save {
  margin-left: auto;
}

.recordingschedule-table ::v-deep .recordingschedule-date {
  position: sticky;
  top: var(--recordingschedule-header-height);
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

.recordingschedule-group-heading {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 24px;
}

.recordingschedule-group-count {
  flex: 0 0 auto;
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.recordingschedule-group-label {
  flex: 0 1 auto;
  min-width: 0;
  max-width: 240px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.recordingschedule-group-nav {
  display: flex;
  align-items: center;
  flex: 1 1 0;
  width: 0;
  min-width: 0;
  gap: 4px;
  overflow-x: auto;
  padding: 2px 0;
  scrollbar-width: thin;
}

.recordingschedule-group-jump {
  flex: 0 0 auto;
  max-width: 180px;
}

.recordingschedule-group-jump:first-child {
  margin-left: auto;
}

.recordingschedule-group-jump.v-btn--disabled {
  background-color: var(--app-surface-soft, rgba(128, 128, 128, 0.12)) !important;
}

.recordingschedule-group-jump-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 1263px) {
  .recordingschedule-table ::v-deep table {
    table-layout: fixed;
    min-width: 640px;
  }

  .recordingschedule-table ::v-deep th,
  .recordingschedule-text,
  .recordingschedule-date {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .recordingschedule-table ::v-deep th:nth-child(1) { width: 64px; }
  .recordingschedule-table ::v-deep th:nth-child(2) { width: 160px; }
  .recordingschedule-table ::v-deep th:nth-child(3),
  .recordingschedule-table ::v-deep th:nth-child(5) { width: 12%; }
  .recordingschedule-table ::v-deep th:nth-child(6) { width: 112px; }
  .recordingschedule-table ::v-deep th:nth-child(7) { width: 64px; }
}
</style>
