'use client';
import { useEffect, useRef, useState } from 'react';
import { Music2, Plus, Search, Pencil, Trash2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Checkbox } from '@/components/ui/checkbox';
import { Dialog, DialogContent, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { LibraryPagination } from '@/components/library-pagination';
import { libraryRequest, useLibrary, type Song, type SongTag } from '@/lib/library';
import { SongTitle, SongVideos } from '@/components/song-title';
import { SongTags, SongTagFilters } from '@/components/song-tags';
import { SongTagPicker } from '@/components/song-tag-picker';

type Draft={id?:number;title:string;artist:string;performed:boolean;videos:string;tag_ids:number[]};
const empty:Draft={title:'',artist:'',performed:false,videos:'',tag_ids:[]};
export default function SongsPage(){
  const [page,setPage]=useState(1);const [performed,setPerformed]=useState(false);const [search,setSearch]=useState('');const [query,setQuery]=useState('');
  const [draft,setDraft]=useState<Draft|null>(null);const [busy,setBusy]=useState(false);const [notice,setNotice]=useState('');const [error,setError]=useState('');const [deleting,setDeleting]=useState<Song|null>(null);
  const form=useRef<HTMLFormElement>(null);
  const [language,setLanguage]=useState('');const [genre,setGenre]=useState('');const [tagBusy,setTagBusy]=useState(false);
  const catalog=useLibrary<SongTag>('/api/song-tags');
  const {data,loading,error:loadError,reload}=useLibrary<Song>(`/api/songs?page=${page}&page_size=20&performed=${performed}&search=${encodeURIComponent(query)}&language_id=${language}&genre_id=${genre}`);
  useEffect(()=>{if(data&&!loading&&page>Math.max(1,Math.ceil(data.total/20)))setPage(Math.max(1,Math.ceil(data.total/20)));},[data,loading,page]);
  async function save(event:React.FormEvent){
    event.preventDefault();if(!draft||tagBusy)return;setError('');setNotice('');
    if(!draft.title.trim()){setError('请填写歌名');return;}setBusy(true);
    try{await libraryRequest(draft.id?`/api/admin/songs/${draft.id}`:'/api/admin/songs',draft.id?'PUT':'POST',{...draft,videos:draft.videos.split(/\r?\n/).map(v=>v.trim()).filter(Boolean)});setDraft(null);setNotice('歌曲已保存');reload();}
    catch(error){setError((error as Error).message);}finally{setBusy(false);}
  }
  async function remove(){if(!deleting)return;setBusy(true);setError('');try{await libraryRequest(`/api/admin/songs/${deleting.id}`,'DELETE');if(draft?.id===deleting.id)setDraft(null);setDeleting(null);setNotice('歌曲已删除');reload();}catch(error){setError((error as Error).message);}finally{setBusy(false);}}
  return <div className="space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-3"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Music2 size={25}/>歌单管理</h1><Button disabled={busy||tagBusy} onClick={()=>{setDraft({...empty});setError('');setNotice('');}}><Plus size={16}/>添加歌曲</Button></header>
    {notice&&<p role="status" className="text-sm text-primary">{notice}</p>}{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
    {draft&&<form ref={form} onSubmit={save} noValidate className="space-y-4 rounded-xl border border-border bg-card p-5 sm:p-6">
      <div className="flex items-center justify-between"><h2 className="font-semibold">{draft.id?'编辑歌曲':'添加歌曲'}</h2><Button variant="ghost" size="icon" type="button" disabled={busy||tagBusy} aria-label="关闭编辑" onClick={()=>setDraft(null)}><X size={18}/></Button></div>
      <div className="grid gap-4 sm:grid-cols-2"><label className="space-y-2 text-sm"><span>歌名</span><Input autoFocus required maxLength={100} value={draft.title} disabled={busy||tagBusy} onChange={e=>setDraft({...draft,title:e.target.value})}/></label><label className="space-y-2 text-sm"><span>歌手（选填）</span><Input maxLength={100} value={draft.artist} disabled={busy||tagBusy} onChange={e=>setDraft({...draft,artist:e.target.value})}/></label></div>
      <label className="block space-y-2 text-sm"><span>关联歌切（选填）</span><textarea value={draft.videos} disabled={busy||tagBusy} onChange={e=>setDraft({...draft,videos:e.target.value})} rows={3} className="w-full resize-y rounded-lg border border-input bg-background p-3 outline-none focus-visible:ring-2 focus-visible:ring-ring" placeholder="粘贴 BV 号或歌切的 B 站完整链接，每行一个"/></label>
      <SongTagPicker tags={catalog.data?.items||[]} selected={draft.tag_ids} onPending={setTagBusy} disabled={busy||tagBusy||catalog.loading||!!catalog.error} onChange={ids=>setDraft(current=>current?{...current,tag_ids:ids}:current)} onCreated={tag=>{setDraft(current=>current?{...current,tag_ids:current.tag_ids.includes(tag.id)?current.tag_ids:[...current.tag_ids,tag.id]}:current);catalog.reload();}}/>
      <label className="flex items-center gap-2 text-sm"><Checkbox checked={draft.performed||!!draft.videos.trim()} disabled={busy||!!draft.videos.trim()} onCheckedChange={value=>setDraft({...draft,performed:value===true})}/>主播唱过这首歌</label>
      <p className="text-xs text-muted-foreground">关联视频后会自动归入“唱过的歌”；没有视频也可以手动标记。</p>
      <div className="flex gap-2"><Button disabled={busy||tagBusy}>{busy?'正在保存…':'保存歌曲'}</Button><Button type="button" variant="outline" disabled={busy||tagBusy} onClick={()=>setDraft(null)}>取消</Button></div>
    </form>}
    <div className="flex flex-wrap items-center justify-between gap-3"><div className="flex gap-1 rounded-lg border border-border bg-card p-1" aria-label="歌曲分类">{[false,true].map(value=><Button key={String(value)} size="sm" variant={performed===value?'default':'ghost'} aria-pressed={performed===value} onClick={()=>{setPerformed(value);setPage(1);}}>{value?'唱过的歌':'全部歌曲'}{data?.counts&&<span className="tabular-nums">{data.counts[value?'performed':'all']}</span>}</Button>)}</div><form role="search" onSubmit={e=>{e.preventDefault();setQuery(search.trim());setPage(1);}} className="flex w-full gap-2 sm:w-72"><Input aria-label="搜索歌名或歌手" placeholder="搜索歌名或歌手" value={search} onChange={e=>setSearch(e.target.value)}/><Button variant="outline" size="icon" aria-label="搜索"><Search size={16}/></Button></form></div>
    <SongTagFilters tags={catalog.data?.items||[]} language={language} genre={genre} disabled={catalog.loading||!!catalog.error} onChange={(kind,id)=>{(kind==='language'?setLanguage:setGenre)(id);setPage(1);}}/>
    {catalog.error&&<p role="alert" className="text-sm text-destructive">{catalog.error}<Button variant="ghost" size="sm" onClick={catalog.reload}>重试标签加载</Button></p>}
    {loading?<p role="status" className="py-12 text-center text-muted-foreground">正在加载…</p>:loadError?<div role="alert"><p>{loadError}</p><Button variant="outline" className="mt-3" onClick={reload}>重试</Button></div>:!data?.items.length?<p className="py-12 text-center text-muted-foreground">{query||language||genre?'没有找到符合条件的歌曲':'暂无歌曲'}</p>:<div className="overflow-hidden rounded-xl border border-border bg-card">{data.items.map(song=><article key={song.id} className="flex flex-wrap items-center justify-between gap-4 border-b border-border p-5 last:border-0"><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><SongTitle song={song}/>{song.performed&&<span className="rounded bg-secondary px-2 py-0.5 text-xs text-primary">唱过</span>}</div>{song.artist&&<p className="mt-1 break-words text-sm text-muted-foreground">{song.artist}</p>}<SongTags tags={song.tags}/></div><div className="flex flex-wrap items-center gap-2"><SongVideos song={song}/><Button variant="outline" size="sm" disabled={busy||tagBusy} onClick={()=>{setError('');setDraft({...song,videos:(song.videos||[]).join('\n'),tag_ids:(song.tags||[]).map(tag=>tag.id)});requestAnimationFrame(()=>form.current?.scrollIntoView({block:'start'}));}}><Pencil size={14}/>编辑</Button><Button variant="ghost" size="sm" disabled={busy||tagBusy} onClick={()=>setDeleting(song)}><Trash2 size={14}/>删除</Button></div></article>)}</div>}
    {data&&<LibraryPagination page={page} total={data.total} size={20} onChange={setPage} disabled={loading||busy}/>}
    <Dialog open={!!deleting} onOpenChange={open=>{if(!open&&!busy)setDeleting(null);}}><DialogContent><DialogTitle>删除歌曲？</DialogTitle><DialogDescription>将从歌单中删除「{deleting?.title}」。</DialogDescription>{error&&<p role="alert" className="text-sm text-destructive">{error}</p>}<DialogFooter><Button variant="outline" disabled={busy||tagBusy} onClick={()=>setDeleting(null)}>取消</Button><Button variant="destructive" disabled={busy||tagBusy} onClick={remove}>确认删除</Button></DialogFooter></DialogContent></Dialog>
  </div>;
}
