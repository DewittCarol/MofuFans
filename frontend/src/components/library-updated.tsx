export function LibraryUpdated({value}: {value?: string | null}) {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  const text = new Intl.DateTimeFormat('zh-CN', {
    timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).format(date);
  return <p className="text-xs leading-6 text-muted-foreground sm:text-sm">
    上次更新于 <time dateTime={date.toISOString()} title="当前公开内容的最近更新时间（北京时间）">{text}</time>
    <span className="ml-1 text-xs">（北京时间）</span>
  </p>;
}
