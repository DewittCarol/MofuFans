'use client';
import { useId, useState } from 'react';
import { Check, Plus, X } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { libraryRequest, type SongTag } from '@/lib/library';

function TagGroup({kind,tags,selected,onChange,onCreated,onPending,disabled}:{kind:SongTag['kind'];tags:SongTag[];selected:number[];onChange:(ids:number[])=>void;onCreated:(tag:SongTag)=>void;onPending:(pending:boolean)=>void;disabled:boolean}) {
  const label=kind==='language'?'语言':'流派';const id=useId();
  const [adding,setAdding]=useState(false);const [name,setName]=useState('');const [busy,setBusy]=useState(false);const [error,setError]=useState('');
  const [localTags,setLocalTags]=useState<SongTag[]>([]);
  const choices=[...tags.filter(tag=>tag.kind===kind),...localTags.filter(tag=>!tags.some(item=>item.id===tag.id))];
  async function create(){
    if(busy||disabled)return;setError('');if(!name.trim()){setError('请填写标签名');return;}setBusy(true);onPending(true);
    try{const tag=await libraryRequest<SongTag>('/api/admin/song-tags','POST',{kind,name:name.trim()});setLocalTags(items=>items.some(item=>item.id===tag.id)?items:[...items,tag]);onCreated(tag);setName('');setAdding(false);}
    catch(error){setError((error as Error).message);}finally{setBusy(false);onPending(false);}
  }
  return <fieldset disabled={disabled||busy} className="min-w-0 space-y-2"><legend className="mb-2 text-sm font-medium">{label}<span className="ml-2 font-normal text-muted-foreground">可多选</span></legend>
    <div className="flex flex-wrap gap-2">{choices.map(tag=><button key={tag.id} type="button" aria-pressed={selected.includes(tag.id)} onClick={()=>onChange(selected.includes(tag.id)?selected.filter(value=>value!==tag.id):[...selected,tag.id])} className={`inline-flex items-center gap-1 rounded-lg border px-2.5 py-1.5 text-sm focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50 ${selected.includes(tag.id)?'border-primary bg-primary text-primary-foreground':'border-border bg-background hover:bg-secondary'}`}>{selected.includes(tag.id)&&<Check size={13}/>} {tag.name}</button>)}<Button type="button" size="sm" variant="ghost" onClick={()=>{setAdding(!adding);setError('');}} aria-expanded={adding}>{adding?<X size={14}/>:<Plus size={14}/>}新建{label}</Button></div>
    {adding&&<div className="flex max-w-sm gap-2"><label htmlFor={id} className="sr-only">新{label}标签名</label><Input id={id} autoFocus placeholder={`新${label}标签`} maxLength={24} value={name} onChange={event=>setName(event.target.value)} onKeyDown={event=>{if(event.key==='Enter'&&!event.nativeEvent.isComposing){event.preventDefault();event.stopPropagation();void create();}}}/><Button type="button" onClick={create}>{busy?'创建中…':'创建'}</Button></div>}
    {error&&<p role="alert" className="text-sm text-destructive">{error}</p>}
  </fieldset>;
}

export function SongTagPicker(props:{tags:SongTag[];selected:number[];onChange:(ids:number[])=>void;onCreated:(tag:SongTag)=>void;onPending:(pending:boolean)=>void;disabled:boolean}) {
  return <div className="space-y-5"><TagGroup kind="language" {...props}/><TagGroup kind="genre" {...props}/></div>;
}
