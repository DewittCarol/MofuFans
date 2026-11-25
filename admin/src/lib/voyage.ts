export const voyageTiers = [
  { id: 'captain', name: '舰长', icon: '/voyage-tiers/captain.png', background: 'rgba(76, 125, 255, 0.6)', border: '#58A1F8', foreground: '#1C285C' },
  { id: 'admiral', name: '提督', icon: '/voyage-tiers/admiral.png', background: 'rgba(167, 115, 241, 0.6)', border: '#D47AFF', foreground: '#382154' },
  { id: 'governor', name: '总督', icon: '/voyage-tiers/governor.png', background: 'rgba(236, 79, 110, 0.6)', border: '#F18087', foreground: '#53172A' },
] as const;
export type VoyageTier = typeof voyageTiers[number]['id'];
export interface VoyageGift {
  section?: 'guard' | 'other'; event?: string; audience?: string; shipping_note?: string;
  name: string; tiers: VoyageTier[];
  description: string; images: string[];
}
export type VoyageCatalogKind = 'guard' | 'lottery';
export interface VoyageIssue {
  kind?: VoyageCatalogKind | 'batch';
  batch?: boolean;
  month?: string; images?: string[]; asset_images?: string[]; shipping_note?: string;
  id: number; title: string; note: string; published: boolean; version: number; gifts: VoyageGift[];
}
export async function voyageRequest<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, { method, credentials: 'include', cache: 'no-store', signal,
    ...(body ? { headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) } : {}) });
  const result = await response.json();
  if (!response.ok || result.code !== 200) throw new Error(result.message || '加载失败，请重试');
  return result.data;
}
