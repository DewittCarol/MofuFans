'use client';

import { QuestionAIReview } from '@/components/question-ai-review';
import { Archive, Check, EyeOff, Inbox, Rainbow, ShieldX, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { ModerationAttachments } from '@/components/moderation-attachments';
import { attachmentUrls } from '@/lib/attachments';
import type { Question, Tag } from '@/lib/api';

export function ModerationCard({ question, tags, isSpam, busy, renderContent, isProtected, onOpen, onChange, onTagChange, onMove, onDelete, onAIRefresh }: {
  onAIRefresh:()=>void;
  question: Question; tags: Tag[]; isSpam: boolean; busy: boolean;
  renderContent: (content: string) => string;
  isProtected: (src: string) => boolean;
  onOpen: (images: string[], index: number) => void;
  onChange: (question: Question, field: 'is_rainbow' | 'is_archive' | 'is_publish', value: boolean) => void;
  onTagChange: (question: Question, id: number) => void;
  onMove: (question: Question) => void;
  onDelete: (id: number) => void;
}) {
  const images = attachmentUrls(question.images || '');
  return <article aria-label={`提问 ${question.id}`} aria-busy={busy}
    className="flex min-w-0 flex-col rounded-xl border border-border bg-card shadow-[0_2px_8px_rgba(var(--shadow-rgb),0.04)]">
    <header className="flex flex-wrap items-center justify-between gap-2 border-b border-border/60 px-5 py-3 text-xs text-muted-foreground">
      <span>#{question.id} · {question.is_real_name ? question.bilibili_name || '实名投稿' : '匿名投稿'}</span>
      <span className={`inline-flex items-center gap-1 ${question.is_publish ? 'text-primary' : ''}`}>
        {question.is_publish && <Check size={13} />}{question.is_publish ? '已公开' : '未公开'}{question.is_archive && ' · 已归档'}{question.is_rainbow && ' · 彩虹'}
      </span>
    </header>
    <div className="flex-1 space-y-4 p-5">
      <div className="whitespace-pre-wrap break-words text-[15px] leading-7 [overflow-wrap:anywhere]"
        dangerouslySetInnerHTML={{ __html: renderContent(question.content) }} />
      <ModerationAttachments images={images} isProtected={isProtected} onOpen={onOpen} />
      <QuestionAIReview question={question} onRefresh={onAIRefresh} disabled={busy} />
      <time dateTime={question.created_at} className="block text-xs text-muted-foreground">{new Date(question.created_at).toLocaleString('zh-CN')}</time>
    </div>
    <fieldset disabled={busy} className="space-y-3 border-t border-border/60 px-5 py-4">
      <legend className="sr-only">审核提问 {question.id}</legend>
      <div className="flex min-w-0 items-center gap-3">
        <span className="text-xs text-muted-foreground">话题</span>
        <Select disabled={busy} value={question.tag_id.toString()} onValueChange={value => onTagChange(question, Number(value))}>
          <SelectTrigger aria-label="提问话题" className="h-8 w-auto min-w-0 max-w-[75%] border-transparent bg-transparent px-0 text-xs shadow-none"><SelectValue /></SelectTrigger>
          <SelectContent>{tags.map(tag => <SelectItem key={tag.id} value={tag.id.toString()}>{tag.tag_name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      <div className="flex flex-wrap gap-2">
        <Button variant={question.is_publish ? 'outline' : 'default'} size="sm" disabled={busy}
          onClick={() => onChange(question, 'is_publish', !question.is_publish)}>
          {question.is_publish ? <EyeOff size={15} /> : <Check size={15} />}{busy ? '保存中…' : question.is_publish ? '取消公开' : isSpam ? '通过并公开' : '公开'}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} onClick={() => onMove(question)}>
          {isSpam ? <Inbox size={15} /> : <ShieldX size={15} />}{isSpam ? '移入收件箱' : '拦截'}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} aria-pressed={question.is_archive}
          onClick={() => onChange(question, 'is_archive', !question.is_archive)}>
          <Archive size={15} />{question.is_archive ? '取消归档' : '归档此卡'}
        </Button>
        <Button variant="outline" size="sm" disabled={busy} aria-pressed={question.is_rainbow}
          onClick={() => onChange(question, 'is_rainbow', !question.is_rainbow)}>
          <Rainbow size={15} />{question.is_rainbow ? '取消彩虹' : '标记为彩虹'}
        </Button>
        <Button variant="ghost" size="sm" disabled={busy} className="text-destructive hover:text-destructive sm:ml-auto"
          onClick={() => onDelete(question.id)}><Trash2 size={15} />删除提问</Button>
      </div>
    </fieldset>
  </article>;
}
