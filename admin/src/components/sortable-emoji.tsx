'use client';

import { useState, type ReactNode } from 'react';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { GripVertical } from 'lucide-react';
import type { EmojiAsset } from '@/lib/emoji-catalog';

export function SortableEmoji({ emoji, disabled, selected, children }: {
  emoji: EmojiAsset; disabled: boolean; selected: boolean; children: ReactNode;
}) {
  const [handleFocused, setHandleFocused] = useState(false);
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({ id: emoji.id, disabled });
  return <div ref={setNodeRef} style={{ transform: CSS.Transform.toString(transform), transition }}
    className={`relative min-w-0 rounded-lg border p-2 motion-reduce:!transition-none ${isDragging ? 'opacity-30' : ''} ${handleFocused ? 'border-primary bg-secondary/70 outline-2 outline-primary' : selected ? 'border-primary bg-secondary/50' : 'border-border bg-background/40'}`}>
    <button ref={setActivatorNodeRef} type="button" {...attributes} {...listeners} disabled={disabled}
      onFocus={() => setHandleFocused(true)} onBlur={() => setHandleFocused(false)}
      aria-label={`拖动排序 ${emoji.name}`} title="拖动排序；也可按空格后用方向键移动"
      className={`absolute left-1/2 top-0.5 flex h-8 w-8 -translate-x-1/2 touch-none items-center justify-center rounded focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-40 ${handleFocused || isDragging ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-primary'} ${isDragging ? 'cursor-grabbing' : 'cursor-grab'}`}>
      <GripVertical size={16} />
    </button>
    {children}
  </div>;
}
