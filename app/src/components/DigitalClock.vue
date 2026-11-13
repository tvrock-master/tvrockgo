<template>
  <time class="gray--text digital-clock">
    <span class="clock-date">
      <span class="clock-year">{{ year }} </span>
      <span class="clock-day-full">{{ day }}</span>
      <span class="clock-day-short">{{ shortDay }}</span>
      <small>({{ week }})</small>
    </span>

    <span class="clock-time">
      <span>{{ hours }}</span>

      <span :style="{ visibility: !blink || seconds % 2 === 0 ? 'visible' : 'hidden' }">:</span>

      <span>{{ minutes }}</span>

      <span v-if="displaySeconds" :style="{visibility: !blink || seconds % 2 === 0 ? 'visible' : 'hidden'}">:</span>

      <span v-if="displaySeconds">{{ seconds }}</span>

      <span v-if="twelveHour">{{ amPm }}</span>
    </span>

    <small v-if="displayDelay" class="clock-delay">({{ delay }}ms)</small>
  </time>
</template>

<script>
import label from '../define.js';
import utility from '../utility.js';

export default {
  name: "DigitalClock",
  props: ["blink", "displaySeconds", "displayDelay", "twelveHour", "twentyEight"],
  data() {
    return { ticker: null, currentTime: utility.now(), serverDelay: utility.getServerDelay() };
  },
  computed: {
    displayDate() {
      const date = new Date(this.currentTime);
      if (this.twentyEight && date.getHours() < 4) date.setDate(date.getDate() - 1);
      return date;
    },
    year() { return this.displayDate.getFullYear() + '年'; },
    day() { return (this.displayDate.getMonth() + 1) + '月' + this.displayDate.getDate() + '日'; },
    shortDay() { return (this.displayDate.getMonth() + 1) + '/' + this.displayDate.getDate(); },
    week() { return label.WEEKW[this.displayDate.getDay()]; },
    hours() {
      const hour = this.currentTime.getHours();
      const displayHour = this.twelveHour ? (hour % 12 || 12) :
        (this.twentyEight && hour < 4 ? hour + 24 : hour);
      return utility.zpad(displayHour, 2);
    },
    minutes() { return utility.zpad(this.currentTime.getMinutes(), 2); },
    seconds() { return utility.zpad(this.currentTime.getSeconds(), 2); },
    amPm() { return this.currentTime.getHours() >= 12 ? 'pm' : 'am'; },
    delay() { return (this.serverDelay > 0 ? '+' : '') + this.serverDelay; },
  },
  created() {
    this.ticker = setInterval(() => {
      this.currentTime = utility.now();
      this.serverDelay = utility.getServerDelay();
    }, 1000);
  },
  beforeDestroy() {
    clearInterval(this.ticker);
  },
};
</script>

<style scoped>
.digital-clock {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
}

.clock-date,
.clock-time,
.clock-delay {
  white-space: nowrap;
}

.clock-day-short {
  display: none;
}

@media (max-width: 1263px) {
  .digital-clock {
    flex-direction: column;
    gap: 0;
    font-size: 14px;
    line-height: 1.4;
  }

  .clock-year,
  .clock-day-full,
  .clock-delay {
    display: none;
  }

  .clock-day-short {
    display: inline;
  }
}
</style>
