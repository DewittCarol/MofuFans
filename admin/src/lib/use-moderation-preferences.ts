'use client';

import { useEffect, useState } from 'react';
import { useAuth } from './auth';
import {
  defaultPreferences, imageIdentity, imageReviewKey, moderationStoragePrefix,
  readPreferences, readReviewedImages, type ModerationPreferences,
} from './moderation-preferences';

export function useModerationPreferences({ allowAdminInspection = false }: { allowAdminInspection?: boolean } = {}) {
  const { user } = useAuth();
  const inspectionMode = allowAdminInspection && user?.username === 'admin';
  const prefix = user ? moderationStoragePrefix(user) : '';
  const [loadedPrefix, setLoadedPrefix] = useState('');
  const [preferences, setPreferences] = useState(defaultPreferences);
  const [seen, setSeen] = useState<Set<string>>(new Set());
  const [storageWarning, setStorageWarning] = useState('');
  const ready = !!prefix && loadedPrefix === prefix;

  useEffect(() => {
    if (!prefix) return;
    const read = () => {
      setStorageWarning('');
      try {
        setPreferences(readPreferences(localStorage.getItem(`${prefix}preferences`)));
        setSeen(inspectionMode ? new Set() : readReviewedImages(localStorage, prefix));
      } catch {
        setPreferences(defaultPreferences);
        setSeen(new Set());
        setStorageWarning(inspectionMode ? '浏览器无法保存显示设置，本次查看仍可使用。' : '浏览器无法保存查看记录，本次查看仍可使用，刷新后图片会重新模糊。');
      }
      setLoadedPrefix(prefix);
    };
    read();
    const sync = (event: StorageEvent) => {
      if (event.key === null || event.key.startsWith(prefix)) read();
    };
    window.addEventListener('storage', sync);
    return () => window.removeEventListener('storage', sync);
  }, [prefix, inspectionMode]);

  const savePreferences = (patch: Partial<ModerationPreferences>) => {
    if (!ready) return;
    const next = { ...preferences, ...patch };
    setPreferences(next);
    try { localStorage.setItem(`${prefix}preferences`, JSON.stringify(next)); }
    catch { setStorageWarning('浏览器无法保存设置，本次操作仍然有效。'); }
  };

  const reveal = (src: string) => {
    if (inspectionMode || !ready) return;
    const identity = imageIdentity(src, window.location.origin);
    setSeen(current => new Set(current).add(identity));
    // Separate keys avoid losing another tab's reviews to a read/modify/write race.
    try { localStorage.setItem(imageReviewKey(prefix, identity), '1'); }
    catch { setStorageWarning('浏览器无法保存查看记录，刷新后这张图片会重新模糊。'); }
  };

  const isProtected = (src: string) => {
    if (inspectionMode) return false;
    // Safe default during hydration, loading and an account change.
    if (!ready) return true;
    return preferences.blurEnabled && !seen.has(imageIdentity(src, window.location.origin));
  };

  return {
    ready, inspectionMode,
    preferences: inspectionMode ? { ...(ready ? preferences : defaultPreferences), blurEnabled: false } : ready ? preferences : defaultPreferences,
    savePreferences, reveal, isProtected, storageWarning,
  };
}
