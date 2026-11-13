import Vue from 'vue';
import Vuetify from 'vuetify/lib/framework';
import colors from 'vuetify/lib/util/colors'

Vue.use(Vuetify);

export default new Vuetify({
    theme: {
      disable: false,
      dark: window.matchMedia('(prefers-color-scheme: dark)').matches,
      themes: {
        light: {
          primary: colors.blueGrey.darken1, // #E53935
          secondary: colors.red.lighten4, // #FFCDD2
          accent: colors.grey.darken3, // #3F51B5
          error: colors.red,
          warning: colors.yellow,
          info: colors.grey,
          success: "#795548",
          appcolor: colors.blueGrey.darken1,
          mycolor: "#e0e0e0",
          settings: "#f0f0f0",
          settingsslider: "#757575",
        },
        dark: {
          primary: "#90b4c8",
          secondary: "#c9959c",
          accent: "#bac6cf",
          error: "#ef858b",
          warning: "#e4c779",
          info: "#9aafc0",
          success: "#bca398",
          appcolor: "#263640",
          mycolor: "#303b45",
          settings: "#252f39",
          settingsslider: "#90b4c8",
        },
      },
    },
});
