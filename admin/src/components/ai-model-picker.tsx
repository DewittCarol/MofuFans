"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Check, ChevronDown, LoaderCircle, Search } from "lucide-react";
import { Popover } from "radix-ui";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

type Model = { id: string; name: string };
type Props = {
  models: Model[];
  value: string;
  loaded: boolean;
  loading: boolean;
  truncated: boolean;
  disabled: boolean;
  canFetch: boolean;
  error: string;
  onFetch: () => void;
  onSelect: (value: string) => void;
};

export function AIModelPicker({ models, value, loaded, loading, truncated, disabled, canFetch, error, onFetch, onSelect }: Props) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(-1);
  const search = useRef<HTMLDivElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const listId = useId();
  const term = query.trim().toLocaleLowerCase();
  const filtered = models.filter(model => `${model.id} ${model.name}`.toLocaleLowerCase().includes(term));

  useEffect(() => {
    if (open && active >= 0) list.current?.querySelector<HTMLElement>(`[data-model-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active, open]);

  const select = (model: Model) => {
    if (disabled || loading) return;
    onSelect(model.id);
    setOpen(false);
  };

  return <Popover.Root open={open} onOpenChange={next => {
    setOpen(next);
    if (next) {
      // The selected model must never become the search term for the full list.
      setQuery(""); setActive(-1);
      if (!loaded && !loading && canFetch) onFetch();
    }
  }}>
    <Popover.Trigger type="button" className={buttonVariants({ variant: "outline" })} disabled={disabled || loading} aria-label="打开模型列表">
      选择模型{loaded ? `（${models.length}）` : ""}<ChevronDown aria-hidden="true" className="size-4" />
    </Popover.Trigger>
    <Popover.Portal>
      <Popover.Content align="end" sideOffset={6} collisionPadding={16} aria-label="选择审核模型"
        onOpenAutoFocus={event => { event.preventDefault(); search.current?.querySelector("input")?.focus(); }}
        className="z-50 flex w-[min(24rem,calc(100vw-2rem))] max-h-[var(--radix-popover-content-available-height)] flex-col overflow-hidden rounded-lg border border-border bg-popover text-popover-foreground shadow-[0_4px_16px_rgba(61,57,52,0.08),inset_1px_1px_0_rgba(255,255,255,0.4)] outline-none">
        <div className="space-y-2 border-b border-border p-3">
          <div ref={search} className="relative">
            <Search aria-hidden="true" className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input className="pl-9" placeholder="搜索模型名称或 ID" aria-label="搜索模型" autoComplete="off" spellCheck={false}
              role="combobox" aria-expanded={open} aria-autocomplete="list" aria-controls={loaded && !loading && !error ? listId : undefined}
              aria-activedescendant={active >= 0 && filtered[active] ? `${listId}-${active}` : undefined}
              value={query} onChange={event => { setQuery(event.target.value); setActive(-1); }}
              onKeyDown={event => {
                if (event.nativeEvent.isComposing || loading || !filtered.length) return;
                if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                  event.preventDefault();
                  setActive(previous => event.key === "ArrowDown" ? Math.min(previous + 1, filtered.length - 1) : previous < 0 ? filtered.length - 1 : Math.max(0, previous - 1));
                } else if (event.key === "Enter" && active >= 0 && filtered[active]) {
                  event.preventDefault(); select(filtered[active]);
                }
              }} />
          </div>
          <p role="status" className="text-xs text-muted-foreground">
            {loading ? "正在读取模型列表…" : loaded ? term ? `匹配 ${filtered.length} / ${models.length} 个模型` : `共 ${models.length} 个模型 · 可滚动查看${truncated ? "（服务未返回完整列表）" : ""}` : "读取后可从列表选择，也可直接填写模型 ID。"}
          </p>
        </div>
        {loading ? <div role="status" className="flex items-center gap-2 px-4 py-6 text-sm text-muted-foreground"><LoaderCircle className="size-4 animate-spin motion-reduce:animate-none" aria-hidden="true" />正在获取模型…</div>
          : error ? <div className="space-y-3 p-4"><p role="alert" className="text-sm text-destructive">{error}</p><Button type="button" size="sm" variant="outline" onClick={onFetch} disabled={!canFetch}>重新获取</Button></div>
          : !loaded ? <p className="p-4 text-sm text-muted-foreground">请先填写服务地址与 API Key，再获取模型列表。也可以直接在外面的输入框填写模型 ID。</p>
          : <div ref={list} id={listId} role="listbox" aria-label="服务返回的模型" className="min-h-0 max-h-72 overflow-y-auto overscroll-contain p-1">
            {filtered.length ? filtered.map((model, index) => <button key={model.id} id={`${listId}-${index}`} data-model-index={index}
              type="button" role="option" tabIndex={-1} aria-selected={model.id === value} disabled={disabled}
              onClick={() => select(model)}
              className={cn("flex min-h-11 w-full items-center gap-3 rounded-lg px-3 py-2 text-left text-sm outline-none hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50", active === index && "bg-accent text-accent-foreground")}>
              <span className="min-w-0 flex-1 break-words [overflow-wrap:anywhere]">{model.id}{model.name !== model.id && <span className="mt-0.5 block text-xs text-muted-foreground">{model.name}</span>}</span>
              {model.id === value && <Check aria-label="当前选择" className="size-4 shrink-0 text-primary" />}
            </button>) : <div className="space-y-2 px-3 py-6 text-sm"><p>{models.length ? "没有匹配的模型" : "服务没有返回模型"}</p>{term && <Button type="button" size="sm" variant="ghost" onClick={() => { setQuery(""); setActive(-1); search.current?.querySelector("input")?.focus(); }}>清空搜索，显示全部</Button>}<p className="text-muted-foreground">也可以关闭列表，直接填写模型 ID。</p></div>}
          </div>}
        <div className="border-t border-border px-3 py-2 text-xs text-muted-foreground">列表由服务提供方返回。请选择支持文本审核的模型；仅生成图片的模型不适合此任务。</div>
      </Popover.Content>
    </Popover.Portal>
  </Popover.Root>;
}
