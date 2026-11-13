<template>

  <v-tabs class="system-settings" v-model="tab" slider-color="settingsslider" slider-size="4" background-color="settings" vertical flat>

    <v-tab>
      <v-icon left>
        mdi-harddisk
      </v-icon>
      ストレージ情報
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-table
      </v-icon>
      番組表
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-console-network
      </v-icon>
      デバイス
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-satellite-uplink
      </v-icon>
      サービス
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-download-network
      </v-icon>
      番組情報取得局
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-clock-time-three-outline
      </v-icon>
      番組情報取得時刻
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-disc-player
      </v-icon>
      録画設定
    </v-tab>

    <v-divider></v-divider>

    <v-tab v-if="powerManagerEnabled">
      <v-icon left>
        mdi-power
      </v-icon>
      電源管理
    </v-tab>

    <v-divider v-if="powerManagerEnabled"></v-divider>

    <v-tab>
      <v-icon left>
        mdi-text-box-outline
      </v-icon>
      録画ログ
    </v-tab>

    <v-divider></v-divider>

    <v-tab href="#system-log">
      <v-icon left>
        mdi-text-box-outline
      </v-icon>
      システムログ
    </v-tab>

    <v-divider></v-divider>

    <v-tab>
      <v-icon left>
        mdi-text-box-outline
      </v-icon>
      ログアウト
    </v-tab>

    <v-divider></v-divider>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <StorageInfo :storageInfo="storageInfo"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <TimeTableSetting :options="options" :api="api"  @optionchange="optionUpdate"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <DeviceControllerSetting :options="options" :api="api"  @optionchange="optionUpdate"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <ServiceSetting :station="station" :api="api" :options="options"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <FetchProgramSetting :station="station" :api="api" :options="options"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <FetchProgramScheduleTimeSetting :api="api"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <RecorderSetting :api="api" :refreshToken="recorderSettingRevision"/>
      </v-card>
    </v-tab-item>

    <v-tab-item v-if="powerManagerEnabled" :transition="false">
      <v-card flat class="settings-page-card">
        <PowerManager
        :api="api"
        :refreshToken="powerManagerSettingRevision"
        @powermanagerchange="$emit('powermanagerchange')"
        />
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <RecordLog :schedule="schedule" :api="api" @select="$emit('select', $event)"/>
      </v-card>
    </v-tab-item>

    <v-tab-item class="settings-log-tab" value="system-log" :transition="false">
      <v-card flat class="settings-page-card settings-log-page">
        <LogSetting :api="api" :active="tab === 'system-log'"/>
      </v-card>
    </v-tab-item>

    <v-tab-item :transition="false">
      <v-card flat class="settings-page-card">
        <LogoutPage/>
      </v-card>
    </v-tab-item>


  </v-tabs>

</template>

<script>
  import TimeTableSetting from './setting/TimeTableSetting';
  import DeviceControllerSetting from './setting/DeviceControllerSetting';
  import ServiceSetting from './setting/ServiceSetting';
  import FetchProgramSetting from './setting/FetchProgramSetting';
  import FetchProgramScheduleTimeSetting from './setting/FetchProgramScheduleTimeSetting';
  import RecorderSetting from './setting/RecorderSetting';
  import PowerManager from './setting/PowerManager';
  import StorageInfo from './setting/StorageInfo';
  import RecordLog from './setting/RecordLog';
  import LogSetting from './setting/LogSetting';
  import LogoutPage from './setting/LogoutPage';

  export default {
    name: 'SystemSetting',

    components: {
      TimeTableSetting,
      DeviceControllerSetting,
      ServiceSetting,
      FetchProgramSetting,
      FetchProgramScheduleTimeSetting,
      RecorderSetting,
      PowerManager,
      StorageInfo,
      RecordLog,
      LogSetting,
      LogoutPage,
    },

    props: [
        'options',
        'api',
        'configure',
        'schedule',
        'station',
        'storageInfo',
        'recorderSettingRevision',
        'powerManagerSettingRevision',
    ],

    data() {
        return {
          tab: 0,
        }
    },

    computed: {
      powerManagerEnabled() {
        return !!(this.configure &&
          this.configure.PowerManagerSetting &&
          this.configure.PowerManagerSetting.powermanager);
      },
    },
    
    methods: {
      optionUpdate: function(v, reloadPrograms = false) {
        this.$emit(reloadPrograms ? 'redrawtt' : 'optionchange', v);
      },
    }

  }
</script>

<style scoped>
.system-settings {
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

.system-settings ::v-deep > .v-tabs-bar {
  height: 100%;
  flex: 0 0 auto;
  overflow: hidden;
}

.system-settings ::v-deep > .v-tabs-bar > .v-slide-group__wrapper {
  overflow-y: auto;
  overscroll-behavior: contain;
}

.system-settings ::v-deep > .v-tabs-bar .v-tabs-bar__content {
  height: max-content;
  min-height: 100%;
}

.system-settings ::v-deep > .v-tabs-items {
  height: 100%;
  min-width: 0;
  min-height: 0;
  flex: 1 1 auto;
}

.system-settings ::v-deep > .v-tabs-items > .v-window__container {
  height: 100%;
}

.system-settings ::v-deep > .v-tabs-items > .v-window__container > .v-window-item {
  height: 100%;
  overflow: auto;
  overscroll-behavior: contain;
}

.settings-page-card {
  min-height: 100%;
  padding-bottom: 16px;
}

/* ログ本文だけをスクロールさせ、下端の余白を常に残す。 */
.system-settings ::v-deep .settings-log-tab {
  overflow: hidden;
}

.settings-log-page {
  height: 100%;
  min-height: 0;
  overflow: hidden;
}
</style>
