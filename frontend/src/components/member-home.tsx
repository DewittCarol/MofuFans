'use client';

import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from 'react';
import Image from 'next/image';
import { BookUser, Package, ShieldCheck } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { MemberAddressBookPanel } from '@/components/member-address-book';
import { MemberCollections } from '@/components/member-collections';
import { MemberSecurityPanel } from '@/components/member-security-panel';
import { useAccountAuth } from '@/lib/account-auth';
import { confirmDiscard, siteDialogs } from '@/lib/site-dialog';
import type { AccountUser } from '@/lib/api';

export type MemberHomeSection = 'addresses' | 'collections' | 'security';
type PanelState = { busy: boolean; dirty: boolean };
const sections = [
  { id: 'addresses', label: '地址簿', icon: BookUser },
  { id: 'collections', label: '礼物与物流', icon: Package },
  { id: 'security', label: '账户安全', icon: ShieldCheck },
] as const;

function MemberAvatar({ src, name }: { src: string; name: string }) {
  const [failed, setFailed] = useState(false);
  return <div className="flex size-14 shrink-0 items-center justify-center overflow-hidden rounded-full border border-border bg-muted text-2xl font-medium sm:size-20">
    {src && !failed ? <Image src={src} alt="" width={80} height={80} unoptimized onError={() => setFailed(true)} className="h-full w-full object-cover" /> : <span aria-hidden="true">{Array.from(name)[0] || '用'}</span>}
  </div>;
}

export function MemberAccountHeader({ user, onLogout }: { user: AccountUser; onLogout: () => void }) {
  const name = user.bilibili_name || user.username || 'B 站用户';
  return <header className="flex items-center gap-4 rounded-xl border border-border bg-card p-4 sm:gap-5 sm:p-6">
    <MemberAvatar key={user.bilibili_avatar} src={user.bilibili_avatar} name={name} />
    <div className="min-w-0 flex-1"><p className="mb-1 text-xs leading-5 text-muted-foreground">个人主页</p><h1 className="break-words text-xl font-semibold leading-snug sm:text-2xl">{name}</h1><p className="mt-2 break-all text-sm tabular-nums text-muted-foreground">UID {user.bilibili_uid}</p></div>
    <Button type="button" variant="outline" className="min-h-11 shrink-0" onClick={onLogout}>退出登录</Button>
  </header>;
}

export function MemberHomeWorkspace({ user, initialSection, onSectionChange, onNotice }: {
  user: AccountUser;
  initialSection: MemberHomeSection;
  onSectionChange: (section: MemberHomeSection) => void;
  onNotice: (message: string) => void;
}) {
  const { openAccountDialog } = useAccountAuth();
  const [active, setActive] = useState(initialSection), [focused, setFocused] = useState(initialSection);
  const [panelState, setPanelState] = useState<PanelState>({ busy: false, dirty: false });
  const [changing, setChanging] = useState(false), [vertical, setVertical] = useState(false);
  const alive = useRef(true), switching = useRef(false), activeRef = useRef(active), stateRef = useRef(panelState);
  const panelGeneration = useRef(0), generation = panelGeneration.current;
  const ownedDialog = useRef<number | null>(null), buttons = useRef<Partial<Record<MemberHomeSection, HTMLButtonElement>>>({});
  const restoreFocus = useRef<MemberHomeSection | null>(null);
  useEffect(() => {
    alive.current = true;
    return () => { alive.current = false; if (ownedDialog.current !== null) siteDialogs.finish(ownedDialog.current, false); ownedDialog.current = null; };
  }, []);
  useEffect(() => { if (!changing && restoreFocus.current) { buttons.current[restoreFocus.current]?.focus(); restoreFocus.current = null; } }, [changing, active]);
  useEffect(() => {
    const media = window.matchMedia('(min-width: 768px)');
    const update = () => setVertical(media.matches); update();
    media.addEventListener('change', update); return () => media.removeEventListener('change', update);
  }, []);
  const reportState = useCallback((state: PanelState) => {
    if (!alive.current || activeRef.current !== active || panelGeneration.current !== generation) return;
    stateRef.current = state;
    setPanelState(previous => previous.busy === state.busy && previous.dirty === state.dirty ? previous : state);
  }, [active, generation]);
  const choose = async (next: MemberHomeSection) => {
    if (next === activeRef.current || stateRef.current.busy || switching.current) return;
    switching.current = true; setChanging(true);
    try {
      if (stateRef.current.dirty) {
        restoreFocus.current = next;
        const previous = siteDialogs.getSnapshot()?.id;
        const confirmation = confirmDiscard(activeRef.current === 'security' ? '已输入的密码将被清除，确定切换吗？' : '当前修改尚未保存，确定放弃并切换吗？');
        const opened = siteDialogs.getSnapshot();
        if (opened && opened.id !== previous) ownedDialog.current = opened.id;
        const accepted = await confirmation;
        ownedDialog.current = null;
        if (!accepted || !alive.current || stateRef.current.busy) return;
      }
      if (!alive.current) return;
      panelGeneration.current++; activeRef.current = next; stateRef.current = { busy: false, dirty: false };
      setPanelState(stateRef.current); setActive(next); setFocused(next); onSectionChange(next);
    } finally { if (alive.current) { switching.current = false; setChanging(false); } }
  };
  const moveFocus = (event: KeyboardEvent<HTMLButtonElement>, section: MemberHomeSection) => {
    if (stateRef.current.busy || switching.current) return;
    const index = sections.findIndex(item => item.id === section);
    let next: number;
    if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = sections.length - 1;
    else if (event.key === 'ArrowRight' || event.key === 'ArrowDown') next = (index + 1) % sections.length;
    else if (event.key === 'ArrowLeft' || event.key === 'ArrowUp') next = (index + sections.length - 1) % sections.length;
    else return;
    event.preventDefault(); const id = sections[next].id; setFocused(id); buttons.current[id]?.focus();
  };

  // At the 1152px shell limit: 1088px inner width = 184px navigation + 24px gap + 880px panel.
  return <div className="grid items-start gap-4 md:grid-cols-[184px_minmax(0,1fr)] md:gap-6">
    <aside className="rounded-xl border border-border bg-card p-2 md:sticky md:top-24">
      <div role="tablist" aria-label="个人主页功能" aria-orientation={vertical ? 'vertical' : 'horizontal'} className="grid grid-cols-3 gap-1 md:grid-cols-1">
        {sections.map(({ id, label, icon: Icon }) => <button key={id} ref={node => { if (node) buttons.current[id] = node; else delete buttons.current[id]; }} type="button" role="tab" id={`member-tab-${id}`} aria-selected={active === id} aria-controls={`member-panel-${id}`} tabIndex={focused === id ? 0 : -1} disabled={panelState.busy || changing} onFocus={() => setFocused(id)} onKeyDown={event => moveFocus(event, id)} onClick={() => void choose(id)} className={`flex min-h-11 min-w-0 flex-col items-center justify-center gap-1.5 rounded-lg px-2 py-3 text-sm leading-5 transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:cursor-wait disabled:opacity-70 md:flex-row md:justify-start md:gap-3 md:px-3 ${active === id ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'}`}><Icon size={18} aria-hidden="true" className="shrink-0" /><span>{label}</span></button>)}
      </div>
      {panelState.busy && <p role="status" className="px-3 pb-1 pt-3 text-xs leading-6 text-muted-foreground">操作进行中，请稍后切换。</p>}
    </aside>
    <div className="min-w-0">
      {sections.map(({ id }) => <section key={id} role="tabpanel" id={`member-panel-${id}`} aria-labelledby={`member-tab-${id}`} hidden={active !== id} tabIndex={active === id ? 0 : -1} aria-busy={active === id && panelState.busy} className="min-h-48 rounded-xl border border-border bg-card p-4 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring sm:p-6">
        {active === id && (id === 'security' ? <MemberSecurityPanel uid={user.bilibili_uid} onDone={onNotice} onStateChange={reportState} /> : !user.signature_verified ? <div className="space-y-4"><h2 className="text-xl font-semibold">完成签名验证</h2><p className="max-w-2xl text-sm leading-7 text-muted-foreground">为保护收货信息，需要补充一次 B 站签名验证。验证完成后即可使用地址簿，查看本人的礼物与物流。</p><Button type="button" onClick={() => openAccountDialog('upgrade')}>验证 B 站身份</Button></div> : id === 'addresses' ? <MemberAddressBookPanel uid={user.bilibili_uid} onStateChange={reportState} /> : <MemberCollections uid={user.bilibili_uid} onStateChange={reportState} />)}
      </section>)}
    </div>
  </div>;
}

export function SignedInMemberHome({ user, authRevision }: { user: AccountUser; authRevision: number }) {
  const { logout } = useAccountAuth();
  const [section, setSection] = useState<MemberHomeSection>(user.signature_verified ? 'addresses' : 'security');
  const [notice, setNotice] = useState('');
  return <div className="space-y-4 md:space-y-6">
    <MemberAccountHeader user={user} onLogout={() => void logout()} />
    {notice && <p role="status" className="rounded-xl border border-border bg-card px-5 py-3 text-sm leading-7 text-primary">{notice}</p>}
    <MemberHomeWorkspace key={`${user.bilibili_uid}:${authRevision}`} user={user} initialSection={section} onSectionChange={next => { setSection(next); setNotice(''); }} onNotice={setNotice} />
  </div>;
}
