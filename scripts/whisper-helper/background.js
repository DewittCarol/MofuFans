const allowedOrigins = new Set(['https://example.com', 'https://preview.example.com', 'http://127.0.0.1:18081', 'http://localhost:18081']);
let running = false;
chrome.runtime.onMessageExternal.addListener((message, sender, respond) => {
  let url;
  try { url = new URL(sender.url); } catch { return false; }
  if (!allowedOrigins.has(url.origin) || !/^\/admin\/dashboard\/(?:voyage|gifts)(?:\/|$)/.test(url.pathname)) return false;
  if (message?.type === 'fans:ping') { respond({ ok: true, kind: 'extension', version: 1 }); return false; }
  if (message?.type !== 'fans:prefill' || !/^[1-9]\d{0,18}$/.test(message.uid) || typeof message.text !== 'string' || !message.text.trim() || message.text.length > 500) { respond({ ok: false, error: '通知格式无效或超过 500 字' }); return false; }
  if (running) { respond({ ok: false, error: '上一条通知仍在填入，请稍候' }); return false; }
  running = true;
  (async () => {
    const tab = await chrome.tabs.create({ url: 'https://message.bilibili.com/#/whisper/mid' + message.uid, active: true });
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      try {
        const ready = await chrome.tabs.sendMessage(tab.id, { type: 'fans:ready' });
        if (ready?.ok) return await chrome.tabs.sendMessage(tab.id, { type: 'fans:fill', uid: message.uid, text: message.text });
      } catch { /* New tab's content script is not loaded yet. */ }
      await new Promise(resolve => setTimeout(resolve, 300));
    }
    return { ok: false, error: 'B 站页面未能加载，请登录 B 站后重试，或使用复制并打开私信' };
  })().then(respond, () => respond({ ok: false, error: '打开私信失败，请使用复制并打开私信' })).finally(() => { running = false; });
  return true;
});
