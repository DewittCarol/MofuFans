'use client';
import {confirmAction} from '@/lib/site-dialog';

import { useEffect, useRef, useState } from 'react';
import { Download, Trash2, Upload } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {uploadGiftFile} from '@/lib/digital-gift-upload';
import { GiftDispatch } from '@/components/digital-gift-dispatch';
import { giftAPI, giftField, giftSize, type DigitalGift, type GiftFile } from '@/lib/digital-gifts';

const blank: DigitalGift = { id: 0, title: '', intro: '', content: '', mode: 'shared', published: false, version: 0, files: [] };

export function GiftEditor({ id, onSaved, onDeleted, onStateChange }: { id: number; onSaved: (id: number) => void; onDeleted: () => void; onStateChange: (state: { busy: boolean; dirty: boolean }) => void }) {
  const [gift, setGift] = useState<DigitalGift>(blank), [password, setPassword] = useState(''), [loading, setLoading] = useState(!!id), [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState(''), [tab, setTab] = useState<'content' | 'dispatch'>('content');
  const [saved, setSaved] = useState<DigitalGift | null>(null), [retry, setRetry] = useState(0);
  const [uploadProgress,setUploadProgress]=useState<{name:string;bytes:number;size:number;index:number;total:number}|null>(null);
  const uploadAbort=useRef<AbortController|null>(null);
  useEffect(()=>()=>uploadAbort.current?.abort(),[]);
  const [dispatchState, setDispatchState] = useState({ busy: false, dirty: false });
  const formValue = (g: DigitalGift) => JSON.stringify([g.title, g.intro, g.content, g.mode, g.published]);
  const dirty = password !== '' || formValue(gift) !== formValue(saved || blank);
  useEffect(() => { onStateChange({ busy: busy || dispatchState.busy, dirty: dirty || dispatchState.dirty }); }, [busy, dirty, dispatchState.busy, dispatchState.dirty, onStateChange]);
  useEffect(() => { if (!id) return; let live = true; setLoading(true); setError(''); giftAPI<DigitalGift>(`/${id}`).then(data => { if (live) { setGift(data); setSaved(data); } }).catch(e => { if (live) setError(e.message); }).finally(() => { if (live) setLoading(false); }); return () => { live = false; }; }, [id, retry]);
  async function save() { if (dispatchState.dirty && !(await confirmAction('保存礼物会按最新名称和密码重新生成私信，当前私信修改会丢失。', { title: '保存礼物并更新私信？', confirmLabel: '保存并更新', cancelLabel: '继续编辑' }))) return; setBusy(true); setError(''); setNotice(''); try { const data = await giftAPI<DigitalGift>(id ? `/${id}` : '', id ? 'PUT' : 'POST', { ...gift, password }); setGift(data); setSaved(data); setPassword(''); setNotice(data.published ? '已保存，观众可以在前台看到这份礼物。' : '草稿已保存，观众暂时看不到。'); onSaved(data.id); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }
  async function upload(files: FileList | null) {
    if (!files?.length || uploadAbort.current) return;
    const list=Array.from(files),controller=new AbortController();uploadAbort.current=controller;
    setBusy(true); setError(''); setNotice(''); let count = 0;
    try {
      for (const [index,file] of list.entries()) {
        if(controller.signal.aborted)break;
        const progress={name:file.name,bytes:0,size:file.size,index:index+1,total:list.length};setUploadProgress(progress);
        const row=await uploadGiftFile(id,file,bytes=>setUploadProgress({...progress,bytes}),controller.signal);
        setGift(g=>({...g,files:[...g.files,row]}));count++;
      }
    } catch(e){if(!controller.signal.aborted)setError((e as Error).message)}
    finally{try{const latest=await giftAPI<DigitalGift>(`/${id}`);setGift(g=>({...g,files:latest.files}))}catch{} setNotice(`${controller.signal.aborted?'上传已取消。':''}已上传 ${count} 个文件。`);setUploadProgress(null);uploadAbort.current=null;setBusy(false)}
  }
  async function removeFile(file: GiftFile) { if (!(await confirmAction(`将删除「${file.name}」，删除后无法恢复。`, { title: '删除礼物文件？', confirmLabel: '删除文件', destructive: true }))) return; setBusy(true); setError(''); try { await giftAPI(`/${id}/files/${file.id}`, 'DELETE'); setGift(g => ({ ...g, files: g.files.filter(f => f.id !== file.id) })); setNotice('文件已删除。'); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }
  async function remove() { if (!(await confirmAction(`将删除「${gift.title}」及其文件和领取记录，关联的原名单会保留。此操作无法恢复。`, { title: '删除电子礼物？', confirmLabel: '删除礼物', destructive: true }))) return; setBusy(true); setError(''); try { await giftAPI(`/${id}`, 'DELETE', { version: gift.version }); onDeleted(); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } }
  if (loading) return <p role="status" className="py-12 text-sm text-muted-foreground">正在读取礼物…</p>;
  if (id && !saved) return <div role="alert"><p>{error}</p><Button variant="outline" className="mt-4" onClick={() => setRetry(n => n + 1)}>重新加载</Button></div>;
  return <section className="min-w-0 rounded-xl border border-border bg-card p-5 sm:p-7">
    <h2 className="break-words text-xl font-semibold">{id ? saved?.title : '新建电子礼物'}</h2>
    {id !== 0 && <div className="mb-6 mt-5 flex gap-2 border-b border-border pb-3" aria-label="礼物管理"><Button variant={tab === 'content' ? 'default' : 'ghost'} aria-pressed={tab === 'content'} disabled={busy || dispatchState.busy} onClick={() => setTab('content')}>礼物内容</Button><Button variant={tab === 'dispatch' ? 'default' : 'ghost'} aria-pressed={tab === 'dispatch'} disabled={busy || dispatchState.busy} onClick={() => setTab('dispatch')}>{gift.mode === 'open' ? '私信' : '名单与私信'}</Button></div>}
    {error && <p role="alert" className="my-4 text-sm text-destructive">{error}</p>}{notice && <p role="status" className="my-4 text-sm text-primary">{notice}</p>}
    <div hidden={tab !== 'content'}>
      <form className="mt-6 space-y-5" onSubmit={e => { e.preventDefault(); void save(); }}>
        <div className="space-y-2"><label htmlFor="gift-title" className="text-sm font-medium">礼物名称</label><input id="gift-title" className={giftField} maxLength={120} required disabled={busy || dispatchState.busy} value={gift.title} onChange={e => setGift({ ...gift, title: e.target.value })} /></div>
        <div className="space-y-2"><label htmlFor="gift-intro" className="text-sm font-medium">公开简介（选填）</label><textarea id="gift-intro" className={giftField} rows={2} maxLength={500} disabled={busy || dispatchState.busy} value={gift.intro} onChange={e => setGift({ ...gift, intro: e.target.value })} /><p className="text-xs text-muted-foreground">解锁前也能看到，不要在这里填写密码或私密内容。</p></div>
        <fieldset disabled={!!id || busy || dispatchState.busy} className="space-y-3"><legend className="mb-3 text-sm font-medium">领取方式{id ? '（创建后固定）' : ''}</legend><div className="grid gap-3 sm:grid-cols-3">{(['open', 'shared', 'individual'] as const).map(mode => <label key={mode} className={`flex items-start gap-3 rounded-lg border p-4 ${gift.mode === mode ? 'border-primary bg-secondary/40' : 'border-border'}`}><input type="radio" name="gift-mode" value={mode} checked={gift.mode === mode} className="mt-1 accent-[var(--primary)]" onChange={() => { setGift({ ...gift, mode }); setPassword(''); }} /><span><span className="block text-sm font-medium">{mode === 'open' ? '无密码' : mode === 'shared' ? '统一密码' : '每人独立密码'}</span><span className="mt-1 block text-xs leading-5 text-muted-foreground">{mode === 'open' ? '上架后，任何人都可以查看和下载。' : mode === 'shared' ? '使用同一个密码领取，可关联名单批量通知。' : '关联名单后，自动为每个 UID 生成密码。'}</span></span></label>)}</div></fieldset>
        {gift.mode === 'shared' && <div className="space-y-2"><label htmlFor="gift-password" className="text-sm font-medium">{id ? '更换密码（选填）' : '礼物密码（选填）'}</label><input id="gift-password" className={giftField} type="text" autoComplete="off" autoCapitalize="none" spellCheck={false} minLength={6} maxLength={64} disabled={busy || dispatchState.busy} value={password} onChange={e => setPassword(e.target.value)} /><p className="text-xs leading-5 text-muted-foreground">{id ? '留空保留原密码；更换后，旧密码及已解锁的访问会失效。' : '留空自动生成；也可设置 6–64 个字符，不含空格。'} 保存后可在“名单与私信”中查看。</p></div>}
        <div className="space-y-2"><label htmlFor="gift-content" className="text-sm font-medium">{gift.mode === 'open' ? '礼物内容' : '解锁后的内容'}（选填）</label><textarea id="gift-content" className={giftField} rows={6} maxLength={6000} disabled={busy || dispatchState.busy} value={gift.content} onChange={e => setGift({ ...gift, content: e.target.value })} /><p className="text-xs text-muted-foreground">可以写给观众的话、使用方法或下载说明。</p></div>
      <section className="space-y-3 border-y border-border py-5"><h3 className="text-base font-semibold">礼物文件</h3>{!id ? <p className="mt-3 text-sm text-muted-foreground">先创建礼物，再上传图片、音频、视频或压缩包。</p> : <><p className="mt-2 text-xs leading-5 text-muted-foreground">不限制单个文件大小，可多选上传。文件上传完成后立即保存。{gift.mode === 'open' ? '上架后即可下载。' : '领取人解锁后可以下载。'}</p><label className={`mt-4 inline-flex min-h-10 items-center gap-2 rounded-md border border-input px-4 text-sm focus-within:outline-2 focus-within:outline-ring ${busy ? 'opacity-50' : 'cursor-pointer hover:bg-secondary'}`}><Upload size={16} />{busy ? '正在处理…' : '上传文件'}<input type="file" multiple disabled={busy || dispatchState.busy} className="sr-only" onChange={e => { const files = e.target.files; void upload(files); e.target.value = ''; }} /></label>{uploadProgress&&<div role="status" className="space-y-2"><p className="break-all text-sm">{uploadProgress.name} · 第 {uploadProgress.index} / {uploadProgress.total} 个文件</p><progress className="h-2 w-full accent-primary" max={Math.max(1,uploadProgress.size)} value={uploadProgress.bytes}/><div className="flex items-center justify-between gap-3"><span className="text-xs text-muted-foreground">{giftSize(uploadProgress.bytes)} / {giftSize(uploadProgress.size)}</span><Button type="button" variant="ghost" size="sm" onClick={()=>uploadAbort.current?.abort()}>取消上传</Button></div></div>}<ul className="mt-3 divide-y divide-border">{gift.files.map(file => <li key={file.id} className="flex items-center gap-3 py-3"><div className="min-w-0 flex-1"><p className="break-all text-sm">{file.name}</p><p className="mt-1 text-xs text-muted-foreground">{giftSize(file.size)}</p></div><a className="rounded p-2 text-primary focus-visible:outline-2 focus-visible:outline-ring" href={`/api/admin/gifts/${id}/files/${file.id}`} aria-label={`下载 ${file.name}`}><Download size={17} /></a><Button type="button" variant="ghost" size="icon" disabled={busy || dispatchState.busy} aria-label={`删除 ${file.name}`} onClick={() => void removeFile(file)}><Trash2 size={17} /></Button></li>)}</ul></>}</section>
        <label className="flex items-center gap-3 text-sm"><input type="checkbox" className="h-4 w-4 accent-[var(--primary)]" checked={gift.published} disabled={busy || dispatchState.busy} onChange={e => setGift({ ...gift, published: e.target.checked })} />上架到前台</label>
        <div className="flex flex-wrap gap-3"><Button type="submit" disabled={busy || dispatchState.busy}>{busy ? '正在处理…' : id ? '保存礼物' : '创建礼物'}</Button>{id !== 0 && <Button type="button" variant="outline" disabled={busy || dispatchState.busy} onClick={() => void remove()}>删除礼物</Button>}</div>
      </form>

    </div>
    {saved && <div hidden={tab !== 'dispatch'}><GiftDispatch gift={saved} onStateChange={setDispatchState} onVersionChange={version=>{setGift(g=>({...g,version}));setSaved(g=>g?{...g,version}:null)}} /></div>}
  </section>;
}
