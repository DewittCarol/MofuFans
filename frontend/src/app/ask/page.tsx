'use client';

import { useState, useEffect, useLayoutEffect, useCallback, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { PostCard } from '@/components/post-card';
import { FileUpload } from '@/components/file-upload';
import { InputEmojiPicker } from '@/components/input-emoji-picker';
import { QuestionJump } from '@/components/question-jump';
import { GoToTop } from '@/components/go-to-top';
import { getQuestions, getTags, getConfig, getInfo, createQuestion, Tag, Question } from '@/lib/api';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAccountAuth } from '@/lib/account-auth';
import { useEmojiCatalog } from '@/lib/emoji-catalog';

export default function HomePage() {
	const { user: accountUser, accountsEnabled, openAccountDialog } = useAccountAuth();
  const { requireVerifiedUserToPost } = useEmojiCatalog();
  const [tags, setTags] = useState<Tag[]>([]);
  const [selectedTag, setSelectedTag] = useState<string>('');
  const [content, setContent] = useState('');
  const [isRainbow, setIsRainbow] = useState(false);
  const [isRealName, setIsRealName] = useState(false);
  const [showImageUpload, setShowImageUpload] = useState(false);
  const [files, setFiles] = useState<File[]>([]);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitSuccess, setSubmitSuccess] = useState(false);
  const [tagError, setTagError] = useState('');
  const [contentError, setContentError] = useState('');
  const [submitError, setSubmitError] = useState('');
  const tagTriggerRef = useRef<HTMLButtonElement>(null);
  const [announcement, setAnnouncement] = useState('');
  const [isLoggedIn, setIsLoggedIn] = useState(false);

  // Questions state
  const [questions, setQuestions] = useState<Question[]>([]);
  const [firstPage, setFirstPage] = useState(1);
  const windowPages = useRef({ first: 1, last: 0 });
  const loadingRef = useRef(false);
  const requestVersion = useRef(0);
  const requestAbort = useRef<AbortController | null>(null);
  const pendingScroll = useRef<{ id: number; top?: number } | null>(null);
  const [targetId, setTargetId] = useState<number | null>(null);
  const [bounds, setBounds] = useState({ minId: 0, maxId: 0 });
  const [listError, setListError] = useState('');
  const [hasMore, setHasMore] = useState(true);
  const [isLoading, setIsLoading] = useState(false);
  const [hideArchive, setHideArchive] = useState(false);
  const [sortOrder, setSortOrder] = useState<'desc' | 'asc'>('desc');

  const { emojiUpdates, archiveUpdates, handleCardCursor, handleCardCursorLeave, getRemoteCursors } = useWebSocket();

  useEffect(() => {
    if (!accountUser) setIsRealName(false);
  }, [accountUser]);

  const observerRef = useRef<IntersectionObserver | null>(null);
  const loadMoreRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // Load initial data
  useEffect(() => {
    const savedContent = localStorage.getItem('ask_content');
    if (savedContent) setContent(savedContent);

    getTags().then((res) => {
      if (res.code === 200 && res.data) {
        const sorted = res.data.length ? [res.data[0], ...res.data.slice(1).reverse()] : [];
        setTags(sorted);
      }
    });

    getConfig().then((res) => {
      if (res.code === 200) {
        setAnnouncement(res.data.announcement || '');
      }
    });

    getInfo().then((res) => {
      setIsLoggedIn(res.code === 200);
    });
  }, []);

  // Jumping loads a small window around the target; it never fetches all earlier pages.
  const loadQuestions = useCallback(async (mode: 'reset' | 'next' | 'previous' | 'jump' = 'next', locateId?: number) => {
    if ((mode === 'next' || mode === 'previous') && loadingRef.current) return;
    const version = ++requestVersion.current;
    requestAbort.current?.abort();
    const abort = new AbortController(); requestAbort.current = abort;
    loadingRef.current = true; setIsLoading(true); setListError('');
    if (mode === 'reset') { setQuestions([]); setTargetId(null); setFirstPage(1); windowPages.current = { first: 1, last: 0 }; }
    const currentPage = mode === 'previous' ? Math.max(1, windowPages.current.first - 1) : mode === 'next' ? windowPages.current.last + 1 : 1;
    try {
      const res = await getQuestions({ page: currentPage, size: 5, locate_id: locateId, publish: true,
        order_by: 'id', order: sortOrder, archive: hideArchive ? false : undefined }, abort.signal);
      if (version !== requestVersion.current) return;
      if (res.code !== 200) throw new Error(res.code === 404 ? `当前列表里没有 #${locateId}，请检查编号或筛选条件。` : '没能加载投稿，请再试一次。');
      const rows = res.data.questions || [];
      setBounds({ minId: res.data.min_id, maxId: res.data.max_id });
      if (mode === 'previous') {
        const anchor = document.querySelector<HTMLElement>('[data-question-anchor]');
        if (anchor) pendingScroll.current = { id: Number(anchor.dataset.questionAnchor), top: anchor.getBoundingClientRect().top };
        setQuestions(previous => [...rows, ...previous.filter(item => !rows.some(row => row.id === item.id))]);
        windowPages.current.first = res.data.page;
      } else if (mode === 'next') {
        setQuestions(previous => [...previous, ...rows.filter(row => !previous.some(item => item.id === row.id))]);
        windowPages.current.last = res.data.page;
      } else {
        setQuestions(rows); windowPages.current = { first: res.data.page, last: res.data.page };
        if (locateId) { pendingScroll.current = { id: locateId }; setTargetId(locateId); }
      }
      setFirstPage(windowPages.current.first);
      setHasMore(windowPages.current.last * res.data.page_size < res.data.total);
    } catch (error) {
      if (version !== requestVersion.current || abort.signal.aborted) return;
      const message = error instanceof Error ? error.message : '没能加载投稿，请再试一次。';
      if (mode === 'jump') throw new Error(message);
      setListError('没能加载投稿，请检查连接后重试。');
    } finally {
      if (version === requestVersion.current) { loadingRef.current = false; setIsLoading(false); }
    }
  }, [sortOrder, hideArchive]);

  useEffect(() => {
    void loadQuestions('reset');
    return () => { ++requestVersion.current; requestAbort.current?.abort(); };
  }, [loadQuestions]);

  useLayoutEffect(() => {
    const pending = pendingScroll.current;
    if (!pending) return;
    pendingScroll.current = null;
    const element = document.getElementById(`ask-question-${pending.id}`);
    if (!element) return;
    if (pending.top !== undefined) window.scrollBy({ top: element.getBoundingClientRect().top - pending.top, behavior: 'instant' });
    else {
      element.focus({ preventScroll: true });
      element.scrollIntoView({ block: 'start', behavior: 'instant' });
    }
  }, [questions]);

  // Infinite scroll
  useEffect(() => {
    if (observerRef.current) {
      observerRef.current.disconnect();
    }

    observerRef.current = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !isLoading && !listError) {
          loadQuestions();
        }
      },
      { threshold: 0.1 }
    );

    if (loadMoreRef.current) {
      observerRef.current.observe(loadMoreRef.current);
    }

    return () => {
      if (observerRef.current) {
        observerRef.current.disconnect();
      }
    };
  }, [hasMore, isLoading, listError, loadQuestions]);

  const handleContentChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setContent(e.target.value);
    setContentError(''); setSubmitError('');
    localStorage.setItem('ask_content', e.target.value);
  };

  const handleEmojiInsert = (tag: string) => {
    const textarea = textareaRef.current;
    if (!textarea) {
      setContent((prev) => prev + tag);
      localStorage.setItem('ask_content', content + tag);
      return;
    }

    const start = textarea.selectionStart;
    const end = textarea.selectionEnd;
    const newContent = content.slice(0, start) + tag + content.slice(end);
    setContent(newContent);
    localStorage.setItem('ask_content', newContent);

    // Restore cursor position after the inserted emoji
    setTimeout(() => {
      textarea.focus();
      textarea.setSelectionRange(start + tag.length, start + tag.length);
    }, 0);
  };

  const handleSubmit = async () => {
    if (isSubmitting) return;
    setSubmitError('');
    if (requireVerifiedUserToPost && !accountUser) {
      openAccountDialog('login');
      return;
    }
    const trimmedContent = content.trim().replace(/｛/g, '{').replace(/｝/g, '}');
    setTagError(selectedTag ? '' : '先选一个话题，再提交提问。');
    setContentError(trimmedContent ? '' : '写下你的提问后再提交。');
    if (!selectedTag || !trimmedContent) {
      if (!selectedTag) tagTriggerRef.current?.focus();
      else textareaRef.current?.focus();
      return;
    }

    setIsSubmitting(true);
    const formData = new FormData();
    formData.append('tag_id', selectedTag);
    formData.append('content', trimmedContent);
    if (isRainbow) formData.append('rainbow', 'true');
    if (accountsEnabled && isRealName) formData.append('real_name', 'true');
    files.forEach((file) => formData.append('files[]', file));

    try {
      const res = await createQuestion(formData);
      if (res.ok) {
        const data = await res.json();
        if (data.code === 200) {
          setSubmitSuccess(true);
          setContent('');
          localStorage.setItem('ask_content', '');
          setSelectedTag('');
          setIsRainbow(false);
          setIsRealName(false);
          setShowImageUpload(false);
          setFiles([]);
          setTimeout(() => setSubmitSuccess(false), 4000);
        } else {
          setSubmitError(data.message || '投稿未成功，请稍后重试。');
        }
      } else if (res.status === 413) {
        setSubmitError('图片太大，请缩小图片体积或者分开投稿。');
      } else {
        setSubmitError('投稿未成功，请稍后重试。内容已保留。');
      }
    } catch (error) {
      console.error('Submit error:', error);
      setSubmitError('连接失败，请检查网络后重试。内容已保留。');
    } finally {
      setIsSubmitting(false);
    }
  };

  // Preview card data
  const previewCard: Question | null = content && selectedTag ? {
    id: 0,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    tag_id: parseInt(selectedTag),
    tag: tags.find((t) => t.id === parseInt(selectedTag)) || { id: 0, tag_name: '', description: '', question_count: 0, created_at: '', updated_at: '' },
    content: '[卡片预览]\n' + content,
    images: files.map((f) => URL.createObjectURL(f)).join(';'),
    images_num: files.length,
    is_hide: false,
    is_rainbow: isRainbow,
    is_real_name: isRealName,
    bilibili_uid: isRealName ? accountUser?.bilibili_uid : undefined,
    bilibili_name: isRealName ? accountUser?.bilibili_name : undefined,
    bilibili_avatar: isRealName ? accountUser?.bilibili_avatar : undefined,
    is_archive: false,
    is_publish: false,
    is_spam: false,
    emojis: '[]',
    likes: 0,
  } : null;

  return (
    <div className="flex flex-col w-full items-center">
      {/* Submit Form */}
      <div className="ask max-w-[600px] w-5/6 relative my-8 p-5 flex flex-col rounded-md fabric-form transition-all duration-220">
        {/* Tag Select */}
        <div className="mb-2.5">
          <Select value={selectedTag} onValueChange={value => { setSelectedTag(value); setTagError(''); setSubmitError(''); }}>
            <SelectTrigger ref={tagTriggerRef} aria-label="投稿话题" aria-invalid={!!tagError} aria-describedby={tagError ? 'post-topic-error' : undefined}
              className={`w-[140px] fabric-pill border-none text-primary text-xs ${tagError ? 'ring-1 ring-destructive' : ''}`}>
              <SelectValue placeholder="选择话题" />
            </SelectTrigger>
            <SelectContent>
              {tags.map((tag) => (
                <SelectItem key={tag.id} value={tag.id.toString()}>
                  #{tag.tag_name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {tagError && <p id="post-topic-error" role="alert" className="mt-2 text-sm text-destructive">{tagError}</p>}
          {selectedTag && (
            <pre className="text-primary text-sm mt-2 whitespace-pre-wrap">
              {tags.find((t) => t.id === parseInt(selectedTag))?.description}
            </pre>
          )}
        </div>

        {/* Content */}
        <div className={`relative ${isRainbow ? 'rainbow-bg' : ''}`}>
          <textarea
            ref={textareaRef}
            aria-label="提问内容" aria-invalid={!!contentError} aria-describedby={contentError ? 'post-content-error' : undefined}
            className={`w-full min-h-[128px] p-4 text-foreground bg-secondary/50 border-2 border-dashed border-[var(--fabric-stitch)] rounded-lg resize-none focus:bg-card focus:border-primary focus:outline-none transition-all duration-220 ${
              isRainbow ? 'rainbow-bg' : ''
            }`}
            style={{ fontFamily: '宋体, Fangsong, STFangsong, sans-serif' }}
            maxLength={800}
            value={content}
            onChange={handleContentChange}
            placeholder="输入你的提问..."
          />
          <div className="absolute bottom-4 right-4">
            <InputEmojiPicker onSelect={handleEmojiInsert} />
          </div>
        </div>

        {contentError && <p id="post-content-error" role="alert" className="mt-2 text-sm text-destructive">{contentError}</p>}

        {/* Options */}
        <div className="flex justify-between my-4 text-sm">
          <div className="text-primary whitespace-pre-wrap">{announcement}</div>
          <div className="flex flex-col items-start gap-1.5 text-primary">
            <Label className="flex cursor-pointer items-center gap-2 leading-6">
              <Checkbox
                checked={isRainbow}
                onCheckedChange={(checked) => setIsRainbow(!!checked)}
              />
              彩虹屁
            </Label>
            <Label className="flex cursor-pointer items-center gap-2 leading-6">
              <Checkbox
                checked={showImageUpload}
                onCheckedChange={(checked) => setShowImageUpload(!!checked)}
              />
              附图
            </Label>
            {accountsEnabled && <Label className={`flex items-center gap-2 leading-6 ${accountUser ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'}`}>
              <Checkbox
                checked={isRealName}
                onCheckedChange={(checked) => setIsRealName(!!checked)}
                disabled={!accountUser}
              />
              {accountUser ? '实名投稿' : '实名投稿（需登录）'}
            </Label>}
          </div>
        </div>

        {/* File Upload */}
        {showImageUpload && (
          <div className="mb-4">
            <FileUpload files={files} onFilesChange={setFiles} maxFiles={6} />
          </div>
        )}

        {/* Submit Button */}
        {submitError && <p role="alert" className="mb-3 text-sm text-destructive">{submitError}</p>}
        <Button
          onClick={handleSubmit}
          disabled={isSubmitting}
        >
          {isSubmitting ? '上传中，请稍等' : requireVerifiedUserToPost && !accountUser ? '登录后提交' : '提交'}
        </Button>

        {/* Success Message */}
        {submitSuccess && (
          <div className="absolute inset-0 flex bg-card text-foreground text-center items-center justify-center z-10 rounded-md border-2 border-dashed border-[var(--fabric-stitch)]">
            提问已提交审核，内容将会在审核通过后放出
          </div>
        )}
      </div>

      {/* Preview Card */}
      {previewCard && <PostCard data={previewCard} isLoggedIn={false} />}

      {/* Filters */}
      <div className="max-w-[600px] w-5/6 flex justify-between items-center mb-4 p-3 rounded-md bg-card border-2 border-dashed border-[var(--fabric-stitch)]">
        <Label className="flex items-center cursor-pointer text-primary">
          <Checkbox
            checked={hideArchive}
            onCheckedChange={(checked) => {
              setHideArchive(!!checked);
              setQuestions([]);
            }}
            className="mr-2"
          />
          隐藏归档卡片
        </Label>
        <Select
          value={sortOrder}
          onValueChange={(v) => {
            setSortOrder(v as 'desc' | 'asc');
            setQuestions([]);
          }}
        >
          <SelectTrigger className="w-[120px] fabric-card border-2 border-dashed border-[var(--fabric-stitch)]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="desc">从新到旧</SelectItem>
            <SelectItem value="asc">从旧到新</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Questions List */}
      {firstPage > 1 && <Button variant="outline" disabled={isLoading} onClick={() => void loadQuestions('previous')} className="my-3">加载前面的投稿</Button>}
      <div id="question-list" className="w-full">
      {questions.map((question) => (
        <div key={question.id} id={`ask-question-${question.id}`} data-question-anchor={question.id} tabIndex={-1}
          className={`w-full scroll-mt-24 outline-none ${targetId === question.id ? '[&_.card-wrap]:ring-4 [&_.card-wrap]:ring-primary [&_.card-wrap]:ring-offset-4 [&_.card-wrap]:ring-offset-background' : ''}`}>
        <PostCard
          data={question}
          isLoggedIn={isLoggedIn}
          emojiUpdates={emojiUpdates}
          archiveUpdates={archiveUpdates}
          remoteCursors={getRemoteCursors(question.id)}
          onCursorMove={(x, y) => handleCardCursor(question.id, x, y)}
          onCursorLeave={() => handleCardCursorLeave(question.id)}
        />
        </div>
      ))}

      </div>
      {/* Load More */}
      <div ref={loadMoreRef} className="py-4 text-primary text-center">
        {listError ? <span role="alert">{listError}<Button variant="ghost" onClick={() => void loadQuestions()}>重试</Button></span> : isLoading ? '加载中...' : hasMore ? '' : '已经到底啦( ´･･)ﾉ(._.`)'}
      </div>

      <style jsx>{`
        .rainbow-bg {
          background: linear-gradient(
            130deg,
            rgba(220, 93, 231, 0.33) 0%,
            rgba(127, 239, 189, 0.33) 33%,
            rgba(255, 245, 137, 0.33) 66%,
            rgba(236, 11, 67, 0.33) 100%
          );
        }
      `}</style>

      <QuestionJump key={`${sortOrder}-${hideArchive}`} floating {...bounds} busy={isLoading} onJump={id => loadQuestions('jump', id)} />
      <GoToTop />
    </div>
  );
}
