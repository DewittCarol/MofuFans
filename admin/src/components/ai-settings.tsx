"use client";

import { useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useAuth } from "@/lib/auth";
import { aiAPI as installationAPI } from "@/lib/ai-api";
import { AIModelPicker } from "@/components/ai-model-picker";

type Protocol = "openai_chat" | "openai_responses" | "anthropic";
type AISettings = {
  enabled: boolean; review_images: boolean; protocol: Protocol; base_url: string; model: string;
  reasoning_effort: string; thinking_mode: string; thinking_budget: number;
  max_tokens: number; chat_token_field: string; spam_prompt: string;
  api_key_configured: boolean;
};
type Draft = Omit<AISettings, "api_key_configured"> & { api_key: string; clear_key: boolean };
const liveRules = "拦截广告、恶意引流、诈骗和明显灌水。\n拦截对主播或其他人的恶意辱骂、威胁、性骚扰及煽动群体仇恨的内容。\n拦截露骨色情、血腥暴力或可能泄露他人住址、联系方式等隐私的内容。\n开启图片审核时，同样检查附图中的画面及可见文字。\n正常消费吐槽、善意玩笑、不同意见不应仅因此拦截；缺乏依据或难以辨认的地方在原因中提示人工复核。";
const defaults: Record<Protocol, string> = {
  openai_chat: "https://api.openai.com/v1", openai_responses: "https://api.openai.com/v1", anthropic: "https://api.anthropic.com/v1",
};
function editable(settings: AISettings): Draft {
  return {
    enabled: settings.enabled, review_images: settings.review_images,
    protocol: settings.protocol, base_url: settings.base_url, model: settings.model,
    reasoning_effort: settings.reasoning_effort, thinking_mode: settings.thinking_mode,
    thinking_budget: settings.thinking_budget, max_tokens: settings.max_tokens,
    chat_token_field: settings.chat_token_field, spam_prompt: settings.spam_prompt,
    api_key: "", clear_key: false,
  };
}
function errorText(error: unknown) {
  if (error instanceof Error && error.name === "TimeoutError") return "请求超时，请检查服务地址，或降低思考等级后重试。";
  return error instanceof Error ? error.message : "连接失败，请稍后重试";
}

export function AISettingsPanel() {
  const { user } = useAuth();
  const owner = !!user?.is_super_admin;
  const [saved, setSaved] = useState<AISettings | null>(null);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [testContent,setTestContent]=useState("");
  const [models, setModels] = useState<{ id: string; name: string }[]>([]);
  const [modelsLoaded, setModelsLoaded] = useState(false);
  const [modelsTruncated, setModelsTruncated] = useState(false);
  const [modelsError, setModelsError] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const load = async () => {
    setBusy("load"); setError("");
    try { const data = await installationAPI<AISettings>("ai-settings"); setSaved(data); setDraft(editable(data)); }
    catch (e) { setError(errorText(e)); } finally { setBusy(""); }
  };
  useEffect(() => { if (owner) void load(); }, [owner]);
  if (!owner) return null;
  const resetModels = () => { setModels([]); setModelsLoaded(false); setModelsTruncated(false); setModelsError(""); };
  const change = <K extends keyof Draft>(key: K, value: Draft[K]) => {
    setDraft(current => current && ({ ...current, [key]: value })); setMessage(""); setError("");
  };
  const run = async (action: "save" | "models" | "test") => {
    if (!draft) return;
    setBusy(action); setError(""); setMessage("");
    if (action === "models") setModelsError("");
    try {
      if (action === "save") {
        const data = await installationAPI<AISettings>("ai-settings", "PUT", draft);
        setSaved(data); setDraft(editable(data)); setMessage("AI 设置已保存，新投稿将使用此配置。");
      } else if (action === "models") {
        const data = await installationAPI<{ models: { id: string; name: string }[]; truncated: boolean }>("ai-settings/models", "POST", draft);
        setModels(data.models);
        setModelsLoaded(true); setModelsTruncated(data.truncated);
        setMessage(data.models.length ? `已获取 ${data.models.length} 个模型${data.truncated ? "（列表未全部返回）" : ""}。点击“选择模型”查看完整列表，也可以直接填写模型 ID。` : "服务未返回可用模型，可以直接填写模型 ID。");
      } else {
        const data = await installationAPI<{ message: string; is_spam: boolean; reason: string; images_tested: number }>("ai-settings/test", "POST", {...draft,test_content:testContent}, undefined, 125000);
        setMessage(`${data.message}：${data.is_spam?"建议拦截":"未发现明显问题"}。原因：${data.reason}。${data.images_tested?"已同时发送一张测试图片。":"本次仅测试文字。"}修改后仍需保存设置。`);
      }
    } catch (e) { setError(errorText(e)); if (action === "models") setModelsError(errorText(e)); } finally { setBusy(""); }
  };
  const changedEndpoint = !!draft && !!saved?.api_key_configured && (draft.base_url.replace(/\/+$/, "") !== saved.base_url.replace(/\/+$/, "") || draft.protocol !== saved.protocol);
  const missingKey = !!draft && (draft.clear_key || (!draft.api_key.trim() && (!saved?.api_key_configured || changedEndpoint)));
  const dirty = !!draft && !!saved && JSON.stringify(draft) !== JSON.stringify(editable(saved));
  return <Card id="ai-settings" tabIndex={-1} aria-label="AI 辅助审核">
    <CardHeader><CardTitle>AI 辅助审核</CardTitle><CardDescription>按你设定的标准检查新投稿，记录原因，将命中的稿件标为低质量供人工复核。后台排队，失败保留投稿；不会自动公开或删除。</CardDescription></CardHeader>
    <CardContent className="space-y-5">
      {!draft && <Button variant="outline" disabled={!!busy} onClick={() => void load()}>{busy ? "正在读取…" : "重新读取 AI 设置"}</Button>}
      {draft && <fieldset disabled={!!busy} className="min-w-0 space-y-5 disabled:opacity-70">
        <label className="flex items-center gap-2 text-sm font-medium"><input type="checkbox" className="h-4 w-4 accent-primary" checked={draft.enabled} onChange={e => change("enabled", e.target.checked)} />开启 AI 辅助审核</label>
        <div className="max-w-3xl space-y-2">
          <label className="flex items-start gap-2 text-sm font-medium"><input type="checkbox" className="mt-1 h-4 w-4 accent-primary" checked={draft.review_images} onChange={e=>change("review_images",e.target.checked)} />同时审核投稿附图（需要支持图片的模型）</label>
          <p className="text-sm text-muted-foreground">默认关闭。开启后将投稿附图发送给所配置的模型，可能增加用量；关闭时只审文字并标明图片未审。图片会缩小处理，小字仍需人工复核；动图会提示人工检查。</p>
        </div>
        <div className="grid max-w-3xl gap-5 sm:grid-cols-2">
          <div className="space-y-2"><Label htmlFor="ai-protocol">接口协议</Label><Select disabled={!!busy} value={draft.protocol} onValueChange={(value: Protocol) => {
            const base_url = Object.values(defaults).includes(draft.base_url) ? defaults[value] : draft.base_url;
            setDraft({ ...draft, protocol: value, base_url, reasoning_effort: "", thinking_mode: "", api_key: "" }); resetModels(); setMessage(""); setError("");
          }}><SelectTrigger id="ai-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="openai_chat">OpenAI · Chat Completions</SelectItem><SelectItem value="openai_responses">OpenAI · Responses</SelectItem><SelectItem value="anthropic">Anthropic · Messages</SelectItem></SelectContent></Select></div>
          <div className="space-y-2"><Label htmlFor="ai-base-url">Base URL</Label><Input id="ai-base-url" type="url" maxLength={2048} autoComplete="off" spellCheck={false} value={draft.base_url} onChange={e => { change("base_url", e.target.value); resetModels(); }} placeholder={defaults[draft.protocol]} /></div>
        </div>
        <p className="text-sm text-muted-foreground">填写 API 基础地址（通常以 /v1 结尾），无需追加 /messages、/responses 或 /chat/completions。兼容服务可使用自己的地址。</p>
        <div className="max-w-3xl space-y-2"><Label htmlFor="ai-key">API Key{saved?.api_key_configured ? " · 已配置" : ""}</Label><Input id="ai-key" type="password" value={draft.api_key} disabled={!!busy || draft.clear_key} onChange={e => { change("api_key", e.target.value); resetModels(); }} autoComplete="new-password" maxLength={8192} placeholder={saved?.api_key_configured ? "留空保留已保存的密钥" : "填写服务提供方的 API Key"} />
          <p className="text-sm text-muted-foreground">密钥加密保存在服务器，仅站点所有者可修改，不会回传到浏览器。{changedEndpoint ? "服务地址或协议已修改，请重新填写对应密钥。" : ""}</p>
          {saved?.api_key_configured && <label className="flex items-center gap-2 text-sm"><input type="checkbox" className="h-4 w-4 accent-primary" checked={draft.clear_key} onChange={e => setDraft({ ...draft, clear_key: e.target.checked, enabled: e.target.checked ? false : draft.enabled, api_key: "" })} />保存时清除密钥并停用审核</label>}
        </div>
        <div className="max-w-3xl space-y-2"><Label htmlFor="ai-model">使用的模型 ID</Label><div className="flex flex-wrap gap-2">
          <Input id="ai-model" className="min-w-0 flex-1 basis-56" maxLength={200} value={draft.model} onChange={e => change("model", e.target.value)} placeholder="选择或填写服务提供方的模型 ID" autoComplete="off" spellCheck={false} />
          <AIModelPicker models={models} value={draft.model} loaded={modelsLoaded} loading={busy === "models"} truncated={modelsTruncated} disabled={!!busy && busy !== "models"} canFetch={!missingKey} error={modelsError} onFetch={() => void run("models")} onSelect={value => change("model", value)} />
          <Button variant="outline" disabled={!!busy || missingKey} onClick={() => void run("models")}>{busy === "models" ? "正在获取…" : modelsLoaded ? "刷新列表" : "获取模型"}</Button></div>
          <p className="text-sm text-muted-foreground">“选择模型”会显示服务返回的全部模型，可在弹层中单独搜索；当前填写的模型不会缩小列表。接口不提供列表时，也可手动填写 ID。选择后需保存才会生效。</p>
        </div>
        <details className="max-w-3xl rounded-lg border border-border p-4"><summary className="cursor-pointer font-medium">思考与兼容性选项</summary><div className="mt-4 space-y-5">
          <p className="text-sm text-muted-foreground">默认跟随服务。不确定模型是否支持时，保持默认；高思考等级可能增加响应时间与用量。</p>
          <div className="grid gap-5 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="ai-effort">思考等级</Label><Select disabled={!!busy} value={draft.reasoning_effort || "default"} onValueChange={v => change("reasoning_effort", v === "default" ? "" : v)}><SelectTrigger id="ai-effort"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="default">服务默认（不发送此参数）</SelectItem>{(draft.protocol === "anthropic" ? ["low", "medium", "high", "xhigh", "max"] : ["none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra"]).map(v => <SelectItem key={v} value={v}>{v}</SelectItem>)}</SelectContent></Select></div>
          <div className="space-y-2"><Label htmlFor="ai-max-tokens">输出 Token 上限</Label><Input id="ai-max-tokens" type="number" min={128} max={131072} value={draft.max_tokens} onChange={e => change("max_tokens", Number(e.target.value))} /></div></div>
          {draft.protocol === "anthropic" && <div className="grid gap-5 sm:grid-cols-2"><div className="space-y-2"><Label htmlFor="ai-thinking">Anthropic 思考模式</Label><Select disabled={!!busy} value={draft.thinking_mode || "default"} onValueChange={v => change("thinking_mode", v === "default" ? "" : v)}><SelectTrigger id="ai-thinking"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="default">服务默认</SelectItem><SelectItem value="adaptive">adaptive · 自适应</SelectItem><SelectItem value="enabled">enabled · 手动预算</SelectItem><SelectItem value="disabled">disabled · 关闭</SelectItem><SelectItem value="between_tools">between_tools · 工具间思考</SelectItem></SelectContent></Select></div>{draft.thinking_mode === "enabled" && <div className="space-y-2"><Label htmlFor="ai-budget">思考 Token 预算</Label><Input id="ai-budget" type="number" min={1024} max={draft.max_tokens - 1} value={draft.thinking_budget} onChange={e => change("thinking_budget", Number(e.target.value))} /><p className="text-xs text-muted-foreground">至少 1024，且小于输出 Token 上限。</p></div>}</div>}
          {draft.protocol === "openai_chat" && <div className="space-y-2"><Label htmlFor="ai-token-field">Token 参数</Label><Select disabled={!!busy} value={draft.chat_token_field} onValueChange={v => change("chat_token_field", v)}><SelectTrigger id="ai-token-field"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="max_completion_tokens">max_completion_tokens · OpenAI</SelectItem><SelectItem value="max_tokens">max_tokens · 兼容接口</SelectItem></SelectContent></Select><p className="text-xs text-muted-foreground">兼容服务若不接受默认参数，可改用 max_tokens。DeepSeek 等服务可选择这一项。</p></div>}
          <p className="text-xs text-muted-foreground">各模型接受的等级与思考模式不同，请以服务文档为准。测试连接会验证当前组合。</p>
        </div></details>
        <div className="max-w-3xl space-y-2"><div className="flex flex-wrap items-center justify-between gap-2"><Label htmlFor="ai-rules">投稿审核标准</Label><Button size="sm" variant="outline" onClick={()=>change("spam_prompt",liveRules)}>填入直播投稿示例标准</Button></div><Textarea id="ai-rules" rows={7} maxLength={4000} value={draft.spam_prompt} onChange={e => change("spam_prompt", e.target.value)} /><p className="text-sm text-muted-foreground">每行描述一类需要拦截的内容。示例按钮会替换下方草稿，保存后才生效；可按话题和主播接受程度修改。已有标准会保留，不会自动替换。</p></div>
        <details className="max-w-3xl space-y-3"><summary className="cursor-pointer text-sm font-medium">用自己的文字试审（不保存为投稿）</summary><div className="space-y-2 pt-3"><Label htmlFor="ai-test-content">测试文字</Label><Textarea id="ai-test-content" rows={3} maxLength={4000} value={testContent} onChange={e=>setTestContent(e.target.value)} placeholder="留空使用固定示例；可粘贴一段文字检查审核标准是否合适" /></div></details>
        <div className="flex flex-wrap gap-3"><Button disabled={!!busy || !dirty || (draft.enabled && (missingKey || !draft.model.trim()))} onClick={() => void run("save")}>{busy === "save" ? "正在保存…" : "保存 AI 设置"}</Button><Button variant="outline" disabled={!!busy || missingKey || !draft.model.trim()} onClick={() => void run("test")}>{busy === "test" ? "正在测试，最多约 2 分钟…" : "测试连接与模型"}</Button></div>
        <p className="text-xs text-muted-foreground">测试会向当前地址发送示例或填写的文字；开启图片审核时还会发送一张测试图片，消耗少量 API 用量。AI 设置单独保存，无需重启。</p>
      </fieldset>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {message && <p role="status" className="text-sm text-primary">{message}</p>}
    </CardContent>
  </Card>;
}
