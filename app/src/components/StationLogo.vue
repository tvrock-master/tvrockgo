<template>
  <img
    v-show="loaded"
    :key="src"
    :src="src"
    class="tt-station-logo"
    alt=""
    @load="loaded = true"
    @error="loaded = false"
  >
</template>

<script>
export default {
  name: 'StationLogo',
  props: {
    sid: {
      type: [Number, String],
      required: true,
    },
  },
  data() {
    return { loaded: false };
  },
  computed: {
    src() {
      return '/static/cdt/' + encodeURIComponent(this.sid) + '_1';
    },
  },
  watch: {
    src() {
      this.loaded = false;
    },
  },
};
</script>
