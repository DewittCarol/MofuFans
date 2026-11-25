'use client';
import { useEffect, useState } from 'react';
import { Copy, Check, ExternalLink } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { videoURL, type Song } from '@/lib/library';

async function copyText(text:string) {
  try { await navigator.clipboard.writeText(text); return; } catch { /* Fall back for older or non-secure browsers. */ }
  const input=document.createElement('textarea');
  input.value=text;input.readOnly=true;input.style.cssText='position:fixed;left:-9999px;top:0;';
  const previous=document.activeElement as HTMLElement|null;
  document.body.appendChild(input);input.select();
  try { if(!document.execCommand('copy'))throw new Error('copy failed'); }
  finally { input.remove();previous?.focus({preventScroll:true}); }
}

export function SongTitle({song}:{song:Song}) {
  const [status,setStatus]=useState<'idle'|'copied'|'failed'>('idle');
  const text=`[${song.title}${song.artist?`-${song.artist}`:''}]`;
  async function copy(){try{await copyText(text);setStatus('copied');}catch{setStatus('failed');}}
  useEffect(()=>{if(status==='idle')return;const timer=setTimeout(()=>setStatus('idle'),3000);return()=>clearTimeout(timer);},[status]);
  return <div className="min-w-0">
    <h2 className="font-medium"><button type="button" title={`复制 ${text}`} aria-label={`复制 ${text}`} className="group inline-flex max-w-full items-center gap-2 rounded text-left text-inherit underline-offset-4 hover:text-primary hover:underline focus-visible:outline-2 focus-visible:outline-ring" onClick={copy}>
      <span className="break-words [overflow-wrap:anywhere]">{song.title}</span>{status==='copied'?<Check size={14} className="shrink-0 text-primary"/>:<Copy size={14} className="shrink-0 text-muted-foreground opacity-60 group-hover:opacity-100"/>}
    </button></h2>
    <span role="status" className={status==='idle'?'sr-only':'mt-1 block text-xs text-primary'}>{status==='copied'?'已复制':status==='failed'?`复制失败，可手动复制：${text}`:''}</span>
  </div>;
}

export function SongVideos({song}:{song:Song}) {
  if(!song.videos?.length)return null;
  return <div className="flex flex-wrap gap-2">{song.videos.map((bv,index)=><Button key={bv} variant="outline" size="sm" asChild><a href={videoURL(bv)} target="_blank" rel="noopener noreferrer" title={bv} aria-label={`观看 ${song.title} 的歌切${song.videos.length>1?` ${index+1}`:''}`}><ExternalLink size={14}/>{song.videos.length>1?`歌切 ${index+1}`:'歌切'}</a></Button>)}</div>;
}
