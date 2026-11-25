'use client';
import {confirmDiscard} from '@/lib/site-dialog';

import { useEffect, useState } from 'react';
import { useSidebarNavigationGuard } from '@/lib/use-sidebar-navigation-guard';
import { Gift, Plus } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { GiftEditor } from '@/components/digital-gift-editor';
import { giftAPI, type DigitalGift } from '@/lib/digital-gifts';

export default function GiftsPage() {
  const [gifts, setGifts] = useState<DigitalGift[]>([]), [selected, setSelected] = useState<number | null>(null), [error, setError] = useState(''), [loading, setLoading] = useState(true);
  const [editorState, setEditorState] = useState({ busy: false, dirty: false });
  async function choose(id: number) {
    if (editorState.busy || id === selected) return;
    if (editorState.dirty && !await confirmDiscard('还有未保存的内容或私信草稿，确定放弃并切换礼物吗？')) return;
    setEditorState({ busy: false, dirty: false }); setSelected(id);
  }
  useEffect(() => {
    if (!editorState.dirty && !editorState.busy) return;
    const beforeUnload = (e: BeforeUnloadEvent) => { e.preventDefault(); e.returnValue = ''; };
    window.addEventListener('beforeunload', beforeUnload);
    return () => { window.removeEventListener('beforeunload', beforeUnload); };
  }, [editorState]);
  useSidebarNavigationGuard(() => !editorState.busy && (!editorState.dirty || confirmDiscard('还有未保存的内容或私信草稿，确定离开吗？')), 'all');
  async function reload() { setError(''); try { setGifts(await giftAPI<DigitalGift[]>('')); } catch (e) { setError((e as Error).message); } finally { setLoading(false); } }
  useEffect(() => { void reload(); }, []);
  return <div className="mx-auto w-full max-w-7xl space-y-6">
    <header className="flex flex-wrap items-center justify-between gap-4"><div><h1 className="flex items-center gap-2 text-2xl font-semibold"><Gift className="text-primary" size={25} />电子礼物</h1><p className="mt-2 text-sm text-muted-foreground">上传文件，选择领取方式，上架后观众就可以到网站领取。</p></div><Button disabled={editorState.busy} onClick={() => choose(0)}><Plus size={17} />新建礼物</Button></header>
    {error && <div role="alert" className="flex flex-wrap items-center gap-3 text-sm text-destructive">{error}<Button size="sm" variant="outline" onClick={() => void reload()}>重新加载</Button></div>}
    <div className="grid items-start gap-6 xl:grid-cols-[230px_minmax(0,1fr)]">
      <nav aria-label="礼物列表" className="space-y-2">{loading ? <p role="status" className="py-6 text-sm text-muted-foreground">正在加载…</p> : gifts.map(gift => <button key={gift.id} type="button" aria-current={selected === gift.id ? 'true' : undefined} className={`w-full rounded-lg border p-4 text-left focus-visible:outline-2 focus-visible:outline-ring ${selected === gift.id ? 'border-primary bg-secondary' : 'border-border bg-card hover:bg-secondary/50'}`} disabled={editorState.busy} onClick={() => choose(gift.id)}><span className="block break-words font-medium">{gift.title}</span><span className="mt-2 block text-xs text-muted-foreground">{gift.published ? '已上架' : '草稿'} · {gift.mode === 'open' ? '无密码' : gift.mode === 'shared' ? '统一密码' : '独立密码'}</span></button>)}</nav>
      {selected !== null ? <GiftEditor key={selected} id={selected} onStateChange={setEditorState} onSaved={id => { setSelected(id); void reload(); }} onDeleted={() => { setEditorState({ busy: false, dirty: false }); setSelected(null); void reload(); }} /> : <section className="py-14 text-center"><h2 className="text-lg font-medium">{gifts.length ? '选择一份礼物开始管理' : '还没有电子礼物'}</h2><p className="mx-auto mt-3 max-w-md text-sm leading-6 text-muted-foreground">可以公开发放、设置统一密码，或关联已有名单，为每个人生成独立密码。</p><Button variant="outline" className="mt-6" onClick={() => setSelected(0)}>新建礼物</Button></section>}
    </div>
  </div>;
}
