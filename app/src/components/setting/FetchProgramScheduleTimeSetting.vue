<template>
  <div class="tab-full-height">
    <v-card-text>
      <v-subheader>番組情報取得時刻</v-subheader>
      <v-row dense>
        <v-col
        v-for="hour in hours"
        :key="hour"
        cols="6"
        sm="4"
        md="3"
        lg="2"
        >
          <v-switch
          :input-value="selectedHours.indexOf(hour) >= 0"
          :label="hour + '時'"
          :disabled="loading || immediateLoading"
          dense
          hide-details
          @change="changeHour(hour)"
          >
          </v-switch>
        </v-col>
      </v-row>
      <v-divider class="my-4"></v-divider>
      <v-row justify="center">
        <v-btn
        color="primary"
        :loading="immediateLoading"
        :disabled="loading || immediateLoading"
        @click="fetchProgramImmediately"
        >
          即時番組情報取得
        </v-btn>
      </v-row>
    </v-card-text>
  </div>
</template>

<script>
  import axios from 'axios'
  import utility from '../../utility.js';

  export default {
    name: 'FetchProgramScheduleTimeSetting',

    props: [
        'api',
    ],

    data() {
        return {
          hours: Array.from({ length: 24 }, (v, i) => i),
          selectedHours: [],
          loading: false,
          immediateLoading: false,
        }
    },

    created() {
      this.load();
    },

    methods: {
      load() {
        if (!this.api.fetchProgramSchedule) {
          return;
        }

        this.loading = true;
        axios.get(this.api.fetchProgramSchedule).then(res => {
          this.selectedHours = this.normalizeHours(res.data.hours || []);
        }).catch(err => {
          utility.err(`request fetch program schedule setting error: ${err}`);
        }).finally(() => {
          this.loading = false;
        });
      },

      changeHour(hour) {
        const next = this.selectedHours.slice();
        const index = next.indexOf(hour);
        if (index >= 0) {
          next.splice(index, 1);
        } else {
          next.push(hour);
        }

        this.selectedHours = this.normalizeHours(next);
        this.save();
      },

      save() {
        this.loading = true;
        axios.put(this.api.fetchProgramSchedule, {
          hours: this.selectedHours,
        }).then(res => {
          this.selectedHours = this.normalizeHours(res.data.hours || []);
        }).catch(err => {
          utility.err(`update fetch program schedule setting error: ${err}`);
          this.load();
        }).finally(() => {
          this.loading = false;
        });
      },

      normalizeHours(hours) {
        return hours
          .filter(hour => hour >= 0 && hour <= 23)
          .filter((hour, index, self) => self.indexOf(hour) === index)
          .sort((a, b) => a - b);
      },

      fetchProgramImmediately() {
        if (!this.api.fetchProgramSchedule) {
          return;
        }

        this.immediateLoading = true;
        axios.post(this.api.fetchProgramSchedule + '/immediate').then(() => {
          utility.log("immediate fetch program schedule success");
        }).catch(err => {
          utility.err(`request immediate fetch program schedule error: ${err}`);
        }).finally(() => {
          this.immediateLoading = false;
        });
      },
    },
  }
</script>

<style scoped>

.tab-full-height {
  min-height: 100%;
}

</style>
