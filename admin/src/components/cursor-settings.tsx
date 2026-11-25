"use client";
import { useEffect, useState } from "react";
import { Card, CardHeader, CardTitle, CardContent, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { getConfig, type CursorTheme } from "@/lib/api";
import { installationAPI } from "@/lib/installation";
export function CursorSettings() {
  const [themes, setThemes] = useState<CursorTheme[]>([]);
  const [value, setValue] = useState("system");
  const [saved, setSaved] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const load = async () => { setBusy(true); setError(""); try { const res = await getConfig(); if (res.code !== 200) throw new Error(res.message || "读取失败"); setThemes(res.data.cursor_themes); setValue(res.data.cursor_theme); setSaved(res.data.cursor_theme); } catch { setError("鼠标样式读取失败，请重试"); } finally { setBusy(false); } };
  useEffect(() => { void load(); }, []);
  return <Card><CardHeader><CardTitle>鼠标样式</CardTitle><CardDescription>使用站点提供的指针样式，单独保存后生效。</CardDescription></CardHeader><CardContent className="space-y-4">
    {themes.length > 0 && <div className="max-w-sm space-y-2"><Label htmlFor="cursor-theme">指针样式</Label><Select value={value} onValueChange={v => { setValue(v); setMessage(""); }} disabled={busy}><SelectTrigger id="cursor-theme"><SelectValue /></SelectTrigger><SelectContent>{themes.map(item => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent></Select></div>}
    {themes.length === 0 ? <Button variant="outline" disabled={busy} onClick={() => void load()}>{busy ? "正在读取…" : "重新读取"}</Button> : <Button disabled={busy || value === saved} onClick={async () => { setBusy(true); setError(""); try { await installationAPI("appearance", "PUT", { cursor_theme: value }); setSaved(value); setMessage("鼠标样式已保存"); } catch (e) { setError(e instanceof Error ? e.message : "保存失败"); } finally { setBusy(false); } }}>保存鼠标样式</Button>}
    {error && <p role="alert" className="text-sm text-destructive">{error}</p>}{message && <p role="status" className="text-sm text-success">{message}</p>}
  </CardContent></Card>;
}
