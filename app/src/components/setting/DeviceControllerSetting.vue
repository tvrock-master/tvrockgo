<template>

    <div class="tab-full-height">

      <v-card-text>
        <v-switch
        :input-value="this.options.captionon"
        :label="'字幕表示初期値'"
        @change="changeCaptionON()"
        >
        </v-switch>
      </v-card-text>

      <!-- v-card-text>
        <v-switch
        :input-value="this.options.bmlon"
        :label="'データ放送表示初期値'"
        @change="changeBmlON()"
        >
        </v-switch>
      </v-card-text -->

      <v-card-text>
        <v-switch
        :input-value="this.options.autopin"
        :label="'デバイス電源OFF時は自動で折りたたむ'"
        @change="changeAutoPinON()"
        >
        </v-switch>
      </v-card-text>

    </div>

</template>

<script>

  export default {
    name: 'DeviceControllerSetting',

    props: [
        'options',
        'api',
    ],

    data() {
        return {
        }
    },
    
    methods: {

      changeCaptionON() {
        let opt = this.options;
        opt.captionon = !opt.captionon;
        this.$emit('optionchange', opt);
      },

      /*
      changeBmlON() {
        let opt = this.options;
        opt.bmlon = !opt.bmlon;
        this.$emit('optionchange', opt);
      },
      */
      
      changeAutoPinON() {
        let opt = this.options;
        opt.autopin = !opt.autopin;
        this.$emit('optionchange', opt);
      },

    }

  }
</script>

<style scoped>

.tab-full-height {
  /* 内容に合わせて伸ばし、親タブで下の設定が切り取られるのを防ぐ。 */
  min-height: 100%;
}

</style>
