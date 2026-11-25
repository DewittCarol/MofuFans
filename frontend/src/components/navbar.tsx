'use client';
import {useSiteTheme} from '@/lib/site-theme';

import { useAdminSession, useVoyageAccess } from '@/lib/admin-session';
import Link from 'next/link';
import { usePathname } from 'next/navigation';
import { LayoutDashboard, LogOut, UserRound } from 'lucide-react';
import { useAccountAuth } from '@/lib/account-auth';
import accountEntry from './account-entry.module.css';

const NAV_ROUTES = [
  { label: '提问', path: '/ask' },
  { label: '话题', path: '/tags' },
  { label: '彩虹屁', path: '/rainbow' },
  { label: '搜索', path: '/search' },
];


export function Navbar() {
 const {theme}=useSiteTheme();
 const MODULE_ROUTES=[{label:theme.modules.ask.label,path:'/ask'},{label:theme.modules.voyage.label,path:'/voyage'},{label:'电子礼物',path:'/gifts'},{label:theme.modules.songs.label,path:'/songs'},{label:theme.modules.gallery.label,path:'/gallery'}];
  const pathname = usePathname();
  const isAdmin = useAdminSession();
  const canSeeVoyage = useVoyageAccess();
  const { user, loading, accountsEnabled, openAccountDialog, logout } = useAccountAuth();
  const isHome = pathname === '/';
  const isQuestion = NAV_ROUTES.some(({path}) => pathname === path || pathname.startsWith(`${path}/`));
  const accountName = user?.bilibili_name || (user ? `UID ${user.bilibili_uid}` : '');

  const isActive = (path: string) => {
    if (path === '/') return pathname === '/';
    return pathname === path || pathname.startsWith(`${path}/`);
  };

  return (
    <header className="navbar fabric-nav min-h-14 px-2 py-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-2 z-[999]">
        <Link href="/" aria-label="返回首页" title="返回首页" className="flex shrink-0 items-center rounded text-primary-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-current">
          {theme.brand.logo&&<img src={theme.brand.logo} alt={theme.brand.streamer_name} className="site-brand-image mr-2.5 h-10 w-10 object-contain"/>}
          {isHome && <span className="text-sm font-medium">{theme.brand.site_name}</span>}
        </Link>
        {!isHome && <nav aria-label="模块导航" className="order-3 flex w-full flex-wrap items-center gap-1 sm:order-none sm:w-auto sm:min-w-0 sm:flex-1">{MODULE_ROUTES.filter(route => route.path !== '/voyage' || canSeeVoyage).map(({ label, path }) => (
          <Link
            key={path}
            href={path}
            aria-current={(path === '/ask' ? isQuestion : isActive(path)) ? 'page' : undefined}
            className={`shrink-0 whitespace-nowrap border border-transparent px-2 py-1.5 rounded text-sm text-primary-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-current sm:px-3 ${
              (path === '/ask' ? isQuestion : isActive(path))
                ? 'bg-white/20 !border-dashed !border-white/30 font-medium'
                : 'hover:bg-white/10'
            }`}
          >
            {label}
          </Link>
        ))}</nav>}
      <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-2">
        {isAdmin === true && <a href="/admin/dashboard/" className="flex h-9 shrink-0 items-center gap-1.5 whitespace-nowrap rounded border border-white/25 px-2 text-xs text-primary-foreground hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white sm:text-sm" title="管理后台"><LayoutDashboard className="h-4 w-4" aria-hidden="true" /><span>管理后台</span></a>}
      {accountsEnabled && <div className="flex min-w-0 items-center gap-2">
        {!loading && user ? (
          <>
            <Link href="/account/" aria-current={isActive('/account') ? 'page' : undefined} aria-label={`${accountName}，进入主页`} className={`${accountEntry.entry} text-primary-foreground hover:bg-white/10`} title={`${accountName} · 进入主页`}>
              <div aria-hidden="true" className="h-8 w-8 shrink-0 rounded-full bg-cover bg-center" style={{ backgroundImage: `url(${user.bilibili_avatar})` }} />
              <span className={accountEntry.identity}>
                <span className={accountEntry.name}>{accountName}</span>
                <span aria-hidden="true" className={accountEntry.hint}>进入主页</span>
              </span>
            </Link>
            <button type="button" onClick={logout} className="flex h-11 w-11 shrink-0 items-center justify-center rounded text-primary-foreground hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-current" title="退出登录" aria-label="退出登录"><LogOut className="h-4 w-4" aria-hidden="true" /></button>
          </>
        ) : (
          <button type="button" disabled={loading} onClick={() => openAccountDialog('login')} className="flex h-9 shrink-0 items-center gap-1.5 rounded px-2 text-sm text-primary-foreground hover:bg-white/10 disabled:opacity-60 sm:px-3" title="登录 / 注册"><UserRound className="h-4 w-4" /><span className="hidden whitespace-nowrap sm:inline">登录 / 注册</span></button>
        )}
      </div>}
      </div>
      {isQuestion && <nav aria-label="提问箱导航" className="order-4 flex w-full flex-wrap gap-1 border-t border-current/10 pt-2 text-primary-foreground">{NAV_ROUTES.map(({label,path}) => <Link key={path} href={path} aria-current={isActive(path) ? 'page' : undefined} className={`rounded px-3 py-1.5 text-sm hover:bg-white/10 focus-visible:outline-2 focus-visible:outline-current ${isActive(path) ? 'bg-white/20 font-medium' : ''}`}>{label}</Link>)}</nav>}
    </header>
  );
}
