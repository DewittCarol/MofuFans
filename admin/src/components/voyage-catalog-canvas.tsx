'use client';

import { useEffect, useRef } from 'react';
import { ImagePlus, Pencil, Plus, Trash2, Check } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { voyageTiers, type VoyageGift, type VoyageIssue, type VoyageTier } from '@/lib/voyage';
import { VoyageTierBadge } from '@/components/voyage-tier-badge';

function EditorField({ id, label, value, placeholder, limit, multiline = false, optional = false, stretch = false, focus, disabled, onChange }: {
  id: string; label: string; value: string; placeholder: string; limit: number; multiline?: boolean; optional?: boolean; stretch?: boolean;
  focus: boolean; disabled: boolean; onChange: (value: string) => void;
}) {
  const input = useRef<HTMLInputElement & HTMLTextAreaElement>(null);
  useEffect(() => { if (focus) input.current?.focus(); }, [focus]);
  return <div className={stretch ? "flex h-full min-w-0 flex-col gap-2" : "flex min-w-0 flex-col gap-2"}>
    <label htmlFor={id} className="text-sm font-medium">{label}{optional && <span className="ml-1 font-normal text-muted-foreground">（选填）</span>}</label>
    {multiline ? <textarea ref={input} id={id} rows={3} maxLength={limit} disabled={disabled} value={value} onChange={event => onChange(event.target.value)} className={`block min-h-24 w-full rounded-md border border-input bg-background px-3 py-2 text-sm leading-7 focus-visible:outline-2 focus-visible:outline-ring ${stretch ? "flex-1 resize-none" : "resize-y"}`} placeholder={placeholder} />
      : <Input ref={input} id={id} maxLength={limit} disabled={disabled} value={value} onChange={event => onChange(event.target.value)} placeholder={placeholder} />}
  </div>;
}

function ReadableDetail({ label, value }: { label: string; value: string }) {
  if (!value) return null;
  return <div className="min-w-0 space-y-1"><p className="text-xs font-medium text-muted-foreground">{label}</p><p className="max-w-prose whitespace-pre-wrap break-words text-sm leading-7 text-muted-foreground">{value}</p></div>;
}

export function VoyageCatalogCanvas({ draft, busy, activeField, onActiveField, onPatch, onPatchGift, onAddGift, onRemoveGift, onImages }: {
  draft: VoyageIssue; busy: boolean; activeField: string | null; onActiveField: (field: string | null) => void;
  onPatch: (values: Partial<VoyageIssue>) => void; onPatchGift: (index: number, values: Partial<VoyageGift>) => void;
  onAddGift: (tier?: VoyageTier) => void; onRemoveGift: (index: number) => void;
  onImages: (index: number | null) => void;
}) {
  const guard = draft.kind === 'guard';
  const itemLabel = guard ? '礼物' : '奖品';
  const infoEditing = ['voyage-title', 'voyage-note', 'voyage-shipping'].includes(activeField || '');
  const field = (id: string, label: string, value: string, placeholder: string, onChange: (value: string) => void, limit: number, multiline = false, optional = false, stretch = false) => <EditorField id={id} label={label} value={value} placeholder={placeholder} limit={limit} multiline={multiline} optional={optional} stretch={stretch} focus={activeField === id} disabled={busy} onChange={onChange} />;
  const editButton = (target: string, label: string, editing: boolean) => <Button id={`${target}-edit`} type="button" variant={editing ? 'outline' : 'ghost'} size="sm" className="shrink-0 text-primary" disabled={busy} aria-expanded={editing} aria-label={editing ? `完成${label}编辑` : `编辑${label}`} onClick={() => {
    onActiveField(editing ? null : target);
    if (editing) requestAnimationFrame(() => document.getElementById(`${target}-edit`)?.focus());
  }}>{editing ? <Check size={14} /> : <Pencil size={14} />}{editing ? '完成编辑' : `编辑${label}`}</Button>;
  const pictures = (images: string[], index: number | null, editing: boolean) => {
    if (!images.length && !editing) return null;
    return <div className="space-y-3">
      {!!images.length && <div className={index === null && editing ? "grid grid-cols-2 gap-3" : "flex flex-wrap gap-2"}>{images.map((src, imageIndex) => <img key={src} src={src} alt={`${index === null ? '海报' : draft.gifts[index].name} ${imageIndex + 1}`} loading="lazy" className={index === null && editing ? "aspect-[3/4] max-h-72 w-full rounded-md bg-background/70 object-contain" : "h-36 w-28 rounded-md bg-background/70 object-contain sm:h-44 sm:w-36"} />)}</div>}
      {index === null && editing && !images.length && <div className="flex min-h-52 items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">还没有添加图片</div>}
      {editing && <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onImages(index)}><ImagePlus size={15} />{images.length ? '修改图片' : index === null ? '添加海报' : `添加${itemLabel}图片`}</Button>}
    </div>;
  };
  const row = (gift: VoyageGift, index: number) => {
    const id = (name: string) => `gift-${name}-${index}`;
    const editing = ['name', 'description', 'audience', 'shipping'].some(name => activeField === id(name));
    return <div key={index} className="space-y-4 py-5 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
        <h4 id={editing ? undefined : id('name')} className="min-w-0 max-w-full break-words"><span className="mr-2 text-xs font-normal text-muted-foreground">{itemLabel}名称</span><span className="break-words text-lg font-semibold">{gift.name || `未命名${itemLabel}`}</span></h4>
        <div className="ml-auto flex shrink-0 flex-wrap items-center gap-2">
          <Button type="button" variant="ghost" size="sm" className="text-destructive hover:text-destructive" disabled={busy} aria-label={`移除${gift.name || itemLabel}`} onClick={() => onRemoveGift(index)}><Trash2 size={14} />移除{itemLabel}</Button>
          {editButton(id('name'), itemLabel, editing)}
        </div>
      </div>
      <div className={`grid gap-4 sm:gap-6 ${editing || gift.shipping_note ? '@min-[620px]:grid-cols-[minmax(0,1fr)_180px]' : ''}`}>
        <div className="min-w-0 space-y-4">
          {editing ? <>
            {field(id('name'), `${itemLabel}名称`, gift.name, `例如：小狗立牌`, value => onPatchGift(index, { name: value }), 80)}
            {field(id('description'), `${itemLabel}介绍`, gift.description, '例如：尺寸、材质、款式等', value => onPatchGift(index, { description: value }), 2000, true, true)}
            {!guard && field(id('audience'), '领取说明', gift.audience || '', '例如：中奖观众如何领取', value => onPatchGift(index, { audience: value }), 160, true, true)}
          </> : <>
            <ReadableDetail label={`${itemLabel}介绍`} value={gift.description} />
            {!guard && <ReadableDetail label="领取说明" value={gift.audience || ''} />}
          </>}
        </div>
        {(editing || gift.shipping_note) && <div className="min-w-0 border-t border-border/60 pt-3 @min-[620px]:border-0 @min-[620px]:pt-0">
          {editing ? field(id('shipping'), '发货情况', gift.shipping_note || '', '例如：预计 10 月寄出', value => onPatchGift(index, { shipping_note: value }), 1000, true, true, true)
            : <div className="space-y-1"><p className="text-xs font-medium text-muted-foreground">发货情况</p><p className="whitespace-pre-wrap break-words text-sm leading-7">{gift.shipping_note}</p></div>}
        </div>}
      </div>
      {pictures(gift.images, index, editing)}
    </div>;
  };
  const add = (tier?: VoyageTier) => <Button id={tier === 'captain' || !guard ? 'add-voyage-gift' : undefined} type="button" variant="outline" size="sm" disabled={busy || draft.gifts.length >= 100} onClick={() => onAddGift(tier)}><Plus size={15} />添加{itemLabel}</Button>;
  return <article className="fabric-card @container min-w-0 rounded-2xl border border-border px-5 py-6 sm:px-7 sm:py-7" aria-label={guard ? '编辑大航海回馈卡片' : '编辑抽奖活动卡片'}>
    <div className="space-y-5 pb-6">
      <header className="flex flex-wrap items-start justify-between gap-3"><h2 className="min-w-0 break-words text-xl font-semibold leading-snug sm:text-2xl">{draft.title || (guard ? '新建大航海回馈' : '新建抽奖活动')}</h2>{editButton('voyage-title', guard ? '回馈信息' : '活动信息', infoEditing)}</header>
      {infoEditing ? <div className="grid items-start gap-6 @min-[720px]:grid-cols-2 @min-[720px]:gap-8">
        <div className="min-w-0 space-y-4">
          {field('voyage-title', guard ? '回馈名称' : '活动名称', draft.title, guard ? '填写回馈名称' : '填写活动名称', title => onPatch({ title }), 80)}
          {field('voyage-note', guard ? '回馈介绍' : '活动介绍', draft.note, guard ? '介绍这个月准备的礼物' : '介绍活动内容与参与方式', note => onPatch({ note }), 3000, true, true)}
          {field('voyage-shipping', '统一发货说明', draft.shipping_note || '', guard ? '例如：本期礼物预计 10 月统一寄出' : '例如：活动结束后两周内寄出奖品', shipping_note => onPatch({ shipping_note }), 3000, true, true)}
        </div>
        <section className="min-w-0 space-y-3" aria-label={guard ? '回馈图片' : '活动图片'}>
          <h3 className="text-sm font-medium">{guard ? '回馈图片' : '活动图片'}<span className="ml-1 font-normal text-muted-foreground">（选填）</span></h3>
          {pictures(draft.images || [], null, true)}
        </section>
      </div> : <>
        <ReadableDetail label={guard ? '回馈介绍' : '活动介绍'} value={draft.note} />
        <ReadableDetail label="统一发货说明" value={draft.shipping_note || ''} />
        {pictures(draft.images || [], null, false)}
      </>}
    </div>
    {guard ? <div className="divide-y divide-border border-t border-border">{voyageTiers.map(tier => <section key={tier.id} className="grid gap-5 py-6 @min-[620px]:grid-cols-[110px_minmax(0,1fr)]">
      <div className="space-y-4"><h3><VoyageTierBadge tier={tier.id} /></h3>{add(tier.id)}</div>
      <div className="min-w-0">
        {draft.gifts.some(gift => gift.tiers.includes(tier.id)) ? <div className="divide-y divide-border/60">{draft.gifts.map((gift, index) => gift.tiers.includes(tier.id) ? row(gift, index) : null)}</div> : <p className="text-sm leading-7 text-muted-foreground">还没有{tier.name}礼物</p>}
      </div>
    </section>)}</div>
      : <section className="grid gap-5 border-t border-border pt-6 @min-[620px]:grid-cols-[110px_minmax(0,1fr)]">
        <div className="space-y-4"><h3 className="text-base font-semibold">活动奖品</h3>{add()}</div>
        {draft.gifts.length ? <div className="min-w-0 divide-y divide-border/60">{draft.gifts.map((gift, index) => row(gift, index))}</div> : <p className="text-sm text-muted-foreground">为这次抽奖添加奖品。</p>}
      </section>}
  </article>;
}
