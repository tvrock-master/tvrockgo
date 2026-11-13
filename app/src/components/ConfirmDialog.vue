<template>
  <v-dialog
    v-model="visible"
    :max-width="maxWidth"
    :overlay-opacity="overlayOpacity"
    content-class="confirm-dialog-content"
    persistent
  >
    <div class="confirm-dialog">
      <div class="confirm-dialog__header">
        <v-icon class="confirm-dialog__icon" color="warning">mdi-alert-circle-outline</v-icon>
        <div class="confirm-dialog__title">{{ dialogTitle }}</div>
      </div>
      <div class="confirm-dialog__message">{{ dialogMessage }}</div>
      <div class="confirm-dialog__actions">
        <v-btn
          class="confirm-dialog__cancel"
          text
          @click="cancel"
        >
          {{ cancelText }}
        </v-btn>
        <v-btn
          class="confirm-dialog__ok"
          :color="okColor"
          depressed
          @click="ok"
        >
          {{ okText }}
        </v-btn>
      </div>
    </div>
  </v-dialog>
</template>

<script>
export default {
  name: 'ConfirmDialog',
  props: {
    maxWidth: {
      type: [Number, String],
      default: 420,
    },
    overlayOpacity: {
      type: [Number, String],
      default: 0.62,
    },
  },
  data() {
    return {
      visible: false,
      dialogTitle: '確認',
      dialogMessage: '',
      okText: 'OK',
      cancelText: 'キャンセル',
      okColor: 'error',
      resolver: null,
    };
  },
  methods: {
    open(options) {
      const opts = options || {};
      this.dialogTitle = opts.title || '確認';
      this.dialogMessage = opts.message || '';
      this.okText = opts.okText || 'OK';
      this.cancelText = opts.cancelText || 'キャンセル';
      this.okColor = opts.okColor || 'error';
      this.visible = true;

      return new Promise(resolve => {
        this.resolver = resolve;
      });
    },
    ok() {
      this.close(true);
    },
    cancel() {
      this.close(false);
    },
    close(result) {
      this.visible = false;
      if (this.resolver) {
        this.resolver(result);
        this.resolver = null;
      }
    },
  },
};
</script>

<style scoped>
.confirm-dialog {
  border-radius: 6px;
  background: var(--app-surface, #fff);
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.32);
  overflow: hidden;
}

.confirm-dialog__header {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 20px 8px;
}

.confirm-dialog__icon {
  flex: 0 0 auto;
}

.confirm-dialog__title {
  color: var(--app-text, #202020);
  font-size: 18px;
  font-weight: 600;
  line-height: 1.35;
}

.confirm-dialog__message {
  padding: 8px 20px 18px 54px;
  color: var(--app-text-secondary, #424242);
  font-size: 14px;
  line-height: 1.6;
  white-space: pre-line;
}

.confirm-dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 16px 16px;
  background: var(--app-surface-soft, #f6f8fa);
}

.confirm-dialog__cancel,
.confirm-dialog__ok {
  min-width: 96px;
}

::v-deep .confirm-dialog-content {
  margin: 24px;
}
</style>
