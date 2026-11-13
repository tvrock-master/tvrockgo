<template>

    <div class="tab-full-height">
      <v-card-text>
        <v-row dense>
          <v-col cols="12" sm="6" md="4">
            <v-select
            v-model="setting.gaijiexclusion"
            :items="gaijiExclusionItems"
            :disabled="loading"
            item-text="text"
            item-value="value"
            label="外字処理"
            @change="changeGaijiExclusion"
            >
            </v-select>
          </v-col>
        </v-row>

        <v-row dense>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            v-model="setting.filenameex"
            :disabled="loading"
            label="録画ファイル名置換コマンド"
            placeholder="@TT"
            clearable
            @change="changeFilenameEx"
            @blur="changeFilenameEx"
            >
              <template v-slot:append-outer>
                <v-tooltip top>
                  <template v-slot:activator="{ on, attrs }">
                    <v-icon
                      v-bind="attrs"
                      v-on="on"
                    >
                      mdi-information
                    </v-icon>
                  </template>
                  <span>
                    <table>
                      <tr><td>@TT</td><td>番組タイトル</td></tr>
                      <tr><td>@SB</td><td>サブタイトル</td></tr>
                      <tr><td>@sb</td><td>サブタイトル （空白抜)</td></tr>
                      <tr><td>@CH</td><td>局名</td></tr>
                      <tr><td>@ch</td><td>局名（空白抜)</td></tr>
                      <tr><td>@DR</td><td>番組の放送時間(分)</td></tr>
                      <tr><td>@HTK</td><td>ハッシュタグ(キーワード・","で複数可)</td></tr>
                      <tr><td>@HTS</td><td>ハッシュタグ(局・","で複数可)</td></tr>
                      <tr><td>@YY</td><td>２４時間表記・年</td></tr>
                      <tr><td>@2YY</td><td>２４時間表記・年（下２桁）</td></tr>
                      <tr><td>@MM</td><td>２４時間表記・月</td></tr>
                      <tr><td>@DD</td><td>２４時間表記・日</td></tr>
                      <tr><td>@SH</td><td>２４時間表記・開始時間・時</td></tr>
                      <tr><td>@SM</td><td>２４時間表記・開始時間・分</td></tr>
                      <tr><td>@SS</td><td>２４時間表記・開始時間・秒</td></tr>
                      <tr><td>@WK</td><td>２４時間表記・開始時間・曜日</td></tr>
                      <tr><td>@WE</td><td>２４時間表記・開始時間・曜日・英語表記</td></tr>
                      <tr><td>@EH</td><td>２４時間表記・終了時間・時</td></tr>
                      <tr><td>@EM</td><td>２４時間表記・終了時間・分</td></tr>
                      <tr><td>@ES</td><td>２４時間表記・終了時間・秒</td></tr>
                      <tr><td>@yy</td><td>２８時間表記・年</td></tr>
                      <tr><td>@mm</td><td>２８時間表記・月</td></tr>
                      <tr><td>@dd</td><td>２８時間表記・日</td></tr>
                      <tr><td>@sh</td><td>２８時間表記・開始時間・時</td></tr>
                      <tr><td>@sm</td><td>２８時間表記・開始時間・分</td></tr>
                      <tr><td>@ss</td><td>２８時間表記・開始時間・秒</td></tr>
                      <tr><td>@wk</td><td>２８時間表記・開始時間・曜日</td></tr>
                      <tr><td>@we</td><td>２８時間表記・開始時間・曜日・英語表記</td></tr>
                      <tr><td>@eh</td><td>２８時間表記・終了時間・時</td></tr>
                      <tr><td>@em</td><td>２８時間表記・終了時間・分</td></tr>
                      <tr><td>@es</td><td>２８時間表記・終了時間・秒</td></tr>
                      <tr><td>@EC</td><td>録画エラー数</td></tr>
                      <tr><td>@RD</td><td>録画ファイルフォルダ</td></tr>
                      <tr><td>@RF</td><td>録画ファイルパス（カスタムコマンドのみ）</td></tr>
                      <tr><td>@RN</td><td>録画ファイル名（カスタムコマンドのみ）</td></tr>
                    </table>
                  </span>
                </v-tooltip>
              </template>
            </v-text-field>
          </v-col>
        </v-row>
 
        <v-row dense>
          <v-col cols="12" sm="6" md="4">
            <v-text-field
            v-model="setting.recordfileextension"
            :disabled="loading"
            label="録画ファイル拡張子"
            placeholder=".ts"
            clearable
            @change="changeRecordFileExtension"
            @blur="changeRecordFileExtension"
            >
            </v-text-field>
          </v-col>
        </v-row>
        <v-row dense>
          <v-col cols="12">
            <v-switch v-model="setting.analyzetsafterrecording" :disabled="loading"
              label="録画後にTS解析処理を行う"
              hint="録画後に再生時間とシーク位置を保存し、再生開始時の解析待ちを短縮します。OFFの場合は再生時に解析します。"
              persistent-hint @change="save" />
          </v-col>
        </v-row>
      </v-card-text>

    </div>

</template>


<script>
  import axios from 'axios'
  import utility from '../../utility.js';

  export default {
    name: 'RecorderSetting',

    props: [
        'api',
        'refreshToken',
    ],

    data() {
        return {
          loading: false,
          setting: {
            gaijiexclusion: 0,
            filenameex: '@TT',
            recordfileextension: '.ts',
            analyzetsafterrecording: false,
          },
          gaijiExclusionItems: [
            { text: '0: 何もしない', value: 0 },
            { text: '1: 置換する', value: 1 },
            { text: '2: 削除する', value: 2 },
          ],
        }
    },

    created() {
      this.load();
    },

    methods: {
      load() {
        if (!this.api.recorderSetting) {
          return;
        }

        this.loading = true;
        axios.get(this.api.recorderSetting).then(res => {
          this.setting = this.normalize(res.data || {});
        }).catch(err => {
          utility.err(`request recorder setting error: ${err}`);
        }).finally(() => {
          this.loading = false;
        });
      },

      save() {
        if (!this.api.recorderSetting) {
          return;
        }

        this.loading = true;
        axios.put(this.api.recorderSetting, this.normalize(this.setting)).then(res => {
          this.setting = this.normalize(res.data || {});
        }).catch(err => {
          utility.err(`update recorder setting error: ${err}`);
          this.load();
        }).finally(() => {
          this.loading = false;
        });
      },

      normalize(v) {
        return {
          gaijiexclusion: this.normalizeGaijiExclusion(v.gaijiexclusion),
          filenameex: v.filenameex || '@TT',
          recordfileextension: v.recordfileextension || '.ts',
          analyzetsafterrecording: v.analyzetsafterrecording === true,
        };
      },

      normalizeGaijiExclusion(v) {
        const n = Number(v);
        if (n >= 0 && n <= 2) {
          return n;
        }
        return 0;
      },

      changeGaijiExclusion(v) {
        this.setting.gaijiexclusion = this.normalizeGaijiExclusion(v);
        this.save();
      },

      changeFilenameEx() {
        this.setting.filenameex = this.setting.filenameex || '@TT';
        this.save();
      },

      changeRecordFileExtension() {
        this.setting.recordfileextension = this.setting.recordfileextension || '.ts';
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

</style>
