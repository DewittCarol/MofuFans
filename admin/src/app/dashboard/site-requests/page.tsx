"use client";
import {confirmAction} from '@/lib/site-dialog';

import { ChangeEvent, DragEvent, useCallback, useEffect, useRef, useState } from "react";
import {
  CheckCircle2, Download, FileBox, FileUp, ListTodo, LoaderCircle,
  Pencil, RotateCcw, Save, Send, ShieldCheck, Trash2, X,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useAuth } from "@/lib/auth";
import {
  createWebsiteAsset, deleteWebsiteAsset, getWebsiteAssets, getWebsiteAssetSettings,
  updateWebsiteAsset, updateWebsiteAssetSettings, type WebsiteAssetList,
  type WebsiteAssetRequest, type WebsiteAssetStatus,
} from "@/lib/api";
import { cn } from "@/lib/utils";

const MAX_FILES = 8;
const MAX_TOTAL_BYTES = 512 * 1024 * 1024;
const fallbackInstructions = "想加新功能、反馈 Bug、调整使用体验，或者上传新内容，都可以在这里发布。说说你想怎么改；如果遇到问题，写下出现问题的页面和操作步骤。截图、素材等附件按需添加，不上传文件也可以发布。";
type Filter = WebsiteAssetStatus | "all";

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).format(new Date(value));
}

export default function RequirementsPage() {
  const { user } = useAuth();
  const isSuperAdmin = Boolean(user?.is_super_admin);
  const inputRef = useRef<HTMLInputElement>(null);
  const [filter, setFilter] = useState<Filter>("pending");
  const [list, setList] = useState<WebsiteAssetList>({ items: [], total: 0, pending_count: 0, completed_count: 0 });
  const [loading, setLoading] = useState(true);
  const [description, setDescription] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [dragging, setDragging] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [notice, setNotice] = useState<{ tone: "success" | "error"; text: string } | null>(null);
  const [instructions, setInstructions] = useState(fallbackInstructions);
  const [instructionDraft, setInstructionDraft] = useState(fallbackInstructions);
  const [editingInstructions, setEditingInstructions] = useState(false);
  const [savingInstructions, setSavingInstructions] = useState(false);
  const [editingRequest, setEditingRequest] = useState<number | null>(null);
  const [requestDraft, setRequestDraft] = useState("");
  const [busyRequest, setBusyRequest] = useState<number | null>(null);

  const load = useCallback(async (nextFilter: Filter) => {
    setLoading(true);
    try {
      const response = await getWebsiteAssets(nextFilter);
      if (response.code === 200) setList(response.data);
      else setNotice({ tone: "error", text: response.message || "需求记录读取失败" });
    } catch {
      setNotice({ tone: "error", text: "需求记录读取失败，请检查网络后重试" });
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void load(filter); }, [filter, load]);
  useEffect(() => {
    void getWebsiteAssetSettings().then((response) => {
      if (response.code === 200) {
        setInstructions(response.data.instructions);
        setInstructionDraft(response.data.instructions);
      }
    }).catch(() => undefined);
  }, []);

  const addFiles = (incoming: File[]) => {
    if (uploading) return;
    setNotice(null);
    const merged = [...files];
    for (const file of incoming) {
      const duplicate = merged.some((current) => current.name === file.name && current.size === file.size && current.lastModified === file.lastModified);
      if (!duplicate) merged.push(file);
    }
    if (merged.length > MAX_FILES) {
      setNotice({ tone: "error", text: "每次最多上传 8 个文件" });
      return;
    }
    if (merged.some((file) => file.size < 1)) {
      setNotice({ tone: "error", text: "不能上传空文件" });
      return;
    }
    if (merged.reduce((sum, file) => sum + file.size, 0) > MAX_TOTAL_BYTES) {
      setNotice({ tone: "error", text: "单次上传总大小不能超过 512 MB" });
      return;
    }
    setFiles(merged);
  };

  const onFileChange = (event: ChangeEvent<HTMLInputElement>) => {
    addFiles(Array.from(event.target.files ?? []));
    event.target.value = "";
  };

  const onDrop = (event: DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    setDragging(false);
    addFiles(Array.from(event.dataTransfer.files));
  };

  const submit = async () => {
    const value = description.trim();
    if (!value) {
      setNotice({ tone: "error", text: "请先填写你想添加、反馈或调整的内容" });
      return;
    }
    setUploading(true);
    setUploadProgress(0);
    setNotice(null);
    try {
      const response = await createWebsiteAsset(value, files, setUploadProgress);
      if (response.code !== 200) {
        setNotice({ tone: "error", text: response.message || "发布失败" });
        return;
      }
      setDescription("");
      setFiles([]);
      setFilter("pending");
      setNotice({ tone: "success", text: "已发布，可以在待处理中查看" });
      await load("pending");
    } catch (error) {
      setNotice({ tone: "error", text: error instanceof Error ? error.message : "发布失败，请重试" });
    } finally {
      setUploading(false);
      setUploadProgress(0);
    }
  };

  const saveInstructions = async () => {
    if (!instructionDraft.trim()) return;
    setSavingInstructions(true);
    try {
      const response = await updateWebsiteAssetSettings(instructionDraft.trim());
      if (response.code === 200) {
        setInstructions(response.data.instructions);
        setInstructionDraft(response.data.instructions);
        setEditingInstructions(false);
        setNotice({ tone: "success", text: "页面说明已保存" });
      } else setNotice({ tone: "error", text: response.message || "页面说明保存失败" });
    } catch {
      setNotice({ tone: "error", text: "页面说明保存失败，请重试" });
    } finally {
      setSavingInstructions(false);
    }
  };

  const saveDescription = async (request: WebsiteAssetRequest) => {
    if (!requestDraft.trim()) return;
    setBusyRequest(request.id);
    try {
      const response = await updateWebsiteAsset(request.id, { description: requestDraft.trim() });
      if (response.code === 200) {
        setEditingRequest(null);
        setNotice({ tone: "success", text: "需求内容已更新" });
        await load(filter);
      } else setNotice({ tone: "error", text: response.message || "需求内容保存失败" });
    } catch {
      setNotice({ tone: "error", text: "需求内容保存失败，请重试" });
    } finally {
      setBusyRequest(null);
    }
  };

  const changeStatus = async (request: WebsiteAssetRequest) => {
    const nextStatus: WebsiteAssetStatus = request.status === "pending" ? "completed" : "pending";
    setBusyRequest(request.id);
    try {
      const response = await updateWebsiteAsset(request.id, { status: nextStatus });
      if (response.code === 200) {
        setNotice({ tone: "success", text: nextStatus === "completed" ? "已标记为完成" : "已重新放回待处理" });
        await load(filter);
      } else setNotice({ tone: "error", text: response.message || "完成情况保存失败" });
    } catch {
      setNotice({ tone: "error", text: "完成情况保存失败，请重试" });
    } finally {
      setBusyRequest(null);
    }
  };

  const removeRequest = async (request: WebsiteAssetRequest) => {
    if (!(await confirmAction(`将删除“${request.description.slice(0, 30)}”${request.files.length ? "及其全部附件" : ""}，此操作无法恢复。`, { title: "删除需求？", confirmLabel: "删除需求", destructive: true }))) return;
    setBusyRequest(request.id);
    try {
      const response = await deleteWebsiteAsset(request.id);
      if (response.code === 200) {
        setNotice({ tone: "success", text: request.files.length ? "需求记录和附件已删除" : "需求记录已删除" });
        await load(filter);
      } else setNotice({ tone: "error", text: response.message || "删除失败" });
    } catch {
      setNotice({ tone: "error", text: "删除失败，请重试" });
    } finally {
      setBusyRequest(null);
    }
  };

  const totalBytes = files.reduce((sum, file) => sum + file.size, 0);
  const filters: { value: Filter; label: string; count: number }[] = [
    { value: "pending", label: "待处理", count: list.pending_count },
    { value: "completed", label: "已完成", count: list.completed_count },
    { value: "all", label: "全部", count: list.pending_count + list.completed_count },
  ];

  return (
    <div className="space-y-8">
      <header className="max-w-3xl space-y-2">
        <div className="flex items-center gap-3"><ListTodo className="h-7 w-7 text-primary" aria-hidden="true" /><h1 className="text-2xl font-semibold tracking-tight text-foreground">需求管理</h1></div>
        <p className="text-sm leading-6 text-muted-foreground">记录站点改进建议、问题和素材交接，便于管理员跟踪处理。内容仅在后台可见。</p>
      </header>

      {notice && <div role="status" aria-live="polite" className={cn("rounded-lg border px-4 py-3 text-sm", notice.tone === "success" ? "border-[var(--success)]/30 bg-[var(--success)]/10 text-[var(--success)]" : "border-destructive/30 bg-destructive/10 text-destructive")}>{notice.text}</div>}

      <section className="grid items-start gap-6 lg:grid-cols-[minmax(0,1.35fr)_minmax(18rem,.65fr)]">
        <Card>
          <CardHeader className="gap-1"><CardTitle>发布需求</CardTitle><p className="text-sm leading-6 text-muted-foreground">新功能、Bug 反馈、体验优化、新增内容都可以。写下具体内容即可发布，附件选填。</p></CardHeader>
          <CardContent className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="asset-description">需求内容</Label>
              <Textarea id="asset-description" rows={5} maxLength={4000} value={description} onChange={(event) => setDescription(event.target.value)} placeholder={"想添加什么，或者哪里需要调整？\n例如：歌单想加一个筛选功能；手机上点不开菜单；这个按钮不太好找；想把附件里的新桌宠加到网站。"} disabled={uploading} />
              <div className="text-right text-xs tabular-nums text-muted-foreground">{description.length} / 4000</div>
            </div>

            <div
              role="button" tabIndex={uploading ? -1 : 0} aria-label="添加附件（选填）" aria-disabled={uploading}
              onClick={() => inputRef.current?.click()}
              onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); inputRef.current?.click(); } }}
              onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
              onDragOver={(event) => event.preventDefault()}
              onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setDragging(false); }}
              onDrop={onDrop}
              className={cn("flex min-h-36 cursor-pointer flex-col items-center justify-center rounded-xl border-2 border-dashed px-5 py-7 text-center outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/40", dragging ? "border-primary bg-primary/10" : "border-border bg-muted/35 hover:border-primary/50 hover:bg-muted/60")}
            >
              <FileUp className="mb-3 h-7 w-7 text-primary" aria-hidden="true" />
              <p className="font-medium text-foreground">添加附件（选填）</p>
              <p className="mt-1 text-sm text-muted-foreground">拖放文件到这里，或点击选择</p>
              <p className="mt-2 text-xs leading-5 text-muted-foreground">截图、素材、压缩包等任意格式，每次最多 8 个文件，共 512 MB。</p>
              <input ref={inputRef} type="file" multiple className="sr-only" onChange={onFileChange} disabled={uploading} />
            </div>

            {files.length > 0 && <div className="divide-y divide-border rounded-xl border border-border bg-card/70">
              {files.map((file, index) => <div key={`${file.name}-${file.size}-${file.lastModified}`} className="flex items-center gap-3 px-3 py-2.5">
                <FileBox className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{file.name}</p><p className="text-xs tabular-nums text-muted-foreground">{formatBytes(file.size)}</p></div>
                <Button type="button" variant="ghost" size="icon-sm" aria-label={`移除 ${file.name}`} disabled={uploading} onClick={(event) => { event.stopPropagation(); setFiles((current) => current.filter((_, fileIndex) => fileIndex !== index)); }}><X /></Button>
              </div>)}
              <div className="flex justify-between px-3 py-2 text-xs tabular-nums text-muted-foreground"><span>{files.length} / 8 个文件</span><span>{formatBytes(totalBytes)} / 512 MB</span></div>
            </div>}

            {uploading && files.length > 0 && <div className="space-y-2" aria-live="polite"><div className="flex items-center justify-between text-sm"><span>{uploadProgress < 100 ? "正在上传附件，请不要关闭页面" : "附件已上传，正在保存需求"}</span><span className="tabular-nums">{uploadProgress}%</span></div><div className="h-2 overflow-hidden rounded-full bg-muted"><div className="h-full bg-primary transition-[width]" style={{ width: `${uploadProgress}%` }} /></div></div>}

            <Button className="w-full sm:w-auto" onClick={() => void submit()} disabled={uploading || !description.trim()}>
              {uploading ? <><LoaderCircle className="animate-spin" />正在发布</> : <><Send />发布需求</>}
            </Button>
          </CardContent>
        </Card>

        <Card className="gap-4">
          <CardHeader className="flex-row items-start justify-between gap-4">
            <CardTitle>发布说明</CardTitle>
            {isSuperAdmin && !editingInstructions && <Button variant="ghost" size="sm" onClick={() => setEditingInstructions(true)}><Pencil />编辑</Button>}
          </CardHeader>
          <CardContent>
            {editingInstructions ? <div className="space-y-3">
              <Textarea aria-label="发布说明" rows={8} maxLength={4000} value={instructionDraft} onChange={(event) => setInstructionDraft(event.target.value)} />
              <div className="flex flex-wrap gap-2"><Button size="sm" onClick={() => void saveInstructions()} disabled={savingInstructions || !instructionDraft.trim()}>{savingInstructions ? <LoaderCircle className="animate-spin" /> : <Save />}保存说明</Button><Button size="sm" variant="ghost" disabled={savingInstructions} onClick={() => { setInstructionDraft(instructions); setEditingInstructions(false); }}>取消</Button></div>
            </div> : <p className="whitespace-pre-wrap text-sm leading-7 text-foreground/85">{instructions}</p>}
            <div className="mt-5 flex items-start gap-2 border-t border-border pt-4 text-xs leading-5 text-muted-foreground"><ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" /><span>发布的内容和附件仅在后台可见，不会直接展示到网站前台。</span></div>
          </CardContent>
        </Card>
      </section>

      <section className="space-y-4" aria-labelledby="asset-queue-title">
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div><h2 id="asset-queue-title" className="text-xl font-semibold tracking-tight">发布记录</h2><p className="mt-1 text-sm text-muted-foreground">在这里查看处理进度，已完成的记录也会保留。</p></div>
          <div className="flex rounded-lg bg-muted p-1" role="tablist" aria-label="按处理状态筛选需求">
            {filters.map((item) => <button key={item.value} type="button" role="tab" aria-selected={filter === item.value} onClick={() => setFilter(item.value)} className={cn("rounded-md px-3 py-1.5 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring", filter === item.value ? "bg-card text-foreground shadow-sm" : "text-muted-foreground hover:text-foreground")}>{item.label}<span className="ml-1.5 tabular-nums">{item.count}</span></button>)}
          </div>
        </div>

        <Card className="gap-0 py-0">
          {loading ? <div className="flex min-h-40 items-center justify-center gap-2 text-sm text-muted-foreground"><LoaderCircle className="h-4 w-4 animate-spin" />正在读取需求记录</div> : list.items.length === 0 ? (
            <div className="flex min-h-48 flex-col items-center justify-center px-6 text-center"><ListTodo className="mb-3 h-8 w-8 text-muted-foreground/60" aria-hidden="true" /><p className="font-medium">{filter === "pending" ? "暂时没有待处理的需求" : filter === "completed" ? "还没有已完成的需求" : "还没有发布记录"}</p><p className="mt-1 text-sm text-muted-foreground">{filter === "completed" ? "处理完成后，记录会显示在这里。" : "有新想法、遇到问题或想上传内容，都可以在上方发布。"}</p></div>
          ) : <div className="divide-y divide-border">
            {list.items.map((request) => {
              const canEdit = isSuperAdmin || request.submitted_by_id === user?.id;
              const busy = busyRequest === request.id;
              return <article key={request.id} className="px-4 py-5 sm:px-6">
                <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
                  <div className="min-w-0 flex-1">
                    <div className="mb-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 font-medium", request.status === "completed" ? "bg-[var(--success)]/12 text-[var(--success)]" : "bg-primary/10 text-primary")}>{request.status === "completed" ? <CheckCircle2 className="h-3.5 w-3.5" /> : <span className="h-1.5 w-1.5 rounded-full bg-current" />}{request.status === "completed" ? "已完成" : "待处理"}</span>
                      <span>{request.submitted_by_name} 提交</span><span aria-hidden="true">·</span><time dateTime={request.created_at}>{formatDate(request.created_at)}</time>
                    </div>
                    {editingRequest === request.id ? <div className="space-y-3"><Textarea rows={4} maxLength={4000} value={requestDraft} onChange={(event) => setRequestDraft(event.target.value)} disabled={busy} /><div className="flex gap-2"><Button size="sm" disabled={busy || !requestDraft.trim()} onClick={() => void saveDescription(request)}>{busy ? <LoaderCircle className="animate-spin" /> : <Save />}保存</Button><Button size="sm" variant="ghost" disabled={busy} onClick={() => setEditingRequest(null)}>取消</Button></div></div> : <p className="whitespace-pre-wrap text-sm leading-7 text-foreground">{request.description}</p>}
                    {request.files.length > 0 && <div className="mt-4 grid gap-2 sm:grid-cols-2">
                      {request.files.map((file) => <a key={file.id} href={`/api/website-assets/files/${file.id}`} download={file.original_name} className="group flex min-w-0 items-center gap-3 rounded-lg border border-border bg-muted/35 px-3 py-2.5 outline-none transition-colors hover:bg-muted/70 focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/40"><FileBox className="h-4 w-4 shrink-0 text-primary" aria-hidden="true" /><span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{file.original_name}</span><span className="block text-xs tabular-nums text-muted-foreground">{formatBytes(file.size)}</span></span><Download className="h-4 w-4 shrink-0 text-muted-foreground transition-colors group-hover:text-foreground" aria-hidden="true" /></a>)}
                    </div>}
                    {request.status === "completed" && request.completed_at && <p className="mt-3 text-xs text-muted-foreground">{request.completed_by_name || "管理员"} 于 {formatDate(request.completed_at)} 标记完成</p>}
                  </div>
                  <div className="flex shrink-0 flex-wrap gap-2 sm:w-36 sm:flex-col">
                    {canEdit && editingRequest !== request.id && <Button variant="outline" size="sm" disabled={busy} onClick={() => { setEditingRequest(request.id); setRequestDraft(request.description); }}><Pencil />编辑内容</Button>}
                    {isSuperAdmin && <Button variant={request.status === "pending" ? "default" : "outline"} size="sm" disabled={busy} onClick={() => void changeStatus(request)}>{busy ? <LoaderCircle className="animate-spin" /> : request.status === "pending" ? <CheckCircle2 /> : <RotateCcw />}{request.status === "pending" ? "标记完成" : "重新处理"}</Button>}
                    {isSuperAdmin && <Button variant="ghost" size="sm" className="text-destructive hover:bg-destructive/10 hover:text-destructive" disabled={busy} onClick={() => void removeRequest(request)}><Trash2 />删除</Button>}
                  </div>
                </div>
              </article>;
            })}
          </div>}
        </Card>
      </section>
    </div>
  );
}
