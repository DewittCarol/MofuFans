'use client';

import { createContext, useCallback, useContext, useEffect, useRef, useState, useMemo } from 'react';
import { createPortal } from 'react-dom';
import { useEmojiCatalog } from '@/lib/emoji-catalog';

type Burst = (rect: DOMRect, emoji: string) => void;
const BurstContext = createContext<Burst>(() => {});
export const useReactionBurst = () => useContext(BurstContext);

export function ReactionBurstProvider({ children }: { children: React.ReactNode }) {
  const { emojis: catalog } = useEmojiCatalog();
  const assets = useMemo(() => Object.fromEntries(catalog.filter(item => item.kind === 'reaction').map(item => [item.value, item.url])), [catalog]);
  const reactions = useMemo(() => Object.fromEntries(catalog.filter(item => item.kind === 'reaction').map(item => [item.value, item])), [catalog]);
  const layerRef = useRef<HTMLDivElement>(null);
  const emitRef = useRef<Burst>(() => {});
  const [mounted, setMounted] = useState(false);
  const burst = useCallback<Burst>((rect, emoji) => {
    emitRef.current(rect, emoji);

  }, [reactions]);
  useEffect(() => { setMounted(true); }, []);

  useEffect(() => {
    const layer = layerRef.current;
    if (!layer) return;
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    const active = new Map<HTMLElement, Animation>();
    const preloads: HTMLImageElement[] = [];
    const remove = (element: HTMLElement) => {
      if (!active.has(element)) return;
      active.delete(element);
      element.remove();
      layer.dataset.particles = String(active.size);
    };
    const clear = () => {
      for (const [element, animation] of active) { animation.cancel(); element.remove(); }
      active.clear(); layer.dataset.particles = '0';
    };
    for (const asset of Object.values(assets)) {
      if (!asset) continue;
      const img = new Image(); img.src = asset;
      preloads.push(img);
      void img.decode().catch(() => {});
    }
    emitRef.current = (rect, emoji) => {
      const width = innerWidth, height = innerHeight;
      if (reduced.matches || document.hidden || rect.bottom <= 0 || rect.top >= height || rect.right <= 0 || rect.left >= width) return;
      const count = width < 600 ? 12 : 18;
      const asset = assets[emoji];
      for (const side of [-1, 1]) {
        const edge = side < 0 ? rect.left : rect.right;
        const room = side < 0 ? rect.left : width - rect.right;
        const direction = room < 100 ? -side : side;
        for (let i = 0; i < count; i++) {
          // A few more visible particles beat a large backlog during busy streams.
          if (active.size >= 108) {
            const oldest = active.entries().next().value;
            if (oldest) { oldest[1].cancel(); remove(oldest[0]); }
          }
          const element = document.createElement('span');
          const size = 24 + Math.random() * 17;
          const x = Math.max(14, Math.min(width - 14, edge));
          const y = Math.max(32, Math.min(height - 40, rect.top + rect.height * (0.51 + Math.random() * 0.14)));
          const vx = direction * (80 + Math.random() * (width < 600 ? 125 : 235));
          const vy = -(265 + Math.random() * 245);
          const spin = (Math.random() - 0.5) * 240;
          const duration = 2050 + Math.random() * 350;
          Object.assign(element.style, {
            position: 'absolute', left: `${x - size / 2}px`, top: `${y - size / 2}px`,
            width: `${size}px`, height: `${size}px`, lineHeight: `${size}px`,
            textAlign: 'center', fontSize: `${size}px`,
            fontFamily: '"Apple Color Emoji", "Segoe UI Emoji", sans-serif',
            pointerEvents: 'none', willChange: 'transform, opacity',
          });
          if (asset) {
            const image = document.createElement('img');
            image.src = asset; image.alt = ''; image.draggable = false;
            Object.assign(image.style, { width: '100%', height: '100%', objectFit: 'contain' });
            element.append(image);
          } else element.textContent = emoji;
          // Calculate the flight once. Only transform and opacity animate on the compositor;
          // neither Canvas redraws nor JavaScript per-frame layout work is required.
          const frames: Keyframe[] = [];
          for (let step = 0; step <= 40; step++) {
            const progress = step / 40;
            const t = progress * duration / 1000;
            frames.push({
              offset: progress,
              transform: `translate3d(${vx * t}px, ${vy * t + 365 * t * t}px, 0) rotate(${spin * t}deg) scale(${Math.min(1, .75 + progress * 4)})`,
              opacity: Math.min(1, (1 - progress) / .23),
            });
          }
          layer.append(element);
          const animation = element.animate(frames, { duration, easing: 'linear', fill: 'both' });
          active.set(element, animation);
          void animation.finished.then(() => remove(element)).catch(() => remove(element));
        }
      }
      layer.dataset.emoji = emoji;
      layer.dataset.bursts = String(Number(layer.dataset.bursts || 0) + 1);
      layer.dataset.particles = String(active.size);
    };
    document.addEventListener('visibilitychange', clear);
    window.addEventListener('resize', clear);
    reduced.addEventListener('change', clear);
    return () => {
      clear(); emitRef.current = () => {};
      document.removeEventListener('visibilitychange', clear);
      window.removeEventListener('resize', clear);
      reduced.removeEventListener('change', clear);
    };
  }, [mounted, assets]);

  return <BurstContext.Provider value={burst}>
    {children}
    {mounted && createPortal(<div ref={layerRef} className="reaction-burst" aria-hidden="true" data-particles="0" />, document.body)}
  </BurstContext.Provider>;
}
