'use client';
import {useEffect,useState} from 'react';
import type {VoyageIssue} from './voyage';
export function useVoyageIssues(){
 const [issues,setIssues]=useState<VoyageIssue[]>([]),[loading,setLoading]=useState(true),[error,setError]=useState(''),[attempt,setAttempt]=useState(0);
 useEffect(()=>{const controller=new AbortController();setLoading(true);setError('');
  void fetch('/api/voyage',{credentials:'include',cache:'no-store',signal:controller.signal}).then(async response=>{const result=await response.json();if(!response.ok||result.code!==200||!Array.isArray(result.data))throw Error('舰礼加载失败，请重试');if(!controller.signal.aborted)setIssues(result.data);}).catch(e=>{if(!controller.signal.aborted)setError(e.message||'舰礼加载失败，请重试');}).finally(()=>{if(!controller.signal.aborted)setLoading(false);});
  return()=>controller.abort();
 },[attempt]);
 return {issues,loading,error,retry:()=>setAttempt(v=>v+1)};
}
