'use client';

import { useEffect, useRef } from 'react';
import type { PetReaction } from '@/lib/pet-reactions';
import styles from './site-pet.module.css';

export function PetReactionPuffs({ reaction, active, still }: {
  reaction: (PetReaction & { id: number }) | null; active: boolean; still: boolean;
}) {
  const layer = useRef<HTMLDivElement>(null);
  const played = useRef<number | null>(null);
  useEffect(() => {
    const el = layer.current;
    if (!el || !reaction || played.current === reaction.id) return;
    played.current = reaction.id;
    if (!active || document.hidden) return;
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    const staticOnly = still || reduced.matches;
    const animations: Animation[] = [];
    const clear = () => { animations.forEach(animation => animation.cancel()); el.replaceChildren(); };
    const rise = Math.min(85, Math.max(16, el.getBoundingClientRect().top-18));
    for (let i = 0; i < (staticOnly ? 1 : 5); i++) {
      const particle = document.createElement('span');
      const x = staticOnly ? 0 : (i-2)*22;
      Object.assign(particle.style, { position: 'absolute', left: '-16px', top: '-16px', width: '32px', height: '32px',
        fontSize: '30px', lineHeight: '32px', textAlign: 'center', fontFamily: '"Apple Color Emoji", "Segoe UI Emoji", sans-serif' });
      if (reaction.url) {
        const img = new Image(); img.src = reaction.url; img.alt = ''; img.draggable = false;
        Object.assign(img.style, { width: '100%', height: '100%', objectFit: 'contain' });
        img.onerror = () => particle.remove(); particle.append(img);
      } else particle.textContent = reaction.value;
      el.append(particle);
      const frames = staticOnly ? [{ opacity: 0 }, { opacity: 1, offset: .1 }, { opacity: 1, offset: .8 }, { opacity: 0 }]
        : [{ transform: 'translate(0, 0) scale(.6)', opacity: 0 },
          { transform: `translate(${x*.4}px, ${-rise*.38}px) scale(1)`, opacity: 1, offset: .2 },
          { transform: `translate(${x}px, ${-rise}px) rotate(${(i-2)*9}deg) scale(.9)`, opacity: 0 }];
      const animation = particle.animate(frames, { duration: staticOnly ? 2700 : 1700+i*110, delay: staticOnly ? 0 : i*85, easing: 'ease-out', fill: 'both' });
      animations.push(animation); void animation.finished.then(() => particle.remove()).catch(() => {});
    }
    document.addEventListener('visibilitychange', clear); reduced.addEventListener('change', clear);
    return () => { clear(); document.removeEventListener('visibilitychange', clear); reduced.removeEventListener('change', clear); };
  }, [reaction, active, still]);
  return <div ref={layer} className={styles.puffs} aria-hidden="true" data-pet-reaction={reaction?.value} />;
}
