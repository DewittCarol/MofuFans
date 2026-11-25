'use client';

import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react';
import { AccountUser, getAccountInfo, logoutAccount } from './api';
import type { AccountDialogMode } from './member-api';
import { AccountDialog } from '@/components/account-dialog';
import { useEmojiCatalog } from './emoji-catalog';

interface AccountAuthValue {
  user: AccountUser | null;
  loading: boolean;
  accountsEnabled: boolean;
  authRevision: number;
  setUser: (user: AccountUser | null) => void;
  openAccountDialog: (mode?: AccountDialogMode) => void;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}
const AccountAuthContext = createContext<AccountAuthValue | null>(null);
const accountEventKey = 'member-session-changed';

export function AccountAuthProvider({ children }: { children: React.ReactNode }) {
  const { accountsEnabled } = useEmojiCatalog();
  const [user, updateUser] = useState<AccountUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [authRevision, setRevision] = useState(0);
  const userRef = useRef<AccountUser | null>(null), requestID = useRef(0);
  const locallySignedOut = useRef(false), logoutPending = useRef(false), logoutUID = useRef('');
  const [logoutError, setLogoutError] = useState('');
  const [dialog, setDialog] = useState<{ open: boolean; mode: AccountDialogMode }>({ open: false, mode: 'login' });
  const applyUser = useCallback((next: AccountUser | null, force = false) => {
    const old = userRef.current;
    if (force || old?.bilibili_uid !== next?.bilibili_uid || old?.signature_verified !== next?.signature_verified || old?.updated_at !== next?.updated_at) setRevision(value => value + 1);
    userRef.current = next; updateUser(next);
  }, []);
  const announce = (kind: 'authenticated' | 'signed-out') => { try { localStorage.setItem(accountEventKey, JSON.stringify({ kind, nonce: `${Date.now()}:${Math.random()}` })); } catch { /* Login does not depend on local storage. */ } };
  const setUser = useCallback((next: AccountUser | null) => { requestID.current++; locallySignedOut.current = !next; setLogoutError(''); applyUser(next, true); announce(next ? 'authenticated' : 'signed-out'); }, [applyUser]);
  const refresh = useCallback(async () => {
    const id = ++requestID.current;
    if (locallySignedOut.current || logoutPending.current) { applyUser(null); setLoading(false); return; }
    try {
      const response = await getAccountInfo();
      if (id === requestID.current) applyUser(response.code === 200 ? response.data : null);
    } catch {
      if (id === requestID.current) applyUser(null);
    } finally { if (id === requestID.current) setLoading(false); }
  }, [applyUser]);

  useEffect(() => {
    if (!accountsEnabled) { requestID.current++; applyUser(null); setDialog(value => ({ ...value, open: false })); setLoading(false); return; }
    setLoading(true); void refresh();
    const sync = () => { void refresh(); };
    const recheck = () => { requestID.current++; applyUser(null, true); void refresh(); };
    const storage = (event: StorageEvent) => {
      if (event.key !== accountEventKey) return;
      requestID.current++; applyUser(null, true); setLogoutError('');
      let signedOut = false;
      try { signedOut = JSON.parse(event.newValue || 'null')?.kind === 'signed-out'; } catch { /* Older tabs send a timestamp. */ }
      locallySignedOut.current = signedOut;
      if (signedOut) { setLoading(false); setDialog(value => ({ ...value, open: false })); }
      else void refresh();
    };
    window.addEventListener('focus', sync); window.addEventListener('member-auth-refresh', recheck); window.addEventListener('storage', storage);
    return () => { requestID.current++; window.removeEventListener('focus', sync); window.removeEventListener('member-auth-refresh', recheck); window.removeEventListener('storage', storage); };
  }, [accountsEnabled, applyUser, refresh]);

  const openAccountDialog = useCallback((mode: AccountDialogMode = 'login') => {
    if (accountsEnabled) setDialog({ open: true, mode });
  }, [accountsEnabled]);
  const logout = useCallback(async () => {
    if (logoutPending.current) return;
    logoutUID.current = userRef.current?.bilibili_uid || logoutUID.current;
    logoutPending.current = true; locallySignedOut.current = true;
    requestID.current++; applyUser(null, true); setLoading(false); setLogoutError(''); setDialog(value => ({ ...value, open: false }));
    try {
      const response = await logoutAccount(logoutUID.current);
      if (response.code !== 200) { setLogoutError(response.message || '退出登录未完成，请重试'); return; }
      // Other tabs must clear only after the server has invalidated the session.
      announce('signed-out');
    } catch {
      setLogoutError('退出登录未完成，请检查网络后重试');
    } finally { logoutPending.current = false; requestID.current++; applyUser(null); }
  }, [applyUser]);

  return <AccountAuthContext.Provider value={{ user: accountsEnabled ? user : null, accountsEnabled, loading, authRevision, setUser, openAccountDialog, refresh, logout }}>
    {logoutError && <div role="alert" className="relative z-50 flex flex-wrap items-center justify-center gap-3 border-b border-border bg-card px-4 py-3 text-sm"><span>{logoutError}。本页个人信息已隐藏。</span><button type="button" className="underline underline-offset-4" onClick={() => void logout()}>重试退出</button></div>}
    {children}
    <AccountDialog open={accountsEnabled && dialog.open} initialMode={dialog.mode} currentUser={user}
      onClose={() => setDialog(value => ({ ...value, open: false }))}
      onAuthenticated={account => { setUser(account); setDialog(value => ({ ...value, open: false })); }} />
  </AccountAuthContext.Provider>;
}

export function useAccountAuth() {
  const value = useContext(AccountAuthContext);
  if (!value) throw new Error('useAccountAuth must be used within AccountAuthProvider');
  return value;
}
