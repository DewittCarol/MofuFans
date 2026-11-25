"use client";

import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { InstallationFields } from "@/components/installation-fields";
import { editableSettings, installationAPI, installationError, type InstallationSettings, type InstallationUpdate } from "@/lib/installation";

const steps = ["确认安装", "创建站点", "存储与集成"];

export default function SetupPage() {
  const [ready, setReady] = useState(false);
  const [step, setStep] = useState(0);
  const [token, setToken] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [site, setSite] = useState("MofuFans");
  const [streamer, setStreamer] = useState("");
  const [saved, setSaved] = useState<InstallationSettings>();
  const [settings, setSettings] = useState<InstallationUpdate>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    installationAPI<{ required: boolean }>("setup/status").then(status => {
      if (!status.required) window.location.replace("/admin/");
      else setReady(true);
    }).catch(error => setError(installationError(error)));
  }, []);
  useEffect(() => { if (ready) heading.current?.focus(); }, [step, ready, done]);
  async function next(event: React.FormEvent) {
    event.preventDefault(); setError("");
    if (step === 1) {
      if (password !== repeat) { setError("两次输入的密码不一致，请重新确认。"); return; }
      setStep(2); return;
    }
    setBusy(true);
    try {
      if (step === 0) {
        const data = await installationAPI<InstallationSettings>("setup/verify", "POST", {}, token.trim());
        setSaved(data);
        const editable = editableSettings(data);
        if (!data.locked.includes("security.secure_cookie") && window.location.protocol === "https:") editable.secure_cookie = true;
        setSettings(editable); setStep(1);
      } else {
        await installationAPI("setup/complete", "POST", { username, password, site_name: site, streamer_name: streamer, settings }, token.trim());
        setToken(""); setPassword(""); setRepeat(""); setSettings(undefined); setDone(true);
      }
    } catch (error) { setError(installationError(error)); }
    finally { setBusy(false); }
  }
  return <main className="min-h-screen bg-background px-4 py-10 sm:py-16">
    <div className="mx-auto max-w-2xl">
      <p className="mb-3 text-sm font-medium text-muted-foreground">MofuFans · 首次启动</p>
      <h1 ref={heading} tabIndex={-1} className="text-2xl font-semibold tracking-tight outline-none">{done ? "站点已经准备好了" : "从这里开始，建立你的粉丝小站"}</h1>
      <p className="mt-3 text-sm leading-6 text-muted-foreground">{done ? "管理员账号已创建。登录后台后，可以继续调整主题、添加内容和配置其他功能。" : "创建管理员账号，给小站起个名字。其他服务可以现在配置，也可以进入后台后再设置。"}</p>
      {!done && <ol className="my-8 grid grid-cols-3 gap-3" aria-label="初始化进度">{steps.map((label, index) => <li key={label} aria-current={step === index ? "step" : undefined}
        className={`border-t-2 pt-3 text-sm ${step === index ? "border-primary font-semibold text-foreground" : "border-border text-muted-foreground"}`}><span className="mr-2">{index + 1}.</span>{label}</li>)}</ol>}
      {done ? <div className="mt-8 space-y-5 rounded-xl border bg-card p-6">
        <p><strong>{site}</strong> · 管理员 {username}</p>
        <p className="text-sm text-muted-foreground">请定期备份服务器的 data 目录，里面包括数据库、上传文件和自动生成的密钥。</p>
        <Button asChild><a href="/admin/">进入后台登录</a></Button>
      </div> : !ready ? <div className="mt-8" role="status">{error || "正在检查站点状态…"}{error && <Button className="ml-3" variant="outline" onClick={() => window.location.reload()}>重试</Button>}</div> :
        <form onSubmit={next} className="rounded-xl border bg-card p-5 sm:p-8">
          <fieldset disabled={busy} className="min-w-0 space-y-5">
            <legend className="mb-5 text-lg font-medium">{steps[step]}</legend>
            {step === 0 && <>
              <p className="text-sm leading-6 text-muted-foreground">从服务器启动日志中复制一次性安装码，确认你是站点的部署者。完成初始化后，安装码会失效。</p>
              <div className="space-y-2"><Label htmlFor="setup-token">一次性安装码</Label><Input id="setup-token" type="password" value={token} onChange={event => setToken(event.target.value)} required autoComplete="off" spellCheck={false} placeholder="粘贴启动日志中的完整安装码" /></div>
              <details className="rounded-lg bg-muted p-4 text-sm"><summary className="cursor-pointer font-medium">在哪里找到安装码？</summary>
                <div className="mt-3 space-y-3 text-muted-foreground"><p>Docker：<code className="break-all">docker compose logs mofufans</code></p><p>systemd：<code className="break-all">sudo journalctl -u mofufans -n 50</code></p><p>也可以读取服务器上的 <code>data/setup-token</code> 文件。安装码不需要发给其他人。</p></div>
              </details>
            </>}
            {step === 1 && <>
              <div className="grid gap-5 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="site-name">站点名称</Label><Input id="site-name" required maxLength={80} value={site} onChange={event => setSite(event.target.value)} /></div>
                <div className="space-y-2"><Label htmlFor="streamer-name">主播名称</Label><Input id="streamer-name" required maxLength={80} value={streamer} onChange={event => setStreamer(event.target.value)} placeholder="希望在站点展示的名字" /></div>
              </div>
              <div className="space-y-2"><Label htmlFor="admin-name">管理员用户名</Label><Input id="admin-name" required maxLength={64} autoComplete="username" value={username} onChange={event => setUsername(event.target.value)} /></div>
              <div className="grid gap-5 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="admin-password">管理员密码</Label><Input id="admin-password" required type="password" minLength={15} maxLength={256} autoComplete="new-password" value={password} onChange={event => setPassword(event.target.value)} aria-describedby="password-help" /></div>
                <div className="space-y-2"><Label htmlFor="repeat-password">再次输入密码</Label><Input id="repeat-password" required type="password" minLength={15} maxLength={256} autoComplete="new-password" value={repeat} onChange={event => setRepeat(event.target.value)} /></div>
              </div><p id="password-help" className="text-sm text-muted-foreground">密码至少 15 个字符，建议使用不易猜测的长口令。这个账号可以管理所有站点设置，请妥善保存。</p>
            </>}
            {step === 2 && saved && settings && <>
              <p className="text-sm leading-6 text-muted-foreground">将创建「{site}」，管理员为「{username}」。{saved.database_type === "mysql" ? "已使用部署时配置的 MySQL。" : "已准备好 SQLite 数据库。"} 保留本地存储、测试 UID 留空即可开始使用。</p>
              <InstallationFields value={settings} onChange={setSettings} saved={saved} token={token.trim()} busy={busy} />
            </>}
          </fieldset>
          {error && <p role="alert" className="mt-5 text-sm text-destructive">{error}</p>}
          <div className="mt-8 flex flex-wrap items-center justify-between gap-3 border-t pt-5">
            {step > 0 ? <Button type="button" variant="outline" disabled={busy} onClick={() => { setError(""); setStep(step - 1); }}>上一步</Button> : <span className="text-xs text-muted-foreground">整个过程只需完成一次</span>}
            <Button type="submit" disabled={busy}>{busy ? (step === 2 ? "正在准备站点…" : "正在确认…") : step === 2 ? "完成配置，启用站点" : "下一步"}</Button>
          </div>
        </form>}
    </div>
  </main>;
}
