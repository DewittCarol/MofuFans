"use client";

import { useState, useEffect, useCallback, useRef, Fragment } from "react";
import { QuestionAIReview, ReviewBatchToolbar, reviewLabel } from "@/components/question-ai-review";
import { QuestionJump } from "@/components/question-jump";
import { ModerationCard } from "@/components/moderation-card";
import { ModerationAttachments } from "@/components/moderation-attachments";
import { useModerationPreferences } from "@/lib/use-moderation-preferences";
import { useEmojiCatalog } from "@/lib/emoji-catalog";
import { renderQuestionContent } from "@/lib/question-content";
import { moderationUpdate } from "@/lib/moderation-update";
import { ImagePreview } from "@/components/image-preview";
import { attachmentUrls } from "@/lib/attachments";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  LayoutGrid, List, ShieldCheck,
  ChevronDown,
  ChevronUp,
  Inbox,
  MoreHorizontal,
  Trash2,
  TrashIcon,
} from "lucide-react";
import {
  getQuestions,
  getTags,
  updateQuestion,
  deleteQuestion,
  Question,
  Tag,
} from "@/lib/api";

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleString("zh-CN");
}

interface QuestionsPageContentProps {
  isSpam: boolean;
}

export function QuestionsPageContent({ isSpam }: QuestionsPageContentProps) {
  const protection = useModerationPreferences({ allowAdminInspection: true });
  const { emojis: catalog } = useEmojiCatalog();
  const renderContent = useCallback((content: string) => renderQuestionContent(content, catalog), [catalog]);
  const [reviewFilter,setReviewFilter]=useState("all");
  const [publishFilter, setPublishFilter] = useState("all");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pending, setPending] = useState<Set<number>>(new Set());
  const pendingRef = useRef(new Set<number>());
  const loadVersion = useRef(0);
  const request = useRef<AbortController | null>(null);
  const skipLoadedQuery = useRef<string | null>(null);
  const toolbarRef = useRef<HTMLElement>(null);
  const [jumpTarget, setJumpTarget] = useState<{ id: number } | null>(null);
  const [bounds, setBounds] = useState({ minId: 0, maxId: 0 });
  const [questions, setQuestions] = useState<Question[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [orderBy, setOrderBy] = useState("id");
  const [order, setOrder] = useState("desc");
  const [tagFilter, setTagFilter] = useState<string>("0");
  const [expandedRows, setExpandedRows] = useState<Set<number>>(new Set());
  const [deleteDialog, setDeleteDialog] = useState<{
    open: boolean;
    id: number;
  }>({ open: false, id: 0 });
  const [isLoading, setIsLoading] = useState(false);
  const [preview, setPreview] = useState<{ images: string[]; index: number } | null>(null);

  const loadTags = useCallback(async () => {
    try {
      const res = await getTags();
      if (res.code === 200 && res.data) {
        setTags(res.data);
      }
    } catch (error) {
      console.error("Failed to load tags:", error);
    }
  }, []);

  const loadQuestions = useCallback(async (locateId?: number, silent=false, background=false) => {
    // A slow poll must never overlap another request or interrupt an action.
    if (background && request.current) return;
    request.current?.abort();
    const controller = new AbortController();
    request.current = controller;
    const version = ++loadVersion.current;
    if(!silent) setIsLoading(true);
    try {
      const res = await getQuestions({
        page, locate_id: locateId,
        page_size: pageSize,
        order_by: orderBy,
        order,
        tag_id: tagFilter !== "0" ? parseInt(tagFilter) : undefined,
        is_spam: isSpam,
        moderation_status: reviewFilter==="all"?undefined:reviewFilter,
        publish: publishFilter === "all" ? undefined : publishFilter === "published",
      }, controller.signal);
      if (version !== loadVersion.current) { if (locateId) throw new Error("筛选条件已改变，请重新跳转。"); return; }
      if (res.code === 200) {
        const lastPage = Math.max(1, Math.ceil(res.data.total / pageSize));
        if (!locateId && page > lastPage) { setPage(lastPage); return; }
        setBounds(current => current.minId === res.data.min_id && current.maxId === res.data.max_id
          ? current : { minId: res.data.min_id, maxId: res.data.max_id });
        if (locateId) {
          if (res.data.page !== page) {
            skipLoadedQuery.current = JSON.stringify([res.data.page, pageSize, orderBy, order, tagFilter, isSpam, publishFilter, reviewFilter]);
            setPage(res.data.page);
          }
          setExpandedRows(previous => new Set(previous).add(locateId));
          setJumpTarget({ id: locateId });
          setNotice(`已跳到 #${locateId}`);
        }
        const incoming = res.data.questions || [];
        // Keep unchanged results stable instead of rebuilding every row.
        setQuestions(current => JSON.stringify(current) === JSON.stringify(incoming) ? current : incoming);
        setTotal(res.data.total);
        setError("");
      } else {
        if (locateId) throw new Error(res.code === 404 ? `当前筛选里没有 #${locateId}，请检查编号或切换筛选条件。` : '没能跳过去，请再试一次。');
        setError(res.message || "提问加载失败，请重试。");
      }
    } catch (error) {
      if (controller.signal.aborted) {
        if (locateId) throw new Error("筛选条件已改变，请重新跳转。");
        return;
      }
      if (locateId) throw error;
      if (version === loadVersion.current) setError("提问加载失败，请检查连接后重试。");
      console.error("Failed to load questions:", error);
    } finally {
      if (request.current === controller) request.current = null;
      if (version === loadVersion.current) setIsLoading(false);
    }
  }, [page, pageSize, orderBy, order, tagFilter, isSpam, publishFilter, reviewFilter]);

  useEffect(() => {
    loadTags();
  }, [loadTags]);

  useEffect(() => {
    const key = JSON.stringify([page, pageSize, orderBy, order, tagFilter, isSpam, publishFilter, reviewFilter]);
    if (skipLoadedQuery.current === key) { skipLoadedQuery.current = null; return; }
    skipLoadedQuery.current = null;
    setJumpTarget(null);
    void loadQuestions();
  }, [loadQuestions, page, pageSize, orderBy, order, tagFilter, isSpam, publishFilter, reviewFilter]);

  useEffect(() => () => { ++loadVersion.current; request.current?.abort(); }, []);
  const hasPendingReviews = questions.some(q => q.moderation?.status === "pending" || q.moderation?.status === "running");
  useEffect(() => {
    if (!hasPendingReviews) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const schedule = () => {
      clearTimeout(timer);
      if (!stopped && !document.hidden) timer = setTimeout(tick, 4000);
    };
    const tick = async () => {
      if (stopped || document.hidden) return;
      try {
        if (!pendingRef.current.size) await loadQuestions(undefined, true, true);
      } finally { schedule(); }
    };
    schedule();
    document.addEventListener('visibilitychange', schedule);
    return () => { stopped = true; clearTimeout(timer); document.removeEventListener('visibilitychange', schedule); };
  }, [hasPendingReviews, loadQuestions]);

  useEffect(() => {
    if (!jumpTarget || isLoading) return;
    const frame = requestAnimationFrame(() => {
      const element = document.getElementById(`admin-question-${jumpTarget.id}`);
      const toolbar = toolbarRef.current;
      const scroller = toolbar?.closest('main');
      if (!element || !toolbar || !scroller) return;
      element.focus({ preventScroll: true });
      scroller.scrollBy({ top: element.getBoundingClientRect().top - scroller.getBoundingClientRect().top - toolbar.offsetHeight - 24, behavior: 'instant' });
    });
    return () => cancelAnimationFrame(frame);
  }, [jumpTarget, isLoading, protection.preferences.view]);

  const toggleRow = (id: number) => {
    const newExpanded = new Set(expandedRows);
    if (newExpanded.has(id)) {
      newExpanded.delete(id);
    } else {
      newExpanded.add(id);
    }
    setExpandedRows(newExpanded);
  };

  const beginMutation = (id: number) => {
    if (pendingRef.current.has(id)) return false;
    pendingRef.current.add(id);
    setPending(new Set(pendingRef.current));
    setError(""); setNotice("");
    return true;
  };
  const finishMutation = (id: number) => {
    pendingRef.current.delete(id);
    setPending(new Set(pendingRef.current));
  };
  const saveQuestion = async (question: Question, patch: Partial<Question>, message: string) => {
    if (!beginMutation(question.id)) return;
    try {
      const next = moderationUpdate(question, patch);
      const res = await updateQuestion(question.id, next);
      if (res.code !== 200) throw new Error(res.message || "保存失败，请重试。");
      // Invalidate an older list response before applying this mutation.
      loadVersion.current++;
      const leavesFilter = next.is_spam !== isSpam
        || (publishFilter !== "all" && next.is_publish !== (publishFilter === "published"))
        || (tagFilter !== "0" && next.tag_id !== Number(tagFilter));
      setQuestions(current => leavesFilter ? current.filter(q => q.id !== question.id)
        : current.map(q => q.id === question.id ? { ...q, ...next } : q));
      setNotice(`#${question.id} ${message}`);
      await loadQuestions(undefined,true);
    } catch (error) {
      setError(error instanceof Error ? error.message : "保存失败，请重试。");
    } finally { finishMutation(question.id); }
  };
  const handleCheckboxChange = (question: Question, field: "is_rainbow" | "is_archive" | "is_publish", value: boolean) => {
    const labels = { is_rainbow: "彩虹标记已更新", is_archive: value ? "已归档" : "已取消归档", is_publish: value ? "已公开" : "已取消公开" };
    return saveQuestion(question, { [field]: value }, labels[field]);
  };
  const handleTagChange = (question: Question, tag_id: number) => saveQuestion(question, { tag_id }, "话题已更新");
  const handleMove = (question: Question) => saveQuestion(question, { is_spam: !isSpam }, isSpam ? "已移入收件箱" : "已移入拦截箱");
  const handleDelete = async () => {
    const id = deleteDialog.id;
    if (!beginMutation(id)) return;
    try {
      const res = await deleteQuestion(id);
      if (res.code !== 200) throw new Error(res.message || "删除失败，请重试。");
      setDeleteDialog({ open: false, id: 0 });
      setNotice(`#${id} 已删除`);
      await loadQuestions();
    } catch (error) { setError(error instanceof Error ? error.message : "删除失败，请重试。"); }
    finally { finishMutation(id); }
  };
  const openAttachment = (images: string[], index: number) => {
    const wasProtected = protection.isProtected(images[index]);
    protection.reveal(images[index]);
    if (!wasProtected) setPreview({ images, index });
  };

  const totalPages = Math.ceil(total / pageSize);

  return (
    <div className="space-y-8">
      {preview && <ImagePreview
        src={preview.images[preview.index]}
        images={preview.images}
        currentIndex={preview.index}
        protectedImage={protection.isProtected(preview.images[preview.index])}
        onReviewThumbnail={() => {
          protection.reveal(preview.images[preview.index]);
          setPreview(null);
        }}
        onIndexChange={index => {
          if (!protection.isProtected(preview.images[index])) protection.reveal(preview.images[index]);
          setPreview({ ...preview, index });
        }}
        onClose={() => setPreview(null)}
      />}
      <header>
        <h1 className="text-2xl font-semibold text-foreground tracking-tight">
          {isSpam ? "拦截箱" : "收件箱"}
        </h1>
      </header>

      <section className="space-y-3" aria-label="查看方式与附图保护">
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div className="inline-flex rounded-lg border border-border bg-card p-1" role="group" aria-label="提问视图">
            <Button variant={protection.preferences.view === "list" ? "secondary" : "ghost"} size="sm"
              disabled={!protection.ready} aria-pressed={protection.preferences.view === "list"}
              onClick={() => protection.savePreferences({ view: "list" })}><List size={16} />列表</Button>
            <Button variant={protection.preferences.view === "cards" ? "secondary" : "ghost"} size="sm"
              disabled={!protection.ready} aria-pressed={protection.preferences.view === "cards"}
              onClick={() => protection.savePreferences({ view: "cards" })}><LayoutGrid size={16} />卡片</Button>
          </div>
          {!protection.inspectionMode && <button type="button" role="switch" aria-checked={protection.preferences.blurEnabled}
            aria-label="模糊未查看的附图" aria-describedby="attachment-protection-hint" disabled={!protection.ready}
            onClick={() => protection.savePreferences({ blurEnabled: !protection.preferences.blurEnabled })}
            className="inline-flex min-h-10 items-center gap-2 rounded-md text-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring disabled:opacity-50">
            <ShieldCheck size={17} className="text-primary" />模糊未查看的附图
            <span className={`inline-flex h-6 w-10 items-center rounded-full p-0.5 ${protection.preferences.blurEnabled ? 'bg-primary' : 'bg-muted-foreground'}`}>
              <span className={`h-5 w-5 rounded-full bg-white transition-transform motion-reduce:transition-none ${protection.preferences.blurEnabled ? 'translate-x-4' : ''}`} />
            </span>
          </button>}
        </div>
        <p id="attachment-protection-hint" className="text-xs leading-relaxed text-muted-foreground">{protection.inspectionMode ? '当前账号直接查看附图，不记录查看状态。点击图片可放大。' : '点击一次去掉模糊，再次点击放大。'}</p>
        {protection.storageWarning && <p role="status" className="text-sm text-destructive">{protection.storageWarning}</p>}
      </section>

      {/* 筛选与排序工具栏 */}
      <section ref={toolbarRef} className="sticky top-2 z-20 rounded-xl border border-border bg-card p-3 sm:p-4 shadow-md">
        <div className="flex items-center gap-4 overflow-x-auto pb-2 sm:flex-wrap sm:gap-6 sm:overflow-visible [&>div]:shrink-0">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">状态</span>
            <Select disabled={pending.size > 0} value={publishFilter} onValueChange={value => { setPublishFilter(value); setPage(1); setNotice(""); }}>
              <SelectTrigger aria-label="公开状态筛选" className="w-[120px]"><SelectValue /></SelectTrigger>
              <SelectContent>
                <SelectItem value="all">全部状态</SelectItem>
                <SelectItem value="unpublished">未公开</SelectItem>
                <SelectItem value="published">已公开</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground whitespace-nowrap">
              话题
            </span>
            <Select
              disabled={pending.size > 0} value={tagFilter}
              onValueChange={(v) => {
                setTagFilter(v);
                setPage(1);
              }}
            >
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="选择话题" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="0">全部话题</SelectItem>
                {tags.map((tag) => (
                  <SelectItem key={tag.id} value={tag.id.toString()}>
                    {tag.tag_name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground whitespace-nowrap">
              排序
            </span>
            <Select
              disabled={pending.size > 0} value={orderBy}
              onValueChange={(v) => {
                setOrderBy(v);
                setPage(1);
              }}
            >
              <SelectTrigger className="w-[120px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="id">ID</SelectItem>
                <SelectItem value="created_at">时间</SelectItem>
                <SelectItem value="images_num">附图数</SelectItem>
                <SelectItem value="is_rainbow">彩虹</SelectItem>
                <SelectItem value="is_archive">归档</SelectItem>
                <SelectItem value="is_publish">公开</SelectItem>
              </SelectContent>
            </Select>
            <Select
              disabled={pending.size > 0} value={order}
              onValueChange={(v) => {
                setOrder(v);
                setPage(1);
              }}
            >
              <SelectTrigger className="w-[90px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="desc">降序</SelectItem>
                <SelectItem value="asc">升序</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium text-foreground whitespace-nowrap">
              每页
            </span>
            <Select
              disabled={pending.size > 0} value={pageSize.toString()}
              onValueChange={(v) => {
                setPageSize(parseInt(v));
                setPage(1);
              }}
            >
              <SelectTrigger className="w-[80px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="10">10</SelectItem>
                <SelectItem value="20">20</SelectItem>
                <SelectItem value="50">50</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <span className="text-sm text-muted-foreground ml-auto">
            共 {total} 条
          </span>
        </div>
        <div className="mt-2 flex justify-end border-t border-border pt-3">
          <QuestionJump key={`${orderBy}-${order}-${tagFilter}-${isSpam}-${publishFilter}-${pageSize}`} {...bounds} busy={isLoading || pending.size > 0} onJump={id => loadQuestions(id)} />
        </div>
      </section>

      <div className="flex flex-wrap items-center gap-3">
        <label htmlFor="review-filter" className="text-sm font-medium">AI 审核状态</label>
        <Select value={reviewFilter} onValueChange={v=>{setReviewFilter(v);setPage(1);}}><SelectTrigger id="review-filter" className="w-52"><SelectValue /></SelectTrigger><SelectContent>
          {Object.entries({all:"全部",unreviewed:"未审 / 未开启",pending:"等待审核",running:"审核中",passed:"AI 未发现明显问题",blocked:"AI 建议拦截",failed:"审核失败",manual:"已人工处理"}).map(([value,label])=><SelectItem key={value} value={value}>{label}</SelectItem>)}
        </SelectContent></Select>
      </div>
      <ReviewBatchToolbar ids={questions.map(q=>q.id)} tagId={Number(tagFilter)} onRefresh={()=>void loadQuestions(undefined,true)} disabled={isLoading||pending.size>0} />
      {error && <div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-destructive/30 bg-card px-4 py-3 text-sm text-destructive">
        <span>{error}</span><Button variant="outline" size="sm" disabled={pending.size > 0} onClick={() => void loadQuestions()}>重新加载</Button>
      </div>}
      <p role="status" aria-live="polite" className={notice ? "text-sm text-primary" : "sr-only"}>{notice}</p>

      {protection.preferences.view === "cards" ? <section aria-label="提问卡片" aria-busy={isLoading}>
        {isLoading ? <div className="grid gap-5">{[0, 1].map(i => <div key={i} className="h-64 animate-pulse rounded-xl border border-border bg-card motion-reduce:animate-none" />)}<span className="sr-only">加载中…</span></div>
        : questions.length === 0 ? <p className="rounded-xl border border-dashed border-border py-16 text-center text-sm text-muted-foreground">{publishFilter === "unpublished" ? "当前没有未公开的提问" : "没有符合条件的提问"}</p>
        : <div className="grid items-start gap-5">{questions.map(question => <div key={question.id} id={`admin-question-${question.id}`} tabIndex={-1}
          className={`rounded-xl outline-none ${jumpTarget?.id === question.id ? 'ring-2 ring-primary ring-offset-4 ring-offset-background' : ''}`}><ModerationCard
          onAIRefresh={()=>void loadQuestions(undefined,true)} question={question} tags={tags} isSpam={isSpam} busy={pending.has(question.id)}
          renderContent={renderContent} isProtected={protection.isProtected} onOpen={openAttachment}
          onChange={handleCheckboxChange} onTagChange={handleTagChange} onMove={handleMove}
          onDelete={id => setDeleteDialog({ open: true, id })} /></div>)}</div>}
      </section> : <section>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-[48px]"></TableHead>
              <TableHead className="w-[56px]">ID</TableHead>
              <TableHead className="w-[120px]">话题</TableHead>
              <TableHead className="min-w-[200px] max-w-[420px]">
                内容
              </TableHead>
              <TableHead className="w-[150px]">投稿者</TableHead>
              <TableHead className="w-[56px] text-center">附图</TableHead>
              <TableHead className="w-[56px] text-center">彩虹</TableHead>
              <TableHead className="w-[56px] text-center">归档</TableHead>
              <TableHead className="w-[56px] text-center">公开</TableHead>
              <TableHead className="w-[140px]">时间</TableHead>
              <TableHead className="w-[60px] text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell colSpan={11} className="text-center py-12">
                  加载中...
                </TableCell>
              </TableRow>
            ) : questions.length === 0 ? (
              <TableRow>
                <TableCell colSpan={11} className="text-center py-12">
                  暂无数据
                </TableCell>
              </TableRow>
            ) : (
              questions.map((question) => (
                <Fragment key={question.id}>
                  <TableRow id={`admin-question-${question.id}`} tabIndex={-1} className={jumpTarget?.id === question.id ? 'bg-accent outline-2 -outline-offset-2 outline-primary' : 'outline-none'}>
                    <TableCell className="whitespace-nowrap">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => toggleRow(question.id)}
                        title={
                          expandedRows.has(question.id) ? "收起" : "展开详情"
                        }
                      >
                        {expandedRows.has(question.id) ? (
                          <ChevronUp className="h-4 w-4" />
                        ) : (
                          <ChevronDown className="h-4 w-4" />
                        )}
                      </Button>
                    </TableCell>
                    <TableCell className="font-mono text-sm whitespace-nowrap">
                      {question.id}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      <Select
                        disabled={pending.has(question.id)}
                        value={question.tag_id.toString()}
                        onValueChange={(v) =>
                          handleTagChange(question, parseInt(v))
                        }
                      >
                        <SelectTrigger className="h-8 w-[100px] text-xs">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {tags.map((tag) => (
                            <SelectItem
                              key={tag.id}
                              value={tag.id.toString()}
                            >
                              {tag.tag_name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </TableCell>
                    <TableCell className="max-w-[420px]"><p className="mb-1 text-xs text-muted-foreground">{reviewLabel(question)}</p>
                      <span
                        className="line-clamp-2 text-sm leading-relaxed"
                        dangerouslySetInnerHTML={{
                          __html: renderContent(question.content.slice(0, 120)),
                        }}
                      />
                      {question.content.length > 120 && (
                        <span className="text-muted-foreground">…</span>
                      )}
                    </TableCell>
                    <TableCell className="whitespace-nowrap">
                      {question.is_real_name && question.bilibili_uid ? (
                        <a href={`https://space.bilibili.com/${question.bilibili_uid}`} target="_blank" rel="noopener noreferrer" className="inline-flex max-w-[145px] items-center gap-2 text-primary hover:underline">
                          {question.bilibili_avatar && <img src={question.bilibili_avatar} alt="" className="h-7 w-7 shrink-0 rounded-full object-cover" />}
                          <span className="truncate" title={`UID ${question.bilibili_uid}`}>{question.bilibili_name}</span>
                        </a>
                      ) : <span className="text-muted-foreground">匿名</span>}
                    </TableCell>
                    <TableCell className="text-center text-sm whitespace-nowrap">
                      {question.images_num}
                    </TableCell>
                    <TableCell className="text-center whitespace-nowrap">
                      <Checkbox
                        disabled={pending.has(question.id)}
                        aria-label="彩虹"
                        checked={question.is_rainbow}
                        onCheckedChange={(checked) =>
                          handleCheckboxChange(
                            question,
                            "is_rainbow",
                            !!checked
                          )
                        }
                      />
                    </TableCell>
                    <TableCell className="text-center whitespace-nowrap">
                      <Checkbox
                        disabled={pending.has(question.id)}
                        aria-label="归档"
                        checked={question.is_archive}
                        onCheckedChange={(checked) =>
                          handleCheckboxChange(
                            question,
                            "is_archive",
                            !!checked
                          )
                        }
                      />
                    </TableCell>
                    <TableCell className="text-center whitespace-nowrap">
                      <Checkbox
                        disabled={pending.has(question.id)}
                        aria-label="公开"
                        checked={question.is_publish}
                        onCheckedChange={(checked) =>
                          handleCheckboxChange(
                            question,
                            "is_publish",
                            !!checked
                          )
                        }
                      />
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                      {formatDate(question.created_at)}
                    </TableCell>
                    <TableCell className="text-right whitespace-nowrap">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            disabled={pending.has(question.id)}
                            aria-label="打开操作菜单"
                          >
                            <MoreHorizontal className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            disabled={pending.has(question.id)}
                            onSelect={() => handleMove(question)}
                          >
                            {isSpam ? <Inbox /> : <TrashIcon />}
                            {isSpam ? "移入收件箱" : "移到拦截箱"}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            className="text-destructive focus:text-destructive"
                            onSelect={() =>
                              setDeleteDialog({ open: true, id: question.id })
                            }
                          >
                            <Trash2 />
                            删除
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                  {expandedRows.has(question.id) && (
                    <TableRow key={`${question.id}-expanded`}>
                      <TableCell
                        colSpan={11}
                        className="bg-muted/30 p-0 align-top"
                      >
                        <div className="w-full max-w-full overflow-hidden">
                          <div className="grid grid-cols-1 gap-6 p-5 lg:grid-cols-3">
                            {/* 左侧：完整内容 + 附图 */}
                            <div className="space-y-5 lg:col-span-2 min-w-0">
                              <div className="min-w-0">
                                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                                  完整内容
                                </h4>
                                <div
                                  className="whitespace-pre-wrap break-words rounded-lg border border-border bg-card px-4 py-3 text-sm leading-relaxed overflow-hidden"
                                  dangerouslySetInnerHTML={{
                                    __html: renderContent(question.content),
                                  }}
                                />
                              </div>
                              <QuestionAIReview question={question} onRefresh={()=>void loadQuestions(undefined,true)} disabled={pending.has(question.id)} />
                              {question.images_num > 0 && (
                                <div className="min-w-0">
                                  <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                                    附图
                                  </h4>
                                  <ModerationAttachments compact images={attachmentUrls(question.images || "")}
                                    isProtected={protection.isProtected} onOpen={openAttachment} />
                                </div>
                              )}
                            </div>
                            {/* 右侧：元数据 + 编辑 */}
                            <div className="space-y-5 lg:col-span-1 min-w-0">
                              <div>
                                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                                  元数据
                                </h4>
                                <ul className="space-y-1.5 rounded-lg border border-border bg-card px-4 py-3 text-sm">
                                  <li>ID: {question.id}</li>
                                  <li>点赞: {question.likes}</li>
                                  <li>图片: {question.images_num}</li>
                                  <li>时间: {formatDate(question.created_at)}</li>
                                  {question.is_real_name && question.bilibili_uid && (
                                    <li>
                                      <a href={`https://space.bilibili.com/${question.bilibili_uid}`} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-2 text-primary hover:underline">
                                        {question.bilibili_avatar && <img src={question.bilibili_avatar} alt="" className="h-6 w-6 rounded-full object-cover" />}
                                        <span title={`UID ${question.bilibili_uid}`}>{question.bilibili_name}</span>
                                      </a>
                                    </li>
                                  )}
                                </ul>
                              </div>
                              <div>
                                <h4 className="mb-2 text-xs font-medium uppercase tracking-wider text-muted-foreground">
                                  编辑
                                </h4>
                                <div className="space-y-3 rounded-lg border border-border bg-card p-4">
                                  <div className="flex items-center justify-between gap-2">
                                    <span className="text-sm">话题</span>
                                    <Select
                                      disabled={pending.has(question.id)}
                        value={question.tag_id.toString()}
                                      onValueChange={(v) =>
                                        handleTagChange(question, parseInt(v))
                                      }
                                    >
                                      <SelectTrigger className="h-8 w-full max-w-[140px]">
                                        <SelectValue />
                                      </SelectTrigger>
                                      <SelectContent>
                                        {tags.map((tag) => (
                                          <SelectItem
                                            key={tag.id}
                                            value={tag.id.toString()}
                                          >
                                            {tag.tag_name}
                                          </SelectItem>
                                        ))}
                                      </SelectContent>
                                    </Select>
                                  </div>
                                  <div className="flex items-center justify-between gap-2">
                                    <span className="text-sm">彩虹</span>
                                    <Checkbox
                                      disabled={pending.has(question.id)}
                        aria-label="彩虹"
                        checked={question.is_rainbow}
                                      onCheckedChange={(c) =>
                                        handleCheckboxChange(
                                          question,
                                          "is_rainbow",
                                          !!c
                                        )
                                      }
                                    />
                                  </div>
                                  <div className="flex items-center justify-between gap-2">
                                    <span className="text-sm">归档</span>
                                    <Checkbox
                                      disabled={pending.has(question.id)}
                        aria-label="归档"
                        checked={question.is_archive}
                                      onCheckedChange={(c) =>
                                        handleCheckboxChange(
                                          question,
                                          "is_archive",
                                          !!c
                                        )
                                      }
                                    />
                                  </div>
                                  <div className="flex items-center justify-between gap-2">
                                    <span className="text-sm">公开</span>
                                    <Checkbox
                                      disabled={pending.has(question.id)}
                        aria-label="公开"
                        checked={question.is_publish}
                                      onCheckedChange={(c) =>
                                        handleCheckboxChange(
                                          question,
                                          "is_publish",
                                          !!c
                                        )
                                      }
                                    />
                                  </div>
                                  <Button
                                    variant="destructive"
                                    size="sm"
                                    className="w-full"
                                    onClick={() =>
                                      setDeleteDialog({
                                        open: true,
                                        id: question.id,
                                      })
                                    }
                                  >
                                    <Trash2 className="h-4 w-4 mr-2" />
                                    删除该提问
                                  </Button>
                                </div>
                              </div>
                            </div>
                          </div>
                        </div>
                      </TableCell>
                    </TableRow>
                  )}
                </Fragment>
              ))
            )}
          </TableBody>
        </Table>
      </section>}

      <section className="flex flex-wrap items-center justify-center gap-4">
        <span className="text-sm text-muted-foreground">
          第 {page} / {totalPages || 1} 页
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage(Math.max(1, page - 1))}
            disabled={isLoading || pending.size > 0 || page === 1}
          >
            上一页
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage(Math.min(totalPages, page + 1))}
            disabled={isLoading || pending.size > 0 || page >= totalPages}
          >
            下一页
          </Button>
        </div>
      </section>

      <Dialog
        open={deleteDialog.open}
        onOpenChange={(open) => setDeleteDialog({ ...deleteDialog, open })}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除提问</DialogTitle>
            <DialogDescription>
              确定要删除这条提问吗？此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setDeleteDialog({ open: false, id: 0 })}
            >
              取消
            </Button>
            <Button variant="destructive" disabled={pending.has(deleteDialog.id)} onClick={handleDelete}>
              {pending.has(deleteDialog.id) ? "删除中…" : "删除"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
