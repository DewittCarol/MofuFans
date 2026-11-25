'use client';
import {BilibiliDirectMessages} from '@/components/bilibili-direct-messages';
import {confirmAction,confirmDiscard} from '@/lib/site-dialog';

import { useCallback, useEffect, useState } from 'react';
import { Copy, Send, RefreshCw } from 'lucide-react';
import {useBilibiliProfiles} from '@/lib/use-bilibili-profiles';
import {BilibiliPerson} from '@/components/bilibili-person';
import {profileName} from '@/lib/bilibili-profiles';
import {GiftRosterPicker} from '@/components/digital-gift-rosters';
import { Button } from '@/components/ui/button';
import { giftAPI, giftField, giftMessage, type DigitalGift, type GiftRecipient } from '@/lib/digital-gifts';
import { connectAndPrefillWhisper, detectWhisperHelper, whisperHelperDetail } from '@/lib/whisper-helper';
import { whisperURL } from '@/lib/voyage-recipients';

export function GiftDispatch({ gift, onStateChange, onVersionChange }: { gift: DigitalGift; onVersionChange:(version:number)=>void; onStateChange: (state: { busy: boolean; dirty: boolean }) => void }) {
  const individual = gift.mode === 'individual', open = gift.mode === 'open', supportsRoster = !open;
  const [rows, setRows] = useState<GiftRecipient[]>([]), [selected, setSelected] = useState<number | null>(null), [uid, setUID] = useState(''), [password, setPassword] = useState(''), [message, setMessage] = useState('');
  const [search, setSearch] = useState(''), [page, setPage] = useState(0), [pageSize,setPageSize]=useState(30), [showRemoved, setShowRemoved] = useState(false);
  const [rosterState,setRosterState]=useState({busy:false,dirty:false});
  const [working, setBusy] = useState(false), [loading, setLoading] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [helperStatus, setHelperStatus] = useState('正在检测私信助手…');
  const [passwordError, setPasswordError] = useState(''), [passwordLoading, setPasswordLoading] = useState(false), [passwordRetry, setPasswordRetry] = useState(0), [messageBase, setMessageBase] = useState('');
  const busy=working||rosterState.busy;
  const activeRows=rows.filter(row=>row.roster_active), removedRows=rows.filter(row=>!row.roster_active), listedRows=showRemoved?removedRows:activeRows;
  const profiles=useBilibiliProfiles([...listedRows.map(r=>r.uid),...(!individual?[uid.trim()]:[])]);
  const manual = !individual && selected === null;
  const selectedRow=selected === null ? (manual ? rows.find(r=>r.uid===uid.trim()) : undefined) : rows.find(r=>r.id===selected);
  const recipient = selectedRow?{...selectedRow,name:selectedRow.name||profiles[selectedRow.uid]?.name||''}:undefined, targetUID = manual ? uid.trim() : recipient?.uid || '';
  const recipientID = recipient?.id, recipientUID = recipient?.uid, recipientVersion = recipient?.version, recipientDisabled = recipient?.disabled, recipientActive = recipient?.roster_active;
  const dirty = rosterState.dirty || message !== messageBase;
  useEffect(() => { onStateChange({ busy, dirty }); }, [busy, dirty, onStateChange]);
  async function selectRecipient(id: number | null) {
    if (busy || id === selected) return;
    if (message !== messageBase && !(await confirmDiscard('私信内容已修改，切换后会生成另一位用户的私信。确定切换吗？'))) return;
    setSelected(id); if(id === null) setUID(''); setError(''); setNotice('');
  }
  async function toggleRemoved() {
    if (busy) return;
    if (message !== messageBase && !(await confirmDiscard('私信内容已修改，切换名单后会重新生成私信。确定切换吗？'))) return;
    setShowRemoved(value=>!value); setSelected(null); setUID(''); setSearch(''); setPage(0); setError(''); setNotice('');
  }
  const filtered = listedRows.filter(row => !search || row.uid.includes(search) || profileName(row.uid,row.name,profiles[row.uid]).includes(search));
  const pages=Math.max(1,Math.ceil(filtered.length/pageSize)),current=Math.min(page,pages-1);
  const displayed = filtered.slice(current * pageSize, (current + 1) * pageSize);
  const blockedReason = loading ? '正在读取通知名单，请稍候。' : rosterState.dirty ? '请先保存关联名单，再准备领取私信。' : !gift.published ? '请先到“礼物内容”勾选“上架到前台”并保存，才能发送领取私信。'
    : recipientActive === false ? '这位用户已不在关联名单中。'
    : recipient?.disabled ? (individual ? '这位用户已被取消本礼物的领取资格。' : '这位用户已停用这份礼物的私信通知。')
    : !/^[1-9]\d{0,18}$/.test(targetUID) ? '请填写有效的 B 站 UID。'
    : !open && passwordLoading ? '正在读取礼物密码，请稍候。'
    : !open && !password ? '礼物密码尚未读取，请点击“重新读取密码”。'
    : !message.trim() ? '请填写私信内容。'
    : message.length > 500 ? '私信最多 500 字，请缩短后重试。' : '';
  const allowed = !blockedReason;
  useEffect(() => {
    let live = true;
    void detectWhisperHelper().then(() => { if (live) setHelperStatus(whisperHelperDetail()); });
    return () => { live = false; };
  }, []);
  const reload = useCallback(async (signal?: AbortSignal) => { setLoading(true); try { const next = await giftAPI<GiftRecipient[]>(`/${gift.id}/recipients`, 'GET', undefined, signal); if (!signal?.aborted) { setRows(next); setError(''); } } catch (e) { if (!signal?.aborted) setError((e as Error).message); } finally { if (!signal?.aborted) setLoading(false); } }, [gift.id]);
  useEffect(() => { if (!supportsRoster) return; const abort = new AbortController(); void reload(abort.signal); return () => abort.abort(); }, [reload, supportsRoster, gift.version]);
  useEffect(() => {
    let live = true; setPassword(''); setMessage(''); setMessageBase(''); setPasswordError(''); setPasswordLoading(false);
    if(open){const text=giftMessage(gift.title,'','',false,true);setMessage(text);setMessageBase(text);return}
    if (individual && (!recipientID || recipientDisabled || recipientActive === false)) return;
    setPasswordLoading(true);
    giftAPI<{ password: string }>(`/${gift.id}/password${individual ? `?uid=${recipientUID}` : ''}`).then(data => { if (live) { const text = giftMessage(gift.title, data.password, recipientUID || '', individual); setPassword(data.password); setMessage(text); setMessageBase(text); } }).catch(e => { if (live) setPasswordError(e.message); }).finally(() => { if (live) setPasswordLoading(false); });
    return () => { live = false; };
  }, [gift.id, gift.title, gift.version, individual, open, recipientID, recipientUID, recipientVersion, recipientDisabled, recipientActive, passwordRetry]);
  async function run(action: () => Promise<void>) { setBusy(true); setError(''); setNotice(''); try { await action(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }
  async function update(row: GiftRecipient, action: string) {
    if ((action === 'reset' || action === 'disable') && !(await confirmAction(action === 'reset' ? `UID ${row.uid} 的旧密码会立即失效，需要重新发私信给对方。` : (individual ? `将取消 UID ${row.uid} 对这份电子礼物的领取资格，不影响原名单和其他礼物。` : `将停止给 UID ${row.uid} 发送这份礼物的私信通知。已经拿到的统一密码仍然可以使用。`), { title: action === 'reset' ? '重置领取密码？' : individual ? '取消本礼物领取资格？' : '停用这份礼物的通知？', confirmLabel: action === 'reset' ? '重置密码' : individual ? '取消资格' : '停用通知', destructive: true }))) return;
    await run(async () => { const next = await giftAPI<GiftRecipient>(`/${gift.id}/recipients/${row.id}`, 'PUT', { action, version: row.version }); setRows(current => current.map(r => r.id === next.id ? {...r,...next} : r)); setNotice(action === 'sent' ? '已标记发出私信。' : action === 'reset' ? '密码已重置，请重新发私信。' : action === 'unsent' ? '已改为待发私信。' : action === 'disable' ? (individual ? '已取消本礼物的领取资格，原名单不变。' : '已停用私信通知，统一密码仍可使用。') : (individual ? '已恢复本礼物的领取资格。' : '已恢复私信通知。')); });
  }
  useEffect(()=>{
    const refresh=()=>{if(supportsRoster&&!busy&&!dirty)void reload()};
    window.addEventListener('focus',refresh);return()=>window.removeEventListener('focus',refresh);
  },[supportsRoster,busy,dirty,reload]);
  const detect = () => run(async () => { setHelperStatus('正在检测私信助手…'); await detectWhisperHelper(); setHelperStatus(whisperHelperDetail()); });
  const fill = () => run(async () => {
    if (!allowed) return;
    if(!open){const latest=await giftAPI<{password:string}>(`/${gift.id}/password${individual?`?uid=${targetUID}`:''}`);if(latest.password!==password){setPasswordRetry(n=>n+1);throw Error('密码已变化，已重新读取，请核对后再发送。')}}
    setHelperStatus('正在连接私信助手并填入…');
    try {
      await connectAndPrefillWhisper(targetUID, message);
      setNotice(`已打开并填入。请在 B 站核对后手动发送${recipient ? '，再回来标记' : ''}。`);
    } finally { setHelperStatus(whisperHelperDetail()); }
  });
  async function copyOpen() {
    if (!allowed) return;
    // Open synchronously within the click so clipboard permission does not lose user activation.
    const tab = window.open('about:blank', '_blank'); if (tab) tab.opener = null;
    await run(async () => { try { await navigator.clipboard.writeText(message); if (tab) tab.location.href = whisperURL(targetUID); setNotice(tab ? '已复制并打开私信，请粘贴后手动发送。' : '已复制；浏览器拦截了新窗口，请点击下方“打开此人的私信”。'); } catch { tab?.close(); throw new Error('复制失败，请选中私信内容手动复制，再打开此人的私信。'); } });
  }
  return <div className="space-y-7">
    <BilibiliDirectMessages scope="gift" scopeID={gift.id} version={gift.version}
      disabledReason={busy||loading?'正在读取或保存，请稍候':!gift.published?'请先保存并上架礼物':rosterState.dirty?'请先保存关联名单':''}
      targets={(showRemoved?activeRows:filtered).filter(r=>!r.disabled&&!r.sent_at).map(r=>({uid:r.uid,version:r.version}))}
      single={allowed&&!recipient?.sent_at?{uid:targetUID,body:message,version:recipient?.version||0}:undefined}
      onFinished={()=>{if(supportsRoster&&!busy)void reload()}}/>

    {error && <div role="alert" className="space-y-2 text-sm text-destructive"><p>{error}</p>{supportsRoster && <Button variant="outline" size="sm" disabled={busy} onClick={() => void run(reload)}>刷新名单</Button>}</div>}
    {notice && <p role="status" className="text-sm text-primary">{notice}</p>}
    {!gift.published && <p className="rounded-lg bg-secondary p-4 text-sm leading-6">这份礼物还在草稿中。准备好文件后，先到“礼物内容”勾选上架并保存，再发送领取私信。</p>}
    {supportsRoster && <>
      <GiftRosterPicker individual={individual} giftID={gift.id} version={gift.version} disabled={working} onStateChange={setRosterState} onSaved={async version=>{onVersionChange(version)}} />
      <section className="space-y-3 border-t border-border pt-6"><div className="flex flex-wrap items-center justify-between gap-3"><h3 className="font-semibold">{showRemoved ? '已移出成员' : individual ? '领取名单' : '通知名单'}（{listedRows.length} 人）</h3><div className="flex flex-wrap items-center gap-2">{removedRows.length>0&&<Button size="sm" variant="ghost" disabled={busy || loading} onClick={() => void toggleRemoved()}>{showRemoved?`返回当前名单（${activeRows.length}）`:`查看已移出（${removedRows.length}）`}</Button>}<Button size="sm" variant="ghost" disabled={busy || loading} onClick={() => void run(reload)}><RefreshCw size={14} />刷新</Button></div></div>{showRemoved&&<p className="text-xs leading-6 text-muted-foreground">这些记录只用于保留历史发送状态和领取信息，不会加入待发私信。</p>}<label htmlFor="gift-search" className="sr-only">搜索 UID 或昵称</label><input id="gift-search" className={giftField} type="search" placeholder="搜索 UID 或昵称" value={search} onChange={e => { setSearch(e.target.value); setPage(0); }} />
        {loading ? <p role="status" className="text-sm text-muted-foreground">正在读取名单…</p> : !filtered.length ? <p className="py-5 text-sm text-muted-foreground">{listedRows.length ? '没有找到这位用户。' : showRemoved ? '没有已移出的历史成员。' : rows.length ? '当前关联名单没有成员。' : '关联名单后，这里会显示成员和私信状态。'}</p> : <ul className="divide-y divide-border">{displayed.map(row => <li key={row.id} className={`flex flex-wrap items-center gap-3 rounded-md px-2 py-3 ${selected === row.id ? 'bg-secondary' : ''}`}><div className="min-w-0 flex-1"><BilibiliPerson uid={row.uid} name={row.name} profile={profiles[row.uid]}/><p className="mt-1 text-xs text-muted-foreground">{!row.roster_active ? '已移出关联名单' : row.disabled ? (individual ? '已取消本礼物资格' : '已停用私信通知') : row.sent_at ? '已发私信' : '待发私信'}</p>{!row.sent_at&&['failed','unknown'].includes(row.message?.status||'')&&<p className="mt-1 text-xs font-semibold text-destructive" title={row.message?.note}>{row.message?.status==='unknown'?'发送结果待确认':'发送失败 · 待复核'}</p>}</div><div className="flex flex-wrap gap-1">{(row.sources?.length||0)>0&&<details className="relative"><summary className="cursor-pointer rounded-md px-3 py-2 text-sm text-primary focus-visible:outline-2 focus-visible:outline-ring">编辑名单资料</summary><div className="absolute right-0 z-20 w-64 max-w-[75vw] rounded-lg border border-border bg-background p-3"><p className="mb-2 text-xs leading-5 text-muted-foreground">在原名单中修改，关联礼物会自动同步。</p>{Array.from(new Map(row.sources?.map(source=>[source.roster_id,source])).values()).map(source=><a key={source.roster_id} className="block break-words rounded px-2 py-2 text-sm hover:bg-secondary" href={`/admin/dashboard/voyage/roster/?roster=${source.roster_id}&uid=${row.uid}`} target="_blank" rel="noopener noreferrer">{source.roster_title}</a>)}</div></details>}<Button size="sm" variant="outline" disabled={busy || row.disabled || !row.roster_active} onClick={() => selectRecipient(row.id)}>准备私信</Button><Button size="sm" variant="ghost" disabled={busy || !row.roster_active} onClick={() => void update(row, row.disabled ? 'enable' : 'disable')}>{individual ? (row.disabled ? '恢复资格' : '取消资格') : (row.disabled ? '恢复通知' : '停用通知')}</Button>{individual && <Button size="sm" variant="ghost" disabled={busy || !row.roster_active} onClick={() => void update(row, 'reset')}>重置密码</Button>}</div></li>)}</ul>}
        <nav aria-label="电子礼物名单分页" className="flex flex-wrap items-center justify-center gap-3 pt-3 text-sm"><span>共 {filtered.length} 人</span><Button size="sm" variant="outline" disabled={busy||current===0} onClick={()=>setPage(current-1)}>上一页</Button><span>{current+1} / {pages}</span><Button size="sm" variant="outline" disabled={busy||current+1>=pages} onClick={()=>setPage(current+1)}>下一页</Button><label className="flex items-center gap-2">每页<select className={`${giftField} !w-auto`} value={pageSize} onChange={e=>{setPageSize(Number(e.target.value));setPage(0)}}>{[10,30,50,100].map(size=><option key={size} value={size}>{size} 人</option>)}</select></label></nav>
      </section>
    </>}
    <section className="space-y-4 border-t border-border pt-6"><h3 className="font-semibold">{recipient ? `给 ${recipient.name || recipient.uid} 的私信` : '领取私信'}</h3>
      {!individual && selected !== null && <Button variant="outline" size="sm" disabled={busy} onClick={() => void selectRecipient(null)}>改为手动填写 UID</Button>}
      {individual && !recipient ? <p className="text-sm text-muted-foreground">点击名单中的“准备私信”，查看这位用户的密码和私信内容。</p> : <>
        {manual && <div className="space-y-2"><label htmlFor="gift-send-uid" className="text-sm font-medium">收件人的 B 站 UID</label><input id="gift-send-uid" inputMode="numeric" maxLength={19} autoComplete="off" className={giftField} value={uid} disabled={busy} onChange={e => setUID(e.target.value)} /><p className="text-xs text-muted-foreground">{open ? '无密码礼物不需要名单。' : '可以在上方关联名单批量通知，也可以单独填写 UID。'}填入 UID 后会自动显示对方资料。</p>{/^[1-9]\d{0,18}$/.test(uid.trim())&&<BilibiliPerson uid={uid.trim()} profile={profiles[uid.trim()]}/>}</div>}
        {!open && <div className="space-y-2">{passwordLoading && <p role="status" className="text-sm text-muted-foreground">正在读取密码…</p>}{passwordError && <div role="alert" className="space-y-2"><p className="text-sm text-destructive">{passwordError}</p><Button variant="outline" size="sm" disabled={busy || passwordLoading} onClick={() => setPasswordRetry(n => n + 1)}>重新读取密码</Button></div>}<label htmlFor="gift-code" className="text-sm font-medium">{individual ? '这位用户的密码' : '统一密码'}</label><input id="gift-code" readOnly value={password} className={`${giftField} font-mono`} onFocus={e => e.target.select()} /><Button size="sm" variant="outline" disabled={busy || !password} onClick={() => void run(async () => { await navigator.clipboard.writeText(password); setNotice('密码已复制。'); })}><Copy size={14} />复制密码</Button></div>}
        <div className="space-y-2"><label htmlFor="gift-message" className="text-sm font-medium">私信内容</label><textarea id="gift-message" className={giftField} rows={7} maxLength={500} value={message} disabled={busy || (!open && !password)} onChange={e => setMessage(e.target.value)} /><div className="flex flex-wrap justify-between gap-2 text-xs text-muted-foreground"><span>{open ? '私信中保留礼物名称，方便对方查找。' : '私信中保留礼物名称和密码，方便对方领取。'}</span><span>{message.length} / 500</span></div></div>
        <p id={`gift-dispatch-status-${gift.id}`} role="status" className="text-sm leading-6 text-muted-foreground">{blockedReason || helperStatus}</p>
        <div className="flex flex-wrap gap-3"><Button disabled={busy || !allowed} aria-describedby={`gift-dispatch-status-${gift.id}`} onClick={() => void fill()}><Send size={15} />打开并自动填入</Button><Button variant="outline" disabled={busy || !allowed} aria-describedby={`gift-dispatch-status-${gift.id}`} onClick={() => void copyOpen()}>复制并打开私信</Button>{recipient && <Button variant="outline" disabled={busy || !gift.published || recipient.disabled || !recipient.roster_active} onClick={() => void update(recipient, recipient.sent_at ? 'unsent' : 'sent')}>{recipient.sent_at ? '改为待发私信' : '我已发出私信'}</Button>}</div>
        {/^[1-9]\d{0,18}$/.test(targetUID) && <a href={whisperURL(targetUID)} target="_blank" rel="noopener noreferrer" className="inline-block text-sm text-primary underline underline-offset-4">打开此人的私信</a>}
        <p className="text-xs leading-6 text-muted-foreground">助手只填入内容，不会自动发送。请在 B 站核对收件人和内容后手动发送{recipient ? '，再回来标记' : ''}。</p>
      </>}
    </section>
    <section className="space-y-3 border-t border-border pt-5"><h3 className="text-sm font-semibold">手动发送 · 私信助手</h3><p role="status" className="text-xs leading-6 text-muted-foreground">{helperStatus}</p><div className="flex flex-wrap items-center gap-4"><Button variant="outline" size="sm" disabled={busy} onClick={() => void detect()}>检测助手</Button><a className="text-sm text-primary underline underline-offset-4" href="/admin/tools/fans-whisper-helper.user.js" target="_blank" rel="noreferrer">安装 / 更新油猴脚本</a><a className="text-sm text-primary underline underline-offset-4" href="/admin/tools/fans-whisper-helper.zip" download>下载浏览器扩展</a></div><p className="text-xs leading-6 text-muted-foreground">二选一安装。电子礼物及测试站需要 1.2.0 或更新版本，更新后刷新本页和 B 站私信页。</p></section>
  </div>;
}
