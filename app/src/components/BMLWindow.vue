<template>
  <section class="bml-window" aria-label="データ放送">
    <div class="bml-toolbar">
      <strong>データ放送</strong>
      <span role="status">{{ status }}</span>
      <button type="button" @click="restart">再読込</button>
      <button type="button" @click="staticView = !staticView">{{ staticView ? 'BML再生' : 'HTML表示' }}</button>
    </div>
    <div ref="viewport" class="bml-viewport" :style="{ height: height * scale + 'px' }">
      <div v-show="!staticView" class="bml-stage" :style="stageStyle">
        <div ref="media" />
        <div ref="host" />
      </div>
      <iframe v-if="staticView" title="変換されたBML" sandbox="" :srcdoc="htmlView" :style="stageStyle" />
    </div>
    <div class="bml-remote" aria-label="データ放送リモコン">
      <button v-for="key in keys" :key="key.code" type="button" :style="{ borderBottomColor: key.color || '#888' }"
        @click="press(key.code)">{{ key.label }}</button>
    </div>
    <p v-if="error" role="alert">{{ error }}</p>
  </section>
</template>

<script>
// import { BMLBrowser, keyCodeToAribKey } from 'web-bml';

export default {
  name: 'BMLWindow',
  props: { packet: { type: Object, required: true } },
  data() {
    return {
      status: 'データ受信中…', error: '', width: 960, height: 540, scale: 1, staticView: false,
      keys: [
        { label: 'd', code: 'DataButton' }, { label: '↑', code: 'Up' },
        { label: '↓', code: 'Down' }, { label: '←', code: 'Left' }, { label: '→', code: 'Right' },
        { label: '決定', code: 'Enter' }, { label: '戻る', code: 'Back' },
        { label: '青', code: 'BlueButton', color: '#2480ff' }, { label: '赤', code: 'RedButton', color: '#f44' },
        { label: '緑', code: 'GreenButton', color: '#0b5' }, { label: '黄', code: 'YellowButton', color: '#fc0' },
        ...Array.from({ length: 10 }, (_, i) => ({ label: String(i), code: 'Digit' + i })),
      ],
    };
  },
  computed: {
    stageStyle() {
      return { width: this.width + 'px', height: this.height + 'px', transform: `scale(${this.scale})`, transformOrigin: 'top left' };
    },
    htmlView() {
      const resources = new Map();
      let entry = null;
      for (const msg of this.packet.Messages || []) {
        if (msg.type !== 'moduleDownloaded') continue;
        const base = `/${msg.componentId.toString(16).padStart(2, '0')}/${msg.moduleId.toString(16).padStart(4, '0')}/`;
        for (const file of msg.files || []) {
          const location = file.contentLocation || '';
          const path = new URL(location, 'https://bml.invalid' + base).pathname;
          const type = file.contentType.type + '/' + file.contentType.subtype;
          resources.set(path, { file, type });
          if (file.html && (!entry || /startup\.(bml|xml)$/i.test(location))) entry = { file, path };
        }
      }
      if (!entry) return '<p>HTML文書を受信中…</p>';
      const doc = new DOMParser().parseFromString(entry.file.html, 'text/html');
      doc.querySelectorAll('[data-bml-src]').forEach(node => {
        try {
          const url = new URL(node.getAttribute('data-bml-src'), 'https://bml.invalid' + entry.path);
          const resource = url.origin === 'https://bml.invalid' && resources.get(url.pathname);
          if (resource && /^image\/(png|jpeg|gif)$/i.test(resource.type)) {
            node.setAttribute('src', `data:${resource.type};base64,${resource.file.dataBase64}`);
          }
        } catch { /* Unresolved broadcast resource: wait for the next carousel. */ }
      });
      return '<!doctype html>' + doc.documentElement.outerHTML;
    },
  },
  // watch: { packet() { this.feed(); } },
  mounted() {
    // this.observer = new ResizeObserver(() => this.resize());
    // this.observer.observe(this.$refs.viewport);
    // this.start();
  },
  beforeDestroy() {
    // if (this.observer) this.observer.disconnect();
    // if (this.runtime) this.runtime.destroy();
    // if (this.audio) this.audio.close();
  },
  methods: {
    resize() { this.scale = Math.min(1, this.$refs.viewport.clientWidth / this.width); },
    start() {
      // this.seen = new Map();
      // this.error = '';
      // this.status = 'データ受信中…';
      // try {
        // this.audio = new (window.AudioContext || window.webkitAudioContext)();
        // // A new host is needed because a ShadowRoot cannot be attached twice.
        // const host = document.createElement('div');
        // this.$refs.host.replaceChildren(host);
        // this.runtime = new BMLBrowser({
          // containerElement: host, mediaElement: this.$refs.media, tabIndex: 0,
          // storagePrefix: 'tvrock-bml-',
          // audioNodeProvider: { getAudioDestinationNode: () => this.audio.destination },
          // fonts: {
            // roundGothic: { source: "local('MS Gothic')" },
            // squareGothic: { source: "local('MS Gothic')" },
          // },
          // // No receiver identity, tuning or bidirectional network privileges.
          // ip: {}, epg: {},
          // showErrorMessage: (title, message) => { this.error = title + ': ' + message; },
          // indicator: {
            // setUrl: (url, loading) => { this.status = loading ? '読み込み中…' : url; },
            // setReceivingStatus: receiving => { if (receiving) this.status = 'データ受信中…'; },
            // setNetworkingGetStatus() {}, setNetworkingPostStatus() {}, setEventName() {},
          // },
        // });
        // this.runtime.addEventListener('load', ev => {
          // this.width = ev.detail.resolution.width;
          // this.height = ev.detail.resolution.height;
          // this.resize();
        // });
        // this.feed();
      // } catch (err) { this.error = 'BML再生を開始できません: ' + err.message; this.staticView = true; }
      // this.resize();
    },
    restart() {
      // if (this.runtime) this.runtime.destroy();
      // if (this.audio) this.audio.close();
      // this.runtime = null;
      // this.start();
    },
    feed() {
      // if (!this.runtime) return;
      // try {
        // for (const msg of this.packet.Messages || []) {
          // // Avoid replaying ModuleUpdated and restarting documents on snapshots.
          // const key = msg.type + ':' + (msg.componentId || 0) + ':' + (msg.moduleId || 0);
          // const signature = msg.type === 'moduleDownloaded' ? `${msg.dataEventId}:${msg.version}` : JSON.stringify(msg);
          // if (this.seen.get(key) === signature) continue;
          // if (msg.type === 'pmt') {
            // for (const k of this.seen.keys()) if (k.startsWith('module')) this.seen.delete(k);
          // }
          // if (msg.type === 'moduleListUpdated') {
            // const ids = new Set(msg.modules.map(m => m.id));
            // for (const k of this.seen.keys()) {
              // if (k.startsWith('moduleDownloaded:' + msg.componentId + ':') && !ids.has(Number(k.split(':')[2]))) this.seen.delete(k);
            // }
          // }
          // this.runtime.emitMessage(msg);
          // this.seen.set(key, signature);
        // }
      // } catch (err) { this.error = 'BMLデータの処理に失敗しました: ' + err.message; }
    },
    press(/* code */) {
      // if (!this.runtime) return;
      // if (this.audio && this.audio.state === 'suspended') this.audio.resume();
      // this.runtime.content.processKeyDown(keyCodeToAribKey(({ Up: 'ArrowUp', Down: 'ArrowDown', Left: 'ArrowLeft', Right: 'ArrowRight', Back: 'Backspace', DataButton: 'd', BlueButton: 'b', RedButton: 'r', GreenButton: 'g', YellowButton: 'y' })[code] || code.replace('Digit', '')));
      // this.runtime.content.processKeyUp(keyCodeToAribKey(({ Up: 'ArrowUp', Down: 'ArrowDown', Left: 'ArrowLeft', Right: 'ArrowRight', Back: 'Backspace', DataButton: 'd', BlueButton: 'b', RedButton: 'r', GreenButton: 'g', YellowButton: 'y' })[code] || code.replace('Digit', '')));
    },
  },
};
</script>

<style scoped>
.bml-window { padding: 6px; background: #202124; color: #fff; border-radius: 4px; }
.bml-toolbar, .bml-remote { display: flex; gap: 6px; align-items: center; flex-wrap: wrap; }
.bml-toolbar { margin-bottom: 6px; font-size: 12px; }
.bml-toolbar span { flex: 1; overflow-wrap: anywhere; }
.bml-viewport { position: relative; width: 100%; overflow: hidden; background: #000; }
.bml-stage, iframe { position: absolute; top: 0; left: 0; border: 0; }
.bml-remote { margin-top: 6px; }
button { background: #3c4043; color: white; border: 1px solid #888; border-radius: 3px; padding: 4px 9px; }
.bml-remote button { border-bottom-width: 4px; min-width: 32px; }
button:focus-visible { outline: 2px solid white; }
p { font-size: 12px; margin: 6px 0 0; }
</style>
