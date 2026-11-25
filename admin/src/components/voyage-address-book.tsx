'use client';
import {useCallback,useEffect,useRef,useState} from 'react';
import {ArrowLeft,RefreshCw,MapPin} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {BilibiliPerson} from '@/components/bilibili-person';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import {validProfileUID} from '@/lib/bilibili-profiles';
import {voyageRequest} from '@/lib/voyage';
import {voyageField} from '@/lib/voyage-recipients';
import {confirmAction} from '@/lib/site-dialog';
import {useSidebarNavigationGuard} from '@/lib/use-sidebar-navigation-guard';
import {addressBookAction,addressUseLabels,consentActionLabels,type AddressBookDetail,type AddressBookPerson} from '@/lib/voyage-address-book';

const date=(value:string)=>new Date(value).toLocaleString('zh-CN');
export default function VoyageAddressBook(){
 const [rows,setRows]=useState<AddressBookPerson[]>([]),[total,setTotal]=useState(0),[page,setPage]=useState(1),[query,setQuery]=useState('');
 const [detail,setDetail]=useState<AddressBookDetail|null>(null),[selected,setSelected]=useState(0),[recipient,setRecipient]=useState(0);
 const [mode,setMode]=useState<''|'confirm_once'|'set_default'>(''),[channel,setChannel]=useState('B站私信'),[confirmedAt,setConfirmedAt]=useState('');
 const [loading,setLoading]=useState(true),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const running=useRef(false),active=useRef(true);
 const profiles=useBilibiliProfiles(detail?[detail.uid]:rows.map(r=>r.uid));
 useSidebarNavigationGuard(()=>!running.current);
 const loadList=useCallback(async(signal?:AbortSignal)=>{
  const data=await voyageRequest<{rows:AddressBookPerson[];total:number}>(`/api/voyage/address-book?page=${page}&q=${encodeURIComponent(query.trim())}`,'GET',undefined,signal);
  if(active.current){setRows(data.rows);setTotal(data.total)}
 },[page,query]);
 const loadDetail=useCallback(async(uid:string)=>{
  const data=await voyageRequest<AddressBookDetail>(`/api/voyage/address-book/${uid}`);
  if(active.current){setDetail(data);setSelected(data.entries.find(e=>e.is_default)?.id||data.entries[0]?.id||0);setRecipient(data.collections[0]?.recipient_id||0);setMode('');setConfirmedAt('')}
 },[]);
 const run=useCallback(async(fn:()=>Promise<void>)=>{
  if(running.current)return;
  running.current=true;setBusy(true);setError('');setNotice('');
  try{await fn()}catch(e){if(active.current)setError((e as Error).message)}finally{running.current=false;if(active.current)setBusy(false)}
 },[]);
 useEffect(()=>{active.current=true;const cancel=new AbortController();const timer=setTimeout(()=>void loadList(cancel.signal).catch(e=>{if(e.name!=='AbortError'&&active.current)setError(e.message)}).finally(()=>{if(active.current&&!cancel.signal.aborted)setLoading(false)}),250);return()=>{clearTimeout(timer);cancel.abort();active.current=false}},[loadList]);
 useEffect(()=>{const uid=new URLSearchParams(window.location.search).get('uid');if(uid&&validProfileUID(uid))void run(()=>loadDetail(uid))},[loadDetail,run]);
 const apply=async(action:'set_default'|'clear_default'|'confirm_once')=>{
  if(!detail)return;
  if(action==='clear_default'&&!await confirmAction('关闭后，后续收集不再自动填写。已经登记的地址仍保留。',{title:'关闭自动填写？',confirmLabel:'关闭自动填写',cancelLabel:'保留设置'}))return;
  await run(async()=>{
   await voyageRequest(`/api/voyage/address-book/${detail.uid}/action`,'POST',addressBookAction(detail,action,selected,recipient,channel,confirmedAt));
   await Promise.all([loadDetail(detail.uid),loadList()]);
   setNotice(action==='confirm_once'?'已将所选地址登记到本批次。':action==='set_default'?'礼物托管已开启，后续符合条件的收集批次将使用所选地址。':'已关闭自动填写，已登记的地址保持不变。');
  });
 };
 const chosen=detail?.entries.find(e=>e.id===selected),pages=Math.max(1,Math.ceil(total/25));
 return <div className="mx-auto w-full max-w-6xl"><header className="mb-6 flex flex-wrap items-start gap-4"><div className="mr-auto"><h1 className="text-2xl font-semibold">地址簿</h1><p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">用户保存过的收货地址会在这里留档。使用历史地址前，请先与用户确认。</p></div><Button size="sm" variant="ghost" disabled={busy} onClick={()=>void run(async()=>{await loadList();if(detail)await loadDetail(detail.uid)})}><RefreshCw size={15}/>刷新</Button></header>
  {error&&<p role="alert" className="mb-5 text-sm leading-6 text-destructive">{error}</p>}{notice&&<p role="status" className="mb-5 text-sm leading-6 text-primary">{notice}</p>}
  {!detail?<><label className="mb-5 block max-w-lg space-y-2 text-sm">搜索用户<Input value={query} disabled={busy} placeholder="B 站 UID 或昵称" onChange={e=>{setQuery(e.target.value);setPage(1)}}/></label>
   {loading?<p role="status" className="py-10 text-muted-foreground">正在读取地址簿…</p>:<div className="divide-y divide-border border-y border-border">{rows.map(row=><article key={row.uid} className="flex flex-wrap items-center gap-5 py-5"><div className="min-w-48 flex-1"><BilibiliPerson uid={row.uid} name={row.name} profile={profiles[row.uid]}/></div><div className="text-sm leading-6"><p>{row.count} 个地址 · {row.enabled?row.source==='admin'?'礼物托管已开启':'已设置常用地址':'尚未设置常用地址'}</p><p className="text-xs text-muted-foreground">最近记录：{date(row.updated_at)}</p></div><Button variant="outline" size="sm" disabled={busy} onClick={()=>void run(()=>loadDetail(row.uid))}>查看地址簿</Button></article>)}{!rows.length&&<p className="py-10 text-sm leading-6 text-muted-foreground">{query?'没有找到对应用户。':'尚无地址记录。用户保存收货地址后，会在这里留档。'}</p>}</div>}
   <div className="mt-5 flex items-center justify-center gap-3 text-sm"><Button variant="outline" size="sm" disabled={busy||page<=1} onClick={()=>setPage(page-1)}>上一页</Button><span>{total} 人 · {page}/{pages} 页</span><Button variant="outline" size="sm" disabled={busy||page>=pages} onClick={()=>setPage(page+1)}>下一页</Button></div>
  </>:<><Button className="mb-5" variant="ghost" size="sm" disabled={busy} onClick={()=>{setDetail(null);setMode('');setConfirmedAt('');setNotice('')}}><ArrowLeft size={15}/>返回地址簿</Button><div className="mb-6"><BilibiliPerson uid={detail.uid} profile={profiles[detail.uid]}/></div>
   <div className="grid gap-7 lg:grid-cols-[minmax(0,1fr)_minmax(18rem,.8fr)]"><section aria-label="已保存的收货地址" className="min-w-0"><h2 className="mb-2 font-semibold">收货地址（{detail.entries.length}）</h2><p className="mb-4 text-xs leading-6 text-muted-foreground">可保存多个地址，常用地址只能设置一个。历史备注仅用于对应收集批次。</p><div className="divide-y divide-border border-y border-border">{detail.entries.map(entry=><article key={entry.id} className="py-5"><label className="flex cursor-pointer items-start gap-3"><input type="radio" name="book-address" className="mt-1 size-4 accent-primary" checked={selected===entry.id} disabled={busy} onChange={()=>{setSelected(entry.id);setMode('');setConfirmedAt('')}}/><div className="min-w-0 text-sm leading-6"><p className="font-medium">{entry.address.name} · {entry.address.phone}{entry.is_default&&<span className="ml-3 text-xs text-primary">常用地址</span>}</p><p className="break-words">{entry.address.address}</p><p className="mt-2 text-xs text-muted-foreground">最近使用：{date(entry.last_used_at)}</p></div></label>{!!entry.uses.length&&<details className="ml-7 mt-3 text-xs leading-6"><summary className="cursor-pointer text-muted-foreground">使用记录（{entry.uses.length}）</summary><div className="mt-2 space-y-3">{entry.uses.map((use,i)=><div key={i}><p>{use.title||'原收集批次'} · {addressUseLabels[use.source]||'历史登记'}</p><p className="text-muted-foreground">{date(use.recorded_at)}</p>{use.note&&<p className="whitespace-pre-wrap break-words">本批次备注：{use.note}</p>}</div>)}</div></details>}</article>)}</div>{!detail.entries.length&&<p className="py-8 text-sm text-muted-foreground">该用户尚无地址记录，请先让用户在收集页面填写。</p>}</section>
    <section aria-label="地址使用与礼物托管" className="min-w-0 space-y-5"><div className="border-b border-border pb-5"><h2 className="font-semibold">常用地址与礼物托管</h2><p className="mt-3 text-sm leading-6">{detail.long_term.enabled?detail.long_term.source==='admin'?'已由主播根据用户委托开启礼物托管。':'用户已开启常用地址自动填写。':'尚未开启自动填写。'}</p>{detail.long_term.enabled&&<Button className="mt-3" size="sm" variant="outline" disabled={busy} onClick={()=>void apply('clear_default')}>关闭自动填写</Button>}</div>
     <div className="space-y-3"><p className="text-sm leading-6">选择左侧地址，与用户确认后再操作。</p><div className="flex flex-wrap gap-2"><Button size="sm" variant={mode==='confirm_once'?'secondary':'outline'} disabled={busy||!chosen||!detail.collections.length} onClick={()=>{setMode('confirm_once');setConfirmedAt('')}}>确认后用于本批次</Button><Button size="sm" variant={mode==='set_default'?'secondary':'outline'} disabled={busy||!chosen||chosen.is_default} onClick={()=>{setMode('set_default');setConfirmedAt('')}}>开启礼物托管</Button></div>{!detail.collections.length&&<p className="text-xs leading-6 text-muted-foreground">没有可补填的收集批次。已有地址和已寄出的记录保持不变。</p>}</div>
     {mode&&chosen&&<form className="space-y-4 border-y border-border py-5" onSubmit={e=>{e.preventDefault();void apply(mode)}}><h3 className="flex items-center gap-2 text-sm font-medium"><MapPin size={16}/>{mode==='confirm_once'?'确认本次使用':'开启礼物托管'}</h3><p className="text-sm leading-6">{chosen.address.name} · {chosen.address.phone}<br/>{chosen.address.address}</p>{mode==='confirm_once'?<label className="block space-y-2 text-sm">使用地址的收集批次<select className={voyageField} disabled={busy} value={recipient} onChange={e=>{setRecipient(Number(e.target.value));setConfirmedAt('')}}>{detail.collections.map(c=><option key={c.recipient_id} value={c.recipient_id}>{c.title}</option>)}</select></label>:<p className="text-sm leading-6 text-muted-foreground">开启后，符合领取条件且尚未填写地址的开放批次将使用此地址。已填写的地址不会自动更改，用户可随时关闭自动填写。</p>}<label className="block space-y-2 text-sm">确认渠道<select className={voyageField} value={channel} disabled={busy} onChange={e=>{setChannel(e.target.value);setConfirmedAt('')}}><option>B站私信</option><option>其他渠道</option></select></label><label className="flex cursor-pointer items-start gap-3 text-sm leading-6"><input type="checkbox" className="mt-1 size-4 shrink-0 accent-primary" checked={!!confirmedAt} disabled={busy} onChange={e=>setConfirmedAt(e.target.checked?new Date().toISOString():'')}/><span>{mode==='confirm_once'?'我已与用户确认，本次使用所选地址。':'用户已明确委托，后续礼物使用所选地址。'}</span></label>{confirmedAt&&<p className="text-xs text-muted-foreground">本次确认时间：{date(confirmedAt)}</p>}<div className="flex gap-2"><Button type="submit" size="sm" disabled={busy||!confirmedAt||mode==='confirm_once'&&!recipient}>{busy?'正在保存…':mode==='confirm_once'?'确认登记本次地址':'确认开启礼物托管'}</Button><Button type="button" size="sm" variant="ghost" disabled={busy} onClick={()=>{setMode('');setConfirmedAt('')}}>取消</Button></div></form>}
     {!!detail.events.length&&<details className="text-sm"><summary className="cursor-pointer font-medium">确认与设置记录</summary><div className="mt-4 space-y-4">{detail.events.map(event=><div key={event.id} className="text-xs leading-6"><p>{consentActionLabels[event.action]||'地址设置变更'}{event.entry_id>0&&` · 地址 #${event.entry_id}`}</p><p className="text-muted-foreground">{date(event.occurred_at)}{event.admin_id?` · 管理员 #${event.admin_id}`:''}{event.channel?` · ${event.channel}`:''}</p>{event.confirmed_at&&<p className="text-muted-foreground">确认于 {date(event.confirmed_at)}</p>}</div>)}</div></details>}
    </section>
   </div>
  </>}
 </div>;
}
