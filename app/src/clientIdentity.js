const client = 'TvRockApp/' + (process.env.VUE_APP_VERSION || 'dev');

export default {
  client,
  socketURL(url) {
    const target = new URL(url);
    target.searchParams.set('X-TvRock-Client', client);
    return target.href;
  },
};
