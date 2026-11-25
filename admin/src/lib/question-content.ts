type ContentEmoji = { kind: string; value: string; url: string; name: string };

export function escapeHTML(value: string): string {
  return value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

export function renderQuestionContent(content: string, emojis: readonly ContentEmoji[]): string {
  const input = new Map(emojis.filter(item => item.kind === 'input' && item.url.startsWith('/') && !item.url.startsWith('//'))
    .map(item => [item.value, item]));
  const tags = [...input.keys()].sort((a, b) => b.length - a.length)
    .map(value => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'));
  const tokens = new RegExp([...tags, 'https?:\\/\\/[^\\s<>"\\[\\]]+', 'BV[a-zA-Z0-9]{10}'].join('|'), 'g');
  const inline = (text: string) => {
    let result = '', last = 0;
    for (const match of text.matchAll(tokens)) {
      result += escapeHTML(text.slice(last, match.index));
      const value = match[0], emoji = input.get(value);
      if (emoji) {
        result += `<img src="${escapeHTML(emoji.url)}" alt="${escapeHTML(emoji.name)}" title="${escapeHTML(emoji.name)}" style="display:inline-block;height:64px;width:64px;vertical-align:middle;margin:0 2px;object-fit:contain;" />`;
      } else {
        const href = value.startsWith('BV') ? `https://www.bilibili.com/video/${value}` : value;
        result += `<a href="${escapeHTML(href)}" target="_blank" rel="noopener noreferrer" style="color:var(--primary);text-decoration:underline;">${escapeHTML(value)}</a>`;
      }
      last = match.index! + value.length;
    }
    return result + escapeHTML(text.slice(last));
  };
  const text = content.replace(/｛/g, '{').replace(/｝/g, '}');
  let result = '', last = 0;
  for (const match of text.matchAll(/\{\{([\s\S]*?)\}\}/g)) {
    result += inline(text.slice(last, match.index));
    result += `<span class="question-spoiler" tabindex="0">${inline(match[1])}</span>`;
    last = match.index! + match[0].length;
  }
  return result + inline(text.slice(last));
}
