'use client';

import { useEffect, useState } from 'react';
import { Button } from '@/components/ui/button';
import { VoyageClaim } from '@/components/voyage-claim';
import { confirmDiscard } from '@/lib/site-dialog';
import { loadMemberCollections, type MemberCollection } from '@/lib/member-api';

export function MemberCollections({ uid, onStateChange }: { uid: string; onStateChange?: (state: { busy: boolean; dirty: boolean }) => void }) {
  const [rows, setRows] = useState<MemberCollection[]>([]), [loading, setLoading] = useState(true), [error, setError] = useState(''), [attempt, setAttempt] = useState(0);
  const [active, setActive] = useState<{ id: number; mode: 'address' | 'shipping' } | null>(null);
  const [formState, setFormState] = useState({ busy: false, dirty: false });
  useEffect(() => { onStateChange?.(formState); }, [formState, onStateChange]);
  useEffect(() => { let valid = true; setLoading(true); setError(''); loadMemberCollections(uid).then(value => { if (valid) setRows(value); }).catch(reason => { if (valid) { setRows([]); setError(reason.message); } }).finally(() => { if (valid) setLoading(false); }); return () => { valid = false; }; }, [uid, attempt]);
  useEffect(() => { const warn = (event: BeforeUnloadEvent) => { if (formState.dirty || formState.busy) { event.preventDefault(); event.returnValue = ''; } }; window.addEventListener('beforeunload', warn); return () => window.removeEventListener('beforeunload', warn); }, [formState]);
  const choose = async (id: number, mode: 'address' | 'shipping') => { if (formState.busy || active?.id !== id && formState.dirty && !await confirmDiscard('地址修改尚未保存，确定切换批次吗？')) return; setActive({ id, mode }); };
  return <section aria-labelledby="member-collections-title" className="space-y-5"><div><h2 id="member-collections-title" className="text-xl font-semibold">我的礼物与物流</h2><p className="mt-2 text-sm leading-7 text-muted-foreground">这里显示你在领取名单中的已发布批次，可直接登记地址或查看寄送进度。</p></div>
    {loading ? <p role="status" className="py-8 text-sm text-muted-foreground">正在读取领取记录…</p> : error ? <div role="alert" className="space-y-3"><p className="text-sm text-destructive">{error}</p><Button variant="outline" onClick={() => setAttempt(value => value + 1)}>重新读取</Button></div> : !rows.length ? <p className="py-8 text-sm leading-7 text-muted-foreground">暂时没有可查看的领取批次。可以先在地址簿保存地址，之后再留意主播的收集通知。</p> : <div className="divide-y divide-border border-y border-border">{rows.map(batch => <article key={batch.id} className="py-6"><div className="flex flex-col items-start gap-4 lg:flex-row lg:justify-between"><div className="w-full min-w-0 lg:flex-1"><h3 className="break-words text-lg font-semibold">{batch.title}</h3><p className="mt-2 text-sm text-muted-foreground">{batch.collecting ? '正在收集地址' : !batch.closed && batch.opens && batch.opens > new Date().toLocaleDateString('sv-SE', { timeZone: 'Asia/Shanghai' }) ? '尚未开始收集' : '收集已结束，可查询记录'}</p></div><div className="flex flex-wrap gap-2">{batch.collecting && <Button size="sm" disabled={formState.busy} onClick={() => void choose(batch.id, 'address')}>填写地址</Button>}<Button size="sm" variant="outline" disabled={formState.busy} onClick={() => void choose(batch.id, 'shipping')}>查看领取与物流</Button></div></div>{batch.note && <p className="mt-4 whitespace-pre-wrap break-words text-sm leading-7">{batch.note}</p>}{active?.id === batch.id && <VoyageClaim key={`${uid}:${batch.id}`} issueID={batch.id} memberUID={uid} mode={active.mode} onModeChange={mode => void choose(batch.id, mode)} onClose={() => setActive(null)} onStateChange={setFormState} />}</article>)}</div>}
  </section>;
}
