installMofuWhisperComposerStyle();
installMofuWhisperPasteFix();
chrome.runtime.onMessage.addListener((message, sender, respond) => {
  if (sender.id !== chrome.runtime.id) return false;
  if (message?.type === 'fans:ready') { respond({ ok: true }); return false; }
  if (message?.type !== 'fans:fill') return false;
  fillMofuWhisper(message).then(respond, () => respond({ ok:false, error:'填入失败，请复制通知后手动粘贴' }));
  return true;
});
