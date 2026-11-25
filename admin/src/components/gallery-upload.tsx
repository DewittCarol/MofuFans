'use client';
import { useEffect, useId, useRef, useState } from 'react';
import { Upload, X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { libraryRequest, gallerySourceURL } from '@/lib/library';

export function GalleryUpload({ admin=false, source="streamer", work, onDone }: { admin?:boolean; source?:"streamer"|"fan"; work?:{id:number;version:number}; onDone?:()=>void }) {
  const id=useId(); const input=useRef<HTMLInputElement>(null);
  const [title,setTitle]=useState(''); const [files,setFiles]=useState<File[]>([]);
  const [sourceURL,setSourceURL]=useState('');
  const [previews,setPreviews]=useState<string[]>([]);const [busy,setBusy]=useState(false);
  const [error,setError]=useState('');const [success,setSuccess]=useState('');
  useEffect(()=>{const urls=files.map(file=>URL.createObjectURL(file));setPreviews(urls);return()=>urls.forEach(url=>URL.revokeObjectURL(url));},[files]);
  async function submit(event:React.FormEvent){
    event.preventDefault();setError('');setSuccess('');
    if(!files.length){setError('请选择图片');return;}
    let link='';
    if(admin&&!work){try{link=gallerySourceURL(sourceURL);}catch(error){setError((error as Error).message);return;}}
    setBusy(true);const form=new FormData();form.append('title',title);form.append('source',source);if(work)form.append('version',String(work.version));files.forEach(file=>form.append('images',file));
    if(admin&&!work)form.append('source_url',link);
    try {await libraryRequest(work?`/api/admin/gallery/${work.id}/images`:admin?'/api/admin/gallery':'/api/gallery','POST',form);setTitle('');setSourceURL('');setFiles([]);setSuccess(work?'图片已加入作品':admin?'作品已发布':'已提交，审核通过后会展示在粉丝约稿中。');onDone?.();}
    catch(error){setError((error as Error).message);}finally{setBusy(false);}
  }
  return <form onSubmit={submit} className="space-y-4 rounded-xl border border-border bg-card p-5 sm:p-6" aria-label={work?'添加作品图片':admin?'上传作品':'投稿粉丝约稿'}>
    {!work&&<div><label htmlFor={`${id}-title`} className="mb-2 block text-sm font-medium">作品名 <span className="font-normal text-muted-foreground">（选填）</span></label><Input id={`${id}-title`} maxLength={100} value={title} onChange={e=>setTitle(e.target.value)} disabled={busy} placeholder="作品名"/></div>}
    {admin&&!work&&<div><label htmlFor={`${id}-source`} className="mb-2 block text-sm font-medium">原动态链接 <span className="font-normal text-muted-foreground">（选填）</span></label><Input id={`${id}-source`} inputMode="url" maxLength={500} value={sourceURL} onChange={e=>setSourceURL(e.target.value)} disabled={busy} placeholder="https://t.bilibili.com/…"/></div>}
    <div className="space-y-3"><input id={`${id}-files`} ref={input} className="sr-only" type="file" accept="image/jpeg,image/png,image/webp,image/gif" multiple disabled={busy} onChange={event=>{
      const next=[...files,...Array.from(event.target.files||[])];event.target.value='';setSuccess('');
      if(next.length>9){setError('每件作品最多 9 张图片');return;}
      if(next.some(f=>!['image/jpeg','image/png','image/webp','image/gif'].includes(f.type)||f.size>12*1024*1024)){setError('请选择不超过 12 MB 的 JPG、PNG、WebP 或 GIF 图片');return;}
      if(next.reduce((sum,f)=>sum+f.size,0)>36*1024*1024){setError('图片总大小不能超过 36 MB');return;}
      setError('');setFiles(next);
    }}/>
      <Button type="button" variant="outline" disabled={busy||files.length>=9} onClick={()=>input.current?.click()}><Upload size={16}/>选择图片</Button>
      <p className="text-xs leading-relaxed text-muted-foreground">最多 9 张，每张不超过 12 MB，总计不超过 36 MB。</p>
      {!!files.length&&<div className="grid grid-cols-3 gap-3 sm:grid-cols-6">{files.map((file,index)=><div key={`${file.name}-${index}`} className="relative aspect-square overflow-hidden rounded-lg bg-secondary"><img src={previews[index]} alt={file.name} className="h-full w-full object-cover"/>
        <button type="button" disabled={busy} onClick={()=>setFiles(current=>current.filter((_,i)=>i!==index))} aria-label={`移除 ${file.name}`} className="absolute right-1 top-1 rounded bg-card p-1 text-foreground focus-visible:outline-2 focus-visible:outline-ring"><X size={16}/></button></div>)}</div>}
    </div>
    {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}{success&&<p role="status" className="text-sm text-primary">{success}</p>}
    <Button type="submit" disabled={busy||!files.length}>{busy?'正在上传…':work?'添加到此作品':admin?'上传并发布':'提交审核'}</Button>
  </form>;
}
