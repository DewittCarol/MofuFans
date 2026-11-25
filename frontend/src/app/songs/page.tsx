'use client';
import { useSiteTheme } from '@/lib/site-theme';
import { useState } from 'react';
import { LibraryUpdated } from '@/components/library-updated';
import { Music2, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { LibraryLoadMore } from '@/components/library-load-more';
import { useInfiniteLibrary } from '@/lib/use-infinite-library';
import { useLibrary, type Song, type SongTag } from '@/lib/library';
import { SongTitle, SongVideos } from '@/components/song-title';
import { SongTags, SongTagFilters } from '@/components/song-tags';

export default function SongPage(){
 const {theme}=useSiteTheme();
  const [performed,setPerformed]=useState(false);const [search,setSearch]=useState('');const [query,setQuery]=useState('');
  const [language,setLanguage]=useState('');const [genre,setGenre]=useState('');
  const catalog=useLibrary<SongTag>('/api/song-tags');
  const {data,items,loading,error,loadMore,hasMore}=useInfiniteLibrary<Song>(`/api/songs?performed=${performed}&search=${encodeURIComponent(query)}&language_id=${language}&genre_id=${genre}`,30);
  return <main className="mx-auto w-full max-w-4xl px-5 pb-16 pt-10 sm:px-8 sm:pt-14">
    <header className="mb-8 flex flex-wrap items-end justify-between gap-3"><div className="flex items-center gap-3">{theme.modules.songs.icon?<img src={theme.modules.songs.icon} alt="" className="h-[30px] w-[30px] object-contain"/>:<Music2 size={30} strokeWidth={1.5} className="text-primary"/>}<h1 className="text-3xl font-semibold">{theme.modules.songs.label}</h1></div><LibraryUpdated value={data?.last_updated_at}/></header>
    <div className="mb-6 flex flex-wrap items-center justify-between gap-4"><div className="flex gap-1 rounded-lg bg-secondary p-1" aria-label="歌曲分类">{[false,true].map(value=><Button key={String(value)} variant={performed===value?'default':'ghost'} size="sm" aria-pressed={performed===value} onClick={()=>{setPerformed(value);}}>{value?'唱过的歌':'全部歌曲'}{data?.counts&&<span className="tabular-nums">{data.counts[value?'performed':'all']}</span>}</Button>)}</div>
      <form role="search" className="flex w-full gap-2 sm:w-72" onSubmit={e=>{e.preventDefault();setQuery(search.trim());}}><Input aria-label="搜索歌名或歌手" placeholder="搜索歌名或歌手" value={search} onChange={e=>setSearch(e.target.value)}/><Button size="icon" variant="outline" aria-label="搜索"><Search size={17}/></Button></form>
    </div>
    <div className="mb-6"><SongTagFilters tags={catalog.data?.items||[]} language={language} genre={genre} disabled={catalog.loading||!!catalog.error} onChange={(kind,id)=>{(kind==='language'?setLanguage:setGenre)(id);}}/>{catalog.error&&<p role="alert" className="mt-2 text-sm text-destructive">{catalog.error}<Button variant="ghost" size="sm" onClick={catalog.reload}>重试</Button></p>}</div>
    {!items.length&&loading?<p role="status" className="py-16 text-center text-muted-foreground">正在加载…</p>:!items.length&&error?<div role="alert" className="py-10"><p>{error}</p><Button variant="outline" className="mt-3" onClick={loadMore}>重试</Button></div>:!items.length?<p className="py-16 text-center text-muted-foreground">{query||language||genre?'没有找到符合条件的歌曲':performed?'暂时没有标记唱过的歌曲':'歌单还没有添加歌曲'}</p>:<div className="overflow-hidden rounded-xl border border-border bg-card">
      {items.map(song=><article key={song.id} className="flex flex-wrap items-center justify-between gap-x-6 gap-y-3 border-b border-border px-5 py-5 last:border-0 sm:px-6"><div className="min-w-0 flex-1"><div className="flex flex-wrap items-center gap-2"><div className="min-w-0 text-lg"><SongTitle song={song}/></div>{song.performed&&<span className="rounded bg-secondary px-2 py-0.5 text-xs text-primary">唱过</span>}</div>{song.artist&&<p className="mt-1 break-words text-sm text-muted-foreground">{song.artist}</p>}<SongTags tags={song.tags}/></div>
      <SongVideos song={song}/></article>)}
    </div>}
    {!!items.length&&<LibraryLoadMore loading={loading} error={error} hasMore={hasMore} onLoad={loadMore}/>}
  </main>;
}
