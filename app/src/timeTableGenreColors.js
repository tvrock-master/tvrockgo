const lightOther = '#f0f0f080';
const darkOther = '#26262680f0"';

const timeTableGenres = [
  ['00', 'ニュース／報道', '#d4ffc880', '#242826f0'],
  ['01', 'スポーツ', '#ffd6d680', '#282624f0'],
  ['02', '情報／ワイドショー', lightOther, '#282428f0'],
  ['03', 'ドラマ', '#ecffca80', '#242826f0'],
  ['04', '音楽', '#d3f1f880', '#242828f0'],
  ['05', 'バラエティ', '#fbffc980', '#282624f0'],
  ['06', '映画', '#d9f5f080', '#242828f0'],
  ['07', 'アニメ／特撮', '#d0d0ff80', '#242628f0'],
  ['08', 'ドキュメンタリー／教養', '#f6f6f680', darkOther],
  ['09', '劇場／公演', lightOther, darkOther],
  ['10', '趣味／教育', lightOther, darkOther],
  ['11', '福祉', lightOther, darkOther],
  ['other', 'その他・予備', lightOther, darkOther],
].map(([code, name, light, dark]) => ({ code, name, light, dark }));

function isGenreColor(color) {
  return typeof color === 'string' && /^#[0-9a-f]{6}([0-9a-f]{2})?$/i.test(color);
}

function normalizeGenreColors(saved) {
  const colors = { light: {}, dark: {} };
  for (const theme of ['light', 'dark']) {
    const overrides = saved && saved[theme];
    if (!overrides) continue;
    for (const { code } of timeTableGenres) {
      if (isGenreColor(overrides[code])) colors[theme][code] = overrides[code];
    }
  }
  return colors;
}

module.exports = { timeTableGenres, isGenreColor, normalizeGenreColors };
