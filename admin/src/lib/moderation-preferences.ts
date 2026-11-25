export type ModerationView = 'list' | 'cards';
export type ModerationPreferences = { blurEnabled: boolean; view: ModerationView };

export const defaultPreferences: ModerationPreferences = { blurEnabled: true, view: 'list' };

export function moderationStoragePrefix(user: { id: number; username: string }) {
  return `joiask:moderation:v1:${user.id}:${encodeURIComponent(user.username)}:`;
}

// Image identity is independent of the question, thumbnail position and route.
// Keep query strings: external services may use them to identify different images.
export function imageIdentity(src: string, origin: string): string {
  try {
    const url = new URL(src, `${origin}/`);
    url.hash = '';
    return url.origin === origin ? url.pathname + url.search : url.href;
  } catch { return src; }
}

export function readPreferences(raw: string | null): ModerationPreferences {
  try {
    const value = JSON.parse(raw || 'null');
    return {
      blurEnabled: typeof value?.blurEnabled === 'boolean' ? value.blurEnabled : true,
      view: value?.view === 'cards' ? 'cards' : 'list',
    };
  } catch { return { ...defaultPreferences }; }
}

export function imageReviewKey(prefix: string, identity: string) {
  return `${prefix}seen:${encodeURIComponent(identity)}`;
}

export function readReviewedImages(storage: Storage, prefix: string): Set<string> {
  const seen = new Set<string>();
  const start = `${prefix}seen:`;
  for (let i = 0; i < storage.length; i++) {
    const key = storage.key(i);
    if (key?.startsWith(start) && storage.getItem(key) === '1') {
      try { seen.add(decodeURIComponent(key.slice(start.length))); } catch { /* Ignore malformed entries. */ }
    }
  }
  return seen;
}
