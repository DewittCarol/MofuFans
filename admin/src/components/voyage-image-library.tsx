'use client';

import { useState } from 'react';
import { Check, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';

export function VoyageImageLibrary({ images, selectedImages, limit, disabled, onSelect }: {
  images: string[]; selectedImages: string[]; limit: number; disabled: boolean; onSelect: (url: string) => void;
}) {
  const [page, setPage] = useState(1);
  const available = [...new Set(images)], pageSize = 24;
  const pages = Math.max(1, Math.ceil(available.length / pageSize)), current = Math.min(page, pages);
  return <section className="space-y-3 border-t border-border pt-4" aria-label="复用本期图片">
    <h3 className="text-sm font-medium">复用本期图片</h3>
    <p className="text-xs leading-6 text-muted-foreground">这里只显示当前卡片上传的图片，可用于海报和多件礼物，不必重复上传。</p>
    {available.length ? <>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">{available.slice((current - 1) * pageSize, current * pageSize).map((url, index) => {
        const selected = selectedImages.includes(url);
        return <button key={url} type="button" disabled={disabled || selected || selectedImages.length >= limit} aria-label={`${selected ? '已添加' : '添加'}图片 ${(current - 1) * pageSize + index + 1}`}
          className={`rounded-lg border p-2 text-xs focus-visible:outline-2 focus-visible:outline-ring disabled:cursor-default ${selected ? 'border-primary bg-secondary text-primary' : 'border-transparent text-primary hover:bg-accent disabled:opacity-50'}`}
          onClick={() => onSelect(url)}>
          <img src={url} alt="本期图片素材" loading="lazy" className="mb-2 h-24 w-full object-contain" />
          <span className="flex items-center justify-center gap-1">{selected ? <Check size={13} /> : <Plus size={13} />}{selected ? '已添加' : '添加图片'}</span>
        </button>;
      })}</div>
      {pages > 1 && <div className="flex flex-wrap items-center justify-center gap-3 pt-2 text-xs tabular-nums"><Button type="button" size="sm" variant="outline" disabled={disabled || current === 1} onClick={() => setPage(current - 1)}>上一页</Button><span>{available.length} 张 · {current} / {pages} 页</span><Button type="button" size="sm" variant="outline" disabled={disabled || current === pages} onClick={() => setPage(current + 1)}>下一页</Button></div>}
    </> : <p className="py-6 text-center text-sm text-muted-foreground">本期还没有图片，可在上方上传。</p>}
    {selectedImages.length >= limit && <p className="text-xs text-muted-foreground">此处已选满 {limit} 张，移除一张后可以继续添加。</p>}
  </section>;
}
