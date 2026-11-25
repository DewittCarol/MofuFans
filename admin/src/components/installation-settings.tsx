"use client";

import { useEffect, useState } from "react";
import { useAuth } from "@/lib/auth";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { InstallationFields } from "@/components/installation-fields";
import { editableSettings, installationAPI, installationError, type InstallationSettings, type InstallationUpdate } from "@/lib/installation";

export function InstallationSettingsPanel() {
  const { user } = useAuth();
  const [saved, setSaved] = useState<InstallationSettings>();
  const [value, setValue] = useState<InstallationUpdate>();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  async function load() {
    setError("");
    try {
      const data = await installationAPI<InstallationSettings>("installation-settings");
      setSaved(data); setValue(editableSettings(data));
    } catch (error) { setError(installationError(error)); }
  }
  useEffect(() => { if (user?.is_super_admin) void load(); }, [user?.is_super_admin]);
  if (!user?.is_super_admin) return null;
  async function save(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setMessage(""); setError("");
    try {
      const data = await installationAPI<InstallationSettings>("installation-settings", "PUT", value);
      setSaved(data); setValue(editableSettings(data));
      setMessage(data.restart_required ? "设置已保存。存储配置将在重启服务后生效；测试 UID 和 Cookie 设置已生效。" : "部署与集成设置已保存并生效。");
    } catch (error) { setError(installationError(error)); }
    finally { setBusy(false); }
  }
  return <Card id="installation-settings" tabIndex={-1} className="fabric-shadow fabric-border scroll-mt-8">
    <CardHeader><CardTitle>部署与集成</CardTitle><CardDescription>管理文件存储、私信测试收件人和 HTTPS 登录设置。仅站点所有者可以修改。</CardDescription></CardHeader>
    <CardContent>
      {saved && value ? <form onSubmit={save} className="space-y-6">
        <p className="text-sm text-muted-foreground">当前数据库：{saved.database_type === "mysql" ? "MySQL" : "SQLite"}。当前文件存储：{saved.active_storage_type === "oss" ? "阿里云 OSS" : "本地"}。数据库连接、端口等高级部署选项通过服务器配置文件设置。</p>
        {saved.restart_required && <p role="status" className="rounded-lg border border-primary/30 p-4 text-sm">有待生效的存储配置。请安排文件迁移并重启服务后再次检查。</p>}
        <InstallationFields value={value} onChange={next => { setValue(next); setMessage(""); }} saved={saved} busy={busy} />
        <details className="rounded-lg border p-4 text-sm"><summary className="cursor-pointer font-medium">更换存储与清除凭证</summary><div className="mt-4 space-y-4">
          <p className="text-muted-foreground">更换存储方式、Bucket 或前缀不会自动迁移已有文件。保存前请备份，完成文件迁移后再重启。Docker 使用 <code>docker compose restart mofufans</code>；systemd 使用 <code>sudo systemctl restart mofufans</code>。</p>
          <label className="flex items-start gap-3"><input type="checkbox" disabled={busy} checked={!!value.confirm_storage_change} onChange={event => setValue({ ...value, confirm_storage_change: event.target.checked })} className="mt-1" /><span>我已确认存储变更，并安排好现有文件的备份与迁移。</span></label>
          {value.storage_type === "local" && <label className="flex items-start gap-3"><input type="checkbox" disabled={busy} checked={!!value.clear_credentials} onChange={event => setValue({ ...value, clear_credentials: event.target.checked, oss: { ...value.oss, access_key: "", secret_key: "" } })} className="mt-1" /><span>保存时清除后台保存的 OSS 凭证（不影响环境变量）。</span></label>}
        </div></details>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        {message && <p role="status" className="text-sm">{message}</p>}
        <Button type="submit" disabled={busy}>{busy ? "正在保存并检查…" : "保存部署与集成设置"}</Button>
      </form> : <div role="status">{error || "正在加载部署设置…"}{error && <Button variant="outline" className="ml-3" onClick={load}>重试</Button>}</div>}
    </CardContent>
  </Card>;
}
