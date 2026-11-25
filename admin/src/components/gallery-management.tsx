'use client';
import { useEffect, useState } from 'react';
import { Images, Plus, Check, X, Pencil, Trash2, ExternalLink } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { GalleryUpload } from '@/components/gallery-upload';
import { ImagePreview } from '@/components/image-preview';
import { ModerationAttachments } from '@/components/moderation-attachments';
import { LibraryPagination } from '@/components/library-pagination';
import { useModerationPreferences } from '@/lib/use-moderation-preferences';
import { libraryRequest, useLibrary, gallerySourceURL, type GalleryWork } from '@/lib/library';
import { galleryImageURL } from '@/lib/image-variants';

export function GalleryManagement({source}:{source:'streamer'|'fan'}){
  const fan=source==='fan';const [page,setPage]=useState(1);const [status,setStatus]=useState(fan?'pending':'published');
  const [adding,setAdding]=useState<GalleryWork|null>(null);const [upload,setUpload]=useState(false);const [busy,setBusy]=useState(false);const [error,setError]=useState('');const [notice,setNotice]=useState('');
  const [editing,setEditing]=useState<{work:GalleryWork;title:string;sourceURL:string}|null>(null);const [deleting,setDeleting]=useState<GalleryWork|null>(null);
  const [preview,setPreview]=useState<{images:string[];index:number;review:boolean}|null>(null);const protection=useModerationPreferences();
  const {data,loading,error:loadError,reload}=useLibrary<GalleryWork>(`/api/admin/gallery?source=${source}&status=${status}&page=${page}&page_size=12`);
  useEffect(()=>{if(data&&!loading&&page>Math.max(1,Math.ceil(data.total/12)))setPage(Math.max(1,Math.ceil(data.total/12)));},[data,loading,page]);
  async function update(work:GalleryWork,body:{status?:string;title?:string;source_url?:string}){
    if(busy)return;
    if(body.source_url!==undefined){try{body.source_url=gallerySourceURL(body.source_url);}catch(error){setError((error as Error).message);return;}}
    setBusy(true);setError('');setNotice('');
    try{await libraryRequest(`/api/admin/gallery/${work.id}`,'PUT',{...body,version:work.version});setEditing(null);setNotice(body.status==='published'?'作品已展示':body.status==='rejected'?(fan?'已标记未通过':'作品已隐藏'):'作品信息已保存');reload();}
    catch(error){setError((error as Error).message);}finally{setBusy(false);}
  }
  async function remove(){if(!deleting)return;setBusy(true);setError('');try{await libraryRequest(`/api/admin/gallery/${deleting.id}`,'DELETE');if(editing?.work.id===deleting.id)setEditing(null);setDeleting(null);setNotice('作品已删除');reload();}catch(error){setError((error as Error).message);}finally{setBusy(false);}}
  const isProtected=(src:string,review:boolean)=>review&&protection.isProtected(src);
  function open(images:string[],index:number,review:boolean){if(isProtected(images[index],review)){protection.reveal(images[index]);return;}setPreview({images,index,review});}
  const tabs=fan?[{key:'pending',label:'待审核'},{key:'published',label:'已展示'},{key:'rejected',label:'未通过'}]:[{key:'published',label:'已展示'},{key:'rejected',label:'已隐藏'}];
  return <div className="space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Images size={25}/>{fan?'粉丝约稿':'主播约稿'}</h1>{<Button onClick={()=>setUpload(value=>!value)}>{upload?<X size={16}/>:<Plus size={16}/>} {upload?'收起上传':'上传作品'}</Button>}</header>
    {upload&&<GalleryUpload admin source={source} onDone={()=>{setStatus('published');setPage(1);reload();}}/>}
    <div className="flex flex-wrap items-center justify-between gap-4"><div className="flex flex-wrap gap-1 rounded-lg border border-border bg-card p-1" aria-label="审核状态">{tabs.map(tab=><Button key={tab.key} variant={status===tab.key?'default':'ghost'} size="sm" aria-pressed={status===tab.key} onClick={()=>{setStatus(tab.key);setPage(1);setEditing(null);setAdding(null);setPreview(null);}}>{tab.label}<span className="tabular-nums">{data?.counts?.[tab.key]??0}</span></Button>)}</div>
      {fan&&status==='pending'&&<label className="flex items-center gap-2 text-sm"><Checkbox disabled={!protection.ready} checked={protection.preferences.blurEnabled} onCheckedChange={value=>protection.savePreferences({blurEnabled:value===true})}/>待审核图片首次查看时模糊</label>}
    </div>
    {notice&&<p role="status" className="text-sm text-primary">{notice}</p>}{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}{fan&&status==='pending'&&protection.storageWarning&&<p role="status" className="text-sm text-muted-foreground">{protection.storageWarning}</p>}
    {loading?<p role="status" className="py-12 text-center text-muted-foreground">正在加载…</p>:loadError?<div role="alert"><p>{loadError}</p><Button variant="outline" className="mt-3" onClick={reload}>重试</Button></div>:!data?.items.length?<p className="py-12 text-center text-muted-foreground">{status==='pending'?'没有待审核作品':'暂无作品'}</p>:<div className="space-y-4">{data.items.map(work=><article key={work.id} className="space-y-4 rounded-xl border border-border bg-card p-5">
      <div className="flex flex-wrap items-start justify-between gap-3"><div className={editing?.work.id===work.id?"min-w-0 basis-full":"min-w-0 flex-1"}>{editing?.work.id===work.id?<form className="space-y-3" onSubmit={event=>{event.preventDefault();void update(work,{title:editing.title,source_url:editing.sourceURL});}}>
        <div className="space-y-1.5"><label htmlFor={`gallery-title-${work.id}`} className="text-sm">作品名</label><Input id={`gallery-title-${work.id}`} autoFocus value={editing.title} maxLength={100} disabled={busy} onChange={e=>setEditing({...editing,title:e.target.value})}/></div>
        <div className="space-y-1.5"><label htmlFor={`gallery-source-${work.id}`} className="text-sm">原动态链接（选填）</label><Input id={`gallery-source-${work.id}`} inputMode="url" value={editing.sourceURL} maxLength={500} disabled={busy} placeholder="https://t.bilibili.com/…" onChange={e=>setEditing({...editing,sourceURL:e.target.value})}/></div>
        <div className="flex gap-2"><Button size="sm" disabled={busy}>保存</Button><Button type="button" size="sm" variant="ghost" disabled={busy} onClick={()=>setEditing(null)}>取消</Button></div>
      </form>:<h2 className="break-words font-medium">{work.title||'未命名作品'}</h2>}{editing?.work.id!==work.id&&work.source_url&&<a href={work.source_url} target="_blank" rel="noreferrer" className="mt-2 inline-flex items-center gap-1 text-sm text-primary underline underline-offset-4">原动态<ExternalLink size={13}/></a>}
      <p className="mt-1 text-xs text-muted-foreground">#{work.id} · {new Date(work.created_at).toLocaleString('zh-CN')}</p></div>
      <div className="flex flex-wrap gap-2">{work.status!=='published'&&<Button size="sm" disabled={busy||loading} onClick={()=>update(work,{status:'published'})}><Check size={14}/>{fan?'通过并展示':'展示作品'}</Button>}{work.status!=='rejected'&&<Button variant="outline" size="sm" disabled={busy||loading} onClick={()=>update(work,{status:'rejected'})}><X size={14}/>{fan&&work.status==='pending'?'不通过':'隐藏作品'}</Button>}<Button variant="ghost" size="sm" disabled={busy} onClick={()=>setEditing({work,title:work.title,sourceURL:work.source_url||''})}><Pencil size={14}/>编辑</Button><Button variant="outline" size="sm" disabled={busy||work.images.length>=24} onClick={()=>setAdding(adding?.id===work.id?null:work)}><Plus size={14}/>添加图片</Button><Button variant="ghost" size="sm" disabled={busy} onClick={()=>setDeleting(work)}><Trash2 size={14}/>删除</Button></div></div>
      {adding?.id===work.id&&<GalleryUpload admin work={work} onDone={()=>{setAdding(null);reload();}}/>}
      <p className="text-xs text-muted-foreground">{work.images.length} 张图片</p>
      <ModerationAttachments images={work.images.map(image=>image.url)} displaySrc={src=>galleryImageURL(src,'thumb')} isProtected={src=>isProtected(src,fan&&work.status==='pending')} onOpen={(images,index)=>open(images,index,fan&&work.status==='pending')}/>
    </article>)}</div>}
    {data&&<LibraryPagination page={page} total={data.total} size={12} onChange={setPage} disabled={loading||busy}/>}
    {preview&&<ImagePreview src={galleryImageURL(preview.images[preview.index],'preview')} originalSrc={preview.images[preview.index]} images={preview.images} currentIndex={preview.index} protectedImage={isProtected(preview.images[preview.index],preview.review)} onReviewThumbnail={()=>{protection.reveal(preview.images[preview.index]);setPreview(null);}} onIndexChange={index=>setPreview({...preview,index})} onClose={()=>setPreview(null)}/>}
    <Dialog open={!!deleting} onOpenChange={open=>{if(!open&&!busy)setDeleting(null);}}><DialogContent><DialogTitle>删除作品？</DialogTitle><DialogDescription>「{deleting?.title||'未命名作品'}」及其图片将被删除。</DialogDescription>{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}<DialogFooter><Button variant="outline" disabled={busy} onClick={()=>setDeleting(null)}>取消</Button><Button variant="destructive" disabled={busy} onClick={remove}>确认删除</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
