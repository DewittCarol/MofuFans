'use client';
import { useSiteTheme } from '@/lib/site-theme';
import { useState } from 'react';
import { LibraryUpdated } from '@/components/library-updated';
import { Images, Plus, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { GalleryUpload } from '@/components/gallery-upload';
import { ImagePreview } from '@/components/image-preview';
import { LibraryLoadMore } from '@/components/library-load-more';
import { GalleryColumns } from '@/components/gallery-columns';
import { useInfiniteLibrary } from '@/lib/use-infinite-library';
import { type GalleryWork } from '@/lib/library';
import { galleryImageURL } from '@/lib/image-variants';

export default function GalleryPage(){
 const {theme}=useSiteTheme();
  const [source,setSource]=useState<'streamer'|'fan'>('streamer');const [upload,setUpload]=useState(false);
  const [preview,setPreview]=useState<{images:string[];index:number}|null>(null);
  const {data,items,loading,error,loadMore,hasMore,reload}=useInfiniteLibrary<GalleryWork>(`/api/gallery?source=${source}`,18);
  return <main className="mx-auto w-full max-w-6xl px-5 pb-16 pt-10 sm:px-8 sm:pt-14">
    <header className="mb-8 flex flex-wrap items-end justify-between gap-3"><div className="flex items-center gap-3">{theme.modules.gallery.icon?<img src={theme.modules.gallery.icon} alt="" className="h-[30px] w-[30px] object-contain"/>:<Images size={30} strokeWidth={1.5} className="text-primary"/>}<h1 className="text-3xl font-semibold">{theme.modules.gallery.label}</h1></div><LibraryUpdated value={data?.last_updated_at}/></header>
    <div className="mb-6 flex flex-wrap items-center justify-between gap-4"><div className="flex gap-1 rounded-lg bg-secondary p-1" aria-label="画廊分类">{(['streamer','fan'] as const).map(value=><Button key={value} size="sm" variant={source===value?'default':'ghost'} aria-pressed={source===value} onClick={()=>{setSource(value);}}>{value==='streamer'?'主播约稿':'粉丝约稿'}</Button>)}</div>
    <Button variant="outline" onClick={()=>setUpload(value=>!value)} aria-expanded={upload}>{upload?<X size={16}/>:<Plus size={16}/>} {upload?'收起投稿':'投稿作品'}</Button></div>
    {upload&&<div className="mb-8"><GalleryUpload onDone={()=>{setSource('fan');reload();}}/></div>}
    {!items.length&&loading?<p role="status" className="py-16 text-center text-muted-foreground">正在加载…</p>:!items.length&&error?<div role="alert"><p>{error}</p><Button variant="outline" className="mt-3" onClick={loadMore}>重试</Button></div>:!items.length?<p className="py-16 text-center text-muted-foreground">暂无作品</p>:<GalleryColumns items={items}>{work=><article key={work.id} className="mb-6 break-inside-avoid overflow-hidden rounded-xl bg-card">
      <button type="button" onClick={()=>setPreview({images:work.images.map(image=>image.url),index:0})} aria-label={`查看${work.title||'作品'}`} className="relative block w-full bg-secondary focus-visible:outline-2 focus-visible:outline-offset-[-4px] focus-visible:outline-ring">
        <img src={galleryImageURL(work.images[0]?.url,'thumb')} alt={work.title||'约稿作品'} width={work.images[0]?.width} height={work.images[0]?.height} loading="lazy" decoding="async" className="h-auto max-h-[600px] w-full object-contain"/>
        {work.images.length>1&&<span className="absolute bottom-3 right-3 rounded bg-card px-2 py-1 text-xs">{work.images.length} 张</span>}
      </button>{work.images.length>1&&<div className="flex gap-1 overflow-x-auto px-3 pt-3" aria-label="作品图片">{work.images.map((im,i)=><button key={im.id} type="button" aria-label={`查看第 ${i+1} 张图片`} onClick={()=>setPreview({images:work.images.map(image=>image.url),index:i})} className="h-12 w-12 shrink-0 overflow-hidden rounded bg-secondary focus-visible:outline-2 focus-visible:outline-ring"><img src={galleryImageURL(im.url,'small')} loading="lazy" decoding="async" alt="" className="h-full w-full object-cover"/></button>)}</div>}{work.title&&<h2 className="break-words px-4 py-3 text-sm font-medium">{work.title}</h2>}
      {(work.credit||work.source_url)&&<p className="px-4 pb-4 text-xs leading-5 text-muted-foreground">{work.credit}{work.source_url&&<a href={work.source_url} target="_blank" rel="noreferrer" className="ml-2 text-primary underline underline-offset-4">原动态</a>}</p>}
    </article>}</GalleryColumns>}
    {!!items.length&&<LibraryLoadMore loading={loading} error={error} hasMore={hasMore} onLoad={loadMore}/>}
    {preview&&<ImagePreview src={galleryImageURL(preview.images[preview.index],'preview')!} originalSrc={preview.images[preview.index]} images={preview.images} currentIndex={preview.index} onIndexChange={index=>setPreview({...preview,index})} onClose={()=>setPreview(null)}/>}
  </main>;
}
