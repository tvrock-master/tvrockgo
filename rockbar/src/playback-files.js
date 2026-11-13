(function(root){
  const key = row => `${row.slot || 0}:${row.id}`;
  const titles = new Intl.Collator('ja',{numeric:true,sensitivity:'base'});
  const dates = new Intl.DateTimeFormat('ja-JP',{year:'numeric',month:'numeric',day:'numeric',hour:'numeric',minute:'numeric',second:'numeric'});
  function reconcile(previous,next) {
    const existing=new Map(previous.map(row=>[key(row),row]));
    const rows=next.map(row=>{
      const old=existing.get(key(row));
      return old && JSON.stringify(old)===JSON.stringify(row)?old:row;
    });
    return rows.length===previous.length && rows.every((row,index)=>row===previous[index])?previous:rows;
  }
  function sorted(rows, order) {
    return rows.slice().sort((a,b)=>{
      let result=order.startsWith('title')?titles.compare(String(a.title||''),String(b.title||'')):Number(a.starttime)-Number(b.starttime);
      if(order.endsWith('desc'))result=-result;
      return result || Number(b.starttime)-Number(a.starttime) || key(a).localeCompare(key(b));
    });
  }
  const date = seconds => seconds && Number.isFinite(Number(seconds)) ? dates.format(new Date(seconds*1000)) : '不明';
  function details(row) {
    return [row.title||'タイトルなし',`${date(row.starttime)} ～ ${date(row.endtime)}`,row.recording?'録画中':'録画済み',`デバイス: ${row.deviceid||''} / SID: ${row.sid||0}`,`録画エラー: ${row.errors||0}`,row.filename||''].join('\n');
  }
  if(typeof module!=='undefined')module.exports={key,sorted,details,reconcile};else root.rockbarPlaybackFiles={key,sorted,details,reconcile};
})(globalThis);
