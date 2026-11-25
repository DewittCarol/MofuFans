export default function Loading() {
  return <main aria-busy="true" className="mx-auto max-w-5xl px-4 py-8 sm:px-6">
    <p role="status" className="text-sm text-muted-foreground">正在打开页面…</p>
    <div aria-hidden="true" className="mt-6 space-y-4 motion-safe:animate-pulse">
      <div className="h-7 w-36 rounded bg-muted" />
      <div className="h-40 rounded-xl border border-border bg-card/70" />
    </div>
  </main>;
}
