'use client';

import {useEffect,useRef,useState} from 'react';
import {Button} from '@/components/ui/button';
import {voyageRequest} from '@/lib/voyage';
import {voyageField} from '@/lib/voyage-recipients';
import {confirmAction} from '@/lib/site-dialog';

interface QueueSummary {id:number;admin_id:number;owner:string;scope:string;scope_id:number;title:string;sender_uid:string;status:string;note:string;interval:number;counts:Record<string,number>;total:number;can_manage:boolean}
interface MonitorItem {id:number;uid:string;name:string;status:string;note:string;sent_at:string|null;updated_at:string}
interface MonitorData {queues:QueueSummary[];selected:QueueSummary|null;items:MonitorItem[];current:MonitorItem[];page:number;page_size:number;total:number;status:string;next_at:string}
const labels:Record<string,string>={prepared:'等待测试',testing:'正在测试',awaiting_confirmation:'等待确认',running:'正在发送',paused:'已暂停',completed:'本轮已处理完',cancelled:'已取消',pending:'待发送',sending:'正在发送',sent:'已发送',failed:'发送失败',unknown:'结果待确认',skipped:'未发送'};
const filters=[['all','全部'],['sent','已发送'],['pending','待发送'],['failed','发送失败'],['unknown','结果待确认']] as const;

export function MessageQueueMonitor({scope,scopeID,onChanged}:{scope:'voyage'|'gift';scopeID:number;onChanged?:()=>void}){
 const [data,setData]=useState<MonitorData|null>(null),[error,setError]=useState(''),[selected,setSelected]=useState(0),[status,setStatus]=useState('all'),[page,setPage]=useState(1),[revision,setRevision]=useState(0),[expanded,setExpanded]=useState(false);
	 const [retrying,setRetrying]=useState(0),[feedback,setFeedback]=useState('');const retryBusy=useRef(false);
 useEffect(()=>{setSelected(0);setStatus('all');setPage(1);setData(null)},[scope,scopeID]);
 useEffect(()=>{
  let stopped=false,inFlight=false;
  const read=async()=>{
   if(inFlight)return;inFlight=true;
   try{
    const params=new URLSearchParams({scope,scope_id:String(scopeID),status,page:String(page)});if(selected)params.set('queue_id',String(selected));
    const value=await voyageRequest<MonitorData>(`/api/bilibili/messages/monitor?${params}`);
    if(!stopped){setData(value);setError('')}
   }catch(e){if(!stopped)setError((e as Error).message)}finally{inFlight=false}
  };
  void read();const timer=window.setInterval(()=>void read(),3000);
  return()=>{stopped=true;window.clearInterval(timer)};
 },[scope,scopeID,selected,status,page,revision]);
 const q=data?.selected,count=(key:string)=>q?.counts[key]||0,processed=q?q.total-count('pending')-count('sending'):0;
	 const nextAt=data?.next_at?new Date(data.next_at):null,cooling=!!nextAt&&nextAt.getTime()>Date.now();
 const choose=(value:string)=>{setStatus(value);setPage(1);setExpanded(true)};
 const pages=Math.max(1,Math.ceil((data?.total||0)/20));
	 const retry=async(item:MonitorItem)=>{
	  if(!q?.can_manage||retryBusy.current)return;retryBusy.current=true;
	  try{
	   if(item.status==='unknown'&&!(await confirmAction(`请先在 B 站确认给 UID ${item.uid} 的这条私信没有发出，再点击确认重试。`,{title:'确认这条私信未发出？',confirmLabel:'已确认未发出，重试'})))return;
	   setRetrying(item.id);setFeedback('');
	   const result=await voyageRequest<{status:string;note:string}>(`/api/bilibili/messages/${q.id}/retry`,'POST',{item_id:item.id,confirmed:item.status==='unknown'});
	   setFeedback(result.status==='sent'?`UID ${item.uid} 重试已发送。`:`UID ${item.uid} 重试仍未成功，请查看记录，并到 B 站私信中核对具体情况。`);setRevision(v=>v+1);onChanged?.();
	  }catch(e){setFeedback((e as Error).message)}finally{setRetrying(0);retryBusy.current=false}
	 };
 return <section aria-label="发送队列与进度" className="space-y-4 rounded-lg border border-border bg-secondary/20 p-4 sm:p-5">
  <div className="flex flex-wrap items-center justify-between gap-3"><h4 className="font-semibold">发送队列与进度</h4><Button size="sm" variant="ghost" onClick={()=>setRevision(v=>v+1)}>刷新进度</Button></div>
  <p className="text-xs leading-5 text-muted-foreground">本站管理员均可查看；发送操作仍使用队列所属账号。每 3 秒自动更新。</p>
  {error&&<p role="alert" className="break-words text-sm text-destructive">{error} <Button size="sm" variant="ghost" onClick={()=>{setSelected(0);setPage(1);setRevision(v=>v+1)}}>重新读取</Button></p>}
  {!data&&!error&&<p role="status" className="text-sm text-muted-foreground">正在读取发送进度…</p>}
  {data&&!q&&<p className="text-sm text-muted-foreground">这个{scope==='voyage'?'收集批次':'电子礼物'}还没有发送队列。</p>}
  {q&&data&&<>
   <label className="block space-y-2 text-sm"><span>查看队列</span><select aria-label="选择管理员发送队列" className={voyageField} value={q.id} onChange={e=>{setSelected(Number(e.target.value));setPage(1)}}>{data.queues.map(queue=><option key={queue.id} value={queue.id}>{queue.owner||`管理员 ${queue.admin_id}`} · {queue.title} · 队列 #{queue.id} · {labels[queue.status]||queue.status} · {queue.total} 人</option>)}</select></label>
   <div className="flex flex-wrap items-center justify-between gap-2 text-sm"><p><strong>{q.owner||`管理员 ${q.admin_id}`}</strong> 的队列 · {labels[q.status]||q.status}</p><p className="text-muted-foreground">发送账号 UID {q.sender_uid} · 间隔 {q.interval} 秒</p></div>
	   <div className="space-y-2"><progress className="h-3 w-full accent-primary" aria-label="本轮队列处理进度" value={processed} max={Math.max(1,q.total)}/><p role="status" className="text-sm">已处理 <strong>{processed} / {q.total}</strong> 人，其中已发送 <strong>{count('sent')}</strong> 人（含手动标记）</p></div>
   <div className="flex flex-wrap gap-2" role="group" aria-label="按发送结果查看明细">{filters.map(([key,label])=><Button key={key} size="sm" variant={status===key&&expanded?'secondary':'outline'} className={['failed','unknown'].includes(key)&&count(key)>0?'border-destructive/50 font-semibold text-destructive':''} onClick={()=>choose(key)}>{label} {key==='all'?q.total:count(key)}</Button>)}</div>
	   {(count('failed')+count('unknown'))>0&&<div role="status" className="space-y-2 rounded-md border border-destructive/40 bg-destructive/5 p-3 text-sm"><p className="font-semibold text-destructive">有 {count('failed')+count('unknown')} 条私信待处理</p><p className="leading-6">失败记录不会自动重发。{q.can_manage?'可点击“发送失败”逐条重试；重试后仍失败，再到 B 站查看具体情况。结果不明的记录需先核对，避免重复发送。':'请由队列所属账号重试或核对。'}</p></div>}
	   {feedback&&<p role="status" className="break-words text-sm leading-6">{feedback}</p>}
   {q.status==='running'&&<p className="text-sm leading-6">{data.current.length?`正在发送：UID ${data.current[0].uid}`:cooling?`等待下一条，最早 ${nextAt!.toLocaleString('zh-CN',{hour12:false})} 继续。`:'正在准备下一条私信。'}</p>}
	   {q.status!=='running'&&cooling&&<p className="text-sm leading-6">发送间隔或冷却尚未结束，下一次重试最早在 {nextAt!.toLocaleString('zh-CN',{hour12:false})}。</p>}
   {q.note&&<p className="break-words text-sm leading-6">{q.note}</p>}
   {expanded&&<div className="space-y-3 border-t border-border pt-3">
    <div className="flex items-center justify-between gap-3"><h5 className="text-sm font-medium">{status==='all'?'全部发送结果':filters.find(([key])=>key===status)?.[1]||'待复核'} · {data.total} 人</h5><Button size="sm" variant="ghost" onClick={()=>setExpanded(false)}>收起明细</Button></div>
	    <ul className="divide-y divide-border">{data.items.map(item=><li key={item.id} className="space-y-2 py-3 text-sm"><div className="flex flex-wrap items-center justify-between gap-2"><span className="min-w-0 break-words">{item.name&&<strong>{item.name} · </strong>}UID {item.uid}</span><span className={['failed','unknown'].includes(item.status)?'font-semibold text-destructive':'text-muted-foreground'}>{labels[item.status]||item.status}</span></div>{item.note&&<p className="break-words leading-6 text-muted-foreground">{item.note}</p>}{['failed','unknown'].includes(item.status)&&<div className="flex flex-wrap items-center gap-3">{q.can_manage&&<Button size="sm" variant="outline" disabled={retrying!==0||cooling} onClick={()=>void retry(item)}>{retrying===item.id?'正在重试…':item.status==='unknown'?'核对后重试':'重试发送'}</Button>}{(item.status==='unknown'||item.note.startsWith('手动重试仍未成功'))&&<a href={`https://message.bilibili.com/#/whisper/mid${item.uid}`} target="_blank" rel="noreferrer" className="inline-block text-primary underline underline-offset-4">打开 B 站私信查看</a>}</div>}</li>)}</ul>
    {!data.items.length&&<p className="py-3 text-sm text-muted-foreground">暂无这类记录。</p>}
    {pages>1&&<div className="flex flex-wrap items-center justify-center gap-3 text-sm"><Button size="sm" variant="outline" disabled={data.page<=1} onClick={()=>setPage(data.page-1)}>上一页</Button><span>{data.page} / {pages} 页</span><Button size="sm" variant="outline" disabled={data.page>=pages} onClick={()=>setPage(data.page+1)}>下一页</Button></div>}
   </div>}
  </>}
 </section>;
}
