<template>
  <v-card-text>
    <v-subheader>番組欄のジャンル色</v-subheader>
    <v-btn-toggle v-model="editTheme" mandatory aria-label="編集する配色">
      <v-btn value="light">ライト</v-btn>
      <v-btn value="dark">ダーク</v-btn>
    </v-btn-toggle>

    <div class="genre-colors mt-3" :class="{ 'genre-colors-dark': editTheme === 'dark' }">
      <button
        v-for="genre in genres"
        :key="genre.code"
        type="button"
        class="genre-color"
        :style="{ backgroundColor: colorFor(genre) }"
        @click="editColor(genre)"
      >{{ genre.name }}</button>
    </div>

    <v-dialog v-model="pickerOpen" max-width="360" aria-labelledby="genre-color-title">
      <v-card v-if="selectedGenre">
        <v-card-title id="genre-color-title">
          {{ selectedGenre.name }}（{{ editTheme === 'dark' ? 'ダーク' : 'ライト' }}）
        </v-card-title>
        <v-card-text class="pb-2">
          <p class="mb-2">H（色相）・S（彩度）・L（明度）で調整できます。</p>
          <v-color-picker v-model="draftColor" class="mx-auto" :width="pickerWidth" mode="hsla" flat />
          <div class="genre-colors mt-2" :class="{ 'genre-colors-dark': editTheme === 'dark' }">
            <div class="genre-color" :style="{ backgroundColor: draftColor }">番組名のプレビュー</div>
          </div>
        </v-card-text>
        <v-card-actions class="flex-wrap">
          <v-btn text @click="draftColor = selectedGenre[editTheme]">既定色に戻す</v-btn>
          <v-spacer />
          <v-btn text @click="pickerOpen = false">キャンセル</v-btn>
          <v-btn text color="primary" @click="saveColor">保存</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </v-card-text>
</template>

<script>
import { timeTableGenres, isGenreColor } from '../../timeTableGenreColors';

export default {
  name: 'TimeTableGenreColorSetting',
  props: {
    value: { type: Object, required: true },
  },
  data() {
    return {
      genres: timeTableGenres,
      editTheme: this.$vuetify.theme.dark ? 'dark' : 'light',
      pickerOpen: false,
      selectedGenre: null,
      draftColor: '',
    };
  },
  computed: {
    pickerWidth() {
      return Math.min(312, Math.max(1, this.$vuetify.breakpoint.width - 80));
    },
  },
  methods: {
    colorFor(genre) {
      return this.value[this.editTheme][genre.code] || genre[this.editTheme];
    },
    editColor(genre) {
      this.selectedGenre = genre;
      this.draftColor = this.colorFor(genre);
      this.pickerOpen = true;
    },
    saveColor() {
      if (!isGenreColor(this.draftColor)) return;
      const overrides = { ...this.value[this.editTheme] };
      const { code } = this.selectedGenre;
      if (this.draftColor.toLowerCase() === this.selectedGenre[this.editTheme]) {
        delete overrides[code];
      } else {
        overrides[code] = this.draftColor;
      }
      this.$emit('input', { ...this.value, [this.editTheme]: overrides });
      this.pickerOpen = false;
    },
  },
};
</script>

<style scoped>
.genre-colors {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 12rem), 1fr));
  gap: 1px;
  max-width: 64rem;
  background-color: #fffff0;
  color: rgba(18, 56, 136, 0.8);
}

.genre-colors-dark {
  background-color: #182129;
  color: #d0daf3;
}

.genre-color {
  min-height: 36px;
  border: 1px solid #80808066;
  padding: 4px 8px;
  color: inherit;
  text-align: left;
}

button.genre-color {
  cursor: pointer;
}
</style>
