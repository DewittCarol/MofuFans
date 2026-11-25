'use client';
import {confirmAction,confirmDiscard} from '@/lib/site-dialog';
import {useEffect,useState,useRef,type ReactNode} from 'react';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import {BilibiliPerson} from '@/components/bilibili-person';
import {BilibiliProfileSync} from '@/components/bilibili-profile-sync';
import {profileName} from '@/lib/bilibili-profiles';
import {Button} from '@/components/ui/button';
import {Tabs,TabsList,TabsTrigger,TabsContent} from '@/components/ui/tabs';
import {VoyageTierBadge} from '@/components/voyage-tier-badge';
import {VoyageBilibiliImport} from '@/components/voyage-bilibili-import';
import {RosterCategoryTabs,groupRosterMonths,type RosterCategory} from '@/components/voyage-roster-browser';
import {voyageRequest,voyageTiers,type VoyageTier} from '@/lib/voyage';
import {voyageField} from '@/lib/voyage-recipients';
export interface Roster {id:number;title:string;category:'monthly'|'event';month:string;version:number}
interface Member {id:number;uid:string;name:string;tier:VoyageTier|'';disabled:boolean;version:number}
interface SyncResult {collections:number;parcels:number;cancelled:number;shipped:number}
interface Preview {added:number;skipped:number;conflicts:number;sync?:SyncResult;rows:{line:number;uid:string;tier:string;name:string;status:string}[]}
function syncNotice(result?:SyncResult){
 if(!result)return '';
 return [result.collections?`已自动更新 ${result.collections} 个收集批次。`:'',result.parcels?`已调整 ${result.parcels} 个未寄出的包裹。`:'',result.cancelled?`已取消 ${result.cancelled} 个不再有礼物的包裹。`:'',result.shipped?'已寄出的记录已保留。':''].join('');
}
const labels:Record<string,string>={new:'新增',existing:'保留原记录',duplicate:'文件内重复',conflict:'档位冲突'};
const tierOrder:Record<VoyageTier|'',number>={captain:0,admiral:1,governor:2,'':3};
function compareMembers(a:Member,b:Member){
 const left=BigInt(a.uid),right=BigInt(b.uid);
 if(left!==right)return left<right?-1:1;
 return tierOrder[a.tier]-tierOrder[b.tier]||a.id-b.id;
}

function MemberTierTabs({monthly,value,onChange,disabled,rows,children}:{monthly:boolean;value:string;onChange:(value:string)=>void;disabled:boolean;rows:Member[];children:ReactNode}){
 if(!monthly)return <>{children}</>;
 const options=[{id:'all',name:'全部'},...voyageTiers,...(value===''||rows.some(r=>!r.tier)?[{id:'',name:'未设置档位'}]:[])];
 return <Tabs value={value} onValueChange={onChange}>
   <TabsList aria-label="按大航海档位筛选" className="mb-3 max-w-full flex-wrap justify-start gap-y-1 group-data-[orientation=horizontal]/tabs:h-auto min-h-10">
    {options.map(t=><TabsTrigger key={t.id} value={t.id} disabled={disabled} className="min-h-9 px-3">
     {t.id&&t.id!=='all'?<VoyageTierBadge tier={t.id as VoyageTier}/>:t.name}
     <span className="tabular-nums text-muted-foreground">{t.id==='all'?rows.length:rows.filter(r=>r.tier===t.id).length}</span>
    </TabsTrigger>)}
   </TabsList>
  <TabsContent value={value}>{children}</TabsContent>
 </Tabs>;
}

function Members({roster,onChange,onDirty}:{roster:Roster;onChange:()=>Promise<void>;onDirty:(v:{busy:boolean;dirty:boolean})=>void}){
 const [rows,setRows]=useState<Member[]>([]),[loading,setLoading]=useState(true),[localBusy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [biliOpen,setBiliOpen]=useState(false),[biliBusy,setBiliBusy]=useState(false);const busy=localBusy||biliBusy;
 const [text,setText]=useState(''),[preview,setPreview]=useState<Preview|null>(null),[showImport,setShowImport]=useState(false),[query,setQuery]=useState(''),[filter,setFilter]=useState('all'),[page,setPage]=useState(1),[editing,setEditing]=useState<Member|null>(null);
 const [tier,setTier]=useState('all'),[pageSize,setPageSize]=useState(30);
 const [selectedIDs,setSelectedIDs]=useState<Set<number>>(new Set());
 const active=useRef(true),running=useRef(false);const path=`/api/voyage/rosters/${roster.id}/members`;
 const load=async()=>{setLoading(true);try{const data=await voyageRequest<Member[]>(path);if(active.current)setRows(data)}catch(e){if(active.current)setError((e as Error).message)}finally{if(active.current)setLoading(false)}};
 useEffect(()=>{const params=new URLSearchParams(window.location.search);if(Number(params.get('roster'))===roster.id)setQuery(params.get('uid')||'');active.current=true;void load();return()=>{active.current=false}},[path]); // eslint-disable-line react-hooks/exhaustive-deps
 useEffect(()=>{onDirty({busy,dirty:!!text.trim()||!!editing});return()=>onDirty({busy:false,dirty:false})},[busy,text,editing,onDirty]);
 const run=async(fn:()=>Promise<void>)=>{if(running.current)return;running.current=true;setBusy(true);setError('');setNotice('');try{await fn()}catch(e){setError((e as Error).message)}finally{running.current=false;setBusy(false)}};
 const importRows=(confirm:boolean)=>run(async()=>{const data=await voyageRequest<Preview>(`${path}/${confirm?'import':'preview'}`,'POST',{text,version:roster.version});if(confirm){setText('');setPreview(null);setShowImport(false);setSelectedIDs(new Set());setNotice(`已添加 ${data.added} 条记录，跳过 ${data.skipped} 条重复记录。${syncNotice(data.sync)}`);await load();await onChange()}else setPreview(data)});
 const previewBilibili=async(value:string)=>{const data=await voyageRequest<Preview>(`${path}/preview`,'POST',{text:value,version:roster.version});if(active.current){setText(value);setPreview(data);setShowImport(true);setBiliOpen(false);setError('');setNotice('请核对下方预览，确认后才会加入名单。')}};
 const save=(row:Member)=>run(async()=>{
  const statusChanged=rows.find(r=>r.id===row.id)?.disabled!==row.disabled;
  const saved=await voyageRequest<Member&{sync:SyncResult}>(`${path}/${row.id}`,'PUT',row);
  setRows(items=>items.map(r=>r.id===saved.id?saved:r));setEditing(null);setSelectedIDs(new Set());
  const result=statusChanged?(saved.disabled?'已排除此条。':'已恢复此条。'):'已保存。';
  setNotice(result+syncNotice(saved.sync));
  await onChange();
 });
 const remove=async(items:Member[])=>{
  if(!items.length||busy||editing)return;
  const target=items.length===1?`UID ${items[0].uid}${roster.category==='monthly'?` 的${voyageTiers.find(t=>t.id===items[0].tier)?.name||'未设置档位'}记录`:' 的记录'}`:`选中的 ${items.length} 条记录`;
  if(!(await confirmAction(`将删除${target}。\n关联的地址收集、未寄包裹和电子礼物资格会自动更新，已寄出的记录保留。删除后不能撤销。`,{title:items.length===1?'删除名单记录？':'删除所选记录？',confirmLabel:items.length===1?'删除记录':`删除 ${items.length} 条记录`,destructive:true})))return;
  void run(async()=>{
   const result=await voyageRequest<{deleted:number;sync:SyncResult}>(`${path}/delete`,'POST',{version:roster.version,items:items.map(r=>({id:r.id,version:r.version}))});
   const removed=new Set(items.map(r=>r.id));setRows(current=>current.filter(r=>!removed.has(r.id)));setSelectedIDs(new Set());setPreview(null);
   setNotice(`已删除 ${result.deleted} 条记录。${syncNotice(result.sync)}`);await onChange();
  });
 };
 const profiles=useBilibiliProfiles([...rows.map(r=>r.uid),...(preview?.rows.map(r=>r.uid)||[])]);
 const matches=rows.filter(r=>(filter==='all'||(filter==='disabled'?r.disabled:!r.disabled))&&`${r.uid} ${profileName(r.uid,r.name,profiles[r.uid])}`.toLowerCase().includes(query.trim().toLowerCase()));
 const filtered=matches.filter(r=>roster.category!=='monthly'||tier==='all'||r.tier===tier).sort(compareMembers);
 const pages=Math.max(1,Math.ceil(filtered.length/pageSize)),current=Math.min(page,pages);
 const pageRows=filtered.slice((current-1)*pageSize,current*pageSize);
 const selectedRows=rows.filter(r=>selectedIDs.has(r.id));
 const pageSelected=pageRows.length>0&&pageRows.every(r=>selectedIDs.has(r.id));
 useEffect(()=>{setSelectedIDs(new Set())},[query,filter,tier,pageSize]);
 const selectPage=(checked:boolean)=>setSelectedIDs(ids=>{const next=new Set(ids);for(const r of pageRows){if(checked)next.add(r.id);else next.delete(r.id)}return next});
 useEffect(()=>{setPage(p=>Math.min(p,pages))},[pages]);
 return <section aria-label={roster.title} className="min-w-0">
  <div className="mb-5 flex flex-wrap items-center gap-3"><div className="mr-auto"><h2 className="text-xl font-semibold">{roster.title}</h2><p className="mt-1 text-sm text-muted-foreground">{roster.category==='monthly'?`${roster.month} · 月度大航海`:'活动 / 抽奖'} · {new Set(rows.map(r=>r.uid)).size} 人{roster.category==='monthly'?` · ${rows.length} 条上舰记录`:null}</p></div><Button variant="outline" disabled={busy||loading} onClick={()=>void load()}>刷新名单</Button>{roster.category==='monthly'&&<Button variant="outline" disabled={busy||!!editing} onClick={async()=>{if(!biliOpen&&text.trim()&&!await confirmDiscard('还有未导入的内容，要放弃并从 B 站重新获取吗？'))return;setText('');setPreview(null);setShowImport(false);setBiliOpen(!biliOpen)}}>{biliOpen?'收起 B 站导入':'从 B 站导入名单'}</Button>}<Button disabled={busy||!!editing} onClick={()=>{setBiliOpen(false);setShowImport(!showImport)}}>{showImport?'收起导入':'导入文件 / 粘贴'}</Button><BilibiliProfileSync uids={rows.map(r=>r.uid)} disabled={busy||loading||!!editing}/></div>
  {biliOpen&&<VoyageBilibiliImport rosterID={roster.id} month={roster.month} onReady={previewBilibili} onBusy={setBiliBusy} onClose={()=>setBiliOpen(false)}/>}
  {showImport&&<section className="mb-6 space-y-4 border-y border-border py-5" aria-label="导入名单">
   <h3 className="font-medium">粘贴名单或选择文件</h3><p className="text-sm leading-6 text-muted-foreground">{roster.category==='monthly'?'列顺序：UID、档位，昵称可不填。导入后自动获取 B 站头像和昵称。同一 UID 上过多个档位，就分行填写。也支持 BiliMessenger 导出的名单。':'每行一个 UID 即可，头像和昵称会自动获取；也可以在 UID 后填写自定义昵称。'} 支持 CSV、TXT 或从 Excel 粘贴，每次最多 2000 行。</p>
   <textarea className={voyageField} aria-label="名单内容" rows={6} value={text} disabled={busy} placeholder={roster.category==='monthly'?'UID,档位\n123456789,舰长':'UID\n123456789'} onChange={e=>{setText(e.target.value);setPreview(null)}}/>
   <div className="flex flex-wrap items-center gap-3"><label className="cursor-pointer text-sm text-primary underline underline-offset-4">选择文件<input type="file" accept=".csv,.tsv,.txt" className="sr-only" disabled={busy} onChange={e=>{const f=e.target.files?.[0];e.target.value='';if(f)void run(async()=>{if(f.size>1024*1024)throw Error('文件最多1MB');const data=await f.arrayBuffer();let value;try{value=new TextDecoder('utf-8',{fatal:true}).decode(data)}catch{value=new TextDecoder('gb18030',{fatal:true}).decode(data)}setText(value);setPreview(null)})}}/></label><Button variant="outline" disabled={busy||!text.trim()} onClick={()=>void importRows(false)}>预览导入</Button></div>
   {preview&&<div className="space-y-3"><p className="text-sm">新增 {preview.added} 条记录，跳过 {preview.skipped} 条重复记录。</p><div className="max-h-64 overflow-auto"><table className="w-full text-left text-sm"><thead><tr><th className="p-2">UID</th><th className="p-2">昵称</th><th className="p-2">档位</th><th className="p-2">结果</th></tr></thead><tbody>{[...preview.rows].sort((a,b)=>Number(b.status==='conflict')-Number(a.status==='conflict')).slice(0,100).map(r=><tr key={r.line} className="border-t border-border"><td className="p-2">{r.uid}</td><td className="p-2">{r.name||profiles[r.uid]?.name||'自动获取中…'}</td><td className="p-2">{voyageTiers.find(t=>t.id===r.tier)?.name||'未设置'}</td><td className="p-2">{labels[r.status]}</td></tr>)}</tbody></table></div>{preview.rows.length>100&&<p className="text-xs text-muted-foreground">已校验全部记录，此处展示前100条。</p>}<Button disabled={busy||preview.conflicts>0||preview.added===0} onClick={()=>void importRows(true)}>确认导入 {preview.added} 条记录</Button></div>}
  </section>}
  <div className="mb-4 flex flex-wrap gap-3"><input className={`${voyageField} min-w-48 flex-1`} aria-label="搜索 UID 或昵称" placeholder="搜索 UID 或昵称" value={query} disabled={!!editing} onChange={e=>{setQuery(e.target.value);setPage(1)}}/><select className={`${voyageField} sm:max-w-36`} aria-label="本名单记录状态" value={filter} disabled={!!editing} onChange={e=>{setFilter(e.target.value);setPage(1)}}><option value="all">全部记录</option><option value="active">正常参与</option><option value="disabled">已排除</option></select></div>
  <MemberTierTabs monthly={roster.category==='monthly'} value={tier} disabled={busy||!!editing} rows={matches} onChange={value=>{setTier(value);setPage(1)}}>
  <p className="mb-3 text-xs leading-6 text-muted-foreground">名单变动会自动更新关联的地址收集、待寄礼物和电子礼物。已寄出的记录保留。</p>
  {error&&<p role="alert" className="mb-3 text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="mb-3 text-sm leading-6 text-primary">{notice}</p>}
  {!loading&&filtered.length>0&&<div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-border py-3 text-sm">
   <label className="flex min-h-9 items-center gap-2"><input type="checkbox" className="size-4 accent-primary" checked={pageSelected} ref={el=>{if(el)el.indeterminate=!pageSelected&&pageRows.some(r=>selectedIDs.has(r.id))}} disabled={busy||!!editing} onChange={e=>selectPage(e.target.checked)}/>全选本页</label>
   {selectedRows.length>0&&<><span className="text-muted-foreground">已选 {selectedRows.length} 条</span>{pageSelected&&filtered.length>selectedRows.length&&filtered.length<=10000&&<Button variant="link" disabled={busy||!!editing} onClick={()=>setSelectedIDs(new Set(filtered.map(r=>r.id)))}>选择全部 {filtered.length} 条筛选结果</Button>}<Button variant="ghost" disabled={busy||!!editing} onClick={()=>setSelectedIDs(new Set())}>取消选择</Button></>}
   <Button variant="outline" size="sm" className="ml-auto text-destructive hover:text-destructive" disabled={busy||!!editing||!selectedRows.length||selectedRows.length>10000} onClick={()=>remove(selectedRows)}>删除所选{selectedRows.length?`（${selectedRows.length}）`:''}</Button>
  </div>}
  {loading?<p role="status">正在加载名单…</p>:!filtered.length?<p className="py-10 text-sm text-muted-foreground">{rows.length?'没有匹配的用户。':'名单还是空的，导入 UID 后即可用于收集批次。'}</p>:<ul className="divide-y divide-border">
   {pageRows.map(r=><li key={r.id} className="py-4">
    <div className="flex flex-wrap items-center gap-x-6 gap-y-3">
     <div className="flex min-w-0 flex-1 basis-64 items-center gap-3">
     <input type="checkbox" className="size-4 shrink-0 accent-primary" aria-label={`选择 UID ${r.uid}${roster.category==='monthly'?` ${voyageTiers.find(t=>t.id===r.tier)?.name||'未设置档位'}`:''}`} checked={selectedIDs.has(r.id)} disabled={busy||!!editing} onChange={e=>setSelectedIDs(ids=>{const next=new Set(ids);if(e.target.checked)next.add(r.id);else next.delete(r.id);return next})}/>
     <div className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 sm:grid-cols-[minmax(0,20rem)_minmax(5rem,1fr)]">
      <BilibiliPerson uid={r.uid} name={r.name} profile={profiles[r.uid]}/>
      <div className="space-y-1">
       {roster.category==='monthly'&&<VoyageTierBadge tier={r.tier}/>}
       {r.disabled&&<p className="text-xs text-muted-foreground">已排除</p>}
      </div>
     </div>
     </div>
     <div className="ml-auto flex items-center gap-2">
      <Button variant="outline" size="sm" disabled={busy||!!editing} onClick={()=>{setBiliOpen(false);setEditing({...r})}}>编辑</Button>
      <Button variant="ghost" size="sm" disabled={busy||!!editing} title={r.disabled?'恢复本名单中的这条记录':'仅排除本名单这条记录，不影响同 UID 的其他档位或月份'} onClick={()=>void save({...r,disabled:!r.disabled})}>{r.disabled?'恢复此条':'排除此条'}</Button>
      <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive" disabled={busy||!!editing} onClick={()=>remove([r])}>删除</Button>
     </div>
    </div>
    {editing?.id===r.id&&<form className="mt-4 flex flex-wrap items-end gap-3" onSubmit={e=>{e.preventDefault();void save(editing)}}>
     <label className="space-y-1 text-sm">自定义昵称（选填）<input placeholder="留空使用 B 站昵称" className={voyageField} maxLength={80} disabled={busy} value={editing.name} onChange={e=>setEditing({...editing,name:e.target.value})}/></label>
     {roster.category==='monthly'&&<label className="space-y-1 text-sm">档位<select className={voyageField} disabled={busy} value={editing.tier} onChange={e=>setEditing({...editing,tier:e.target.value as VoyageTier|''})}><option value="">未设置</option>{voyageTiers.map(t=><option key={t.id} value={t.id}>{t.name}</option>)}</select></label>}
     <Button disabled={busy} type="submit">保存</Button><Button disabled={busy} variant="ghost" type="button" onClick={()=>setEditing(null)}>取消</Button>
    </form>}
   </li>)}
  </ul>}
  {!loading&&<nav aria-label="名单分页" className="mt-5 flex flex-wrap items-center justify-center gap-x-4 gap-y-3 border-t border-border pt-5 text-sm">
   <p className="tabular-nums text-muted-foreground" role="status">共 {filtered.length} 条</p>
   <div className="flex items-center gap-3">
    <Button variant="outline" size="sm" disabled={busy||!!editing||current===1} onClick={()=>setPage(current-1)}>上一页</Button>
    <span className="whitespace-nowrap tabular-nums">第 {current} / {pages} 页</span>
    <Button variant="outline" size="sm" disabled={busy||!!editing||current===pages} onClick={()=>setPage(current+1)}>下一页</Button>
   </div>
   <label className="flex items-center gap-2 whitespace-nowrap">每页<select className={`${voyageField} !w-auto`} aria-label="每页条数" value={pageSize} disabled={busy||!!editing} onChange={e=>{setPageSize(Number(e.target.value));setPage(1)}}>{[10,30,50,100].map(size=><option key={size} value={size}>{size} 条</option>)}</select></label>
  </nav>}
  </MemberTierTabs>
 </section>
}
export default function VoyageRosters(){
 const [rosters,setRosters]=useState<Roster[]>([]),[selected,setSelected]=useState(0),[loading,setLoading]=useState(true),[error,setError]=useState('');
 const [creating,setCreating]=useState(false),[busy,setBusy]=useState(false),[editorState,setEditorState]=useState({busy:false,dirty:false});
 const [category,setCategory]=useState<RosterCategory>('monthly'),[year,setYear]=useState('all');
 const [draft,setDraft]=useState({title:'',category:'monthly',month:new Date().toISOString().slice(0,7)});
 const load=async()=>{try{const data=await voyageRequest<Roster[]>('/api/voyage/rosters');setRosters(data);setError('')}catch(e){setError((e as Error).message)}finally{setLoading(false)}};
 useEffect(()=>{void (async()=>{setLoading(true);try{const data=await voyageRequest<Roster[]>('/api/voyage/rosters');setRosters(data);const id=Number(new URLSearchParams(window.location.search).get('roster'));const target=data.find(r=>r.id===id);if(target){setCategory(target.category);setSelected(target.id)}}catch(e){setError((e as Error).message)}finally{setLoading(false)}})()},[]);
 useEffect(()=>{const warn=(e:BeforeUnloadEvent)=>{if(editorState.busy||editorState.dirty||creating){e.preventDefault();e.returnValue=''}};window.addEventListener('beforeunload',warn);return()=>window.removeEventListener('beforeunload',warn)},[editorState,creating]);
 const leave=async()=>!busy&&!editorState.busy&&(!editorState.dirty||await confirmDiscard('还有未保存的名单内容，要放弃修改吗？'));
 const visible=rosters.filter(r=>r.category===category&&(year==='all'||r.month.startsWith(year)));
 const chosen=visible.find(r=>r.id===selected)||visible[0];
 const create=async()=>{if(!(await leave()))return;setBusy(true);setError('');try{const row=await voyageRequest<Roster>('/api/voyage/rosters','POST',draft);await load();setCategory(row.category);setYear('all');setSelected(row.id);setCreating(false);setDraft({...draft,title:''})}catch(e){setError((e as Error).message)}finally{setBusy(false)}};
 return <div className="mx-auto w-full max-w-[1440px]">
  <header className="mb-5 flex items-center justify-between gap-4"><h1 className="text-2xl font-semibold">名单管理</h1><Button disabled={busy||editorState.busy} onClick={()=>{setDraft({...draft,category});setCreating(!creating)}}>{creating?'取消新建':'新建名单'}</Button></header>
  {error&&<div role="alert" className="mb-4 text-sm text-destructive">{error}<Button variant="ghost" onClick={()=>void load()}>重新加载</Button></div>}
  {creating&&<form className="mb-6 grid gap-4 border-y border-border py-5 sm:grid-cols-3" onSubmit={e=>{e.preventDefault();void create()}}>
   <label className="space-y-2 text-sm">名单类别<select className={voyageField} disabled={busy} value={draft.category} onChange={e=>setDraft({...draft,category:e.target.value})}><option value="monthly">大航海</option><option value="event">活动 / 抽奖</option></select></label>
   <label className="space-y-2 text-sm">所属月份{draft.category==='event'?'（选填）':''}<input type="month" className={voyageField} disabled={busy} required={draft.category==='monthly'} value={draft.month} onChange={e=>setDraft({...draft,month:e.target.value})}/></label>
   <label className="space-y-2 text-sm">名单名称<input className={voyageField} disabled={busy} required maxLength={80} value={draft.title} placeholder={draft.category==='monthly'?'例如：2026年8月大航海':'例如：8月转盘抽奖'} onChange={e=>setDraft({...draft,title:e.target.value})}/></label>
   <Button className="justify-self-start" disabled={busy||editorState.busy} type="submit">创建名单</Button>
  </form>}
  <RosterCategoryTabs value={category} disabled={busy||editorState.busy} onChange={async value=>{if(await leave()){setCategory(value);setYear("all");setSelected(0)}}}>
   {loading?<p role="status">正在加载名单…</p>:<div className="grid gap-8 lg:grid-cols-[17rem_minmax(0,1fr)]">
    <aside className="space-y-4" aria-label="按月份选择名单">
     <label className="block"><span className="sr-only">名单年份</span><select className={voyageField} value={year} disabled={busy||editorState.busy} onChange={async e=>{const value=e.target.value;if(await leave()){setYear(value);setSelected(0)}}}><option value="all">全部年份</option>{[...new Set(rosters.filter(r=>r.category===category).map(r=>r.month.slice(0,4)).filter(Boolean))].sort().reverse().map(y=><option key={y}>{y}</option>)}</select></label>
     <div className="max-h-96 space-y-5 overflow-y-auto lg:max-h-[65vh]">{groupRosterMonths(visible).map(group=><section key={group.month} aria-label={group.title}>
      <h2 className="mb-2 px-3 text-sm font-semibold">{group.title}</h2>
      <div className="space-y-1">{group.rows.map(r=><button key={r.id} type="button" disabled={busy||editorState.busy} aria-current={chosen?.id===r.id?'true':undefined} className={`w-full rounded-md px-3 py-3 text-left text-sm break-words focus-visible:outline-2 focus-visible:outline-ring ${chosen?.id===r.id?'bg-secondary font-medium':'hover:bg-secondary/50'}`} onClick={async()=>{if(r.id!==chosen?.id&&await leave())setSelected(r.id)}}>{r.title}</button>)}</div>
     </section>)}{!visible.length&&<p className="px-3 py-5 text-sm text-muted-foreground">还没有{category==='monthly'?'大航海':'活动'}名单。</p>}</div>
    </aside>
    {chosen?<Members key={chosen.id} roster={chosen} onChange={load} onDirty={setEditorState}/>:<p className="py-8 text-sm text-muted-foreground">点击“新建名单”，再导入用户 UID。</p>}
   </div>}
  </RosterCategoryTabs>
 </div>;
}
