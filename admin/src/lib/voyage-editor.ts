import type { VoyageIssue, VoyageCatalogKind } from './voyage';

export type VoyageEditorValidation = { index: number | null; field: string; message: string };
export const catalogLabel = (kind: VoyageIssue['kind']) => kind === 'lottery' ? '抽奖活动' : '大航海回馈';
export const monthLabel = (month: string) => `${month.slice(0, 4)}年${Number(month.slice(5))}月`;
export const validMonth = (month: string) => /^(?!0000)\d{4}-(0[1-9]|1[0-2])$/.test(month);

export function freshVoyageCatalog(month: string, kind: VoyageCatalogKind): VoyageIssue {
  return { id: 0, kind, month, title: kind === 'guard' ? `${monthLabel(month)}大航海回馈` : '', note: '', shipping_note: '', images: [], asset_images: [], gifts: [], published: false, version: 0 };
}
export function collectVoyageImages(issue: VoyageIssue): string[] {
  return [...new Set([...(issue.asset_images || []), ...(issue.images || []), ...issue.gifts.flatMap(gift => gift.images)])];
}
export function createVoyageEditorDraft(issue: VoyageIssue): VoyageIssue {
  const draft = structuredClone(issue);
  draft.images = draft.images || [];
  draft.asset_images = collectVoyageImages(draft);
  // Each displayed tier owns its own gift, including independent image arrays.
  if (draft.kind === 'guard') draft.gifts = draft.gifts.flatMap(gift => {
    const tiers = [...new Set(gift.tiers.length ? gift.tiers : ['captain' as const])];
    return tiers.map(tier => ({ ...structuredClone(gift), tiers: [tier] }));
  });
  return draft;
}
export function reuseVoyageImage(draft: VoyageIssue, index: number | null, url: string): VoyageIssue {
  const images = index === null ? draft.images || [] : draft.gifts[index]?.images;
  if (!images || images.includes(url)) return draft;
  const limit = index === null ? 24 : 6;
  if (images.length >= limit) throw new Error(index === null ? '海报最多 24 张图片，请先移除一张。' : '每件礼物最多 6 张图片，请先移除一张。');
  if (index === null) return { ...draft, images: [...images, url] };
  return { ...draft, gifts: draft.gifts.map((gift, i) => i === index ? { ...gift, images: [...images, url] } : gift) };
}
// Retain the card's materials even when their last visible placement is removed.
export function patchVoyageDraft(draft: VoyageIssue, values: Partial<VoyageIssue>): VoyageIssue {
  const next = { ...draft, ...values };
  return { ...next, asset_images: [...new Set([...collectVoyageImages(draft), ...collectVoyageImages(next)])] };
}
// Reveal the editing target on the content card before focusing it.
export function validateVoyageIssue(draft: VoyageIssue): VoyageEditorValidation | null {
  if (!validMonth(draft.month || '')) return { index: null, field: 'voyage-month', message: '请选择所属月份。' };
  if (draft.kind !== 'guard' && draft.kind !== 'lottery') return { index: null, field: 'voyage-title', message: '请从月份下创建大航海回馈或抽奖活动。' };
  if (!draft.title.trim()) return { index: null, field: 'voyage-title', message: draft.kind === 'guard' ? '请填写回馈名称。' : '请填写活动名称。' };
  if (draft.published && !draft.gifts.length) return { index: null, field: 'add-voyage-gift', message: '请至少添加一件礼物或奖品再发布。' };
  for (const [index, gift] of draft.gifts.entries()) {
    if (!gift.name.trim()) return { index, field: `gift-name-${index}`, message: '请填写礼物或奖品名称。' };
    if (draft.kind === 'guard' && gift.tiers.length !== 1) return { index, field: `gift-name-${index}`, message: '请重新打开卡片，在对应档位下编辑礼物。' };
    if (draft.kind === 'lottery' && gift.tiers.length) return { index, field: `gift-name-${index}`, message: '抽奖活动不关联大航海档位，请重新打开这张活动卡。' };
  }
  return null;
}
