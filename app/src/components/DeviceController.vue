<template>

    <v-card>

    <v-container fluid fill-height>
      <div class="device-pin" v-for="item in device" :key="item.ID">
        <v-btn class="device-pinned-btn mx-1 my-1" width="120"
          :title="(options.devconpin || {})[item.ID] ? '展開する' : '折りたたむ'"
          :aria-label="item.ID + ((options.devconpin || {})[item.ID] ? 'を展開する' : 'を折りたたむ')"
          :aria-expanded="!(options.devconpin || {})[item.ID]"
          @click="(options.devconpin || {})[item.ID] ? devconPin(item.ID) : pinDevice(item.ID)" outlined>
          <v-icon left>
            {{ (options.devconpin || {})[item.ID] ? 'mdi-chevron-down' : 'mdi-chevron-up' }}
          </v-icon>
          <span>{{ item.ID }}</span>
        </v-btn>
      </div>      
    </v-container>      

    <v-container fluid fill-height v-for="item in visibleDevices" :key="item.ID">

    <v-row class="mx-auto" dense>

    <v-col>

      <v-card class="pa-0 ma-2 device-card"
      :class="{'devcolor-off' : item.State==label.DSTAT_OFF,'devcolor-on': item.State>label.DSTAT_OFF}"
      hover
      >
        <v-btn
          icon
          class="device-pin-btn"
          title="折りたたむ"
          aria-label="折りたたむ"
          @click="pinDevice(item.ID)"
        >
          <v-icon dark>
            mdi-chevron-up
          </v-icon>
        </v-btn>

        <!-- OFF -->
        <v-row class="mx-auto" dense v-if="item.State==label.DSTAT_OFF">

          <v-col cols="1">
            <v-card-text class="text-h5 pa-0 ma-0 white--text">
              <p>{{ item.ID }}</p>
            </v-card-text>
          </v-col>

          <v-col cols="11" class="pr-14">

            <v-card-actions class="py-0">

              <v-btn
              class="mx-1 my-1"
              large
              width="120"
              @click="bootDevice(item.RegistryCode, label.DBOOT_TYPE_WATCH)"
              >
                <v-icon dark>
                  mdi-power-standby
                </v-icon>
                視聴
              </v-btn>

              <v-btn
              class="mx-1 my-1"
              large
              width="120"
              @click="bootDevice(item.RegistryCode, label.DBOOT_TYPE_RECORDER)"
              >
                <v-icon dark>
                  mdi-power-standby
                </v-icon>
                録画
              </v-btn>

              <v-btn
              v-if="item.TunerType == label.TUNER_TYPE_TS && !item.Hybrid"
              class="mx-1 my-1"
              large
              width="120"
              @click="bootDevice(item.RegistryCode, label.DBOOT_TYPE_ONESEG_WATCH)"
              >
                <v-icon dark>
                  mdi-power-standby
                </v-icon>
                視聴(ワ)
              </v-btn>

              <v-btn
              v-if="item.TunerType == label.TUNER_TYPE_TS && !item.Hybrid"
              class="mx-1 my-1"
              large
              width="120"
              @click="bootDevice(item.RegistryCode, label.DBOOT_TYPE_ONESEG_RECORD)"
              >
                <v-icon dark>
                  mdi-power-standby
                </v-icon>
                録画(ワ)
              </v-btn>

            </v-card-actions>
          </v-col>
        </v-row>


        <!-- Ready -->
        <v-row class="mx-auto" dense v-if="item.State==label.DSTAT_READY || item.State==label.DSTAT_RECORDING">

          <v-col cols="1">
            <v-card-text class="text-h5 pa-0 ma-0 white--text">
              <p>{{ item.ID }}</p>
            </v-card-text>
          </v-col>

          <v-col cols="11" class="pr-14">

            <v-card
            class="pa-1 ma-0"
            :class="deviceInfoClass(item)"
            flat outlined>

            <v-card-title class="text-left pa-0 ma-0 text--darken-4" outlined>
              <p>{{
              item.CurrentProgram == null ? '' : 
              uxtmtohm(item.CurrentProgram.Start) +
              '～' +
              uxtmtohm(item.CurrentProgram.End) + ' '
              }}</p>
              <p class="blue--text text--darken-4">&emsp;{{ item.CurrentProgram == null ? '' : item.CurrentProgram.Title }}</p>
            </v-card-title>           
            <v-card-subtitle class="text-left pa-0 ma-0 text--darken-4" outlined>
              <p>{{ item.CurrentProgram == null ? '' : item.CurrentProgram.Subtitle }}</p>
            </v-card-subtitle>
            <v-divider class="pa-1 ma-1"></v-divider>
            <v-card-subtitle class="text-left pa-0 ma-0 text--darken-4" outlined>
              <p>
              次の番組: {{
              item.NextProgram == null ? '' :
              uxtmtohm(item.NextProgram.Start) +
              '～' +
              uxtmtohm(item.NextProgram.End)
              }}
              &emsp;{{ item.NextProgram == null ? '' : item.NextProgram.Title }}</p>
            </v-card-subtitle>

            </v-card>

            <v-card class="pa-1 mt-2"
              v-if="captionsw[item.ID] && captions && captions[item.ID]"
              :class="deviceInfoClass(item)"
              flat outlined>
              <CaptionWindow :items="captions && captions[item.ID] ? captions[item.ID].items : []" />
            </v-card>

            <!-- BMLWindow v-if="bmlsw[item.ID] && bml && bml[item.ID]" class="mt-2"
              :key="item.ID + ':' + bml[item.ID].Sid + ':' + bml[item.ID].Session"
              :packet="bml[item.ID]" / -->

          </v-col>

        </v-row>

        <!-- Boot -->
        <v-row v-if="item.State==label.DSTAT_BOOTING">
          <v-col class="pa-4 ma=4">
            <div class="text-center">
              <v-progress-circular
              :size="100"
              :width="10"
              indeterminate
              color="#f0f0f0"
              >
              </v-progress-circular>
            </div>
          </v-col>
        </v-row>

        <!-- Ready -->
        <v-row v-if="item.State==label.DSTAT_READY" class="mx-auto" dense>

          <v-col cols="1">
          </v-col>

          <v-col>
            <v-btn
            class="mx-1 my-1"
            large
            width="120"
            @click="bootDevice(item.RegistryCode, label.DBOOT_OFF)"
            >
              <v-icon dark>
                mdi-power-standby
              </v-icon>
              終了
            </v-btn>

            <v-btn
            class="mx-1 my-1"
            large
            width="120"
            @click="comSend(item.RegistryCode, label.COM_RECORD_START, item.ID)"
            >
              <v-icon large color="red">
                mdi-record-rec
              </v-icon>
              録画開始
            </v-btn>

            <v-btn
            class="mx-1 my-1"
            large
            width="120"
            @click="reqSend(item.RegistryCode, label.COM_CAPTURE, item.ID)"
            >
              <v-icon>
                mdi-camera
              </v-icon>
              キャプチャ
            </v-btn>

            <v-switch
            v-model="captionsw[item.ID]"
            label="字幕"
            class="devcaption-switch ml-8 my-0 me-0 pa-0"
            dense
            hide-details
            >
            </v-switch>

            <!-- v-switch
            v-model="bmlsw[item.ID]"
            label="データ放送"
            class="devbml-switch ml-8 my-0 me-0 pa-0"
            dense
            hide-details
            >
            </v-switch -->

          </v-col>

          <v-col class="pr-14">
            <v-select
            class="chgstream ml-1 mr-0 my-1"
            item-text="Name"
            item-value="Sid"
            :v-model="curservice[item.ID]"
            :items="item.services"
            :value="item.current"
            outlined
            dense
            hide-details
            height="44"
            @change="changeService(item.RegistryCode, $event)"
            eager
            filled
            >
            </v-select>

          </v-col>

        </v-row>

        <!-- Recording -->
        <v-row dense v-if="item.State==label.DSTAT_RECORDING">

          <v-col cols="1">
          </v-col>

          <v-col>
            <v-btn
            class="mx-1 my-1"
            large
            width="120"
            @click="comSend(item.RegistryCode, label.COM_RECORD_STOP, item.ID)"
            >
              <v-icon large>
                mdi-stop
              </v-icon>
              録画停止
            </v-btn>

            <v-btn
            class="mx-1 my-1"
            large
            width="120"
            @click="reqSend(item.RegistryCode, label.COM_CAPTURE, item.ID)"
            >
              <v-icon>
                mdi-camera
              </v-icon>
              キャプチャ
            </v-btn>
            <v-switch
            v-model="captionsw[item.ID]"
            label="字幕"
            class="devcaption-switch ma-0 pa-0"
            dense
            hide-details
            >
            </v-switch>
            <!-- v-switch
            v-model="bmlsw[item.ID]"
            label="データ放送"
            class="devbml-switch ml-8 my-0 me-0 pa-0"
            dense
            hide-details
            >
            </v-switch -->
          </v-col>

          <v-col class="d-flex align-center">
            <div class="recording-service mx-1 my-1">
              <div class="text-caption">録画中のサービス</div>
              <div>{{ item.current ? item.current.Name : '' }}</div>
            </div>
          </v-col>

        </v-row>

      </v-card>

    </v-col>

    </v-row>

    </v-container>

    </v-card>

</template>

<script>
  import label from '../define.js';
  import utility from '../utility.js';
  import CaptionWindow from './CaptionWindow.vue';
//  const BMLWindow = () => import(/* webpackChunkName: 'bml' */ './BMLWindow.vue');

  export default {
    name: 'DeviceController',
//    components: { CaptionWindow, BMLWindow },
    components: { CaptionWindow },

    data() {
        return {
          label: null,
          curservice: {},
          captionsw: {},
//          bmlsw: {},
          autoPinTimers: {},
        }
    },

    props: [
        'device',
        'captions',
//        'bml',
        'schedule',
        'station',
        'api',
        'socket',
        'options',
    ],

    computed: {
      devicePowerStates() {
        const states = {};
        for (const item of Object.values(this.device || {})) {
          states[item.ID] = [label.DSTAT_BOOTING, label.DSTAT_READY, label.DSTAT_RECORDING].includes(item.State);
        }
        return states;
      },
      visibleDevices() {
        return Object.values(this.device || {}).filter(item => !(this.options.devconpin || {})[item.ID]);
      },
      autoPinDeviceIds() {
        return this.options.autopin
          ? this.visibleDevices.filter(item => item.State === label.DSTAT_OFF).map(item => item.ID)
          : [];
      },
    },

    methods: {
      deviceInfoClass(item) {
        if (item.State === label.DSTAT_RECORDING) {
          return 'devinfo-rec';
        }
        const now = Math.floor(utility.now() / label.MSEC);
        const watching = item.State === label.DSTAT_READY && (this.schedule || []).some(schedule =>
          schedule.deviceid === item.ID &&
          schedule.sid === item.CurrentService &&
          schedule.validate && !schedule.complete &&
          schedule.watchonly && !schedule.recordonly &&
          schedule.starttime - schedule.idletime <= now &&
          now < schedule.endtime + schedule.tailtime
        );
        return watching ? 'devinfo-watch' : 'devinfo';
      },
      
      bootDevice(id, bt) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
          this.socket.send('BDV:' + JSON.stringify({ registryid: String(id), boottype: bt }));
        }
      },

      pinDevice(id) {
        let opt = this.options;
        if (!opt.devconpin) {
          this.$set(opt, 'devconpin', {});
        }
        this.$set(opt.devconpin, id, true);
        this.$emit('optionchange', opt);
      },

      devconPin(id) {
        let opt = this.options;
        if (!opt.devconpin) {
          this.$set(opt, 'devconpin', {});
        }
        this.$set(opt.devconpin, id, false);
        this.$emit('optionchange', opt);
      },

      comSend(id, com, msg) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
          utility.log("id="+id+", com="+com+", msg="+msg)
          this.socket.send('COM:' + JSON.stringify({ id: String(id), command: String(com), message: String(msg) }));
        }
      },

      reqSend(id, com, msg) {
        if (this.socket && this.socket.readyState === WebSocket.OPEN) {
          utility.log("id="+id+", com="+com+", msg="+msg)
          this.socket.send('REQ:' + JSON.stringify({ id: String(id), command: String(com), message: String(msg) }));
        }
      },

      uxtmtohm(uxtm) {
        if (uxtm === -1) {
          return '未定';
        }
        let d = new Date(uxtm * label.MSEC);
        let hour = ('0' + d.getHours()).slice(-2);
        let min = ('0' + d.getMinutes()).slice(-2);
        return hour + ':' + min;
      },

      changeService(id, sid) {
        // 放送種別にかかわらず、選択サービス・親サービスの順で送る。
        const service = this.station.all[sid];
        if (service && service.Parent !== 0 && service.Parent !== service.Sid) {
          this.reqSend(id, "StreamChange", label.SUBCHANNEL_CODE<<16|(sid&0xffff));
          sid = service.Parent;
        }

        this.reqSend(id, "StreamChange", sid)
      },

    },

    watch: {
      'options.captionon'(enabled) {
        for (const id of Object.keys(this.captionsw)) {
          this.$set(this.captionsw, id, enabled);
        }
      },
      devicePowerStates: {
        immediate: true,
        handler(states, previousStates = {}) {
          if (!this.options.autopin) return;
          for (const id of Object.keys(states)) {
            if (states[id] && !previousStates[id] && (this.options.devconpin || {})[id]) {
              this.devconPin(id);
            }
          }
        },
      },
      autoPinDeviceIds: {
        immediate: true,
        handler(ids) {
          const eligibleIds = new Set(ids.map(String));
          for (const id of Object.keys(this.autoPinTimers)) {
            if (!eligibleIds.has(id)) {
              clearTimeout(this.autoPinTimers[id]);
              delete this.autoPinTimers[id];
            }
          }
          for (const id of ids) {
            if (Object.prototype.hasOwnProperty.call(this.autoPinTimers, id)) continue;
            this.autoPinTimers[id] = setTimeout(() => {
              delete this.autoPinTimers[id];
              if (this.autoPinDeviceIds.includes(id)) {
                this.pinDevice(id);
              }
            }, 10000);
          }
        },
      },
      device: {
        immediate: true,
        handler(devices) {
          for (const device of Object.values(devices || {})) {
            if (!Object.prototype.hasOwnProperty.call(this.captionsw, device.ID)) {
              this.$set(this.captionsw, device.ID, this.options.captionon);
            }
            /*
            if (!Object.prototype.hasOwnProperty.call(this.bmlsw, device.ID)) {
              this.$set(this.bmlsw, device.ID, this.options.bmlon);
            }
            */
          }
        },
      },
    },

    created() {
      this.label = label;
    },

    beforeDestroy() {
      for (const timer of Object.values(this.autoPinTimers)) {
        clearTimeout(timer);
      }
    },

  }
</script>

<style scoped>
.recording-service {
  height: 44px;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.devinfo {
  background-color: var(--app-surface-soft, rgb(226, 226, 226));
}

.devinfo-rec {
  background-color: var(--app-recording, rgb(226, 188, 188));
}

.devinfo-watch {
  background-color: var(--app-watch, rgb(255, 235, 156));
}

.devcolor-on {
  background-color: var(--app-device-on, rgb(179, 179, 187))
}

.device-card {
  position: relative;
}

.device-card,
.devinfo {
  /* A quiet brushed-metal finish over the existing light/dark surface colours. */
  background-image:
    linear-gradient(115deg, rgba(0, 0, 0, 0.07) 0%, rgba(255, 255, 255, 0.06) 28%, rgba(0, 0, 0, 0.03) 48%, rgba(255, 255, 255, 0.04) 65%, rgba(0, 0, 0, 0.08) 100%),
    repeating-linear-gradient(0deg, rgba(255, 255, 255, 0.018) 0, rgba(255, 255, 255, 0.018) 1px, rgba(0, 0, 0, 0.018) 1px, rgba(0, 0, 0, 0.018) 3px);
}

.device-pin-btn {
  position: absolute;
  top: 6px;
  right: 6px;
  z-index: 2;
  color: #ffffff !important;
  background-color: rgba(0, 0, 0, 0.2);
}

.device-pin-btn:hover {
  background-color: rgba(0, 0, 0, 0.4);
}

.device-pinned-btn.v-btn--outlined {
  border-color: #9e9e9e !important;
}

.device-pinned-btn span {
  display: flex;
  align-items: center;
  line-height: 1;
}

.devcolor-off {
  background-color: var(--app-device-off, rgb(118, 118, 129))
}

</style>

<style>

.chgstream .v-select__selections {
  color: var(--app-link, rgb(67, 58, 148));
}

.chgstream .v-select__selection,
.chgstream .v-select__selection--comma,
.chgstream .v-select.v-text-field input {
  color: var(--app-text, rgb(40, 40, 40)) !important;
}

.devcaption-switch {
  display: inline-flex;
  vertical-align: middle;
  min-height: 28px;
}

.devbml-switch {
  display: inline-flex;
  vertical-align: middle;
  min-height: 28px;
}

.device-pin {
  display: flex;
  gap: 12px;
  align-items: left;
}

</style>
