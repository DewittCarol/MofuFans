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
