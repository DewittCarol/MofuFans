'use client';
import { Button } from '@/components/ui/button';
import { ChevronLeft, ChevronRight } from 'lucide-react';
export function LibraryPagination({page,total,size,onChange,disabled=false}:{page:number;total:number;size:number;onChange:(page:number)=>void;disabled?:boolean}) {
  const pages=Math.max(1,Math.ceil(total/size));
  if(pages===1&&page===1)return null;
  return <nav aria-label="分页" className="mt-7 flex flex-wrap items-center justify-center gap-3 text-sm">
    <Button variant="outline" size="sm" disabled={disabled||page<=1} onClick={()=>onChange(page-1)} aria-label="上一页"><ChevronLeft size={16}/></Button>
    <span className="tabular-nums text-muted-foreground">{page} / {pages} · 共 {total} 条</span>
    <Button variant="outline" size="sm" disabled={disabled||page>=pages} onClick={()=>onChange(page+1)} aria-label="下一页"><ChevronRight size={16}/></Button>
  </nav>;
}
