const label = require('./define.js');
const utility = require('./utility.js');

// 平均的な局名（全角8文字程度）とロゴ・操作ボタンに、少し広めの余白を確保する。
// 時間軸は途中と右端に各24pxあり、dividerによって本数が変わる。
const getServiceDisplayCount = (options, width) => {
  if (options.dispservice !== 0) return Math.max(1, Number(options.dispservice) || 1);
  const divider = Math.max(1, Number(options.divider) || 1);
  for (let count = 12; count > 1; count--) {
    if (count * 200 + (Math.ceil(count / divider) + 1) * 24 <= width) return count;
  }
  return 1;
};

// 局名と番組で同じ列幅を使う。時間列は局数にかかわらず読める幅を確保する。
const gridColumns = (sl, opt) => {
  const divider = opt.weekly ? 100 : Math.max(1, Number(opt.divider) || 1);
  const columns = [];
  for (let i = 0; i < sl; i++) {
    if (i % divider === 0) columns.push('24px');
    columns.push('minmax(0, 1fr)');
  }
  columns.push('24px');
  return columns.join(' ');
}

// 読み込み成功時だけ表示し、存在しない画像は取り除く。
const renderStationLogo = (sid, opt) => {
  if (!opt.displogo) {
    return '';
  }
  return '<img class="tt-station-logo" src="/static/cdt/' + encodeURIComponent(sid) +
    '_1" alt="" style="display:none" onload="this.style.display=\'\'" onerror="this.remove()">';
}

// サービス名描画
const renderStationHeaders = (st, dt, opt, add) => {
  if (!st || st.length<1)
      return '';
  let h = '<div class="hstkp tt-grid-header" style="grid-template-columns:' + gridColumns(dt.Week ? label.WEEK : dt.ServiceList.length, opt) + '">';
  if (dt.Week) {
    let sid = st[0];
    let d = new Date((dt.HeaderStart == null ? dt.Start : dt.HeaderStart) * label.MSEC);
    let s = dt.Sta[sid];
    if (s) {
      for (let i = 0; i < label.WEEK; i++) {
        if (i == 0) {
          h += '<div class="hstkn"></div>';
        }
        h += '<div class="stb' + sid + '" data-week-start="' + Math.floor(d.getTime() / label.MSEC) +
        '">' + (i == 0 ? renderStationLogo(sid, opt) + '<span class="tt-station-name" title="' + utility.htmlEscape(s.Name) + '">' + utility.htmlEscape(s.Name) + '</span>' +
        '<span class="mdi mdi-arrow-left-drop-circle" onclick="tvpWeeklyReverse('+sid+')" style="cursor:pointer"></span>' : utility.zpadDT(d)) +
        '</div>';
        d.setDate(d.getDate()+1);
      }
    }
  } else {
    let n = 0;
    for (const sid of dt.ServiceList) {
      let s = dt.Sta[sid];
      if (s) {
        if (n % Math.max(1, Number(opt.divider) || 1) === 0) {
          h += '<div class="hstkn"><span class="mdi '+(add<0?"mdi-arrow-up":"mdi-arrow-down")+'" onclick="changeHours('+add+')" style="cursor:pointer"></span></div>';
        }
        h += '<div class="stb'+sid+'">' + renderStationLogo(sid, opt) + '<span class="tt-station-name" title="' + utility.htmlEscape(s.Name) + '">' + utility.htmlEscape(s.Name) + '</span>' +
        '<span class="mdi mdi-arrow-right-drop-circle" onclick="tvpWeekly('+sid+')" style="cursor:pointer"></span></div>';
      }
      n++;
    }
  }
  h += '<div class="hstkn"><span class="mdi '+(add<0?"mdi-arrow-up":"mdi-arrow-down")+'" onclick="changeHours('+add+')" style="cursor:pointer"></span></div>';
  h += '</div>';
  return h;
}

// 仮想表初期化
const createMinuteGrid = (m, s) => {
  let tbl = new Array(m);
  for(let i = 0; i < m; i++) {
    tbl[i] = new Array(s).fill(0);
  }
  return tbl;
}

// 仮想表描画
const fillMinuteGrid = (so,dt,tbl,st,ed) => {
  if (so.length < 1)
    return tbl;
  // 番組ごとに該当する分を埋める・各分で全番組を検索し直さない。
  const columns = dt.Week ? Array.from({ length: label.WEEK }, (_, i) => i) : dt.ServiceList;
  columns.forEach((sid, column) => {
    const offset = dt.Week ? column * label.DAY : 0;
    for (const program of dt.Tvp[sid] || []) {
      const start = Math.max(st, program.StartM - offset);
      const end = Math.min(ed, program.EndM - offset);
      for (let minute = start; minute < end; minute++) {
        // 重複時は従来どおりリストの先頭を優先する。
        if (tbl[minute - st][column] === 0) tbl[minute - st][column] = program;
      }
    }
  });
  return tbl;
}

// 番組表描画
// html&cssはグローバル
// class="mdi mdi-home" でアイコン表示 
// cssは"assets/tvrock.css"で指定・このスクリプト内では定義しない
const getPixelsPerMinute = (options = {}) => [2, 4, 8].includes(options.hours) ? options.hours : 4;

const renderProgramGrid = (minuteGrid, startDate, minuteCount, stationCount, options, programs) => {
  const minutesPerHour = 60;
  // ノーマルは1分4px（5分枠20px）。ショート・ロングはその半分・倍の高さ。
  const pixelsPerMinute = getPixelsPerMinute(options);
  let html = '<div class="tt-program-grid" style="grid-template-columns:' + gridColumns(stationCount, options) + ';grid-template-rows:repeat(' + minuteCount + ', minmax(0, 1fr));height:' + (minuteCount * pixelsPerMinute) + 'px">';
  let now = utility.now().getTime();
  let weekAgo = now-24*60*60*7*1000;
  let timeColumnInterval = options.weekly ? 100 : Math.max(1, Number(options.divider) || 1);
  // 時刻列は時間ごと、番組列は連続する枠ごとに描画する。
  for (let row = 0; row < minuteCount; row += minutesPerHour) {
    const hour = (startDate.getHours() + Math.floor(row / minutesPerHour)) % 24;
    const displayHour = options.twentyEight && hour < 4 ? hour + 24 : hour;
    const timeCell = gridColumn => '<div data-grid-key="time_' + (startDate.getTime() + row * 60000) + '_' + gridColumn + '" class="hstk' + hour + '" style="grid-column:' + gridColumn +
      ';grid-row:' + (row + 1) + ' / span ' + Math.min(minutesPerHour, minuteCount - row) + '">' + displayHour + '</div>';
    for (let columnIndex = 0; columnIndex < stationCount; columnIndex += timeColumnInterval) {
      html += timeCell(columnIndex + Math.floor(columnIndex / timeColumnInterval) + 1);
    }
    html += timeCell(-2);
  }
  const guideType = Number(options.guidetype);
  for (let columnIndex = 0; columnIndex < stationCount; columnIndex++) {
    const gridColumn = columnIndex + Math.floor(columnIndex / timeColumnInterval) + 2;
    for (let row = 0; row < minuteCount;) {
      const program = minuteGrid[row][columnIndex] || 0;
      let duration = 0;
      for (let nextRow = row; nextRow < minuteCount && program === (minuteGrid[nextRow][columnIndex] || 0); nextRow++) {
        duration++;
      }
      let recentArchive = program !=0 ? program.EndD.getTime() < now && weekAgo <= program.EndD.getTime() : false;
      let ended = program !=0 ? program.EndD.getTime() < now : false;
      let programKey = program != 0 ? program.Sid + '_' + program.Eid + '_' + program.Start : '';
      if (program != 0) {
        programs[programKey] = program;
      }

      const startTime = program ? utility.zpadTM(program.StartD, options.twentyEight) : '';
      const endTime = program ? utility.zpadTM(program.EndD, options.twentyEight) : '';
      const contentHtml = program ? utility.replaceTextWithLinks(program.Content || '') : '';
      // 表示用だけを変換し、検索・予約で使う元の番組情報は保持する。
      const titleHtml = program ? utility.htmlEscape(program.Title) : '';
      const subtitleHtml = program ? utility.htmlEscape(program.Subtitle) : '';
      const deviceHtml = program ? utility.htmlEscape(program.DeviceID) : '';

      // 詳細on/off
      let detailEvents = guideType == 1 || guideType == 3 ?
      'onmouseenter="pgClick(' + program.Sid + ',' + program.Eid + ',1,event)" onmouseleave="pgClick(' +
      program.Sid + ',' + program.Eid + ',2)"' : '';
      if (guideType === 0 || guideType === 1) {
        detailEvents += ' onblur="pgClick(' + program.Sid + ',' + program.Eid + ',2)"';
      }
      let showInlineSubtitle = duration >= options.subtitlemin;
      let showInlineContent = duration >= options.contentmin;
      // Two lines fit above 30px; scale the compact label with the height mode.
      const shortLabel = duration * pixelsPerMinute <= 30;

      html += program!=0 ?
      '<div data-grid-key="program_' + columnIndex + '_' + programKey + '" id="L' + program.Sid  + '_' + program.Eid + '" class="' + (program.IsRec ? 'pgx' :
      (row != 0 && program.StartD.getTime() <= now && now < program.EndD.getTime() ? 'pgb' : 'pgc')) +
      utility.zpad(program.Cnib_a, 2) + utility.zpad(program.Cnib_b, 2) +
      (recentArchive ? 'opc' : '') + '" data-watch="' + !!(program.IsRec && program.IsWatch) + '" data-short="' + shortLabel + '" data-duration="' + duration +
      '" data-caption-key="' + programKey + '" tabindex="0" style="grid-column:' +
      gridColumn + ';grid-row:' + (row + 1) + ' / span ' + duration + '" ' + detailEvents + '><div class="pst">' +

      // 録画デバイス
      (program.IsRec ? '<div class="pdt' + (program.IsWatch ? ' pdt-watch' : '') + '">' + (program.Complete ? "※終了 - ": "") + deviceHtml + '</div>' : '') +
      '<p class="pdt">' + (program.IsRec ? '<i id="pcrec" class="mdi mdi-clock-outline"></i>':'') +

      // 内容
      startTime + '～' + endTime +
      ' ('+program.DurationM+'分・'+program.Eid+')' + '</p>' +
      '<div class="pbp"><p class="ptt"><span class="tt-short-time">' + startTime + '</span>' +
      '<span class="tt-title-text">' + titleHtml + '</span></p>' +
      (showInlineSubtitle ? '<p class="pts tt-description-spacing">' + subtitleHtml + '</p>':'') +
      (showInlineContent ? '<p class="ptc tt-description-spacing">' + contentHtml + '</p>':'') +
      '<div class="pcp" id="pcp' + program.Sid + '_' + program.Eid + '" style="display:none;">' +
      (program.IsRec ? '<div class="pdt' + (program.IsWatch ? ' pdt-watch' : '') + '">' + (program.Complete ? '※終了 - ' : '') + deviceHtml + '</div>' : '') +
      '<p class="pdt">' + startTime + '～' + endTime +
      ' (' + program.DurationM+'分・'+ program.Eid + ')</p>' +
      '<p class="ptt"><a href="https://google.com/search?q=' + encodeURIComponent(program.Title) +
      '&udm=14" target="_blank" rel="noopener noreferrer">' + titleHtml +
      '</a> <a href="https://google.com/search?q=' + encodeURIComponent(program.Title) +
      '&udm=50" target="_blank" rel="noopener noreferrer" class="tt-title-icon-link" title="Google検索 (udm=50)">&#x2726;</a></p>' +
      '<p class="pts tt-description-spacing">' + subtitleHtml + '</p>' +
      '<p class="ptc tt-description-spacing">' + contentHtml + '</p>' +
      '<div class="tt-program-actions">' +
      (ended ? '' : (program.IsRec && !program.Complete ?

      // 予約削除
      (program.StartD.getTime() <= now && now < program.EndD.getTime() ?
      '<label class="mdi mdi-clock-outline ctlbtn1" onclick="pgStop(' +
       program.ID + ')">' + '録画停止</label>' :
      (program.SearchID === 0 ?
      '<label class="mdi mdi-clock-outline ctlbtn1" onclick="pgDelete(' +
       program.ID + ')">' + '予約削除</label>' :
      '<label class="mdi mdi-clock-outline ctlbtn1" onclick="pgInvalid(' +
       program.ID + ')">' + '予約無効</label>')) :

      // 予約
      '<label class="mdi mdi-clock-outline ctlbtn1" onclick="pgReserve(' +
      program.Sid + ',' + program.Eid + ',' + program.Start + ',' + program.End +
      ')">録画予約</label>' +
      (options.watchbutton ?
      '<label class="mdi mdi-television ctlbtn1 tt-watch-button" onclick="pgReserve(' +
      program.Sid + ',' + program.Eid + ',' + program.Start + ',' + program.End +
      ',true)">視聴予約</label>' : '')

      )) +
      '<label class="mdi mdi-archive-search-outline ctlbtn2" onclick="pgSearch(\'' +
      utility.htmlEscape(programKey) + '\')">検索</label></div>' +
      
      '</div></div></div></div>'
      : '<div data-grid-key="blank_' + columnIndex + '_' + (startDate.getTime() + row * 60000) + '" class="pblank" style="grid-column:' + gridColumn + ';grid-row:' + (row + 1) + ' / span ' + duration + '"></div>';
      row += duration;
    }
  }
  return html + '</div>';
}

// 次の番組開始・終了までの時間を調べる
const getNextProgramTransitionDelay = (tt) => {
  let now = utility.now().getTime();
  let nxt = 0;
  const columns = tt.Week ? Array.from({ length: label.WEEK }, (_, i) => i) : tt.ServiceList;
  for (const column of columns) {
    for (const program of tt.Tvp[column] || []) {
      for (const time of [program.StartD.getTime(), program.EndD.getTime()]) {
        const delay = time - now;
        if (delay > 0 && (nxt === 0 || delay < nxt)) nxt = delay;
      }
    }
  }
  return nxt;
}


// HTMLを一括交換せず、残っている番組枠・局名・読み込み済みロゴを維持する。
const patchTimeTableHtml = (element, html) => {
  const template = element.ownerDocument.createElement('template');
  template.innerHTML = html || '';
  const patchChildren = (parent, incoming) => {
    const key = (node, index) => node.getAttribute('data-grid-key') ||
      node.getAttribute('data-caption-key') || index + ':' + node.className;
    const previous = new Map(Array.from(parent.children, (node, index) => [key(node, index), node]));
    let cursor = parent.firstElementChild;
    Array.from(incoming.children).forEach((next, index) => {
      const nodeKey = key(next, index);
      let node = previous.get(nodeKey);
      previous.delete(nodeKey);
      if (!node) {
        node = next.cloneNode(true);
        node._timeTableHtml = next.innerHTML;
        for (const child of node.querySelectorAll('.tt-program-grid > div, .tt-grid-header')) {
          child._timeTableHtml = child.innerHTML;
        }
      } else {
        for (const attribute of Array.from(node.attributes)) {
          if (!next.hasAttribute(attribute.name)) node.removeAttribute(attribute.name);
        }
        for (const attribute of Array.from(next.attributes)) {
          // 選択後の先読み・予約更新でも、実行中の点滅を打ち切らない。
          // 解除はscrollToTargetが登録したanimationendに任せる。
          const value = attribute.name === 'class' && next.hasAttribute('data-caption-key') &&
            node.classList.contains('selected-program-highlight') ?
            attribute.value + ' selected-program-highlight' : attribute.value;
          if (node.getAttribute(attribute.name) !== value) node.setAttribute(attribute.name, value);
        }
        if (next.matches('.tt-program-grid, .tt-virtual-timeline, .tt-virtual-grid')) {
          patchChildren(node, next);
        } else if (node._timeTableHtml !== next.innerHTML) {
          node.innerHTML = next.innerHTML;
          node._timeTableHtml = next.innerHTML;
        }
      }
      if (node !== cursor) parent.insertBefore(node, cursor);
      cursor = node.nextElementSibling;
    });
    for (const node of previous.values()) node.remove();
  };
  patchChildren(element, template.content);
};

module.exports = { createMinuteGrid, fillMinuteGrid, renderStationHeaders, renderProgramGrid, getNextProgramTransitionDelay, patchTimeTableHtml, getPixelsPerMinute, getServiceDisplayCount };
