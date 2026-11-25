export type WhisperHelper = 'userscript' | 'extension';
type Reply = { ok: boolean; error?: string; kind?: WhisperHelper; version?: string | number };
const extensionID = typeof process === 'undefined' ? '' : process.env.NEXT_PUBLIC_WHISPER_EXTENSION_ID || '';
let scriptTransport: 'dom' | 'window' = 'window';
let helperDetail = '';

function domRequest(type: string, payload: Record<string, string> = {}, timeout = 1800): Promise<Reply> {
  return new Promise((resolve, reject) => {
    const id = crypto.randomUUID();
    const listener = (event: Event) => {
      const detail = (event as CustomEvent).detail;
      if (typeof detail !== 'string' || detail.length > 8000) return;
      let result;
      try { result = JSON.parse(detail); } catch { return; }
      if (result?.channel !== 'fans-whisper-response' || result.id !== id) return;
      clearTimeout(timer); document.removeEventListener('fans-whisper-response-v2', listener); resolve(result);
    };
    const timer = window.setTimeout(() => { document.removeEventListener('fans-whisper-response-v2', listener); reject(new Error('油猴助手未响应')); }, timeout);
    document.addEventListener('fans-whisper-response-v2', listener);
    document.dispatchEvent(new CustomEvent('fans-whisper-request-v2', { detail: JSON.stringify({ channel:'fans-whisper-request', id, type, ...payload }) }));
  });
}

function scriptRequest(type: string, payload: Record<string, string> = {}, timeout = 1200): Promise<Reply> {
  return new Promise((resolve, reject) => {
    const id = crypto.randomUUID();
    const listener = (event: MessageEvent) => {
      if (event.source !== window || event.origin !== location.origin || event.data?.channel !== 'fans-whisper-response' || event.data.id !== id) return;
      clearTimeout(timer); window.removeEventListener('message', listener); resolve(event.data);
    };
    const timer = window.setTimeout(() => { window.removeEventListener('message', listener); reject(new Error('油猴助手未响应')); }, timeout);
    window.addEventListener('message', listener);
    window.postMessage({ channel: 'fans-whisper-request', id, type, ...payload }, location.origin);
  });
}
function extensionRequest(type: string, payload: Record<string, string> = {}): Promise<Reply> {
  type Runtime = { lastError?: { message?: string }; sendMessage: (id: string, message: unknown, callback: (reply?: Reply) => void) => void };
  const runtime = (window as unknown as { chrome?: { runtime?: Runtime } }).chrome?.runtime;
  return new Promise((resolve, reject) => {
    if (!extensionID || !runtime?.sendMessage) { reject(new Error('尚未安装扩展')); return; }
    const timer = window.setTimeout(() => reject(new Error('扩展未响应，请检查 B 站登录状态')), type === 'fans:ping' ? 1500 : 45000);
    try {
      runtime.sendMessage(extensionID, { type, ...payload }, reply => {
        clearTimeout(timer);
        if (runtime.lastError || !reply) reject(new Error('扩展未连接，请刷新页面后检测')); else resolve(reply);
      });
    } catch { clearTimeout(timer); reject(new Error('扩展未连接')); }
  });
}
let pendingDetection: Promise<WhisperHelper | null> | null = null;
export function detectWhisperHelper(): Promise<WhisperHelper | null> {
  // Page entry and a quick first click can share the same in-flight probe.
  if (!pendingDetection) pendingDetection = findWhisperHelper().finally(() => { pendingDetection = null; });
  return pendingDetection;
}
async function findWhisperHelper(): Promise<WhisperHelper | null> {
  helperDetail = '';
  for (const transport of ['dom', 'window'] as const) {
    try {
      const result = await (transport === 'dom' ? domRequest : scriptRequest)('fans:ping');
      if (result.ok) { scriptTransport = transport; helperDetail = typeof result.version === 'string' ? `油猴脚本 ${result.version} 已连接。` : '油猴脚本已连接；建议更新到 1.1.2，以兼容认证码文案和 Windows 粘贴。'; return 'userscript'; }
      if (result.error) helperDetail = result.error;
    } catch { /* Try the older script bridge, then the extension. */ }
  }
  try { const result = await extensionRequest('fans:ping'); if (result.ok) { helperDetail = '浏览器扩展已连接。'; return 'extension'; } } catch { /* Copy flow stays available. */ }
  if (!helperDetail) {
    try {
      const raw = document.documentElement.getAttribute('data-fans-whisper-helper');
      const state = raw && raw.length < 1000 ? JSON.parse(raw) : null;
      if (state?.state === 'error' && typeof state.error === 'string') helperDetail = state.error;
      else if (state?.version) helperDetail = `检测到脚本 ${String(state.version).slice(0,20)} 已启动，但通信没有完成。请刷新页面后重试。`;
    } catch { /* Unknown marker is not evidence of a working connection. */ }
  }
  if (!helperDetail) helperDetail = '未收到脚本响应。请确认 Tampermonkey 允许用户脚本运行、已安装完整脚本，再刷新本页。脚本开关显示启用，并不代表浏览器已允许它执行。';
  return null;
}
export function whisperHelperDetail() { return helperDetail; }
export async function prefillWhisper(helper: WhisperHelper, uid: string, text: string) {
  text = text.replace(/\r\n?/g, '\n');
  const result = helper === 'userscript' ? await (scriptTransport === 'dom' ? domRequest : scriptRequest)('fans:prefill', { uid, text }, 45000) : await extensionRequest('fans:prefill', { uid, text });
  if (!result.ok) throw new Error(result.error || '填入失败，请使用复制并打开私信');
}

export async function connectAndPrefillWhisper(uid: string, text: string) {
  const helper = await detectWhisperHelper();
  if (!helper) throw new Error(whisperHelperDetail() || '未连接私信助手，请刷新页面后重试');
  await prefillWhisper(helper, uid, text);
}
