'use client';

import { useEffect, useId, useRef, useState } from 'react';
import { ChevronLeft, ChevronRight, Delete, GripVertical, Hash, Keyboard, RotateCcw, X } from 'lucide-react';

import { useQuestionJumpPosition } from './use-question-jump-position';

interface QuestionJumpProps {
  minId: number;
  maxId: number;
  busy?: boolean;
  floating?: boolean;
  onJump: (id: number) => Promise<void>;
}

export function QuestionJump({ minId, maxId, busy = false, floating = false, onJump }: QuestionJumpProps) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState('');
  const [message, setMessage] = useState('');
  const [jumping, setJumping] = useState(false);
  const [presetStart, setPresetStart] = useState(0);
  const input = useRef<HTMLInputElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  const submitting = useRef(false);
  const id = useId();
  const floatingPosition = useQuestionJumpPosition(floating);
  const disabled = busy || jumping;
  const firstPreset = Math.max(0, Math.ceil(minId / 5) - 1);
  const lastPreset = Math.max(firstPreset, Math.floor(maxId / 5) - 4);
  const start = Math.max(firstPreset, Math.min(presetStart, lastPreset));
  const keys = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '清空', '0', '退格'];
  const keyClass = 'inline-flex min-h-10 items-center justify-center rounded-md border border-border bg-background px-2 text-sm tabular-nums hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-40 disabled:cursor-not-allowed';

  useEffect(() => { if (open) input.current?.focus({ preventScroll: true }); }, [open]);

  const close = () => { setOpen(false); toggle.current?.focus({ preventScroll: true }); };
  const changeValue = (next: string) => {
    setValue(next.replace(/[^0-9]/g, '').slice(0, 10));
    setMessage('');
  };
  const pressKey = (key: string) => {
    if (key === '清空') changeValue('');
    else {
      const from = input.current?.selectionStart ?? value.length;
      const to = input.current?.selectionEnd ?? value.length;
      const prefix = key === '退格' && from === to ? Math.max(0, from - 1) : from;
      changeValue(value.slice(0, prefix) + (key === '退格' ? '' : key) + value.slice(to));
      requestAnimationFrame(() => input.current?.setSelectionRange(prefix + (key === '退格' ? 0 : 1), prefix + (key === '退格' ? 0 : 1)));
    }
    input.current?.focus({ preventScroll: true });
  };
  const jump = async (target = Number(value)) => {
    if (submitting.current || disabled) return;
    if (!Number.isSafeInteger(target) || target < 1) { setMessage('输入卡片上的编号，比如 25。'); return; }
    submitting.current = true; setJumping(true); setMessage(''); setValue(String(target));
    setPresetStart(Math.max(0, Math.floor((target - 1) / 20) * 4));
    try {
      await onJump(target);
      setOpen(false);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : '没能跳过去，请再试一次。');
    } finally { submitting.current = false; setJumping(false); }
  };

  const entry = <form onSubmit={event => { event.preventDefault(); void jump(); }} className="flex min-w-0 items-center gap-2">
    <label htmlFor={`${id}-number`} className="sr-only">投稿编号</label>
    <div className="relative min-w-0 flex-1">
      <Hash size={14} aria-hidden="true" className="pointer-events-none absolute left-2.5 top-3 text-primary" />
      <input ref={input} id={`${id}-number`} type="text" inputMode="numeric" pattern="[0-9]*" autoComplete="off"
        value={value} onChange={event => changeValue(event.target.value)} disabled={disabled}
        placeholder="投稿编号" aria-describedby={message ? `${id}-message` : undefined}
        className="h-10 w-full rounded-md border border-border bg-background pl-7 pr-2 text-sm tabular-nums placeholder:text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50" />
    </div>
    <button type="submit" disabled={disabled} className="h-10 shrink-0 rounded-md bg-primary px-3 text-sm text-primary-foreground hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50">{jumping ? '查找中' : '跳转'}</button>
  </form>;

  return <div className={floating ? 'fixed z-40' : 'relative w-full sm:w-[270px]'}
    style={floating ? { left: floatingPosition.position?.x ?? 0, top: floatingPosition.position?.y ?? 0, visibility: floatingPosition.visible ? 'visible' : 'hidden' } : undefined}
    onKeyDown={event => { if (event.key === 'Escape' && open) { event.stopPropagation(); close(); } }}>
    {floating ? <div className="flex items-center rounded-lg border border-border bg-card text-primary shadow-md">
      <button type="button" {...floatingPosition.handle} aria-label="拖动跳转面板；方向键移动，Home 恢复位置" title="拖动调整位置" className={`flex h-11 w-7 touch-none items-center justify-center rounded-l-lg hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring ${floatingPosition.dragging ? 'cursor-grabbing' : 'cursor-grab'}`}><GripVertical size={16} /></button>
      <button ref={toggle} type="button" onClick={() => setOpen(!open)} aria-expanded={open} aria-controls={`${id}-panel`}
      className="flex min-h-11 items-center gap-1.5 rounded-r-lg pl-1 pr-3 text-sm text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"><Hash size={17} />{open ? '收起跳转' : '快速跳转'}</button></div>
      : <div className="flex items-center gap-2"><div className="min-w-0 flex-1">{entry}</div><button ref={toggle} type="button" aria-label={open ? '收起数字键盘' : '打开数字键盘'} aria-expanded={open} aria-controls={`${id}-panel`} onClick={() => setOpen(!open)} className={keyClass}><Keyboard size={18} /></button></div>}
    {open && <section id={`${id}-panel`} aria-label="快速跳转" style={floating ? floatingPosition.panelStyle : undefined} className={`absolute right-0 w-[250px] max-w-[calc(100vw-2rem)] overflow-y-auto overscroll-contain rounded-xl border border-border bg-card p-3 text-foreground shadow-lg ${floating ? '!right-auto' : 'top-12 z-20 max-h-[55dvh]'}`}>
      <div className="mb-3 flex items-center justify-between"><span className="text-sm font-medium">跳到哪一条？</span>{floating && <button type="button" onClick={floatingPosition.reset} title="恢复到卡片旁边" aria-label="恢复跳转面板位置" className="ml-auto rounded p-1 text-muted-foreground hover:bg-accent"><RotateCcw size={15} /></button>}<button type="button" onClick={close} aria-label="收起跳转" className="rounded p-1 text-muted-foreground hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"><X size={16} /></button></div>
      {floating && entry}
      <div className="my-3 grid grid-cols-3 gap-1.5" aria-label="数字键盘">{keys.map(key => <button key={key} type="button" disabled={disabled} aria-label={key} onMouseDown={event => event.preventDefault()} onClick={() => pressKey(key)} className={keyClass}>{key === '退格' ? <Delete size={17} /> : key}</button>)}</div>
      <div className="flex items-center gap-1" aria-label="每五条快速跳转">
        <button type="button" disabled={start <= firstPreset || disabled} aria-label="更小的编号" onClick={() => setPresetStart(Math.max(firstPreset, start - 4))} className={`${keyClass} !px-1`}><ChevronLeft size={15} /></button>
        {[0, 1, 2, 3].map(offset => (start + offset + 1) * 5).filter(number => number <= maxId).map(number => <button type="button" key={number} disabled={disabled} onClick={() => void jump(number)} className={`${keyClass} min-w-0 flex-1 !px-0.5 text-xs`}>{number}</button>)}
        <button type="button" disabled={start >= lastPreset || disabled} aria-label="更大的编号" onClick={() => setPresetStart(Math.min(lastPreset, start + 4))} className={`${keyClass} ml-auto !px-1`}><ChevronRight size={15} /></button>
      </div>
      {message && <p id={`${id}-message`} role="alert" className="mt-2 text-xs leading-relaxed text-destructive">{message}</p>}
    </section>}
    {message && !open && <p id={`${id}-message`} role="alert" className={`mt-2 max-w-[250px] rounded-md border border-border bg-card p-2 text-xs leading-relaxed text-destructive ${floating ? '' : 'absolute right-0 top-full z-30 shadow-md'}`}>{message}</p>}
  </div>;
}
