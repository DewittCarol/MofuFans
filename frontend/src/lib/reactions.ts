import type { EmojiData } from './api';

// HTTP success and WebSocket snapshots may arrive in either order. Counts only grow.
export function mergeReactions(previous: EmojiData[], incoming: EmojiData[]) {
  const counts = new Map(previous.map(item => [item.value, item.count]));
  const increased: string[] = [];
  for (const item of incoming) {
    if (!Number.isSafeInteger(item.count) || item.count < 0) continue;
    if (item.count > (counts.get(item.value) || 0)) {
      counts.set(item.value, item.count);
      increased.push(item.value);
    }
  }
  return { emojis: [...counts].map(([value, count]) => ({ value, count })), increased: [...new Set(increased)] };
}
