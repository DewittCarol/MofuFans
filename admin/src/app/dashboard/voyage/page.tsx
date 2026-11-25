'use client';

import { useEffect, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft, ChevronLeft, ChevronRight, ExternalLink, Plus, Sailboat, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Dialog, DialogContent, DialogTitle, DialogHeader, DialogDescription, DialogFooter } from '@/components/ui/dialog';
import { voyageRequest, voyageTiers, type VoyageIssue, type VoyageGift, type VoyageTier, type VoyageCatalogKind } from '@/lib/voyage';
import { catalogLabel, monthLabel, validMonth, freshVoyageCatalog, createVoyageEditorDraft, collectVoyageImages, patchVoyageDraft, reuseVoyageImage, validateVoyageIssue } from '@/lib/voyage-editor';
import { useSidebarNavigationGuard } from '@/lib/use-sidebar-navigation-guard';
import { VoyageCatalogCanvas } from '@/components/voyage-catalog-canvas';
import { VoyageImageEditor } from '@/components/voyage-image-editor';
import { VoyageImageLibrary } from '@/components/voyage-image-library';

type EditorDialog = { type: 'images'; index: number | null } | { type: 'month' } | { type: 'delete' } | { type: 'removeGift'; index: number };
const todayMonth = () => { const now = new Date(); return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`; };

export default function VoyageAdminPage() {
  const router = useRouter();
  const [issues, setIssues] = useState<VoyageIssue[]>([]);
  const [month, setMonth] = useState('');
  const [kind, setKind] = useState<VoyageCatalogKind>('guard');
  const [draft, setDraft] = useState<VoyageIssue | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const busyRef = useRef(false);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [pendingNavigation, setPendingNavigation] = useState<(() => void) | null>(null);
  const [activeField, setActiveField] = useState<string | null>(null);
  const [dialog, setDialog] = useState<EditorDialog | null>(null);
  const [moveMonth, setMoveMonth] = useState('');
  const dialogTrigger = useRef<HTMLElement | null>(null);

  const load = async () => {
    setLoading(true); setError('');
    const remembered = sessionStorage.getItem('mofu-voyage-month');
    setMonth(remembered && validMonth(remembered) ? remembered : todayMonth());
    try {
      const next = await voyageRequest<VoyageIssue[]>('/api/voyage/issues');
      setIssues(next);
    } catch (error) { setError(error instanceof Error ? error.message : '加载失败，请重试'); }
    finally { setLoading(false); }
  };
  useEffect(() => { void load(); }, []);
  useEffect(() => {
    if (!dirty && !busy) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); event.returnValue = ''; };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty, busy]);
  const openDialog = (next: EditorDialog) => { dialogTrigger.current = document.activeElement as HTMLElement | null; setDialog(next); };
  const navigate = (action: () => void) => {
    if (busyRef.current) return;
    if (dirty) { setPendingNavigation(() => action); return; }
    action();
  };
  useSidebarNavigationGuard(url => {
    if (busyRef.current) return false;
    if (dirty) { navigate(() => router.push(url.pathname.replace(/^\/admin(?=\/|$)/, '') + url.search + url.hash)); return false; }
    return true;
  });
  const showMonth = (value: string, category = kind) => {
    setMonth(value); setKind(category); setDraft(null); setDirty(false); setActiveField(null); setError(''); setNotice('');
    sessionStorage.setItem('mofu-voyage-month', value);
  };
  const open = (issue: VoyageIssue) => {
    setDraft(createVoyageEditorDraft(issue)); setDirty(!issue.id); setError(''); setNotice('');
    setActiveField(issue.title ? null : 'voyage-title');
  };
  const patch = (values: Partial<VoyageIssue>) => {
    setDraft(current => current ? patchVoyageDraft(current, values) : null); setDirty(true); setNotice(''); setError('');
  };
  const patchGift = (index: number, values: Partial<VoyageGift>) => {
    if (draft) patch({ gifts: draft.gifts.map((gift, i) => i === index ? { ...gift, ...values } : gift) });
  };
  const addGift = (tier?: VoyageTier) => {
    if (!draft || busyRef.current || draft.gifts.length >= 100) return;
    const gift: VoyageGift = { name: '', section: kind === 'guard' ? 'guard' : 'other', event: '', audience: '', shipping_note: '', tiers: kind === 'guard' ? [tier || 'captain'] : [], description: '', images: [] };
    setActiveField(`gift-name-${draft.gifts.length}`); patch({ gifts: [...draft.gifts, gift] });
  };
  const save = async () => {
    if (!draft || busyRef.current) return;
    const invalid = validateVoyageIssue(draft);
    if (invalid) {
      setError(invalid.message);
      if (invalid.field === 'voyage-month') { setMoveMonth(draft.month || month); openDialog({ type: 'month' }); }
      else setActiveField(invalid.field);
      requestAnimationFrame(() => { const target = document.getElementById(invalid.field); target?.focus(); target?.scrollIntoView({ block: 'center' }); });
      return;
    }
    busyRef.current = true; setBusy(true); setError(''); setNotice('');
    try {
      const saved = await voyageRequest<VoyageIssue>(draft.id ? `/api/voyage/issues/${draft.id}` : '/api/voyage/issues', draft.id ? 'PUT' : 'POST', draft);
      setIssues(current => [saved, ...current.filter(issue => issue.id !== saved.id)]);
      setDraft(createVoyageEditorDraft(saved)); setDirty(false); setActiveField(null); setNotice(saved.published ? '已保存，前台可见' : '草稿已保存');
      setMonth(saved.month!); sessionStorage.setItem('mofu-voyage-month', saved.month!);
    } catch (error) { setError(error instanceof Error ? error.message : '保存失败，请重试'); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const remove = async () => {
    if (!draft?.id || busyRef.current) return;
    busyRef.current = true; setBusy(true); setError('');
    try {
      await voyageRequest(`/api/voyage/issues/${draft.id}`, 'DELETE');
      setIssues(current => current.filter(issue => issue.id !== draft.id)); setDraft(null); setDirty(false); setDialog(null); setNotice('已删除');
    } catch (error) { setError(error instanceof Error ? error.message : '删除失败，请重试'); }
    finally { busyRef.current = false; setBusy(false); }
  };
  const upload = async (index: number | null, files: File[]) => {
    if (!draft || !files.length || busyRef.current) return;
    const before = index === null ? (draft.images || []) : draft.gifts[index].images;
    if (files.length + before.length > (index === null ? 24 : 6)) { setError(index === null ? '海报最多 24 张' : '每件礼物或奖品最多 6 张图片'); return; }
    busyRef.current = true; setBusy(true); setError('');
    const images = [...before]; const failures: string[] = [];
    for (const file of files) {
      try {
        if (file.size > 12 * 1024 * 1024) throw new Error('图片超过 12 MB');
        const data = new FormData(); data.append('file', file);
        const response = await fetch('/api/voyage-assets', { method: 'POST', credentials: 'include', body: data });
        const result = await response.json();
        if (!response.ok || result.code !== 200) throw new Error(result.message || '上传失败');
        const url: string = result.data.url;
        if (!images.includes(url)) images.push(url);
      } catch (error) { failures.push(`${file.name}：${error instanceof Error ? error.message : '上传失败'}`); }
    }
    if (index === null) patch({ images }); else patchGift(index, { images });
    setError(failures.join('\n'));
    setBusy(false); busyRef.current = false;
  };
  const year = Number(month.slice(0, 4));
  const years = [...new Set([year - 1, year, year + 1, new Date().getFullYear(), ...issues.map(issue => Number(issue.month?.slice(0, 4)))])].filter(y => y > 0 && y < 10000).sort((a, b) => b - a);
  const monthIssues = issues.filter(issue => issue.month === month);
  const visible = monthIssues.filter(issue => issue.kind === kind).sort((a, b) => b.id - a.id);
  const imageIndex = dialog?.type === 'images' ? dialog.index : null;
  const imageLabel = imageIndex === null ? '海报' : kind === 'guard' ? '礼物图片' : '奖品图片';
  const selectedImages = draft ? imageIndex === null ? draft.images || [] : draft.gifts[imageIndex]?.images || [] : [];
  const imageTarget = imageIndex === null ? '海报' : `${kind === 'guard' ? (voyageTiers.find(tier => draft?.gifts[imageIndex]?.tiers[0] === tier.id)?.name || '') + ' · ' : ''}${draft?.gifts[imageIndex]?.name || '未命名礼物'}`;

  if (!month) return <div className="space-y-6"><h1 className="text-2xl font-semibold">舰礼配置</h1><p role="status" className="text-sm text-muted-foreground">正在加载…</p></div>;
  return <div className="mx-auto w-full max-w-[1440px] space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-4"><h1 className="flex items-center gap-2 text-2xl font-semibold"><Sailboat size={25} />舰礼配置</h1><a href="/voyage/" target="_blank" rel="noreferrer" className="flex min-h-10 items-center gap-1.5 text-sm text-primary underline underline-offset-4">查看前台<ExternalLink size={14} /></a></header>
    <div className="space-y-7">
      <section className="space-y-4" aria-label="全年礼物概览">
        <div className="flex max-w-52 items-center gap-1">
          <Button type="button" variant="ghost" size="icon" disabled={busy || year <= 1} aria-label="上一年" onClick={() => navigate(() => showMonth(`${String(year - 1).padStart(4, '0')}-${month.slice(5)}`))}><ChevronLeft size={18} /></Button>
          <select aria-label="选择年份" value={year} disabled={busy} className="h-10 min-w-0 flex-1 rounded-md bg-transparent px-2 text-base font-semibold focus-visible:outline-2 focus-visible:outline-ring" onChange={event => { const value = `${event.target.value.padStart(4, '0')}-${month.slice(5)}`; navigate(() => showMonth(value)); }}>{years.map(value => <option key={value} value={value}>{value} 年</option>)}</select>
          <Button type="button" variant="ghost" size="icon" disabled={busy || year >= 9999} aria-label="下一年" onClick={() => navigate(() => showMonth(`${String(year + 1).padStart(4, '0')}-${month.slice(5)}`))}><ChevronRight size={18} /></Button>
        </div>
        <nav className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6" aria-label={`${year}年的月份`}>{Array.from({ length: 12 }, (_, i) => {
          const value = `${String(year).padStart(4, '0')}-${String(i + 1).padStart(2, '0')}`;
          const entries = issues.filter(issue => issue.month === value);
          const guards = entries.filter(issue => issue.kind === 'guard').length, lotteries = entries.filter(issue => issue.kind === 'lottery').length;
          const published = entries.filter(issue => issue.published).length, drafts = entries.length - published;
          return <button key={value} type="button" disabled={busy} aria-current={value === month ? 'date' : undefined} onClick={() => navigate(() => showMonth(value))}
            className={`min-h-32 rounded-xl border p-3 text-left focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50 ${value === month ? 'border-primary bg-secondary' : 'border-border bg-card/60 hover:bg-accent'}`}>
            <span className={`mb-2 block text-base font-semibold ${value === month ? 'text-primary' : 'text-foreground'}`}>{i + 1} 月</span>
            {entries.length ? <><span className="block text-xs leading-6">航海回馈 <span className="font-medium tabular-nums">{guards}</span></span><span className="block text-xs leading-6">抽奖活动 <span className="font-medium tabular-nums">{lotteries}</span></span>
              <span className="mt-2 flex flex-wrap gap-x-2 text-xs leading-5">{!!published && <span className="text-primary">已发布 {published}</span>}{!!drafts && <span className="text-muted-foreground">草稿 {drafts}</span>}</span></>
              : <span className="block py-4 text-xs text-muted-foreground">暂无回馈或活动</span>}
          </button>;
        })}</nav>
      </section>
      <section className="min-w-0 space-y-5">
        <header className="space-y-4"><h2 className="text-2xl font-semibold">{monthLabel(month)}</h2><nav className="flex gap-6 border-b border-border" aria-label="礼物类别">{(['guard', 'lottery'] as const).map(category => <button type="button" key={category} disabled={busy} aria-current={kind === category ? 'page' : undefined} onClick={() => { if (category !== kind) navigate(() => showMonth(month, category)); }} className={`min-h-11 border-b-2 py-3 text-sm focus-visible:outline-2 focus-visible:outline-ring ${kind === category ? 'border-primary font-medium text-primary' : 'border-transparent text-muted-foreground hover:text-primary'}`}>{catalogLabel(category)}<span className="ml-2 text-xs tabular-nums">{monthIssues.filter(issue => issue.kind === category).length}</span></button>)}</nav></header>
        {loading ? <p role="status" className="py-10 text-sm text-muted-foreground">正在加载…</p> : !draft ? <>
          <div className="flex flex-wrap items-center justify-between gap-3"><p className="max-w-prose text-sm text-muted-foreground">{kind === 'guard' ? '按舰长、提督、总督展示这个月的礼物。' : '每次抽奖单独创建，奖品与领取说明放在活动里。'}</p><Button type="button" className="shrink-0" onClick={() => open(freshVoyageCatalog(month, kind))}><Plus size={16} />新建{catalogLabel(kind)}</Button></div>
          {visible.length ? <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,240px),300px))] justify-start gap-5">{visible.map(issue => { const cover = issue.images?.[0] || issue.gifts.flatMap(gift => gift.images)[0]; return <button key={issue.id} type="button" onClick={() => open(issue)} className="fabric-card flex min-w-0 flex-col overflow-hidden rounded-2xl border border-border text-left hover:border-primary focus-visible:outline-2 focus-visible:outline-ring">
            {cover && <img src={cover} alt="" loading="lazy" className="aspect-[3/4] w-full bg-background/50 object-cover" />}
            <div className="flex w-full flex-1 flex-col gap-3 p-5"><div className="flex flex-wrap items-center gap-2"><h3 className="min-w-0 flex-1 break-words text-lg font-semibold">{issue.title}</h3><span className="text-xs text-muted-foreground">{issue.published ? '已发布' : '草稿'}</span></div>{issue.note && <p className="line-clamp-2 break-words text-sm leading-7 text-muted-foreground">{issue.note}</p>}<div className="mt-auto flex items-center justify-between pt-3 text-sm"><span className="text-muted-foreground">{issue.gifts.length} 件{kind === 'guard' ? '礼物' : '奖品'}</span><span className="text-primary">编辑</span></div></div>
          </button>; })}</div> : <div className="rounded-xl border border-dashed border-border px-6 py-14 text-center"><h3 className="font-medium">这个月还没有{catalogLabel(kind)}</h3><p className="mt-2 text-sm text-muted-foreground">{kind === 'guard' ? '新建回馈卡后，在对应档位下添加礼物。' : '新建活动卡后，填写活动名称并添加奖品。'}</p></div>}
        </> : <>
          <div className="flex flex-wrap items-center justify-between gap-2"><Button type="button" variant="ghost" className="-ml-3" disabled={busy} onClick={() => navigate(() => showMonth(month))}><ArrowLeft size={16} />返回本月</Button><div className="flex flex-wrap items-center gap-3"><span className="text-xs text-muted-foreground">{draft.id ? issues.find(issue => issue.id === draft.id)?.published ? '已发布' : '草稿' : '尚未保存'}</span><Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => { setMoveMonth(draft.month || month); openDialog({ type: 'month' }); }}>修改月份{draft.month !== month ? ` · ${draft.month}` : ''}</Button>{!!draft.id && <Button type="button" variant="ghost" size="sm" disabled={busy} aria-label={`删除${draft.title}`} className="text-muted-foreground hover:text-destructive" onClick={() => openDialog({ type: 'delete' })}><Trash2 size={15} />删除</Button>}</div></div>
          <p className="text-sm text-muted-foreground">点击“编辑{kind === 'guard' ? '礼物' : '奖品'}”修改名称、图片和发货信息，完成后在底部保存。</p>
          <VoyageCatalogCanvas draft={draft} busy={busy} activeField={activeField} onActiveField={setActiveField} onPatch={patch} onPatchGift={patchGift} onAddGift={addGift} onRemoveGift={index => openDialog({ type: 'removeGift', index })} onImages={index => openDialog({ type: 'images', index })} />
          <footer className="sticky bottom-0 z-10 space-y-3 border-t border-border bg-background py-4 pb-[max(1rem,env(safe-area-inset-bottom))]">
            {error && <p role="alert" className="max-h-28 overflow-auto whitespace-pre-wrap text-sm text-destructive">{error}</p>}
            <div className="flex flex-wrap items-center justify-between gap-3"><label className="flex min-h-10 cursor-pointer items-center gap-2.5 text-sm"><input type="checkbox" disabled={busy} className="h-4 w-4 accent-[var(--primary)]" checked={draft.published} onChange={event => patch({ published: event.target.checked })} />在前台展示</label><div className="flex flex-wrap items-center gap-3"><span role="status" className="text-xs text-muted-foreground">{busy ? '正在处理…' : dirty ? '修改尚未保存' : notice || '已保存'}</span><Button type="button" onClick={() => { void save(); }} disabled={busy}>{busy ? '处理中…' : draft.published ? '保存并发布' : '保存草稿'}</Button></div></div>
          </footer>
        </>}
        {!draft && error && <div role="alert" className="whitespace-pre-wrap text-sm text-destructive">{error}<Button type="button" variant="ghost" disabled={loading} onClick={() => { void load(); }}>重新加载</Button></div>}
        {!draft && notice && <p role="status" className="text-sm text-primary">{notice}</p>}
      </section>
    </div>
    <Dialog open={!!pendingNavigation} onOpenChange={open => { if (!open) setPendingNavigation(null); }}><DialogContent><DialogHeader><DialogTitle>放弃未保存的修改？</DialogTitle><DialogDescription>当前卡片的修改尚未保存，切换后会丢失。</DialogDescription></DialogHeader><DialogFooter><Button variant="outline" onClick={() => setPendingNavigation(null)}>继续编辑</Button><Button onClick={() => { pendingNavigation?.(); setPendingNavigation(null); }}>放弃并切换</Button></DialogFooter></DialogContent></Dialog>
    <Dialog open={!!dialog} onOpenChange={open => { if (!open && !busyRef.current) setDialog(null); }}><DialogContent className={dialog?.type === 'images' ? 'max-h-[85svh] overflow-y-auto sm:max-w-2xl' : ''} onCloseAutoFocus={event => { event.preventDefault(); dialogTrigger.current?.focus(); }}>
      <DialogHeader><DialogTitle>{dialog?.type === 'images' ? `编辑${imageTarget}图片` : dialog?.type === 'month' ? '修改所属月份' : dialog?.type === 'removeGift' ? '移除这件礼物或奖品？' : `删除“${draft?.title}”？`}</DialogTitle><DialogDescription>{dialog?.type === 'images' ? '添加或移除图片只影响此处，其他礼物和海报不变。保存卡片后观众才能看到。' : dialog?.type === 'month' ? '保存卡片后，它会出现在新的月份下。' : dialog?.type === 'removeGift' ? '保存卡片后，观众将不再看到这件礼物或奖品。' : '这张卡片和其中的礼物展示将被删除。'}</DialogDescription></DialogHeader>
      {draft && dialog?.type === 'images' && <div className="space-y-5"><VoyageImageEditor images={selectedImages} limit={imageIndex === null ? 24 : 6} label={imageLabel} disabled={busy} onChange={images => imageIndex === null ? patch({ images }) : patchGift(imageIndex, { images })} onUpload={files => { void upload(imageIndex, files); }} /><VoyageImageLibrary images={collectVoyageImages(draft)} selectedImages={selectedImages} limit={imageIndex === null ? 24 : 6} disabled={busy} onSelect={url => { try { patch(reuseVoyageImage(draft, imageIndex, url)); } catch (error) { setError(error instanceof Error ? error.message : '无法选用图片'); } }} /></div>}
      {dialog?.type === 'month' && <div className="space-y-2"><label htmlFor="voyage-month" className="text-sm font-medium">所属月份</label><Input id="voyage-month" type="month" value={moveMonth} disabled={busy} onChange={event => setMoveMonth(event.target.value)} /></div>}
      {error && <p role="alert" className="whitespace-pre-wrap text-sm text-destructive">{error}</p>}
      <DialogFooter>{dialog?.type === 'delete' || dialog?.type === 'removeGift' ? <><Button variant="outline" disabled={busy} onClick={() => setDialog(null)}>取消</Button><Button variant="destructive" disabled={busy} onClick={() => { if (dialog.type === 'delete') void remove(); else if (draft) { patch({ gifts: draft.gifts.filter((_, i) => i !== dialog.index) }); setActiveField(null); setDialog(null); } }}>{dialog.type === 'delete' ? '删除卡片' : '移除'}</Button></> : <Button disabled={busy || (dialog?.type === 'month' && !validMonth(moveMonth))} onClick={() => { if (dialog?.type === 'month') patch({ month: moveMonth }); setDialog(null); }}>完成</Button>}</DialogFooter>
    </DialogContent></Dialog>
  </div>;
}
