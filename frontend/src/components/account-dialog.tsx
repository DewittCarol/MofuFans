'use client';

import { useEffect, useRef, useState } from 'react';
import { CheckCircle2, Copy, Loader2, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { type AccountUser, loginAccount } from '@/lib/api';
import { checkSignature, finishSignature, getSignatureStatus, passwordProblem, signatureSecondsLeft, startSignature,
  type AccountDialogMode, type SignatureChallenge, type SignaturePurpose } from '@/lib/member-api';
import { clearSignatureDismissal, clearSignatureProgress, dismissSignatureProgress, isSignatureDismissed, readSignatureProgress, recoverSignatureProgress, sameSignatureProgress, saveSignatureProgress } from '@/lib/signature-progress';

const purposeLabel = { register: '注册', recover: '找回密码', upgrade: '身份验证' };

interface AccountDialogProps {
  open: boolean;
  initialMode: AccountDialogMode;
  currentUser?: AccountUser | null;
  onClose: () => void;
  onAuthenticated: (user: AccountUser) => void;
}

export function AccountDialog({ open, initialMode, currentUser, onClose, onAuthenticated }: AccountDialogProps) {
  const [mode, setMode] = useState<AccountDialogMode>(initialMode);
  const [uid, setUID] = useState(''), [loginName, setLoginName] = useState('');
  const [legacyLogin, setLegacyLogin] = useState(false);
  const [password, setPassword] = useState(''), [confirmPassword, setConfirmPassword] = useState('');
  const [verification, setVerification] = useState<SignatureChallenge | null>(null);
  const [resumeCandidate, setResumeCandidate] = useState<SignatureChallenge | null>(null);
  const [recovery, setRecovery] = useState<'checking' | 'ready' | 'unavailable'>('checking'), [recoveryAttempt, setRecoveryAttempt] = useState(0);
  const [error, setError] = useState(''), [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false), [remaining, setRemaining] = useState(0);
  const epoch = useRef(0), running = useRef(false), panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    epoch.current++; running.current = false; setBusy(false);
    setMode(initialMode); setUID(initialMode === 'upgrade' ? currentUser?.bilibili_uid || '' : '');
    setLoginName(''); setLegacyLogin(false); setPassword(''); setConfirmPassword(''); setVerification(null); setResumeCandidate(null); setRemaining(0); setError(''); setNotice('');
    if (!open) return;
    setRecovery('checking');
    const ticket = epoch.current, cached = readSignatureProgress();
    void getSignatureStatus().then(response => {
      if (epoch.current !== ticket) return;
      if (response.code >= 500 || response.code === 429) { setRecovery('unavailable'); setError('暂时无法读取验证进度，请稍后重试'); return; }
      if (response.code !== 200) {
        if (cached) clearSignatureProgress(cached.challenge_id); setRecovery('ready');
        if (cached || response.code !== 404) setError(response.message || '上次验证已失效，请重新开始');
        return;
      }
      const restored = recoverSignatureProgress(response.data, cached);
      if (!restored) {
        if (cached) clearSignatureProgress(cached.challenge_id); setRecovery('ready');
        if (cached || response.data) setError(response.data?.status === 'consumed' ? '上次验证已完成，请登录或重新开始' : '上次验证已过期，请重新开始');
        return;
      }
      if (restored.purpose === 'upgrade' && restored.bilibili_uid !== currentUser?.bilibili_uid) {
        if (cached) clearSignatureProgress(cached.challenge_id); setRecovery('ready'); setError('此验证申请属于其他账号，请重新开始'); return;
      }
      const dismissed = isSignatureDismissed(restored);
      if (!dismissed) saveSignatureProgress(restored); setRecovery('ready');
      if (!dismissed && initialMode === restored.purpose && (!cached || sameSignatureProgress(cached, restored))) {
        setVerification(restored); setUID(restored.bilibili_uid); setRemaining(signatureSecondsLeft(restored));
        setNotice(`已恢复上次的${purposeLabel[restored.purpose]}进度，UID ${restored.bilibili_uid}。`);
      } else setResumeCandidate(restored);
    }).catch(() => { if (epoch.current === ticket) { setRecovery('unavailable'); setError('无法读取验证进度，请检查网络后重试'); } });
    const previous = document.activeElement as HTMLElement | null;
    const timer = window.setTimeout(() => panel.current?.querySelector<HTMLElement>('input,button')?.focus(), 0);
    return () => { epoch.current++; window.clearTimeout(timer); previous?.focus(); };
  }, [open, initialMode, currentUser?.bilibili_uid, recoveryAttempt]);

  useEffect(() => {
    const progress = verification || resumeCandidate;
    if (!open || !progress) return;
    const tick = () => { const left = signatureSecondsLeft(progress); setRemaining(left); if (!left) clearSignatureProgress(progress.challenge_id); };
    tick(); const timer = window.setInterval(tick, 1000);
    return () => window.clearInterval(timer);
  }, [open, verification, resumeCandidate]);

  const reset = (next: AccountDialogMode = mode) => {
    if (running.current) return;
    const abandoned = verification || resumeCandidate || readSignatureProgress();
    dismissSignatureProgress(abandoned);
    if (!abandoned) clearSignatureProgress();
    epoch.current++; setMode(next); setVerification(null); setResumeCandidate(null); setRecovery('ready'); setError(''); setNotice('');
    setPassword(''); setConfirmPassword(''); setRemaining(0);
  };
  const resume = () => {
    if (!resumeCandidate || running.current) return;
    if (signatureSecondsLeft(resumeCandidate) <= 0) { reset(resumeCandidate.purpose); setError('上次验证已过期，请重新开始'); return; }
    clearSignatureDismissal(resumeCandidate.challenge_id); saveSignatureProgress(resumeCandidate);
    epoch.current++; setMode(resumeCandidate.purpose); setUID(resumeCandidate.bilibili_uid); setVerification(resumeCandidate); setResumeCandidate(null);
    setPassword(''); setConfirmPassword(''); setError(''); setNotice(`已恢复上次的${purposeLabel[resumeCandidate.purpose]}进度，UID ${resumeCandidate.bilibili_uid}。`);
  };
  const perform = async (work: (active: () => boolean) => Promise<void>) => {
    if (running.current) return;
    const ticket = epoch.current; running.current = true; setBusy(true); setError(''); setNotice('');
    const active = () => epoch.current === ticket;
    try { await work(active); } catch { if (active()) setError('网络连接异常，请稍后重试'); }
    finally { if (active()) { running.current = false; setBusy(false); } }
  };
  const start = (event: React.FormEvent) => {
    event.preventDefault();
    if (!/^[1-9]\d{0,18}$/.test(uid.trim())) { setError('请输入正确的 B 站 UID'); return; }
    void perform(async active => {
      clearSignatureProgress(); setResumeCandidate(null);
      const response = await startSignature(uid.trim(), mode as SignaturePurpose);
      if (!active()) return;
      if (response.code === 200) {
        const next = recoverSignatureProgress(response.data, null);
        if (!next || next.bilibili_uid !== uid.trim() || next.purpose !== mode) { clearSignatureProgress(); setError('验证申请状态异常，请重新开始'); return; }
        clearSignatureDismissal(); saveSignatureProgress(next); setVerification(next); setRemaining(signatureSecondsLeft(next));
      }
      else setError(response.message || '无法开始验证');
    });
  };
  const check = () => void perform(async active => {
    const response = await checkSignature(verification!.challenge_id);
    if (!active()) return;
    if (response.code === 200) {
      if (response.data.challenge_id !== verification!.challenge_id || response.data.bilibili_uid !== verification!.bilibili_uid) { clearSignatureProgress(verification!.challenge_id); setVerification(null); setPassword(''); setConfirmPassword(''); setError('验证申请已变化，请重新开始'); return; }
      const next = recoverSignatureProgress({ ...verification!, ...response.data }, null);
      if (!next || next.purpose !== verification!.purpose) { clearSignatureProgress(verification!.challenge_id); setVerification(null); setError('验证申请已失效，请重新开始'); return; }
      saveSignatureProgress(next); setVerification(next); setRemaining(signatureSecondsLeft(next));
    }
    else { if ([401,403,404,408,409,410].includes(response.code)) { clearSignatureProgress(verification!.challenge_id); setVerification(null); setPassword(''); setConfirmPassword(''); } setError(response.message || '暂未读取到验证文字，请确认签名已保存后再检查'); }
  });
  const finish = (event: React.FormEvent) => {
    event.preventDefault();
    if (mode !== 'upgrade') { const issue = passwordProblem(password, confirmPassword); if (issue) { setError(issue); return; } }
    void perform(async active => {
      const response = await finishSignature(verification!.challenge_id, mode === 'upgrade' ? undefined : password);
      if (!active()) return;
      if (response.code === 200) {
        clearSignatureProgress(verification!.challenge_id); clearSignatureDismissal(verification!.challenge_id);
        if (response.data.bilibili_uid !== verification!.bilibili_uid) { setVerification(null); setPassword(''); setConfirmPassword(''); setError('账号状态已变化，请重新登录'); return; }
        setPassword(''); setConfirmPassword(''); onAuthenticated(response.data);
      }
      else { if ([401,403,404,408,409,410].includes(response.code)) { clearSignatureProgress(verification!.challenge_id); setVerification(null); setPassword(''); setConfirmPassword(''); } setError(response.message || '无法完成操作，请重试'); }
    });
  };
  const login = (event: React.FormEvent) => {
    event.preventDefault();
    void perform(async active => {
      const response = await loginAccount(loginName.trim(), password, legacyLogin);
      if (!active()) return;
      if (response.code === 200) { clearSignatureProgress(resumeCandidate?.challenge_id); setPassword(''); onAuthenticated(response.data); }
      else setError(response.message || '登录失败');
    });
  };
  const copy = async () => {
    try { await navigator.clipboard.writeText(verification?.verification_text || ''); setNotice('验证文字已复制'); }
    catch { setNotice('请选中上方文字后手动复制'); }
  };
  if (!open) return null;
  const title = { login: '登录', register: '注册账号', recover: '找回密码', upgrade: '验证 B 站身份' }[mode];
  const expired = !!verification && (remaining === 0 || ['expired', 'consumed'].includes(verification.status));
  const close = () => { if (!running.current) { epoch.current++; setPassword(''); setConfirmPassword(''); onClose(); } };

  return <div className="fixed inset-0 z-[1200] flex items-center justify-center bg-black/45 px-4 py-6" data-pet-obscured="">
    <div ref={panel} role="dialog" aria-modal="true" aria-labelledby="account-dialog-title" aria-busy={busy}
      className="max-h-[90svh] w-full max-w-md overflow-y-auto rounded-md border border-[var(--fabric-stitch)] bg-card p-5 text-foreground shadow-xl sm:p-6"
      onMouseDown={event => event.stopPropagation()}
      onKeyDown={event => {
        if (event.key === 'Escape') close();
        if (event.key !== 'Tab') return;
        const controls = panel.current?.querySelectorAll<HTMLElement>('button:not([disabled]),input:not([disabled]),textarea:not([disabled]),a[href],summary');
        if (!controls?.length) return;
        const first = controls[0], last = controls[controls.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }}>
      <div className="mb-5 flex items-center justify-between gap-4"><h2 id="account-dialog-title" className="text-xl font-semibold">{title}</h2><button type="button" onClick={close} disabled={busy} className="flex size-11 items-center justify-center rounded hover:bg-secondary disabled:opacity-60" aria-label="关闭"><X size={20} /></button></div>
      {recovery === 'checking' ? <p role="status" className="py-6 text-sm leading-7 text-muted-foreground">正在读取验证进度…</p> : recovery === 'unavailable' ? <div className="space-y-4"><p className="text-sm leading-7">验证进度暂时无法确认。可以重试恢复，也可以重新开始。</p><div className="flex flex-wrap gap-3"><Button type="button" onClick={() => setRecoveryAttempt(value => value + 1)}>重试恢复</Button><Button type="button" variant="outline" onClick={() => reset()}>重新开始</Button></div></div> : <>
      {resumeCandidate && <section className="mb-5 space-y-3 rounded-lg border border-border p-4" aria-label="未完成的验证"><p className="text-sm font-medium">有一项未完成的{purposeLabel[resumeCandidate.purpose]}</p><p className="text-sm text-muted-foreground">UID {resumeCandidate.bilibili_uid} · {remaining > 0 ? `剩余 ${Math.floor(remaining / 60)} 分 ${remaining % 60} 秒` : '已过期'}</p><Button type="button" variant="outline" className="min-h-11" disabled={busy} onClick={resume}>{remaining > 0 ? `继续上次${purposeLabel[resumeCandidate.purpose]}` : '重新开始验证'}</Button></section>}
      {mode === 'login' ? <form className="space-y-4" onSubmit={login}>
        <div><label htmlFor="member-login-name" className="mb-2 block text-sm font-medium">{legacyLogin ? '原登录名' : 'B 站 UID 或原登录名'}</label><Input id="member-login-name" value={loginName} onChange={e => setLoginName(e.target.value)} autoComplete="username" maxLength={32} disabled={busy} required /><button type="button" disabled={busy} onClick={() => { setLegacyLogin(value => !value); setError(''); }} className="mt-1 min-h-11 text-xs text-muted-foreground hover:underline">{legacyLogin ? '返回 UID 登录' : '原登录名是纯数字？使用原登录名登录'}</button></div>
        <div><label htmlFor="member-login-password" className="mb-2 block text-sm font-medium">本站密码</label><Input id="member-login-password" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" disabled={busy} required /></div>
        <Button type="submit" disabled={busy} className="w-full min-h-11">{busy ? <><Loader2 size={16} className="animate-spin" />正在登录…</> : '登录'}</Button>
        <div className="flex flex-wrap justify-between gap-3 text-sm"><button type="button" disabled={busy} onClick={() => reset('register')} className="min-h-11 text-primary underline-offset-4 hover:underline">注册新账号</button><button type="button" disabled={busy} onClick={() => reset('recover')} className="min-h-11 text-muted-foreground underline-offset-4 hover:underline">忘记密码</button></div>
      </form> : !verification ? <form className="space-y-4" onSubmit={start}>
        <p className="text-sm leading-7 text-muted-foreground">{mode === 'upgrade' ? '完成一次签名验证后，即可使用地址簿，并查看本人的领取和物流记录。' : mode === 'recover' ? '通过 B 站签名重新验证账号归属后，即可设置新的本站密码。' : '首次注册需要验证 B 站账号归属。验证完成后，使用 UID 和本站密码即可登录。'}</p>
        <div><label htmlFor="member-signature-uid" className="mb-2 block text-sm font-medium">你的 B 站 UID</label><Input id="member-signature-uid" inputMode="numeric" value={uid} onChange={e => setUID(e.target.value)} disabled={busy || mode === 'upgrade'} autoComplete="off" maxLength={19} required /></div>
        {resumeCandidate && <p className="text-xs leading-6 text-muted-foreground">开始新的验证会替换上次未完成的进度。</p>}
        <Button type="submit" disabled={busy || mode === 'upgrade' && !uid} className="w-full min-h-11">{busy ? '正在生成验证文字…' : '开始签名验证'}</Button>
        {mode !== 'upgrade' && <button type="button" disabled={busy} onClick={() => reset('login')} className="min-h-11 text-sm text-muted-foreground hover:underline">返回登录</button>}
      </form> : expired ? <div className="space-y-4"><p className="text-sm leading-7">本次验证已过期，请重新生成验证文字。</p><Button className="min-h-11 w-full" onClick={() => reset()} disabled={busy}>重新验证</Button></div>
        : verification.status === 'verified' ? <form onSubmit={finish} className="space-y-4">
          <div className="space-y-2 rounded-lg border border-border p-4"><p className="flex items-center gap-2 font-medium"><CheckCircle2 size={18} className="text-success" />签名验证通过</p><p className="break-words text-sm">{verification.bilibili_name} · UID {verification.bilibili_uid}</p><p className="text-sm text-muted-foreground">可以恢复原来的 B 站签名了。</p></div>
          {mode !== 'upgrade' && <><div><label htmlFor="member-new-password" className="mb-2 block text-sm font-medium">{mode === 'recover' ? '新的本站密码' : '设置本站密码'}</label><Input id="member-new-password" type="password" autoComplete="new-password" value={password} onChange={e => setPassword(e.target.value)} disabled={busy} required /><p className="mt-2 text-xs leading-6 text-muted-foreground">至少 8 个字符，请勿使用 B 站登录密码。</p></div><div><label htmlFor="member-confirm-password" className="mb-2 block text-sm font-medium">确认密码</label><Input id="member-confirm-password" type="password" autoComplete="new-password" value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} disabled={busy} required /></div></>}
          <p className="text-xs leading-6 text-muted-foreground">剩余 {Math.floor(remaining / 60)} 分 {remaining % 60} 秒。关闭后可继续完成，密码需要重新输入。{mode === 'recover' && '重设密码后，其他设备需要重新登录。'}</p>
          <Button type="submit" disabled={busy} className="w-full min-h-11">{busy ? '正在保存…' : mode === 'register' ? '创建账号并登录' : mode === 'recover' ? '重设密码并登录' : '完成验证'}</Button>
        </form> : <div className="space-y-4">
          <p className="text-sm leading-7">{verification.verification_text ? '复制下方验证文字，打开你的 ' : '这是更新前发起的验证，原验证文字仍然有效。如果已修改 ' }<a href={`https://space.bilibili.com/${verification.bilibili_uid}`} target="_blank" rel="noopener noreferrer" className="text-primary underline underline-offset-4">{verification.verification_text ? 'B 站个人空间' : 'B 站个人签名'}</a>{verification.verification_text ? '，编辑个人签名，将文字添加到原签名末尾，并用空格隔开。保存后返回本页检查，验证通过后可恢复原签名。' : '，可直接检查验证；如果还未修改且没有保存原文字，请重新生成。'}</p>
          {verification.verification_text && <div><label htmlFor="member-verification-text" className="mb-2 block text-sm font-medium">本次验证文字</label><Input id="member-verification-text" readOnly autoComplete="off" value={verification.verification_text} onFocus={e => e.target.select()} className="h-11 w-full font-mono text-sm" /><Button type="button" variant="outline" size="sm" className="mt-2 min-h-11" onClick={copy}><Copy size={15} />复制验证文字</Button></div>}
          {verification.verification_text && <details className="rounded-md border border-border px-3">
            <summary className="min-h-11 cursor-pointer py-3 text-sm font-medium text-primary underline-offset-4 hover:underline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-current">查看签名填写位置示例</summary>
            <figure className="space-y-3 pb-3">
              <p className="text-sm leading-6">网页端的个人签名位于昵称下方，点击该处即可编辑。</p>
              {/* Keep the supplied screenshot unchanged; the current challenge is only copied from the field above. */}
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src="/images/signature-verification-example.svg" width={342} height={90} alt="B 站网页端个人空间：昵称下方的个人签名填写位置" className="block h-auto max-w-full rounded" />
              <figcaption className="text-xs leading-6 text-muted-foreground">图片仅为填写位置示例，请使用上方本次生成的验证文字。</figcaption>
            </figure>
          </details>}
          <p className="text-xs leading-6 text-muted-foreground">剩余 {Math.floor(remaining / 60)} 分 {remaining % 60} 秒。仅使用你在本站生成的文字，不要替他人添加验证内容。</p>
          <Button className="w-full min-h-11" disabled={busy} onClick={check}>{busy ? '正在读取签名…' : '已修改签名，检查验证'}</Button>
          <p className="text-xs leading-6 text-muted-foreground">签名更新可能稍有延迟。如果暂未通过，请稍等后再次检查，无需反复修改。</p>
          <p className="text-xs leading-6 text-muted-foreground">有效期内关闭或刷新页面后，可继续本次验证。</p>
          <button type="button" disabled={busy} onClick={() => reset()} className="min-h-11 text-sm text-muted-foreground hover:underline">重新选择 UID 或生成验证文字</button>
        </div>}
      </>}
      {error && <p role="alert" className="mt-4 text-sm leading-6 text-destructive">{error}</p>}
      {notice && <p role="status" className="mt-4 text-sm leading-6 text-primary">{notice}</p>}
    </div>
  </div>;
}
