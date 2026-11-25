'use client';

import { useSiteTheme } from '@/lib/site-theme';
import { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { getConfig } from '@/lib/api';

interface Sprite {
  sprite: string;
  frames: number;
  frameWidth: number;
  frameHeight: number;
  frameMs: number;
  hotspot: [number, number];
}
interface Manifest {
  cursors: Record<string, Sprite>;
  cssBindings: Record<string, string[]>;
}

export function MofuCursor() {
 const {theme,ready}=useSiteTheme();
 return ready && theme.appearance.cursor ? <ActiveCursor/> : null;
}
function ActiveCursor() {
  const [manifestPath, setManifestPath] = useState('');
  const [mounted, setMounted] = useState(false);
  const layer = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setMounted(true);
    let alive = true;
    let pending = false;
    let selected: string | null = null;
    let siteTheme = '';
    let themes: { id: string; manifest?: string }[] = [];
    try { selected = sessionStorage.getItem('site-cursor-preview'); } catch { /* Use the website default. */ }
    const apply = () => setManifestPath(themes.find(theme => theme.id === selected)?.manifest || siteTheme);
    const preview = (event: Event) => {
      const value = (event as CustomEvent).detail;
      if (value !== null && !themes.some(theme => theme.id === value)) return;
      selected = value;
      try { if (value === null) sessionStorage.removeItem('site-cursor-preview'); else sessionStorage.setItem('site-cursor-preview', value); } catch { /* In-memory preview still works. */ }
      apply();
    };
    const refresh = async () => {
      if (document.hidden || pending) return;
      pending = true;
      try {
        const result = await getConfig();
        if (alive && result.code === 200) {
          const theme = result.data.cursor_themes?.find(t => t.id === result.data.cursor_theme);
          themes = result.data.cursor_themes || [];
          siteTheme = theme?.manifest || '';
          apply();
        }
      } catch { /* Keep the current theme if the API is temporarily unavailable. */ }
      finally { pending = false; }
    };
    void refresh();
    const timer = window.setInterval(refresh, 15000);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    window.addEventListener('site-cursor-preview', preview);
    return () => {
      alive = false;
      clearInterval(timer);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
      window.removeEventListener('site-cursor-preview', preview);
    };
  }, []);

  useEffect(() => {
    const el = layer.current;
    if (!manifestPath || !el) return;
    const abort = new AbortController();
    const root = document.documentElement;
    const fine = matchMedia('(any-pointer: fine)');
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    let manifest: Manifest | null = null;
    const ready = new Map<string, Sprite>();
    let current = '';
    let target: Element | null = null;
    let x = 0, y = 0, frame = 0;
    let hasPointer = false;
    const hide = () => {
      delete root.dataset.mofuCursor;
      el.style.visibility = 'hidden';
      hasPointer = false;
      target = null;
    };
    const render = () => {
      frame = 0;
      const sprite = ready.get(current);
      if (!sprite || !hasPointer) return;
      el.style.transform = `translate3d(${x - sprite.hotspot[0]}px, ${y - sprite.hotspot[1]}px, 0)`;
    };
    const move = (event: PointerEvent) => {
      if (event.pointerType !== 'mouse' || !fine.matches || !manifest || !ready.has('Normal')) {
        hide(); return;
      }
      x = event.clientX; y = event.clientY;
      const nextTarget = event.target instanceof Element ? event.target : null;
      if (nextTarget !== target || !hasPointer) {
        target = nextTarget;
        // Intent is inherited through CSS; never toggle a global selector to read it.
        let intent = target ? getComputedStyle(target).getPropertyValue('--site-cursor-intent').trim() || 'default' : 'default';
        if (target instanceof HTMLElement && target.style.cursor) intent = target.style.cursor;
        if (target?.closest(':disabled,[aria-disabled="true"]')) intent = 'not-allowed';
        else if (target?.closest('[aria-busy="true"]')) intent = 'progress';
        else if (target?.closest('input:not([type="checkbox"]):not([type="radio"]):not([type="button"]):not([type="submit"]),textarea,[contenteditable="true"]')) intent = 'text';
        else if (target?.closest('a,button,[role="button"],select,summary,label')) intent = 'pointer';
        if (intent === 'none') { hide(); return; }
        const key = manifest.cssBindings[intent]?.[0] || 'Normal';
        const selected = ready.has(key) ? key : 'Normal';
        if (selected !== current) {
          current = selected;
          const sprite = ready.get(current)!;
          el.style.width = `${sprite.frameWidth}px`;
          el.style.height = `${sprite.frameHeight}px`;
          el.style.backgroundImage = `url("${sprite.sprite}")`;
          el.style.setProperty('--cursor-end', `${-sprite.frameWidth * sprite.frames}px`);
          el.style.animation = reduced.matches || sprite.frames < 2 ? 'none' : `site-cursor-frames ${sprite.frames * sprite.frameMs}ms steps(${sprite.frames}) infinite`;
        }
        if (root.dataset.mofuCursor !== 'active') root.dataset.mofuCursor = 'active';
      }
      hasPointer = true;
      el.style.visibility = 'visible';
      if (!frame) frame = requestAnimationFrame(render);
    };
    const leave = (event: PointerEvent) => { if (!event.relatedTarget) hide(); };
    const preferences = () => { current = ''; hide(); };
    const load = async () => {
      try {
        const url = new URL(manifestPath, location.origin);
        if (url.origin !== location.origin || !url.pathname.startsWith('/cursors/')) return;
        const response = await fetch(url, { signal: abort.signal });
        if (!response.ok) return;
        manifest = await response.json() as Manifest;
        await Promise.all(Object.entries(manifest.cursors).map(async ([key, sprite]) => {
          const asset = new URL(sprite.sprite, url);
          if (asset.origin !== location.origin || !asset.pathname.startsWith('/cursors/') ||
            !Number.isInteger(sprite.frames) || sprite.frames < 1 || sprite.frames > 200 ||
            !Number.isFinite(sprite.frameMs) || sprite.frameMs < 16 ||
            sprite.frameWidth < 1 || sprite.frameWidth > 128 || sprite.frameHeight < 1 || sprite.frameHeight > 128 ||
            !Array.isArray(sprite.hotspot) || !sprite.hotspot.every(Number.isFinite)) return;
          const img = new Image();
          img.src = asset.href;
          try {
            await img.decode();
            if (!abort.signal.aborted && img.naturalWidth === sprite.frameWidth * sprite.frames && img.naturalHeight === sprite.frameHeight) {
              ready.set(key, { ...sprite, sprite: asset.href });
            }
          } catch { /* This cursor state falls back to Normal. */ }
        }));
      } catch { /* Native cursor remains visible when assets cannot be loaded. */ }
    };
    void load();
    window.addEventListener('pointermove', move, { passive: true });
    window.addEventListener('pointerout', leave);
    window.addEventListener('blur', hide);
    window.addEventListener('scroll', hide, { passive: true });
    document.addEventListener('visibilitychange', hide);
    fine.addEventListener('change', preferences);
    reduced.addEventListener('change', preferences);
    return () => {
      abort.abort(); hide(); cancelAnimationFrame(frame);
      el.style.animation = 'none';
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerout', leave);
      window.removeEventListener('blur', hide);
      window.removeEventListener('scroll', hide);
      document.removeEventListener('visibilitychange', hide);
      fine.removeEventListener('change', preferences);
      reduced.removeEventListener('change', preferences);
    };
  }, [manifestPath, mounted]);

  return mounted ? createPortal(<div ref={layer} className="site-cursor" aria-hidden="true" />, document.body) : null;
}
