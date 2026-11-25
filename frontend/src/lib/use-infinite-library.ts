'use client';
import {useCallback, useEffect, useRef, useState} from 'react';
import {libraryRequest, type LibraryPage} from './library';

interface Feed<T> {
  path: string;
  items: T[];
  data?: LibraryPage<T>;
  loading: boolean;
  error: string;
  hasMore: boolean;
}

export function useInfiniteLibrary<T extends {id:number}>(path:string, size:number) {
  const [feed,setFeed]=useState<Feed<T>>({path,items:[],loading:true,error:'',hasMore:true});
  const [revision,setRevision]=useState(0);
  const loader=useRef<(()=>void)|null>(null);
  useEffect(()=>{
    const abort=new AbortController();
    let page=1, busy=false, hasMore=true;
    let items:T[]=[];
    setFeed({path,items,loading:true,error:'',hasMore:true});
    const load=async()=>{
      if(busy || !hasMore || abort.signal.aborted)return;
      busy=true;
      setFeed(current=>({...current,loading:true,error:''}));
      try {
        const data=await libraryRequest<LibraryPage<T>>(`${path}${path.includes('?')?'&':'?'}page=${page}&page_size=${size}`,'GET',undefined,abort.signal);
        if(abort.signal.aborted)return;
        if(!Array.isArray(data.items)||data.page!==page||!Number.isFinite(data.total))throw Error('列表加载失败，请重试');
        // Offset pages may overlap when new entries arrive. Keep existing entries
        // in place, and append each new ID only once.
        const seen=new Set(items.map(item=>item.id));
        const added=data.items.filter(item=>{if(seen.has(item.id))return false;seen.add(item.id);return true;});
        items=[...items,...added];
        hasMore=data.items.length>0 && page*size<data.total;
        page++;
        setFeed({path,items,data,loading:false,error:'',hasMore});
      } catch(error) {
        if(!abort.signal.aborted)setFeed(current=>({...current,loading:false,error:error instanceof Error?error.message:'加载失败，请重试'}));
      } finally {busy=false;}
    };
    loader.current=load;
    void load();
    return()=>{abort.abort();if(loader.current===load)loader.current=null;};
  },[path,size,revision]);
  const loadMore=useCallback(()=>{loader.current?.();},[]);
  const reload=useCallback(()=>setRevision(value=>value+1),[]);
  // Do not flash the previous category while its replacement effect starts.
  const current=feed.path===path?feed:{path,items:[] as T[],loading:true,error:'',hasMore:true,data:undefined};
  return {...current,loadMore,reload};
}
