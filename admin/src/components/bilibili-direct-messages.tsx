'use client';

import {useCallback,useEffect,useRef,useState} from 'react';
import {QRCodeSVG} from 'qrcode.react';
import {Button} from '@/components/ui/button';
import {voyageRequest} from '@/lib/voyage';
import {voyageField} from '@/lib/voyage-recipients';
import {confirmAction} from '@/lib/site-dialog';
import {MessageQueueMonitor} from '@/components/message-queue-monitor';

export interface DirectMessageTarget {uid:string;body?:string;version:number}
interface Connection {status:string;qr_url?:string;account?:{uid:string;name:string}}
interface MessageItem {id:number;uid:string;status:string;note:string;text:string;sent_at:string|null}
interface QueueView {
 queue:{id:number;scope:string;scope_id:number;sender_uid:string;test_uid:string;status:string;test_status:string;note:string;interval:number;test_at:string|null};
 items:MessageItem[];test_text:string;next_at:string;
}
const api='/api/bilibili/messages',login='/api/voyage/bilibili/session';
const intervalOptions=[5,10,15,30,60,120,300];
const labels:Record<string,string>={prepared:'等待测试',testing:'正在测试',awaiting_confirmation:'等待确认测试',running:'正在发送',paused:'已暂停',completed:'已完成',cancelled:'已取消',pending:'待发送',sending:'正在发送',sent:'已发送',failed:'发送失败',unknown:'需要核对',skipped:'未发送'};

export function BilibiliDirectMessages({scope,scopeID,version,configVersion=0,targets,single,disabledReason='',onFinished}:{
 scope:'voyage'|'gift';scopeID:number;version:number;configVersion?:number;targets:DirectMessageTarget[];single?:DirectMessageTarget;disabledReason?:string;onFinished:()=>void;
}) {
 const [connection,setConnection]=useState<Connection>({status:'disconnected'}),[view,setView]=useState<QueueView|null>(null);
 const [loading,setLoading]=useState(true),[busy,setBusy]=useState(false),[error,setError]=useState(''),[pollPaused,setPollPaused]=useState(false);
 const [interval,setInterval]=useState(5),[queueInterval,setQueueInterval]=useState(5),[preview,setPreview]=useState('test'),[page,setPage]=useState(0),[confirmed,setConfirmed]=useState(false);
 const operation=useRef(false),mounted=useRef(true),finished=useRef(''),doneCallback=useRef(onFinished);doneCallback.current=onFinished;
 const q=view?.queue,connected=connection.status==='connected',active=!!q&&!['completed','cancelled'].includes(q.status);
 const sameScope=q?.scope===scope&&q?.scope_id===scopeID;
 useEffect(()=>{if(q)setQueueInterval(q.interval)},[q?.id,q?.interval]);
 const senderUID=connected?connection.account?.uid||'':'';
 const eligibleTargets=senderUID?targets.filter(item=>item.uid!==senderUID):targets,selfTargetCount=targets.length-eligibleTargets.length;
 const eligibleSingle=single&&single.uid!==senderUID?single:undefined;
 const read=useCallback(async()=>{
  const [session,queue]=await Promise.all([voyageRequest<Connection>(login),voyageRequest<QueueView|null>(`${api}?scope=${scope}&scope_id=${scopeID}`)]);
  if(mounted.current){setConnection(session);setView(queue)}
 },[scope,scopeID]);
 useEffect(()=>{mounted.current=true;setLoading(true);void read().catch(e=>{if(mounted.current){setError((e as Error).message);setPollPaused(true)}}).finally(()=>{if(mounted.current)setLoading(false)});return()=>{mounted.current=false}},[read]);
 useEffect(()=>{
  if(busy||loading||pollPaused)return;
  let cancelled=false;
  const timer=window.setTimeout(async()=>{
   try{
    if(['waiting','scanned'].includes(connection.status)){
     const data=await voyageRequest<Connection>(`${login}/poll`,'POST',{});if(!cancelled)setConnection(data);
    }else if(active){await read()}
   }catch(e){if(!cancelled){setError((e as Error).message);setPollPaused(true)}}
  },3000);
  return()=>{cancelled=true;window.clearTimeout(timer)};
 },[connection,view,busy,loading,pollPaused,active,read]);
 useEffect(()=>{
  if(!q||!['completed','cancelled','paused'].includes(q.status))return;
  const stamp=`${q.id}:${q.status}:${view?.items.filter(i=>i.status==='sent').length}`;
  if(finished.current!==stamp){finished.current=stamp;doneCallback.current()}
 },[q,view]);
 const run=async(fn:()=>Promise<void>)=>{
  if(operation.current)return;operation.current=true;setBusy(true);setError('');
  try{await fn();if(mounted.current)setPollPaused(false)}catch(e){if(mounted.current){setError((e as Error).message);setPollPaused(true)}}finally{operation.current=false;if(mounted.current)setBusy(false)}
 };
 const sessionAction=(method='POST')=>run(async()=>{
  if(method==='DELETE'&&!(await confirmAction('断开后会清除本站保存的 B 站登录，并暂停待发私信。',{title:'断开 B 站登录？',confirmLabel:'断开登录'})))return;
  setConnection(await voyageRequest<Connection>(login,method,{}));await read();
 });
 const prepare=(items:DirectMessageTarget[])=>run(async()=>{
  const data=await voyageRequest<QueueView>(api,'POST',{scope,scope_id:scopeID,version,config_version:configVersion,interval,items});setView(data);setPreview('test');setConfirmed(false);setPage(0);
 });
 const action=(action:string,item?:MessageItem)=>run(async()=>{
  if(!q)return;
  if(action==='cancel'&&!(await confirmAction('尚未发出的私信会停止。已经发出的私信保留，正在发送的一条仍可能完成。',{title:'取消这个队列？',confirmLabel:'取消队列',destructive:true})))return;
  if((['retry','resolve_sent'].includes(action)||(action==='skip'&&item?.status==='unknown'))&&!(await confirmAction((action==='retry'||action==='skip')?`请先在 B 站确认给 UID ${item?.uid} 的这条消息没有发出。加入待发后仍需点击继续发送。`:`确认已在 B 站看到给 UID ${item?.uid} 的这条私信？`,{title:(action==='retry'||action==='skip')?'确认没有发出？':'确认已经发出？',confirmLabel:'已核对，确认'})))return;
  const data=await voyageRequest<QueueView>(`${api}/${q.id}/action`,'POST',{action,item_id:item?.id,interval:action==='set_interval'?queueInterval:undefined,confirmed:action==='confirm'?confirmed:true});setView(data);
 });
 const test=()=>run(async()=>{
  if(!q)return;let retryConfirmed=false;
  if(q.test_status==='unknown'){retryConfirmed=await confirmAction(`上次请求没有返回确定结果。请先查看 B 站私信，确认 UID ${q.test_uid} 没有收到这条测试消息。`,{title:'确认测试没有发出？',confirmLabel:'确认未发出，重新测试'});if(!retryConfirmed)return}
  setView(await voyageRequest<QueueView>(`${api}/${q.id}/test`,'POST',{retry_confirmed:retryConfirmed}));setConfirmed(false);
 });
 const currentText=preview==='test'?view?.test_text:view?.items.find(i=>String(i.id)===preview)?.text;
 const items=view?.items||[],sent=items.filter(i=>i.status==='sent').length,remaining=items.filter(i=>i.status==='pending').length;
	 const unresolved=items.some(i=>['unknown','sending'].includes(i.status));
 const pages=Math.max(1,Math.ceil(items.length/20)),current=Math.min(page,pages-1);
 const nextAt=view?.next_at?new Date(view.next_at):null,cooling=!!nextAt&&nextAt.getTime()>Date.now();
 return <section aria-label="扫码登录直接私信" className="mb-6 space-y-5 rounded-xl border border-border p-5 sm:p-6">
  <MessageQueueMonitor scope={scope} scopeID={scopeID} onChanged={()=>{void read().catch(e=>setError((e as Error).message));doneCallback.current()}}/>
  <div className="flex flex-wrap items-start justify-between gap-3"><div><h3 className="font-semibold">直接发私信</h3><p className="mt-1 text-sm leading-6 text-muted-foreground">预览内容 → 向测试账号发送 → 确认后逐条发送</p></div><Button variant="ghost" size="sm" disabled={busy||loading} onClick={()=>void run(read)}>刷新状态</Button></div>
  {loading?<p role="status" className="text-sm">正在读取 B 站登录与私信队列…</p>:<>
   <div className="space-y-3">
    {connected?<div className="flex flex-wrap items-center gap-3 text-sm"><span>发送账号：<strong>{connection.account?.name||connection.account?.uid}</strong> · UID {connection.account?.uid}</span><Button size="sm" variant="ghost" disabled={busy} onClick={()=>void sessionAction('DELETE')}>断开登录</Button></div>:<>
     {connection.qr_url&&['waiting','scanned'].includes(connection.status)?<div className="flex flex-col gap-3 sm:flex-row sm:items-center"><QRCodeSVG value={connection.qr_url} size={176} marginSize={4} title="B 站登录二维码"/><div className="space-y-3 text-sm"><p>{connection.status==='scanned'?'请在手机上确认登录。':'用主播的哔哩哔哩 App 扫码登录。'}</p><Button variant="outline" size="sm" disabled={busy} onClick={()=>void sessionAction()}>刷新二维码</Button></div></div>:<Button disabled={busy} onClick={()=>void sessionAction()}>{busy?'正在连接…':'扫码登录 B 站'}</Button>}
    </>}
    <p className="max-w-prose text-xs leading-5 text-muted-foreground">登录会加密保存在本站，名单导入和两处私信共用。关闭页面无需重新扫码；登录失效时再验证，也可随时断开。</p>
   </div>
   {!active&&!unresolved&&<div className="space-y-3 border-t border-border pt-4">
    <label className="flex flex-wrap items-center gap-3 text-sm">每条私信的间隔<select aria-label="私信发送间隔" className={`${voyageField} !w-auto`} value={interval} disabled={busy} onChange={e=>setInterval(Number(e.target.value))}>{intervalOptions.map(n=><option key={n} value={n}>{n} 秒</option>)}</select></label>
    <div className="flex flex-wrap gap-3">{eligibleSingle&&<Button disabled={busy||!connected||!!disabledReason} onClick={()=>void prepare([eligibleSingle])}>预览这条私信</Button>}<Button variant={eligibleSingle?'outline':'default'} disabled={busy||!connected||!!disabledReason||eligibleTargets.length===0} onClick={()=>void prepare(eligibleTargets.slice(0,500))}>{eligibleTargets.length>500?`预览前 500 条待发（共 ${eligibleTargets.length} 条）`:`预览全部待发（${eligibleTargets.length}）`}</Button></div>
    {selfTargetCount>0&&<p role="status" className="text-sm leading-6 text-muted-foreground">已自动排除当前发送账号 UID {senderUID}，不会给自己发送私信。{eligibleTargets.length>0&&`其余 ${eligibleTargets.length} 人可继续预览。`}</p>}
    {disabledReason&&<p className="text-sm text-muted-foreground">{disabledReason}</p>}{eligibleTargets.length>500&&<p className="text-sm text-muted-foreground">每次处理 500 条。完成后重新准备，会自动排除已经发出的用户。</p>}
	    <p className="text-xs leading-5 text-muted-foreground">单次报错会保留记录，继续下一人；连续 3 次报错时保守冷却 10 分钟，成功一条会重置计数。B 站明确限流时等待冷却，登录失效时需重新扫码。失败记录可在上方逐条重试。</p>
   </div>}
   {q&&view&&<div className="space-y-4 border-t border-border pt-4">
    <div className="flex flex-wrap items-center justify-between gap-3"><p role="status" className="text-sm font-medium">{labels[q.status]||q.status} · 已发 {sent} / {items.length} 条 · 间隔 {q.interval} 秒</p>{active&&<div className="flex gap-2">{q.status==='running'&&<Button size="sm" variant="outline" disabled={busy} onClick={()=>void action('pause')}>暂停发送</Button>}<Button size="sm" variant="ghost" disabled={busy} onClick={()=>void action('cancel')}>取消队列</Button></div>}</div>
	    {!sameScope&&<p className="text-sm text-muted-foreground">下方是当前登录账号在{q.scope==='voyage'?'地址收集':'电子礼物'}中的待发队列，编号 {q.scope_id}。当前页面的各管理员发送进度请看上方“发送队列与进度”。</p>}
    {q.note&&q.status!=='completed'&&<p role="status" className="text-sm leading-6">{q.note}</p>}
    {active&&!['running','testing'].includes(q.status)&&<div className="flex flex-wrap items-center gap-3"><label className="flex flex-wrap items-center gap-3 text-sm">每条私信的间隔<select aria-label="当前队列发送间隔" className={`${voyageField} !w-auto`} value={queueInterval} disabled={busy} onChange={e=>setQueueInterval(Number(e.target.value))}>{Array.from(new Set([...intervalOptions,q.interval])).sort((a,b)=>a-b).map(n=><option key={n} value={n}>{n} 秒</option>)}</select></label><Button size="sm" variant="outline" disabled={busy||queueInterval===q.interval} onClick={()=>void action('set_interval')}>保存间隔</Button><p className="w-full text-xs leading-5 text-muted-foreground">保存后继续发送时生效。连续报错或 B 站明确限流时，等待冷却后自动继续。</p></div>}
    {cooling&&<p className="text-xs text-muted-foreground">下一条最早在 {nextAt!.toLocaleString('zh-CN',{hour12:false})} 发送。</p>}
    {active&&<>
     <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.35fr)]">
      <div className="space-y-3"><label className="block space-y-2 text-sm"><span>查看私信内容</span><select className={voyageField} value={preview} onChange={e=>setPreview(e.target.value)}><option value="test">测试消息 → UID {q.test_uid}</option>{items.map(i=><option key={i.id} value={i.id}>UID {i.uid} · {labels[i.status]}</option>)}</select></label><p className="text-xs leading-5 text-muted-foreground">测试消息使用示例密码，不会把其他人的领取凭据发给测试账号。正式私信会分别使用每个人的内容。需要修改时，取消队列后重新准备。</p></div>
      <div className="min-w-0 whitespace-pre-wrap break-words rounded-md bg-muted/40 p-4 text-sm leading-6" aria-label="私信内容预览">{currentText||'请选择一条私信查看。'}</div>
     </div>
     {q.test_status!=='sent'?<Button disabled={busy||!connected||q.status==='testing'||cooling} onClick={()=>void test()}>{q.status==='testing'?'正在发送测试…':q.test_status==='pending'?`向 UID ${q.test_uid} 发送测试消息`:'重新发送测试消息'}</Button>:q.status!=='running'&&<div className="space-y-3"><label className="flex items-start gap-2 text-sm leading-6"><input className="mt-1 accent-primary" type="checkbox" checked={confirmed} onChange={e=>setConfirmed(e.target.checked)}/><span>已确认 UID {q.test_uid} 收到测试消息，且内容无误</span></label><Button disabled={busy||!connected||!confirmed||cooling||items.some(i=>i.status==='sending')} onClick={()=>void action('confirm')}>确认测试无误，{sent?'继续':'开始'}发送{remaining>0?` ${remaining} 条`:''}</Button></div>}
    </>}
    <details><summary className="cursor-pointer text-sm focus-visible:outline-2 focus-visible:outline-ring">查看每条发送结果</summary><p className="mt-3 text-xs leading-5 text-muted-foreground">失败记录请在上方“发送队列与进度”中逐条重试。</p><ul className="mt-3 divide-y divide-border">{items.slice(current*20,(current+1)*20).map(item=><li key={item.id} className="flex flex-wrap items-center justify-between gap-3 py-3 text-sm"><div><span>UID {item.uid} · {labels[item.status]||item.status}</span>{item.note&&<p className="mt-1 max-w-prose text-xs leading-5 text-muted-foreground">{item.note}</p>}</div>{q.status!=='running'&&q.status!=='testing'&&['unknown','failed'].includes(item.status)&&<Button size="sm" variant="outline" disabled={busy} onClick={()=>void action('resolve_sent',item)}>已在 B 站确认发出</Button>}</li>)}</ul>{pages>1&&<div className="flex items-center justify-center gap-3 pt-3 text-sm"><Button size="sm" variant="outline" disabled={current===0} onClick={()=>setPage(current-1)}>上一页</Button><span>{current+1} / {pages}</span><Button size="sm" variant="outline" disabled={current+1>=pages} onClick={()=>setPage(current+1)}>下一页</Button></div>}</details>
   </div>}
  </>}
  {error&&!connected&&<Button size="sm" variant="outline" disabled={busy} onClick={()=>void sessionAction('DELETE')}>清除保存的登录</Button>}
  {error&&<p role="alert" className="text-sm leading-6 text-destructive">{error}{pollPaused?'（已停止自动检查，可点击“刷新状态”重试。）':''}</p>}
 </section>;
}
