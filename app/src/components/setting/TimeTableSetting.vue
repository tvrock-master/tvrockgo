<template>

    <div class="tab-full-height">

      <v-card-text>
        <v-subheader>カラーテーマ</v-subheader>
        <v-btn-toggle :value="options.forcetheme" @change="changeForceTheme" mandatory aria-label="カラーテーマ">
          <v-btn :value="0">Windowsに合わせる</v-btn>
          <v-btn :value="1">ライト</v-btn>
          <v-btn :value="2">ダーク</v-btn>
        </v-btn-toggle>
      </v-card-text>

      <v-card-text>
        <v-subheader>番組表の高さ</v-subheader>
        <v-btn-toggle :value="pixelsPerMinute" @change="changeHeight" mandatory aria-label="番組表の高さ">
          <v-btn :value="2">ショート</v-btn>
          <v-btn :value="4">ノーマル</v-btn>
          <v-btn :value="8">ロング</v-btn>
        </v-btn-toggle>
      </v-card-text>

      <v-card-text>
        <v-subheader>サービス表示範囲</v-subheader>
        <v-btn-toggle v-model="dispservice" @change="changeDispService" mandatory>
          <v-btn>自動</v-btn>
          <v-btn>2列</v-btn>
          <v-btn>4列</v-btn>
          <v-btn>6列</v-btn>
          <v-btn>8列</v-btn>
          <v-btn>10列</v-btn>
          <v-btn>12列</v-btn>
        </v-btn-toggle>
      </v-card-text>

      <v-card-text>
        <v-subheader>サブタイトル表示閾値</v-subheader>
        <v-btn-toggle v-model="subtitlemin" @change="changeSubtitleMin" mandatory>
          <v-btn>常に表示</v-btn>
          <v-btn>15分</v-btn>
          <v-btn>30分</v-btn>
          <v-btn>60分</v-btn>
          <v-btn>90分</v-btn>
          <v-btn>120分</v-btn>
          <v-btn>180分</v-btn>
          <v-btn>360分</v-btn>
        </v-btn-toggle>
      </v-card-text>

      <v-card-text>
        <v-subheader>番組詳細表示閾値</v-subheader>
        <v-btn-toggle v-model="contentmin" @change="changeContentMin" mandatory>
          <v-btn>常に表示</v-btn>
          <v-btn>15分</v-btn>
          <v-btn>30分</v-btn>
          <v-btn>60分</v-btn>
          <v-btn>90分</v-btn>
          <v-btn>120分</v-btn>
          <v-btn>180分</v-btn>
          <v-btn>360分</v-btn>
        </v-btn-toggle>
      </v-card-text>
          
      <v-card-text>
        <v-subheader>番組欄表示アクション</v-subheader>
        <v-btn-toggle v-model="guidetype" @change="changeGuideType" mandatory>
          <v-btn>クリック</v-btn>
          <v-btn>マウスオーバー</v-btn>
          <v-btn>ポップアップ</v-btn>
          <v-btn>ポップアップ（マウスオーバー）</v-btn>
        </v-btn-toggle>
      </v-card-text>

      <v-card-text>
        <v-subheader>時間軸</v-subheader>
        <v-col cols="3">
          <v-slider
          v-model="divider"
          :max="12"
          :min="1"
          :label="divider+'局毎'"
          @change="changeDivider"
          ></v-slider>
        </v-col>
      </v-card-text>

      <v-card-text>
        <v-switch
        :input-value="options.displogo"
        label="局ロゴ表示"
        @change="changeDispLogo"
        >
        </v-switch>
      </v-card-text>

      <v-card-text>
        <v-switch
        :input-value="this.options.twentyEight"
        :label="'28時間表記'"
        @change="changeTwentyEight()"
        >
        </v-switch>
      </v-card-text>

      <v-card-text>
        <v-switch
        :input-value="this.options.sideselection"
        :label="'放送波・時間帯を横に表示'"
        @change="changeSideSelection()"
        >
        </v-switch>
      </v-card-text>


      <v-card-text>
        <v-switch
        :input-value="this.options.captiontimetable"
        :label="'番組クリック時に字幕履歴を表示'"
        @change="changeCaptionTimeTable()"
        >
        </v-switch>
      </v-card-text>

      <v-card-text>
        <v-switch
        :input-value="this.options.watchbutton"
        :label="'視聴予約ボタンを表示'"
        @change="changeWatchButton()"
        >
        </v-switch>
      </v-card-text>

      <TimeTableGenreColorSetting :value="options.genreColors" @input="changeGenreColors" />

    </div>

</template>

<script>

  import { getPixelsPerMinute } from '../../timeTableRenderer';
  import TimeTableGenreColorSetting from './TimeTableGenreColorSetting.vue';

  const dispservicebox = [0,2,4,6,8,10,12];
  const subtitleminbox = [0,15,30,60,90,120,180,360];
  const contentminbox = [0,15,30,60,90,120,180,360];

  export default {
    name: 'TimeTableSetting',
    components: { TimeTableGenreColorSetting },

    props: [
        'options',
        'api',
    ],

    data() {
        return {
          dispservice: dispservicebox.indexOf(this.options.dispservice),
          guidetype: this.options.guidetype,
          subtitlemin: subtitleminbox.indexOf(this.options.subtitlemin),
          contentmin: contentminbox.indexOf(this.options.contentmin),
          divider: this.options.divider
        }
    },
    
    computed: {
      pixelsPerMinute() {
        return getPixelsPerMinute(this.options);
      },
    },

    methods: {
      changeHeight(value) {
        const opt = this.options;
        opt.hours = getPixelsPerMinute({ hours: value });
        this.$emit('optionchange', opt);
      },

      changeForceTheme(value) {
        let opt = this.options;
        opt.forcetheme = value;
        this.$emit('optionchange', opt);
      },

      changeDispService(v) {
        let opt = this.options;
        opt.dispservice = dispservicebox[v];
        this.$emit('optionchange', opt, true);
      },

      changeGuideType(v) {
        let opt = this.options;
        opt.guidetype = v;
        this.$emit('optionchange', opt);
      },

      changeSubtitleMin(v) {
        let opt = this.options;
        opt.subtitlemin = subtitleminbox[v];
        this.$emit('optionchange', opt);
      },

      changeContentMin(v) {
        let opt = this.options;
        opt.contentmin = contentminbox[v];
        this.$emit('optionchange', opt);
      },

      changeDivider(v) {
        let opt = this.options;
        opt.divider = v
        this.$emit('optionchange', opt);
      },

      changeDispLogo(value) {
        let opt = this.options;
        opt.displogo = value;
        this.$emit('optionchange', opt);
      },

      changeTwentyEight() {
        let opt = this.options;
        opt.twentyEight = !opt.twentyEight;
        this.$emit('optionchange', opt);
      },

      changeSideSelection() {
        let opt = this.options;
        opt.sideselection = !opt.sideselection;
        this.$emit('optionchange', opt);
      },

      changeCaptionTimeTable() {
        let opt = this.options;
        opt.captiontimetable = !opt.captiontimetable;
        this.$emit('optionchange', opt);
      },

      changeWatchButton() {
        let opt = this.options;
        opt.watchbutton = !opt.watchbutton;
        this.$emit('optionchange', opt);
      },
      
      changeGenreColors(genreColors) {
        this.$emit('optionchange', { ...this.options, genreColors });
      }

    }

  }
</script>

<style scoped>

.tab-full-height {
  /* 内容に合わせて伸ばし、親タブで下の設定が切り取られるのを防ぐ。 */
  min-height: 100%;
}

</style>
