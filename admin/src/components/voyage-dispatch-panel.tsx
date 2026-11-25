'use client';
import {useSiteTheme} from '@/lib/site-theme';
import {BilibiliDirectMessages} from '@/components/bilibili-direct-messages';
import {confirmDiscard} from '@/lib/site-dialog';

import Link from 'next/link';
import {useGuardedLink} from '@/lib/use-sidebar-navigation-guard';
import { useCallback, useEffect, useRef, useState } from 'react';
import { RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { ShipmentAddress, type DeliveryRow } from '@/components/voyage-shipments';
import {inheritedAddressNotice,type ShipmentRecipient} from '@/lib/voyage-delivery';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import { VoyagePerson } from '@/components/voyage-person';
import { voyageRequest, type VoyageIssue } from '@/lib/voyage';
import { recipientMessage, voyageDispatchBlockReason, voyageField, whisperURL, type DispatchConfig, type DispatchData, type VoyageRecipient } from '@/lib/voyage-recipients';
import { detectWhisperHelper, prefillWhisper, whisperHelperDetail, type WhisperHelper } from '@/lib/whisper-helper';

export function Delivery({ issue, onStateChange, onPublish, publishBusy=false }: { issue: VoyageIssue; onPublish?:()=>Promise<void>;publishBusy?:boolean;onStateChange?: (state:{busy:boolean;dirty:boolean})=>void }) {
  const {theme}=useSiteTheme();
  const guardedLink=useGuardedLink();
  const [rawRows, setRows] = useState<DeliveryRow[]>([]);
  const [config, setConfig] = useState<DispatchConfig | null>(null);
  const [draft, setDraft] = useState<DispatchConfig | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const active = useRef(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');


  const [addressViewer, setAddressViewer] = useState(0);
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState(0);
  const [helper, setHelper] = useState<WhisperHelper | null>(null);
  const [detecting, setDetecting] = useState(false);
  const detectingRef = useRef(false);
  const [helperDetail, setHelperDetail] = useState('');
  const [manual, setManual] = useState<{ text: string; uid: string } | null>(null);
  const profiles=useBilibiliProfiles(rawRows.map(row=>row.uid));
  const rows=rawRows.map(row=>({...row,name:row.name||profiles[row.uid]?.name||'',avatar:profiles[row.uid]?.avatar||row.avatar}));
  const path = `/api/voyage/issues/${issue.id}`;
  const dirty = !!draft && !!config && (draft.template !== config.template || draft.entry_url !== config.entry_url);
  useEffect(()=>{onStateChange?.({busy,dirty});return()=>onStateChange?.({busy:false,dirty:false})},[busy,dirty,onStateChange]);
  const load = useCallback(async () => {
    setLoading(true); setError('');
    try {
      const [data, shipments] = await Promise.all([voyageRequest<DispatchData>(`${path}/dispatch`), voyageRequest<ShipmentRecipient[]>(`${path}/shipments`)]);
      const deliveries = new Map(shipments.map(row => [row.id, row.delivery]));
      if (!active.current) return;
      setRows(data.recipients.map(row => ({...row,delivery:deliveries.get(row.id) || {address:null,courier:'',tracking:'',shipping_note:'',submitted_at:null,shipped_at:null,version:0}}))); setAddressViewer(0); setConfig(data.config); setDraft(data.config);
      setSelected(current => data.recipients.some(row => row.id === current) ? current : data.recipients[0]?.id || 0);
    } catch (error) { if (active.current) setError(error instanceof Error ? error.message : '加载失败，请重试'); }
    finally { if (active.current) setLoading(false); }
  }, [path]);
  const detect = useCallback(async () => {
    if (detectingRef.current) return;
    detectingRef.current = true;
    setDetecting(true);
    const found = await detectWhisperHelper();
    detectingRef.current = false;
    if (active.current) { setHelper(found); setHelperDetail(whisperHelperDetail()); setDetecting(false); }
  }, []);
  useEffect(() => {
    active.current = true; void load(); void detect();
    const onFocus = () => { void detect(); };
    window.addEventListener('focus', onFocus);
    return () => { active.current = false; window.removeEventListener('focus', onFocus); };
  }, [load, detect]);
  const save = async () => {
    if (!draft || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError('');
    try {
      const saved = await voyageRequest<DispatchConfig>(`${path}/dispatch`, 'PUT', draft);
      setDraft(saved); setConfig(saved); setNotice('本批次私信模板已保存。');
    } catch (error) { setError(error instanceof Error ? error.message : '保存失败，请重试'); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const update = async (row: VoyageRecipient, action: string) => {
    const saved = await voyageRequest<VoyageRecipient>(`/api/voyage/issues/${row.issue_id}/recipients/${row.id}`, 'PUT', { action, version: row.version });
    const merged = { ...row, ...saved };
    if (active.current) setRows(current => current.map(item => item.id === row.id ? {...item,...merged} : item));
    return merged;
  };
  const markSent = async (row: DeliveryRow) => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(true); setError('');
    try { await update(row, row.sent_at ? 'unsent' : 'sent'); setNotice(row.sent_at ? '已撤销发送标记。' : `已确认向 ${row.name || row.uid} 发送私信。`); }
    catch (error) { setError(error instanceof Error ? error.message : '状态保存失败，请刷新重试'); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const send = async (row: DeliveryRow, automatic: boolean) => {
    if (!config || busyRef.current || row.sent_at) return;
    const message = recipientMessage(config, issue, row, theme.brand.streamer_name);
    const reason=voyageDispatchBlockReason(issue.published,row,dirty,publishBusy,message.length);if(reason){setError(reason);return;}
    busyRef.current = true; setBusy(true); setError(''); setNotice(''); setManual(null);
    let target: Window | null = null;
    try {
      if (automatic) {
        const connected=helper||await detectWhisperHelper();
        if(!connected)throw new Error('未连接私信助手。请安装后刷新页面，或使用“复制并打开私信”。');
        setHelper(connected);
        // Validate the row version before handing a credential to either helper.
        await update(row, 'prepared');
        await prefillWhisper(connected, row.uid, message);
        setNotice(`已填入 ${row.name || row.uid} 的私信，请核对并发送，回来后点击“标记已发私信”。`);
      } else {
        if (!navigator.clipboard?.writeText) throw new Error('浏览器未开放剪贴板，请从下方手动复制私信');
        // Start clipboard permission and open a placeholder within the same click gesture.
        const copying = navigator.clipboard.writeText(message);
        target = window.open('about:blank', '_blank');
        if (target) target.opener = null;
        await copying;
        await update(row, 'prepared');
        if (target) { target.location.replace(whisperURL(row.uid)); setNotice('已复制内容并打开私信，粘贴、核对后发送。'); }
        else { setManual({ text: message, uid: row.uid }); setNotice('私信已复制，浏览器拦截了新窗口，请点击下方“打开对应私信”。'); }
      }
    } catch (error) {
      if (target && !target.closed) target.close();
      setError(error instanceof Error ? error.message : '操作失败，请重试');
      // A stale/revoked row must be refreshed before exposing another send action.
      if (!(error instanceof Error) || !/已被其他页面修改|资格已停用|不在本批次/.test(error.message)) setManual({ text: message, uid: row.uid });
    } finally { busyRef.current = false; setBusy(false); }
  };
  const blockReason=(row:DeliveryRow)=>voyageDispatchBlockReason(issue.published,row,dirty,busy||publishBusy,config?recipientMessage(config,issue,row).length:0);
  const previewRow = rows.find(row => row.id === selected);
  const previewText = draft && previewRow ? recipientMessage(draft, issue, previewRow, theme.brand.streamer_name) : '';
  const states=[['all','全部'],['missing','未填地址'],['addressed','已有地址'],['inherited','使用常用地址'],['pending','未发私信'],['review','私信待复核'],['sent','已发私信']];
  const filtered=rows.filter(row=>!row.disabled&&(status==='all'||status==='missing'&&!row.delivery.address||status==='addressed'&&!!row.delivery.address||status==='inherited'&&row.delivery.address_source==='long_term'||status==='pending'&&!row.sent_at||status==='review'&&!row.sent_at&&['failed','unknown'].includes(row.message?.status||'')||status==='sent'&&!!row.sent_at)&&`${row.uid} ${row.name}`.toLowerCase().includes(query.trim().toLowerCase()));
  const pageCount=Math.max(1,Math.ceil(filtered.length/25)),currentPage=Math.min(page,pageCount),visible=filtered.slice((currentPage-1)*25,currentPage*25);
  return <>
    <BilibiliDirectMessages scope="voyage" scopeID={issue.id} version={issue.version} configVersion={config?.version||0}
      disabledReason={loading||busy||publishBusy?'正在读取或保存，请稍候':!issue.published?'请先发布这个地址收集批次':dirty?'私信模板有修改，请先保存':''}
      targets={config?filtered.filter(r=>!r.sent_at&&!r.code_error&&r.code).map(r=>({uid:r.uid,version:r.version,body:recipientMessage(config,issue,r)})):[]}
      single={config&&previewRow&&!previewRow.disabled&&!previewRow.sent_at&&previewRow.code?{uid:previewRow.uid,version:previewRow.version,body:recipientMessage(config,issue,previewRow)}:undefined}
      onFinished={()=>{if(!dirty&&!busy)void load()}}/>

    {!issue.published&&<section className="mb-5 space-y-3 rounded-md bg-secondary/50 p-4" aria-label="发布批次后发送私信">
      <p className="text-sm font-medium">私信按钮暂不可用：这个批次还没发布。</p>
      <p className="text-sm leading-6 text-muted-foreground">先发布批次，用户才能通过私信打开地址登记入口。</p>
      {onPublish&&<Button disabled={dirty||busy||publishBusy||loading||!rows.some(row=>!row.disabled)} onClick={()=>void onPublish()}>{publishBusy?'正在发布…':'发布批次'}</Button>}
      {!loading&&!rows.some(row=>!row.disabled)&&<p className="text-sm text-muted-foreground">先到“收集范围”选择并保存收件人。</p>}
    </section>}
    <p className="mb-5 text-sm leading-6 text-muted-foreground">用户可自行完成签名验证，登录后查看本人领取批次、填写地址和查询物流，无需逐人发送认证码。下方私信用于通知和核对信息，原认证码入口仍可使用。</p>
    {dirty&&<div className="mb-5 flex flex-wrap items-center gap-3 text-sm"><p>私信模板有修改，请先保存，再发布批次或发送私信。</p><Button size="sm" disabled={busy||publishBusy} onClick={()=>void save()}>保存私信模板</Button></div>}
    <details className="mb-5 rounded-xl border border-border bg-card p-4">
      <summary className="cursor-pointer text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring">手动发送 · 私信助手 · {detecting ? '正在检测…' : helper === 'userscript' ? '油猴脚本已连接' : helper === 'extension' ? '扩展已连接' : '尚未连接'}</summary>
      <div className="mt-4 space-y-3 text-sm leading-6 text-muted-foreground">
        <p>只需主播安装，油猴脚本和扩展任选一种。在同一个浏览器里打开后台并登录 B 站，之后可一键填入私信，由你核对后发送。</p>
        <p><a href="/admin/tools/fans-whisper-helper.user.js" target="_blank" rel="noreferrer" className="font-medium text-primary underline underline-offset-4">安装油猴脚本</a>：已有 Tampermonkey 等脚本管理器时打开此链接安装。如果显示源码，可新建脚本，粘贴全部源码并保存。</p>
        <p><a href="/admin/tools/fans-whisper-helper.zip" download className="font-medium text-primary underline underline-offset-4">下载 Chrome / Edge 扩展</a>：解压后，在浏览器扩展管理页开启开发者模式，选择“加载已解压的扩展程序”，选中含 manifest.json 的文件夹。</p>
        <p>安装后请刷新后台，助手不会自动发送消息。</p>
        {!detecting && helperDetail && <p className={helper ? 'text-primary' : 'text-foreground'}>{helperDetail}</p>}
        {!helper && <p>Edge / Chrome：打开 Tampermonkey 的扩展详情，检查“允许用户脚本”；没有此项时，按油猴提示检查“开发人员模式”。<a href="https://www.tampermonkey.net/faq.php?locale=zh&q=Q209" target="_blank" rel="noreferrer" className="ml-1 text-primary underline underline-offset-4">查看油猴说明</a></p>}
        <Button variant="outline" size="sm" disabled={detecting} onClick={() => void detect()}><RefreshCw size={14} />检测助手</Button>
      </div>
    </details>
    {loading ? <p role="status" className="py-8 text-sm text-muted-foreground">正在加载私信名单…</p> : draft && <>
      <details className="mb-6 rounded-xl border border-border bg-card p-4" open={rows.length === 0 || undefined}>
        <summary className="cursor-pointer text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring">本批次私信模板与预览{dirty ? ' · 有未保存的修改' : ''}</summary>
        <div className="mt-5 grid gap-x-6 gap-y-5 xl:grid-cols-2">
          <div className="space-y-2"><Label htmlFor="dispatch-url">登记链接（选填）</Label><Input id="dispatch-url" value={draft.entry_url} disabled={busy} placeholder="留空使用本站；也可填写其他登记链接" onChange={event=>setDraft({...draft,entry_url:event.target.value})}/><p className="text-xs leading-5 text-muted-foreground">留空使用本批次站内入口；私信内容会按每个人的名单记录生成。</p></div>
          <div className="space-y-2"><Label htmlFor="preview-recipient">预览收件人</Label>{rows.length?<Select value={String(selected)} onValueChange={value=>setSelected(Number(value))}><SelectTrigger id="preview-recipient" className="w-full"><SelectValue/></SelectTrigger><SelectContent>{rows.slice(0,200).map(row=><SelectItem key={row.id} value={String(row.id)}>{row.name||row.uid} · {row.uid}</SelectItem>)}</SelectContent></Select>:<p className="text-sm text-muted-foreground">请先导入名单</p>}</div>
          <div className="space-y-2"><Label htmlFor="dispatch-template">私信内容</Label><textarea id="dispatch-template" className={`${voyageField} block h-80 resize-none sm:h-96`} disabled={busy} maxLength={2000} value={draft.template} onChange={event=>setDraft({...draft,template:event.target.value})}/><p className="text-xs leading-5 text-muted-foreground">可用变量：{'{昵称}、{UID}、{批次名称}、{本次礼物}、{认证码}、{登记地址}、{地址提示}'}。自动沿用者会附上核对提示；可用“地址提示”调整位置。保留 UID 与认证码；私信最多 500 字。</p><div className="flex gap-3 pt-2"><Button disabled={busy||!dirty} onClick={()=>void save()}>保存私信模板</Button>{dirty&&<Button variant="ghost" disabled={busy} onClick={()=>setDraft(config)}>撤销修改</Button>}</div></div>
          <div className="space-y-2"><Label>私信预览</Label><div className="h-80 overflow-y-auto rounded-md border border-border bg-secondary/40 px-3 py-2 sm:h-96">{previewRow?(previewRow.disabled||previewRow.code_error?<p className="text-sm text-destructive">{previewRow.disabled?'该用户的领取资格已停用。':'认证码无法读取，请检查服务器加密密钥。'}</p>:<pre className="whitespace-pre-wrap break-words font-sans text-sm leading-6">{previewText}</pre>):<p className="text-sm text-muted-foreground">请先导入名单</p>}</div>{previewRow&&<p className={`text-xs leading-5 ${previewText.length>500?'text-destructive':'text-muted-foreground'}`}>{previewText.length}/500 字</p>}</div>
        </div>
      </details>
      <div className="mb-5 flex flex-wrap items-center gap-3"><p className="mr-auto text-sm">已收齐 <strong>{rows.filter(r=>!r.disabled&&r.delivery.address).length}</strong> / {rows.filter(r=>!r.disabled).length} 人的地址</p><Button variant="outline" size="sm" disabled={busy||dirty} onClick={()=>void load()}>刷新登记情况</Button><Button variant="outline" size="sm" asChild><Link href={`/dashboard/voyage/shipping/?collection=${issue.id}`} onClick={e=>guardedLink(e,()=>!busy&&!publishBusy&&(!dirty||confirmDiscard("私信模板尚未保存，确定放弃修改并去发货管理吗？")))}>去安排发货</Link></Button></div>
      <div className="mb-5 flex flex-wrap gap-3"><Input className="max-w-sm" aria-label="搜索登记用户" placeholder="搜索 UID 或昵称" value={query} onChange={e=>{setQuery(e.target.value);setPage(1)}}/><div className="flex flex-wrap gap-1" role="group" aria-label="登记情况筛选">{states.map(([id,label])=><Button key={id} size="sm" variant={status===id?'secondary':'ghost'} onClick={()=>{setStatus(id);setPage(1)}}>{label}</Button>)}</div></div>
      <div className="divide-y divide-border border-y border-border">{visible.map(row=><article key={row.id} className="py-5">
       <div className="flex flex-wrap items-start gap-4"><div className="min-w-40 flex-1"><VoyagePerson person={row}/><p className="mt-3 max-w-2xl text-sm leading-6">{row.batch_items?.map(i=>i.label).join('、')}</p></div><div className="min-w-28 text-sm leading-6"><p className={row.delivery.address?'text-primary':'text-muted-foreground'}>{row.delivery.address_source==='long_term'?row.delivery.long_term_origin==='admin'?'礼物托管已自动填写':'已使用常用地址':row.delivery.address_source==='admin_confirmed'?'主播已确认本次地址':row.delivery.address?'地址已填写':'等待填写地址'}</p><p className="text-xs text-muted-foreground">{row.sent_at?'私信已标记发送':'私信尚未标记发送'}</p>{!row.sent_at&&['failed','unknown'].includes(row.message?.status||'')&&<p className="mt-1 font-semibold text-destructive" title={row.message?.note}>{row.message?.status==='unknown'?'发送结果待确认':'发送失败 · 待复核'}</p>}</div><div className="flex flex-wrap gap-2">{row.delivery.address&&<Button size="sm" variant="outline" aria-expanded={addressViewer===row.id} onClick={()=>setAddressViewer(addressViewer===row.id?0:row.id)}>查看地址</Button>}<Button size="sm" disabled={!!blockReason(row)||!!row.sent_at} title={blockReason(row)||undefined} onClick={()=>void send(row,true)}>打开并自动填入</Button><Button size="sm" variant="outline" disabled={!!blockReason(row)||!!row.sent_at} onClick={()=>void send(row,false)}>复制并打开</Button><Button size="sm" variant="ghost" disabled={busy} onClick={()=>void markSent(row)}>{row.sent_at?'撤销私信标记':'标记已发私信'}</Button></div></div>
       {row.delivery.address_source==='long_term'&&<p className="mt-3 max-w-3xl text-sm leading-6 text-foreground">{inheritedAddressNotice(row.delivery)}</p>}
       {(row.delivery.address||row.delivery.has_history)&&<p className="mt-3 text-sm leading-6">{!row.delivery.address&&'有历史地址，可联系用户确认。'}<Link className="ml-2 text-primary underline underline-offset-4" href={`/dashboard/voyage/address-book/?uid=${row.uid}`} onClick={e=>guardedLink(e,()=>!busy&&!publishBusy&&(!dirty||confirmDiscard('私信模板尚未保存，是否放弃修改并查看地址簿？')))}>查看地址簿</Link></p>}
       {!row.sent_at&&blockReason(row)&&<p className="mt-3 text-sm text-primary">{blockReason(row)}</p>}{addressViewer===row.id&&<div className="mt-4 rounded-xl bg-secondary/30 p-4"><ShipmentAddress delivery={row.delivery}/></div>}
      </article>)}</div>
      {!visible.length&&<p className="py-10 text-center text-sm text-muted-foreground">这个筛选下暂时没有用户。</p>}
      <div className="mt-5 flex items-center justify-center gap-3 text-sm"><Button size="sm" variant="outline" disabled={currentPage===1} onClick={()=>setPage(currentPage-1)}>上一页</Button><span>{filtered.length} 人 · {currentPage}/{pageCount} 页</span><Button size="sm" variant="outline" disabled={currentPage===pageCount} onClick={()=>setPage(currentPage+1)}>下一页</Button></div>
      {manual&&<section className="mt-5 space-y-3 rounded-xl border border-border p-4"><p className="text-sm">可手动复制这段私信并粘贴。</p><pre className="whitespace-pre-wrap break-words font-sans text-sm">{manual.text}</pre><a className="text-primary underline" href={whisperURL(manual.uid)} target="_blank" rel="noreferrer">打开对应私信</a></section>}
    </>}
    {error&&<p role="alert" className="mt-4 text-sm text-destructive">{error}</p>}{notice&&<p role="status" className="mt-4 text-sm text-primary">{notice}</p>}
  </>;
}
