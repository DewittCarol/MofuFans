'use client';

import { useState, useRef, useEffect, useMemo } from 'react';
import { addEmoji, EmojiData } from '@/lib/api';

import { useEmojiCatalog } from '@/lib/emoji-catalog';

interface EmojiPickerProps {
  questionId: number;
  emojis: EmojiData[];
  onReact: (emojis: EmojiData[]) => void;
}

export function EmojiPicker({ questionId, emojis: emojisProp, onReact }: EmojiPickerProps) {
  const { emojis: catalog, loading, error: catalogError, refresh, allowRepeatedReactions } = useEmojiCatalog();
  const available = catalog.filter(item => item.kind === 'reaction' && !item.is_deleted);
  const byValue = useMemo(() => new Map(catalog.filter(item => item.kind === 'reaction').map(item => [item.value, item])), [catalog]);
  const [panelOpen, setPanelOpen] = useState(false);
  const [posted, setPosted] = useState(false);
  const blocked = posted && !allowRepeatedReactions;
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const pendingRef = useRef(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const pickerRef = useRef<HTMLDivElement>(null);

  // Sort emojis by count
  const emojis = [...emojisProp].sort((a, b) => b.count - a.count);

  // Close panel when clicking outside
  useEffect(() => {
    if (!panelOpen) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (pickerRef.current && !pickerRef.current.contains(e.target as Node)) {
        setPanelOpen(false);
      }
    };

    const escape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { setPanelOpen(false); triggerRef.current?.focus(); }
    };
    pickerRef.current?.querySelector<HTMLButtonElement>('[data-reaction-option]')?.focus();
    document.addEventListener('keydown', escape);
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('keydown', escape);
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [panelOpen]);

  const handlePostEmoji = async (value: string) => {
    if (blocked || pendingRef.current) return;
    pendingRef.current = true;
    setPending(true); setError(''); setPanelOpen(false);
    triggerRef.current?.focus();
    try {
      const res = await addEmoji(questionId, value);
      if (res.code === 200) {
        setPosted(true);
        if (res.data?.emojis) onReact(res.data.emojis);
      } else {
        setError(res.message || '没有贴上，请重试');
      }
    } catch {
      setError('网络断开了，请重试');
    } finally {
      pendingRef.current = false; setPending(false);
    }
  };

  return (
    <div ref={pickerRef} className="picker inline-block select-none relative">
      {panelOpen && (
        <div className="panel absolute bottom-12 right-4 w-[240px] max-w-[calc(100vw-64px)] max-h-[280px] overflow-y-auto z-[999] bg-card p-2.5 rounded border-2 border-dashed border-[var(--fabric-stitch)] grid grid-cols-5 shadow-lg">
          {loading && <p role="status" className="col-span-5 p-2 text-sm">加载中…</p>}
          {catalogError && <button type="button" className="col-span-5 p-2 text-xs text-destructive" onClick={refresh}>表情加载失败，点击重试</button>}
          {!loading && !catalogError && !available.length && <p className="col-span-5 p-2 text-sm text-muted-foreground">暂时没有可用的表情</p>}
          {available.map(({ id, value, url: asset, name }) => (
            <button
              type="button"
              data-reaction-option
              aria-label={`贴 ${name}`} title={name}
              disabled={blocked || pending}
              key={id}
              className="emoji-button flex text-base items-center justify-center cursor-pointer m-0.5 p-1 rounded hover:bg-accent transition-all duration-200"
              onClick={() => handlePostEmoji(value)}
            >
              {asset ? (
                <img src={asset} alt={name} className="h-7 w-7 object-contain" />
              ) : (
                value
              )}
            </button>
          ))}
        </div>
      )}
      <div className="flex flex-row">
        {emojis.map(({ value, count }) => (
          <button
            type="button"
            aria-label={`${byValue.get(value)?.is_deleted ? "已移除表情" : "贴"} ${byValue.get(value)?.name || value}，已有 ${count} 个`}
            disabled={blocked || pending || !byValue.has(value) || byValue.get(value)?.is_deleted}
            key={value}
            className="emoji-button flex text-base items-center cursor-pointer m-0.5 px-1 border border-dashed border-[var(--fabric-stitch)] rounded hover:bg-accent transition-all duration-200"
            onClick={() => handlePostEmoji(value)}
          >
            {byValue.get(value)?.url ? (
              <img src={byValue.get(value)!.url} alt={byValue.get(value)!.name} className="h-5 w-5 object-contain" />
            ) : (
              value
            )}
            <span className="text-xs ml-0.5">{count}</span>
          </button>
        ))}
        <button
          type="button"
          ref={triggerRef}
          aria-label={blocked ? "已贴表情" : pending ? "正在贴表情" : "贴表情"}
          aria-expanded={panelOpen}
          aria-busy={pending}
          aria-disabled={blocked || pending}
          className="emoji-button flex text-base items-center cursor-pointer m-0.5 px-1 border border-dashed border-[var(--fabric-stitch)] rounded hover:bg-accent transition-all duration-200"
          onClick={() => { if (!blocked && !pending) { setError(''); if (!panelOpen) void refresh(); setPanelOpen(!panelOpen); } }}
        >
          <div
            className="h-6 w-6 bg-current"
            style={{
              maskImage: 'url(/emoji_add.svg)',
              WebkitMaskImage: 'url(/emoji_add.svg)',
            }}
          />
        </button>
      </div>
      <span className="sr-only" role="status">{posted ? '已贴表情' : pending ? '正在贴表情' : ''}</span>
      {error && <p role="alert" className="absolute bottom-9 right-0 w-max max-w-[240px] rounded border border-border bg-card px-3 py-2 text-xs text-destructive shadow-md">{error}</p>}
    </div>
  );
}
