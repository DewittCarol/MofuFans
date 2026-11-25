'use client';
import {useState} from 'react';
import {Truck} from 'lucide-react';
import {ImagePreview} from '@/components/image-preview';
import {VoyageTierBadge} from '@/components/voyage-tier-badge';
import {voyageTiers,type VoyageIssue,type VoyageGift} from '@/lib/voyage';
export function VoyageIssueContents({issue}:{issue:VoyageIssue}) {
 const [preview,setPreview]=useState<{images:string[];index:number}|null>(null);
 const guard=issue.kind==='guard'?issue.gifts:[];
 const other=issue.kind==='lottery'?issue.gifts:[];
 const shared=issue.images||[];
 const pictures=(images:string[],label:string)=><div className="mt-3 flex flex-wrap gap-2">{images.map((src,index)=><button key={`${src}-${index}`} type="button" onClick={()=>setPreview({images,index})} aria-label={`放大${label}图片 ${index+1}`} className="h-36 w-28 overflow-hidden rounded-md bg-background/70 focus-visible:outline-2 focus-visible:outline-ring sm:h-44 sm:w-36"><img src={src} alt={`${label} ${index+1}`} loading="lazy" className="h-full w-full object-contain"/></button>)}</div>;
 const giftRow=(gift:VoyageGift,index:number)=>{
  const showActivity=issue.kind==='lottery';
  return <div key={index} className="grid gap-3 py-4 first:pt-0 last:pb-0 @min-[620px]:grid-cols-[minmax(0,1fr)_155px] sm:gap-5"><div className="min-w-0"><h4 className="break-words text-sm font-medium">{gift.name}</h4>{showActivity&&gift.audience&&<p className="mt-1 text-xs leading-6 text-muted-foreground">{gift.audience}</p>}{gift.description&&<p className="mt-1 whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">{gift.description}</p>}{!!gift.images.length&&pictures(gift.images,gift.name)}</div><div className="text-xs leading-6 text-muted-foreground">{gift.shipping_note&&<p className="whitespace-pre-wrap break-words"><span className="mr-2 sm:mb-1 sm:block">发货情况</span><span className="text-foreground">{gift.shipping_note}</span></p>}</div></div>;
 };
 return <div className="@container space-y-6 border-t border-border pt-6">{issue.note&&<p className="whitespace-pre-wrap text-sm leading-7 text-muted-foreground">{issue.note}</p>}{!!shared.length&&pictures(shared,'海报')}
 {!!guard.length&&<section><h3 className="mb-3 text-base font-semibold">大航海回馈</h3><div className="divide-y divide-border border-y border-border">{voyageTiers.filter(t=>guard.some(g=>g.tiers.includes(t.id))).map(t=><section key={t.id} className="grid gap-4 py-5 @min-[620px]:grid-cols-[85px_minmax(0,1fr)]"><h4><VoyageTierBadge tier={t.id}/></h4><div className="min-w-0 divide-y divide-border/60">{guard.filter(g=>g.tiers.includes(t.id)).map(giftRow)}</div></section>)}</div></section>}
 {!!other.length&&<section><h3 className="mb-3 text-base font-semibold">活动奖品</h3><div className="divide-y divide-border border-y border-border py-5">{other.map((g,i)=><div key={i} className="py-4 first:pt-0 last:pb-0">{giftRow(g,i)}</div>)}</div></section>}
 {issue.shipping_note&&<section className="text-sm leading-7"><h3 className="mb-1 flex items-center gap-2 font-medium"><Truck size={16}/>发货说明</h3><p className="whitespace-pre-wrap text-muted-foreground">{issue.shipping_note}</p></section>}
 {preview&&<ImagePreview src={preview.images[preview.index]} images={preview.images} currentIndex={preview.index} onIndexChange={index=>setPreview({...preview,index})} onClose={()=>setPreview(null)}/>}
 </div>;
}
