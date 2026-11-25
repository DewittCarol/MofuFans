'use client';
import {useEffect,useMemo,useState,type ReactNode} from 'react';
import type {GalleryWork} from '@/lib/library';

export function GalleryColumns({items,children}:{items:GalleryWork[];children:(work:GalleryWork)=>ReactNode}) {
  const [count,setCount]=useState(1);
  useEffect(()=>{
    const medium=matchMedia('(min-width: 640px)'),large=matchMedia('(min-width: 1024px)');
    const update=()=>setCount(large.matches?3:medium.matches?2:1);
    update();medium.addEventListener('change',update);large.addEventListener('change',update);
    return()=>{medium.removeEventListener('change',update);large.removeEventListener('change',update);};
  },[]);
  const columns=useMemo(()=>{
    const result:GalleryWork[][]=Array.from({length:count},()=>[]),heights=Array(count).fill(0);
    for(const work of items){
      const column=heights.indexOf(Math.min(...heights));
      result[column].push(work);
      const image=work.images[0];
      heights[column]+=Math.min(600,image?.width?320*image.height/image.width:320)+(work.images.length>1?60:0)+(work.title?48:0)+(work.credit||work.source_url?64:0)+24;
    }
    return result;
  },[items,count]);
  // CSS multicol would rebalance every old card on append. Assign columns using
  // stable image dimensions, so only newly loaded cards are placed on scrolling.
  return <div className="grid items-start gap-6" style={{gridTemplateColumns:`repeat(${count},minmax(0,1fr))`}}>{columns.map((column,i)=><div key={i} className="min-w-0">{column.map(children)}</div>)}</div>;
}
