<template>

  <v-simple-table dense>

    <template v-slot:default >

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left">
            地上波
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.ts"
        :key="key"
        :class="item.FetchProgram ? '' : 'servicesetting-off'"
        >
          <td>
            <v-switch
            :input-value="item.FetchProgram"
            @change="changeFetchProgram(item.Sid)"
            >
              <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
              </template>
            </v-switch>
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-5">
        <tr>
          <th class="text-left">
            BS
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.bs"
        :key="key"
        :class="item.FetchProgram ? '' : 'servicesetting-off'"
        >
          <td>
            <v-switch
            :input-value="item.FetchProgram"
            @change="changeFetchProgram(item.Sid)"
            >
              <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
              </template>
            </v-switch>
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left">
            CS
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.cs"
        :key="key"
        :class="item.FetchProgram ? '' : 'servicesetting-off'"
        >
          <td>
            <v-switch
            :input-value="item.FetchProgram"
            @change="changeFetchProgram(item.Sid)"
            >
              <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
              </template>
            </v-switch>
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left">
            ワンセグ
          </th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.os"
        :key="key"
        :class="item.FetchProgram ? '' : 'servicesetting-off'"
        >
          <td>
            <v-switch
            :input-value="item.FetchProgram"
            @change="changeFetchProgram(item.Sid)"
            >
              <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
              </template>
            </v-switch>
          </td>
        </tr>
      </tbody>

    </template>

  </v-simple-table>

</template>

<script>
  import axios from 'axios'
  import utility from '../../utility.js';
  import StationLogo from '../StationLogo.vue';

  export default {
    name: 'FetchProgramSetting',

    components: { StationLogo },

    props: [
        'options',
        'api',
        'station',
    ],

    methods: {
      // 番組情報取得のon/off
      changeFetchProgram(id) {
        let val = this.station.all[id];
        val.FetchProgram = !val.FetchProgram;
        axios.put(this.api.station, JSON.stringify(val)).then(res => {
          utility.log(res.status);
          // API updateを行った時点でwsから通知が来て再描画
        }).catch(err => {
          utility.err(`request station error: ${err}`);
        });
      }
    },

    data() {
        return {
        }
    },
  }
</script>

<style scoped>
.servicesetting-off {
  background-color: var(--app-disabled, rgb(150, 150, 150));
}

</style>

