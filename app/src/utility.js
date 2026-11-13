const label = require('./define.js');

const newpat1 = /.*(🈟|\[新\])|（新）|\(新\)|＜新＞.*/g;
const endpat1 = /.*(🈡|\[終\])|（終）|\(終\)|＜終＞.*/g;
const firstpat1 = /.*第(1|一)回|第(1|一)話|第(1|一)幕|#0*1($|^[0-9])|（初）|🈠|No\.0*1($|^[0-9])/g;
const freepat1 = /.*(🈚|\[無\]).*/g;

const titlePrefixMarks = /^[\s🈀-🉑🅊-🅿🔴🟠🟡🟢🔵🟣⚪⚫★☆◇◆□■●○◎▽▼△▲]+/u;
const titleSuffixMarks = /[\s🈀-🉑🅊-🅿🔴🟠🟡🟢🔵🟣⚪⚫★☆◇◆□■●○◎▽▼△▲]+$/u;

let delayMilliseconds = 0;

const now = () => {
  let dt = new Date();
  dt.setMilliseconds(dt.getMilliseconds() + delayMilliseconds);
  return dt;
}

const getServerDelay = () => {
  return delayMilliseconds;
}

const setServerDate = (dt) => {
  let delay = dt - (new Date()).getTime();
  if (Math.abs(delay) < 600000) {
    delayMilliseconds = delay
  }
}

const stripBoundaryMarks = (text) => {
    return text.replace(titlePrefixMarks, '').replace(titleSuffixMarks, '').trim();
}

const extractUnifiedTitle = (title) => {
    if (!title) {
        return '';
    }

    let text = stripBoundaryMarks(String(title).replace(/[\s\u3000]+/g, ' ').trim());
    text = text.replace(/^(?:[【\x5b][^】\x5d]{1,30}[】\x5d][\s\u3000]*)+/, '').trim();
    text = stripBoundaryMarks(text);

    // 行頭のジャンル名＋カッコの、ジャンル名部分を除去する（例：アニメ「... -> 「...）
    text = text.replace(/^(?:アニメ|テレビアニメ|TVアニメ|ドラマ|映画|劇場版|劇場|時代劇|特撮)(?=[「『])/, '').trim();

    // 行頭がカッコの場合のみ、その中身を抽出する
    const match = text.match(/^[「『]([^」』]+)[」』]/);
    if (match) {
        text = match[1].trim();
    }

    // 通常のジャンル名除去（スペースあり）
    text = text.replace(/^(?:アニメ|テレビアニメ|TVアニメ|ドラマ|映画|劇場版|劇場|時代劇|特撮)[\s\u3000]+/, '').trim();

    let hasQuote = /[「『【\x5b]/.test(text);
    let quoteCandidate = text.split(/[「『【\x5b]/)[0].trim();
    let separatorCandidate = text.split(/[ \u3000★☆◇◆□■●○◎▽▼△▲]/)[0].trim();

    let ret = (hasQuote ? quoteCandidate : (separatorCandidate || quoteCandidate || text)).trim();
    ret = ret.replace(/[\s\u3000]*(?:[#＃][\s\u3000]*[0-9０-９]+\s*-\s*[#＃][\s\u3000]*[0-9０-９]+|[#＃][\s\u3000]*[0-9０-９]+)[\s\u3000]*$/i, '').trim();
    ret = ret.replace(/[\s\u3000]*(?:第[0-9０-９]+[話回週]|[（(][0-9０-９]+[）)])(?:[\s\u3000]*第[0-9０-９]+[話回週])*[\s\u3000]*$/g, '').trim();
    return stripBoundaryMarks(ret) || text;
}

const uxtmtomdhmj = (uxtm, te) => {
  let d = new Date(uxtm * label.MSEC);
  let hour = d.getHours();
  if (te && hour < 4) {
    d.setHours(-4);
    hour += 24;
  }
  let month = d.getMonth() + 1;
  let day = d.getDate();
  let min = ('0' + d.getMinutes()).slice(-2);
  return (month + '月' + day + '日(' +
  label.WEEKW[d.getDay()] + ')　' +
  hour + ':' + min);
}

const uxtmtomdh = (uxtm, org, te) => {
  let d = new Date(uxtm * label.MSEC);
  let h = d.getHours();
  if (te && h < 4) {
    d.setHours(-4);
    h += 24;
  }
  return (org.getFullYear() != d.getFullYear() ? d.getFullYear() + '/' : '') +
  (d.getMonth() + 1) +
  '/' + d.getDate() +
  '<small>(' +
  label.WEEKW[d.getDay()] +
  ')</small> ' + h + ':00';
}

const uxtmtomd = (uxtm, te) => {
  let d = new Date(uxtm * label.MSEC);
  let h = d.getHours();
  if (te && h < 4) {
    d.setHours(-4);
    h += 24;
  }
  let month = d.getMonth() + 1;
  let day = d.getDate();
  return ''+month+'月'+day+'日('+label.WEEKW[d.getDay()] + ')';
}

const uxtmtohm = (uxtm, te) => {
  let d = new Date(uxtm * label.MSEC);
  let h = d.getHours();
  if (te && h < 4) {
    d.setHours(-4);
    h += 24;
  }
  let min = ('0' + d.getMinutes()).slice(-2);
  return h + ':' + min;
}

const uxtmtoymdhmj = (uxtm, te) => {
  let d = new Date(uxtm * label.MSEC);
  let hour = d.getHours();
  if (te && hour < 4) {
    d.setHours(-4);
    hour += 24;
  }
  let year ="";
  let tyer = new Date().getFullYear();
  if (d.getFullYear() != tyer) {
    year = d.getFullYear() + "年";
  }
  let month = d.getMonth() + 1;
  let day = d.getDate();
  let min = ('0' + d.getMinutes()).slice(-2);
  return year+(month + '月' + day + '日(' +
  label.WEEKW[d.getDay()] + ')　' +
  hour + ':' + min);
}

const tmtoux = (t) => {
  return parseInt(t.getTime()/1000);
}

const zpad = (n,l) => {
  return ('00000000'+n).slice(-l);
}

const zpadTM = (t, te) => {
  let h = t.getHours();
  return (te && h < 4 ? h+24 : h) + ':' + ('00' + t.getMinutes()).slice(-2);
}

const zpadDT = (t) => {
  return (t.getMonth() + 1) + '月' + t.getDate() + '日（'+label.WEEKW[t.getDay()]+'）';
}

const htmlEscape = (text) => {
  if (!text) return '';
  return String(text)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

const socialRregex = /(x|tiktok|twitter|instagram)\s*@([a-zA-Z0-9_.]+)/ig;
function replaceSocialAccounts(text) {
  return text.replace(socialRregex, (match, platform, id) => {
    const platformLower = platform.toLowerCase();
    let url = "";
    if (platformLower === "x" || platformLower === "twitter") {
      url = `https://x.com/${id}`;
    } else if (platformLower === "tiktok") {
      url = `https://www.tiktok.com/@${id}`;
    } else if (platformLower === "instagram") {
      url = `https://www.instagram.com/${id}`;
    }
    return `<a href="${url}" target="_blank" rel="noopener noreferrer">${match}</a>`;
  });
}

const urlRegex = /(https?:\/\/[-a-zA-Z0-9@:%_+.~#?&//=()]+)/g;
const replaceTextWithLinks = (str) => {
  // URLは元の文字列から検出し、文章・属性・リンク文字列をそれぞれエスケープする。
  // 先に全文をエスケープすると、URL内の &amp; などで検出が途切れてしまう。
  const text = String(str || '');
  let html = '';
  let offset = 0;
  for (const match of text.matchAll(urlRegex)) {
    const url = htmlEscape(match[0]);
    html += htmlEscape(text.slice(offset, match.index)) +
      '<a href="' + url + '" target="_blank" rel="noopener noreferrer">' + url + '</a>';
    offset = match.index + match[0].length;
  }
  return html + htmlEscape(text.slice(offset));
}

const sleep = (ms) => new Promise(resolve => setTimeout(resolve, ms));

const waitForElement = async (id, maxAttempts = 30, interval = 100) => {
  for (let i = 0; i < maxAttempts; i++) {
    const el = document.getElementById(id);
    if (el) return el;
    await sleep(interval);
  }
  return null;
};

const triggerBlink = (tar) => {
  if (!tar || tar.classList.contains('selected-program-highlight')) return;
  tar.classList.add('selected-program-highlight');
  tar.addEventListener('animationend', () => {
    tar.classList.remove('selected-program-highlight');
  }, { once: true });
};

const scrollToTarget = async (id, retryCount = 0) => {
  if (retryCount >= 30) {
    return;
  }
  const tar = await waitForElement(id, 30, 100);
  if (!tar) {
    return;
  }

  tar.scrollIntoView({ behavior: 'smooth', block: 'start' });

  triggerBlink(tar);

  const checkCount = 30; 
  const checkInterval = 100;
  for (let i = 0; i < checkCount; i++) {
    await sleep(checkInterval);
    const currentEl = document.getElementById(id);
    if (!currentEl || currentEl !== tar) {
      scrollToTarget(id, retryCount + 1);
      return; 
    }
  }
};

function delayTicks(me, ticks, callback) {
  if (ticks <= 0) {
    callback();
    return;
  }
  me.$nextTick(() => {
    delayTicks(me, ticks - 1, callback);
  });
}

const weekOptions = label.WEEKW.map((name, day) => ({ text: name + '曜日', value: day }));

const weekMaskToList = mask => weekOptions.filter(item => (mask & (1 << item.value)) !== 0)
  .map(item => item.value);

const weekListToMask = list => (list || []).reduce((mask, day) => mask | (1 << day), 0);

module.exports = {
    newpat1: newpat1,
    endpat1: endpat1,
    firstpat1: firstpat1,
    freepat1: freepat1,
    titlePrefixMarks: titlePrefixMarks,
    titleSuffixMarks: titleSuffixMarks,
    now: now,
    getServerDelay: getServerDelay,
    setServerDate: setServerDate,
    stripBoundaryMarks: stripBoundaryMarks,
    extractUnifiedTitle: extractUnifiedTitle,
    uxtmtoymdhmj: uxtmtoymdhmj,
    uxtmtomdhmj: uxtmtomdhmj,
    uxtmtomdh: uxtmtomdh,
    uxtmtomd: uxtmtomd,
    tmtoux: tmtoux,
    uxtmtohm: uxtmtohm,
    zpad: zpad,
    zpadTM: zpadTM,
    zpadDT: zpadDT,
    htmlEscape: htmlEscape,
    replaceTextWithLinks: replaceTextWithLinks,
    replaceSocialAccounts: replaceSocialAccounts,
    scrollToTarget: scrollToTarget,
    delayTicks: delayTicks,
    weekOptions,
    weekMaskToList,
    weekListToMask,
    log: label.DEBUG ? console.log : () => {},
    err: console.error
}
