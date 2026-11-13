const { defineConfig } = require('@vue/cli-service')
const path = require('path')
process.env.VUE_APP_VERSION = (process.env.APP_VERSION || process.env.VUE_APP_VERSION ||
  process.env.CI_COMMIT_TAG || (process.env.GITHUB_REF_TYPE === 'tag' ? process.env.GITHUB_REF_NAME : '') || 'dev').replace(/^v/, '')

module.exports = defineConfig({

  pages: {
    index: {
      entry: "src/main.js",
      title: "TvRockGO",

    },
  },

  configureWebpack: {
    devtool: 'source-map',
  },

  devServer: {
    static: {
      directory: path.join(__dirname, 'public'),
      publicPath: '/static',
    },
  },

  transpileDependencies: [
    'vuetify'
  ],

  publicPath: './',

  outputDir: '../static'

})
