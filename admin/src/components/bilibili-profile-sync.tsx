'use client';
import {useEffect,useRef,useState} from 'react';
import {RefreshCw} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {bilibiliProfiles,syncBilibiliProfiles,validProfileUID,type ProfileSyncProgress} from '@/lib/bilibili-profiles';

export function BilibiliProfileSync({uids,disabled}:{uids:string[];disabled?:boolean}){
 const [progress,setProgress]=useState<ProfileSyncProgress|null>(null);
 const [stopping,setStopping]=useState(false),[message,setMessage]=useState('');
 const current=useRef<{stopped:boolean}|null>(null);
 useEffect(()=>()=>{if(current.current)current.current.stopped=true;current.current=null},[]);
 const sync=async()=>{
  if(current.current||disabled)return;
  const job={stopped:false};current.current=job;setMessage('');setStopping(false);
  const result=await syncBilibiliProfiles(uids,{
   refresh:uid=>bilibiliProfiles.refresh(uid),
   isStopped:()=>job.stopped,
   onProgress:value=>{if(current.current===job)setProgress(value)},
  });
  if(current.current!==job)return;
  current.current=null;setProgress(null);setStopping(false);
  const remaining=result.total-result.completed;
  const summary=`已同步 ${result.succeeded} 人${result.failed?`，${result.failed} 人未完整同步`:''}${remaining?`，${remaining} 人未同步`:''}。`;
  setMessage(`${result.stopped?'已停止。':result.paused?'同步已暂停。':''}${summary}${result.error?`${result.error}。`:''}自定义昵称已保留。`);
 };
 return <>
  <Button variant="outline" disabled={disabled||!!progress||!uids.some(validProfileUID)} onClick={()=>void sync()} title="从 B 站同步整张名单的头像与昵称；同 UID 只获取一次，自定义昵称保留。">
   <RefreshCw aria-hidden="true" className={`size-4 ${progress?'animate-spin motion-reduce:animate-none':''}`}/>
   {progress?`同步中 ${progress.completed} / ${progress.total}`:'同步头像与昵称'}
  </Button>
  {progress&&<Button variant="ghost" disabled={stopping} onClick={()=>{if(current.current)current.current.stopped=true;setStopping(true)}}>{stopping?'正在停止…':'停止同步'}</Button>}
  {(progress||message)&&<p role="status" aria-live="polite" className="basis-full text-sm leading-6 text-muted-foreground">
   {progress?`${stopping?'完成当前用户后停止。':'正在从 B 站同步整张名单。'}已处理 ${progress.completed} / ${progress.total} 人，自定义昵称保留。`:message}
  </p>}
 </>;
}
