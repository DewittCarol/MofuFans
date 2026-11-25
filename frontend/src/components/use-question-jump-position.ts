'use client';

import { useCallback, useEffect, useRef, useState, type KeyboardEvent, type PointerEvent } from 'react';

const storageKey = 'question-jump-position:v1';
type Point = { x: number; y: number };
const clamp = (point: Point): Point => ({ x: Math.max(12, Math.min(point.x, window.innerWidth - 160)), y: Math.max(88, Math.min(point.y, window.innerHeight - 60)) });

export function useQuestionJumpPosition(enabled: boolean) {
  const [position, setPosition] = useState<Point | null>(null);
  const [visible, setVisible] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [viewport, setViewport] = useState({ width: 0, height: 0 });
  const manual = useRef(false);
  const current = useRef<Point>({ x: 0, y: 0 });
  const drag = useRef<{ pointer: number; start: Point; origin: Point } | null>(null);
  const update = useCallback((point: Point) => { current.current = clamp(point); setPosition(current.current); }, []);
  const save = () => {
    manual.current = true;
    try { localStorage.setItem(storageKey, JSON.stringify({ ...current.current, width: window.innerWidth, height: window.innerHeight })); } catch { /* Position memory is optional. */ }
  };
  useEffect(() => {
    if (!enabled) return;
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey) || 'null');
      if (saved && [saved.x, saved.y, saved.width, saved.height].every(Number.isFinite) && saved.width > 0 && saved.height > 0) {
        manual.current = true;
        update({ x: saved.x * window.innerWidth / saved.width, y: saved.y * window.innerHeight / saved.height });
      }
    } catch { /* Ignore an unavailable or outdated saved position. */ }
    let frame = 0;
    const measure = () => {
      frame = 0;
      const cards = document.querySelectorAll<HTMLElement>('[data-question-anchor]');
      const first = cards[0]?.querySelector<HTMLElement>('.card-wrap');
      const last = cards[cards.length - 1];
      setViewport({ width: window.innerWidth, height: window.innerHeight });
      if (!first || !last) { setVisible(false); return; }
      const rect = first.getBoundingClientRect();
      setVisible(rect.top < window.innerHeight - 60 && last.getBoundingClientRect().bottom > 90);
      if (!drag.current) update(manual.current ? current.current : { x: rect.right + 16, y: Math.max(rect.top + 24, window.innerHeight * 0.38) });
    };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(measure); };
    const observer = new MutationObserver(schedule);
    const list = document.getElementById('question-list');
    if (list) observer.observe(list, { childList: true });
    window.addEventListener('scroll', schedule, { passive: true });
    window.addEventListener('resize', schedule);
    measure();
    return () => { observer.disconnect(); cancelAnimationFrame(frame); window.removeEventListener('scroll', schedule); window.removeEventListener('resize', schedule); };
  }, [enabled, update]);

  const reset = () => {
    manual.current = false;
    try { localStorage.removeItem(storageKey); } catch { /* Optional storage. */ }
    window.dispatchEvent(new Event('resize'));
  };
  return {
    position, visible, dragging,
    panelStyle: position ? { position: 'fixed' as const, left: Math.max(12, Math.min(position.x, viewport.width - 262)), top: Math.max(12, Math.min(position.y + 54, viewport.height - 420)), maxHeight: 'calc(100dvh - 24px)' } : undefined,
    reset,
    handle: {
      onPointerDown: (event: PointerEvent<HTMLButtonElement>) => {
        if (event.button !== 0) return;
        event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId);
        drag.current = { pointer: event.pointerId, start: { x: event.clientX, y: event.clientY }, origin: current.current };
        setDragging(true);
      },
      onPointerMove: (event: PointerEvent<HTMLButtonElement>) => {
        const active = drag.current;
        if (!active || active.pointer !== event.pointerId) return;
        update({ x: active.origin.x + event.clientX - active.start.x, y: active.origin.y + event.clientY - active.start.y });
      },
      onPointerUp: (event: PointerEvent<HTMLButtonElement>) => {
        if (drag.current?.pointer !== event.pointerId) return;
        drag.current = null; setDragging(false); save(); event.currentTarget.releasePointerCapture(event.pointerId);
      },
      onPointerCancel: () => { drag.current = null; setDragging(false); save(); },
      onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => {
        if (event.key === 'Home') { event.preventDefault(); reset(); return; }
        const delta: Record<string, Point> = { ArrowLeft: { x: -16, y: 0 }, ArrowRight: { x: 16, y: 0 }, ArrowUp: { x: 0, y: -16 }, ArrowDown: { x: 0, y: 16 } };
        if (!delta[event.key]) return;
        event.preventDefault(); update({ x: current.current.x + delta[event.key].x, y: current.current.y + delta[event.key].y }); save();
      },
    },
  };
}
