'use client';
import { useSiteTheme } from '@/lib/site-theme';

import { useEffect, useRef, useState } from 'react';
import { ImagePlus, RotateCcw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { getConfig } from '@/lib/api';

export function BackgroundSettings() {
 const {theme}=useSiteTheme();
  const [url, setUrl] = useState('');
  const [opacity, setOpacity] = useState(25);
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState('');
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState(false);
  const input = useRef<HTMLInputElement>(null);
  const load = async () => {
    setMessage('');
    try {
      const res = await getConfig();
      if (res.code !== 200) throw new Error('背景设置加载失败，请重试。');
      setUrl(res.data.background_url || ''); setOpacity(res.data.background_opacity ?? 25); setReady(true);
    } catch { setMessage('背景设置加载失败，请重试。'); setError(true); }
  };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!file) { setPreview(''); return; }
    const value = URL.createObjectURL(file); setPreview(value);
    return () => URL.revokeObjectURL(value);
  }, [file]);
  const choose = (value?: File) => {
    if (!value) return;
    if (!['image/jpeg','image/png','image/webp'].includes(value.type) || value.size > 12*1024*1024) {
      setMessage('请选择 12 MB 以内的 JPG、PNG 或 WebP 图片。'); setError(true); return;
    }
    setFile(value); setDirty(true); setMessage('');
  };
  const save = async () => {
    if (busy) return;
    setBusy(true); setMessage(''); setError(false);
    try {
      let target = url;
      if (file) {
        const body = new FormData(); body.append('file',file);
        const response = await fetch('/api/background-assets',{ method:'POST',credentials:'include',body });
        const result = await response.json();
        if (result.code !== 200) throw new Error(result.message || '背景上传失败。');
        target = result.data.url;
      }
      const response = await fetch('/api/background',{ method:'PUT',credentials:'include',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:target,opacity}) });
      const result = await response.json();
      if (result.code !== 200) throw new Error(result.message || '背景保存失败。');
      setUrl(result.data.url); setFile(null); setDirty(false); setMessage('背景已保存，前台会自动更新。');
    } catch (err) { setError(true); setMessage(err instanceof Error ? err.message : '连接失败，请重试。'); }
    finally { setBusy(false); }
  };
  const image = preview || url;
  return <Card id="background-settings" tabIndex={-1} aria-label="前台背景">
    <CardHeader><CardTitle>前台背景</CardTitle><CardDescription>图片自动填满背景，超出部分会裁切。</CardDescription></CardHeader>
    <CardContent className="space-y-4">
      <div className="relative isolate flex aspect-[16/9] max-w-xl items-center justify-center overflow-hidden rounded-lg border border-border bg-background" aria-label="背景效果预览">
        {image && <img src={image} alt="所选背景" className="absolute inset-0 -z-10 h-full w-full object-cover" style={{opacity:opacity/100}} />}
        <div className="max-w-[80%] rounded-lg bg-card px-5 py-4 text-sm text-foreground shadow-sm"><p className="font-medium">{theme.brand.site_name} {theme.modules.ask.label}</p><p className="mt-1 text-muted-foreground">这是一张提问卡片，看看文字是否清楚。</p></div>
      </div>
      <input ref={input} type="file" accept="image/jpeg,image/png,image/webp" className="sr-only" aria-label="选择前台背景" disabled={!ready || busy} onChange={e=>{choose(e.target.files?.[0]);e.target.value='';}} />
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" disabled={!ready || busy} onClick={()=>input.current?.click()}><ImagePlus size={16}/>{image?'更换图片':'选择图片'}</Button>
        <Button variant="ghost" disabled={!ready || busy || !image} onClick={()=>{setFile(null);setUrl('');setDirty(true);setMessage('');}}><RotateCcw size={16}/>恢复默认背景</Button>
      </div>
      <div className="max-w-xl space-y-2">
        <label htmlFor="background-opacity" className="flex justify-between text-sm"><span>背景清晰度</span><span className="tabular-nums">{opacity}%</span></label>
        <input id="background-opacity" type="range" min="0" max="100" value={opacity} disabled={!ready || busy || !image} className="block h-6 w-full accent-primary" onChange={e=>{setOpacity(Number(e.target.value));setDirty(true);setMessage('');}} />
        <p className="text-xs leading-relaxed text-muted-foreground">0% 完全淡出，100% 显示原图。建议先从 25% 开始。支持 JPG、PNG、WebP，最大 12 MB。</p>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button disabled={!ready || busy || !dirty} onClick={save}>{busy?'保存中…':'保存背景'}</Button>
        {!ready && error && <Button variant="outline" onClick={load}>重新加载</Button>}
        {message && <p role={error?'alert':'status'} className={`text-sm ${error?'text-destructive':'text-primary'}`}>{message}</p>}
      </div>
    </CardContent>
  </Card>;
}
