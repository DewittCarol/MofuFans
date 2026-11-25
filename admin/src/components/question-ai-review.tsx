"use client";
import {useEffect,useState} from "react";
import type {Question} from "@/lib/api";
import {aiAPI} from "@/lib/ai-api";
import {Button} from "@/components/ui/button";

const labels:Record<string,string>={pending:"等待 AI 审核",running:"AI 审核中",passed:"AI 未发现明显问题",blocked:"AI 建议拦截",failed:"AI 审核失败",skipped:"AI 未开启",manual:"已由人工处理"};
type QueueResult={queued:number;skipped:number;failed_ids:number[];has_more:boolean};
export function reviewLabel(q:Question){return labels[q.moderation?.status||""]||"尚未 AI 审核";}
export function QuestionAIReview({question:q,onRefresh,disabled=false}:{question:Question;onRefresh:()=>void;disabled?:boolean}){
 const [busy,setBusy]=useState(false),[error,setError]=useState("");
 const r=q.moderation;
 const queued=r?.status==="pending"||r?.status==="running";
 const retry=async()=>{
  setBusy(true);setError("");
  try{const data=await aiAPI<QueueResult>("moderation/queue","POST",{ids:[q.id]});if(data.failed_ids.length)throw new Error("加入队列失败，请稍后重试");onRefresh();}
  catch(e){setError(e instanceof Error?e.message:"审核请求失败，请重试");}finally{setBusy(false);}
 };
 return <div className="space-y-2 border-t border-border/60 pt-3 text-sm" aria-label={`投稿 ${q.id} 的 AI 审核`}>
  <div className="flex flex-wrap items-center justify-between gap-2"><strong className="font-medium">{reviewLabel(q)}</strong><Button size="sm" variant="outline" disabled={disabled||busy||queued} onClick={()=>void retry()}>{busy?"正在排队…":queued?"等待结果…":r&&r.status!=="skipped"?"重新审核":"AI 审核此条"}</Button></div>
  {r?.reason&&<p className="whitespace-pre-wrap break-words [overflow-wrap:anywhere]">{r.reason}</p>}
  {r?.error&&<p className="break-words text-destructive">{r.error}{r.status==="failed"?"。审核失败不代表内容安全，仍需人工检查。":""}</p>}
  {q.images_num>0&&<p className="text-muted-foreground">{r?.image_status==="complete"&&r.status!=="failed"?`本次已检查 ${r.images_checked} 张图片（缩小副本，小字仍需复核）`:r?.image_status==="disabled"?"本次仅审文字，附图未审。":queued?"图片审核状态将在完成后显示。":"附图未完成审核，请人工查看。"}</p>}
  {(r?.decision_locked||q.is_publish||q.is_archive)&&<p className="text-muted-foreground">保留人工决定及公开、归档状态；复审只提供建议。</p>}
  {r?.checked_at&&<p className="text-xs text-muted-foreground">{r.model||"AI"} · {new Date(r.checked_at).toLocaleString("zh-CN")}</p>}
  {error&&<p role="alert" className="text-destructive">{error}</p>}
 </div>;
}
export function ReviewBatchToolbar({ids,tagId,onRefresh,disabled=false}:{ids:number[];tagId:number;onRefresh:()=>void;disabled?:boolean}){
 const [status,setStatus]=useState<{enabled:boolean;review_images:boolean}|null>(null);
 const [busy,setBusy]=useState(""),[message,setMessage]=useState("");
 useEffect(()=>{let active=true;aiAPI<{enabled:boolean;review_images:boolean}>("moderation/status").then(v=>{if(active)setStatus(v)}).catch(()=>{if(active)setMessage("暂时无法读取 AI 状态，可刷新页面重试")});return()=>{active=false};},[]);
 const queue=async(scope:"page"|"topic")=>{
  setBusy(scope);setMessage("");
  try{const data=await aiAPI<QueueResult>("moderation/queue","POST",scope==="page"?{ids}:{tag_id:tagId,scope:"unreviewed"});setMessage(`已加入 ${data.queued} 条，跳过 ${data.skipped} 条${data.failed_ids.length?`，${data.failed_ids.length} 条未能入队，请重试`:""}${data.has_more?"。本话题还有未审投稿，本批完成后可继续补审":""}。`);onRefresh();}
  catch(e){setMessage(e instanceof Error?e.message:"补审请求失败，请重试");}finally{setBusy("");}
 };
 return <details className="rounded-lg border border-border px-4 py-3">
  <summary className="cursor-pointer text-sm font-medium">AI 审核与历史补审{status?status.enabled?` · ${status.review_images?"文字与图片":"仅文字"}`:" · 尚未开启":""}</summary>
  <div className="mt-3 space-y-3 text-sm">
   <p className="text-muted-foreground">新投稿自动排队；历史补审按已保存的设置调用模型，会产生 API 用量。失败保留投稿，人工操作始终优先。</p>
   {!status?.enabled?<p>先到<a className="text-primary underline underline-offset-4" href="/admin/dashboard/settings/#ai-settings">系统设置 → AI 辅助审核</a>配置并开启。</p>:<div className="flex flex-wrap gap-2">
    <Button variant="outline" size="sm" disabled={disabled||!!busy||!ids.length} onClick={()=>void queue("page")}>{busy==="page"?"正在加入队列…":`审核当前页 ${ids.length} 条`}</Button>
    <Button variant="outline" size="sm" disabled={disabled||!!busy||!tagId} onClick={()=>void queue("topic")}>{busy==="topic"?"正在加入队列…":"补审所选话题未审稿件（最多 100 条）"}</Button>
    <Button variant="ghost" size="sm" disabled={disabled||!!busy} onClick={onRefresh}>刷新审核结果</Button>
   </div>}
   <p className="text-xs text-muted-foreground">话题补审包含未审核、停用期间收到和失败的稿件，跳过已公开、已归档及人工处理的稿件。先在上方选择一个话题；结果可用“AI 审核状态”筛选。</p>
   {message&&<p role="status">{message}</p>}
  </div>
 </details>;
}
