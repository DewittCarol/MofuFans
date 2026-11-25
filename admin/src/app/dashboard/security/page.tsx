'use client';

import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react';
import { useAuth } from '@/lib/auth';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { confirmAction } from '@/lib/site-dialog';

type Ban={id:string;kind:string;ip?:string;target?:string;scope:string;reason:string;until:string};
type Event={id:string;at:string;first_at:string;kind:string;ip:string;scope:string;target?:string;count:number};
type SecurityView={bans:Ban[];events:Event[];active_bans:number;connections:number;persistence_ok:boolean;retention_days:number;event_limit:number;client_ip?:string;private_client_ip?:boolean};
const kinds:Record<string,string>={auth_success:'验证成功',auth_failure:'验证失败',rate_limit:'请求限速',global_limit:'全局限速',busy:'并发已满',blocked:'封禁拦截',auto_ban:'自动封禁',manual_ban:'手动封禁',unban:'解除封禁',origin_denied:'来源不允许',invalid_message:'无效实时消息',message_limit:'实时消息过频',connection_limit:'实时连接已满',request_rejected:'请求被拒绝'};
const scopes:Record<string,string>={all:'所有接口',read:'页面数据读取',download:'文件读取',write:'管理与业务操作',post:'投稿',reaction:'表情互动',upload:'上传',claim:'领取凭据',signature:'签名与初始化',upstream:'外部服务',stream:'实时连接','admin-login':'管理员验证','member-login':'粉丝验证'};
const reasons:Record<string,string>={auth_failures:'连续验证失败',repeated_abuse:'持续超限或无效请求',manual:'站点所有者设置'};
const stamp=(value:string)=>new Date(value).toLocaleString('zh-CN');
const number=(value:number)=>value.toLocaleString('zh-CN');

async function request(path:string,method='GET',body?:unknown){
  const response=await fetch('/api/admin/security'+path,{method,credentials:'include',cache:'no-store',headers:{'Content-Type':'application/json','X-MofuFans-Request':'settings'},...(body===undefined?{}:{body:JSON.stringify(body)})});
  const result=await response.json();return result;
}

export default function SecurityPage(){
  const {user,isLoading,confirmPassword}=useAuth();
  const owner=!!user?.is_super_admin;
  const [data,setData]=useState<SecurityView|null>(null),[error,setError]=useState(''),[message,setMessage]=useState('');
  const [busy,setBusy]=useState(false),[updated,setUpdated]=useState('');
  const [view,setView]=useState<'bans'|'events'>('bans'),[filter,setFilter]=useState(''),[kind,setKind]=useState('all'),[page,setPage]=useState(0);
  const [ip,setIP]=useState(''),[minutes,setMinutes]=useState('60');
  const load=useCallback(async()=>{setBusy(true);try{const result=await request('');if(result.code!==200)throw new Error(result.message||'安全记录读取失败');setData(result.data);setUpdated(new Date().toLocaleTimeString('zh-CN'));setError('');}catch(e){setError(e instanceof Error?e.message:'安全记录读取失败');}finally{setBusy(false);}},[]);
  useEffect(()=>{if(owner)void load();else setData(null);},[owner,load]);
  useEffect(()=>{setPage(0);},[view,filter,kind]);
  const events=useMemo(()=>data?.events.filter(e=>(kind==='all'||e.kind===kind)&&`${e.ip} ${e.target||''} ${scopes[e.scope]||e.scope}`.toLowerCase().includes(filter.trim().toLowerCase()))||[],[data,kind,filter]);
  const bans=useMemo(()=>data?.bans.filter(b=>`${b.ip||''} ${b.target||''} ${scopes[b.scope]||b.scope}`.toLowerCase().includes(filter.trim().toLowerCase()))||[],[data,filter]);
  const failures=data?.events.filter(e=>e.kind==='auth_failure').reduce((sum,e)=>sum+e.count,0)||0;
  const act=async(path:string,method:string,body?:unknown)=>{
    if(!await confirmPassword())return false;
    setBusy(true);setError('');setMessage('');
    try{let result=await request(path,method,body);if(result.code===428&&await confirmPassword(true))result=await request(path,method,body);if(result.code!==200)throw new Error(result.message||'操作未完成');await load();return true;}catch(e){setError(e instanceof Error?e.message:'操作未完成');return false;}finally{setBusy(false);}
  };
  const unban=async(b:Ban)=>{
    const subject=b.ip||`目标标识 ${b.target?.slice(0,12)}`;
    if(!await confirmAction(`解除 ${subject} 在“${scopes[b.scope]||b.scope}”中的封禁？后续请求仍受正常限额限制。`,{title:'解除封禁',confirmLabel:'解除',cancelLabel:'保留'}))return;
    if(await act('/bans/'+b.id,'DELETE'))setMessage('封禁已解除，正常限额仍然生效。');
  };
  const ban=async(e:FormEvent)=>{
    e.preventDefault();if(!await confirmAction(`将 ${ip.trim()} 对本站所有接口的访问暂停 ${minutes} 分钟。请确认它不是自己的出口或代理网关；误封后可停服并使用本地恢复命令。`,{title:'封禁该 IP？',confirmLabel:'封禁',cancelLabel:'取消',destructive:true}))return;
    if(await act('/bans','POST',{ip:ip.trim(),minutes:Number(minutes)})){setIP('');setMessage('临时封禁已保存，到期自动解除。');}
  };
  if(isLoading)return <div className="space-y-4"><h1 className="text-2xl font-semibold">安全</h1><p role="status">正在确认管理员身份…</p></div>;
  if(!owner)return <div className="space-y-4"><h1 className="text-2xl font-semibold">安全</h1><p>仅站点所有者可查看安全记录和管理封禁。</p></div>;
  const rows=view==='bans'?bans:events,total=rows.length,visible=rows.slice(page*25,(page+1)*25);
  return <div className="min-w-0 space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-2xl font-semibold">安全</h1><p className="mt-2 text-sm text-muted-foreground">应用防护默认启用。自动封禁只限制对应接口，手动封禁限制该 IP 的所有接口。</p></div><div className="flex items-center gap-3"><span className="text-xs text-muted-foreground">{updated?`${updated} 更新`:'尚未读取'}</span><Button variant="outline" onClick={()=>void load()} disabled={busy}>{busy?'正在读取…':'刷新记录'}</Button></div></header>
    {error&&<div role="alert" className="rounded-lg border border-destructive/30 p-4 text-sm"><p>{error}</p><Button className="mt-2" size="sm" variant="outline" disabled={busy} onClick={()=>void load()}>重新读取</Button></div>}
    {message&&<p role="status" className="text-sm">{message}</p>}
    {data?.client_ip&&<p className="break-all text-xs leading-6 text-muted-foreground">当前识别的客户端 IP：{data.client_ip}。{data.private_client_ip?'这是内网或回环地址；如通过代理开放公网，请核对可信代理，避免所有访客共用同一限额。':'通过 CDN 或代理时，请确认这里显示的是客户端出口，而非代理节点。'}</p>}
    {data&&!data.persistence_ok&&<p role="alert" className="rounded-lg border border-destructive/30 p-4 text-sm">安全状态暂时无法保存。新的验证或写入会受到保护性限制，请检查持久化目录的权限与可用空间。</p>}
    <dl className="grid grid-cols-2 gap-4 rounded-xl bg-card p-4 sm:p-5 lg:grid-cols-4">
      {[['当前封禁',data?.active_bans],['保留记录中的验证失败',data?failures:undefined],['当前实时连接',data?.connections],['状态持久化',data?(data.persistence_ok?'正常':'需处理'):undefined]].map(([label,value])=><div key={String(label)} className="min-w-0"><dt className="min-h-10 text-sm text-muted-foreground lg:min-h-0">{label}</dt><dd className="mt-2 text-2xl tabular-nums">{value===undefined?'—':typeof value==='number'?number(value):value}</dd></div>)}
    </dl>
    <section className="min-w-0 rounded-xl bg-card p-4 sm:p-5" aria-label="封禁与安全事件">
      <div className="flex flex-wrap items-center justify-between gap-4"><div className="flex gap-1" role="group" aria-label="记录类型"><Button variant={view==='bans'?'secondary':'ghost'} aria-pressed={view==='bans'} onClick={()=>setView('bans')}>封禁中</Button><Button variant={view==='events'?'secondary':'ghost'} aria-pressed={view==='events'} onClick={()=>setView('events')}>事件记录</Button></div><div className="flex flex-wrap gap-3"><Input aria-label="筛选IP或目标标识" placeholder="筛选 IP 或目标标识" value={filter} onChange={e=>setFilter(e.target.value)} className="w-full sm:w-64"/>{view==='events'&&<select aria-label="事件类型" value={kind} onChange={e=>setKind(e.target.value)} className="h-10 rounded-md border border-input bg-background px-3 text-sm"><option value="all">全部事件</option>{Object.entries(kinds).map(([id,label])=><option key={id} value={id}>{label}</option>)}</select>}</div></div>
      <p className="my-4 text-xs leading-6 text-muted-foreground">{data?`最多保留 ${number(data.event_limit)} 组事件，不超过 ${data.retention_days} 天。短时重复事件会合并，统计仅覆盖当前保留记录。`:'正在读取安全记录。'}时间按当前设备时区显示；目标标识为不可逆摘要。</p>
      {visible.length===0?<p className="rounded-lg border border-border p-5 text-sm">{busy&&!data?'正在读取…':filter||kind!=='all'?'没有符合筛选条件的记录。':view==='bans'?'当前没有生效中的封禁。':'尚无安全事件。'}</p>:<ul className="divide-y divide-border">{visible.map(row=>view==='bans'?<li key={row.id} className="flex flex-wrap items-start justify-between gap-3 py-4"><div className="min-w-0 flex-1"><p className="break-all text-sm font-medium">{(row as Ban).ip||`目标 ${(row as Ban).target?.slice(0,12)}`}</p><p className="mt-1 text-sm">{scopes[row.scope]||row.scope} · {reasons[(row as Ban).reason]||(row as Ban).reason}</p><p className="mt-1 text-xs text-muted-foreground">到期 {stamp((row as Ban).until)}</p></div><Button variant="outline" size="sm" disabled={busy} onClick={()=>void unban(row as Ban)}>解除封禁</Button></li>:<li key={row.id} className="flex items-start justify-between gap-4 py-4"><div className="min-w-0"><p className="text-sm font-medium">{kinds[(row as Event).kind]||(row as Event).kind} · {scopes[row.scope]||row.scope}</p><p className="mt-1 break-all text-sm">{(row as Event).ip}{row.target&&<span className="text-muted-foreground"> · 目标 {row.target.slice(0,12)}</span>}</p><p className="mt-1 text-xs text-muted-foreground">{stamp((row as Event).at)}</p></div><p className="shrink-0 text-right text-sm tabular-nums">{number((row as Event).count)} 次</p></li>)}</ul>}
      {total>25&&<div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm"><p className="tabular-nums">{page*25+1}–{Math.min((page+1)*25,total)} / {number(total)}</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page===0} onClick={()=>setPage(p=>p-1)}>上一页</Button><Button variant="outline" size="sm" disabled={(page+1)*25>=total} onClick={()=>setPage(p=>p+1)}>下一页</Button></div></div>}
    </section>
    <details className="rounded-xl bg-card p-4 sm:p-5"><summary className="cursor-pointer text-sm font-medium">手动添加临时封禁</summary><form onSubmit={ban} className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_12rem_auto] lg:items-end"><div className="space-y-2"><Label htmlFor="ban-ip">客户端 IP</Label><Input id="ban-ip" value={ip} onChange={e=>setIP(e.target.value)} placeholder="IPv4 或 IPv6 地址" maxLength={64} required disabled={busy}/></div><div className="space-y-2"><Label htmlFor="ban-minutes">封禁时长</Label><select id="ban-minutes" value={minutes} onChange={e=>setMinutes(e.target.value)} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" disabled={busy}><option value="15">15 分钟</option><option value="60">1 小时</option><option value="1440">1 天</option><option value="10080">7 天</option></select></div><Button type="submit" variant="outline" disabled={busy||!ip.trim()}>添加封禁</Button><p className="text-xs leading-6 text-muted-foreground lg:col-span-3">先核对真实客户端 IP，避免封到自己或共享代理。已有服务器防火墙与 Fail2ban 继续独立生效，本页不修改它们。</p></form></details>
  </div>;
}
