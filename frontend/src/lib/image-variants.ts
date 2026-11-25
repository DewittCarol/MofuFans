export type GalleryImageVariant = 'small' | 'thumb' | 'preview';

// Only transform this site's guarded gallery URLs. Blob previews and other images
// retain their existing behavior; original URLs remain available for explicit saves.
export function galleryImageURL(src: string | undefined, variant: GalleryImageVariant) {
  if (!src || !/^\/api\/gallery\/assets\/[a-f0-9]{32}\.(jpg|png|webp|gif)$/.test(src)) return src;
  return `${src}?variant=${variant}-v1`;
}

export function backgroundImageURL(src: string, small = false) {
  if (!/^\/api\/background-assets\/[a-f0-9]{64}\.(jpg|png|webp)$/.test(src)) return src;
  return `${src}?variant=background${small ? '-small' : ''}-v1`;
}
