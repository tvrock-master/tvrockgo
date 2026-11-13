const assert = require('assert');
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright');
const compiler = require('vue-template-compiler');

async function main() {
  const source = fs.readFileSync(path.join(__dirname, '../src/components/CaptionHistoryWindow.vue'), 'utf8');
  const sfc = compiler.parseComponent(source);
  const browser = await chromium.launch({ channel: 'msedge', headless: true });
  try {
    const page = await browser.newPage();
    await page.setContent('<div id="app"></div>');
    await page.addScriptTag({ path: require.resolve('vue/dist/vue.js') });
    await page.addScriptTag({ path: require.resolve('vuetify/dist/vuetify.js') });
    const errors = await page.evaluate(async ({ script, template }) => {
      const errors = [];
      Vue.config.errorHandler = error => errors.push(String(error));
      const axios = { get: async () => ({ data: { items: [{ ID: 1, Text: '字幕' }], snapshot: 1, hasMore: false } }) };
      const component = new Function('axios', script.replace("import axios from 'axios';", '').replace('export default', 'return'))(axios);
      component.template = template;
      const app = new Vue({
        vuetify: new Vuetify(),
        components: { CaptionHistoryWindow: component },
        data: () => ({ details: false, program: null, anchor: null, count: 0 }),
        template: `<div><div v-if="details" class="details">詳細</div><CaptionHistoryWindow v-if="program" :key="program.Eid" :program="program" :anchor="anchor" api="/caption" @close="program = null"/><button @click="count++">番組表操作</button></div>`,
      }).$mount('#app');
      app.anchor = app.$el.querySelector('button');
      for (let i = 0; i < 30; i++) {
        app.details = false;
        app.program = { Sid: 101, Eid: i, Start: 100, Title: '番組' + i };
        await Vue.nextTick();
        await Vue.nextTick();
        if (document.querySelectorAll('body > .caption-history').length !== 1) errors.push('panel missing');
        // Opening another program inserts details before the existing caption component.
        app.details = true;
        app.program = null;
        await Vue.nextTick();
        if (document.querySelector('.caption-history')) errors.push('orphan panel');
        app.$el.querySelector('button').click();
        await Vue.nextTick();
        if (app.count !== i + 1 || !app.$el.querySelector('.details')) errors.push('timetable stopped updating');
      }
      if (!errors.length) app.$destroy();
      return errors;
    }, { script: sfc.script.content, template: sfc.template.content });
    assert.deepStrictEqual(errors, []);
    console.log('Caption history repeated sibling insertion / cleanup: OK (30 cycles)');
  } finally { await browser.close(); }
}
main().catch(error => { console.error(error); process.exitCode = 1; });
