'use client';
import { useSiteTheme } from '@/lib/site-theme';

import { useVoyageAccess } from '@/lib/admin-session';
import Link from 'next/link';
import { MailOpen, Sailboat, ArrowUpRight, Music2, Images, Gift } from 'lucide-react';
import styles from '@/app/portal.module.css';

export default function HomeMenu() {
 const {theme}=useSiteTheme();
  const canSeeVoyage = useVoyageAccess();
  return <main className={styles.home}>
    <header className={styles.intro}>
      {theme.brand.avatar&&<img src={theme.brand.avatar} width={72} height={72} alt={theme.brand.streamer_name} className={`${styles.avatar} site-brand-image object-contain`} />}
      <h1>{theme.brand.site_name}</h1>
      {theme.brand.bilibili_uid && <a href={`https://space.bilibili.com/${theme.brand.bilibili_uid}`} target="_blank" rel="noopener noreferrer" className="mt-3 inline-flex items-center gap-1.5 text-sm text-primary hover:underline">B 站主页 <ArrowUpRight size={14} aria-hidden="true" /></a>}

    </header>
    <nav aria-label="主菜单" className={styles.entries}>
      <Link href="/ask/" className={styles.entry} style={{color:theme.appearance.module_colors?theme.colors.ask:theme.colors.primary}}>
        <ArrowUpRight size={20} className={styles.arrow} aria-hidden="true" />
        {theme.modules.ask.icon?<img src={theme.modules.ask.icon} alt="" className={`${styles.icon} object-contain`}/>:<MailOpen strokeWidth={1.4} className={styles.icon} aria-hidden="true" />}
        <h2>{theme.modules.ask.label}</h2>
      </Link>
      {canSeeVoyage && <Link href="/voyage/" className={styles.entry} style={{color:theme.appearance.module_colors?theme.colors.voyage:theme.colors.primary}}>
        <ArrowUpRight size={20} className={styles.arrow} aria-hidden="true" />
        {theme.modules.voyage.icon?<img src={theme.modules.voyage.icon} alt="" className={`${styles.icon} object-contain`}/>:<Sailboat strokeWidth={1.4} className={styles.icon} aria-hidden="true" />}
        <h2>{theme.modules.voyage.label}</h2>
      </Link>}
      <Link href="/gifts/" className={styles.entry} style={{color:theme.appearance.module_colors?theme.colors.gifts:theme.colors.primary}}><ArrowUpRight size={20} className={styles.arrow} aria-hidden="true"/>{theme.modules.gifts.icon?<img src={theme.modules.gifts.icon} alt="" className={`${styles.icon} object-contain`}/>:<Gift strokeWidth={1.4} className={styles.icon} aria-hidden="true"/>}<h2>{theme.modules.gifts.label}</h2></Link>
      <Link href="/songs/" className={styles.entry} style={{color:theme.appearance.module_colors?theme.colors.songs:theme.colors.primary}}>
        <ArrowUpRight size={20} className={styles.arrow} aria-hidden="true" />
        {theme.modules.songs.icon?<img src={theme.modules.songs.icon} alt="" className={`${styles.icon} object-contain`}/>:<Music2 strokeWidth={1.4} className={styles.icon} aria-hidden="true" />}
        <h2>{theme.modules.songs.label}</h2>
      </Link>
      <Link href="/gallery/" className={styles.entry} style={{color:theme.appearance.module_colors?theme.colors.gallery:theme.colors.primary}}>
        <ArrowUpRight size={20} className={styles.arrow} aria-hidden="true" />
        {theme.modules.gallery.icon?<img src={theme.modules.gallery.icon} alt="" className={`${styles.icon} object-contain`}/>:<Images strokeWidth={1.4} className={styles.icon} aria-hidden="true" />}
        <h2>{theme.modules.gallery.label}</h2>
      </Link>
    </nav>
  </main>;
}
