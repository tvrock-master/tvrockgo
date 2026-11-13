// Electron bounds and display work areas use device-independent pixels.
function resizedBounds(current, size, area, threshold=20) {
  const width=Math.min(area.width,Math.max(370,Math.round(size.width)));
  const height=Math.min(area.height,Math.max(150,Math.round(size.height)));
  function position(start,length,next,edge,extent) {
    const nearStart=Math.abs(start-edge),nearEnd=Math.abs(edge+extent-start-length);
    const anchored=nearEnd<=threshold && nearEnd<nearStart ? edge+extent-next : nearStart<=threshold ? edge : start;
    return Math.round(Math.max(edge,Math.min(anchored,edge+extent-next)));
  }
  return {x:position(current.x,current.width,width,area.x,area.width),y:position(current.y,current.height,height,area.y,area.height),width,height};
}
function snappedBounds(bounds,area,threshold=20) {
  function position(start,length,edge,extent) {
    const first=Math.abs(start-edge),last=Math.abs(start+length-edge-extent);
    return last<=threshold && last<first ? edge+extent-length : first<=threshold ? edge : start;
  }
  return {...bounds,x:position(bounds.x,bounds.width,area.x,area.width),y:position(bounds.y,bounds.height,area.y,area.height)};
}
function attachEdgeSnapping(win,screen,enabled) {
  let dragging=false,queued=null;
  // Never cancel/reposition inside will-move. Windows owns the native drag
  // rectangle until the move ends; changing it there can enlarge the window.
  const moving=()=>{dragging=true;};
  const moved=()=>{
    if(!dragging)return;
    dragging=false;
    if(queued!==null)clearImmediate(queued);
    queued=setImmediate(()=>{
      queued=null;
      if(dragging || win.isDestroyed() || !enabled() || win.isMaximized() || win.isFullScreen())return;
      const bounds=win.getBounds();
      const next=snappedBounds(bounds,screen.getDisplayMatching(bounds).workArea);
      if(next.x===bounds.x && next.y===bounds.y)return;
      win.setPosition(next.x,next.y);
    });
  };
  const detach=()=>{
    win.removeListener('will-move',moving);win.removeListener('moved',moved);
    win.removeListener('closed',detach);
    if(queued!==null)clearImmediate(queued);
    queued=null;dragging=false;
  };
  win.on('will-move',moving);
  win.on('moved',moved);win.once('closed',detach);
  return detach;
}
function createWindowLayout(win,screen,config) {
  const state=config.windowLayout ||= {compact:false};
  function remember(bounds=win.getNormalBounds()) {
    state[state.compact?'compactSize':'normalSize']={width:bounds.width,height:bounds.height};
  }
  function toggle() {
    const current=win.getNormalBounds();remember(current);
    const compact=!state.compact;
    const size=state[compact?'compactSize':'normalSize'] || (compact?{width:680,height:240}:{width:900,height:600});
    const target=resizedBounds(current,size,screen.getDisplayMatching(current).workArea);
    if(win.isMaximized())win.unmaximize();
    win.setBounds(target);state.compact=compact;
    return {compact};
  }
  return {toggle,remember,get compact(){return !!state.compact;}};
}
module.exports={resizedBounds,snappedBounds,attachEdgeSnapping,createWindowLayout};
