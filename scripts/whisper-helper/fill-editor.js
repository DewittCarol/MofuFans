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
