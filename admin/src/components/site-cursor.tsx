'use client';

import { useEffect, useState } from 'react';
import { getConfig } from '@/lib/api';
import { useSiteTheme } from '@/lib/site-theme';

interface Sprite {
  sprite: string;
  frames: number;
  frameWidth: number;
  frameHeight: number;
  hotspot: [number, number];
}
interface Manifest {
  cursors: Record<string, Sprite>;
  cssBindings: Record<string, string[]>;
}

// Let the browser move the pointer independently of page rendering. The admin
// uses one frame of each theme sprite, with no tracking overlay or animation.
const selectors: Record<string, string> = {
  default: 'html, body, :where(.cursor-default)',
  pointer: ':where(a, button, select, summary, label, [role="button"], [role="combobox"], input[type="checkbox"], input[type="radio"], .cursor-pointer)',
  text: ':where(input:not([type="checkbox"]):not([type="radio"]):not([type="button"]):not([type="submit"]), textarea, [contenteditable="true"], .cursor-text)',
  move: ':where(.cursor-move)',
  grab: ':where(.cursor-grab)',
  grabbing: ':where(.cursor-grabbing)',
  'ew-resize': ':where(.cursor-ew-resize)',
  'ns-resize': ':where(.cursor-ns-resize)',
  'nwse-resize': ':where(.cursor-nwse-resize)',
  'nesw-resize': ':where(.cursor-nesw-resize)',
  crosshair: ':where(.cursor-crosshair)',
  help: ':where(.cursor-help)',
  progress: ':where([aria-busy="true"], [aria-busy="true"] *, .cursor-progress)',
  wait: ':where(.cursor-wait)',
  'not-allowed': ':where(:disabled, [aria-disabled="true"], [aria-disabled="true"] *, .cursor-not-allowed)',
};

export function MofuCursor() {
  const { theme, ready } = useSiteTheme();
  return ready && theme.appearance.cursor ? <ActiveCursor /> : null;
}

function ActiveCursor() {
  const [manifestPath, setManifestPath] = useState('');
  const [cursorCSS, setCursorCSS] = useState('');

  useEffect(() => {
    let alive = true;
    let pending = false;
    const refresh = async () => {
      if (document.hidden || pending) return;
      pending = true;
      try {
        const result = await getConfig();
        if (alive && result.code === 200) {
          const theme = result.data.cursor_themes?.find(t => t.id === result.data.cursor_theme);
          setManifestPath(theme?.manifest || '');
        }
      } catch { /* Keep the current theme if the API is temporarily unavailable. */ }
      finally { pending = false; }
    };
    void refresh();
    const timer = window.setInterval(refresh, 15000);
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    return () => {
      alive = false;
      clearInterval(timer);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, []);

  useEffect(() => {
    setCursorCSS('');
    if (!manifestPath) return;
    const abort = new AbortController();
    const load = async () => {
      try {
        const url = new URL(manifestPath, location.origin);
        if (url.origin !== location.origin || !url.pathname.startsWith('/cursors/')) return;
        const response = await fetch(url, { signal: abort.signal });
        if (!response.ok) return;
        const manifest = await response.json() as Manifest;
        const ready = new Map<string, string>();
        // Only decode states that can actually be used by the admin.
        const keys = new Set(['Normal', ...Object.keys(selectors).map(intent =>
          manifest.cssBindings[intent === 'grab' || intent === 'grabbing' ? 'move' : intent]?.[0]).filter(Boolean)]);
        await Promise.all([...keys].map(async key => {
          try {
            const sprite = manifest.cursors[key];
            if (!sprite) return;
            const asset = new URL(sprite.sprite, url);
            if (asset.origin !== location.origin || !asset.pathname.startsWith('/cursors/') ||
              !Number.isInteger(sprite.frames) || sprite.frames < 1 || sprite.frames > 200 ||
              !Number.isInteger(sprite.frameWidth) || sprite.frameWidth < 1 || sprite.frameWidth > 128 ||
              !Number.isInteger(sprite.frameHeight) || sprite.frameHeight < 1 || sprite.frameHeight > 128 ||
              !Array.isArray(sprite.hotspot) || sprite.hotspot.length !== 2 ||
              !sprite.hotspot.every((n, i) => Number.isInteger(n) && n >= 0 && n < (i ? sprite.frameHeight : sprite.frameWidth))) return;
            const img = new Image();
            img.src = asset.href;
            await img.decode();
            if (abort.signal.aborted || img.naturalWidth !== sprite.frameWidth * sprite.frames || img.naturalHeight !== sprite.frameHeight) return;
            const canvas = document.createElement('canvas');
            canvas.width = sprite.frameWidth;
            canvas.height = sprite.frameHeight;
            const context = canvas.getContext('2d');
            if (!context) return;
            context.drawImage(img, 0, 0, canvas.width, canvas.height, 0, 0, canvas.width, canvas.height);
            ready.set(key, `url("${canvas.toDataURL('image/png')}") ${sprite.hotspot[0]} ${sprite.hotspot[1]}`);
          } catch { /* A missing or invalid state keeps its native cursor. */ }
        }));
        if (abort.signal.aborted) return;
        const rules = Object.entries(selectors).map(([intent, selector]) => {
          const binding = intent === 'grab' || intent === 'grabbing' ? 'move' : intent;
          const cursor = ready.get(manifest.cssBindings[binding]?.[0] || (intent === 'default' ? 'Normal' : ''));
          return `${selector} { cursor: ${cursor ? `${cursor}, ` : ''}${intent}; }`;
        }).join('\n');
        setCursorCSS(`@media (any-pointer: fine) { ${rules}\n:where(.cursor-none) { cursor: none; } }`);
      } catch { /* Native cursor remains available when the theme cannot load. */ }
    };
    void load();
    return () => abort.abort();
  }, [manifestPath]);

  return cursorCSS ? <style data-mofu-native-cursor>{cursorCSS}</style> : null;
}
