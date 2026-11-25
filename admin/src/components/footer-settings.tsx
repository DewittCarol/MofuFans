"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { installationAPI } from "@/lib/installation";
import { emptyFooter, readFooter, policeLink, type Footer } from "../../../shared/footer";
import { safeAsset } from "../../../shared/theme";

export function FooterSettings() {
  const [draft, setDraft] = useState<Footer>(emptyFooter);
  const [saved, setSaved] = useState<Footer | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const file = useRef<HTMLInputElement>(null);
  const load = async () => {
    setBusy("load"); setError("");
    try { const data = readFooter(await installationAPI<Record<string, unknown>>("config")); setSaved(data); setDraft(data); }
    catch (e) { setError(e instanceof Error ? e.message : "读取失败，请重试"); } finally { setBusy(""); }
  };
  useEffect(() => { void load(); }, []);
  const change = (key: keyof Footer, value: string) => { setDraft(s => ({ ...s, [key]: value })); setMessage(""); setError(""); };
  const save = async () => {
    setBusy("save"); setError(""); setMessage("");
    try { const data = await installationAPI<Footer>("footer-settings", "PUT", draft); setDraft(data); setSaved(data); setMessage("页脚备案已保存，前台将自动更新。"); }
    catch (e) { setError(e instanceof Error ? e.message : "保存失败，请重试"); } finally { setBusy(""); }
  };
  const upload = async (image?: File) => {
    if (!image) return;
    if (image.size > 12 * 1024 * 1024) { setError("图片不能超过 12 MB"); return; }
    setBusy("upload"); setError("");
    try {
      const form = new FormData(); form.append("file", image);
      const response = await fetch("/api/theme-assets", { method: "POST", credentials: "same-origin", body: form });
      const result = await response.json();
      if (!response.ok || result.code !== 200) throw new Error(result.message || "图片上传失败");
      change("footer_police_icon", result.data.url);
    } catch (e) { setError(e instanceof Error ? e.message : "图片上传失败"); }
    finally { setBusy(""); if (file.current) file.current.value = ""; }
  };
  return <Card aria-label="页脚备案">
    <CardHeader><CardTitle>页脚备案</CardTitle><CardDescription>填写后显示在前台所有页面底部，留空则隐藏对应项目。与主题预设独立保存。</CardDescription></CardHeader>
    <CardContent className="space-y-5">
      {!saved && <Button variant="outline" disabled={!!busy} onClick={() => void load()}>{busy ? "正在读取…" : "重新读取页脚设置"}</Button>}
      <fieldset disabled={!!busy || !saved} className="space-y-5 disabled:opacity-70">
        <div className="grid gap-5 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="footer-icp">ICP 备案号</Label><Input id="footer-icp" value={draft.footer_icp_number} maxLength={80} onChange={e => change("footer_icp_number", e.target.value)} placeholder="填写已取得的完整备案号" /><p className="text-xs text-muted-foreground">点击后前往工信部备案查询网站。</p></div>
        <div className="space-y-2"><Label htmlFor="footer-police">公安备案号</Label><Input id="footer-police" value={draft.footer_police_number} maxLength={80} onChange={e => change("footer_police_number", e.target.value)} placeholder="填写省市简称、数字编号及号字" /></div></div>
        <div className="space-y-2"><Label htmlFor="footer-police-url">公安备案查询链接</Label><Input id="footer-police-url" type="url" maxLength={2048} value={draft.footer_police_url} onChange={e => change("footer_police_url", e.target.value)} placeholder="https://beian.mps.gov.cn/…" /><p className="text-xs text-muted-foreground">可粘贴官方查询链接；留空时自动根据备案号中的 14 位数字生成。</p></div>
        <div className="space-y-2"><Label htmlFor="footer-police-icon">公安备案图标（可选）</Label><div className="flex flex-wrap items-center gap-2"><Input id="footer-police-icon" className="min-w-0 flex-1 basis-56" maxLength={2048} value={draft.footer_police_icon} onChange={e => change("footer_police_icon", e.target.value)} placeholder="上传官方提供的图标，或填写 HTTPS 图片链接" /><Button variant="outline" onClick={() => file.current?.click()} disabled={!!busy}>{busy === "upload" ? "正在上传…" : "上传图标"}</Button>{draft.footer_police_icon && <Button variant="ghost" onClick={() => change("footer_police_icon", "")}>移除</Button>}</div><input ref={file} hidden type="file" accept="image/png,image/jpeg,image/webp" onChange={e => void upload(e.target.files?.[0])} /></div>
        <div className="rounded-lg border border-border bg-background p-4"><p className="mb-3 text-xs text-muted-foreground">页脚预览</p><div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm text-muted-foreground">
          {draft.footer_icp_number && <a href="https://beian.miit.gov.cn/" target="_blank" rel="noopener noreferrer" className="underline underline-offset-4">{draft.footer_icp_number}</a>}
          {draft.footer_police_number && <a href={policeLink(draft.footer_police_url)} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1.5 underline underline-offset-4">{draft.footer_police_icon && safeAsset(draft.footer_police_icon) && <img src={draft.footer_police_icon} alt="" className="h-4 w-4 object-contain" />}{draft.footer_police_number}</a>}
          {!draft.footer_icp_number && !draft.footer_police_number && <span>未填写备案信息，前台隐藏此区域。</span>}
        </div></div>
        <Button onClick={() => void save()} disabled={!!busy || !saved || JSON.stringify(draft) === JSON.stringify(saved)}>{busy === "save" ? "正在保存…" : "保存页脚备案"}</Button>
      </fieldset>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {message && <p role="status" className="text-sm text-success">{message}</p>}
    </CardContent>
  </Card>;
}
