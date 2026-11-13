<template>

  <v-simple-table dense>

    <template v-slot:default >

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left servicesetting-name-column">
            地上波
          </th>
          <th class="text-left servicesetting-hashtag-column">HashTag</th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.tsall"
        :key="key"
        :class="item.View ? '' : 'servicesetting-off'"
        >
          <td class="servicesetting-name-column">
              <v-switch
              :input-value="item.View"
              @change="changeView(item.Sid)"
              >
                <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
                </template>
              </v-switch>
          </td>
          <td class="servicesetting-hashtag-column">
              <v-text-field
                :value="item.HashTag"
                dense
                hide-details
                class="servicesetting-hashtag"
                @change="changeHashTag(item.Sid, $event)"
              />
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-5">
        <tr>
          <th class="text-left servicesetting-name-column">
            BS
          </th>
          <th class="text-left servicesetting-hashtag-column">HashTag</th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.bsall"
        :key="key"
        :class="item.View ? '' : 'servicesetting-off'"
        >
          <td class="servicesetting-name-column">
              <v-switch
              :input-value="item.View"
              @change="changeView(item.Sid)"
              >
                <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
                </template>
              </v-switch>
          </td>
          <td class="servicesetting-hashtag-column">
              <v-text-field
                :value="item.HashTag"
                dense
                hide-details
                class="servicesetting-hashtag"
                @change="changeHashTag(item.Sid, $event)"
              />
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left servicesetting-name-column">
            CS
          </th>
          <th class="text-left servicesetting-hashtag-column">HashTag</th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.cs"
        :key="key"
        :class="item.View ? '' : 'servicesetting-off'"
        >
          <td class="servicesetting-name-column">
              <v-switch
              :input-value="item.View"
              @change="changeView(item.Sid)"
              >
                <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
                </template>
              </v-switch>
          </td>
          <td class="servicesetting-hashtag-column">
              <v-text-field
                :value="item.HashTag"
                dense
                hide-details
                class="servicesetting-hashtag"
                @change="changeHashTag(item.Sid, $event)"
              />
          </td>
        </tr>
      </tbody>

      <thead class="blue lighten-4">
        <tr>
          <th class="text-left servicesetting-name-column">
            ワンセグ
          </th>
          <th class="text-left servicesetting-hashtag-column">HashTag</th>
        </tr>
      </thead>
      <tbody>
        <tr
        v-for="item,key in station.os"
        :key="key"
        :class="item.View ? '' : 'servicesetting-off'"
        >
          <td class="servicesetting-name-column">
              <v-switch
              :input-value="item.View"
              >
                <template v-slot:label>
                <StationLogo v-if="options.displogo" :sid="item.Sid" />
                {{ item.Name + '(' + item.OrgSid + ')' }}
                </template>
              </v-switch>
          </td>
          <td class="servicesetting-hashtag-column">
              <v-text-field
                :value="item.HashTag"
                dense
                hide-details
                class="servicesetting-hashtag"
                @change="changeHashTag(item.Sid, $event)"
              />
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
    name: 'ServiceSetting',

    components: { StationLogo },

    props: [
        'options',
        'api',
        'station',
    ],

    methods: {
      // サービスのon/off
      changeView(id) {
        let val = this.station.all[id];
        val.View = (val.View == 0 ? 1 : 0);
        axios.put(this.api.station, JSON.stringify(val)).then(res => {
          utility.log(res.status);
          // API updateを行った時点でwsから通知が来て再描画
        }).catch(err => {
          utility.err(`request station error: ${err}`);
        });
      },

      // 局のHashTagを更新
      changeHashTag(id, hashTag) {
        let val = this.station.all[id];
        val.HashTag = hashTag;
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

.servicesetting-name-column {
  width: 50%;
}

.servicesetting-hashtag-column {
  width: 50%;
}

.servicesetting-hashtag {
  max-width: 100%;
}

</style>

