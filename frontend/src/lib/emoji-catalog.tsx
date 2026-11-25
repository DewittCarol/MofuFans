'use client';

import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { emptyFooter, readFooter, type Footer } from '../../../shared/footer';

export type EmojiKind = 'input' | 'reaction';
export interface EmojiCollection { id: number; name: string; }
export interface EmojiAsset {
  id: number; kind: EmojiKind; name: string; value: string; url: string;
  is_deleted: boolean; builtin: boolean;
  collection_id: number; sort_order: number;
}
const EmojiCatalogContext = createContext<{
  emojis: EmojiAsset[]; collections: EmojiCollection[]; loading: boolean; error: string; refresh: () => Promise<void>;
  allowRepeatedReactions: boolean;
  accountsEnabled: boolean;
  requireVerifiedUserToPost: boolean;
  background: { url: string; opacity: number };
  footer: Footer;
} | null>(null);

export function EmojiCatalogProvider({ children }: { children: React.ReactNode }) {
  const [emojis, setEmojis] = useState<EmojiAsset[]>([]);
  const [collections, setCollections] = useState<EmojiCollection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [allowRepeatedReactions, setAllowRepeatedReactions] = useState(false);
  const [accountsEnabled, setAccountsEnabled] = useState(false);
  const [requireVerifiedUserToPost, setRequireVerifiedUserToPost] = useState(false);
  const [background, setBackground] = useState({ url: '', opacity: 25 });
  const [footer, setFooter] = useState<Footer>(emptyFooter);
  const request = useRef<AbortController | null>(null);
  const refresh = useCallback(async () => {
    request.current?.abort();
    const controller = new AbortController(); request.current = controller;
    const policy = fetch('/api/config', { credentials: 'include', cache: 'no-store', signal: controller.signal })
      .then(response => response.json()).then(result => {
        if (!controller.signal.aborted && result.code === 200) {
          setAllowRepeatedReactions(!!result.data.allow_repeated_reactions);
          const nextFooter = readFooter(result.data);
          setFooter(current => JSON.stringify(current) === JSON.stringify(nextFooter) ? current : nextFooter);
          setAccountsEnabled(!result.data.disable_member_accounts);
          setRequireVerifiedUserToPost(!result.data.disable_member_accounts && !!result.data.require_verified_user_to_post);
          const url = typeof result.data.background_url === 'string' && /^\/api\/background-assets\/[a-f0-9]{64}\.(png|jpg|webp)$/.test(result.data.background_url) ? result.data.background_url : '';
          const opacity = Number.isFinite(result.data.background_opacity) ? Math.max(0, Math.min(100, result.data.background_opacity)) : 25;
          setBackground(current => current.url === url && current.opacity === opacity ? current : { url, opacity });
        }
      }).catch(() => {});
    try {
      const response = await fetch('/api/emoji-catalog', { credentials: 'include', cache: 'no-store', signal: controller.signal });
      if (!response.ok) throw new Error('load');
      const result = await response.json();
      if (result.code !== 200 || !Array.isArray(result.data?.emojis) || !Array.isArray(result.data?.collections)) throw new Error('load');
      if (controller.signal.aborted) return;
      setEmojis(current => JSON.stringify(current) === JSON.stringify(result.data.emojis) ? current : result.data.emojis);
      setCollections(current => JSON.stringify(current) === JSON.stringify(result.data.collections) ? current : result.data.collections);
      setError('');
    } catch {
      if (!controller.signal.aborted) setError('表情加载失败，请重试');
    } finally {
      await policy;
      if (!controller.signal.aborted) setLoading(false);
    }
  }, []);
  useEffect(() => {
    void refresh();
    const update = () => { if (!document.hidden) void refresh(); };
    window.addEventListener('focus', update);
    document.addEventListener('visibilitychange', update);
    const timer = window.setInterval(update, 15000);
    return () => {
      request.current?.abort(); clearInterval(timer);
      window.removeEventListener('focus', update);
      document.removeEventListener('visibilitychange', update);
    };
  }, [refresh]);
  return <EmojiCatalogContext.Provider value={{ emojis, collections, loading, error, refresh, allowRepeatedReactions, background, footer, accountsEnabled, requireVerifiedUserToPost }}>{children}</EmojiCatalogContext.Provider>;
}

export function useEmojiCatalog() {
  const context = useContext(EmojiCatalogContext);
  if (!context) throw new Error('EmojiCatalogProvider is required');
  return context;
}
