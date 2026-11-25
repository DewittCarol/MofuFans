export default function DashboardLoading() {
  return <section aria-busy="true" aria-label="正在打开页面" className="space-y-6">
    <p role="status" className="text-sm text-muted-foreground">正在打开页面…</p>
    <div aria-hidden="true" className="space-y-4 motion-safe:animate-pulse">
      <div className="h-7 w-36 rounded bg-muted" />
      <div className="h-11 w-full rounded-lg bg-muted/70" />
      <div className="rounded-xl border border-border bg-card p-6">
        {[80, 64, 72].map(width => <div key={width} className="mb-5 h-4 rounded bg-muted last:mb-0" style={{ width: `${width}%` }} />)}
      </div>
    </div>
  </section>;
}
