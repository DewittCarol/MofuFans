'use client';

import { useEmojiCatalog } from '@/lib/emoji-catalog';
import { backgroundImageURL } from '@/lib/image-variants';

export function SiteBackground() {
  const { background } = useEmojiCatalog();
  if (!background.url) return null;
  return <div aria-hidden="true" className="pointer-events-none fixed inset-0 -z-10 bg-background" data-site-background>
    <picture className="block h-full w-full">
      <source media="(max-width: 767px)" srcSet={backgroundImageURL(background.url, true)} />
      <img src={backgroundImageURL(background.url)} alt="" decoding="async" className="h-full w-full object-cover" style={{ opacity: background.opacity / 100 }} draggable={false} />
    </picture>
  </div>;
}
