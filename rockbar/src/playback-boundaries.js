(function(root){
  function markers(playback) {
    const duration=Number(playback?.durationMs)||0;
    return [...(Array.isArray(playback?.manualBookmarks)?playback.manualBookmarks:[]),...(Array.isArray(playback?.programBoundaries)?playback.programBoundaries:[]),...(Array.isArray(playback?.cmBoundaries)?playback.cmBoundaries:[])]
      .filter(row=>Number.isFinite(row.positionMs)&&row.positionMs>=0&&row.positionMs<=duration)
      .slice().sort((a,b)=>a.positionMs-b.positionMs)
      .filter((row,index,rows)=>!index||row.positionMs!==rows[index-1].positionMs);
  }
  // Seeking uses whole seconds and a PCR index spaced about 500 ms apart.
  // Treat the surrounding 1.5 seconds as the current boundary to avoid jumping
  // repeatedly to the same candidate after a rounded seek.
  function adjacent(rows,position,direction) {
    return direction<0?rows.findLast(row=>row.positionMs<position-1500):rows.find(row=>row.positionMs>position+1500);
  }
  function status(playback,rows) {
    const manual=rows.filter(row=>row.kind==='manual').length;
    const programs=rows.filter(row=>row.kind==='program-change').length;
    if(manual||programs) {
      const parts=[];
      if(manual)parts.push(`しおり ${manual}件`);
      if(programs)parts.push(`番組切替 ${programs}件`);
      const automatic=rows.length-manual-programs;if(automatic)parts.push(`境界候補 ${automatic}件（未確定）`);
      return parts.join(' / ');
    }
    if(rows.length)return `境界候補 ${rows.length}件（未確定）`;
    if(playback?.recording || playback?.cmStatus==='recording')return '追いかけ再生では境界解析を行いません';
    if(playback?.phase==='idle')return '境界候補：ファイル未再生';
    if(playback?.phase==='indexing')return '境界候補を解析中…';
    if(playback?.cmStatus==='not-signaled')return '境界候補なし（対応信号なし）';
    if(playback?.cmStatus==='no-supported-splice-boundary')return '境界候補なし（対応する境界なし）';
    if(playback?.cmStatus)return '境界候補なし';
    return '境界候補の情報なし';
  }
  const api={markers,adjacent,status};
  if(typeof module==='object'&&module.exports)module.exports=api;
  root.rockbarPlaybackBoundaries=api;
})(typeof globalThis==='object'?globalThis:this);
