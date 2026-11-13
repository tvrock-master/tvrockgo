<template>
  <div class="log-setting">
    <v-card-text class="log-toolbar">
      <v-row dense align="center">
        <v-col cols="12" sm="5" md="3">
          <v-select
          v-model="selectedDate"
          :items="dateItems"
          :disabled="loading"
          item-text="text"
          item-value="value"
          label="日付"
          hide-details
          @change="changeDate"
          >
          </v-select>
        </v-col>

        <v-col cols="12" sm="4" md="2">
          <v-select
          v-model="selectedLevel"
          :items="levelItems"
          :disabled="loading"
          item-text="text"
          item-value="value"
          label="レベル"
          hide-details
          @change="changeLevel"
          >
          </v-select>
        </v-col>

        <v-col cols="12" sm="3" md="7" class="d-flex align-center">
          <v-btn icon :disabled="loading" @click="load(true)">
            <v-icon>mdi-refresh</v-icon>
          </v-btn>
          <v-progress-circular
          v-if="loading"
          indeterminate
          size="22"
          width="2"
          class="ml-2"
          >
          </v-progress-circular>
          <span class="text-caption ml-3 grey--text text--darken-1">{{ statusText }}</span>
        </v-col>
      </v-row>
    </v-card-text>

    <pre class="log-content">{{ content }}</pre>
  </div>
</template>

<script>
  import axios from 'axios'
  import utility from '../../utility.js';

  const UPDATE_INTERVAL = 5000;

  const pad = (v) => String(v).padStart(2, '0');

  const formatDateValue = (date) => {
    return String(date.getFullYear()) + pad(date.getMonth() + 1) + pad(date.getDate());
  };

  const formatDateText = (date) => {
    return String(date.getFullYear()) + '/' + pad(date.getMonth() + 1) + '/' + pad(date.getDate());
  };

  export default {
    name: 'LogSetting',

    props: [
        'api',
        'active',
    ],

    data() {
        return {
          loading: false,
          selectedDate: '',
          selectedLevel: 'info',
          dateItems: [],
          levelItems: [
            { text: '全て', value: 'all' },
            { text: 'debug', value: 'debug' },
            { text: 'info', value: 'info' },
            { text: 'error', value: 'error' },
            { text: 'warn', value: 'warn' },
          ],
          content: '',
          modTime: 0,
          size: 0,
          timer: null,
          lastLoadedAt: null,
          fileName: '',
        }
    },

    computed: {
      statusText() {
        if (this.loading) {
          return '読み込み中';
        }
        if (!this.fileName) {
          return '';
        }
        let text = this.fileName;
        if (this.size > 0) {
          text += ' / ' + this.formatSize(this.size);
        }
        if (this.lastLoadedAt) {
          text += ' / ' + this.lastLoadedAt;
        }
        return text;
      },
    },

    created() {
      this.dateItems = this.createDateItems();
      this.selectedDate = this.dateItems.length > 0 ? this.dateItems[0].value : '';
      if (this.active) {
        this.startPolling(true);
      }
    },

    beforeDestroy() {
      this.stopPolling();
    },

    methods: {
      createDateItems() {
        let items = [];
        let base = utility.now();
        base = new Date(base.getFullYear(), base.getMonth(), base.getDate());
        for (let i = 0; i <= 7; i++) {
          let d = new Date(base);
          d.setDate(base.getDate() - i);
          items.push({
            text: formatDateText(d),
            value: formatDateValue(d),
          });
        }
        return items;
      },

      changeDate() {
        this.resetLog();
        if (this.active) {
          this.load(true);
        }
      },

      changeLevel() {
        this.resetLog();
        if (this.active) {
          this.load(true);
        }
      },

      resetLog() {
        this.content = '';
        this.modTime = 0;
        this.size = 0;
        this.fileName = '';
        this.lastLoadedAt = null;
      },

      startPolling(forceLoad) {
        if (this.timer) {
          return;
        }
        if (forceLoad) {
          this.load(true);
        }
        this.timer = setInterval(() => {
          this.load(false);
        }, UPDATE_INTERVAL);
      },

      stopPolling() {
        if (this.timer) {
          clearInterval(this.timer);
          this.timer = null;
        }
      },

      load(force) {
        if (!this.active || !this.api || !this.api.log || !this.selectedDate || this.loading) {
          return;
        }

        this.loading = true;
        let params = {};
        params.level = this.selectedLevel;
        if (!force && this.modTime && this.size) {
          params.modtime = this.modTime;
          params.size = this.size;
        }

        axios.get(this.api.log + '/' + this.selectedDate, { params: params }).then(res => {
          let data = res.data || {};
          this.fileName = data.filename || '';
          if (!data.unchanged) {
            this.content = data.content || '';
            this.modTime = data.modtime || 0;
            this.size = data.size || 0;
          }
          this.lastLoadedAt = this.formatTime(utility.now());
        }).catch(err => {
          if (err.response && err.response.status === 404) {
            this.fileName = this.selectedDate + '0000.log';
            this.content = 'ログファイルがありません';
            this.modTime = 0;
            this.size = 0;
            this.lastLoadedAt = this.formatTime(utility.now());
            return;
          }
          utility.err(`request log error: ${err}`);
        }).finally(() => {
          this.loading = false;
        });
      },

      formatTime(date) {
        return pad(date.getHours()) + ':' + pad(date.getMinutes()) + ':' + pad(date.getSeconds());
      },

      formatSize(size) {
        if (size >= 1024 * 1024) {
          return (size / 1024 / 1024).toFixed(1) + ' MB';
        }
        if (size >= 1024) {
          return (size / 1024).toFixed(1) + ' KB';
        }
        return String(size) + ' B';
      },
    },

    watch: {
      active: function(v) {
        if (v) {
          this.startPolling(true);
        } else {
          this.stopPolling();
        }
      },
    },
  }
</script>

<style scoped>
.log-setting {
  height: 100%;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.log-toolbar {
  flex: 0 0 auto;
}

.log-content {
  flex: 1 1 auto;
  min-height: 0;
  margin: 0;
  padding: 12px 16px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-word;
  background: #111;
  color: #f5f5f5;
  font-family: Consolas, Menlo, Monaco, monospace;
  font-size: 12px;
  line-height: 1.5;
}
</style>
