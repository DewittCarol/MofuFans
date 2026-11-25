'use client';
import {useSiteTheme} from '@/lib/site-theme';

import {Suspense, useEffect, useMemo, useRef, useState} from 'react';
import {useSearchParams, useRouter} from 'next/navigation';
import Link from 'next/link';
import {Sailboat, ChevronDown} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {VoyageIssueContents} from '@/components/voyage-issue-detail';
import {useVoyageIssues} from '@/lib/use-voyage-issues';
import type {VoyageIssue} from '@/lib/voyage';

function issueYear(issue: VoyageIssue) { return issue.month!.slice(0, 4); }

function issueID(value: string | null) {
  if (!value || !/^[1-9]\d*$/.test(value)) return null;
  const id = Number(value);
  return Number.isSafeInteger(id) ? id : null;
}

function toggle<T>(current: Set<T>, value: T) {
  const next = new Set(current);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  return next;
}

function VoyageList() {
  const {theme}=useSiteTheme();
  const {issues, loading, error, retry} = useVoyageIssues();
  const params = useSearchParams();
  const router=useRouter();
  const legacyClaim=issueID(params.get('claim'));
  useEffect(()=>{if(legacyClaim)router.replace(`/voyage/collections/?batch=${legacyClaim}`);},[legacyClaim,router]);
  const detailID = issueID(params.get('expand')) || issueID(params.get('id'));
  const [expandedYears, setExpandedYears] = useState<Set<string>>(() => new Set());
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());

  const [pendingScroll, setPendingScroll] = useState<number | null>(null);
  const [linkNotice, setLinkNotice] = useState('');
  const handledLink = useRef<string | null>(null);

  const years = useMemo(() => {
    const groups = new Map<string, VoyageIssue[]>();
    for (const issue of issues) {
      const year = issueYear(issue);
      groups.set(year, [...(groups.get(year) || []), issue]);
    }
    return [...groups.entries()].sort(([a], [b]) => {
      if (a === 'other') return 1;
      if (b === 'other') return -1;
      return Number(b) - Number(a);
    });
  }, [issues]);

  // Wait for the requested issue before opening its year and gift details.
  // Search params can also change while this page is already mounted.
  useEffect(() => {
    const targetID = detailID;
    if (!targetID) {
      handledLink.current = null;
      setLinkNotice('');
      return;
    }
    if (loading || error) return;
    const link = `detail:${targetID}`;
    if (handledLink.current === link) return;
    const issue = issues.find(item => item.id === targetID);
    if (!issue) {
      setLinkNotice('链接中的这张礼物卡片暂不可查看，可能尚未发布或已移除。');
      return;
    }
    setLinkNotice('');
    handledLink.current = link;
    const selectedIssue=issue;
    setExpandedYears(current => new Set(current).add(issueYear(selectedIssue)));
    setExpanded(current => new Set(current).add(selectedIssue.id));
    setPendingScroll(selectedIssue.id);
  }, [detailID, issues, loading, error]);

  useEffect(() => {
    if (pendingScroll === null) return;
    // A frame after React reveals the year ensures the target has a layout box.
    const frame = requestAnimationFrame(() => {
      const target = document.getElementById(`voyage-issue-${pendingScroll}`);
      if (target) {
        target.focus({preventScroll: true});
        target.scrollIntoView({block: 'start', behavior: 'auto'});
      }
      setPendingScroll(null);
    });
    return () => cancelAnimationFrame(frame);
  }, [pendingScroll]);

  return <main className="mx-auto min-h-[calc(100svh-56px)] w-full max-w-6xl px-5 pb-16 pt-10 sm:px-8 sm:pt-14">
    <header className="mb-8 flex items-center gap-3">
      {theme.modules.voyage.icon?<img src={theme.modules.voyage.icon} alt="" className="h-[30px] w-[30px] object-contain"/>:<Sailboat size={30} strokeWidth={1.5} className="text-primary"/>}
      <h1 className="text-3xl font-semibold">{theme.modules.voyage.label}</h1>
    </header>
    <nav className="mb-6 flex gap-6 border-b border-border text-sm" aria-label="大航海页面"><span aria-current="page" className="border-b-2 border-primary py-3 font-medium text-primary">礼物详情</span><Link className="min-h-11 py-3 text-muted-foreground hover:text-primary" href="/voyage/collections/">地址与物流</Link></nav>
    {loading ? <p role="status" className="py-12 text-sm text-muted-foreground">正在加载…</p>
      : error ? <div role="alert"><p>{error}</p><Button className="mt-4" variant="outline" onClick={retry}>重试</Button></div>
      : <>
        {linkNotice && <p role="status" className="mb-6 text-sm text-muted-foreground">{linkNotice}</p>}
        {!issues.length ? <p className="py-16 text-center text-muted-foreground">暂无舰礼</p>
          : <div className="space-y-3">{years.map(([year, entries]) => {
            const open = expandedYears.has(year);
            return <section key={year} aria-labelledby={`voyage-year-heading-${year}`}>
              <h2 id={`voyage-year-heading-${year}`}>
                <button type="button" className="mb-4 flex min-h-16 w-full items-center justify-between gap-4 border-b border-border py-4 text-left text-xl font-semibold text-foreground hover:text-primary focus-visible:rounded-md focus-visible:outline-2 focus-visible:outline-ring"
                  aria-expanded={open} aria-controls={`voyage-year-${year}`}
                  onClick={() => setExpandedYears(current => toggle(current, year))}>
                  <span>{year}年</span>
                  <span className="flex shrink-0 items-center gap-2 text-sm font-normal text-muted-foreground">
                    {open ? '收起' : '展开'}<ChevronDown size={18} aria-hidden="true" className={open ? 'rotate-180' : ''}/>
                  </span>
                </button>
              </h2>
              <div id={`voyage-year-${year}`} hidden={!open} className="space-y-4 pb-6">
                {[...new Set(entries.map(issue => issue.month!))].sort().reverse().map(month => <section key={month} className="space-y-5 pb-6" aria-labelledby={`voyage-month-${month}`}>
                  <h3 id={`voyage-month-${month}`} className="text-xl font-semibold">{Number(month.slice(5))}月</h3>
                  {(['guard', 'lottery'] as const).map(kind => {
                    const cards = entries.filter(issue => issue.month === month && issue.kind === kind);
                    if (!cards.length) return null;
                    return <section key={kind} className="space-y-3"><h4 className="text-sm font-medium text-primary">{kind === 'guard' ? '大航海回馈' : '抽奖活动'}</h4>
                      {cards.map(issue => <article key={issue.id} id={`voyage-issue-${issue.id}`} tabIndex={-1} aria-labelledby={`voyage-title-${issue.id}`} className="fabric-card scroll-mt-24 rounded-2xl border border-border px-5 pt-5 focus:outline-none sm:px-7 sm:pt-6">
                        <h5 id={`voyage-title-${issue.id}`} className="break-words text-xl font-semibold">{issue.title}</h5>
                        <button type="button" className="my-2 flex min-h-11 w-full items-center gap-1.5 rounded text-left text-sm text-muted-foreground hover:text-primary focus-visible:outline-2 focus-visible:outline-ring" aria-expanded={expanded.has(issue.id)} aria-controls={`issue-${issue.id}`} onClick={() => setExpanded(current => toggle(current, issue.id))}>
                          <ChevronDown size={16} aria-hidden="true" className={expanded.has(issue.id) ? 'rotate-180' : ''}/>{expanded.has(issue.id) ? '收起详情' : '查看详情'}
                        </button>
                        {expanded.has(issue.id) && <div id={`issue-${issue.id}`} className="pb-6"><VoyageIssueContents issue={issue}/></div>}
                      </article>)}
                    </section>;
                  })}
                </section>)}
              </div>
            </section>;
          })}</div>}
      </>}
  </main>;
}

export default function Page() {
  return <Suspense fallback={null}><VoyageList/></Suspense>;
}
