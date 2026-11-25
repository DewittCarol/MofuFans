// ==UserScript==
// @name         MofuFans 私信填入助手
// @namespace    https://example.com/
// @version      1.2.0
// @description  加高 B 站私信输入区，将后台选定的舰礼或电子礼物通知填入对应私信，发送由你确认。仅主播需要安装。
// @match        https://example.com/admin/*
// @match        https://preview.example.com/admin/*
// @match        http://127.0.0.1:18081/admin/*
// @match        http://localhost:18081/admin/*
// @match        https://message.bilibili.com/*
// @grant        GM_getValue
// @grant        GM_setValue
// @grant        GM_deleteValue
// @grant        GM_listValues
// @grant        GM_openInTab
// @run-at       document-idle
// @sandbox      DOM
// @noframes
// ==/UserScript==


/* Shared by the extension and userscript. Never submits a message. */
function installMofuWhisperComposerStyle() {
  if (location.hostname !== 'message.bilibili.com' || document.getElementById('fans-whisper-layout')) return;
  const style = document.createElement('style');
  style.id = 'fans-whisper-layout';
  // Bilibili's footer includes the toolbar and send row. Its textarea uses
  // height: calc(100% - 102px), so expand the footer rather than its editor.
  // Match the CSS-module prefix without depending on a particular build hash.
  // A stylesheet also covers editors recreated when switching conversations.
  style.textContent = `
    [class*="_ChatFooter_"]:has(.msb-textarea .brt-editor[contenteditable="true"]) {
      height: clamp(220px, 46vh, 370px) !important;
      min-height: 162px !important;
      max-height: 65% !important;
    }
  `;
  (document.head || document.documentElement).appendChild(style);
}

function normalizeMofuWhisperText(text) {
  return text.replace(/\r\n?/g, '\n');
}

function insertMofuWhisperText(text) {
  const lines = normalizeMofuWhisperText(text).split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (i && !document.execCommand('insertLineBreak', false)) return false;
    if (lines[i] && !document.execCommand('insertText', false, lines[i])) return false;
  }
  return true;
}

function installMofuWhisperPasteFix() {
  if (location.hostname !== 'message.bilibili.com') return;
  document.addEventListener('paste', event => {
    const editor = event.target?.closest?.('div.brt-editor[contenteditable="true"]');
    const original = event.clipboardData?.getData('text/plain');
    // Only touch MofuFans claim notifications with Windows line endings.
    // Ordinary messages, images, and intentionally empty lines stay untouched.
    if (!editor || !original?.includes('\r') || !(/UID\s*[:：]/i.test(original) || /MofuFans/.test(original)) || !/(?:认证码|领取码|礼物密码)\s*[:：]/.test(original)) return;
    const text = normalizeMofuWhisperText(original);
    if (text.length > 500) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    editor.focus();
    insertMofuWhisperText(text);
  }, true);
}

async function fillMofuWhisper(job) {
  const deadline = Date.now() + 25000;
  const correctRecipient = () => location.hostname === 'message.bilibili.com' && location.hash.split('?')[0] === '#/whisper/mid' + job.uid;
  if (!/^[1-9]\d{0,18}$/.test(job.uid) || typeof job.text !== 'string' || !job.text.trim() || job.text.length > 500) return { ok: false, error: '私信内容无效或超过 500 字' };
  const text = normalizeMofuWhisperText(job.text);
  let stableEditor = null;
  let stableSince = 0;
  while (Date.now() < deadline) {
    if (!correctRecipient()) return { ok: false, error: '私信对象已改变，已取消填入' };
    const editor = document.querySelector('div.brt-editor[contenteditable="true"]');
    if (editor && editor.getClientRects().length) {
      if (editor.innerText.trim()) return { ok: false, error: '输入框已有草稿，请先处理草稿，再重新点击自动填入' };
      // The SPA can replace its first editor during startup. Wait for a stable node.
      if (stableEditor !== editor) { stableEditor = editor; stableSince = Date.now(); }
      if (Date.now() - stableSince < 900) { await new Promise(resolve => setTimeout(resolve, 300)); continue; }
      editor.focus();
      // Native line breaks match Bilibili's <br> layout and preserve blank lines.
      const inserted = insertMofuWhisperText(text);
      await new Promise(resolve => setTimeout(resolve, 900));
      if (!correctRecipient()) return { ok: false, error: '私信对象已改变，请检查当前草稿' };
      if (document.querySelector('div.brt-editor[contenteditable="true"]') !== editor) return { ok:false, error:'B 站重新加载了输入框，请返回后台重试填入' };
      const actual = normalizeMofuWhisperText(editor.innerText).trim();
      if (!inserted || actual !== text.trim()) return { ok: false, error: '编辑器未完整接收文本，请回后台复制通知后手动粘贴' };
      const banner = document.createElement('div');
      banner.textContent = 'MofuFans 已填入通知 · 请核对收件人与内容后发送';
      banner.setAttribute('role', 'status');
      Object.assign(banner.style, { position:'fixed', bottom:'20px', left:'50%', transform:'translateX(-50%)', zIndex:'2147483647', padding:'12px 18px', borderRadius:'8px', background:'#fff6ed', color:'#633d35', border:'1px solid #dfc2ac', font:'14px/1.6 sans-serif', maxWidth:'85vw' });
      document.body.appendChild(banner);
      setTimeout(() => banner.remove(), 8000);
      return { ok: true };
    }
    await new Promise(resolve => setTimeout(resolve, 300));
  }
  return { ok: false, error: '没有找到私信输入框，请先在此浏览器登录 B 站，再重试' };
}


(async () => {
  'use strict';
  const prefix = 'fans-whisper-job:';
  const ttl = 60000;
  const version = '1.2.0';
  const allowed = new Set(['https://example.com', 'https://preview.example.com', 'http://127.0.0.1:18081', 'http://localhost:18081']);
  const isBili = location.hostname === 'message.bilibili.com';
  if (!isBili && !allowed.has(location.origin)) return;
  if (isBili) {
    installMofuWhisperComposerStyle();
    installMofuWhisperPasteFix();
  }
  const report = (state, error = '') => {
    document.documentElement.setAttribute('data-fans-whisper-helper', JSON.stringify({version, state, error}));
  };
  report('starting');
  const storageAvailable = typeof GM_getValue === 'function' && typeof GM_setValue === 'function' && typeof GM_deleteValue === 'function' && typeof GM_listValues === 'function';
  const storageError = '脚本已运行，但无法使用油猴存储。请完整安装新版脚本，并检查脚本管理器的运行权限。';
  if (!storageAvailable || typeof GM_openInTab !== 'function') { report('error', storageError); return; }
  report('ready');
  // Cleanup never blocks listener registration or hides the startup error.
  const sweep = async () => {
    for (const key of await GM_listValues()) {
      if (!key.startsWith(prefix)) continue;
      const value = await GM_getValue(key);
      if (!value || value.expires < Date.now()) await GM_deleteValue(key);
    }
  };
  void sweep().catch(() => report('error', storageError));
  if (location.hostname === 'message.bilibili.com') {
    const url = new URL(location.href);
    const id = url.searchParams.get('fans_draft');
    if (!id || !/^[a-f0-9-]{36}$/.test(id)) return;
    const key = prefix + id;
    let job;
    try { job = await GM_getValue(key); } catch { report('error', storageError); return; }
    if (!job || job.expires < Date.now() || job.claimed) return;
    // Claim before awaiting the editor so reloads cannot replay a draft.
    try { await GM_setValue(key, { expires: job.expires, claimed: true }); } catch { report('error', storageError); return; }
    url.searchParams.delete('fans_draft');
    history.replaceState(history.state, '', url.href);
    let result;
    document.documentElement.setAttribute('data-fans-whisper-fill', 'waiting');
    try { result = await fillMofuWhisper(job); } catch { result = { ok: false, error: '填入失败，请回到后台复制后手动粘贴' }; }
    document.documentElement.setAttribute('data-fans-whisper-fill', result.ok ? 'filled' : 'error');
    try { await GM_setValue(key, { expires: job.expires, result }); } catch { report('error', storageError); }
    setTimeout(() => { void Promise.resolve(GM_deleteValue(key)).catch(() => {}); }, ttl);
    return;
  }
  let running = false;
  const received = new Set();
  const handle = async (message, respond) => {
    if (!allowed.has(location.origin) || !/^\/admin\/dashboard\/(?:voyage|gifts)(?:\/|$)/.test(location.pathname)) return;
    if (message?.channel !== 'fans-whisper-request' || typeof message.id !== 'string' || !/^[a-f0-9-]{36}$/.test(message.id)) return;
    const reply = result => respond({ channel:'fans-whisper-response', id:message.id, version, ...result });
    if (message.type === 'fans:ping') {
      try { await GM_getValue(prefix + 'health'); report('ready'); reply({ ok:true, kind:'userscript' }); }
      catch { report('error', storageError); reply({ ok:false, error:storageError }); }
      return;
    }
    if (message.type !== 'fans:prefill' || !/^[1-9]\d{0,18}$/.test(message.uid) || typeof message.text !== 'string' || !message.text.trim() || message.text.length > 500) { reply({ ok:false, error:'私信内容无效或超过 500 字' }); return; }
    if (received.has(message.id)) return;
    if (running) { reply({ ok:false, error:'上一条通知仍在填入，请稍候' }); return; }
    received.add(message.id);
    setTimeout(() => received.delete(message.id), ttl);
    running = true;
    const key = prefix + message.id;
    try {
      await GM_setValue(key, { uid:message.uid, text:message.text, expires:Date.now()+ttl });
      // Only an opaque handoff ID goes into the URL, never the code or message.
      GM_openInTab('https://message.bilibili.com/?fans_draft=' + message.id + '#/whisper/mid' + message.uid, { active:true, insert:true });
      const deadline = Date.now()+40000;
      while (Date.now()<deadline) {
        await new Promise(resolve => setTimeout(resolve,400));
        const value = await GM_getValue(key);
        if (value?.result) { reply(value.result); return; }
      }
      reply({ ok:false, error:'未收到填入结果，请检查 B 站是否已登录、油猴脚本是否在两边启用，再重试' });
    } catch { reply({ ok:false, error:'油猴助手未能打开私信，请使用复制并打开私信' }); }
    finally { try { await GM_deleteValue(key); } catch { report('error', storageError); } running = false; }
  };
  // Strings cross userscript isolation worlds without sharing Window wrappers.
  document.addEventListener('fans-whisper-request-v2', event => {
    if (typeof event.detail !== 'string' || event.detail.length > 8000) return;
    let message; try { message = JSON.parse(event.detail); } catch { return; }
    void handle(message, result => document.dispatchEvent(new CustomEvent('fans-whisper-response-v2', {detail:JSON.stringify(result)})));
  });
  // Keep compatibility with already-deployed admin pages.
  window.addEventListener('message', event => {
    if (event.source !== window || event.origin !== location.origin) return;
    void handle(event.data, result => window.postMessage(result, location.origin));
  });
})();
