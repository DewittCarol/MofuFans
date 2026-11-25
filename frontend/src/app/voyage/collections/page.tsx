'use client';
import {confirmDiscard} from '@/lib/site-dialog';
import {Suspense,useEffect,useState} from 'react';
import Link from 'next/link';
import {useSearchParams} from 'next/navigation';
import {Truck} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {VoyageClaim} from '@/components/voyage-claim';
import {voyageRequest} from '@/lib/voyage';
import {useAccountAuth} from '@/lib/account-auth';
interface Batch {id:number;title:string;note:string;opens:string;closes:string;collecting:boolean;closed:boolean}
function Collections(){
 const {user,accountsEnabled,authRevision,openAccountDialog}=useAccountAuth();
 const memberUID=user?.signature_verified?user.bilibili_uid:undefined;
 const [rows,setRows]=useState<Batch[]>([]),[loading,setLoading]=useState(true),[error,setError]=useState(''),[attempt,setAttempt]=useState(0),[active,setActive]=useState<{id:number;mode:'address'|'shipping'}|null>(null),[notice,setNotice]=useState('');
 const [formState,setFormState]=useState({busy:false,dirty:false});
 const choose=async(id:number,mode:'address'|'shipping')=>{if(formState.busy)return;if(active?.id!==id&&formState.dirty&&!await confirmDiscard('地址修改尚未保存，确定切换批次吗？'))return;setActive({id,mode})};
 useEffect(()=>{const warn=(e:BeforeUnloadEvent)=>{if(formState.busy||formState.dirty){e.preventDefault();e.returnValue=''}};window.addEventListener('beforeunload',warn);return()=>window.removeEventListener('beforeunload',warn)},[formState]);
 const params=useSearchParams(),requested=params.get('batch');
 useEffect(()=>{let live=true;setLoading(true);setError('');voyageRequest<Batch[]>('/api/voyage/collections').then(data=>{if(live)setRows(data)}).catch(e=>{if(live)setError(e.message)}).finally(()=>{if(live)setLoading(false)});return()=>{live=false}},[attempt]);
 useEffect(()=>{if(loading||error||!requested)return;const raw=requested||'';if(!/^[1-9]\d*$/.test(raw)){setNotice('批次链接无效，请从下方选择。');return}const id=Number(raw),target=rows.find(r=>r.id===id);if(target){setActive({id:target.id,mode:target.collecting?'address':'shipping'});setNotice('')}else setNotice('这个批次暂不可查看，可能尚未开放。')},[requested,rows,loading,error]);
 useEffect(()=>{if(!active)return;const frame=requestAnimationFrame(()=>document.getElementById(`collection-${active.id}`)?.scrollIntoView({block:'start',behavior:'auto'}));return()=>cancelAnimationFrame(frame)},[active?.id]);
 return <main className="mx-auto min-h-[calc(100svh-56px)] w-full max-w-5xl px-5 pb-16 pt-10 sm:px-8 sm:pt-14"><header className="mb-6 flex items-center gap-3"><Truck size={28} strokeWidth={1.5} className="text-primary"/><h1 className="text-3xl font-semibold">地址与物流</h1></header><nav className="mb-6 flex gap-6 border-b border-border text-sm" aria-label="大航海页面"><Link className="min-h-11 py-3 text-muted-foreground hover:text-primary" href="/voyage/">礼物详情</Link><span aria-current="page" className="border-b-2 border-primary py-3 font-medium text-primary">地址与物流</span></nav><p className="mb-8 max-w-2xl text-sm leading-7 text-muted-foreground">登录并完成签名验证后，可直接查看本人的领取内容、登记地址和查询物流。此前收到的私信认证码仍可使用。</p>{accountsEnabled&&<div className="mb-6 flex flex-wrap items-center gap-3 text-sm">{memberUID?<><span>当前登录 UID {memberUID}</span><Link href="/account/" className="text-primary underline underline-offset-4">我的地址簿与领取记录</Link></>:<><Button size="sm" onClick={()=>openAccountDialog(user?'upgrade':'login')}>{user?'完成签名验证':'登录后直接查看'}</Button><span className="text-muted-foreground">也可以在下方使用原私信认证码。</span></>}</div>}
 {notice&&<p role="status" className="mb-5 text-sm text-muted-foreground">{notice}</p>}{loading?<p role="status" className="py-10">正在加载收集批次…</p>:error?<div role="alert"><p className="text-sm text-destructive">{error}</p><Button className="mt-3" variant="outline" onClick={()=>setAttempt(n=>n+1)}>重新加载</Button></div>:!rows.length?<p className="py-12 text-sm text-muted-foreground">暂时没有开放的收集批次，请留意主播通知。</p>:<div className="divide-y divide-border border-y border-border">{rows.map(batch=><article id={`collection-${batch.id}`} key={batch.id} className="scroll-mt-24 py-6"><div className="flex flex-wrap items-start justify-between gap-4"><div className="min-w-0 flex-1"><h2 className="break-words text-xl font-semibold">{batch.title}</h2><p className="mt-2 text-sm text-muted-foreground">{batch.collecting?'正在收集地址':!batch.closed&&batch.opens&&batch.opens>new Date().toLocaleDateString('sv-SE',{timeZone:'Asia/Shanghai'})?'尚未开始收集':'收集已结束，可查询记录'}{batch.opens||batch.closes?` · ${batch.opens||'即日起'} 至 ${batch.closes||'另行通知'}`:''}</p></div><div className="flex flex-wrap gap-2">{batch.collecting&&<Button size="sm" disabled={formState.busy} onClick={()=>choose(batch.id,'address')}>填写地址</Button>}<Button variant="outline" size="sm" disabled={formState.busy} onClick={()=>choose(batch.id,'shipping')}>查看领取内容与物流</Button></div></div>{batch.note&&<p className="mt-4 max-w-3xl whitespace-pre-wrap break-words text-sm leading-7">{batch.note}</p>}{active?.id===batch.id&&<VoyageClaim key={`${batch.id}:${user?.bilibili_uid||'guest'}:${authRevision}`} memberUID={memberUID} issueID={batch.id} mode={active.mode} onModeChange={mode=>choose(batch.id,mode)} onStateChange={setFormState} onClose={()=>setActive(null)}/>}</article>)}</div>}
 </main>
}
export default function Page(){return <Suspense fallback={<p className="p-8">正在加载…</p>}><Collections/></Suspense>}
