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
const kinds:Record<string,string>={auth_success:'登录或验证成功',auth_failure:'登录或验证失败',rate_limit:'访问过于频繁',global_limit:'网站访问量过高',busy:'当前任务已满',blocked:'已拦截被封禁 IP',auto_ban:'自动封禁',manual_ban:'手动封禁',unban:'已解除封禁',origin_denied:'请求来源异常',invalid_message:'互动消息无效',message_limit:'互动消息过于频繁',connection_limit:'实时连接过多',request_rejected:'请求被拒绝'};
const scopes:Record<string,string>={all:'数据读取与操作',read:'浏览数据',download:'下载文件',write:'后台修改',post:'提交投稿',reaction:'表情互动',upload:'上传文件',claim:'领取验证',signature:'注册与首次设置',upstream:'调用外部服务',stream:'实时互动','admin-login':'后台登录','member-login':'粉丝登录'};
const reasons:Record<string,string>={auth_failures:'反复登录或验证失败',repeated_abuse:'持续频繁访问或发送无效请求',manual:'管理员手动添加'};
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
    const subject=b.ip||`验证对象 ${b.target?.slice(0,12)}`;
    if(!await confirmAction(`解除 ${subject} 的“${scopes[b.scope]||b.scope}”访问限制？解除后，访问过于频繁时仍会被限制。`,{title:'解除封禁',confirmLabel:'解除封禁',cancelLabel:'取消'}))return;
    if(await act('/bans/'+b.id,'DELETE'))setMessage('已解除封禁。');
  };
  const ban=async(e:FormEvent)=>{
    e.preventDefault();if(!await confirmAction(`封禁 ${ip.trim()}，持续 ${minutes} 分钟？期间该地址将无法登录、投稿或读取网站数据。请勿封禁自己的 IP；共享代理地址可能对应多位访客。`,{title:'确认封禁该 IP',confirmLabel:'封禁',cancelLabel:'取消',destructive:true}))return;
    if(await act('/bans','POST',{ip:ip.trim(),minutes:Number(minutes)})){setIP('');setMessage('临时封禁已保存，到期自动解除。');}
  };
  if(isLoading)return <div className="space-y-4"><h1 className="text-2xl font-semibold">安全</h1><p role="status">正在确认管理员身份…</p></div>;
  if(!owner)return <div className="space-y-4"><h1 className="text-2xl font-semibold">安全</h1><p>仅站点所有者可查看安全记录和管理封禁。</p></div>;
  const rows=view==='bans'?bans:events,total=rows.length,visible=rows.slice(page*25,(page+1)*25);
  return <div className="min-w-0 space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-2xl font-semibold">安全与访问记录</h1><p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">网站会自动限制频繁访问和反复登录失败。你可以在这里查看拦截记录，也可以手动封禁或解封 IP。</p></div><div className="flex items-center gap-3"><span className="text-xs text-muted-foreground">{updated?`${updated} 更新`:'尚未读取'}</span><Button variant="outline" onClick={()=>void load()} disabled={busy}>{busy?'正在读取…':'刷新记录'}</Button></div></header>
    {error&&<div role="alert" className="rounded-lg border border-destructive/30 p-4 text-sm"><p>{error}</p><Button className="mt-2" size="sm" variant="outline" disabled={busy} onClick={()=>void load()}>重新读取</Button></div>}
    {message&&<p role="status" className="text-sm">{message}</p>}
    {data&&!data.persistence_ok&&<p role="alert" className="rounded-lg border border-destructive/30 p-4 text-sm">安全记录暂时无法保存，新的登录和修改操作可能受限。请让维护人员检查数据目录的权限和磁盘空间。</p>}
    <dl className="grid grid-cols-2 gap-4 rounded-xl bg-card p-4 sm:p-5 lg:grid-cols-4">
      {[['正在生效的封禁',data?.active_bans],['记录内的验证失败',data?failures:undefined],['实时互动连接',data?.connections],['安全记录保存',data?(data.persistence_ok?'正常':'需处理'):undefined]].map(([label,value])=><div key={String(label)} className="min-w-0"><dt className="min-h-10 text-sm text-muted-foreground lg:min-h-0">{label}</dt><dd className="mt-2 text-2xl tabular-nums">{value===undefined?'—':typeof value==='number'?number(value):value}</dd></div>)}
    </dl>
    <section className="min-w-0 rounded-xl bg-card p-4 sm:p-5" aria-label="封禁与安全事件">
      <div className="flex flex-wrap items-center justify-between gap-4"><div className="flex gap-1" role="group" aria-label="记录类型"><Button variant={view==='bans'?'secondary':'ghost'} aria-pressed={view==='bans'} onClick={()=>setView('bans')}>封禁中</Button><Button variant={view==='events'?'secondary':'ghost'} aria-pressed={view==='events'} onClick={()=>setView('events')}>事件记录</Button></div><div className="flex flex-wrap gap-3"><Input aria-label="筛选IP或匿名编号" placeholder="筛选 IP 或匿名编号" value={filter} onChange={e=>setFilter(e.target.value)} className="w-full sm:w-64"/>{view==='events'&&<select aria-label="事件类型" value={kind} onChange={e=>setKind(e.target.value)} className="h-10 rounded-md border border-input bg-background px-3 text-sm"><option value="all">全部事件</option>{Object.entries(kinds).map(([id,label])=><option key={id} value={id}>{label}</option>)}</select>}</div></div>
      <p className="my-4 text-sm leading-6 text-muted-foreground">{data?`只保留最近 ${data.retention_days} 天内、最多 ${number(data.event_limit)} 条安全记录，不是全部访问日志。达到上限时会清理较早记录，短时间内的重复事件会合并显示。上方统计只计算当前保留的记录。`:'正在读取安全记录。'}</p>
      {visible.length===0?<p className="rounded-lg border border-border p-5 text-sm">{busy&&!data?'正在读取…':filter||kind!=='all'?'没有符合筛选条件的记录。':view==='bans'?'当前没有生效中的封禁。':'尚无安全事件。'}</p>:<ul className="divide-y divide-border">{visible.map(row=>view==='bans'?<li key={row.id} className="flex flex-wrap items-start justify-between gap-3 py-4"><div className="min-w-0 flex-1"><p className="break-all text-sm font-medium">{(row as Ban).ip||`匿名编号 ${(row as Ban).target?.slice(0,12)}`}</p><p className="mt-1 text-sm">{scopes[row.scope]||row.scope} · {reasons[(row as Ban).reason]||(row as Ban).reason}</p><p className="mt-1 text-xs text-muted-foreground">到期 {stamp((row as Ban).until)}</p></div><Button variant="outline" size="sm" disabled={busy} onClick={()=>void unban(row as Ban)}>解除封禁</Button></li>:<li key={row.id} className="flex items-start justify-between gap-4 py-4"><div className="min-w-0"><p className="text-sm font-medium">{kinds[(row as Event).kind]||(row as Event).kind} · {scopes[row.scope]||row.scope}</p><p className="mt-1 break-all text-sm">{(row as Event).ip}{row.target&&<span className="text-muted-foreground"> · 匿名编号 {row.target.slice(0,12)}</span>}</p><p className="mt-1 text-xs text-muted-foreground">{stamp((row as Event).at)}</p></div><p className="shrink-0 text-right text-sm tabular-nums">{number((row as Event).count)} 次</p></li>)}</ul>}
      {total>25&&<div className="mt-4 flex flex-wrap items-center justify-between gap-3 text-sm"><p className="tabular-nums">{page*25+1}–{Math.min((page+1)*25,total)} / {number(total)}</p><div className="flex gap-2"><Button variant="outline" size="sm" disabled={page===0} onClick={()=>setPage(p=>p-1)}>上一页</Button><Button variant="outline" size="sm" disabled={(page+1)*25>=total} onClick={()=>setPage(p=>p+1)}>下一页</Button></div></div>}
    </section>
    <details className="rounded-xl bg-card p-4 sm:p-5"><summary className="cursor-pointer text-sm font-medium">手动封禁 IP</summary><form onSubmit={ban} className="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_12rem_auto] lg:items-end"><div className="space-y-2"><Label htmlFor="ban-ip">要封禁的 IP</Label><Input id="ban-ip" value={ip} onChange={e=>setIP(e.target.value)} placeholder="IPv4 或 IPv6 地址" maxLength={64} required disabled={busy}/></div><div className="space-y-2"><Label htmlFor="ban-minutes">封禁时长</Label><select id="ban-minutes" value={minutes} onChange={e=>setMinutes(e.target.value)} className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm" disabled={busy}><option value="15">15 分钟</option><option value="60">1 小时</option><option value="1440">1 天</option><option value="10080">7 天</option></select></div><Button type="submit" variant="outline" disabled={busy||!ip.trim()}>添加封禁</Button><p className="text-sm leading-6 text-muted-foreground lg:col-span-3">手动封禁会限制该 IP 的数据读取与操作，到期自动解除。请勿填写自己的 IP；封禁共享代理可能影响多位访客。</p></form></details>
    <details className="rounded-xl bg-card p-4 sm:p-5"><summary className="cursor-pointer text-sm font-medium">如何理解这些记录</summary><div className="mt-4 space-y-3 text-sm leading-6 text-muted-foreground"><p>自动封禁通常只限制触发异常的功能，例如后台登录；手动封禁则限制该 IP 的数据读取与操作。封禁针对网站的数据接口，静态页面可能仍能打开。</p><p>为了保护账号和领取口令，部分记录只显示匿名编号。同一编号代表同一验证对象。时间按当前设备的时区显示。</p>{data?.client_ip&&<p className="break-all">系统识别的本次访问 IP：{data.client_ip}。{data.private_client_ip?'当前显示内网地址。如果网站已开放公网，请让维护人员检查代理设置，避免把所有访客识别为同一个地址。':'若使用 CDN 或代理，请让维护人员核对这个地址是否正确。'}</p>}<p>本页只管理网站内的保护。服务器防火墙、CDN 和 Fail2ban 的限制需由维护人员另外检查。</p></div></details>
  </div>;
}
