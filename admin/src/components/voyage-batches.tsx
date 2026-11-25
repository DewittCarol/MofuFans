'use client';
import {confirmDiscard} from '@/lib/site-dialog';
import {useEffect,useState,useRef,useCallback} from 'react';
import {RosterCategoryTabs,groupRosterMonths,type RosterCategory} from '@/components/voyage-roster-browser';
import {ChevronRight} from 'lucide-react';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import {BilibiliPerson} from '@/components/bilibili-person';
import {Button} from '@/components/ui/button';
import {useSidebarNavigationGuard} from '@/lib/use-sidebar-navigation-guard';
import {voyageRequest,voyageTiers,type VoyageIssue} from '@/lib/voyage';
import {voyageField} from '@/lib/voyage-recipients';
import {Delivery} from '@/components/voyage-dispatch-panel';
import type {Roster} from '@/components/voyage-rosters';
interface Source {roster_id:number;tiers:string[]}
interface Batch {id:number;title:string;note:string;published:boolean;closed:boolean;opens:string;closes:string;sources:Source[];version:number}
interface Preview {token:string;total:number;added:number;changed:number;removed:number;warnings:string[];rows:{uid:string;name:string;change:string;items:{label:string}[]}[]}

function BatchEditor({batch,rosters,onReload,onState}:{batch:Batch;rosters:Roster[];onReload:()=>Promise<void>;onState:(v:{busy:boolean;dirty:boolean})=>void}){
 const [draft,setDraft]=useState(batch),[sources,setSources]=useState<Source[]>(batch.sources||[]),[preview,setPreview]=useState<Preview|null>(null),[busy,setBusy]=useState(false),[tab,setTab]=useState('settings'),[error,setError]=useState(''),[notice,setNotice]=useState(''),[search,setSearch]=useState(''),[year,setYear]=useState('all'),[category,setCategory]=useState<RosterCategory>('monthly'),[dispatchState,setDispatchState]=useState({busy:false,dirty:false});
 const guard=useRef(false),base=JSON.stringify(batch),metaDirty=JSON.stringify(draft)!==base,sourceDirty=JSON.stringify(sources)!==JSON.stringify(batch.sources||[]),dirty=metaDirty||sourceDirty;
 useEffect(()=>{setDraft(batch);setPreview(null)},[batch]);
 // A settings save changes batch.version but must retain unsaved source choices.
 const sourceBase=JSON.stringify(batch.sources||[]);
 useEffect(()=>{setSources(JSON.parse(sourceBase));setPreview(null)},[sourceBase]);
 useEffect(()=>{onState({busy:busy||dispatchState.busy,dirty:dirty||dispatchState.dirty});return()=>onState({busy:false,dirty:false})},[busy,dirty,dispatchState,onState]);
 const run=async(fn:()=>Promise<void>)=>{if(guard.current)return;guard.current=true;setBusy(true);setError('');setNotice('');try{await fn()}catch(e){setError((e as Error).message)}finally{guard.current=false;setBusy(false)}};
 const save=()=>run(async()=>{if(metaDirty){await voyageRequest(`/api/voyage/batches/${batch.id}`,'PUT',draft);await onReload();setNotice('基础设置已保存。')}setTab('sources')});
 const sync=(confirm:boolean)=>run(async()=>{const result=await voyageRequest<Preview>(`/api/voyage/batches/${batch.id}/sync`,'POST',{sources,version:batch.version,confirm,token:confirm?preview?.token:''});if(confirm){await onReload();setPreview(null);setTab('dispatch');setNotice(`已保存，共 ${result.total} 位收件人。`)}else setPreview(result)});
 const publish=()=>run(async()=>{await voyageRequest(`/api/voyage/batches/${batch.id}`,'PUT',{...batch,published:true});await onReload();setNotice('批次已发布，可以发送私信请用户填写地址。')});
 const choose=(roster:Roster,enabled:boolean)=>{setPreview(null);setSources(current=>enabled?[...current,{roster_id:roster.id,tiers:roster.category==='monthly'?voyageTiers.map(t=>t.id):[]}]:current.filter(s=>s.roster_id!==roster.id))};
 const changeTiers=(id:number,tiers:string[])=>{setPreview(null);setSources(current=>current.map(s=>s.roster_id===id?{...s,tiers}:s))};
 const switchTab=async(next:string)=>{if(busy||dispatchState.busy)return;if(dispatchState.dirty&&!await confirmDiscard('私信模板尚未保存，确定离开吗？'))return;setTab(next);setError('')};
 const issue:VoyageIssue={id:batch.id,title:batch.title,note:batch.note,published:batch.published,version:batch.version,gifts:[],batch:true};
 const profiles=useBilibiliProfiles(preview?.rows.map(r=>r.uid)||[]);
 const filtered=rosters.filter(r=>r.category===category&&(year==='all'||r.month.startsWith(year))&&`${r.title} ${r.month}`.includes(search.trim()));
 return <section className="min-w-0"><header className="mb-4"><h2 className="text-2xl font-semibold">{batch.title}</h2><p className="mt-1 text-sm text-muted-foreground">{batch.published?(batch.closed?'已结束收集 · 可查询物流':'已发布'):'草稿 · 尚未发布'}</p></header>
  <nav className="mb-6" aria-label="地址收集流程"><ol className="flex items-center gap-1 rounded-xl border border-border bg-card p-1 sm:gap-2 sm:p-2">
   {[['settings','基础设置'],['sources','收集范围'],['dispatch','地址与私信']].map(([id,title],index)=><li key={id} className="flex min-w-0 flex-1 items-center gap-1 sm:gap-2">
    <button type="button" disabled={busy||dispatchState.busy} aria-current={tab===id?'step':undefined} className={`flex min-h-14 min-w-0 flex-1 flex-col items-center justify-center gap-2 rounded-lg px-2 py-3 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-ring disabled:cursor-wait disabled:opacity-60 sm:flex-row ${tab===id?'bg-secondary font-semibold text-primary':'text-muted-foreground hover:bg-secondary/50 hover:text-foreground'}`} onClick={()=>switchTab(id)}><span aria-hidden="true" className={`flex size-6 shrink-0 items-center justify-center rounded-full text-xs tabular-nums ${tab===id?'bg-primary text-primary-foreground':'border border-border'}`}>{index+1}</span><span><span className="sr-only">第 {index+1} 步：</span>{title}</span></button>
    {index<2&&<ChevronRight size={16} aria-hidden="true" className="shrink-0 text-muted-foreground"/>}
   </li>)}
  </ol></nav>
  {error&&<p role="alert" className="mb-4 text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="mb-4 text-sm leading-6 text-primary">{notice}</p>}
  {tab==='settings'&&<form className="max-w-3xl space-y-5 rounded-xl border border-border bg-card p-5 sm:p-6" onSubmit={e=>{e.preventDefault();void save()}}>
   <label className="block space-y-2 text-sm">批次名称<input className={voyageField} required maxLength={80} disabled={busy} value={draft.title} onChange={e=>setDraft({...draft,title:e.target.value})}/></label>
   <label className="block space-y-2 text-sm">收集说明<textarea className={voyageField} rows={4} maxLength={3000} disabled={busy} value={draft.note} placeholder="例如：本次集中收集5至8月舰礼及8月转盘奖品的寄送地址。" onChange={e=>setDraft({...draft,note:e.target.value})}/></label>
   <div className="grid gap-4 sm:grid-cols-2"><label className="space-y-2 text-sm">开始日期（选填）<input type="date" className={voyageField} disabled={busy} value={draft.opens} onChange={e=>setDraft({...draft,opens:e.target.value})}/></label><label className="space-y-2 text-sm">截止日期（选填）<input type="date" className={voyageField} disabled={busy} value={draft.closes} min={draft.opens||undefined} onChange={e=>setDraft({...draft,closes:e.target.value})}/></label></div>
   <div className="space-y-3 text-sm"><label className="flex min-h-11 items-center gap-2"><input type="checkbox" className="size-4 accent-primary" disabled={busy} checked={draft.published} onChange={e=>setDraft({...draft,published:e.target.checked})}/>在网站上公开这个批次</label><label className="flex min-h-11 items-center gap-2"><input type="checkbox" className="size-4 accent-primary" disabled={busy} checked={draft.closed} onChange={e=>setDraft({...draft,closed:e.target.checked})}/>结束地址收集，保留查询入口</label><p className="text-xs leading-6 text-muted-foreground">日期按北京时间计算，截止日当天仍可填写。结束收集后仍可安排发货；已有地址且还有礼物未寄完的用户可以更新地址。已寄包裹保留寄出时的地址。</p></div>
   <div className="flex gap-3"><Button type="submit" disabled={busy}>{metaDirty?'保存并继续':'下一步：收集范围'}</Button><Button variant="ghost" type="button" disabled={busy||!metaDirty} onClick={()=>setDraft(batch)}>撤销修改</Button></div>
  </form>}
  {tab==='sources'&&<div className="space-y-5">
   <RosterCategoryTabs value={category} onChange={value=>{setCategory(value);setYear("all");setSearch("")}} disabled={busy}>
    <p className="mb-4 text-sm leading-6 text-muted-foreground">{category==='monthly'?'选择需要统一登记地址的月份和档位，之后可以分开寄送。':'选择需要一起登记地址的活动名单。'}保存范围后，名单的新增、修改和删除会自动同步到本批次。</p>
    <div className="mb-4 flex flex-wrap gap-3">
     <label className="min-w-48 flex-1"><span className="sr-only">搜索名单</span><input className={voyageField} placeholder="搜索月份或名单名称" value={search} onChange={e=>setSearch(e.target.value)}/></label>
     <label className="w-full sm:w-40"><span className="sr-only">名单年份</span><select className={voyageField} value={year} onChange={e=>setYear(e.target.value)}><option value="all">全部年份</option>{[...new Set(rosters.filter(r=>r.category===category).map(r=>r.month.slice(0,4)).filter(Boolean))].sort().reverse().map(y=><option key={y}>{y}</option>)}</select></label>
    </div>
    <div className="max-h-[32rem] space-y-6 overflow-y-auto border-y border-border py-4">{groupRosterMonths(filtered).map(group=><section key={group.month} aria-label={group.title}>
     <h3 className="mb-2 text-sm font-semibold">{group.title}</h3>
     <div className="divide-y divide-border">{group.rows.map(roster=>{const selected=sources.find(s=>s.roster_id===roster.id);return <div key={roster.id} className="py-2">
      <label className="flex min-h-11 items-center gap-3 text-sm"><input type="checkbox" className="size-4 shrink-0 accent-primary" checked={!!selected} disabled={busy} onChange={e=>choose(roster,e.target.checked)}/><span className="break-words">{roster.title}</span></label>
      {selected&&roster.category==='monthly'&&<fieldset className="ml-7 flex flex-wrap gap-x-5 gap-y-2"><legend className="sr-only">{roster.title}要寄送的档位</legend><label className="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" className="accent-primary" checked={selected.tiers.length===3} disabled={busy} onChange={e=>changeTiers(roster.id,e.target.checked?voyageTiers.map(t=>t.id):[])}/>全部</label>{voyageTiers.map(tier=><label key={tier.id} className="flex min-h-10 items-center gap-2 text-sm"><input type="checkbox" className="accent-primary" disabled={busy} checked={selected.tiers.includes(tier.id)} onChange={e=>changeTiers(roster.id,e.target.checked?[...selected.tiers,tier.id]:selected.tiers.filter(t=>t!==tier.id))}/>{tier.name}</label>)}</fieldset>}
     </div>})}</div>
    </section>)}{!filtered.length&&<p className="py-5 text-sm text-muted-foreground">没有找到名单，可先到名单管理创建。</p>}</div>
   </RosterCategoryTabs>
   <div className="space-y-2 text-sm"><p className="font-medium">已选 {sources.length} 张名单</p>{sources.map(source=>{const r=rosters.find(r=>r.id===source.roster_id);return <p key={source.roster_id}>{r?.title||'已移除名单'}：{r?.category==='event'?'全部参与者':source.tiers.map(t=>voyageTiers.find(x=>x.id===t)?.name).join('、')||'请至少选择一个档位'}</p>})}</div>
   <div className="flex flex-wrap gap-3"><Button variant="outline" disabled={busy||metaDirty} onClick={()=>void sync(false)}>预览收件人</Button><Button variant="ghost" disabled={busy||!sourceDirty} onClick={()=>{setSources(batch.sources||[]);setPreview(null)}}>撤销选择</Button></div>{metaDirty&&<p className="text-sm text-muted-foreground">基础设置还有未保存内容，请先保存。</p>}
   {preview&&<section className="space-y-4 border-t border-border pt-5" aria-label="同步预览"><h3 className="font-medium">确认收件人</h3><p className="text-sm">共 {preview.total} 人；新增 {preview.added} 人，内容变化 {preview.changed} 人，移出 {preview.removed} 人。</p>{(preview.changed>0||preview.removed>0)&&<p className="text-sm leading-6 text-muted-foreground">领取内容变更后需要重新发送私信；移出的用户将不能再填写或查看这批地址。</p>}{!!preview.warnings.length&&<details className="text-sm"><summary className="cursor-pointer">查看 {preview.warnings.length} 条未加入说明</summary><ul className="mt-3 max-h-52 list-disc space-y-1 overflow-auto pl-5">{preview.warnings.map((w,i)=><li key={i}>{w}</li>)}</ul></details>}<div className="max-h-80 overflow-auto"><table className="w-full text-left text-sm"><thead><tr><th className="p-2">用户</th><th className="p-2">本次领取内容</th><th className="p-2">变化</th></tr></thead><tbody>{[...preview.rows].sort((a,b)=>Number(a.change==='unchanged')-Number(b.change==='unchanged')).slice(0,100).map(row=><tr key={row.uid} className="border-t border-border"><td className="p-2"><BilibiliPerson uid={row.uid} name={row.name} profile={profiles[row.uid]}/></td><td className="p-2 leading-6">{row.items.map(i=>i.label).join('、')||'移出本批次'}</td><td className="p-2">{{new:'新增',changed:'内容变化',removed:'移出',unchanged:'不变'}[row.change]}</td></tr>)}</tbody></table></div>{preview.rows.length>100&&<p className="text-xs text-muted-foreground">已校验全部用户，此处展示前100人，变化优先。</p>}<Button disabled={busy||(preview.total===0&&batch.published)} onClick={()=>void sync(true)}>{preview.total===0?'清空这批名单':'保存范围，下一步'}</Button></section>}
  </div>}
  {tab==='dispatch'&&(dirty?<div className="space-y-4 py-4"><p className="text-sm">{metaDirty?'基础设置还有修改没保存。':'你刚选的名单还没保存。'}保存后就可以预览并发送私信。</p><Button onClick={()=>switchTab(metaDirty?'settings':'sources')}>{metaDirty?'去保存设置':'去保存名单'}</Button></div>:<Delivery issue={issue} onStateChange={setDispatchState} onPublish={publish} publishBusy={busy}/>)}
 </section>
}
export default function VoyageBatches(){
 const [batches,setBatches]=useState<Batch[]>([]),[rosters,setRosters]=useState<Roster[]>([]),[selected,setSelected]=useState(0),[loading,setLoading]=useState(true),[error,setError]=useState(''),[creating,setCreating]=useState(false),[title,setTitle]=useState(''),[busy,setBusy]=useState(false),[editorState,setEditorState]=useState({busy:false,dirty:false});
 const load=useCallback(async()=>{const result=await Promise.all([voyageRequest<Batch[]>('/api/voyage/batches'),voyageRequest<Roster[]>('/api/voyage/rosters')]);setBatches(result[0]);setRosters(result[1]);setSelected(id=>id||Number(new URLSearchParams(window.location.search).get('batch'))||result[0][0]?.id||0)},[]);
 useEffect(()=>{void load().catch(e=>setError(e.message)).finally(()=>setLoading(false))},[load]);
 useEffect(()=>{const refresh=()=>{if(!busy&&!editorState.busy&&!editorState.dirty&&!creating)void load().catch(e=>setError(e.message))};window.addEventListener('focus',refresh);return()=>window.removeEventListener('focus',refresh)},[load,busy,editorState.busy,editorState.dirty,creating]);
 useEffect(()=>{const warn=(e:BeforeUnloadEvent)=>{if(editorState.busy||editorState.dirty||title){e.preventDefault();e.returnValue=''}};window.addEventListener('beforeunload',warn);return()=>window.removeEventListener('beforeunload',warn)},[editorState,title]);
 const batch=batches.find(b=>b.id===selected),leave=async()=>!busy&&!editorState.busy&&(!editorState.dirty||await confirmDiscard('有未保存的批次内容，确定离开吗？'));
 useSidebarNavigationGuard(()=>leave());
 const create=async()=>{if(!(await leave()))return;setBusy(true);setError('');try{const b=await voyageRequest<Batch>('/api/voyage/batches','POST',{title});await load();setSelected(b.id);setCreating(false);setTitle('')}catch(e){setError((e as Error).message)}finally{setBusy(false)}};
 return <div className="mx-auto w-full max-w-[1440px]"><header className="mb-5 flex items-center justify-between gap-4"><h1 className="text-2xl font-semibold">地址收集</h1><Button disabled={busy||editorState.busy} onClick={()=>setCreating(!creating)}>{creating?'取消新建':'新建批次'}</Button></header>
  <p className="mb-6 text-sm leading-6 text-muted-foreground">设置收集时间和范围，再发私信请用户登记地址。礼物准备好后，再去发货管理分次寄送。</p>
  {error&&<div role="alert" className="mb-4 text-sm text-destructive">{error}<Button variant="ghost" onClick={()=>void load().then(()=>setError('')).catch(e=>setError(e.message))}>重新加载</Button></div>}
  {creating&&<form className="mb-6 flex flex-wrap items-end gap-3 border-y border-border py-5" onSubmit={e=>{e.preventDefault();void create()}}><label className="min-w-56 flex-1 space-y-2 text-sm">批次名称<input className={voyageField} required maxLength={80} disabled={busy} value={title} placeholder="例如：2026年9月地址登记" onChange={e=>setTitle(e.target.value)}/></label><Button type="submit" disabled={busy||editorState.busy||!title.trim()}>创建批次</Button></form>}
  {loading?<p role="status">正在加载批次…</p>:<div className="grid gap-8 lg:grid-cols-[15rem_minmax(0,1fr)]"><aside className="max-h-80 space-y-1 overflow-auto lg:max-h-[70vh]" aria-label="选择收集批次">{batches.map(b=><button type="button" key={b.id} disabled={busy||editorState.busy} aria-current={b.id===selected?'true':undefined} className={`w-full rounded-md p-3 text-left text-sm focus-visible:outline-2 focus-visible:outline-ring ${b.id===selected?'bg-secondary font-medium':'hover:bg-secondary/50'}`} onClick={async()=>{if(await leave()){setSelected(b.id);setEditorState({busy:false,dirty:false})}}}>{b.title}<span className="mt-1 block text-xs text-muted-foreground">{b.published?(b.closed?'收集已结束':'前台可见'):'草稿'}</span></button>)}</aside>{batch?<BatchEditor key={batch.id} batch={batch} rosters={rosters} onReload={load} onState={setEditorState}/>:<p className="py-8 text-sm text-muted-foreground">先创建一个收集批次，再选择月份或活动名单。</p>}</div>}
 </div>
}
