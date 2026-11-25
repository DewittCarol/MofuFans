'use client';
import {confirmDiscard} from '@/lib/site-dialog';
import {useState,useEffect,useRef} from 'react';
import {X,Check,Truck} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Select,SelectTrigger,SelectContent,SelectValue,SelectItem} from '@/components/ui/select';
import {Label} from '@/components/ui/label';
import {voyageRequest} from '@/lib/voyage';
import {blankAddress,claimPayload,memberClaimPayload,longTermSetDisabledReason,refreshClaimRecord,usesLongTermAddress,type ClaimAction,type VoyageAddress,type VoyageDelivery} from '@/lib/voyage-claim';
const field='w-full rounded-md border border-input bg-background px-3 py-2 text-sm leading-6 focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-60';
export function VoyageClaim({issueID,memberUID,mode,onModeChange:setMode,onClose,onStateChange}:{issueID:number;memberUID?:string;mode:'address'|'shipping';onModeChange:(mode:'address'|'shipping')=>void;onClose:()=>void;onStateChange?:(v:{busy:boolean;dirty:boolean})=>void}) {
 const [uid,setUID]=useState(''),[code,setCode]=useState('');
 const [selectedHistory,setSelectedHistory]=useState('');
 const [record,setRecord]=useState<VoyageDelivery|null>(null),[address,setAddress]=useState<VoyageAddress>({...blankAddress});
 const [busy,setBusy]=useState(false),[error,setError]=useState(''),[notice,setNotice]=useState('');
 const [pendingAction,setPendingAction]=useState<ClaimAction|null>(null),[longTermError,setLongTermError]=useState(''),[longTermNotice,setLongTermNotice]=useState('');
 const editable=record?.can_edit_address??(record?.collecting!==false&&!record?.shipped_at);
 const running=useRef(false),lifetime=useRef(0),dirty=!!record&&JSON.stringify(address)!==JSON.stringify(record.address||blankAddress);
 useEffect(()=>{lifetime.current++;running.current=false;setRecord(null);setAddress({...blankAddress});setCode('');setError('');setNotice('');setLongTermError('');setLongTermNotice('');return()=>{lifetime.current++;running.current=false}},[issueID,memberUID]);
 const longTermEnabled=!!record?.long_term?.enabled,setDisabledReason=longTermSetDisabledReason(record,dirty);
 useEffect(()=>{onStateChange?.({busy,dirty});return()=>onStateChange?.({busy:false,dirty:false})},[busy,dirty,onStateChange]);
 const leave=async()=>!running.current&&(!dirty||await confirmDiscard('地址修改尚未保存，确定放弃吗？'));
 const request=async(action:ClaimAction,bookEntryID?:number)=>{
  if(running.current)return;
  if (!memberUID && (!uid.trim() || !code.trim())) {setError('请输入 B 站 UID 和私信认证码');return;}
  const preferenceAction=action==='set_long_term'||action==='cancel_long_term'||action==='set_book_default';
  if(preferenceAction){setLongTermError('');setLongTermNotice('')}else{setError('');setNotice('')}
  if(action==='set_long_term'&&setDisabledReason){setLongTermError(setDisabledReason);return}
  if(action==='set_book_default'&&dirty){setLongTermError('收货地址有未保存的修改，请先保存，再设置常用地址。');return}
  const ticket=lifetime.current;running.current=true;setBusy(true);setPendingAction(action);
  try{
    if(action==='cancel_long_term'&&dirty&&!await confirmDiscard('地址修改尚未保存，是否放弃修改并关闭自动填写？'))return;
    const data=await voyageRequest<VoyageDelivery>(memberUID?`/api/account/collections/${issueID}/claim`:`/api/voyage/issues/${issueID}/claim`,'POST',memberUID?memberClaimPayload(record,address,memberUID,action,bookEntryID):claimPayload(record,address,uid,code,action,bookEntryID));
    if(lifetime.current!==ticket)return;
    setRecord(action==='view'?refreshClaimRecord(record,data,dirty):data);if(action!=='view'||!dirty)setAddress(data.address||{...blankAddress});
    if(action==='save'){setNotice('本次地址已保存。');setLongTermError('');setLongTermNotice('')}
    if(preferenceAction){setError('');setNotice('')}
    if(action==='set_long_term')setLongTermNotice(longTermEnabled?'常用地址已更新。':'已启用常用地址。');
    if(action==='set_book_default')setLongTermNotice('已将所选地址设为常用地址。');
    if(action==='cancel_long_term')setLongTermNotice('已关闭自动填写，已保存的地址不变。');
    if(action==='view'){setLongTermError('');setLongTermNotice('')}
  }catch(e){if(lifetime.current===ticket){if(memberUID&&[401,403,408].includes((e as Error & {code?:number}).code||0)){setRecord(null);setAddress({...blankAddress});setCode('')}(preferenceAction?setLongTermError:setError)(e instanceof Error?e.message:'操作失败，请重试');}}finally{if(lifetime.current===ticket){running.current=false;setBusy(false);setPendingAction(null);}}
 };
 useEffect(()=>{if(memberUID)void request('view')},[memberUID,issueID]);
 const history=record?.address_book?.length?record.address_book.map(h=>({key:String(h.id),id:h.id,address:h.address,title:h.is_default?'常用地址':h.uses[0]?.title||'已保存的地址',isDefault:h.is_default})):(record?.history||[]).map((h,i)=>({key:`previous-${i}`,id:0,address:h.address,title:h.issue_title,isDefault:false}));
 const historic=history.find(h=>h.key===selectedHistory);
 return <section aria-label="地址与物流" className="mt-5 border-t border-border pt-5">
  <div className="mb-5 flex items-center gap-4"><h3 className="mr-auto font-medium">{record?'地址与物流':mode==='address'?'填写地址':'查询物流'}</h3><Button type="button" variant="ghost" size="sm" onClick={async()=>{if(await leave())onClose()}} disabled={busy} aria-label="收起地址与物流"><X size={18}/></Button></div>
  {!record?<form noValidate autoComplete="off" onSubmit={e=>{e.preventDefault();void request('view');}} className="max-w-xl space-y-4">
   {memberUID?<p className="text-sm leading-7 text-muted-foreground">当前登录 UID {memberUID}，可直接查看本人的领取内容与物流，无需私信认证码。</p>:<>
   <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor={`uid-${issueID}`}>B 站 UID</Label><Input id={`uid-${issueID}`} name="voyage-bilibili-uid" required inputMode="numeric" autoComplete="off" maxLength={19} value={uid} onChange={e=>setUID(e.target.value)} disabled={busy}/></div>
   <div className="space-y-2"><Label htmlFor={`code-${issueID}`}>私信认证码</Label><Input id={`code-${issueID}`} name="voyage-claim-code" required type="password" autoComplete="new-password" autoCapitalize="none" autoCorrect="off" spellCheck={false} aria-describedby={`code-hint-${issueID}`} maxLength={80} value={code} onChange={e=>setCode(e.target.value)} disabled={busy}/><p id={`code-hint-${issueID}`} className="text-xs text-muted-foreground">六位认证码区分大小写。</p></div></div>
   </>}
   <Button disabled={busy} type="submit">{busy?'正在读取…':memberUID?'查看本次领取记录':'验证并查看'}</Button>
  </form>:<>
   <div className="mb-5 flex flex-wrap gap-2"><Button size="sm" variant={mode==='address'?'default':'outline'} disabled={busy} onClick={()=>setMode('address')}>收货地址</Button><Button size="sm" variant={mode==='shipping'?'default':'outline'} disabled={busy} onClick={()=>setMode('shipping')}>物流信息</Button><Button size="sm" variant="ghost" disabled={busy} onClick={async()=>{if(memberUID){void request('view');return}if(!(await leave()))return;setRecord(null);setAddress({...blankAddress});setCode('');setError('');setNotice('');setLongTermError('');setLongTermNotice('');}}>{memberUID?'刷新地址与物流':'重新验证'}</Button></div>
   {!record.items?.length&&!!record.labels?.length&&<div className="mb-5 space-y-2"><h4 className="font-medium">本次领取内容</h4><ul className="list-disc space-y-1 pl-5 text-sm leading-6">{record.labels.map((label,i)=><li key={i}>{label}</li>)}</ul></div>}
   {!editable&&<p className="mb-4 text-sm text-muted-foreground">本次地址登记已结束。仍可查看寄送进度；尚未填地址的用户请联系主播补登记。</p>}

   {mode==='address'?<><form noValidate autoComplete="off" className="max-w-xl space-y-4" onSubmit={e=>{e.preventDefault();void request('save');}}>
    {!!history.length&&<section className="space-y-3 border-b border-border pb-4" aria-label="我的地址簿"><Label>我的地址簿</Label><Select value={selectedHistory} disabled={busy} onValueChange={setSelectedHistory}><SelectTrigger aria-label="选择历史地址"><SelectValue placeholder="选择已保存的收货地址"/></SelectTrigger><SelectContent>{history.map(h=><SelectItem key={h.key} value={h.key} textValue={`${h.isDefault?'常用地址 · ':''}${h.address.name} · ${h.address.address}`}><span className="block max-w-[70vw] whitespace-normal break-words text-left"><span className="block">{h.address.name} · {h.address.phone}{h.isDefault&&' · 常用地址'}</span><span className="block text-xs text-muted-foreground">{h.address.address} · {h.title}</span></span></SelectItem>)}</SelectContent></Select><div className="flex flex-wrap gap-2">{editable&&<Button type="button" size="sm" variant="outline" disabled={busy||!historic} onClick={()=>{if(historic){setAddress({...historic.address,note:address.note});setNotice('已填入所选地址，请核对后保存。')}}}>填入本次地址</Button>}{historic?.id!==0&&<Button type="button" size="sm" variant="ghost" disabled={busy||!historic||historic.isDefault||dirty} onClick={()=>historic&&void request('set_book_default',historic.id)}>{historic?.isDefault?'已是常用地址':'设为常用地址'}</Button>}</div><p className="text-xs leading-6 text-muted-foreground">可保存多个收货地址，常用地址只能设置一个。选择其他地址不会自动修改本次登记。</p></section>}
    <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor={`name-${issueID}`}>收件人</Label><Input id={`name-${issueID}`} autoComplete="off" required maxLength={40} value={address.name} disabled={busy||!editable} onChange={e=>setAddress({...address,name:e.target.value})}/></div><div className="space-y-2"><Label htmlFor={`phone-${issueID}`}>手机号码</Label><Input id={`phone-${issueID}`} autoComplete="off" type="tel" required maxLength={25} value={address.phone} disabled={busy||!editable} onChange={e=>setAddress({...address,phone:e.target.value})}/></div></div>
    <div className="space-y-2"><Label htmlFor={`address-${issueID}`}>收货地址</Label><textarea id={`address-${issueID}`} autoComplete="off" className={field} rows={3} required minLength={5} maxLength={300} placeholder="省、市、区及详细地址" value={address.address} disabled={busy||!editable} onChange={e=>setAddress({...address,address:e.target.value})}/></div>
    <div className="space-y-2"><Label htmlFor={`note-${issueID}`}>备注（选填）</Label><Input id={`note-${issueID}`} autoComplete="off" maxLength={300} value={address.note} disabled={busy||!editable} onChange={e=>setAddress({...address,note:e.target.value})}/></div>
    <p className="text-xs leading-6 text-muted-foreground">这份地址用于本次登记的礼物，可分多个包裹寄送。更新地址后，未寄包裹会由主播重新核对；已寄包裹的地址不会改变。</p>{editable&&<Button type="submit" disabled={busy}>{pendingAction==='save'?'正在保存…':'保存地址'}</Button>}
    {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="flex items-center gap-2 text-sm text-primary"><Check size={16}/>{notice}</p>}
   </form>
   <section aria-labelledby={`long-term-title-${issueID}`} aria-busy={pendingAction==='set_long_term'||pendingAction==='cancel_long_term'} className="mt-6 max-w-xl space-y-3 rounded-xl border border-border bg-card p-4 sm:p-5">
    <h4 id={`long-term-title-${issueID}`} className="font-medium">常用收货地址</h4>
    <label className="flex min-h-11 cursor-pointer items-center gap-3 text-sm leading-6"><input type="checkbox" className="size-4 shrink-0 accent-primary" checked={longTermEnabled} disabled={busy||!longTermEnabled&&!!setDisabledReason} aria-describedby={`long-term-hint-${issueID} long-term-state-${issueID}`} onChange={e=>void request(e.target.checked?'set_long_term':'cancel_long_term')}/><span>以后使用此地址</span></label>
    <p id={`long-term-state-${issueID}`} className="text-sm leading-6">{longTermEnabled?(usesLongTermAddress(record)?'已启用常用地址。':'当前常用地址与本次地址不同。'):'开启后，后续礼物将自动使用已保存的地址。'}</p>
    {longTermEnabled&&record.long_term?.source==='admin'&&<p className="text-sm leading-6">已由主播根据你的委托开启礼物托管，你可以随时关闭自动填写。</p>}
    <p className="text-sm leading-6">上方填写的备注信息仅对此次收集批次起效，后续收集中若自动读取常用地址，将不会有备注信息。</p>
    {longTermEnabled&&!usesLongTermAddress(record)&&<Button type="button" size="sm" variant="outline" disabled={busy||!!setDisabledReason} onClick={()=>void request('set_long_term')}>更新常用地址</Button>}
    {setDisabledReason&&<p className="text-xs leading-6 text-muted-foreground">{setDisabledReason}</p>}
    {record.address_source==='long_term'&&<p className="text-sm leading-6">已使用常用地址填写，请核对收货信息。{record.long_term_changed?'常用地址已更新或停用，本次仍保留原填写地址。':''}</p>}
    <p id={`long-term-hint-${issueID}`} className="text-xs leading-6 text-muted-foreground">设置立即生效，无需再次点击“保存地址”。已填写的地址不会自动更改。如需变更，请修改未寄出礼物的收货地址，或联系主播。</p>
    {(pendingAction==='set_long_term'||pendingAction==='cancel_long_term')&&<p role="status" className="text-sm text-muted-foreground">{pendingAction==='cancel_long_term'?'正在关闭自动填写…':longTermEnabled?'正在更新常用地址…':'正在启用常用地址…'}</p>}
    {longTermError&&<p role="alert" className="text-sm text-destructive">{longTermError}</p>}{longTermNotice&&<p role="status" className="flex items-center gap-2 text-sm text-primary"><Check size={16}/>{longTermNotice}</p>}
   </section></>:<div className="space-y-6">
    {!!record.items?.length&&<section><h4 className="mb-3 font-medium">你的礼物进度</h4><div className="divide-y divide-border rounded-xl border border-border px-4">{record.items.map((item,i)=><div key={i} className="flex items-start justify-between gap-4 py-3 text-sm"><p>{item.label}</p><span className="shrink-0 text-primary">{item.status}</span></div>)}</div></section>}
    {!!record.parcels?.length?<section className="space-y-4"><h4 className="font-medium">寄送包裹</h4>{record.parcels.map(p=><article key={p.id} className="rounded-xl border border-border p-4 text-sm"><div className="mb-3 flex items-center gap-2 font-medium"><Truck size={18}/><span>包裹 #{p.id}</span><span className="ml-auto text-primary">{p.status}</span></div><p className="mb-4 leading-6">{p.labels.join('、')}</p>{p.shipped_at?<dl className="grid grid-cols-[5em_1fr] gap-3"><dt className="text-muted-foreground">快递公司</dt><dd>{p.courier}</dd><dt className="text-muted-foreground">快递单号</dt><dd className="break-all select-all">{p.tracking}</dd><dt className="text-muted-foreground">寄出时间</dt><dd>{new Date(p.shipped_at).toLocaleString('zh-CN')}</dd></dl>:<p className="text-muted-foreground">正在准备寄送，寄出后会在这里显示物流。</p>}{p.note&&<p className="mt-3 whitespace-pre-wrap break-words">{p.note}</p>}</article>)}</section>:<p className="text-sm text-muted-foreground">{record.address?'地址已收到，礼物准备好后会安排寄送。':'尚未填写地址，请先到收货地址中登记。'}</p>}
    {!memberUID&&<Button size="sm" variant="outline" disabled={busy} onClick={()=>void request('view')}>刷新寄送进度</Button>}
   </div>}

  </>}
  {(!record||mode==='shipping')&&<>{error&&<p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="mt-4 flex items-center gap-2 text-sm text-primary"><Check size={16}/>{notice}</p>}</>}
 </section>;
}
