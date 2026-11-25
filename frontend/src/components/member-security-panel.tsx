'use client';

import { useEffect, useRef, useState } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useAccountAuth } from '@/lib/account-auth';
import { changeMemberPassword, passwordProblem, revokeOtherMemberSessions } from '@/lib/member-api';

export function MemberSecurityPanel({ uid, onDone, onStateChange }: { uid: string; onDone: (message: string) => void; onStateChange?: (state: { busy: boolean; dirty: boolean }) => void }) {
  const { user, setUser, openAccountDialog, refresh } = useAccountAuth();
  const [currentPassword, setCurrentPassword] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('');
  const [revokePassword, setRevokePassword] = useState(''), [busy, setBusy] = useState(false), [error, setError] = useState('');
  const running = useRef(false), alive = useRef(true);
  const dirty = !!(currentPassword || password || confirmation || revokePassword);
  useEffect(() => { onStateChange?.({ busy, dirty }); }, [busy, dirty, onStateChange]);
  useEffect(() => {
    const warn = (event: BeforeUnloadEvent) => { if (dirty || busy) { event.preventDefault(); event.returnValue = ''; } };
    window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn);
  }, [dirty, busy]);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const submit = async (revoke = false) => {
    if (running.current) return;
    if (!revoke) { const message = passwordProblem(password, confirmation); if (message) { setError(message); return; } }
    running.current = true; setBusy(true); setError('');
    try {
      const response = revoke ? await revokeOtherMemberSessions(revokePassword, uid) : await changeMemberPassword(currentPassword, password, uid);
      if (!alive.current) return;
      if (response.code === 200) { setCurrentPassword(''); setPassword(''); setConfirmation(''); setRevokePassword(''); onDone(revoke ? '其他设备的登录已退出。当前设备仍保持登录。' : '本站密码已更新，其他设备需要重新登录。'); setUser(response.data); }
      else { setError(response.message || '操作失败，请稍后重试'); if ([401,403,408].includes(response.code)) void refresh(); }
    } catch { if (alive.current) setError('网络连接异常，请稍后重试'); }
    finally { if (alive.current) { running.current = false; setBusy(false); } }
  };
  return <section className="space-y-8" aria-labelledby="member-security-title"><div><h2 id="member-security-title" className="text-xl font-semibold">账户安全</h2><p className="mt-2 text-sm leading-7 text-muted-foreground">B 站 UID {user?.bilibili_uid} · {user?.signature_verified ? '已通过签名验证' : '尚未完成签名验证'}</p></div>
    <form className="max-w-xl space-y-4" onSubmit={event => { event.preventDefault(); void submit(); }}><h3 className="font-medium">修改本站密码</h3><div className="space-y-2"><Label htmlFor="current-member-password">当前密码</Label><Input id="current-member-password" type="password" autoComplete="current-password" required disabled={busy} value={currentPassword} onChange={e => setCurrentPassword(e.target.value)} /></div><div className="space-y-2"><Label htmlFor="new-member-password">新密码</Label><Input id="new-member-password" type="password" autoComplete="new-password" required disabled={busy} value={password} onChange={e => setPassword(e.target.value)} /><p className="text-xs leading-6 text-muted-foreground">至少 8 个字符，请勿使用 B 站登录密码。</p></div><div className="space-y-2"><Label htmlFor="confirm-member-password">确认新密码</Label><Input id="confirm-member-password" type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={e => setConfirmation(e.target.value)} /></div><div className="flex flex-wrap gap-3"><Button type="submit" disabled={busy}>保存新密码</Button><Button type="button" variant="ghost" disabled={busy} onClick={() => openAccountDialog('recover')}>忘记当前密码</Button></div></form>
    <form className="max-w-xl space-y-4 border-t border-border pt-6" onSubmit={event => { event.preventDefault(); void submit(true); }}><div><h3 className="font-medium">退出其他设备</h3><p className="mt-2 text-sm leading-7 text-muted-foreground">确认密码后，其他设备需要重新登录。当前设备可以继续使用。</p></div><div className="space-y-2"><Label htmlFor="revoke-member-password">本站密码</Label><Input id="revoke-member-password" type="password" autoComplete="current-password" required disabled={busy} value={revokePassword} onChange={e => setRevokePassword(e.target.value)} /></div><Button type="submit" variant="outline" disabled={busy}>退出其他设备</Button></form>
    {error && <p role="alert" className="text-sm leading-7 text-destructive">{error}</p>}
  </section>;
}
