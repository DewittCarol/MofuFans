'use client';

import {useEffect, useRef, useState} from 'react';
import {QRCodeSVG} from 'qrcode.react';
import {Button} from '@/components/ui/button';
import {voyageRequest} from '@/lib/voyage';

interface ImportResult {text:string; records:number; people:number; from:string; through:string}
interface ImportJob {
 roster_id:number; month:string; status:'running'|'done'|'error'; error?:string;
 progress:{day:string; completed:number; total:number; records:number}; result?:ImportResult;
}
interface Connection {
 status:'disconnected'|'waiting'|'scanned'|'connected'|'expired';
 qr_url?:string; account?:{uid:string; name:string}; job?:ImportJob;
}
const endpoint='/api/voyage/bilibili';

export function VoyageBilibiliImport({rosterID, month, onReady, onBusy, onClose}:{
 rosterID:number; month:string; onReady:(text:string)=>Promise<void>;
 onBusy:(busy:boolean)=>void; onClose:()=>void;
}) {
 const [connection,setConnection]=useState<Connection>({status:'disconnected'});
 const [loading,setLoading]=useState(true), [acting,setActing]=useState(false);
 const [error,setError]=useState(''), [paused,setPaused]=useState(false);
 const active=useRef(true), operation=useRef(false), generation=useRef(0);
 const job=connection.job, fetching=job?.status==='running';
 const busy=loading||acting;
 const future=month>new Intl.DateTimeFormat('sv-SE',{timeZone:'Asia/Shanghai',year:'numeric',month:'2-digit'}).format(new Date());

 useEffect(()=>{
  active.current=true;
  void voyageRequest<Connection>(`${endpoint}/session`).then(data=>{if(active.current)setConnection(data)})
   .catch(e=>{if(active.current){setError((e as Error).message);setPaused(true)}})
   .finally(()=>{if(active.current)setLoading(false)});
  return()=>{active.current=false};
 },[]);
 useEffect(()=>{onBusy(busy||fetching);return()=>onBusy(false)},[busy,fetching,onBusy]);

 // Schedule the next poll only after this request finishes; no overlapping requests.
 useEffect(()=>{
  if(loading||acting||paused||(!fetching&&!['waiting','scanned'].includes(connection.status)))return;
  let cancelled=false;
  const version=generation.current;
  const timer=window.setTimeout(async()=>{
   try {
    const data=fetching
     ?await voyageRequest<Connection>(`${endpoint}/session`)
     :await voyageRequest<Connection>(`${endpoint}/session/poll`,'POST',{});
    if(!cancelled&&active.current&&generation.current===version)setConnection(data);
   }catch(e){if(!cancelled&&active.current&&generation.current===version){setError((e as Error).message);setPaused(true)}}
  },2500);
  return()=>{cancelled=true;window.clearTimeout(timer)};
 },[connection,loading,acting,paused,fetching]);

 const action=async(fn:()=>Promise<void>)=>{
  if(operation.current)return;
  operation.current=true;generation.current++;setActing(true);setError('');
  try{await fn();if(active.current)setPaused(false)}
  catch(e){if(active.current)setError((e as Error).message)}
  finally{operation.current=false;if(active.current)setActing(false)}
 };
 const update=(path:string,method='POST')=>action(async()=>{
  const data=await voyageRequest<Connection>(`${endpoint}${path}`,method,{});
  if(active.current)setConnection(data);
 });
 const resume=()=>action(async()=>{
  const data=await voyageRequest<Connection>(`${endpoint}/session`);
  if(active.current)setConnection(data);
 });

 return <section aria-label="从 B 站获取名单" className="mb-6 space-y-4 border-y border-border py-5">
  <div className="flex items-start justify-between gap-3">
   <div><h3 className="font-medium">从 B 站获取 {month} 大航海名单</h3>
    <p className="mt-2 max-w-prose text-sm leading-6 text-muted-foreground">用主播本人的 B 站账号扫码登录，获取这个月的上舰、续费流水，再预览确认导入。</p></div>
   <Button variant="ghost" size="sm" disabled={busy||fetching} onClick={onClose}>收起</Button>
  </div>
  {loading?<p role="status" className="text-sm text-muted-foreground">正在检查登录状态…</p>:<>
   {['disconnected','expired'].includes(connection.status)&&<div className="space-y-3">
    {connection.status==='expired'&&<p role="status" className="text-sm">二维码或登录已过期，请重新扫码。</p>}
    <Button disabled={busy} onClick={()=>void update('/session')}>{acting?'正在获取二维码…':'扫码登录 B 站'}</Button>
   </div>}
   {['waiting','scanned'].includes(connection.status)&&connection.qr_url&&<div className="flex flex-col gap-4 sm:flex-row sm:items-center">
    <QRCodeSVG value={connection.qr_url} size={192} marginSize={4} title="B 站登录二维码" className="shrink-0"/>
    <div className="space-y-3 text-sm"><p role="status">{connection.status==='scanned'?'已扫码，请在手机上确认登录。':'打开哔哩哔哩 App，扫描二维码。'}</p>
     <p className="text-muted-foreground">请确认登录的是要收集名单的主播账号。</p>
     <div className="flex flex-wrap gap-2"><Button variant="outline" disabled={busy} onClick={()=>void update('/session')}>刷新二维码</Button><Button variant="ghost" disabled={busy} onClick={()=>void update('/session','DELETE')}>取消登录</Button></div>
    </div>
   </div>}
   {connection.status==='connected'&&connection.account&&<div className="space-y-3">
    <div className="flex flex-wrap items-center gap-3 text-sm"><p>已登录 <strong>{connection.account.name||connection.account.uid}</strong> · UID {connection.account.uid}</p>
     <Button variant="ghost" size="sm" disabled={busy} onClick={()=>void update('/session','DELETE')}>{fetching?'停止并断开':'断开登录'}</Button></div>
    {!fetching&&<Button disabled={busy||future} onClick={()=>void update(`/rosters/${rosterID}/fetch`)}>{job?'重新获取':'获取'} {month} 名单</Button>}
    {future&&<p className="text-sm text-muted-foreground">这个月份还没开始，届时再来获取名单。</p>}
   </div>}
   {fetching&&<div className="space-y-3">
    <p role="status" className="text-sm">{job.roster_id!==rosterID?`正在获取 ${job.month} 的名单。`:null}{job.progress.day?`正在读取 ${job.progress.day}，已完成 ${job.progress.completed}/${job.progress.total} 天，找到 ${job.progress.records} 条记录。`:'正在连接 B 站，请稍候…'}</p>
    <progress className="h-2 w-full max-w-md accent-primary" aria-label="获取名单进度" value={job.progress.completed} max={job.progress.total||1}/>
    <div><Button variant="outline" disabled={busy} onClick={()=>void update('/job','DELETE')}>取消获取</Button></div>
   </div>}
   {job?.status==='error'&&<p role="alert" className="text-sm leading-6 text-destructive">{job.error} 名单尚未导入。</p>}
   {job?.status==='done'&&job.result&&<div className="space-y-3 text-sm">
    <p role="status">{job.result.from} 至 {job.result.through}：{job.result.records?`共 ${job.result.people} 人、${job.result.records} 条上舰记录。`:'没有查到上舰记录，请确认账号和月份是否正确。'}</p>
    {job.roster_id===rosterID?job.result.records>0&&<Button disabled={busy} onClick={()=>void action(()=>onReady(job.result!.text))}>预览导入 {job.result.records} 条记录</Button>:<p className="text-muted-foreground">这是 {job.month} 的结果，请切换到对应月份查看，或重新获取当前月份。</p>}
   </div>}
  </>}
  {error&&<div role="alert" className="space-y-2 text-sm text-destructive"><p>{error}</p>{paused&&<Button variant="outline" disabled={busy} onClick={()=>void resume()}>重新检查状态</Button>}</div>}
  <p className="max-w-prose text-xs leading-5 text-muted-foreground">按购买、续费发生的月份统计，不包含往月购买、在本月仍有效的记录。同一 UID 的不同档位分别保留。登录会加密保存在本站，与两处直接私信共用。关闭页面无需重新扫码；登录失效时再验证，断开后清除本站保存的凭据。</p>
 </section>;
}
