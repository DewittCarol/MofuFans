'use client';

import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { confirmDiscard } from '@/lib/site-dialog';
import { blankAddress, type VoyageAddress } from '@/lib/voyage-claim';
import { clearMemberDefaultAddress, loadMemberAddressBook, MemberRequestError, saveMemberAddress, setMemberDefaultAddress, type MemberAddressBook } from '@/lib/member-api';

export function MemberAddressBookPanel({ uid, onStateChange }: { uid: string; onStateChange?: (state: { busy: boolean; dirty: boolean }) => void }) {
  const [book, setBook] = useState<MemberAddressBook | null>(null), [loading, setLoading] = useState(true);
  const [draft, setDraft] = useState<VoyageAddress>({ ...blankAddress }), [editing, setEditing] = useState(false);
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  const alive = useRef(true), running = useRef(false);
  const currentUID = useRef(uid); currentUID.current = uid;
  const dirty = editing && !!(draft.name || draft.phone || draft.address);
  useEffect(() => { onStateChange?.({ busy, dirty }); }, [busy, dirty, onStateChange]);
  const acceptBook = (value: MemberAddressBook) => {
    if (value.uid !== uid) {
      setBook(null); setDraft({ ...blankAddress }); setEditing(false);
      window.dispatchEvent(new Event('member-auth-refresh'));
      throw new MemberRequestError('登录账号已变化，请刷新页面后重试', 403);
    }
    setBook(value);
  };
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => { setBook(null); setDraft({ ...blankAddress }); setEditing(false); }, [uid]);
  useEffect(() => {
    let active = true; setLoading(true); setError('');
    loadMemberAddressBook(uid).then(value => { if (active) acceptBook(value); }).catch(reason => { if (active) { setBook(null); setError(reason.message); } }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [uid, attempt]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => { if (dirty || busy) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn);
  }, [dirty, busy]);
  const action = async (work: () => Promise<MemberAddressBook>, success: string, saved = false) => {
    if (running.current) return;
    running.current = true; setBusy(true); setError(''); setNotice('');
    try {
      const result = await work();
      if (!alive.current || currentUID.current !== uid) return;
      acceptBook(result); setNotice(success);
      if (saved) { setDraft({ ...blankAddress }); setEditing(false); }
    } catch (reason) {
      if (!alive.current || currentUID.current !== uid) return;
      const error = reason as MemberRequestError;
      if ([401,403,408].includes(error.code)) { setBook(null); setDraft({ ...blankAddress }); setEditing(false); }
      setError(error.message || '操作失败，请稍后重试');
    } finally { if (alive.current) { running.current = false; setBusy(false); } }
  };
  const edit = async (address: VoyageAddress = { ...blankAddress }) => {
    if (running.current || dirty && !await confirmDiscard('地址修改尚未保存，确定放弃吗？')) return;
    setDraft({ ...address, note: '' }); setEditing(true); setError(''); setNotice('');
  };
  const preferred = book?.entries.find(entry => entry.is_default);
  return <section aria-labelledby="member-book-title" className="space-y-6">
    <div className="flex flex-wrap items-start justify-between gap-4"><div><h2 id="member-book-title" className="text-xl font-semibold">我的地址簿</h2><p className="mt-2 max-w-2xl text-sm leading-7 text-muted-foreground">可以提前保存收货地址，无需等待收集批次开放。常用地址只能设置一个。</p></div><Button type="button" variant="outline" disabled={busy || loading || !book} onClick={() => void edit()}>添加地址</Button></div>
    {loading ? <p role="status" className="py-8 text-sm text-muted-foreground">正在读取地址簿…</p> : book && <>
      <section className="space-y-3 rounded-xl border border-border bg-card p-4 sm:p-5" aria-label="常用收货地址">
        <h3 className="font-medium">常用收货地址</h3>
        {book.long_term.enabled ? <>
          {preferred ? <div className="space-y-1 text-sm leading-7"><p>{preferred.address.name} · {preferred.address.phone}</p><p className="break-words">{preferred.address.address}</p></div> : <p className="text-sm leading-7">已开启自动填写。此前设置的地址可在对应收集记录中查看。</p>}
          {book.long_term.source === 'admin' && <p className="text-sm leading-7">主播已根据你的委托开启礼物托管。你可以随时停止自动填写。</p>}
          <Button type="button" variant="outline" disabled={busy || dirty} onClick={() => void action(() => clearMemberDefaultAddress(book.long_term.version, uid), '已停止自动填写。已保存的地址和已登记的批次不变。')}>停止以后自动填写</Button>
        </> : <p className="text-sm leading-7">尚未开启自动填写。可在下方选择一个已保存的地址，设为常用地址。</p>}
        <p className="text-xs leading-6 text-muted-foreground">设置后，有领取资格且开放收集的后续批次将使用常用地址。已登记和已寄出的地址不会自动更改；批次备注不会沿用。</p>
        {dirty && <p className="text-xs leading-6 text-muted-foreground">请先保存或取消下方的地址修改，再调整常用地址。</p>}
      </section>
      {editing && <form className="max-w-2xl space-y-4 rounded-xl border border-border p-4 sm:p-5" onSubmit={event => { event.preventDefault(); void action(() => saveMemberAddress(draft, uid), '地址已保存。需要以后自动使用时，请再将其设为常用地址。', true); }}>
        <h3 className="font-medium">保存收货地址</h3><p className="text-xs leading-6 text-muted-foreground">修改后会另存一条地址，不会覆盖已有批次的登记。备注请在对应批次填写。</p>
        <div className="grid gap-4 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="book-name">收件人</Label><Input id="book-name" autoComplete="name" required maxLength={40} disabled={busy} value={draft.name} onChange={e => setDraft({ ...draft, name: e.target.value })} /></div><div className="space-y-2"><Label htmlFor="book-phone">手机号码</Label><Input id="book-phone" type="tel" autoComplete="tel" required maxLength={25} disabled={busy} value={draft.phone} onChange={e => setDraft({ ...draft, phone: e.target.value })} /></div></div>
        <div className="space-y-2"><Label htmlFor="book-address">收货地址</Label><textarea id="book-address" autoComplete="street-address" rows={3} required minLength={5} maxLength={300} disabled={busy} value={draft.address} onChange={e => setDraft({ ...draft, address: e.target.value })} placeholder="省、市、区及详细地址" className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm leading-7 focus-visible:outline-2 focus-visible:outline-ring" /></div>
        <div className="flex gap-3"><Button type="submit" disabled={busy}>{busy ? '正在保存…' : '保存到地址簿'}</Button><Button type="button" variant="ghost" disabled={busy} onClick={async () => { if (!dirty || await confirmDiscard('地址修改尚未保存，确定取消吗？')) { setDraft({ ...blankAddress }); setEditing(false); } }}>取消</Button></div>
      </form>}
      {!book.entries.length ? <div className="py-8 text-sm leading-7 text-muted-foreground">地址簿中还没有收货地址。添加后，可在领取礼物时直接选择。</div> : <div className="divide-y divide-border border-y border-border">{book.entries.map(entry => <article key={entry.id} className="space-y-3 py-5"><div className="flex flex-wrap items-start justify-between gap-3"><p className="break-words font-medium">{entry.address.name}<span className="ml-3 text-sm font-normal">{entry.address.phone}</span></p>{entry.is_default && <span className="shrink-0 text-sm text-primary">常用地址</span>}</div><p className="break-words text-sm leading-7">{entry.address.address}</p><div className="flex flex-wrap gap-2"><Button type="button" variant="outline" size="sm" disabled={busy || dirty || entry.is_default} onClick={() => void action(() => setMemberDefaultAddress(entry.id, book.long_term.version, uid), '已设置常用地址。已登记批次的地址保持不变。')}>{entry.is_default ? '已设为常用' : '设为常用地址'}</Button><Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void edit(entry.address)}>修改后另存</Button></div></article>)}</div>}
    </>}
    {error && <div role="alert" className="space-y-3"><p className="text-sm leading-7 text-destructive">{error}</p><Button type="button" variant="outline" disabled={busy || loading} onClick={() => setAttempt(value => value + 1)}>重新读取</Button></div>}
    {notice && <p role="status" className="text-sm leading-7 text-primary">{notice}</p>}
  </section>;
}
