<template>
  <div class="tab-full-height">
    <v-card-text>
      <div class="text-subtitle-1 mb-4">TvRockGO ({{ version }})</div>
      <v-subheader></v-subheader>
      <v-simple-table>
        <template v-slot:default>
          <tbody>
            <tr>
              <td>
                <v-btn color="error" depressed @click="openLogoutConfirm">
                  <v-icon left>mdi-logout</v-icon>
                  ログアウト
                </v-btn>
              </td>
            </tr>
          </tbody>
        </template>
      </v-simple-table>
    </v-card-text>
    <confirm-dialog ref="confirmDialog"></confirm-dialog>
  </div>
</template>

<script>
  import ConfirmDialog from '../ConfirmDialog.vue';

  export default {
    name: 'LogoutPage',

    data() {
      return { version: process.env.VUE_APP_VERSION || 'dev' };
    },

    components: {
      ConfirmDialog,
    },

    methods: {
      async openLogoutConfirm() {
        const confirmed = await this.$refs.confirmDialog.open({
          title: 'ログアウト',
          message: 'ログアウトします。よろしいですか？',
          okText: 'ログアウト',
          okColor: 'error',
        });
        if (!confirmed) {
          return;
        }
        window.location.href = '/logout';
      },
    },
  }
</script>

<style scoped>
</style>
