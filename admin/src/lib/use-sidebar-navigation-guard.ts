'use client';

import {useEffect, useRef, type MouseEvent as ReactMouseEvent} from 'react';
import {useRouter} from 'next/navigation';
import {guardNavigation, isPlainNavigation, type LeaveDecision} from '@/lib/guarded-navigation';

function useNavigate() {
  const router = useRouter();
  return (url: URL) => {
    if (url.origin === window.location.origin && /^\/admin(?:\/|$)/.test(url.pathname)) {
      router.push(url.pathname.replace(/^\/admin(?=\/|$)/, '') + url.search + url.hash);
    } else window.location.assign(url.href);
  };
}

export function useGuardedLink() {
  const navigate = useNavigate();
  const live = useRef(true);
  useEffect(() => { live.current = true; return () => { live.current = false; }; }, []);
  return (event: ReactMouseEvent<HTMLAnchorElement>, canLeave: () => LeaveDecision) => {
    if (!isPlainNavigation(event) || event.currentTarget.target === '_blank' || event.currentTarget.hasAttribute('download')) return;
    const url = new URL(event.currentTarget.href);
    guardNavigation(event, canLeave, () => { if (live.current) navigate(url); });
  };
}

// Capture the click before Next handles it, then continue only after explicit approval.
export function useSidebarNavigationGuard(canLeave: (url: URL) => LeaveDecision, scope: 'sidebar' | 'all' = 'sidebar') {
  const navigate = useNavigate();
  const latest = useRef({canLeave, navigate});
  latest.current = {canLeave, navigate};
  useEffect(() => {
    let live = true;
    const guard = (event: MouseEvent) => {
      if (!isPlainNavigation(event)) return;
      const link = event.target instanceof Element ? event.target.closest<HTMLAnchorElement>('a[href]') : null;
      if (!link || link.target === '_blank' || link.hasAttribute('download') || (scope === 'sidebar' && !link.closest('nav[aria-label="管理菜单"]'))) return;
      const url = new URL(link.href);
      if (url.origin === window.location.origin && url.pathname.replace(/\/$/, '') === window.location.pathname.replace(/\/$/, '') && url.search === window.location.search) return;
      guardNavigation(event, () => latest.current.canLeave(url), () => { if (live) latest.current.navigate(url); });
    };
    document.addEventListener('click', guard, true);
    return () => { live = false; document.removeEventListener('click', guard, true); };
  }, [scope]);
}
