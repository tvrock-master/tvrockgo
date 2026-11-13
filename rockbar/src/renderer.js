const $ = id => document.getElementById(id);
const api = window.rockbar;
if (api.platform === 'browser') {
  document.body.classList.add('browser');
  for (const id of ['pin', 'minimize', 'close']) $(id).hidden = true;
  $('server').readOnly = true;
  $('connection-help').textContent = '表示中のtvrockgoに接続します。ログインは別タブで行い、完了後にこのタブへ戻ってください。';
}
let devices = [], stations = [], selected = localStorage.getItem('device') || '', connected = false;
let pending = false, pendingTimer, noticeTimer;
let timekeeperMode = false;
let displayedDevice = null;
let followedService = '';
let powerManagerSetting = null, sleepTimerPending = false;
let nativeViewer=false, remoteViewerState={configured:[],views:[],pending:[]}, viewerSettings=[];
function remoteConfigured(device) { return nativeViewer && remoteViewerState.configured.includes(device?.RegistryCode); }
function remoteView(device) { return remoteViewerState.views.find(v=>v.device===device?.RegistryCode); }
function remoteViewText(device) {
  const view=remoteView(device); if (!view) return '';
  const labels={preparing:'準備中','waiting-source':'送信デバイス待ち','waiting-receiver':'受信準備待ち','checking-source':'送信状態を確認中',starting:'送信開始中',streaming:'配信中',receiving:'受信中',stopping:'終了中'};
  return ' · このPCの視聴: '+(labels[view.phase]||view.phase);
}
function renderSleepTimer() {
  const button = $('sleep-timer');
  const enabled = !!powerManagerSetting?.powermanager;
  const on = enabled && !powerManagerSetting.sleeptimerpause;
  button.hidden = !enabled;
  button.disabled = !connected || sleepTimerPending;
  button.setAttribute('aria-pressed', String(on));
  button.title = `スリープタイマー: ${on ? 'ON（クリックで一時停止）' : 'OFF（クリックで再開）'}`;
  button.setAttribute('aria-label', button.title);
}
let programs = {}, guideError = '', guideSignature = '', connectionMessage = '';
const multiWaves = new Map();
const serverClock = new rockbarTime.ServerClock();
const playbackClocks = new Map();
const knownPlaybackSessions = new Set();
let playbackFiles=[], playbackFilesLoading=false, playbackFilesError='', playbackFilesDirty=true, playbackFilesRequest=0;
let playbackFileOrder=localStorage.getItem('playback-file-order')||'date-desc';
const playbackFileOrders=['date-desc','date-asc','title-asc','title-desc'];
if(!playbackFileOrders.includes(playbackFileOrder))playbackFileOrder='date-desc';
const playbackFileSelection=new Map();
let playbackScrollSession=null;
function currentPlaybackFileKey(device) {
  const playback=device?.Playback;
  return playback?.scheduleId && ['indexing','preparing','playing','paused'].includes(playback.phase)?`${playback.slot||0}:${playback.scheduleId}`:null;
}
function scrollToCurrentPlaybackFile() {
  const device=devices.find(item=>item.ID===selected);
  if(timekeeperMode||device?.Kind!=='playback')return false;
  const key=currentPlaybackFileKey(device);
  if(!key)return scrollToSelectedPlaybackFile();
  playbackFileSelection.set(device.Playback.session,key);
  playbackScrollSession=device.Playback.session;
  renderPlaybackFiles();
  return playbackScrollSession===null;
}
function scrollToSelectedPlaybackFile() {
  if(timekeeperMode || devices.find(item=>item.ID===selected)?.Kind!=='playback')return false;
  const container=$('playback-file-list');
  const row=container.querySelector('[aria-selected="true"]');
  if(!row || !container.clientHeight)return false;
  const bounds=container.getBoundingClientRect(),target=row.getBoundingClientRect();
  container.scrollTo({top:container.scrollTop+target.top-bounds.top-container.clientTop+(target.height-container.clientHeight)/2,behavior:matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth'});
  playbackScrollSession=null;
  return true;
}
let renderedPlaybackFiles=null;
let sortedPlaybackFiles=null;
let playbackCreating=false, playbackCreateTimer;
async function refreshPlaybackFiles() {
  if(!connected || playbackFilesLoading || !api.playbackRecordings)return;
  const request=++playbackFilesRequest;
  playbackFilesDirty=false;playbackFilesLoading=true;playbackFilesError='';renderPlaybackFiles();
  try {const rows=await api.playbackRecordings();if(request===playbackFilesRequest && connected)playbackFiles=rockbarPlaybackFiles.reconcile(playbackFiles,Array.isArray(rows)?rows:[]);}
  catch(error){if(request===playbackFilesRequest)playbackFilesError=error.message;}
  finally {if(request===playbackFilesRequest){playbackFilesLoading=false;renderPlaybackFiles();if(playbackFilesDirty && devices.find(device=>device.ID===selected)?.Kind==='playback')void refreshPlaybackFiles();}}
}
function selectedPlaybackFile() {
  const device=devices.find(item=>item.ID===selected);
  return playbackFiles.find(row=>rockbarPlaybackFiles.key(row)===playbackFileSelection.get(device?.Playback?.session));
}
async function loadPlaybackFile(row) {
  if(!row)return;
  try {const result=await api.playback(selected,'load',{id:row.id,slot:row.slot||0});if(result?.error)notice(result.error);}
  catch(error){notice(error.message);}
}
function renderPlaybackFiles() {
  const device=devices.find(item=>item.ID===selected);if(device?.Kind!=='playback')return;
  const session=device.Playback.session;
  const currentKey=currentPlaybackFileKey(device);
  if(!playbackFileSelection.has(session)&&currentKey)playbackFileSelection.set(session,currentKey);
  const selection=playbackFileSelection.get(session);
  const previous=renderedPlaybackFiles;
  const status=playbackFiles.length?'':playbackFilesError||(playbackFilesLoading?'録画一覧を取得中…':'録画ファイルはありません');
  if(previous && previous.rows===playbackFiles && previous.order===playbackFileOrder && previous.session===session && previous.status===status) {
    if(previous.selection!==selection) {
      $('playback-file-list').querySelector('[aria-selected="true"]')?.setAttribute('aria-selected','false');
      if(selection)$('playback-file-list').querySelector(`[data-file="${CSS.escape(selection)}"]`)?.setAttribute('aria-selected','true');
      previous.selection=selection;
    }
    if(playbackScrollSession===session)scrollToSelectedPlaybackFile();
    return;
  }
  renderedPlaybackFiles={rows:playbackFiles,order:playbackFileOrder,session,selection,status};
  const sortButton=$('playback-file-sort');
  sortButton.textContent={'date-desc':'日付 ↓','date-asc':'日付 ↑','title-asc':'タイトル ↑','title-desc':'タイトル ↓'}[playbackFileOrder];
  sortButton.title=({'date-desc':'日付が新しい順','date-asc':'日付が古い順','title-asc':'タイトル昇順','title-desc':'タイトル降順'}[playbackFileOrder])+'（クリックで切り替え）';
  sortButton.setAttribute('aria-label',sortButton.title);
  const list=$('playback-file-list');
  const existing=new Map([...list.querySelectorAll('[data-file]')].map(button=>[button.dataset.file,button]));
  if(!sortedPlaybackFiles || sortedPlaybackFiles.source!==playbackFiles || sortedPlaybackFiles.order!==playbackFileOrder)sortedPlaybackFiles={source:playbackFiles,order:playbackFileOrder,rows:rockbarPlaybackFiles.sorted(playbackFiles,playbackFileOrder)};
  const rows=sortedPlaybackFiles.rows;
  if(!rows.length){if(list.textContent!==status)list.textContent=status;return;}
  const nodes=rows.map(row=>{
    const key=rockbarPlaybackFiles.key(row);let button=existing.get(key);
    if(!button){button=document.createElement('button');button.type='button';button.dataset.file=key;button.className='playback-file';button.setAttribute('role','option');}
    const text=(row.recording?'● 録画中  ':'')+(row.title||'タイトルなし');
    if(button.textContent!==text)button.textContent=text;
    if(button.recordingRow!==row){button.title=rockbarPlaybackFiles.details(row);button.recordingRow=row;}
    const selectedValue=String(selection===key);
    if(button.getAttribute('aria-selected')!==selectedValue)button.setAttribute('aria-selected',selectedValue);
    button.onclick=()=>{playbackFileSelection.set(session,key);render();};
    button.ondblclick=()=>loadPlaybackFile(row);
    button.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();loadPlaybackFile(row);}};
    return button;
  });
  if(nodes.length!==list.children.length||nodes.some((node,i)=>list.children[i]!==node))list.replaceChildren(...nodes);
  if(playbackScrollSession===session)scrollToSelectedPlaybackFile();
}
$('playback-file-sort').onclick=()=>{playbackFileOrder=playbackFileOrders[(playbackFileOrders.indexOf(playbackFileOrder)+1)%playbackFileOrders.length];localStorage.setItem('playback-file-order',playbackFileOrder);renderPlaybackFiles();$('playback-file-list').scrollTop=0;};
$('playback-device').onclick=async()=>{
  if(playbackCreating)return;
  playbackCreating=true;render();
  try {const result=await api.playback('','create',0);if(result?.error){playbackCreating=false;notice(result.error);}}
  catch(error){playbackCreating=false;notice(error.message);}
  clearTimeout(playbackCreateTimer);playbackCreateTimer=setTimeout(()=>{playbackCreating=false;render();},5000);render();
};
let playbackSeekPreview = null;
const playbackStatus = new rockbarPlaybackStatus.PlaybackStatus();
let tickerMessageTimer;
function scheduleTickerMessage() {
  clearTimeout(tickerMessageTimer);
  const remaining = playbackStatus.remaining();
  if (remaining > 0) tickerMessageTimer = setTimeout(() => render(), remaining + 1);
}
const tickerContainer = $('ticker').parentElement;
let tickerPaused = false;
function resetTickerToClock() {
  // Use layout properties for the stopped position. Seeking a paused CSS
  // animation can remain pending on the compositor until it is played again.
  tickerContainer.classList.add('ticker-paused');
}
function resumeTickerFromClock() {
  tickerContainer.classList.remove('ticker-paused');
  const ticker = $('ticker');
  const animation = ticker.getAnimations().find(animation => animation.animationName === 'marquee');
  if (!animation) return;
  const width = ticker.getBoundingClientRect().width;
  const padding = parseFloat(getComputedStyle(ticker).paddingLeft) || 0;
  animation.pause();
  animation.currentTime = width > 0 ? Number(animation.effect.getTiming().duration) * padding / width : 0;
  animation.play();
}
function toggleTicker() {
  tickerPaused = !tickerPaused;
  if (tickerPaused) resetTickerToClock();
  else resumeTickerFromClock();
  tickerContainer.setAttribute('aria-pressed', String(tickerPaused));
  tickerContainer.setAttribute('aria-label', tickerPaused ? 'ステータスのスクロールを再開' : 'ステータスのスクロールを先頭で停止');
}
tickerContainer.setAttribute('role', 'button');
tickerContainer.tabIndex = 0;
tickerContainer.setAttribute('aria-pressed', 'false');
tickerContainer.setAttribute('aria-label', 'ステータスのスクロールを先頭で停止');
tickerContainer.addEventListener('click', toggleTicker);
tickerContainer.addEventListener('keydown', event => {
  if ((event.key === 'Enter' || event.key === ' ') && !event.repeat) { event.preventDefault(); toggleTicker(); }
});
window.addEventListener('resize', () => { if (tickerPaused) resetTickerToClock(); });
document.fonts.addEventListener('loadingdone', () => { if (tickerPaused) resetTickerToClock(); });
function renderTicker(text) {
  const value = playbackStatus.message() || text;
  if ($('ticker').textContent !== value) {
    $('ticker').textContent = value;
    if (tickerPaused) resetTickerToClock();
  }
  $('ticker').parentElement.title = value;
}
function formatPlaybackTime(milliseconds) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60].map(value => String(value).padStart(2, '0')).join(':');
}
let playbackBoundaries=[],playbackBoundarySignature='',playbackMarkersEnabled=null;
let bookmarkPending=false,bookmarkMenuTarget=null;
function closeBookmarkMenu() { $('playback-bookmark-menu').hidden=true;bookmarkMenuTarget=null; }
function renderPlaybackBoundaries(device) {
  const playback=device.Playback;
  const signature=JSON.stringify([playback.session,playback.durationMs,playback.cmStatus,playback.cmBoundaries,playback.manualBookmarks,playback.programBoundaries,playback.phase==='indexing',playback.phase==='idle']);
  if(signature===playbackBoundarySignature)return;
  playbackBoundarySignature=signature;
  closeBookmarkMenu();
  playbackBoundaries=rockbarPlaybackBoundaries.markers(playback);
  $('playback-cm-status').textContent=rockbarPlaybackBoundaries.status(playback,playbackBoundaries);
  const duration=playback.durationMs||0;
  $('playback-cm-markers').replaceChildren(...playbackBoundaries.map(row=>{
    const button=document.createElement('button');button.type='button';button.className='playback-cm-marker';
    button.style.left=`${duration?row.positionMs/duration*100:0}%`;
    const manual=row.kind==='manual';
    button.classList.toggle('manual',manual);
    const program=row.kind==='program-change';button.classList.toggle('program-boundary',program);
    const kind=manual?'しおり':program?'番組切替':row.kind==='splice-out'?'CM開始候補':row.kind==='splice-in'?'CM終了候補':'境界候補';
    button.title=`${kind} ${formatPlaybackTime(row.positionMs)}`+(manual?'（右クリックまたはDeleteキーで削除）':program?`（現在番組ID ${row.previousEventId??0} → ${row.eventId??0} / EIT p/f）`:'（TS制御信号による候補・未確定）');
    button.setAttribute('aria-label',`${button.title}へ移動`);
    button.onclick=()=>seekPlaybackPosition(row.positionMs);
    if(manual) {
      button.oncontextmenu=event=>{
        event.preventDefault();if(button.disabled||playbackSeekPreview)return;
        const current=devices.find(item=>item.ID===selected);
        if(current?.ID!==device.ID||current?.Playback?.session!==playback.session)return;
        bookmarkMenuTarget={device:device.ID,session:playback.session,generation:current.Playback.generation,position:row.positionMs};
        const menu=$('playback-bookmark-menu');menu.hidden=false;
        const bounds=button.getBoundingClientRect();
        menu.style.left=`${Math.max(0,Math.min(event.clientX||bounds.left,innerWidth-menu.offsetWidth-4))}px`;
        menu.style.top=`${Math.max(0,Math.min(event.clientY||bounds.bottom,innerHeight-menu.offsetHeight-4))}px`;
        $('playback-bookmark-delete').focus();
      };
      button.onkeydown=event=>{if(event.key==='Delete'){event.preventDefault();void changePlaybackBookmark('bookmark-delete',row.positionMs);}};
    }
    return button;
  }));
  playbackMarkersEnabled=null;
}
function renderPlaybackBoundaryControls(position,enabled) {
  const add=$('playback-bookmark-add'),addDisabled=!enabled||bookmarkPending||!!playbackSeekPreview;
  if(add.disabled!==addDisabled)add.disabled=addDisabled;
  for(const button of document.querySelectorAll('[data-cm-direction]')) {
    const target=rockbarPlaybackBoundaries.adjacent(playbackBoundaries,position,Number(button.dataset.cmDirection));
    const disabled=!enabled||!target;
    if(button.disabled!==disabled)button.disabled=disabled;
    const text=Number(button.dataset.cmDirection)<0?'前のしおり・境界候補へ':'次のしおり・境界候補へ';
    if(button.getAttribute('aria-label')!==text)button.setAttribute('aria-label',text);
    const title=target?`${text} ${formatPlaybackTime(target.positionMs)}`:text;
    if(button.title!==title)button.title=title;
  }
  if(playbackMarkersEnabled!==enabled) {
    playbackMarkersEnabled=enabled;
    for(const button of $('playback-cm-markers').children)button.disabled=!enabled;
  }
}
function renderPlaybackTime() {
  const device = devices.find(item => item.ID === selected);
  if(bookmarkMenuTarget && (!connected || device?.ID!==bookmarkMenuTarget.device || device?.Playback?.session!==bookmarkMenuTarget.session || device?.Playback?.generation!==bookmarkMenuTarget.generation))closeBookmarkMenu();
  if (playbackSeekPreview && (playbackSeekPreview.device !== device?.ID || playbackSeekPreview.session !== device?.Playback?.session || !connected || playbackSeekPreview.committed && (playbackSeekPreview.generation !== device.Playback.generation || performance.now() >= playbackSeekPreview.expires))) playbackSeekPreview = null;
  if (device?.Kind !== 'playback') return;
  const clock = playbackClocks.get(device.ID);
  const position = playbackSeekPreview?.position ?? clock?.now() ?? device.Playback.positionMs;
  const timeText=`${formatPlaybackTime(position)} / ${formatPlaybackTime(device.Playback.durationMs)}`;
  if($('playback-position').textContent!==timeText)$('playback-position').textContent=timeText;
  const seek = $('playback-seek');
  const duration = Math.max(0, device.Playback.durationMs || 0);
  if(seek.max!==String(duration))seek.max=String(duration);
  seek.value = String(position);
  seek.disabled = !connected || !duration || !['playing', 'paused'].includes(device.Playback.phase);
  renderPlaybackBoundaryControls(position,!seek.disabled);
  seek.style.setProperty('--seek-progress', `${duration ? Math.max(0, Math.min(100, position / duration * 100)) : 0}%`);
  if(seek.getAttribute('aria-valuetext')!==timeText)seek.setAttribute('aria-valuetext',timeText);
  renderProgress();
}
const progressSegments = Array.from({ length: 20 }, () => {
  const segment = document.createElement('span');
  segment.setAttribute('aria-hidden', 'true');
  $('program-progress-bar').append(segment);
  return segment;
});
function renderProgress() {
  const device = devices.find(item => item.ID === selected);
  const milliseconds = serverClock.now();
  let view = connected && [2, 3].includes(device?.State) && milliseconds !== null
    ? programGuide.progress(device.CurrentProgram, milliseconds / 1000) : null;
  const isPlayback = device?.Kind === 'playback';
  if (isPlayback && device.Playback.durationMs > 0) {
    const ratio = Math.max(0, Math.min(1, (playbackClocks.get(device.ID)?.now() ?? device.Playback.positionMs) / device.Playback.durationMs));
    view = {ratio, percent: Math.floor(ratio * 100), segments: Math.floor(ratio * progressSegments.length), text: `${Math.floor(ratio * 100)}%`};
  }
  $('program-progress-bar').setAttribute('aria-label', isPlayback ? 'ファイル再生の進行状況' : '番組の進行状況');
  $('program-progress').hidden = !view;
  if (!view) return;
  $('program-progress-bar').setAttribute('aria-valuenow', String(view.percent));
  $('program-progress-bar').setAttribute('aria-valuetext', view.text);
  if($('program-progress-text').textContent!==view.text)$('program-progress-text').textContent=view.text;
  progressSegments.forEach((segment, index) => {
    segment.classList.toggle('filled', index < view.segments);
    const fill=String(Math.max(0, Math.min(1, view.ratio * progressSegments.length - index)));
    if(segment.style.getPropertyValue('--fill')!==fill)segment.style.setProperty('--fill',fill);
  });
}
let schedules = null, scheduleError = '';
let recordingStatus = null;
function renderSchedule() {
  const milliseconds = serverClock.now();
  const message = !connected ? 'サーバーに接続してください' : scheduleError ||
    (schedules === null ? '予約を取得中…' : milliseconds === null ? 'サーバー時刻を取得中…' : '');
  const now = milliseconds === null ? null : Math.floor(milliseconds / 1000);
  const view = message ? { label: message, time: '', title: '' } :
    recordingSchedules.display(recordingSchedules.select(schedules, selected, now), now);
  if($('schedule-label').textContent!==view.label)$('schedule-label').textContent=view.label;
  if($('schedule-time').textContent!==view.time)$('schedule-time').textContent=view.time;
  if($('schedule-title').textContent!==view.title)$('schedule-title').textContent=view.title;
  $('schedule-title').title = view.title;
}
const displayWaves = new Map();
const receivedServices = new Map();
function followReceivedServices() {
  for (const device of devices) {
    const station = stations.find(item => item.Sid === device.CurrentService);
    const wave = programGuide.wave(device, stations);
    const signature = JSON.stringify([device.CurrentService, wave, station?.Parent || 0]);
    if (receivedServices.get(device.ID) === signature) continue;
    receivedServices.set(device.ID, signature);
    if ([1, 2].includes(wave) && station?.Parent) {
      displayWaves.set(device.ID, wave);
      multiWaves.set(JSON.stringify([device.ID, wave]), true);
    }
  }
}
const popupHoverDelay = 60000;
let contentPopupTimer, contentProgramKey = '', contentText = '';
let contentTarget = null, currentContentProgram = null;
let contentPinned = false;
const dismissedContentClicks = new WeakSet();
const contentPopup = $('program-content');
function closeProgramContent() {
  clearTimeout(contentPopupTimer);
  contentPinned = false;
  $('program-content-hint').hidden = true;
  contentPopup.hidePopover();
}
function contentTargetActive() {
  return !timekeeperMode && contentTarget?.isConnected && contentTarget.matches(':hover, :focus');
}
function positionProgramContent() {
  const row = contentTarget?.classList.contains('service-program') ? contentTarget : null;
  contentPopup.style.removeProperty('top');
  contentPopup.style.removeProperty('max-height');
  if (!row || !contentPopup.matches(':popover-open')) return;
  // Leave the hovered row exposed so double-clicking can still tune it.
  const rect = row.getBoundingClientRect();
  const gap = 24;
  const above = rect.top - gap - 8;
  const below = window.innerHeight - rect.bottom - gap - 8;
  const placeAbove = above >= below;
  contentPopup.style.maxHeight = Math.max(1, placeAbove ? above : below) + 'px';
  contentPopup.style.top = (placeAbove ? Math.max(8, rect.top - gap - contentPopup.offsetHeight) : rect.bottom + gap) + 'px';
}
function showProgramContent(event) {
  if (timekeeperMode || contentPinned) return;
  clearTimeout(contentPopupTimer);
  const target = event.currentTarget;
  contentPopupTimer = setTimeout(() => {
    if (timekeeperMode || contentPinned || !target.isConnected || !target.matches(':hover, :focus')) return;
    contentTarget = target;
    const row = target.classList.contains('service-program') ? target : null;
    renderProgramContent(row ? row.program : currentContentProgram, row);
    if (contentProgramKey && contentTargetActive()) {
      contentPopup.showPopover();
      positionProgramContent();
    }
  }, popupHoverDelay);
}
function hideProgramContent() {
  if (contentPinned) return;
  clearTimeout(contentPopupTimer);
  contentPopupTimer = setTimeout(() => {
    if (!contentPinned && !contentTargetActive() && !contentPopup.matches(':hover, :focus-within')) closeProgramContent();
  }, 150);
}
function pinProgramContent(target) {
  clearTimeout(contentPopupTimer);
  contentTarget = target;
  const row = target.classList.contains('service-program') ? target : null;
  renderProgramContent(row ? row.program : currentContentProgram, row);
  if (!contentProgramKey) return;
  contentPinned = true;
  $('program-content-hint').hidden = false;
  contentPopup.showPopover();
  positionProgramContent();
}
function handleProgramContentClick(event) {
  // An outside click dismisses the existing popup, without reopening another.
  if (!dismissedContentClicks.has(event)) pinProgramContent(event.currentTarget);
}
for (const id of ['title', 'subtitle']) {
  $(id).addEventListener('mouseenter', showProgramContent);
  $(id).addEventListener('focus', showProgramContent);
  $(id).addEventListener('mouseleave', hideProgramContent);
  $(id).addEventListener('blur', hideProgramContent);
  $(id).addEventListener('click', handleProgramContentClick);
}
contentPopup.addEventListener('mouseenter', () => clearTimeout(contentPopupTimer));
contentPopup.addEventListener('mouseleave', hideProgramContent);
contentPopup.addEventListener('focusin', () => clearTimeout(contentPopupTimer));
contentPopup.addEventListener('focusout', hideProgramContent);
window.addEventListener('resize', positionProgramContent);
document.addEventListener('scroll', event => {
  if (contentTarget?.classList.contains('service-program') && !contentPopup.contains(event.target)) positionProgramContent();
}, true);
document.addEventListener('keydown', event => {
  if (event.key === 'Escape') { closeProgramContent(); closeCaptionHistory(); }
});
document.addEventListener('click', event => {
  if (contentPopup.matches(':popover-open') && !contentPopup.contains(event.target)) {
    dismissedContentClicks.add(event);
    closeProgramContent();
  }
  if (captionPopup.matches(':popover-open') && !captionPopup.contains(event.target)) {
    dismissedContentClicks.add(event);
    closeCaptionHistory();
  }
}, true);
function renderProgramContent(program, row = null) {
  const key = connected && (program || row) ? `${selected}:${program?.Sid ?? row?.dataset.sid}:${program?.Eid}:${program?.Start}` : '';
  if (key !== contentProgramKey) {
    // Background refreshes must not cancel a pending hover.
    if (contentPopup.matches(':popover-open') || contentPinned) closeProgramContent();
    contentPopup.scrollTop = 0;
  }
  contentProgramKey = key;
  contentPopup.setAttribute('aria-label', row ? '放送中番組の詳細' : '現在番組の詳細');
  $('program-content-context').textContent = key && row ? row.popupContext : '';
  $('program-content-context').hidden = !key || !row;
  const title = key ? program?.Title || '番組情報なし' : '';
  const search = $('program-content-search');
  if (search.textContent !== title) search.textContent = title;
  const searchTitle = key ? program?.Title || '' : '';
  for (const [id, mode] of [['program-content-search', 14], ['program-content-ai', 50]]) {
    const link = $(id);
    if (searchTitle) link.setAttribute('href', `https://google.com/search?q=${encodeURIComponent(searchTitle)}&udm=${mode}`);
    else link.removeAttribute('href');
  }
  $('program-content-ai').hidden = !searchTitle;
  $('program-content-subtitle').textContent = key ? program?.Subtitle || '' : '';
  $('program-content-subtitle').hidden = !$('program-content-subtitle').textContent;
  const text = key ? program?.Content || '番組の詳細情報はありません。' : '';
  // Keep focused links intact during the periodic device refresh.
  if (text === contentText) return;
  contentText = text;
  const container = $('program-content-text');
  container.replaceChildren();
  const urls = /https?:\/\/[-a-zA-Z0-9@:%_+.~#?&//=()]+/g;
  let offset = 0;
  for (const match of text.matchAll(urls)) {
    container.append(document.createTextNode(text.slice(offset, match.index)));
    const link = document.createElement('a');
    link.textContent = match[0];
    link.href = match[0];
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    container.append(link);
    offset = match.index + match[0].length;
  }
  container.append(document.createTextNode(text.slice(offset)));
}
let watchedDevice = null, captionData = { key: '', items: [] }, captionSignature = '';
const captionScroller = new CaptionScroller($('captions'), $('caption-lines'));
let captionPopupTimer;
let captionPinned = false;
const captionPopup = $('caption-history');
function closeCaptionHistory() {
  clearTimeout(captionPopupTimer);
  captionPinned = false;
  captionPopup.hidePopover();
}
function toggleCaptionHistory(event) {
  if (dismissedContentClicks.has(event)) return;
  if (captionPopup.matches(':popover-open')) { closeCaptionHistory(); return; }
  clearTimeout(captionPopupTimer);
  captionPinned = true;
  captionPopup.showPopover();
}
for (const section of document.querySelectorAll('.foldable')) {
  const storageKey = `rockbar:${section.id}:open`;
  try { section.open = localStorage.getItem(storageKey) !== 'false'; } catch {}
  section.addEventListener('toggle', () => {
    try { localStorage.setItem(storageKey, String(section.open)); } catch {}
    if ((section.id === 'captions-section' || section.id === 'current-section') && !section.open) {
      closeCaptionHistory();
    }
    if (section.id === 'current-section' && !section.open) {
      closeProgramContent();
    }
  });
}
function showCaptionHistory() {
  if (captionPinned) return;
  clearTimeout(captionPopupTimer);
  if (captionPopup.matches(':popover-open')) return;
  captionPopupTimer = setTimeout(() => {
    if ($('captions').matches(':hover, :focus')) captionPopup.showPopover();
  }, popupHoverDelay);
}
function hideCaptionHistory() {
  if (captionPinned) return;
  clearTimeout(captionPopupTimer);
  captionPopupTimer = setTimeout(() => {
    if (!$('captions').matches(':hover, :focus') && !captionPopup.matches(':hover, :focus-within')) closeCaptionHistory();
  }, 150);
}
$('captions').addEventListener('mouseenter', showCaptionHistory);
$('captions').addEventListener('focus', showCaptionHistory);
$('captions').addEventListener('mouseleave', hideCaptionHistory);
$('captions').addEventListener('blur', hideCaptionHistory);
$('captions').addEventListener('click', toggleCaptionHistory);
captionPopup.addEventListener('mouseenter', () => clearTimeout(captionPopupTimer));
captionPopup.addEventListener('mouseleave', hideCaptionHistory);
captionPopup.addEventListener('focusout', hideCaptionHistory);
$('captions').addEventListener('keydown', event => {
  if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); toggleCaptionHistory(event); }
});
function displayWave(device) {
  const actual = programGuide.wave(device, stations);
  const waves = programGuide.switchWaves(device, stations);
  if (!waves.length) return actual;
  const saved = displayWaves.get(device.ID);
  return waves.includes(saved) ? saved : waves.includes(actual) ? actual : waves[0];
}
function renderCaptions(device) {
  const key = rockbarCaptions.programKey(device);
  const matches = captionData.key === key && captionData.deviceID === device?.ID;
  const items = matches ? captionData.items : [];
  const viewKey = selected + ':' + key;
  const signature = JSON.stringify([viewKey, items, captionData.error]);
  if (signature === captionSignature) return;
  captionSignature = signature;
  captionScroller.update(viewKey, items.slice(-2));
  const history = $('caption-history-lines');
  history.replaceChildren(...items.slice(-50).map(rockbarCaptions.captionRow));
  $('caption-history-status').textContent = items.length ? '' : '字幕を待っています';
  history.scrollTop = history.scrollHeight;
}
const states = ['OFF', '起動中', 'ON AIR', 'REC', '終了中'];
function isWatchOnly(device) {
  if (device?.State !== 2) return false;
  if (recordingStatus?.current?.[device.ID]?.watchonly === true) return true;
  const milliseconds = serverClock.now();
  if (milliseconds === null) return false;
  const now = milliseconds / 1000;
  const active = (schedules || []).filter(item => now >= item.starttime - (Number(item.readytime) || 0));
  const schedule = recordingSchedules.select(active, device.ID, now);
  return schedule?.watchonly === true;
}
const stateText = device => device?.Kind==='playback' ? ({idle:'待受中',indexing:'解析中',preparing:'準備中',playing:'再生中',paused:'一時停止'}[device.Playback?.phase]||'Playback') : isWatchOnly(device) ? 'LIVE' : states[device?.State] || '?';
const time = value => Number.isFinite(value) && value > 0
  ? new Date(value * 1000).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' }) : '未定';
const range = program => program ? `${time(program.Start)} — ${time(program.End)}` : '--:-- — --:--';
const programTiming = program => program ? [program.Start, program.End] : null;
const currentProgramScroller = new ProgramScroller($('current-program'));
const serviceScroller = new ProgramScroller($('service'));
const nextProgramScroller = new ProgramScroller($('next'));
function resetProgramDisplay() {
  closeProgramContent();
  closeCaptionHistory();
  contentTarget = null;
  currentContentProgram = null;
  contentProgramKey = '';
  for (const scroller of [currentProgramScroller, serviceScroller, nextProgramScroller]) scroller.reset();
  for (const row of $('programs').children) row.scrollers?.forEach(scroller => scroller.reset());
}
function notice(message) {
  playbackStatus.show(message);
  scheduleTickerMessage();
  render();
  $('notice').textContent = message;
  $('notice').classList.add('visible');
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => $('notice').classList.remove('visible'), 4500);
}
let nextProgramTransition=Infinity;
function renderPrograms() {
  const device = devices.find(item => item.ID === selected);
  const wave = displayWave(device);
  const waves = programGuide.switchWaves(device, stations);
  const switchable = waves.length > 0;
  const nextWave = waves.find(item => item !== wave);
  $('wave').textContent = (programGuide.names[wave] || '') + ' 放送中の番組' + (switchable ? ' ⇄' : '');
  $('wave').disabled = !switchable;
  $('wave').title = switchable ? programGuide.names[nextWave] + 'の放送中一覧へ切替（選局はしません）' : '放送中の番組';
  const multiAvailable = [1, 2].includes(wave);
  const multiKey = JSON.stringify([selected, wave]);
  const multi = multiAvailable && !!multiWaves.get(multiKey);
  $('multi').hidden = !multiAvailable;
  $('multi').setAttribute('aria-pressed', String(multi));
  $('multi').title = multi ? '通常表示へ切替（親局のみ）' : 'マルチ表示へ切替（サブチャンネルを含む）';
  const now = serverClock.now() === null ? NaN : serverClock.now() / 1000;
  nextProgramTransition=Infinity;
  for(const entries of Object.values(programs))for(const entry of entries||[])for(const boundary of [entry.Start,entry.End])if(boundary>now)nextProgramTransition=Math.min(nextProgramTransition,boundary*1000);
  const rows = programGuide.services(device, stations, wave, multi).map(station => ({
    station, program: programGuide.current(programs[station.Sid], now),
  }));
  const disabled = !connected || pending || device?.State !== 2;
  const message = !connected ? connectionMessage || 'サーバーに接続してください' : guideError ||
    (!wave ? '受信サービスが判明すると同じ放送波の番組を表示します' : !rows.length ? '表示対象のサービスがありません' : '');
  const signature = JSON.stringify([selected, device?.CurrentService, device?.State, disabled, message, rows]);
  if (signature === guideSignature) return;
  guideSignature = signature;
  const container = $('programs');
  const scroll = container.scrollTop;
  const focus = document.activeElement?.dataset.sid;
  const existing = new Map([...container.children].filter(row => row.dataset.sid).map(row => [row.dataset.sid, row]));
  const children = rows.map(({ station, program }) => {
    let row = existing.get(String(station.Sid));
    if (!row) {
      row = document.createElement('button');
      row.className = 'service-program';
      row.dataset.sid = station.Sid;
      row.setAttribute('aria-controls', 'program-content');
      row.addEventListener('mouseenter', showProgramContent);
      row.addEventListener('focus', showProgramContent);
      row.addEventListener('mouseleave', hideProgramContent);
      row.addEventListener('blur', hideProgramContent);
      row.scrollers = Array.from({ length: 3 }, () => {
        const span = document.createElement('span');
        row.append(span);
        return new ProgramScroller(span);
      });
    }
    existing.delete(String(station.Sid));
    row.disabled = disabled;
    row.setAttribute('aria-current', String(station.Sid === device.CurrentService));
    const texts = [station.Name, program ? `${time(program.Start)}～${time(program.End)}` : '--:--～--:--', program?.Title || '番組情報なし'];
    row.scrollers.forEach((scroller, index) => scroller.update(index ? programTiming(program) : null, [[scroller.content, texts[index]]]));
    row.program = program;
    row.popupContext = [texts.slice(0, 2).join('　'),
      device.State === 3 ? '（録画中は選局できません）' : '（クリックで詳細固定／ダブルクリックまたはフォーカス中にEnterで選局）',
    ].filter(Boolean).join('\n');
    row.setAttribute('aria-label', [texts.join('　'), program?.Subtitle, row.popupContext.split('\n')[1]].filter(Boolean).join('\n'));
    if (contentTarget === row) { renderProgramContent(program, row); positionProgramContent(); }
    row.onclick = handleProgramContentClick;
    row.ondblclick = () => { closeProgramContent(); tune(station.Sid); };
    row.onkeydown = event => { if (event.key === 'Enter' && !event.repeat) { event.preventDefault(); closeProgramContent(); tune(station.Sid); } };
    return row;
  });
  for (const row of existing.values()) {
    if (contentTarget === row) {
      closeProgramContent();
      contentTarget = null;
    }
    row.scrollers.forEach(scroller => scroller.destroy());
  }
  if (message) { const empty = document.createElement('p'); empty.className = 'guide-message'; empty.textContent = message; children.unshift(empty); }
  // Leave retained rows attached so focus and running animations survive polling.
  for (const child of [...container.children]) if (!children.includes(child)) child.remove();
  children.forEach((child, index) => {
    if (container.children[index] !== child) container.insertBefore(child, container.children[index] || null);
  });
  if (focus) [...container.children].find(row => row.dataset.sid === focus)?.focus({ preventScroll: true });
  container.scrollTop = scroll;
}
async function tune(sid) {
  if (pending) return;
  pending = true; render();
  try {
    const result = await api.tune(selected, sid);
    notice(result.error || '選局を送信しました。受信サービスの更新を待っています。');
  } catch (error) { notice(error.message); }
  clearTimeout(pendingTimer);
  pendingTimer = setTimeout(() => { pending = false; render(); }, 1800);
}
function clearTimekeeperTransfer(button) {
  const transfer = button.tkTransfer;
  if (!transfer) return;
  button.tkTransfer = null;
  transfer.animation.cancel();
  transfer.ghost.remove();
  transfer.target.style.opacity = '';
  transfer.source.style.opacity = '';
}
function transferTimekeeperTitle(button) {
  clearTimekeeperTransfer(button);
  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
  const source = button.children[8], target = button.children[3];
  const from = source.getBoundingClientRect(), to = target.getBoundingClientRect();
  if (!from.width || !to.width) return;
  const ghost = source.cloneNode(true);
  const style = getComputedStyle(source);
  ghost.className = 'tk-transfer';
  ghost.setAttribute('aria-hidden', 'true');
  Object.assign(ghost.style, { position: 'fixed', left: `${from.left}px`, top: `${from.top}px`,
    width: `${from.width}px`, height: `${from.height}px`, color: style.color, font: style.font,
    padding: style.padding, boxSizing: 'border-box', overflow: 'hidden', whiteSpace: 'nowrap',
    textOverflow: 'ellipsis', pointerEvents: 'none', zIndex: '10' });
  document.body.append(ghost);
  target.style.opacity = '0';
  source.style.opacity = '0';
  const animation = ghost.animate([
    { transform: 'translate(0,0)', width: `${from.width}px` },
    { transform: `translate(${to.left - from.left}px,${to.top - from.top}px)`, width: `${to.width}px` },
  ], { duration: 650, easing: 'cubic-bezier(.22,.61,.36,1)', fill: 'forwards' });
  button.tkTransfer = { animation, ghost, target, source };
  animation.onfinish = () => clearTimekeeperTransfer(button);
}
function renderDevices() {
  if (!devices.some(device => device.ID === selected)) selected = devices[0]?.ID || '';
  $('playback-device').disabled=!connected||api.platform==='browser'||playbackCreating;
  const existing = new Map([...$('devices').children].map(button => [button.dataset.device, button]));
  const normalDevices = devices.filter(device => device.Kind !== 'playback');
  const orderedDevices = timekeeperMode ? normalDevices : normalDevices.concat(devices.filter(device => device.Kind === 'playback'));
  const buttons = orderedDevices.map((device, index) => {
    let button = existing.get(device.ID);
    if (!button) {
      button = document.createElement('button'); button.dataset.device = device.ID;
      const led = document.createElement('span'); led.className = 'led';
      const name = document.createElement('span'); name.className = 'name';
      button.append(led, name, document.createElement('small'));
      for (let index = 0; index < 8; index++) {
        const cell = document.createElement('span');
        cell.className = 'tk-cell'; cell.hidden = true;
        button.append(cell);
      }
      button.onclick = () => { const reselected=selected===device.ID; selected = device.ID; followedService = ''; localStorage.setItem('device', selected); render(); if(reselected && device.Kind==='playback')scrollToCurrentPlaybackFile(); };
      button.ondblclick = () => performAction(device.ID, 'power');
      bindRecordBoot(button, () => device.ID);
    }
    button.setAttribute('aria-pressed', String(device.ID === selected));
    button.classList.toggle('device-playback', device.Kind === 'playback');
    button.classList.toggle('device-group-start', device.Kind === 'playback' && normalDevices.length > 0 && index === normalDevices.length);
    button.children[0].className = 'led' + (device.State === 3 ? ' rec' : device.State === 2 ? (isWatchOnly(device) ? ' live' : ' on') : '');
    if(button.children[1].textContent!==device.ID)button.children[1].textContent=device.ID;
    const label=stateText(device);
    if(button.children[2].textContent!==label)button.children[2].textContent=label;
    button.title = device.ID + ' · ' + stateText(device) + (device.Kind === 'playback' ? ' · ファイル再生' : ' · ダブルクリックで電源ON/OFF、OFF時に長押しで録画デバイスを起動（録画中は不可）');
    if (timekeeperMode) {
      const now = serverClock.now();
      const view = connected && now !== null && (schedules !== null || recordingStatus !== null)
        ? recordingSchedules.timekeeper(device, schedules, stations, Math.floor(now / 1000), recordingStatus?.current, recordingStatus?.recording) : null;
      const message = !connected ? 'サーバーに接続してください' : scheduleError || (schedules === null ? '予約を取得中…' : 'サーバー時刻を取得中…');
      const nextMessage = scheduleError || (schedules === null ? '予約を取得中…' : '');
      if (view?.waiting && view.currentID !== button.tkView?.currentID && view.currentID === button.tkView?.nextID) {
        transferTimekeeperTitle(button);
      } else if (button.tkView?.currentID !== view?.currentID) clearTimekeeperTransfer(button);
      button.tkView = view;
      [...button.children].slice(3).forEach((cell, index) => {
        cell.hidden = false;
        const text = view && index >= 5 && nextMessage ? (index === 5 ? nextMessage : '') : view ? view.values[index] : [0, 5].includes(index) ? message : '';
        if (cell.textContent !== text) cell.textContent = text;
        cell.title = text;
      });
      button.style.setProperty('--recorded', `${(view?.ratio || 0) * 100}%`);
    } else {
      clearTimekeeperTransfer(button);
      button.tkView = null;
    }
    return button;
  });
  if (buttons.length !== $('devices').children.length || buttons.some((button, i) => $('devices').children[i] !== button)) $('devices').replaceChildren(...buttons);
}
function render() {
  renderDevices();
  const device = devices.find(item => item.ID === selected);
  const playbackMode=device?.Kind==='playback';
  document.body.classList.toggle('playback-mode', playbackMode);
  $('next-section').hidden=playbackMode;
  $('schedule-section').hidden=playbackMode;
  $('programs').hidden=playbackMode;
  $('programs').previousElementSibling.hidden=playbackMode;
  $('playback-panel').hidden=!playbackMode;
  $('playback-files-section').hidden=!playbackMode;
  renderControls(device);
  if (timekeeperMode) {
    followedService = '';
    const normalDevices=devices.filter(device=>device.Kind!=='playback');
    renderTicker(connected ? `TIMEKEEPER · ${normalDevices.filter(device => device.State === 3).length} 台録画中 / ${normalDevices.length} 台` : '未接続 · 接続先とtvrockgoの起動状態を確認してください');
    return;
  }
  if (displayedDevice !== selected) {
    resetProgramDisplay();
    displayedDevice = selected;
    playbackScrollSession=playbackMode?device.Playback.session:null;
    if(playbackMode)scrollToCurrentPlaybackFile();
  }
  if (watchedDevice !== selected) {
    watchedDevice = selected;
    api.watchDevice(selected).catch(error => notice(error.message));
  }
  const live = device && [2, 3].includes(device.State);
  if (playbackMode) {
    const playback=device.Playback;
    const labels={idle:'受信待受中',indexing:'再生時間を解析中',preparing:'受信準備中',playing:'再生中',paused:'一時停止中'};
    serviceScroller.update(null,[[serviceScroller.content,'ファイル再生']]);
    const playbackLabel=playback.phase==='playing' && playback.waitingForData?'録画の追記待ち':labels[playback.phase]||playback.phase;
    $('state').textContent=playbackLabel;
    $('state').classList.remove('recording','watch-only');
    currentProgramScroller.update(null,[[$('program-time'),''],[$('title'),playback.title||'再生するファイルを選択してください'],[$('subtitle'),playbackLabel]]);
    renderPlaybackBoundaries(device);
    renderPlaybackTime();
    renderPlaybackFiles();
    if(!playbackFilesLoading && playbackFilesDirty)void refreshPlaybackFiles();
    const playing = playback.phase === 'playing';
    const toggle = $('playback-pause');
    toggle.dataset.playback = playing ? 'pause' : 'play';
    toggle.querySelector('.icon-pause').toggleAttribute('hidden', !playing);
    toggle.querySelector('.icon-play').toggleAttribute('hidden', playing);
    toggle.title = playing ? '一時停止' : '再生';
    toggle.setAttribute('aria-label', toggle.title);
    $('playback-stop').hidden = false;
    for(const button of $('playback-panel').querySelectorAll('[data-playback]')) {
      const active=['playing','paused'].includes(playback.phase);
      button.disabled=!connected || (button.dataset.playback==='stop'?playback.phase==='idle':button===toggle?(!active && !(playback.phase==='idle' && selectedPlaybackFile())):!active);
    }
    renderTicker(`${device.ID} / ${playbackLabel} / ${playback.title}`);
    currentContentProgram=null;renderProgramContent(null);renderCaptions(device);
    return;
  }
  const current = live ? device.CurrentProgram : null;
  const next = live ? device.NextProgram : null;
  const service = stations.find(item => item.Sid === device?.CurrentService)?.Name || (live ? `SID ${device.CurrentService}` : 'サービス未選択');
  serviceScroller.update(null, [[serviceScroller.content, service]]);
  renderProgress();
  $('state').textContent = device ? stateText(device) : 'OFF';
  $('state').classList.toggle('recording', device?.State === 3);
  $('state').classList.toggle('watch-only', isWatchOnly(device));
  renderCaptions(device);
  currentProgramScroller.update(programTiming(current), [
    [$('program-time'), range(current)],
    [$('title'), current?.Title || (device ? (device.State === 0 ? '電源OFF' : '番組情報を待っています') : 'デバイスを選択してください')],
    [$('subtitle'), current?.Subtitle || (device?.State === 0 ? '電源クリックで視聴、長押しで録画デバイスを起動します。' : '')],
  ]);
  currentContentProgram = current;
  if (!contentTarget?.classList.contains('service-program')) renderProgramContent(current);
  nextProgramScroller.update(programTiming(next), [[nextProgramScroller.content, next ? `${range(next)}  ${next.Title}` : '番組情報はありません']]);
  const info = live ? device.DeviceInfo || {} : {};
  const number = key => info[key] == null ? '—' : Number(info[key]).toLocaleString('ja-JP');
  const text = !connected ? '未接続 · 接続先とtvrockgoの起動状態を確認してください' : device
    ? `${device.ID}  /  ${service}${live ? ` (${device.CurrentService & 0xffff})` : ''}  /  ${stateText(device)}${remoteViewText(device)}    ·    Bitrate ${number('bitrate')}    ·    Signal ${number('signal')}    ·    Error ${number('error')}    ·    Scramble ${number('scramble')}`
    : 'デバイスが登録されていません';
  renderTicker(text);
  renderPrograms();
  // Follow reception changes once, leaving manual scrolling alone during polling.
  const serviceKey = connected && device?.CurrentService ? JSON.stringify([selected, device.CurrentService]) : '';
  if (!serviceKey) followedService = '';
  else if (serviceKey !== followedService && scrollToCurrentService()) followedService = serviceKey;
  renderSchedule();
}
function renderControls(device) {
  if(device?.Kind==='playback') {
    $('power').disabled=!connected;
    $('power').title='再生デバイスを終了';$('power').setAttribute('aria-label',$('power').title);
    $('stop').disabled=!connected||device.Playback.phase==='idle';
      $('stop').title='再生を停止';$('stop').setAttribute('aria-label',$('stop').title);
    for(const id of ['record','capture'])$(id).disabled=true;
    return;
  }
  const disabled = !connected || pending || !device;
  $('power').disabled = disabled || remoteViewerState.pending.includes(device?.RegistryCode) || !(remoteConfigured(device) && remoteView(device)) && !(remoteConfigured(device)?[0,2,3]:[0,2]).includes(device?.State);
  $('record').disabled = disabled || device?.State !== 2 || !device?.CurrentProgram || !device?.CurrentService;
  $('stop').disabled = disabled || !(device?.State === 3 || isWatchOnly(device));
  $('stop').title = isWatchOnly(device) ? '視聴予約を停止' : '録画を停止';
  $('stop').setAttribute('aria-label', $('stop').title);
  $('capture').disabled = disabled || ![2, 3].includes(device?.State);
  $('power').title = remoteConfigured(device) ? (remoteView(device)?'このPCの視聴を終了（録画は継続）':'このPCで視聴を開始') : device?.State === 3 ? '録画中は電源OFFにできません' : device?.State === 0 ? 'クリックで視聴デバイス、長押しで録画デバイスを起動' : '電源OFF';
  $('power').setAttribute('aria-label', $('power').title);
}
api.onUpdate(({ type, value }) => {
  if(type==='schedule-update')playbackFilesDirty=true;
  if (type === 'playback' || type === 'playback-status') { playbackStatus.receive(value); scheduleTickerMessage(); }
  if (type === 'viewer-state') remoteViewerState=value;
  if (type === 'power-manager') powerManagerSetting = value;
  if (type === 'schedules') { schedules = value; scheduleError = ''; }
  if (type === 'schedule-error') scheduleError = value;
  if (type === 'recording-status') recordingStatus = value;
  if (type === 'guide-visible') $('guide').setAttribute('aria-pressed', String(value));
  if (type === 'devices') {
    devices = value;
    const fresh=devices.filter(device=>device.Kind==='playback'&&!knownPlaybackSessions.has(device.Playback?.session));
    if(fresh.length) {selected=fresh.at(-1).ID;localStorage.setItem('device',selected);playbackCreating=false;clearTimeout(playbackCreateTimer);}
    knownPlaybackSessions.clear();for(const device of devices)if(device.Playback)knownPlaybackSessions.add(device.Playback.session);
    const active = new Set();
    for (const device of devices) if (device.Kind === 'playback' && device.Playback) {
      active.add(device.ID);
      if (!playbackClocks.has(device.ID)) playbackClocks.set(device.ID, new rockbarTime.PlaybackClock());
      playbackClocks.get(device.ID).sync(device.Playback);
    }
    for (const id of playbackClocks.keys()) if (!active.has(id)) playbackClocks.delete(id);
  }
  if (type === 'captions') captionData = value;
  if (type === 'server-time') { if (value === null) serverClock.reset(); else serverClock.sync(value); clock(); }
  if (type === 'stations') stations = Object.values(value || {});
  if (type === 'devices' || type === 'stations') followReceivedServices();
  if (type === 'programs') { programs = value; guideError = ''; }
  if (type === 'guide-error') guideError = value;
  if (type === 'status') {
    connected = value.connected;
    connectionMessage = value.message;
    $('connection').classList.toggle('connected', connected);
    $('connection').title = value.message;
    if (!connected) { playbackFilesRequest++;playbackFilesLoading=false;playbackFiles=[];playbackFilesDirty=true;knownPlaybackSessions.clear();playbackFileSelection.clear();stations = []; programs = {}; guideError = ''; pending = false; clearTimeout(pendingTimer); receivedServices.clear(); for (const clock of playbackClocks.values()) clock.freeze(); playbackStatus.reset(); clearTimeout(tickerMessageTimer); }
  }
  if (type === 'notice') notice(value);
  renderSleepTimer();
  render();
});
async function performAction(id, action) {
  if (pending) return;
  const device = devices.find(item => item.ID === id);
  if(device?.Kind==='playback') {
    try {const result=await api.playback(id,action==='power'?'close':'stop',0);if(result?.error)notice(result.error);}catch(error){notice(error.message);}return;
  }
  if (['power', 'power-record'].includes(action) && (!connected || !(action==='power' && remoteConfigured(device) && remoteView(device)) && !(action === 'power-record' ? [0] : remoteConfigured(device)?[0,2,3]:[0,2]).includes(device?.State))) {
    notice(device?.State === 3 ? '録画中は電源OFFにできません' : '現在の状態では電源操作できません');
    return;
  }
  pending = true; render();
  try {
    const result = await api.action(id, action);
    notice(result.error || '操作を送信しました。デバイスの状態更新を待っています。');
  } catch (error) { notice(error.message); }
  clearTimeout(pendingTimer);
  pendingTimer = setTimeout(() => { pending = false; render(); }, 1800);
}
for (const action of ['power', 'record', 'stop', 'capture']) $(action).onclick = () => performAction(selected, action);
for(const button of $('playback-panel').querySelectorAll('[data-playback]'))button.onclick=async()=>{
  const device=devices.find(item=>item.ID===selected);
  if(button.dataset.playback==='play' && device?.Playback?.phase==='idle') {await loadPlaybackFile(selectedPlaybackFile());return;}
  try {const result=await api.playback(selected,button.dataset.playback,Number(button.dataset.seconds)||0);if(result.error)notice(result.error);}catch(error){notice(error.message);}
};
for(const button of document.querySelectorAll('[data-cm-direction]'))button.onclick=()=>{
  const device=devices.find(item=>item.ID===selected);
  if(device?.Kind!=='playback'||button.disabled)return;
  const position=playbackSeekPreview?.position??playbackClocks.get(device.ID)?.now()??device.Playback.positionMs;
  const target=rockbarPlaybackBoundaries.adjacent(playbackBoundaries,position,Number(button.dataset.cmDirection));
  if(target)void seekPlaybackPosition(target.positionMs);
};
$('playback-bookmark-add').onclick=()=>changePlaybackBookmark('bookmark-add');
document.addEventListener('keydown', handlePlaybackShortcut);
function handlePlaybackShortcut(event) {
  if(event.defaultPrevented || event.repeat || event.isComposing || event.altKey || event.metaKey)return;
  if(!connected || timekeeperMode || document.querySelector('dialog[open]'))return;
  if(event.target.closest('input, textarea, select, [contenteditable]:not([contenteditable="false"]), [role="menu"]'))return;
  const device=devices.find(item=>item.ID===selected);
  if(device?.Kind!=='playback' || !['playing','paused'].includes(device.Playback?.phase))return;
  let button;
  if(event.key===' ' && !event.ctrlKey && !event.shiftKey)button=$('playback-pause');
  else if(['ArrowLeft','ArrowRight'].includes(event.key) && !(event.ctrlKey && event.shiftKey)) {
    const direction=event.key==='ArrowRight'?1:-1;
    button=event.shiftKey?$(direction>0?'playback-cm-next':'playback-cm-previous'):
      document.querySelector(`[data-playback="seek"][data-seconds="${direction*(event.ctrlKey?30:15)}"]`);
  }
  if(!button)return;
  event.preventDefault();
  button.click();
}
async function changePlaybackBookmark(action,position) {
  const device=devices.find(item=>item.ID===selected);
  if(bookmarkPending||device?.Kind!=='playback'||$('playback-seek').disabled||playbackSeekPreview)return;
  const at=position??playbackClocks.get(device.ID)?.now()??device.Playback.positionMs;
  bookmarkPending=true;closeBookmarkMenu();renderPlaybackTime();
  try {
    const result=await api.playback(selected,action,{positionMs:Math.round(at),generation:device.Playback.generation});
    if(result?.error)notice(result.error);
  } catch(error){notice(error.message);}
  finally {bookmarkPending=false;renderPlaybackTime();}
}
$('playback-bookmark-delete').onclick=()=>{
  const target=bookmarkMenuTarget,device=devices.find(item=>item.ID===selected);
  if(target&&target.device===selected&&target.session===device?.Playback?.session&&target.generation===device.Playback.generation)void changePlaybackBookmark('bookmark-delete',target.position);
  else closeBookmarkMenu();
};
document.addEventListener('pointerdown',event=>{if(!$('playback-bookmark-menu').contains(event.target))closeBookmarkMenu();});
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&bookmarkMenuTarget){closeBookmarkMenu();$('playback-bookmark-add').focus();}});
window.addEventListener('resize',closeBookmarkMenu);
window.addEventListener('blur',closeBookmarkMenu);
async function seekPlaybackPosition(position) {
  const device=devices.find(item=>item.ID===selected);
  if(device?.Kind!=='playback'||$('playback-seek').disabled)return;
  const target={device:device.ID,session:device.Playback.session,position,committed:true,generation:device.Playback.generation,expires:performance.now()+5000};
  playbackSeekPreview=target;
  renderPlaybackTime();
  try {
    const result=await api.playback(target.device,'seek-to',Math.floor(target.position/1000));
    if(result?.error){if(playbackSeekPreview===target)playbackSeekPreview=null;notice(result.error);}
  } catch(error){if(playbackSeekPreview===target)playbackSeekPreview=null;notice(error.message);}
  renderPlaybackTime();
}
$('playback-seek').addEventListener('input', () => {
  const device = devices.find(item => item.ID === selected);
  if (device?.Kind !== 'playback' || $('playback-seek').disabled) return;
  playbackSeekPreview = {device: device.ID, session: device.Playback.session, position: Number($('playback-seek').value)};
  renderPlaybackTime();
});
$('playback-seek').addEventListener('change', async () => {
  const target = playbackSeekPreview;
  if (!target || target.device !== selected || target.session !== devices.find(item => item.ID === selected)?.Playback?.session) return;
  await seekPlaybackPosition(target.position);
});
$('playback-seek').addEventListener('pointercancel', () => { playbackSeekPreview = null; renderPlaybackTime(); });
function bindRecordBoot(button, deviceID) {
  bindLongPress(button, () => connected && !pending && devices.find(item => item.ID === deviceID())?.State === 0,
    () => {
      selected = deviceID();
      localStorage.setItem('device', selected);
      performAction(selected, 'power-record');
    });
}
bindRecordBoot($('power'), () => selected);
$('timekeeper-toggle').onclick = async () => {
  timekeeperMode = !timekeeperMode;
  document.body.classList.toggle('timekeeper', timekeeperMode);
  document.querySelector('.program').inert = timekeeperMode;
  document.querySelector('.program').setAttribute('aria-hidden', String(timekeeperMode));
  $('timekeeper-toggle').setAttribute('aria-pressed', String(timekeeperMode));
  $('timekeeper-toggle').setAttribute('aria-label', timekeeperMode ? 'デバイス・選局モードへ切替' : 'タイムキーパーモードへ切替');
  $('timekeeper-toggle').title = $('timekeeper-toggle').getAttribute('aria-label');
  document.querySelectorAll('.tk-heading, .tk-cell').forEach(cell => { cell.hidden = !timekeeperMode; });
  resetProgramDisplay();
  captionScroller.setActive(!timekeeperMode);
  for (const scroller of [currentProgramScroller, serviceScroller, nextProgramScroller]) scroller.setActive(!timekeeperMode);
  for (const row of $('programs').children) row.scrollers?.forEach(scroller => scroller.setActive(!timekeeperMode));
  if (!timekeeperMode) document.querySelector('.device-panel').scrollLeft = 0;
  render();
  try { await api.setMode(timekeeperMode ? 'timekeeper' : 'device'); }
  catch (error) { notice(error.message); }
};
$('guide').onclick = async () => {
  try {
    const result = await api.guide();
    if (result.error) notice(result.error);
  } catch (error) { notice(error.message); }
};
if (api.platform === 'browser') {
  $('guide').removeAttribute('aria-pressed');
  $('guide').title = '番組表を新しいタブで開く';
  $('guide').setAttribute('aria-label', $('guide').title);
}
function scrollToCurrentService() {
  const device = devices.find(item => item.ID === selected);
  if(device?.Kind==='playback')return scrollToCurrentPlaybackFile();
  if (!device?.CurrentService || timekeeperMode) return false;
  const container = $('programs');
  const currentRow = () => [...container.children].find(row => row.dataset.sid === String(device.CurrentService));
  if (!currentRow()) {
    const wave = programGuide.wave(device, stations);
    displayWaves.set(selected, wave);
    const station = stations.find(item => item.Sid === device.CurrentService);
    if ([1, 2].includes(wave) && station?.Parent) multiWaves.set(JSON.stringify([selected, wave]), true);
    renderPrograms();
  }
  const row = currentRow();
  if (!row || !container.clientHeight) return false;
  if (container.scrollHeight <= container.clientHeight) return true;
  const bounds = container.getBoundingClientRect();
  const target = row.getBoundingClientRect();
  container.scrollTo({
    top: container.scrollTop + target.top - bounds.top - container.clientTop + (target.height - container.clientHeight) / 2,
    behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
  });
  return true;
}
$('service').onclick = scrollToCurrentService;
$('service').addEventListener('keydown', event => {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    scrollToCurrentService();
  }
});
$('wave').onclick = () => {
  const device = devices.find(item => item.ID === selected);
  const waves = programGuide.switchWaves(device, stations);
  if (!waves.length) return;
  displayWaves.set(selected, waves.find(item => item !== displayWave(device))); renderPrograms();
};
$('multi').onclick = () => {
  const device = devices.find(item => item.ID === selected);
  const wave = displayWave(device);
  if (![1, 2].includes(wave)) return;
  const key = JSON.stringify([selected, wave]);
  multiWaves.set(key, !multiWaves.get(key));
  renderPrograms();
};
function configTab(tab) {
  for (const [name, panel] of [['connection', 'config-form'], ['viewer', 'viewer-config'], ['options', 'options-config']]) {
    $(panel).hidden = tab !== name;
    const button=$('config-' + name + '-tab');
    button.setAttribute('aria-pressed', String(tab === name));
    button.setAttribute('aria-selected', String(tab === name));
    button.tabIndex=tab===name?0:-1;
  }
}
$('settings').onclick = () => { configTab('connection'); $('config').showModal(); };
$('config').addEventListener('close',()=>{ if (api.closeViewerSettings) void api.closeViewerSettings(); });
$('config-connection-tab').onclick=()=>configTab('connection');
$('config-options-tab').onclick=()=>configTab('options');
document.querySelector('.config-tabs').addEventListener('keydown',event=>{
  if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;
  const tabs=[...document.querySelectorAll('.config-tabs [role="tab"]')].filter(tab=>!tab.hidden);
  const current=tabs.indexOf(document.activeElement);if(current<0)return;
  const next=event.key==='Home'?0:event.key==='End'?tabs.length-1:(current+(event.key==='ArrowRight'?1:-1)+tabs.length)%tabs.length;
  event.preventDefault();tabs[next].focus();tabs[next].click();
});
$('color-theme').value = window.rockbarTheme.mode;
$('color-theme').onchange = () => {
  try {
    window.rockbarTheme.setMode($('color-theme').value);
    $('theme-error').textContent = '';
  } catch {
    $('color-theme').value = window.rockbarTheme.mode;
    $('theme-error').textContent = 'カラーテーマを保存できませんでした。';
  }
};
$('options-close').onclick=()=>$('config').close();
let savedSnapToEdges=true;
$('snap-to-edges').disabled=!api.windowOptions;
if(!api.windowOptions)$('snap-to-edges-help').textContent='ウィンドウの吸着はデスクトップ版Rockbarで使用できます。';
$('snap-to-edges').onchange=async()=>{
  const input=$('snap-to-edges');input.disabled=true;
  try {
    const result=await api.windowOptions({snapToEdges:input.checked});
    if(result.error)throw new Error(result.error);
    savedSnapToEdges=result.snapToEdges;input.checked=savedSnapToEdges;
    $('window-options-error').textContent='';
  } catch(error) {input.checked=savedSnapToEdges;$('window-options-error').textContent=error.message;}
  finally {input.disabled=false;}
};
function loadViewerDevice() {
  const entry=viewerSettings.find(d=>d.registry===$('viewer-device').value);
  const s=entry?.setting || {enabled:false,executable:'',options:'/d BonDriver_SRT.dll /nd',port:3230,transport:'srt'};
  const playback=entry?.kind==='playback';
  $('viewer-transport').replaceChildren(...(playback?['srt']:['srt','udp']).map(value=>{const option=document.createElement('option');option.value=value;option.textContent=value==='srt'?'SRT（BonDriver_SRT）':'UDP（従来方式）';return option;}));
  $('viewer-transport').disabled=playback;
  $('viewer-playback-transport').hidden=!playback;
  $('viewer-enabled').checked=s.enabled; $('viewer-executable').value=s.executable;
  $('viewer-options').value=s.options; $('viewer-port').value=s.port;
  $('viewer-transport').value=playback?'srt':s.transport || 'udp';
  $('viewer-srt-base').value=s.srt?.basePort ?? 3230;
  $('viewer-srt-latency').value=s.srt?.latencyMs ?? 120;
  $('viewer-srt-secret').value=s.srt?.passphrase ?? '';
  if(playback && s.transport!=='srt')$('viewer-transport').onchange();
  $('viewer-srt').hidden=$('viewer-transport').value!=='srt';
  $('viewer-current-service').checked=s.stream?.currentServiceOnly ?? true;
  $('viewer-caption').checked=s.stream?.caption ?? true;
  $('viewer-data').checked=s.stream?.dataBroadcast ?? true;
  $('viewer-oneseg').checked=s.stream?.oneSeg ?? false;
  updateOneSegSetting();
}
$('config-viewer-tab').onclick=async()=>{
  const result=await api.viewerSettings($('server').value.trim());
  $('viewer-error').textContent=result.error || ''; $('viewer-error').classList.remove('success'); viewerSettings=result.devices || [];
  $('viewer-device').replaceChildren(...viewerSettings.map(d=>{ const option=document.createElement('option'); option.value=d.registry; option.textContent=d.id; return option; }));
  $('viewer-url').textContent=$('server').value.trim(); loadViewerDevice(); configTab('viewer');
};
function updateOneSegSetting() {
  const oneSeg=$('viewer-oneseg').checked;
  $('viewer-current-service').disabled=oneSeg;
  if(oneSeg)$('viewer-current-service').checked=true;
}
$('viewer-oneseg').onchange=updateOneSegSetting;
$('viewer-transport').onchange=()=>{
  const srt=$('viewer-transport').value==='srt';
  $('viewer-srt').hidden=!srt;
  if (srt) {
    $('viewer-options').value=$('viewer-options').value.replace(/BonDriver_UDP\.dll/ig,'BonDriver_SRT.dll');
    if (Number($('viewer-port').value)>=1234 && Number($('viewer-port').value)<1244) $('viewer-port').value=Number($('viewer-srt-base').value)+Number($('viewer-port').value)-1234;
  } else {
    $('viewer-options').value=$('viewer-options').value.replace(/BonDriver_SRT\.dll/ig,'BonDriver_UDP.dll');
    const index=Number($('viewer-port').value)-Number($('viewer-srt-base').value);
    if (index>=0 && index<10) $('viewer-port').value=1234+index;
  }
};
$('viewer-device').onchange=loadViewerDevice;
$('viewer-cancel').onclick=()=>$('config').close();
$('viewer-browse').onclick=async()=>{ const r=await api.pickViewer(); if (r.path) $('viewer-executable').value=r.path; $('viewer-error').textContent=r.error || ''; };
$('viewer-save').onclick=async()=>{
  const value={enabled:$('viewer-enabled').checked,executable:$('viewer-executable').value,options:$('viewer-options').value,port:Number($('viewer-port').value),stream:{currentServiceOnly:$('viewer-current-service').checked,caption:$('viewer-caption').checked,dataBroadcast:$('viewer-data').checked,oneSeg:$('viewer-oneseg').checked}};
  value.transport=$('viewer-transport').value;
  if(value.transport==='srt')value.srt={basePort:Number($('viewer-srt-base').value),latencyMs:Number($('viewer-srt-latency').value),passphrase:$('viewer-srt-secret').value};
  const result=await api.saveViewerSettings($('server').value.trim(),$('viewer-device').value,value);
  $('viewer-error').textContent=result.error || '保存しました';
  $('viewer-error').classList.toggle('success',!result.error);
  if (result.setting) { const entry=viewerSettings.find(d=>d.registry===$('viewer-device').value); if (entry) entry.setting=result.setting; }
};
$('sleep-timer').onclick = async () => {
  if (!connected || sleepTimerPending || !powerManagerSetting?.powermanager) return;
  sleepTimerPending = true;
  renderSleepTimer();
  try {
    const result = await api.sleepTimerPause(!powerManagerSetting.sleeptimerpause);
    if (result.error) notice(result.error);
  } catch (error) { notice(error.message); }
  finally { sleepTimerPending = false; renderSleepTimer(); }
};
$('cancel').onclick = () => $('config').close();
$('login').onclick = async () => {
  const result = await api.login($('server').value.trim());
  $('config-error').textContent = result.error || '';
  if (!result.error) $('config').close();
};
$('minimize').onclick = () => api.window('minimize');
$('close').onclick = () => api.window('close');
$('pin').onclick = async () => $('pin').setAttribute('aria-pressed', String(await api.window('pin')));
let layoutPending=false;
const normalFoldsKey='rockbar:normal-window-folds';
function applyWindowLayout(compact,initial=false) {
  if(compact && (!initial || localStorage.getItem(normalFoldsKey)===null)) {
    const folds=Object.fromEntries([...document.querySelectorAll('.foldable')].map(section=>[section.id,section.open]));
    localStorage.setItem(normalFoldsKey,JSON.stringify(folds));
  }
  if(compact) $('current-section').open=false;
  else if(!initial) {
    try {const folds=JSON.parse(localStorage.getItem(normalFoldsKey)||'{}');for(const section of document.querySelectorAll('.foldable'))if(typeof folds[section.id]==='boolean')section.open=folds[section.id];}catch {}
  }
  const text=compact?'通常表示に戻す':'スモール表示に切り替え';
  $('window-layout').setAttribute('aria-pressed',String(compact));
  $('window-layout').setAttribute('aria-label',text);$('window-layout').title=text;
  $('window-layout').querySelector('img').src=`assets/window-${compact?'expand':'compact'}.svg`;
}
$('window-layout').onclick=async()=>{
  if(layoutPending)return;
  layoutPending=true;$('window-layout').disabled=true;
  try {const result=await api.windowLayout();if(result.error)notice(result.error);else applyWindowLayout(result.compact);}
  catch(error){notice(error.message);}
  finally {layoutPending=false;$('window-layout').disabled=false;}
};
$('config-form').onsubmit = async event => {
  event.preventDefault();
  const result = await api.connect($('server').value.trim());
  $('config-error').textContent = result.error || '';
  if (!result.error) $('config').close();
};
function clock() {
  const now = serverClock.now();
  $('clock').textContent = now === null ? '--:--:--' : new Date(now).toLocaleTimeString('ja-JP', { hour12: false });
  $('clock').title = now === null ? 'サーバー時刻を取得中' : 'サーバー時刻';
}
function tick() {
  clock();
  if(timekeeperMode){renderDevices();return;}
  const device=devices.find(item=>item.ID===selected);
  if(device?.Kind==='playback')return;
  renderProgress();renderSchedule();
  if(serverClock.now()>=nextProgramTransition)renderPrograms();
}
clock(); setInterval(tick, 1000);
setInterval(() => { if (!timekeeperMode && !document.hidden) renderPlaybackTime(); }, 100);
api.settings().then(settings => { savedSnapToEdges=settings.snapToEdges!==false;$('snap-to-edges').checked=savedSnapToEdges; if(api.windowLayout){applyWindowLayout(!!settings.compact,true);$('window-layout').hidden=false;} nativeViewer=!!settings.nativeViewer; $('config-viewer-tab').hidden=!nativeViewer; $('server').value = settings.server; return api.connect(settings.server); }).catch(error => notice(error.message));
render();
