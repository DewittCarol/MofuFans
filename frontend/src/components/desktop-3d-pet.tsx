'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, Pause, PawPrint, Play, RotateCcw, Smile, Hand, Scaling } from 'lucide-react';
import { clamp, petPosition, readPetPreferences, type PetPreferences } from '@/lib/pet-model';
import { preservePetControlAnchor } from '@/lib/pet-control-anchor';
import { pet3dExpressions, pet3dActions, type Pet3dExpression } from '@/lib/pet3d-model';
import type { PetEngine } from '@/lib/pet-engine';
import type { Pet3dModelDefinition } from '@/lib/pet3d-renderer';
import { petReactionProfile, type PetReaction } from '@/lib/pet-reactions';
import { PetReactionPuffs } from './pet-reaction-puffs';
import styles from './site-pet.module.css';
import { usePetDefaultExpanded } from '@/lib/admin-session';

const defaults: PetPreferences = { ...readPetPreferences(null, false), mode: '3d', model3d: 0 };
type Props = { model: Pet3dModelDefinition; label: string; storageKey: string };

export function Desktop3dPet({ model, label, storageKey: preferenceKey }: Props) {
  const entryExpanded = usePetDefaultExpanded();
  const entryInitialized = useRef(false);
  const [preferences, setPreferences] = useState<PetPreferences | null>(null);
  const current = useRef(defaults);
  const root = useRef<HTMLDivElement>(null);
  const canvas = useRef<HTMLCanvasElement>(null);
  const stage = useRef<HTMLButtonElement>(null);
  const reopen = useRef<HTMLButtonElement>(null);
  const renderer = useRef<PetEngine | null>(null);
  const [status, setStatus] = useState<'loading' | 'ready' | 'error'>('loading');
  const [retry, setRetry] = useState(0);
  const [progress, setProgress] = useState(0);
  const [ratio, setRatio] = useState(.84);
  const [message, setMessage] = useState('');
  const [covered, setCovered] = useState(false);
  const [smallScreen, setSmallScreen] = useState(true);
  const messageTimer = useRef(0);
  const lastResponse = useRef(0);
  const expression = useRef(0);
  const [faceMenuOpen, setFaceMenuOpen] = useState(false);
  const [face, setFace] = useState<Pet3dExpression>('neutral');
  const selectedFace = useRef<Pet3dExpression>('neutral');
  const faceControl = useRef<HTMLDivElement>(null);
  const faceButton = useRef<HTMLButtonElement>(null);
  const [sizeMenuOpen, setSizeMenuOpen] = useState(false);
  const sizeControl = useRef<HTMLDivElement>(null);
  const sizeButton = useRef<HTMLButtonElement>(null);
  const controlBar = useRef<HTMLDivElement>(null);
  const resizeAnchor = useRef<{ x: number; y: number } | null>(null);
  const [actionMenuOpen, setActionMenuOpen] = useState(false);
  const actionControl = useRef<HTMLDivElement>(null);
  const actionButton = useRef<HTMLButtonElement>(null);
  const [puff, setPuff] = useState<(PetReaction & { id: number }) | null>(null);
  const puffSequence = useRef(0);
  const dragging = useRef<{ id: number; x: number; y: number; startX: number; startY: number; moved: boolean } | null>(null);
  const skipClick = useRef(false);
  const focusAfterToggle = useRef(false);

  const update = useCallback((patch: Partial<PetPreferences>) => {
    const value = { ...current.current, ...patch };
    current.current = value; setPreferences(value);
    try { localStorage.setItem(preferenceKey, JSON.stringify(value)); } catch { /* Storage can be disabled. */ }
  }, [preferenceKey]);

  const position = useCallback(() => {
    const el = root.current;
    if (!el) return;
    const bounds = petPosition(current.current.x, current.current.y, document.documentElement.clientWidth,
      window.innerHeight, el.offsetWidth, el.offsetHeight);
    el.style.transform = `translate3d(${bounds.x}px, ${bounds.y}px, 0)`;
    return bounds;
  }, []);

  const rememberResizeAnchor = useCallback(() => {
    if (resizeAnchor.current) return;
    const rect = controlBar.current?.getBoundingClientRect();
    if (rect) resizeAnchor.current = { x: rect.left + rect.width / 2, y: rect.bottom };
  }, []);

  const restoreResizeAnchor = useCallback(() => {
    const el = root.current, bar = controlBar.current, anchor = resizeAnchor.current;
    if (!el || !bar || !anchor) return false;
    const rootRect = el.getBoundingClientRect(), barRect = bar.getBoundingClientRect();
    const next = preservePetControlAnchor(anchor.x, anchor.y, barRect.left + barRect.width / 2, barRect.bottom,
      rootRect.left, rootRect.top, document.documentElement.clientWidth, window.innerHeight, el.offsetWidth, el.offsetHeight);
    resizeAnchor.current = null;
    current.current = { ...current.current, x: next.normalizedX, y: next.normalizedY };
    el.style.transform = `translate3d(${next.x}px, ${next.y}px, 0)`;
    try { localStorage.setItem(preferenceKey, JSON.stringify(current.current)); } catch { /* Storage can be disabled. */ }
    return true;
  }, []);

  const resizeModel = useCallback((size3d: number) => {
    if (size3d === current.current.size3d) return;
    rememberResizeAnchor();
    update({ size3d });
  }, [rememberResizeAnchor, update]);

  const say = useCallback((text: string) => {
    window.clearTimeout(messageTimer.current); setMessage(text);
    messageTimer.current = window.setTimeout(() => setMessage(''), 2600);
  }, []);

  useEffect(() => {
    const small = matchMedia('(max-width: 900px)');
    const screenChanged = () => setSmallScreen(small.matches);
    screenChanged(); small.addEventListener('change', screenChanged);
    if (entryExpanded !== null && !entryInitialized.current) {
      entryInitialized.current = true;
      let saved: string | null = null;
      try { saved = localStorage.getItem(preferenceKey); } catch { /* Use defaults. */ }
      const initial = { ...readPetPreferences(saved, small.matches), collapsed: !entryExpanded, mode: '3d' as const, model3d: 0 };
      current.current = initial; setPreferences(initial);
    }
    return () => { window.clearTimeout(messageTimer.current); small.removeEventListener('change', screenChanged); };
  }, [entryExpanded, preferenceKey]);

  const collapsed = preferences?.collapsed ?? true;
  const initialized = preferences !== null;
  const paused = preferences?.paused ?? false;
  const mode = '3d';
  useEffect(() => { setFaceMenuOpen(false); setActionMenuOpen(false); setSizeMenuOpen(false); }, [mode, collapsed, smallScreen]);
  useEffect(() => {
    const el = root.current;
    if (!el || collapsed || smallScreen) return;
    let hideTimer = 0;
    const hide = () => {
      hideTimer = 0;
      if (el.querySelector(':focus-visible')) return;
      delete el.dataset.near;
      setFaceMenuOpen(false); setActionMenuOpen(false); setSizeMenuOpen(false);
    };
    const near = (event: PointerEvent) => {
      if (event.pointerType !== 'mouse') return;
      const r = el.getBoundingClientRect(), margin = 44;
      if (dragging.current || (event.clientX >= r.left-margin && event.clientX <= r.right+margin && event.clientY >= r.top-margin && event.clientY <= r.bottom+margin)) {
        window.clearTimeout(hideTimer); hideTimer = 0; el.dataset.near = 'true';
      } else if (el.dataset.near && !hideTimer) hideTimer = window.setTimeout(hide, 450);
    };
    document.addEventListener('pointermove', near, { passive: true });
    document.addEventListener('pointerleave', hide);
    return () => { window.clearTimeout(hideTimer); document.removeEventListener('pointermove', near); document.removeEventListener('pointerleave', hide); };
  }, [collapsed, initialized, smallScreen]);
  useEffect(() => {
    if (!faceMenuOpen) return;
    const outside = (event: PointerEvent) => {
      if (!faceControl.current?.contains(event.target as Node)) setFaceMenuOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { setFaceMenuOpen(false); faceButton.current?.focus({ preventScroll: true }); }
    };
    document.addEventListener('pointerdown', outside);
    document.addEventListener('keydown', escape);
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape); };
  }, [faceMenuOpen]);
  useEffect(() => {
    if (!actionMenuOpen) return;
    const outside = (event: PointerEvent) => { if (!actionControl.current?.contains(event.target as Node)) setActionMenuOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { setActionMenuOpen(false); actionButton.current?.focus({ preventScroll: true }); } };
    document.addEventListener('pointerdown', outside); document.addEventListener('keydown', escape);
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape); };
  }, [actionMenuOpen]);
  useEffect(() => {
    if (!sizeMenuOpen) return;
    const outside = (event: PointerEvent) => { if (!sizeControl.current?.contains(event.target as Node)) setSizeMenuOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === 'Escape') { setSizeMenuOpen(false); sizeButton.current?.focus({ preventScroll: true }); } };
    document.addEventListener('pointerdown', outside); document.addEventListener('keydown', escape);
    return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', escape); };
  }, [sizeMenuOpen]);
  useEffect(() => {
    const el = root.current;
    if (!el) return;
    const observer = new ResizeObserver(() => { if (!restoreResizeAnchor()) position(); renderer.current?.resize(); });
    observer.observe(el); position();
    window.addEventListener('resize', position);
    window.visualViewport?.addEventListener('resize', position);
    if (focusAfterToggle.current) {
      (collapsed ? reopen.current : stage.current)?.focus({ preventScroll: true });
      focusAfterToggle.current = false;
    }
    return () => { observer.disconnect(); window.removeEventListener('resize', position); window.visualViewport?.removeEventListener('resize', position); };
  }, [collapsed, initialized, smallScreen, position, restoreResizeAnchor]);

  useEffect(() => {
    if (!preferences || collapsed || smallScreen || !canvas.current) return;
    const target = canvas.current;
    const abort = new AbortController();
    let pet: PetEngine | null = null;
    let idle = 0, fallback = 0;
    let reactionTimer = 0;
    const reduced = matchMedia('(prefers-reduced-motion: reduce)');
    const updateActivity = () => {
      const modal = !!document.querySelector('[role="dialog"]:not([data-state="closed"]), [role="alertdialog"], [data-pet-obscured]');
      setCovered(modal);
      pet?.setActive(!document.hidden && !modal);
    };
    const updateMotion = () => pet?.setPreferences(current.current.paused, reduced.matches);
    const move = (event: PointerEvent) => {
      if (event.pointerType !== 'mouse' || !pet || dragging.current) return;
      pet.setPointer(event.clientX/window.innerWidth*2-1, event.clientY/window.innerHeight*2-1);
    };
    const leave = () => pet?.setPointer(0, 0);
    const showReaction = (reaction: PetReaction) => {
      if (!pet || abort.signal.aborted || document.hidden || root.current?.hasAttribute('data-covered')) return;
      lastResponse.current = performance.now();
      const profile = petReactionProfile(reaction);
      if (pet.reactToEmoji) pet.reactToEmoji(profile.face, profile.action);
      else pet.respond(profile.pngExpression);
      setMessage(''); setPuff({ ...reaction, id: ++puffSequence.current });
    };
    const react = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      const kind = typeof detail === 'string' ? detail : detail?.kind;
      if (!pet || document.hidden || (kind !== 'question' && kind !== 'reaction')) return;
      if (kind === 'reaction' && typeof detail?.reaction?.value === 'string') {
        window.clearTimeout(reactionTimer);
        const delay = Math.max(0, 750-(performance.now()-lastResponse.current));
        if (delay) reactionTimer = window.setTimeout(() => showReaction(detail.reaction), delay);
        else showReaction(detail.reaction);
      } else if (performance.now()-lastResponse.current >= 2500) {
        lastResponse.current = performance.now(); pet.respond(2);
        if (kind === 'question') say('收到你的提问啦！');
      }
    };
    const observer = new MutationObserver(updateActivity);
    // Dialogs add/remove elements; avoid watching animation style changes.
    observer.observe(document.body, { childList: true, subtree: true });
    const load = async () => {
      setStatus('loading'); setProgress(0);
      try {
        const { Pet3dRenderer } = await import('@/lib/pet3d-renderer');
        if (abort.signal.aborted) return;
        pet = await Pet3dRenderer.load(target, { signal: abort.signal, model: 0, definition: model,
          onProgress: value => { if (!abort.signal.aborted) setProgress(value); },
          onError: () => { if (!abort.signal.aborted) { abort.abort(); setStatus('error'); } },
        });
        if (abort.signal.aborted) { pet.dispose(); return; }
        renderer.current = pet; setRatio(pet.aspectRatio);
        pet.setExpression?.(selectedFace.current);
        updateMotion(); updateActivity(); pet.resize();
        setStatus('ready');
      } catch {
        pet?.dispose();
        if (!abort.signal.aborted) { abort.abort(); setStatus('error'); }
      }
    };
    if (typeof window.requestIdleCallback === 'function') idle = window.requestIdleCallback(() => { void load(); }, { timeout: 1400 });
    else fallback = window.setTimeout(() => { void load(); }, 300);
    document.addEventListener('visibilitychange', updateActivity);
    document.addEventListener('pointermove', move, { passive: true });
    document.addEventListener('pointerleave', leave);
    window.addEventListener('blur', leave);
    window.addEventListener('mofu-pet-response', react);
    reduced.addEventListener('change', updateMotion);
    return () => {
      abort.abort(); if (idle) window.cancelIdleCallback(idle); window.clearTimeout(fallback);
      window.clearTimeout(reactionTimer); setPuff(null);
      observer.disconnect(); pet?.dispose(); renderer.current = null;
      document.removeEventListener('visibilitychange', updateActivity);
      document.removeEventListener('pointermove', move); document.removeEventListener('pointerleave', leave);
      window.removeEventListener('blur', leave); window.removeEventListener('mofu-pet-response', react);
      reduced.removeEventListener('change', updateMotion);
    };
    // The renderer remains mounted through preference/position changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collapsed, retry, say, smallScreen, model]);

  useEffect(() => {
    renderer.current?.setPreferences(paused, matchMedia('(prefers-reduced-motion: reduce)').matches);
  }, [paused]);

  const toggle = (value: boolean) => {
    focusAfterToggle.current = true; setMessage(''); setCovered(false);
    if (!value) setStatus('loading');
    update({ collapsed: value });
  };

  const moveTo = (x: number, y: number, persist = false) => {
    const bounds = position();
    if (!bounds) return;
    const next = { x: clamp((x-bounds.minX)/Math.max(1, bounds.maxX-bounds.minX)),
      y: clamp((y-bounds.minY)/Math.max(1, bounds.maxY-bounds.minY)) };
    current.current = { ...current.current, ...next }; position();
    if (persist) update(next);
  };

  const pointerDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (!event.isPrimary || event.button !== 0) return;
    const bounds = position(); if (!bounds) return;
    dragging.current = { id: event.pointerId, x: event.clientX, y: event.clientY, startX: bounds.x, startY: bounds.y, moved: false };
    skipClick.current = false;
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const pointerMove = (event: React.PointerEvent<HTMLButtonElement>) => {
    const drag = dragging.current;
    if (!drag || drag.id !== event.pointerId) return;
    const dx = event.clientX-drag.x, dy = event.clientY-drag.y;
    if (!drag.moved && Math.hypot(dx, dy) < 6) return;
    drag.moved = true; skipClick.current = true;
    if (root.current) root.current.dataset.dragging = 'true';
    moveTo(drag.startX+dx, drag.startY+dy);
  };
  const pointerEnd = (event: React.PointerEvent<HTMLButtonElement>) => {
    const drag = dragging.current;
    if (!drag || event.pointerId !== drag.id) return;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    dragging.current = null;
    if (root.current) delete root.current.dataset.dragging;
    if (drag.moved) update({ x: current.current.x, y: current.current.y });
  };

  if (!preferences || smallScreen) return null;
  return createPortal(
    <div ref={root} className={`${styles.pet} ${collapsed ? styles.collapsed : ''}`} data-desktop-pet="" data-mode={mode} data-status={status} data-covered={covered || undefined}
      style={{ '--pet-ratio': ratio, '--pet-scale': preferences.size3d / 50 } as React.CSSProperties}>
      {collapsed ? (
        <button ref={reopen} type="button" className={styles.reopen} onClick={() => toggle(false)} aria-label={`唤出${label}桌宠`} title="唤出桌宠">
          <PawPrint size={20} aria-hidden="true" />
        </button>
      ) : (
        <>
          <PetReactionPuffs reaction={puff} active={!covered} still={paused} />
          <div className={styles.bubble} role="status" aria-live="polite">{message}</div>
          <button ref={stage} type="button" className={styles.stage} aria-label={`${label}桌宠，点击换表情，拖动可移动位置`} aria-describedby="desktop-pet-help"
            onPointerDown={pointerDown} onPointerMove={pointerMove} onPointerUp={pointerEnd} onPointerCancel={pointerEnd} onLostPointerCapture={pointerEnd}
            onClick={() => {
              if (skipClick.current) { skipClick.current = false; return; }
              if (status !== 'ready') return;
              expression.current = (expression.current+1)%3;
              renderer.current?.respond(expression.current);
            }}
            onKeyDown={event => {
              const step = event.shiftKey ? 48 : 16;
              const directions: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, -step], ArrowDown: [0, step] };
              if (directions[event.key]) {
                event.preventDefault(); const bounds = position();
                if (bounds) moveTo(bounds.x+directions[event.key][0], bounds.y+directions[event.key][1], true);
              } else if (event.key === 'Home') { event.preventDefault(); update({ x: 0, y: 1 }); position(); }
              else if (event.key === 'Escape') { event.preventDefault(); toggle(true); }
            }}>
            <canvas key={`${model.url}-${retry}`} ref={canvas} className={styles.canvas} aria-hidden="true" data-ready={status === 'ready'} />
            {status === 'loading' && <span className={styles.placeholder} role="status"><PawPrint size={28} /><span>{progress >= 95 ? '准备 3D 画面…' : `加载 3D 桌宠…${progress ? ` ${progress}%` : ''}`}</span></span>}
          </button>
          {status === 'error' && <div className={styles.error} role="status"><span>3D 桌宠暂时无法显示，请重试或收起</span><button type="button" onClick={() => { setStatus('loading'); setRetry(value => value+1); }}>重新加载</button></div>}
          <div className={styles.toolbar}>
          <div ref={controlBar} className={styles.controls} role="group" aria-label="桌宠操作">
            {mode === '3d' && <div ref={faceControl} className={styles.expressionControl}>
              <button ref={faceButton} type="button" disabled={status !== 'ready'} aria-label="切换3D表情"
                aria-expanded={faceMenuOpen} aria-controls="desktop-pet-expressions"
                title={`切换表情（当前：${pet3dExpressions.find(item => item.id === face)?.name}）`}
                onClick={() => { setFaceMenuOpen(value => !value); setActionMenuOpen(false); setSizeMenuOpen(false); }}><Smile size={15} /></button>
              {faceMenuOpen && <div id="desktop-pet-expressions" className={styles.expressions} role="group" aria-label="3D表情">
                {pet3dExpressions.map(item => <button key={item.id} type="button" aria-pressed={face === item.id} onClick={() => {
                  selectedFace.current = item.id; setFace(item.id); renderer.current?.setExpression?.(item.id);
                  setFaceMenuOpen(false); faceButton.current?.focus({ preventScroll: true });
                }}>{item.name}</button>)}
              </div>}
            </div>}
            {mode === '3d' && <div ref={actionControl} className={styles.expressionControl}>
              <button ref={actionButton} type="button" disabled={status !== 'ready' || paused} aria-label="选择3D动作"
                aria-expanded={actionMenuOpen} aria-controls="desktop-pet-actions" title={paused ? '继续动画后可播放动作' : '选择动作'}
                onClick={() => { setActionMenuOpen(value => !value); setFaceMenuOpen(false); setSizeMenuOpen(false); }}><Hand size={15} /></button>
              {actionMenuOpen && <div id="desktop-pet-actions" className={styles.expressions} role="group" aria-label="3D动作">
                {pet3dActions.map(item => <button key={item.id} type="button" onClick={() => {
                  renderer.current?.playAction?.(item.id); setActionMenuOpen(false); actionButton.current?.focus({ preventScroll: true });
                }}>{item.name}</button>)}
              </div>}
            </div>}
            {mode === '3d' && <div ref={sizeControl} className={styles.expressionControl}>
              <button ref={sizeButton} type="button" aria-label="调整3D模型大小" title="调整大小" aria-expanded={sizeMenuOpen} aria-controls="desktop-pet-size"
                onClick={() => { setSizeMenuOpen(value => !value); setFaceMenuOpen(false); setActionMenuOpen(false); }}><Scaling size={15} /></button>
              {sizeMenuOpen && <div id="desktop-pet-size" className={`${styles.expressions} ${styles.sizePanel}`}>
                <label htmlFor="desktop-pet-size-range">模型大小 <output>{preferences.size3d}%</output></label>
                <input id="desktop-pet-size-range" type="range" min="25" max="100" step="1" value={preferences.size3d}
                  onChange={event => resizeModel(Number(event.target.value))} />
                <button type="button" onClick={() => resizeModel(50)}>恢复默认 · 50%</button>
              </div>}
            </div>}
            <button type="button" disabled={status !== 'ready'} onClick={() => update({ paused: !current.current.paused })}
              aria-label={preferences.paused ? '继续桌宠动画' : '暂停桌宠动画'} aria-pressed={preferences.paused} title={preferences.paused ? '继续动画' : '暂停动画'}>
              {preferences.paused ? <Play size={15} /> : <Pause size={15} />}
            </button>
            <button type="button" onClick={() => { update({ x: 0, y: 1 }); position(); }} aria-label="桌宠回到左下角" title="回到左下角"><RotateCcw size={15} /></button>
            <button type="button" onClick={() => toggle(true)} aria-label="收起桌宠" title="收起"><ChevronDown size={16} /></button>
          </div>
          </div>
          <span id="desktop-pet-help" className={styles.srOnly}>点击换表情，拖动换位置。键盘方向键可移动，Home 回到左下角，Escape 收起。</span>
        </>
      )}
    </div>, document.body,
  );
}
