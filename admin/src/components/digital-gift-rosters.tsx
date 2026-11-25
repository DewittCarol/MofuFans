'use client';
import {useEffect,useState} from 'react';
import {Button} from '@/components/ui/button';
import {giftAPI,giftField} from '@/lib/digital-gifts';
import {voyageRequest} from '@/lib/voyage';
import {RosterCategoryTabs,groupRosterMonths,type RosterCategory} from '@/components/voyage-roster-browser';
type Roster={id:number;title:string;category:'monthly'|'event';month:string;version:number};
export function GiftRosterPicker({individual,giftID,version,disabled,onSaved,onStateChange}:{individual:boolean;giftID:number;version:number;disabled:boolean;onSaved:(version:number)=>Promise<void>;onStateChange:(state:{busy:boolean;dirty:boolean})=>void}){
 const [rosters,setRosters]=useState<Roster[]>([]),[selected,setSelected]=useState<number[]>([]),[saved,setSaved]=useState<number[]>([]);
 const [loading,setLoading]=useState(true),[busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [category,setCategory]=useState<RosterCategory>('monthly'),[query,setQuery]=useState(''),[retry,setRetry]=useState(0);
 const dirty=JSON.stringify([...selected].sort())!==JSON.stringify([...saved].sort());
 useEffect(()=>{onStateChange({busy,dirty});return()=>onStateChange({busy:false,dirty:false})},[busy,dirty,onStateChange]);
 useEffect(()=>{
  let live=true;setLoading(true);setError('');
  Promise.all([voyageRequest<Roster[]>('/api/voyage/rosters'),giftAPI<{roster_ids:number[]}>(`/${giftID}/rosters`)])
   .then(([all,binding])=>{if(live){setRosters(all);setSelected(binding.roster_ids);setSaved(binding.roster_ids)}})
   .catch(e=>{if(live)setError(e.message)}).finally(()=>{if(live)setLoading(false)});
  return()=>{live=false};
 },[giftID,retry]);
 useEffect(()=>{
  const refresh=()=>{void voyageRequest<Roster[]>('/api/voyage/rosters').then(setRosters).catch(()=>{})};
  window.addEventListener('focus',refresh);return()=>window.removeEventListener('focus',refresh);
 },[]);
 async function save(){
  setBusy(true);setError('');setNotice('');
  try{const result=await giftAPI<{version:number;roster_ids:number[]}>(`/${giftID}/rosters`,'PUT',{version,roster_ids:selected});setSaved(result.roster_ids);setNotice(`关联已保存。名单有变化时，${individual ? '领取名单' : '通知名单'}会自动更新。`);await onSaved(result.version)}
  catch(e){setError((e as Error).message)}finally{setBusy(false)}
 }
 const blocked=disabled||busy;
 const shown=rosters.filter(r=>r.category===category&&`${r.month} ${r.title}`.includes(query.trim()));
 return <section className="space-y-4">
  <div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-semibold">关联名单</h3><a href="/admin/dashboard/voyage/roster/" target="_blank" rel="noopener noreferrer" className="text-sm text-primary underline underline-offset-4">上传 / 管理名单</a></div>
  <p className="text-sm leading-6 text-muted-foreground">点击“上传 / 管理名单”导入成员，回到这里勾选并保存关联。同一个 UID 会自动合并，之后增删成员会自动同步。{individual ? '每人生成一份独立密码。' : '名单用于批量通知，所有人收到同一个密码；停用通知不会使统一密码失效。'}</p>
  {error&&<p role="alert" className="text-sm text-destructive">{error}<Button type="button" variant="link" size="sm" disabled={blocked} onClick={()=>setRetry(n=>n+1)}>重新加载</Button></p>}
  {notice&&<p role="status" className="text-sm text-primary">{notice}</p>}
  {loading?<p role="status" className="text-sm text-muted-foreground">正在读取名单…</p>:<>
   <input className={giftField} aria-label="搜索要关联的名单" placeholder="搜索月份或名单名称" value={query} disabled={blocked} onChange={e=>setQuery(e.target.value)}/>
   <RosterCategoryTabs value={category} onChange={setCategory} disabled={blocked}>
    <div className="max-h-80 space-y-4 overflow-y-auto pr-2">{groupRosterMonths(shown).map(group=><section key={group.month}><h4 className="mb-1 text-xs font-medium text-muted-foreground">{group.title}</h4>{group.rows.map(roster=><label key={roster.id} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-md px-2 py-2 text-sm hover:bg-secondary"><input type="checkbox" className="size-4 shrink-0 accent-primary" disabled={blocked} checked={selected.includes(roster.id)} onChange={e=>{setSelected(ids=>e.target.checked?[...ids,roster.id]:ids.filter(id=>id!==roster.id));setNotice('')}}/><span className="break-words">{roster.title}</span></label>)}</section>)}{!shown.length&&<p className="py-5 text-sm text-muted-foreground">{query?'没有匹配的名单。':'还没有这类名单，请先到名单管理中创建。'}</p>}</div>
   </RosterCategoryTabs>
   <p className="text-sm text-muted-foreground">已选 {selected.length} 张名单{selected.length?`：${rosters.filter(r=>selected.includes(r.id)).map(r=>r.title).join('、')}`:(individual ? '。保存后，这份礼物将没有可领取的成员。' : '。保存后，将没有批量通知对象，仍可手动填写 UID 发私信。')}</p>
   <div className="flex flex-wrap gap-3"><Button type="button" disabled={blocked||!dirty} onClick={()=>void save()}>保存关联</Button>{dirty&&<Button type="button" variant="ghost" disabled={blocked} onClick={()=>setSelected(saved)}>撤销修改</Button>}</div>
  </>}
 </section>;
}
