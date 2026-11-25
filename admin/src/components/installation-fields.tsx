"use client";

import { useId, useState } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { installationAPI, installationError, type InstallationSettings, type InstallationUpdate } from "@/lib/installation";

export function InstallationFields({ value, onChange, saved, token, busy = false }: {
  value: InstallationUpdate; onChange: (value: InstallationUpdate) => void; saved: InstallationSettings; token?: string; busy?: boolean;
}) {
  const id = useId();
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState("");
  const [failed, setFailed] = useState(false);
  const locked = (key: string) => saved.locked.includes(key);
  const change = (next: InstallationUpdate) => { setResult(""); onChange(next); };
  async function test() {
    setTesting(true); setResult("");
    try {
      await installationAPI(token ? "setup/storage-test" : "installation-settings/test", "POST", value, token);
      setFailed(false); setResult("OSS 读写检查通过。保存设置后才会使用此配置。");
    } catch (error) { setFailed(true); setResult(installationError(error)); }
    finally { setTesting(false); }
  }
  const fields = [
    ["endpoint", "Endpoint", "https://oss-cn-hangzhou.aliyuncs.com"],
    ["region", "地域", "cn-hangzhou"],
    ["bucket", "Bucket 名称", "你的私有 Bucket 名称"],
    ["prefix", "文件前缀", "mofufans"],
    ["access_key", "AccessKey ID", saved.oss.has_access_key ? "已保存，留空保持不变" : "填写 AccessKey ID"],
    ["secret_key", "AccessKey Secret", saved.oss.has_secret_key ? "已保存，留空保持不变" : "填写 AccessKey Secret"],
  ] as const;
  return <fieldset disabled={busy || testing} className="min-w-0 space-y-6">
    <legend className="sr-only">存储与集成配置</legend>
    {saved.locked.length > 0 && <p className="text-sm text-muted-foreground">标有「环境变量」的项目由部署配置控制，请在服务器修改对应环境变量后重启。</p>}
    <fieldset className="space-y-3" disabled={locked("storage_type")}>
      <legend className="mb-2 font-medium">文件存储 {locked("storage_type") && <span className="text-sm text-muted-foreground">· 环境变量</span>}</legend>
      <label className="flex items-start gap-3 rounded-lg border p-4 cursor-pointer">
        <input type="radio" name={`${id}-storage`} value="local" checked={value.storage_type === "local"} onChange={() => change({ ...value, storage_type: "local" })} className="mt-1 accent-[var(--primary)]" />
        <span><span className="block font-medium">本地存储</span><span className="text-sm text-muted-foreground">文件保存在这台服务器。无需开通其他服务，适合先开始使用。</span></span>
      </label>
      <label className="flex items-start gap-3 rounded-lg border p-4 cursor-pointer">
        <input type="radio" name={`${id}-storage`} value="oss" checked={value.storage_type === "oss"} onChange={() => change({ ...value, storage_type: "oss" })} className="mt-1 accent-[var(--primary)]" />
        <span><span className="block font-medium">阿里云 OSS</span><span className="text-sm text-muted-foreground">将上传文件保存在自己的私有 Bucket，需要填写访问凭证。</span></span>
      </label>
    </fieldset>
    {value.storage_type === "oss" && <div className="space-y-4 border-l-2 pl-4">
      <p className="text-sm text-muted-foreground">请在 OSS 控制台创建私有 Bucket；Endpoint 和地域必须匹配。凭证需要此前缀的读取、写入、列举、删除和对象 ACL 权限。密钥保存后不会回显。</p>
      <div className="grid gap-4 sm:grid-cols-2">{fields.map(([key, label, placeholder]) => <div className="space-y-2" key={key}>
        <Label htmlFor={`${id}-${key}`}>{label}{locked(`oss.${key}`) ? " · 环境变量" : ""}</Label>
        <Input id={`${id}-${key}`} value={value.oss[key]} type={key.includes("key") ? "password" : "text"} autoComplete="off" spellCheck={false}
          disabled={locked(`oss.${key}`)} placeholder={placeholder} onChange={event => change({ ...value, oss: { ...value.oss, [key]: event.target.value } })} />
      </div>)}</div>
      <Button type="button" variant="outline" onClick={test} disabled={busy || testing}>{testing ? "正在检查 OSS…" : "检查 OSS 连接"}</Button>
      <p className="text-xs text-muted-foreground">检查会在此前缀下写入、读取并删除一个临时文件；保存 OSS 配置时也会进行检查。</p>
      {result && <p role={failed ? "alert" : "status"} className={`text-sm ${failed ? "text-destructive" : "text-foreground"}`}>{result}</p>}
    </div>}
    <div className="space-y-2">
      <Label htmlFor={`${id}-uid`}>私信测试收件 UID（选填）{locked("bilibili.test_recipient_uid") ? " · 环境变量" : ""}</Label>
      <Input id={`${id}-uid`} value={value.test_recipient_uid} inputMode="numeric" pattern="[1-9][0-9]{0,18}" maxLength={19} disabled={locked("bilibili.test_recipient_uid")}
        placeholder="填写自己用于收取测试私信的 B 站 UID" onChange={event => change({ ...value, test_recipient_uid: event.target.value })} aria-describedby={`${id}-uid-help`} />
      <p id={`${id}-uid-help`} className="text-sm text-muted-foreground">默认没有测试账号。留空可正常使用站点，创建私信队列前需要补填，并先确认测试私信收到。</p>
    </div>
    <label className="flex items-start gap-3">
      <input type="checkbox" checked={value.secure_cookie} disabled={locked("security.secure_cookie")} onChange={event => change({ ...value, secure_cookie: event.target.checked })} className="mt-1" />
      <span><span className="block font-medium">仅通过 HTTPS 传输登录 Cookie{locked("security.secure_cookie") ? " · 环境变量" : ""}</span>
        <span className="text-sm text-muted-foreground">公网 HTTPS 站点建议开启。本机 HTTP 调试时关闭，否则浏览器无法保持登录。</span></span>
    </label>
  </fieldset>;
}
