'use client';

import { Button } from '@/components/ui/button';
import { SignedInMemberHome } from '@/components/member-home';
import { useAccountAuth } from '@/lib/account-auth';

export default function AccountPage() {
  const { user, loading, accountsEnabled, authRevision, openAccountDialog } = useAccountAuth();
  return <main className="mx-auto min-h-[calc(100svh-56px)] w-full max-w-6xl px-4 pb-16 pt-6 sm:px-8 sm:pt-10">
    {loading ? <section className="rounded-xl border border-border bg-card p-6"><h1 className="text-2xl font-semibold">个人主页</h1><p role="status" className="mt-4 text-sm leading-7 text-muted-foreground">正在检查登录状态…</p></section> : !accountsEnabled ? <section className="rounded-xl border border-border bg-card p-6"><h1 className="text-2xl font-semibold">个人主页</h1><p className="mt-4 text-sm leading-7 text-muted-foreground">本站暂未开放账号登录，请留意主播通知。</p></section> : !user ? <section className="space-y-5 rounded-xl border border-border bg-card p-6"><h1 className="text-2xl font-semibold">个人主页</h1><p className="max-w-2xl text-sm leading-7">登录后即可查看自己的地址与领取记录。首次注册通过 B 站签名验证身份，无需等待主播发送私信。</p><div className="flex flex-wrap gap-3"><Button onClick={() => openAccountDialog('login')}>登录</Button><Button variant="outline" onClick={() => openAccountDialog('register')}>注册账号</Button></div></section> : <SignedInMemberHome key={user.bilibili_uid} user={user} authRevision={authRevision} />}
  </main>;
}
