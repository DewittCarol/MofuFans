'use client';

import { useState, useRef } from 'react';
import { Popover } from 'radix-ui';

import { useEmojiCatalog } from '@/lib/emoji-catalog';

interface InputEmojiPickerProps {
  onSelect: (tag: string) => void;
}

export function InputEmojiPicker({ onSelect }: InputEmojiPickerProps) {
  const { emojis, collections, loading, error, refresh } = useEmojiCatalog();
  const available = emojis.filter(emoji => emoji.kind === 'input' && !emoji.is_deleted);
  const sets = collections.map(collection => ({
    ...collection,
    cover: available.find(emoji => emoji.collection_id === collection.id),
  })).filter(collection => collection.cover);
  const [collectionID, setCollectionID] = useState<number | null>(null);
  const selected = sets.find(collection => collection.id === collectionID) || sets[0];
  const options = available.filter(emoji => emoji.collection_id === selected?.id);
  const [panelOpen, setPanelOpen] = useState(false);
  const insertedRef = useRef(false);

  const handleOpenChange = (open: boolean) => {
    if (open) {
      insertedRef.current = false;
      void refresh();
    }
    setPanelOpen(open);
  };

  const handleSelect = (tag: string) => {
    insertedRef.current = true;
    onSelect(tag);
    setPanelOpen(false);
  };

  return (
    <Popover.Root open={panelOpen} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <button
          type="button"
          aria-label="插入表情"
          className="p-2 text-primary hover:bg-accent rounded transition-all duration-200 border-2 border-dashed border-[var(--fabric-stitch)]"
          title="插入表情"
        >
          <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <circle cx="12" cy="12" r="10"/>
            <path d="M8 14s1.5 2 4 2 4-2 4-2"/>
            <line x1="9" y1="9" x2="9.01" y2="9"/>
            <line x1="15" y1="9" x2="15.01" y2="9"/>
          </svg>
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top" align="end" sideOffset={8} collisionPadding={16}
          aria-label="选择表情"
          onCloseAutoFocus={event => {
            // The form restores the text cursor after inserting an emoji.
            if (insertedRef.current) event.preventDefault();
          }}
          className="z-[999] flex h-96 w-96 max-w-[calc(100vw-32px)] max-h-[min(24rem,var(--radix-popover-content-available-height))] flex-col rounded-lg border border-border bg-card p-3 shadow-lg"
        >
          {!!sets.length && <>
            <div role="group" aria-label="投稿表情合集" className="mb-2 flex max-h-28 shrink-0 flex-wrap gap-1 overflow-y-auto overscroll-contain border-b border-border p-1 pb-2">
              {sets.map(collection => <button
                key={collection.id} type="button" aria-label={collection.name}
                aria-pressed={selected?.id === collection.id} title={collection.name}
                className={`flex size-11 shrink-0 items-center justify-center rounded-md border focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring ${selected?.id === collection.id ? 'border-primary/50 bg-secondary text-primary' : 'border-transparent text-muted-foreground hover:bg-accent'}`}
                onClick={() => setCollectionID(collection.id)}
              >
                {collection.cover?.url
                  ? <img src={collection.cover.url} alt="" aria-hidden="true" width={28} height={28} draggable={false} className="size-7 object-contain" />
                  : <span aria-hidden="true" className="text-sm">{collection.name.slice(0, 1)}</span>}
              </button>)}
            </div>
            <p aria-live="polite" className="mb-2 shrink-0 break-words px-1 text-xs text-muted-foreground">{selected?.name}</p>
          </>}
          {loading && <p role="status" className="p-3 text-center text-sm">加载中…</p>}
          {error && <button type="button" className="p-2 text-sm text-destructive" onClick={refresh}>{error}，点击重试</button>}
          {!loading && !error && !options.length && <p className="p-3 text-center text-sm text-muted-foreground">暂时没有可用的表情</p>}
          <div key={selected?.id} aria-label={selected ? `${selected.name}表情` : '投稿表情'} className="grid min-h-0 flex-1 content-start grid-cols-[repeat(auto-fill,minmax(3.5rem,1fr))] gap-1 overflow-y-auto overscroll-contain">
            {options.map(({ name, value, url }) => (
              <button type="button" aria-label={`插入 ${name}`}
                key={value}
                className="flex min-h-[4.5rem] min-w-0 flex-col items-center justify-start gap-1 rounded p-1 hover:bg-accent focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-ring"
                onClick={() => handleSelect(value)}
                title={name}
              >
                <img src={url} alt="" aria-hidden="true" width={40} height={40} draggable={false} className="size-10 shrink-0 object-contain" />
                <span className="w-full break-words text-center text-xs leading-4 text-muted-foreground">{name}</span>
              </button>
            ))}
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
