"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import styles from "./settings-navigation.module.css";
import { useAuth } from "@/lib/auth";

const baseSections = [
  { id: "submission-settings", label: "投稿权限" },
  { id: "voyage-settings", label: "舰礼展示" },
  { id: "bilibili-verification", label: "B 站注册验证" },
];

export function SettingsNavigation({ children }: { children: ReactNode }) {
  const { user } = useAuth();
  const isOwner = !!user?.is_super_admin;
  const sections = useMemo(() => isOwner ? [{ id: "installation-settings", label: "部署与集成" }, ...baseSections, { id: "ai-settings", label: "AI 辅助审核" }] : baseSections, [isOwner]);
  const container = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(sections[0].id);

  const jumpTo = (id: string) => {
    const target = document.getElementById(id);
    const main = container.current?.closest("main");
    if (!target || !main) return;
    const offset = matchMedia("(min-width: 1024px)").matches ? 32 : 80;
    main.scrollTo({
      top: main.scrollTop + target.getBoundingClientRect().top - main.getBoundingClientRect().top - offset,
      behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "instant" : "smooth",
    });
    target.focus({ preventScroll: true });
    window.history.replaceState(window.history.state, "", `#${id}`);
  };

  useEffect(() => {
    const el = container.current;
    const main = el?.closest("main");
    if (!el || !main) return;
    let frame = 0;
    const measure = () => {
      frame = 0;
      const threshold = main.getBoundingClientRect().top + (matchMedia("(min-width: 1024px)").matches ? 48 : 96);
      let current = sections[0].id;
      for (const section of sections) {
        const target = document.getElementById(section.id);
        if (target && target.getBoundingClientRect().top <= threshold) current = section.id;
      }
      if (main.scrollTop + main.clientHeight >= main.scrollHeight - 2) current = sections[sections.length - 1].id;
      setActive(current);
    };
    const schedule = () => { if (!frame) frame = requestAnimationFrame(measure); };
    const observer = new ResizeObserver(schedule);
    observer.observe(el); observer.observe(main);
    main.addEventListener("scroll", schedule, { passive: true });
    measure();
    return () => { observer.disconnect(); main.removeEventListener("scroll", schedule); cancelAnimationFrame(frame); };
  }, [sections]);

  return <div ref={container} className={styles.layout}>
    <nav className={styles.navigation} aria-label="设置分区导航">
      <p className={styles.label}>设置导航</p>
      <div className={styles.links}>
        {sections.map(section => <a key={section.id} href={`#${section.id}`}
          aria-current={active === section.id ? "location" : undefined}
          onClick={event => { event.preventDefault(); jumpTo(section.id); }}>{section.label}</a>)}
        <a className={styles.saveLink} href="#save-settings" onClick={event => { event.preventDefault(); jumpTo("save-settings"); }}>保存设置</a>
      </div>
      <select aria-label="跳转到设置分区" className={styles.mobileSelect} value={active} onChange={event => jumpTo(event.target.value)}>
        {sections.map(section => <option key={section.id} value={section.id}>{section.label}</option>)}
        <option value="save-settings">保存设置</option>
      </select>
    </nav>
    <div className={styles.content}>{children}</div>
  </div>;
}
