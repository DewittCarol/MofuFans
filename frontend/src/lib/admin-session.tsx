'use client';

import { createContext, useContext, useEffect, useState } from 'react';

const AdminSession = createContext<boolean | null>(null);
const VoyageVisibility = createContext<boolean | null>(null);
const PetEntryExpanded = createContext<boolean | null>(null);

export function AdminSessionProvider({ children }: { children: React.ReactNode }) {
  const [isAdmin, setIsAdmin] = useState<boolean | null>(null);
  useEffect(() => {
    let pending: AbortController | undefined;
    const check = async () => {
      pending?.abort();
      const controller = new AbortController();
      pending = controller;
      try {
        const response = await fetch('/api/info', { credentials: 'include', cache: 'no-store', signal: controller.signal });
        const result = await response.json();
        if (!controller.signal.aborted) setIsAdmin(response.ok && result.code === 200 && Number.isInteger(result.data?.id));
      } catch { if (!controller.signal.aborted) setIsAdmin(false); }
    };
    const refresh = () => { if (!document.hidden) void check(); };
    void check();
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    const timer = window.setInterval(refresh, 30000);
    return () => {
      pending?.abort();
      window.clearInterval(timer);
      window.removeEventListener('focus', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, []);
  const [publicVoyage, setPublicVoyage] = useState<boolean | null>(null);
  const [petExpanded, setPetExpanded] = useState<boolean | null>(null);
  useEffect(() => {
    let pending: AbortController | undefined;
    const check = async () => {
      pending?.abort();
      const controller = new AbortController(); pending = controller;
      try {
        const response = await fetch('/api/config', { credentials: 'include', cache: 'no-store', signal: controller.signal });
        const result = await response.json();
        if (!controller.signal.aborted) {
          setPublicVoyage(response.ok && result.code === 200 && result.data?.voyage_public_enabled === true);
          setPetExpanded(response.ok && result.code === 200 && result.data?.pet_default_expanded === true);
        }
      } catch { if (!controller.signal.aborted) { setPublicVoyage(false); setPetExpanded(false); } }
    };
    const refresh = () => { if (!document.hidden) void check(); };
    void check();
    window.addEventListener('focus', refresh);
    document.addEventListener('visibilitychange', refresh);
    const timer = window.setInterval(refresh, 30000);
    return () => { pending?.abort(); window.clearInterval(timer); window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh); };
  }, []);
  return <AdminSession.Provider value={isAdmin}><VoyageVisibility.Provider value={publicVoyage}><PetEntryExpanded.Provider value={petExpanded}>{children}</PetEntryExpanded.Provider></VoyageVisibility.Provider></AdminSession.Provider>;
}

export const useAdminSession = () => useContext(AdminSession);
export const usePetDefaultExpanded = () => useContext(PetEntryExpanded);
export function useVoyageAccess(): boolean | null {
  const admin = useContext(AdminSession), visible = useContext(VoyageVisibility);
  if (admin === true || visible === true) return true;
  return admin === null || visible === null ? null : false;
}
