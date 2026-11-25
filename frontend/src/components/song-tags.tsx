'use client';
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import type { SongTag } from '@/lib/library';

export function SongTags({tags}:{tags?:SongTag[]}) {
  if(!tags?.length)return null;
  return <div className="mt-2 flex flex-wrap gap-1.5">{tags.map(tag=><span key={tag.id} title={tag.kind==='language'?'语言':'流派'} className="rounded bg-secondary px-2 py-0.5 text-xs text-primary">{tag.name}</span>)}</div>;
}

export function SongTagFilters({tags,language,genre,onChange,disabled=false}:{tags:SongTag[];language:string;genre:string;onChange:(kind:'language'|'genre',id:string)=>void;disabled?:boolean}) {
  return <div className="flex flex-wrap items-center gap-2" aria-label="歌曲标签筛选">
    {(['language','genre'] as const).map(kind=><Select key={kind} value={(kind==='language'?language:genre)||'all'} onValueChange={value=>onChange(kind,value==='all'?'':value)} disabled={disabled}>
      <SelectTrigger className="min-w-32" aria-label={kind==='language'?'筛选语言':'筛选流派'}><SelectValue/></SelectTrigger>
      <SelectContent><SelectItem value="all">{kind==='language'?'全部语言':'全部流派'}</SelectItem>{tags.filter(tag=>tag.kind===kind).map(tag=><SelectItem key={tag.id} value={String(tag.id)}>{tag.name}</SelectItem>)}</SelectContent>
    </Select>)}
    {(language||genre)&&<Button type="button" size="sm" variant="ghost" onClick={()=>{onChange('language','');onChange('genre','');}}>清除标签筛选</Button>}
  </div>;
}
