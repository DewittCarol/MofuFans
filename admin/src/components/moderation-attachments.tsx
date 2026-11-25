'use client';

import { EyeOff, ImageOff } from 'lucide-react';
import { useState } from 'react';

function Attachment({ src, index, protectedImage, onOpen }: {
  src: string; index: number; protectedImage: boolean; onOpen: () => void;
}) {
  const [failed, setFailed] = useState(false);
  return <button type="button" onClick={event => {
    if (event.detail >= 2) return;
    if (failed) { setFailed(false); return; }
    onOpen();
  }}
    aria-label={protectedImage ? `去除附图 ${index + 1} 的模糊` : `放大附图 ${index + 1}`}
    data-image-protected={protectedImage}
    className="relative aspect-square min-w-0 overflow-hidden rounded-lg border border-border bg-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
    {failed ? <span className="flex h-full items-center justify-center gap-2 text-xs text-muted-foreground"><ImageOff size={16} />加载失败，点击重试</span> :
      <img src={src} alt={`附图 ${index + 1}`} loading="lazy" draggable={false}
        onError={() => setFailed(true)} className="absolute inset-0 h-full w-full object-cover"
        style={protectedImage ? { filter: 'blur(16px)', transform: 'scale(1.18)' } : undefined} />}
    {protectedImage && <span className="absolute inset-0 flex items-center justify-center bg-card/15">
      <span className="flex items-center gap-2 rounded-lg border border-border/70 bg-card/90 px-3 py-2 text-sm font-medium text-foreground">
        <EyeOff size={17} strokeWidth={1.5} />点击去模糊
      </span>
    </span>}
    {!protectedImage && !failed && <span className="absolute bottom-1.5 right-1.5 rounded bg-card/95 px-2 py-0.5 text-xs text-foreground">点击放大</span>}
  </button>;
}

export function ModerationAttachments({ images, isProtected, onOpen, compact = false, displaySrc }: {
  images: string[]; isProtected: (src: string) => boolean;
  onOpen: (images: string[], index: number) => void; compact?: boolean;
  displaySrc?: (src: string) => string;
}) {
  if (!images.length) return null;
  return <div className={compact ? 'grid grid-cols-[repeat(auto-fill,minmax(128px,1fr))] gap-3'
    : images.length === 1 ? 'grid max-w-[260px] grid-cols-1'
    : 'grid max-w-[840px] grid-cols-[repeat(auto-fit,minmax(min(100%,180px),1fr))] gap-3'}>
    {images.map((src, index) => <Attachment key={`${src}:${index}`} src={displaySrc ? displaySrc(src) : src} index={index}
      protectedImage={isProtected(src)} onOpen={() => onOpen(images, index)} />)}
  </div>;
}
