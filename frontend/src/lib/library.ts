import { useEffect, useState } from 'react';

export interface SongTag { id:number; kind:'language'|'genre'; name:string }
export interface Song { id: number; title: string; artist: string; performed: boolean; videos: string[]; tags:SongTag[] }
export interface GalleryWork {
  source_url?:string; credit?:string;
  id: number; title: string; source: 'streamer' | 'fan'; status: 'pending' | 'published' | 'rejected';
  version: number; created_at: string; images: { id: number; url: string; width: number; height: number }[];
}
export interface LibraryPage<T> { items: T[]; total: number; page: number; page_size: number; counts?: Record<string, number>; last_updated_at?: string | null }
export const videoURL = (video: string) => { const [bv,query]=video.split('?');const p=new URLSearchParams(query||'').get('p');return `https://www.bilibili.com/video/${encodeURIComponent(bv)}/${p&&/^[1-9][0-9]{0,3}$/.test(p)?`?p=${p}`:''}`; };

export async function libraryRequest<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const form = body instanceof FormData;
  const response = await fetch(path, { method, credentials: 'include', cache: 'no-store', signal,
    ...(body !== undefined ? { body: form ? body : JSON.stringify(body), ...(form ? {} : { headers: { 'Content-Type': 'application/json' } }) } : {}) });
  const result = await response.json().catch(() => { throw new Error('服务暂时不可用，请稍后重试'); });
  if (!response.ok || result.code !== 200) throw new Error(result.message || '操作失败，请重试');
  return result.data;
}
export function useLibrary<T>(path: string) {
  const [data, setData] = useState<LibraryPage<T>>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const abort = new AbortController();
    setLoading(true); setError('');
    libraryRequest<LibraryPage<T>>(path, 'GET', undefined, abort.signal)
      .then(value => { if (!abort.signal.aborted) setData(value); })
      .catch(error => { if (!abort.signal.aborted) setError(error.message); })
      .finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [path, revision]);
  return { data, loading, error, reload: () => setRevision(value => value + 1) };
}
