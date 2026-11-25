'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { Upload, Trash2, Undo2, Pencil, SmilePlus, FolderPlus } from 'lucide-react';
import { DndContext, DragOverlay, closestCenter, PointerSensor, KeyboardSensor, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core';
import { SortableContext, rectSortingStrategy, arrayMove, sortableKeyboardCoordinates } from '@dnd-kit/sortable';
import { SortableEmoji } from '@/components/sortable-emoji';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { useEmojiCatalog, type EmojiAsset, type EmojiKind, type EmojiCollection } from '@/lib/emoji-catalog';

async function mutateEmoji(path: string, method: 'POST' | 'PUT' | 'DELETE', data?: unknown) {
  const response = await fetch(path, {
    method, credentials: 'include', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  const result = await response.json();
  if (!response.ok || result.code !== 200) throw new Error(result.message || '操作失败，请重试');
  return result.data;
}

const changeEmojiStatus = (ids: number[], is_deleted: boolean) => mutateEmoji('/api/emojis/status', 'PUT', { ids, is_deleted });
const commonEmojis = [
  ['🐶', '小狗'], ['🥰', '喜欢'], ['😂', '笑哭'], ['🥹', '感动'],
  ['🤔', '思考'], ['🙈', '害羞'], ['😈', '小恶魔'], ['🤡', '小丑'],
  ['✨', '闪亮'], ['🔥', '火热'], ['👏', '鼓掌'], ['❤️', '爱心'],
];

export function EmojiSettings() {
  const { emojis, collections, loading, error: loadError, refresh } = useEmojiCatalog();
  const [kind, setKind] = useState<EmojiKind>('input');
  const [collectionID, setCollectionID] = useState<number | null>(null);
  const [removedCollectionIDs, setRemovedCollectionIDs] = useState<Set<number>>(new Set());
  const availableCollections = collections.filter(item => !removedCollectionIDs.has(item.id));
  const collection = availableCollections.find(item => item.id === collectionID) || availableCollections[0];
  const [deletingCollection, setDeletingCollection] = useState<(EmojiCollection & { count: number }) | null>(null);
  const [deleteCollectionError, setDeleteCollectionError] = useState('');
  const cancelCollectionDelete = useRef<HTMLButtonElement>(null);
  const [collectionEdit, setCollectionEdit] = useState<'create' | 'rename' | null>(null);
  const [collectionName, setCollectionName] = useState('');
  const [collectionError, setCollectionError] = useState('');
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [progress, setProgress] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [undoIDs, setUndoIDs] = useState<number[]>([]);
  const [deleting, setDeleting] = useState<EmojiAsset[]>([]);
  const [renaming, setRenaming] = useState<EmojiAsset | null>(null);
  const [editName, setEditName] = useState('');
  const [editError, setEditError] = useState('');
  const [addingUnicode, setAddingUnicode] = useState(false);
  const [unicodeValue, setUnicodeValue] = useState('');
  const [unicodeName, setUnicodeName] = useState('');
  const [unicodeError, setUnicodeError] = useState('');
  const input = useRef<HTMLInputElement>(null);
  const [dragID, setDragID] = useState<number | null>(null);
  const [dragSnapshot, setDragSnapshot] = useState<EmojiAsset[] | null>(null);
  const [pendingOrder, setPendingOrder] = useState<EmojiAsset[] | null>(null);
  const source = useMemo(() => emojis.filter(emoji => emoji.kind === kind && !emoji.is_deleted && (kind === 'reaction' || emoji.collection_id === collection?.id)), [emojis, kind, collection?.id]);
  const active = pendingOrder || dragSnapshot || source;
  const controlsDisabled = busy || dragID !== null;
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }), useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }));
  useEffect(() => {
    // Also clear stale actions when another administrator removes this collection.
    setSelected(new Set()); setUndoIDs([]); setPendingOrder(null);
    setDragID(null); setDragSnapshot(null); setRenaming(null); setDeleting([]); setCollectionEdit(null);
  }, [kind, collection?.id]);
  useEffect(() => {
    // Keep the successful order visible while a slow catalog refresh catches up.
    if (pendingOrder && pendingOrder.map(item => item.id).join() === source.map(item => item.id).join()) setPendingOrder(null);
  }, [emojis, pendingOrder, source]);
  const resetScope = () => { setSelected(new Set()); setError(''); setNotice(''); setRenaming(null); setAddingUnicode(false); setCollectionEdit(null); setPendingOrder(null); };

  const saveCollection = async () => {
    if (!collectionEdit || busyRef.current || !collectionName.trim() || (collectionEdit === 'rename' && !collection)) return;
    busyRef.current = true; setBusy(true); setCollectionError('');
    try {
      const saved = await mutateEmoji(collectionEdit === 'create' ? '/api/emoji-collections' : `/api/emoji-collections/${collection?.id}`, collectionEdit === 'create' ? 'POST' : 'PUT', { name: collectionName.trim() });
      await refresh(); resetScope(); setCollectionID(saved.id); setNotice(collectionEdit === 'create' ? `已创建“${saved.name}”，现在可以上传表情。` : '合集名称已保存');
    } catch (error) { setCollectionError(error instanceof Error ? error.message : '合集保存失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };
  const removeCollection = async () => {
    if (!deletingCollection || busyRef.current) return;
    busyRef.current = true; setBusy(true); setDeleteCollectionError('');
    try {
      await mutateEmoji(`/api/emoji-collections/${deletingCollection.id}`, 'DELETE');
      // Hide it immediately even if the follow-up catalog request fails.
      setRemovedCollectionIDs(current => new Set([...current, deletingCollection.id]));
      setCollectionID(availableCollections.find(item => item.id !== deletingCollection.id)?.id ?? null);
      resetScope(); setUndoIDs([]); setDeleting([]); setDeletingCollection(null);
      setNotice(`已删除“${deletingCollection.name}”，历史投稿中的表情仍可显示。`);
      await refresh();
    } catch (error) { setDeleteCollectionError(error instanceof Error ? error.message : '合集删除失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };
  const saveOrder = async ({ active: dragged, over }: DragEndEvent) => {
    setDragID(null); setDragSnapshot(null);
    if (!over || dragged.id === over.id || busyRef.current) return;
    const oldIndex = active.findIndex(item => item.id === dragged.id), newIndex = active.findIndex(item => item.id === over.id);
    if (oldIndex < 0 || newIndex < 0) return;
    const next = arrayMove(active, oldIndex, newIndex);
    busyRef.current = true; setBusy(true); setPendingOrder(next); setError(''); setNotice('正在保存顺序…');
    try {
      await mutateEmoji('/api/emojis/order', 'PUT', { kind, collection_id: kind === 'input' ? collection?.id : 0, ids: next.map(item => item.id), expected_ids: active.map(item => item.id) });
      setNotice('顺序已保存，前台会同步更新'); await refresh();
    } catch (error) {
      setPendingOrder(null); setNotice(''); setError(error instanceof Error ? error.message : '排序失败，请重试'); await refresh();
    } finally { setBusy(false); busyRef.current = false; }
  };
  const selectedCount = active.filter(emoji => selected.has(emoji.id)).length;
  const allSelected = active.length > 0 && active.every(emoji => selected.has(emoji.id));

  const upload = async (files: File[]) => {
    if (!files.length || busyRef.current || (kind === 'input' && !collection)) return;
    if (files.length > 20) { setError('每次最多上传 20 张，请分批选择。'); return; }
    busyRef.current = true; setBusy(true); setError(''); setNotice('');
    const failures: string[] = []; let succeeded = 0;
    for (let index = 0; index < files.length; index++) {
      const file = files[index];
      setProgress(`正在上传 ${index + 1} / ${files.length}`);
      try {
        if (file.size > 5 * 1024 * 1024) throw new Error('超过 5 MB');
        const data = new FormData(); data.append('kind', kind); data.append('file', file);
        if (kind === 'input' && collection) data.append('collection_id', String(collection.id));
        const response = await fetch('/api/emojis', { method: 'POST', credentials: 'include', body: data });
        const result = await response.json();
        if (!response.ok || result.code !== 200) throw new Error(result.message || '上传失败');
        succeeded++;
      } catch (error) { failures.push(`${file.name}：${error instanceof Error ? error.message : '网络错误'}`); }
    }
    setPendingOrder(null); await refresh();
    setNotice(succeeded ? `已添加 ${succeeded} 个${kind === 'input' ? '投稿' : '互动'}表情` : '');
    setError(failures.join('\n')); setProgress(''); setBusy(false); busyRef.current = false;
  };

  const remove = async () => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(true); setError(''); setNotice('');
    const ids = deleting.map(item => item.id);
    try {
      await changeEmojiStatus(ids, true);
      setUndoIDs(ids); setSelected(new Set()); setDeleting([]); setPendingOrder(null);
      setNotice(`已删除 ${ids.length} 个表情`); await refresh();
    } catch (error) { setError(error instanceof Error ? error.message : '删除失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };
  const undo = async () => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(true); setError('');
    try {
      await changeEmojiStatus(undoIDs, false); setUndoIDs([]); setPendingOrder(null);
      setNotice('已恢复删除的表情'); await refresh();
    } catch (error) { setError(error instanceof Error ? error.message : '恢复失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };

  const rename = async () => {
    if (!renaming || busyRef.current) return;
    if (!editName.trim()) { setEditError('请输入表情名称'); return; }
    busyRef.current = true; setBusy(true); setEditError(''); setNotice('');
    try {
      await mutateEmoji(`/api/emojis/${renaming.id}/name`, 'PUT', { name: editName.trim() });
      setRenaming(null); setPendingOrder(null); setNotice(`已改名为“${editName.trim()}”`); await refresh();
    } catch (error) { setEditError(error instanceof Error ? error.message : '改名失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };

  const addUnicode = async () => {
    if (busyRef.current) return;
    busyRef.current = true; setBusy(true); setUnicodeError(''); setNotice('');
    try {
      await mutateEmoji('/api/emojis/unicode', 'POST', { value: unicodeValue.trim(), name: unicodeName.trim() });
      setNotice(`已添加 ${unicodeValue.trim()}`); setUnicodeValue(''); setUnicodeName(''); setUndoIDs([]); setPendingOrder(null); await refresh();
    } catch (error) { setUnicodeError(error instanceof Error ? error.message : '添加失败，请重试'); }
    finally { setBusy(false); busyRef.current = false; }
  };

  return <Card id="emoji-settings" tabIndex={-1} aria-label="表情包设置" className="hover:translate-y-0">
    <CardHeader>
      <CardTitle>表情包设置</CardTitle>
      <CardDescription>修改后直接生效。</CardDescription>
    </CardHeader>
    <CardContent className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div role="group" aria-label="表情类型" className="inline-flex rounded-lg border border-border p-1">
          {([['input', '投稿表情'], ['reaction', '互动表情']] as const).map(([value, label]) => <Button key={value} type="button"
            variant={kind === value ? 'secondary' : 'ghost'} size="sm" aria-pressed={kind === value} disabled={controlsDisabled}
            onClick={() => { setKind(value); resetScope(); }}>{label}</Button>)}
        </div>
        <div className="flex flex-wrap gap-2">
          {kind === 'reaction' && <Button type="button" variant="outline" disabled={controlsDisabled || loading || !!loadError} aria-expanded={addingUnicode} aria-controls="add-unicode-emoji"
            onClick={() => { setAddingUnicode(!addingUnicode); setRenaming(null); setUnicodeError(''); }}><SmilePlus size={16} />添加 Emoji</Button>}
          <Button type="button" disabled={controlsDisabled || loading || !!loadError || (kind === 'input' && !collection)} onClick={() => input.current?.click()}><Upload size={16} />{busy && progress ? progress : kind === 'input' ? '上传到当前合集' : '上传图片'}</Button>
        </div>
        <input ref={input} type="file" multiple accept="image/png,image/jpeg,image/gif,image/webp" className="sr-only" aria-label="选择表情图片" disabled={busy}
          onChange={event => { const files = Array.from(event.target.files || []); event.target.value = ''; void upload(files); }} />
      </div>
      {kind === 'input' && <div className="flex flex-wrap items-center gap-2">
        <Label htmlFor="emoji-collection">投稿合集</Label>
        <Select value={collection ? String(collection.id) : ''} disabled={controlsDisabled || loading || !!loadError} onValueChange={value => { setCollectionID(Number(value)); resetScope(); }}>
          <SelectTrigger id="emoji-collection" className="w-full sm:w-60"><SelectValue placeholder="选择合集" /></SelectTrigger>
          <SelectContent>{availableCollections.map(item => <SelectItem key={item.id} value={String(item.id)}>{item.name}</SelectItem>)}</SelectContent>
        </Select>
        <Button type="button" variant="outline" size="sm" disabled={controlsDisabled || loading || !!loadError}
          onClick={() => { setCollectionEdit('create'); setCollectionName(''); setCollectionError(''); setRenaming(null); }}><FolderPlus size={15} />新建合集</Button>
        <Button type="button" variant="ghost" size="sm" disabled={controlsDisabled || !collection || !!loadError}
          onClick={() => { if (!collection) return; setCollectionEdit('rename'); setCollectionName(collection.name); setCollectionError(''); setRenaming(null); }}><Pencil size={14} />合集改名</Button>
        <Button type="button" variant="ghost" size="sm" disabled={controlsDisabled || loading || !collection || !!loadError}
          onClick={() => { if (!collection) return; setDeletingCollection({ ...collection, count: source.length }); setDeleteCollectionError(''); }}><Trash2 size={14} />删除合集</Button>
      </div>}
      {kind === 'input' && collectionEdit && <form className="space-y-3 rounded-lg bg-secondary/40 p-4" aria-label={collectionEdit === 'create' ? '新建合集' : '合集改名'} onSubmit={event => { event.preventDefault(); void saveCollection(); }}>
        <Label htmlFor="emoji-collection-name">{collectionEdit === 'create' ? '新合集名称' : '合集名称'}</Label>
        <Input id="emoji-collection-name" autoFocus maxLength={32} value={collectionName} disabled={busy} placeholder="例如：日常表情" aria-invalid={!!collectionError} aria-describedby={collectionError ? 'collection-error' : undefined} onChange={event => setCollectionName(event.target.value)} />
        {collectionError && <p id="collection-error" role="alert" className="text-sm text-destructive">{collectionError}</p>}
        <div className="flex gap-2"><Button type="submit" disabled={busy || !collectionName.trim()}>{collectionEdit === 'create' ? '创建合集' : '保存名称'}</Button><Button type="button" variant="ghost" disabled={busy} onClick={() => setCollectionEdit(null)}>取消</Button></div>
      </form>}
      <p className="text-sm text-muted-foreground">{kind === 'input' ? '用于投稿输入框，点击后插入正文。' : '用于卡片上的贴表情和散落效果。可以直接添加 Emoji，也可以上传图片。'}图片支持 PNG、JPG、GIF、WebP，每张不超过 5 MB；可以一次选多张。</p>
      <p className="text-xs text-muted-foreground">拖动表情上方的手柄即可排序，松手自动保存。也可选中手柄后按空格、方向键，再按空格放下。点击名称改名；删除不影响历史内容。</p>
      {kind === 'reaction' && addingUnicode && <form id="add-unicode-emoji" aria-label="添加 Emoji" className="space-y-3 rounded-lg bg-secondary/40 p-4"
        onSubmit={event => { event.preventDefault(); void addUnicode(); }}>
        <div role="group" aria-label="常用 Emoji" className="flex flex-wrap gap-1">
          {commonEmojis.map(([value, label]) => <button key={value} type="button" aria-label={`选择 ${label} ${value}`} title={label} disabled={busy}
            className="flex h-10 w-10 items-center justify-center rounded-md text-2xl hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-40"
            onClick={() => { setUnicodeValue(value); setUnicodeError(''); }}>{value}</button>)}
        </div>
        <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
          <div className="space-y-1.5"><Label htmlFor="unicode-emoji-value">Emoji 表情</Label>
            <Input id="unicode-emoji-value" autoFocus value={unicodeValue} maxLength={64} placeholder="输入或粘贴，如 🐶" disabled={busy} autoComplete="off"
              aria-invalid={!!unicodeError} aria-describedby={unicodeError ? 'unicode-emoji-error' : 'unicode-emoji-help'}
              onChange={event => { setUnicodeValue(event.target.value); setUnicodeError(''); }} /></div>
          <div className="space-y-1.5"><Label htmlFor="unicode-emoji-name">名称（选填）</Label>
            <Input id="unicode-emoji-name" value={unicodeName} maxLength={32} placeholder="例如：小狗开心" disabled={busy}
              onChange={event => setUnicodeName(event.target.value)} /></div>
        </div>
        <p id="unicode-emoji-help" className="text-xs text-muted-foreground">每次添加一个；名称留空时使用 Emoji 本身。电脑上也可以按 Win + . 打开系统表情面板。</p>
        {unicodeError && <p id="unicode-emoji-error" role="alert" className="text-sm text-destructive">{unicodeError}</p>}
        <div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy || !unicodeValue.trim()}>{busy ? '添加中…' : '添加表情'}</Button>
          <Button type="button" variant="ghost" disabled={busy} onClick={() => setAddingUnicode(false)}>收起</Button></div>
      </form>}
      {renaming && <form aria-label="修改表情名称" className="space-y-3 rounded-lg bg-secondary/40 p-4" onSubmit={event => { event.preventDefault(); void rename(); }}>
        <div className="flex items-center gap-3">
          {renaming.url ? <img src={renaming.url} alt="" className="h-12 w-12 shrink-0 object-contain" /> : <span aria-hidden="true" className="text-3xl">{renaming.value}</span>}
          <div className="min-w-0 flex-1 space-y-1.5"><Label htmlFor="emoji-edit-name">表情名称</Label>
            <Input key={renaming.id} id="emoji-edit-name" autoFocus value={editName} maxLength={32} disabled={busy} aria-invalid={!!editError}
              aria-describedby={editError ? 'emoji-rename-error' : undefined} onChange={event => { setEditName(event.target.value); setEditError(''); }} /></div>
        </div>
        {editError && <p id="emoji-rename-error" role="alert" className="text-sm text-destructive">{editError}</p>}
        <div className="flex flex-wrap gap-2"><Button type="submit" disabled={busy || !editName.trim()}>{busy ? '保存中…' : '保存名称'}</Button>
          <Button type="button" variant="ghost" disabled={busy} onClick={() => setRenaming(null)}>取消</Button></div>
      </form>}
      {loadError && <div role="alert" className="flex items-center gap-3 text-sm text-destructive">{loadError}<Button type="button" variant="outline" size="sm" onClick={refresh}>重试加载</Button></div>}
      <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <label className="flex min-h-9 items-center gap-2"><input type="checkbox" className="h-4 w-4 accent-[var(--primary)]" disabled={busy || !active.length} checked={allSelected}
          onChange={() => setSelected(allSelected ? new Set() : new Set(active.map(item => item.id)))} />全选 <span className="text-muted-foreground">共 {active.length} 个</span></label>
        {!!selectedCount && <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => setDeleting(active.filter(item => selected.has(item.id)))}><Trash2 size={15} />删除所选（{selectedCount}）</Button>}
      </div>
      {loading ? <p role="status" className="py-8 text-center text-sm text-muted-foreground">加载表情中…</p> :
        active.length === 0 ? <p className="rounded-lg border border-dashed border-border px-4 py-10 text-center text-sm text-muted-foreground">{kind === 'input' && !collection ? '还没有投稿合集。点击“新建合集”，再上传表情图片。' : kind === 'input' ? '还没有投稿表情，上传图片即可添加。' : '还没有互动表情，可以添加 Emoji 或上传图片。'}</p> :
        <DndContext sensors={sensors} collisionDetection={closestCenter}
          accessibility={{ screenReaderInstructions: { draggable: '按空格提起表情，方向键移动，空格放下，Esc 取消。' }, announcements: {
            onDragStart: () => '已提起表情，使用方向键移动。',
            onDragOver: ({ over }) => over ? `将移动到第 ${active.findIndex(item => item.id === over.id) + 1} 个位置。` : '移回表情列表可放下。',
            onDragEnd: () => '已放下表情。', onDragCancel: () => '已取消排序。',
          } }}
          onDragStart={({ active: item }) => { setDragSnapshot(active); setDragID(Number(item.id)); }}
          onDragCancel={() => { setDragID(null); setDragSnapshot(null); }} onDragEnd={saveOrder}>
        <SortableContext items={active.map(item => item.id)} strategy={rectSortingStrategy}>
        <div aria-label="可排序的表情" className="grid max-h-[420px] grid-cols-[repeat(auto-fill,minmax(110px,1fr))] gap-3 overflow-y-auto p-1">
          {active.map(emoji => <SortableEmoji key={emoji.id} emoji={emoji} disabled={busy || !!loadError} selected={selected.has(emoji.id)}>
            <label className="flex cursor-pointer flex-col items-center gap-2 pt-7">
              <input type="checkbox" aria-label={`选择 ${emoji.name}`} className="absolute left-2 top-2 h-4 w-4 accent-[var(--primary)]" checked={selected.has(emoji.id)} disabled={busy}
                onChange={() => setSelected(current => { const next = new Set(current); if (next.has(emoji.id)) next.delete(emoji.id); else next.add(emoji.id); return next; })} />
              {emoji.url ? <img src={emoji.url} alt={emoji.name} loading="lazy" className="h-14 w-14 object-contain" /> : <span className="flex h-14 w-14 items-center justify-center text-3xl">{emoji.value}</span>}
            </label>
            <button type="button" disabled={busy} aria-label={`改名 ${emoji.name}`} title={`改名 ${emoji.name}`}
              className="mt-1 flex min-h-8 w-full items-center justify-center gap-1 rounded text-xs text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-40"
              onClick={() => { setRenaming(emoji); setEditName(emoji.name); setEditError(''); setAddingUnicode(false); }}>
              <span className="min-w-0 truncate">{emoji.name}</span><Pencil size={12} className="shrink-0" />
            </button>
            <button type="button" disabled={busy} aria-label={`删除 ${emoji.name}`} title={`删除 ${emoji.name}`} onClick={() => setDeleting([emoji])}
              className="absolute right-1 top-1 rounded p-1.5 text-muted-foreground hover:bg-accent hover:text-destructive focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-40"><Trash2 size={13} /></button>
          </SortableEmoji>)}
        </div>
        </SortableContext>
        {typeof document !== 'undefined' && createPortal(<DragOverlay dropAnimation={null} zIndex={1000}>{dragID !== null && (() => {
          const item = active.find(emoji => emoji.id === dragID);
          return item ? <div className="pointer-events-none flex h-full flex-col items-center justify-center gap-2 rounded-lg bg-card p-4 text-xs shadow-lg">{item.url ? <img src={item.url} alt="" className="h-14 w-14 object-contain" /> : <span className="text-3xl">{item.value}</span>}<span className="max-w-full truncate">{item.name}</span></div> : null;
        })()}</DragOverlay>, document.body)}
        </DndContext>}
      {error && <p role="alert" className="whitespace-pre-wrap break-words text-sm text-destructive">{error}</p>}
      <div className="flex flex-wrap items-center gap-3 text-sm"><span role="status" className="text-primary">{progress || notice}</span>
        {!!undoIDs.length && <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={undo}><Undo2 size={14} />撤销删除</Button>}
      </div>
    </CardContent>
    <Dialog open={!!deletingCollection} onOpenChange={open => { if (!open && !busy) setDeletingCollection(null); }}>
      <DialogContent onOpenAutoFocus={event => { event.preventDefault(); cancelCollectionDelete.current?.focus(); }}>
        <DialogHeader><DialogTitle className="break-words">删除“{deletingCollection?.name}”合集？</DialogTitle>
          <DialogDescription>合集及其中 {deletingCollection?.count ?? 0} 个可用表情将从选择面板中移除，历史投稿中的表情仍可显示。删除合集后无法撤销。</DialogDescription></DialogHeader>
        {deleteCollectionError && <p role="alert" className="text-sm text-destructive">{deleteCollectionError}</p>}
        <DialogFooter><Button ref={cancelCollectionDelete} type="button" variant="outline" disabled={busy} onClick={() => setDeletingCollection(null)}>取消</Button>
          <Button type="button" variant="destructive" disabled={busy} onClick={removeCollection}>{busy ? '删除中…' : '删除合集'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
    <Dialog open={deleting.length > 0} onOpenChange={open => { if (!open && !busy) setDeleting([]); }}>
      <DialogContent><DialogHeader><DialogTitle>删除{deleting.length === 1 ? `“${deleting[0].name}”` : `这 ${deleting.length} 个表情`}？</DialogTitle>
        <DialogDescription>它们将不再出现在表情选择面板里。历史投稿和已贴的表情保持原样，删除后可以撤销。</DialogDescription></DialogHeader>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <DialogFooter><Button type="button" variant="outline" disabled={busy} onClick={() => setDeleting([])}>取消</Button>
          <Button type="button" variant="destructive" disabled={busy} onClick={remove}>{busy ? '删除中…' : '删除表情'}</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  </Card>;
}
