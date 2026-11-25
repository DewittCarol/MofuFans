'use client';
import {useEffect,useRef} from 'react';
import {Button} from './ui/button';

export function LibraryLoadMore({loading,error,hasMore,onLoad}:{loading:boolean;error:string;hasMore:boolean;onLoad:()=>void}) {
  const anchor=useRef<HTMLDivElement>(null);
  useEffect(()=>{
    if(loading||error||!hasMore||!anchor.current||typeof IntersectionObserver==='undefined')return;
    const observer=new IntersectionObserver(entries=>{
      if(entries.some(entry=>entry.isIntersecting))onLoad();
    },{rootMargin:'480px 0px'});
    observer.observe(anchor.current);
    return()=>observer.disconnect();
  },[loading,error,hasMore,onLoad]);
  return <div ref={anchor} className="flex min-h-24 flex-col items-center justify-center gap-2 py-6 text-sm text-muted-foreground">
    {error?<><p role="alert">{error}</p><Button variant="outline" size="sm" onClick={onLoad}>重新加载</Button></>:loading?<p role="status">正在加载…</p>:hasMore?<Button variant="ghost" size="sm" onClick={onLoad}>加载更多</Button>:<p role="status" className="text-xs">已全部显示</p>}
  </div>;
}
