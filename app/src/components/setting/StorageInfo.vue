<template>
  <div class="tab-full-height storage-info">
    <v-card-text>
      <v-subheader>ストレージ情報</v-subheader>

      <v-simple-table v-if="sortedStorageInfo.length > 0">
        <template v-slot:default>
          <tbody>
            <tr v-for="item in sortedStorageInfo" :key="item.key">
              <td class="storage-key">{{ item.key }}</td>
              <td>
                <div class="storage-usage">
                  <v-progress-linear
                    :value="item.percent"
                    height="18"
                    rounded
                    color="primary"
                  >
                    <strong :class="['storage-percent', item.percentClass]">{{ item.percentText }}</strong>
                  </v-progress-linear>
                  <div class="storage-detail">
                    {{ formatBytes(item.using) }} / {{ formatBytes(item.size) }}
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </template>
      </v-simple-table>

      <div v-else class="text-body-2 grey--text text--darken-1">
        ストレージ情報はありません
      </div>
    </v-card-text>
  </div>
</template>

<script>
  export default {
    name: 'StorageInfo',

    props: [
      'storageInfo',
    ],

    computed: {
      sortedStorageInfo() {
        if (!this.storageInfo) {
          return [];
        }

        return Object.keys(this.storageInfo)
          .sort()
          .map(key => {
            const value = this.storageInfo[key] || {};
            const using = Number(value.Using) || 0;
            const size = Number(value.Size) || 0;
            const rawPercent = size > 0 ? (using / size) * 100 : 0;
            const percent = Math.min(Math.max(rawPercent, 0), 100);

            return {
              key,
              using,
              size,
              percent,
              percentText: rawPercent.toFixed(1) + '%',
              percentClass: this.percentClass(rawPercent),
            };
          });
      },
    },

    methods: {
      percentClass(percent) {
        if (percent >= 90) {
          return 'storage-percent-red';
        }
        if (percent <= 80) {
          return 'storage-percent-blue';
        }
        return 'storage-percent-yellow';
      },

      formatBytes(bytes) {
        if (!Number.isFinite(bytes) || bytes <= 0) {
          return '0 B';
        }

        const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
        let value = bytes;
        let unitIndex = 0;

        while (value >= 1024 && unitIndex < units.length - 1) {
          value /= 1024;
          unitIndex++;
        }

        return value.toFixed(unitIndex === 0 ? 0 : 1) + ' ' + units[unitIndex];
      },
    },
  }
</script>

<style scoped>
.tab-full-height {
  min-height: 100%;
}

.storage-info {
  overflow: visible;
}

.storage-key {
  width: 180px;
  font-weight: 600;
  white-space: nowrap;
}

.storage-usage {
  max-width: 720px;
  padding: 12px 0;
}

.storage-percent {
  font-weight: 700;
}

.storage-percent-blue {
  color: #2888f8;
  text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.5);
}

.storage-percent-yellow {
  color: #f8f828;
  text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.5);
}

.storage-percent-red {
  color: #fd6537;
  text-shadow: 2px 2px 4px rgba(0, 0, 0, 0.5);
}

.storage-detail {
  margin-top: 4px;
  font-size: 0.85rem;
  color: var(--app-text-secondary, rgba(0, 0, 0, 0.6));
}
</style>
