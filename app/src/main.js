import Vue from 'vue'
import App from './App.vue'
import vuetify from './plugins/vuetify'
import axios from 'axios'
import clientIdentity from './clientIdentity'
axios.defaults.headers.common['X-TvRock-Client'] = clientIdentity.client
// Importing the global css file
import "@/assets/tvrock.css"
import "@/assets/theme.css"

Vue.config.productionTip = false

new Vue({
  vuetify,
  render: h => h(App)
}).$mount('#app')
