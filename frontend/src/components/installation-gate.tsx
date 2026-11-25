"use client";

import { useEffect, useState, type ReactNode } from "react";

export function InstallationGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<"loading" | "ready" | "setup" | "error">("loading");
  useEffect(() => {
    let mounted = true;
    fetch("/api/setup/status", { cache: "no-store", signal: AbortSignal.timeout(15000) }).then(async response => {
      const result = await response.json();
      if (!response.ok || result.code !== 200) throw new Error("Service unavailable");
      if (mounted) setState(result.data.required ? "setup" : "ready");
    }).catch(() => { if (mounted) setState("error"); });
    return () => { mounted = false; };
  }, []);
  if (state === "ready") return children;
  return <main className="flex min-h-screen items-center justify-center px-6"><div className="max-w-md space-y-4 rounded-xl border bg-card p-8 text-center" role="status">
    <h1 className="text-xl font-semibold">{state === "loading" ? "正在连接小站…" : state === "setup" ? "小站准备中" : "暂时无法连接小站"}</h1>
    {state === "setup" && <><p className="text-sm text-muted-foreground">站点所有者完成首次配置后，这里就会开放。</p><a className="inline-block underline underline-offset-4" href="/admin/">我是站点所有者，开始配置</a></>}
    {state === "error" && <><p className="text-sm text-muted-foreground">请稍后再试，或联系站点管理员检查服务状态。</p><button className="underline underline-offset-4" onClick={() => window.location.reload()}>重新连接</button></>}
  </div></main>;
}
