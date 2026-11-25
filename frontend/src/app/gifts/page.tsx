'use client';

import { useEffect, useState } from 'react';
import { ChevronDown, Download, Gift, LockKeyhole } from 'lucide-react';
import { Button } from '@/components/ui/button';

type File = { id: number; name: string; size: number; media_type: string };
type GiftData = { id: number; title: string; intro: string; mode: 'shared' | 'individual' | 'open'; content: string; files: File[] };
const field = 'w-full rounded-md border border-input bg-background px-3 py-2.5 text-base focus-visible:outline-2 focus-visible:outline-ring';
function fileSize(size:number){if(size<1024)return `${size} B`;const units=['KB','MB','GB','TB'];let value=size/1024,index=0;while(value>=1024&&index<3){value/=1024;index++}return `${value.toFixed(1)} ${units[index]}`;}
class GiftError extends Error { constructor(message: string, public code: number) { super(message); } }
async function api<T>(path: string, body?: object): Promise<T> {
  const response = await fetch(`/api/gifts${path}`, { method: body ? 'POST' : 'GET', credentials: 'include', cache: 'no-store', headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined });
  if (!response.ok) throw new GiftError('连接失败，请稍后重试', response.status);
  const data = await response.json(); if (data.code !== 200) throw new GiftError(data.message, data.code); return data.data;
}
function GiftContents({ gift }: { gift: GiftData }) {
  const [content, setContent] = useState<GiftData | null>(null), [uid, setUID] = useState(''), [password, setPassword] = useState('');
  const [busy, setBusy] = useState(true), [error, setError] = useState(''), [preview, setPreview] = useState<number | null>(null), [retry,setRetry]=useState(0);
  useEffect(() => { let live = true; api<GiftData>(`/${gift.id}`).then(data => { if (live) setContent(data); }).catch(e => { if (live && e.code !== 401) setError(e.message); }).finally(() => { if (live) setBusy(false); }); return () => { live = false; }; }, [gift.id,retry]);
  const unlock = async () => { setBusy(true); setError(''); try { setContent(await api<GiftData>(`/${gift.id}/unlock`, { uid, password })); setPassword(''); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  const lock = async () => { setBusy(true); setError(''); try { await api(`/${gift.id}/lock`, {}); setContent(null); setPreview(null); } catch (e) { setError((e as Error).message); } finally { setBusy(false); } };
  return <div className="border-t border-border pb-6 pt-5">
    {error && <p role="alert" className="mb-4 text-sm text-destructive">{error}</p>}
    {content ? <>
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3"><p role="status" className="text-sm text-primary">{gift.mode==='open'?'无需密码，可直接查看和下载':'礼物已解锁'}</p>{gift.mode!=='open'&&<Button variant="ghost" disabled={busy} onClick={() => void lock()}><LockKeyhole size={15} />重新锁定</Button>}</div>
      {content.content && <p className="mb-6 max-w-prose whitespace-pre-wrap break-words text-sm leading-7">{content.content}</p>}
      {!content.files.length ? <p className="text-sm text-muted-foreground">这份礼物暂时没有附件。</p> : <ul className="divide-y divide-border">{content.files.map(file => {
        const url = `/api/gifts/${gift.id}/files/${file.id}`;
        const canPreview = /^(image\/(png|jpeg|gif|webp)|audio\/(mpeg|wave|ogg)|video\/(mp4|webm))$/.test(file.media_type);
        return <li key={file.id} className="py-4"><div className="flex flex-wrap items-center justify-between gap-3"><div className="min-w-0 flex-1"><p className="break-all font-medium">{file.name}</p><p className="mt-1 text-xs text-muted-foreground">{fileSize(file.size)}</p></div><div className="flex items-center gap-3">{canPreview && <Button variant="outline" size="sm" aria-expanded={preview === file.id} onClick={() => setPreview(preview === file.id ? null : file.id)}>{preview === file.id ? '收起' : '查看'}</Button>}<a href={url} download className="inline-flex min-h-10 items-center gap-1.5 rounded-md px-3 text-sm text-primary underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-ring"><Download size={16} />下载</a></div></div>
          {preview === file.id && <div className="mt-4">{file.media_type.startsWith('image/') ? <img src={`${url}?preview=1`} alt={file.name} className="max-h-[65vh] max-w-full rounded-lg object-contain" onError={() => setError(gift.mode==='open'?'预览加载失败，请刷新页面重试。':'预览加载失败，请重试；若解锁已过期，请重新输入密码。')} /> : file.media_type.startsWith('audio/') ? <audio controls preload="metadata" src={`${url}?preview=1`} className="w-full" /> : <video controls playsInline preload="metadata" src={`${url}?preview=1`} className="max-h-[65vh] w-full rounded-lg" />}</div>}
        </li>;
      })}</ul>}
    </> : busy ? <p role="status" className="text-sm text-muted-foreground">正在读取礼物…</p> : gift.mode==='open' ? <Button variant="outline" onClick={()=>{setBusy(true);setError('');setRetry(n=>n+1)}}>重新加载</Button> : <form className="max-w-md space-y-4" onSubmit={e => { e.preventDefault(); void unlock(); }}>
      <p className="text-sm leading-6 text-muted-foreground">{gift.mode === 'individual' ? '输入你的 B 站 UID 和私信中收到的专属密码。' : '输入私信中收到的礼物密码，即可查看和下载。'}</p>
      {gift.mode === 'individual' && <div className="space-y-2"><label htmlFor={`uid-${gift.id}`} className="text-sm font-medium">B 站 UID</label><input id={`uid-${gift.id}`} className={field} inputMode="numeric" autoComplete="off" pattern="[1-9][0-9]{0,18}" maxLength={19} required value={uid} disabled={busy} onChange={e => setUID(e.target.value)} /></div>}
      <div className="space-y-2"><label htmlFor={`password-${gift.id}`} className="text-sm font-medium">礼物密码</label><input id={`password-${gift.id}`} type="password" className={field} autoComplete="off" autoCapitalize="none" spellCheck={false} maxLength={64} required value={password} disabled={busy} onChange={e => setPassword(e.target.value)} /><p className="text-xs text-muted-foreground">密码区分大小写。</p></div>
      <Button type="submit" disabled={busy}>{busy ? '正在读取…' : '解锁礼物'}</Button>
    </form>}
  </div>;
}

export default function GiftsPage() {
  const [gifts, setGifts] = useState<GiftData[]>([]), [loading, setLoading] = useState(true), [error, setError] = useState(''), [active, setActive] = useState<number | null>(null), [retry, setRetry] = useState(0);
  useEffect(() => { let live = true; setLoading(true); setError(''); api<GiftData[]>('').then(data => { if (live) setGifts(data); }).catch(e => { if (live) setError(e.message); }).finally(() => { if (live) setLoading(false); }); return () => { live = false; }; }, [retry]);
  return <main className="mx-auto min-h-[calc(100svh-56px)] w-full max-w-4xl px-5 pb-16 pt-10 sm:px-8 sm:pt-14">
    <header className="mb-8"><div className="flex items-center gap-3"><Gift size={30} strokeWidth={1.5} className="text-primary" aria-hidden="true" /><h1 className="text-3xl font-semibold">电子礼物</h1></div><p className="mt-4 text-sm leading-6 text-muted-foreground">选择礼物查看内容。需要密码的礼物，请使用私信中收到的密码领取。</p></header>
    {loading ? <p role="status" className="py-12 text-sm text-muted-foreground">正在加载礼物…</p> : error ? <div role="alert"><p>{error}</p><Button variant="outline" className="mt-4" onClick={() => setRetry(n => n + 1)}>重新加载</Button></div> : gifts.length ? <div className="space-y-4">{gifts.map(gift => <article key={gift.id} className="fabric-card rounded-2xl border border-border px-5 sm:px-7">
      <button type="button" className="flex w-full items-center justify-between gap-5 py-6 text-left focus-visible:outline-2 focus-visible:outline-ring" aria-expanded={active === gift.id} aria-controls={`gift-${gift.id}`} onClick={() => setActive(active === gift.id ? null : gift.id)}><span className="min-w-0"><span className="block break-words text-xl font-semibold">{gift.title}</span><span className="mt-1 block text-xs text-muted-foreground">{gift.mode==='open'?'无需密码':gift.mode==='individual'?'UID 与独立密码领取':'凭密码领取'}</span>{gift.intro && <span className="mt-2 block whitespace-pre-wrap break-words text-sm leading-6 text-muted-foreground">{gift.intro}</span>}</span><ChevronDown size={20} className={`shrink-0 text-primary ${active === gift.id ? 'rotate-180' : ''}`} aria-hidden="true" /></button>
      {active === gift.id && <div id={`gift-${gift.id}`}><GiftContents gift={gift} /></div>}
    </article>)}</div> : <p className="py-16 text-center text-muted-foreground">暂时没有上架的礼物。</p>}
  </main>;
}
