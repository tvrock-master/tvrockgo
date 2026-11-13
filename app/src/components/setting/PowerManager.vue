<template>
  <div class="tab-full-height power-manager-setting">
    <v-card-text>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-switch
          v-model="setting.sleeptimerpause"
          :disabled="loading"
          label="スリープタイマーの一時停止"
          @change="changeBoolean('sleeptimerpause', $event)"
          >
          </v-switch>
        </v-col>
      </v-row>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-switch
          v-model="setting.preventsleep"
          :disabled="loading"
          label="スリープを抑止"
          @change="changeBoolean('preventsleep', $event)"
          >
          </v-switch>
        </v-col>
      </v-row>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-switch
          v-model="setting.preventdisplayoff"
          :disabled="loading"
          label="ディスプレイOFFを抑止"
          @change="changeBoolean('preventdisplayoff', $event)"
          >
          </v-switch>
        </v-col>
      </v-row>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-switch
          v-model="setting.preventifdevicerunning"
          :disabled="loading"
          label="デバイス起動中はスリープへ移行しない"
          @change="changeBoolean('preventifdevicerunning', $event)"
          >
          </v-switch>
        </v-col>
      </v-row>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          v-model.number="setting.wakebeforeschedule"
          :disabled="loading"
          type="number"
          min="0"
          step="1"
          label="次の予約の開始時刻から指定時間前に解除(分)(0でOFF)"
          @change="changeNumber('wakebeforeschedule')"
          @blur="changeNumber('wakebeforeschedule')"
          >
          </v-text-field>
        </v-col>
      </v-row>

      <v-row dense>
        <v-col cols="12" sm="6" md="4">
          <v-text-field
          v-model.number="setting.wakeupsuppressduration"
          :disabled="loading"
          type="number"
          min="0"
          step="1"
          label="スリープ解除後のスリープ抑制時間(秒)"
          @change="changeNumber('wakeupsuppressduration')"
          @blur="changeNumber('wakeupsuppressduration')"
          >
          </v-text-field>
        </v-col>
      </v-row>
    </v-card-text>
  </div>
</template>

<script>
  import axios from 'axios'
  import utility from '../../utility.js';

  export default {
    name: 'PowerManager',

    props: [
        'api',
        'refreshToken',
    ],

    data() {
        return {
          loading: false,
          setting: {
            sleeptimerpause: false,
            preventifdevicerunning: true,
            preventsleep: false,
            preventdisplayoff: false,
            wakebeforeschedule: 0,
            wakeupsuppressduration: 0,
          },
        }
    },

    created() {
      this.load();
    },

    methods: {
      load() {
        if (!this.api.powerManagerSetting) {
          return;
        }

        this.loading = true;
        axios.get(this.api.powerManagerSetting).then(res => {
          this.setting = this.normalize(res.data || {});
        }).catch(err => {
          utility.err(`request power manager setting error: ${err}`);
        }).finally(() => {
          this.loading = false;
        });
      },

      save() {
        if (!this.api.powerManagerSetting) {
          return;
        }

        this.loading = true;
        axios.put(this.api.powerManagerSetting, this.normalize(this.setting)).then(res => {
          this.setting = this.normalize(res.data || {});
          this.$emit('powermanagerchange');
        }).catch(err => {
          utility.err(`update power manager setting error: ${err}`);
          this.load();
        }).finally(() => {
          this.loading = false;
        });
      },

      normalize(v) {
        return {
          sleeptimerpause: !!v.sleeptimerpause,
          preventifdevicerunning: !!v.preventifdevicerunning,
          preventsleep: !!v.preventsleep,
          preventdisplayoff: !!v.preventdisplayoff,
          wakebeforeschedule: this.normalizeNumber(v.wakebeforeschedule),
          wakeupsuppressduration: this.normalizeNumber(v.wakeupsuppressduration),
        };
      },

      normalizeNumber(v) {
        const n = Number(v);
        if (Number.isFinite(n) && n >= 0) {
          return Math.floor(n);
        }
        return 0;
      },

      changeBoolean(key, value) {
        this.setting[key] = !!value;
        this.save();
      },

      changeNumber(key) {
        this.setting[key] = this.normalizeNumber(this.setting[key]);
        this.save();
      },
    },

    watch: {
      refreshToken: function() {
        this.load();
      },
    },
  }
</script>

<style scoped>
.tab-full-height {
  min-height: 100%;
}

.power-manager-setting {
  overflow: visible;
}
</style>
