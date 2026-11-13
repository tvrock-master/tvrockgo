<template>
  <div class="record-log">
    <v-alert v-if="playbackMessage" :type="playbackError ? 'error' : 'info'" dense dismissible @input="playbackMessage = ''">{{ playbackMessage }}</v-alert>
    <v-simple-table class="record-log-table" dense>
      <template v-slot:default>
        <thead>
          <tr class="record-log-head">
            <th class="text-left">
              Record
            </th>
            <th class="text-left">Playback</th>
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
              Errors
            </th>
          </tr>
        </thead>
        <tbody>
          <template v-for="(item,index) in recordLogs">
            <tr
              v-if="(index > 0 && recordLogs[index].DateMD !== recordLogs[index-1].DateMD) || index === 0"
              :key="'splitmd-' + item.id"
            >
              <td colspan="7" class="text-subtitle-1 record-log-date">
                {{ item.DateMD }}
              </td>
            </tr>

            <tr
              :key="item.id"
              :class="rowClass(item)"
              @click="selectReserve(item)"
            >
              <td>
                <v-icon :class="statusIconClass(item)" small>
                  {{ statusIcon(item) }}
                </v-icon>
              </td>
              <td>
                <v-btn icon small :disabled="!item.filename || item.watchonly || playbackLoading !== null" :loading="playbackLoading === item.id" :aria-label="item.title + 'を再生'" title="接続中の全Rockbarで再生" @click.stop="playRecording(item)">
                  <v-icon small>mdi-play</v-icon>
                </v-btn>
              </td>
              <td>{{ item.DateHM }}</td>
              <td>{{ item.ServiceName }}</td>
              <td>{{ item.title }}</td>
              <td>{{ item.deviceid }}</td>
              <td>
                <v-chip
                  v-if="hasErrors(item)"
                  color="error"
                  text-color="white"
                  x-small
                >
                  {{ formatErrors(item) }}
                </v-chip>
                <span v-else>{{ formatErrors(item) }}</span>
              </td>
            </tr>
          </template>
        </tbody>
      </template>
    </v-simple-table>
  </div>
</template>

<script>
  import axios from 'axios';
  import utility from '../../utility.js';

  export default {
    name: 'RecordLog',

    props: [
        'schedule',
        'api',
    ],
    data() { return { playbackLoading: null, playbackMessage: '', playbackError: false }; },

    computed: {
      recordLogs() {
        if (!this.schedule) {
          return [];
        }
        let now = utility.tmtoux(utility.now());
        return this.schedule
          .filter(item => item.complete || item.endtime <= now)
          .slice()
          .sort((a, b) => b.starttime - a.starttime);
      },
    },

    methods: {
      async playRecording(item) {
        this.playbackLoading = item.id;
        this.playbackMessage = '';
        try {
          await axios.post(this.api.playback, { scheduleId: item.id });
          this.playbackError = false;
          this.playbackMessage = '接続中のRockbarで再生を準備しています。';
        } catch (error) {
          this.playbackError = true;
          this.playbackMessage = error.response?.data?.error || '再生を開始できませんでした。';
        } finally { this.playbackLoading = null; }
      },
      hasErrors(item) {
        return Number(item && item.errors) > 0;
      },

      formatErrors(item) {
        return String(Number(item && item.errors) || 0);
      },

      rowClass(item) {
        if (!item || !item.complete) {
          return 'record-log-failed';
        }
        if (this.hasErrors(item)) {
          return 'record-log-warning';
        }
        return 'record-log-complete';
      },

      statusIcon(item) {
        if (!item || !item.complete) {
          return 'mdi-close';
        }
        if (this.hasErrors(item)) {
          return 'mdi-triangle-outline';
        }
        return 'mdi-circle-outline';
      },

      statusIconClass(item) {
        if (!item || !item.complete) {
          return 'record-log-status-failed';
        }
        if (this.hasErrors(item)) {
          return 'record-log-status-warning';
        }
        return 'record-log-status-complete';
      },

      selectReserve(item) {
        this.$emit('select', item);
      },
    },
  }
</script>

<style scoped>
.record-log-head {
  color: var(--app-text, #202020F0);
  background-color: var(--app-table-header, rgb(140, 180, 220, 0.9));
}

.record-log-table ::v-deep th,
.record-log-table ::v-deep td {
  height: 34px;
  padding-top: 2px;
  padding-bottom: 2px;
  line-height: 1.25;
}

.record-log-table ::v-deep tbody tr {
  cursor: pointer;
}

.record-log-complete {
  background-color: var(--app-surface-soft, rgb(245, 248, 252));
}

.record-log-warning {
  background-color: var(--app-watch, rgb(255, 245, 157));
}

.record-log-failed {
  background-color: var(--app-recording, rgb(226, 188, 188));
}

.record-log-status-complete {
  color: var(--app-success-text, #2e7d32);
}

.record-log-status-warning {
  color: var(--app-warning-text, #f57f17);
}

.record-log-status-failed {
  color: var(--app-error-text, #b71c1c);
}

.record-log-date {
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

</style>
