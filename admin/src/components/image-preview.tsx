'use client';

import { useState, useRef, useEffect, useCallback } from 'react';
import { Dialog } from 'radix-ui';
import { X, ZoomIn, ZoomOut, RotateCw, Download, ChevronLeft, ChevronRight, Maximize, EyeOff } from 'lucide-react';

interface ImagePreviewProps {
  src: string;
  originalSrc?: string;
  images?: string[];
  currentIndex?: number;
  onClose: () => void;
  onIndexChange?: (index: number) => void;
  protectedImage?: boolean;
  onReviewThumbnail?: () => void;
}

export function ImagePreview({ src, originalSrc, images, currentIndex = 0, onClose, onIndexChange, protectedImage = false, onReviewThumbnail }: ImagePreviewProps) {
  const hasMultiple = (images?.length ?? 0) > 1;
  const [scale, setScale] = useState(1);
  const [position, setPosition] = useState({ x: 0, y: 0 });
  const [rotation, setRotation] = useState(0);
  const [natural, setNatural] = useState({ width: 0, height: 0 });
  const [size, setSize] = useState({ width: 0, height: 0 });
  const [error, setError] = useState('');
  const [downloading, setDownloading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [stage, setStage] = useState<HTMLDivElement | null>(null);
  const imageRef = useRef<HTMLImageElement>(null);
  const openerRef = useRef<HTMLElement | null>(typeof document !== 'undefined' && document.activeElement instanceof HTMLElement ? document.activeElement : null);
  const points = useRef(new Map<number, { x: number; y: number }>());
  const gesture = useRef({ x: 0, y: 0, px: 0, py: 0, distance: 0, scale: 1, time: 0, pinched: false });

  const reset = useCallback(() => { setScale(1); setPosition({ x: 0, y: 0 }); setRotation(0); }, []);
  const navigate = useCallback((direction: number) => {
    if (images?.length && onIndexChange) onIndexChange((currentIndex + direction + images.length) % images.length);
  }, [images, currentIndex, onIndexChange]);
  const zoom = useCallback((delta: number) => {
    if (!protectedImage) setScale(value => Math.max(.5, Math.min(6, value + delta)));
  }, [protectedImage]);

  useEffect(() => {
    reset(); setNatural({ width: 0, height: 0 }); setError('');
    points.current.clear(); setDragging(false);
  }, [src, protectedImage, reset]);
  useEffect(() => {
    if (!stage) return;
    const update = () => setSize({ width: stage.clientWidth, height: stage.clientHeight });
    update();
    const observer = new ResizeObserver(update); observer.observe(stage);
    const wheel = (event: WheelEvent) => { event.preventDefault(); zoom(event.deltaY > 0 ? -.12 : .12); };
    stage.addEventListener('wheel', wheel, { passive: false });
    return () => { observer.disconnect(); stage.removeEventListener('wheel', wheel); };
  }, [zoom, stage]);

  const rotated = rotation % 180 !== 0;
  const pad = size.width < 600 ? 20 : 112;
  const fit = natural.width && natural.height ? Math.min(
    Math.max(1, size.width - pad) / (rotated ? natural.height : natural.width),
    Math.max(1, size.height - 24) / (rotated ? natural.width : natural.height),
  ) : 0;

  const download = async () => {
    setDownloading(true); setError('');
    try {
      const downloadSrc = originalSrc || src;
      const response = await fetch(downloadSrc);
      if (!response.ok) throw new Error('download');
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a'); a.href = url;
      a.download = downloadSrc.split('/').pop()?.split('?')[0] || 'image';
      a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch { setError('下载失败，可以打开原图后保存。'); }
    finally { setDownloading(false); }
  };

  const toolbar = 'inline-flex h-10 w-10 shrink-0 items-center justify-center rounded-md text-white hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-white disabled:opacity-40';
  return <Dialog.Root open onOpenChange={open => { if (!open) onClose(); }}>
    <Dialog.Portal>
      <Dialog.Overlay className="fixed inset-0 z-[2000] bg-viewer/95" />
      <Dialog.Content
        className="fixed inset-0 z-[2001] flex h-[100dvh] flex-col text-white outline-none"
        aria-describedby={undefined}
        onCloseAutoFocus={event => { event.preventDefault(); openerRef.current?.focus(); }}
        onKeyDown={event => {
          if (event.key === 'ArrowLeft' && hasMultiple) { event.preventDefault(); navigate(-1); }
          if (event.key === 'ArrowRight' && hasMultiple) { event.preventDefault(); navigate(1); }
          if (event.key === '+' || event.key === '=') zoom(.2);
          if (event.key === '-') zoom(-.2);
          if (event.key === '0') reset();
        }}
      >
        <Dialog.Title className="sr-only">附图预览</Dialog.Title>
        <div className="flex min-h-14 shrink-0 items-center justify-between gap-2 px-2 sm:px-5">
          <span className="shrink-0 text-sm text-white/85">{hasMultiple ? `${currentIndex + 1} / ${images!.length}` : '附图'}</span>
          <div className="flex items-center gap-0.5 sm:gap-2">
            <button type="button" className={toolbar} aria-label="缩小" disabled={protectedImage} title="缩小 (-)" onClick={() => zoom(-.2)}><ZoomOut size={19} /></button>
            <button type="button" className={toolbar} aria-label="放大" disabled={protectedImage} title="放大 (+)" onClick={() => zoom(.2)}><ZoomIn size={19} /></button>
            <button type="button" className={toolbar} aria-label="适应窗口" disabled={protectedImage} title="适应窗口 (0)" onClick={reset}><Maximize size={18} /></button>
            <button type="button" className={toolbar} aria-label="旋转" disabled={protectedImage} onClick={() => setRotation(value => (value + 90) % 360)}><RotateCw size={19} /></button>
            {!originalSrc && <button type="button" className={toolbar} aria-label="下载图片" disabled={protectedImage || downloading} onClick={download}><Download size={19} /></button>}
            <Dialog.Close asChild><button type="button" className={toolbar} aria-label="关闭图片" title="关闭 (Esc)"><X size={22} /></button></Dialog.Close>
          </div>
        </div>
        <div ref={setStage} className="relative min-h-0 flex-1 overflow-hidden" style={{ touchAction: 'none', cursor: protectedImage ? 'default' : dragging ? 'grabbing' : 'grab' }}
          onPointerDown={event => {
            if (protectedImage || event.button !== 0 || (event.target as Element).closest('button,a')) return;
            event.currentTarget.setPointerCapture(event.pointerId);
            points.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
            const values = [...points.current.values()];
            if (values.length === 2) {
              gesture.current.distance = Math.hypot(values[0].x - values[1].x, values[0].y - values[1].y);
              gesture.current.scale = scale; gesture.current.pinched = true;
            } else {
              gesture.current = { x: event.clientX, y: event.clientY, px: position.x, py: position.y, distance: 0, scale, time: Date.now(), pinched: false };
            }
            setDragging(true);
          }}
          onPointerMove={event => {
            if (!points.current.has(event.pointerId)) return;
            points.current.set(event.pointerId, { x: event.clientX, y: event.clientY });
            const values = [...points.current.values()];
            if (values.length === 2 && gesture.current.distance > 0) {
              const distance = Math.hypot(values[0].x - values[1].x, values[0].y - values[1].y);
              setScale(Math.max(.5, Math.min(6, gesture.current.scale * distance / gesture.current.distance)));
            } else if (!gesture.current.pinched && (event.pointerType === 'mouse' || scale > 1)) {
              setPosition({ x: gesture.current.px + event.clientX - gesture.current.x, y: gesture.current.py + event.clientY - gesture.current.y });
            }
          }}
          onPointerUp={event => {
            const start = gesture.current;
            const dx = event.clientX - start.x, dy = event.clientY - start.y;
            if (!start.pinched && event.pointerType !== 'mouse' && scale <= 1 && hasMultiple && Math.abs(dx) > 60 && Math.abs(dx) > Math.abs(dy) * 1.5 && Date.now() - start.time < 500) navigate(dx > 0 ? -1 : 1);
            points.current.delete(event.pointerId);
            if (!points.current.size) setDragging(false);
          }}
          onPointerCancel={() => { points.current.clear(); setDragging(false); }}
          onDoubleClick={event => { if (!protectedImage && !(event.target as Element).closest('button,a')) { setScale(scale > 1 ? 1 : 2); setPosition({ x: 0, y: 0 }); } }}
        >
          {!protectedImage && !natural.width && !error && <p role="status" className="absolute inset-0 flex items-center justify-center text-sm text-white/80">图片加载中…</p>}
          {!protectedImage && <div className="absolute inset-0 flex items-center justify-center" style={{ pointerEvents: 'none' }}>
            <img key={src} ref={imageRef} src={src} alt={`附图 ${currentIndex + 1}`} decoding="async" draggable={false}
              onLoad={event => setNatural({ width: event.currentTarget.naturalWidth, height: event.currentTarget.naturalHeight })}
              onError={() => setError('图片未能加载，请稍后重试。')}
              className="select-none object-contain"
              style={{ width: natural.width * fit, height: natural.height * fit, maxWidth: 'none', opacity: fit ? 1 : 0,
                transform: `translate3d(${position.x}px, ${position.y}px, 0) rotate(${rotation}deg) scale(${scale})`,
                transition: dragging ? 'none' : 'transform 100ms ease-out', willChange: dragging ? 'transform' : 'auto' }} />
          </div>}
          {protectedImage && <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 px-14 text-center">
            <EyeOff size={32} strokeWidth={1.5} className="text-white/70" />
            <p>这张附图尚未查看</p>
            <p className="text-sm text-white/70">先在卡片中查看缩略图，再决定是否放大。</p>
            <button type="button" onClick={onReviewThumbnail} className="rounded-lg bg-white px-5 py-3 text-sm font-medium text-foreground hover:bg-white/90 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-white">返回卡片查看</button>
          </div>}
          {hasMultiple && <>
            <button type="button" className={`${toolbar} absolute left-1 top-1/2 -translate-y-1/2 bg-black/35 sm:left-4`} aria-label="上一张图片" onClick={() => navigate(-1)}><ChevronLeft size={26} /></button>
            <button type="button" className={`${toolbar} absolute right-1 top-1/2 -translate-y-1/2 bg-black/35 sm:right-4`} aria-label="下一张图片" onClick={() => navigate(1)}><ChevronRight size={26} /></button>
          </>}
        </div>
        <div className="flex min-h-12 shrink-0 flex-wrap items-center justify-center gap-x-4 gap-y-1 px-4 py-2 text-sm text-white/80" aria-live="polite">
          {protectedImage ? <span>附图保护已开启{hasMultiple ? ' · ← → 切换图片' : ''}</span> : <>
            {error ? <span role="alert">{error}</span> : <><span>{scale === 1 ? '适应窗口' : `${Math.round(scale * 100)}%`}</span><span className="hidden sm:inline">拖动查看 · 滚轮缩放{hasMultiple ? ' · ← → 切换' : ''}</span></>}
            {originalSrc && <button type="button" disabled={downloading} onClick={download} className="inline-flex min-h-11 items-center gap-1.5 rounded-md px-2 hover:bg-white/15 focus-visible:outline-2 focus-visible:outline-white disabled:opacity-40"><Download size={17} />{downloading ? '正在下载…' : '下载原图'}</button>}
            <a href={originalSrc || src} target="_blank" rel="noopener noreferrer" className="underline underline-offset-4 hover:text-white">打开原图</a>
          </>}
        </div>
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>;
}
